package systemserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Server is the system server that manages apps, nginx configs, and permissions.
type Server struct {
	cfg         *Config
	nginx       *NginxGenerator
	perms       *PermissionManager
	kubeClient  kubernetes.Interface
	dynClient   dynamic.Interface
	apps        map[string]*Application // keyed by namespace/name
	appsMu      sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
}

func NewServer(cfg *Config) (*Server, error) {
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("get in-cluster config: %w", err)
	}

	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("create kubernetes client: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("create dynamic client: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Server{
		cfg:        cfg,
		nginx:      NewNginxGenerator(cfg.NginxConfigPath, cfg.NginxReloadCmd, cfg.UserZone),
		perms:      NewPermissionManager(),
		kubeClient: kubeClient,
		dynClient:  dynClient,
		apps:       make(map[string]*Application),
		ctx:        ctx,
		cancel:     cancel,
	}, nil
}

// Run starts both the API server and the CRD watcher.
func (s *Server) Run() error {
	// Start the Application CRD watcher
	go func() {
		for {
			if err := s.watchApplications(); err != nil {
				log.Printf("application watch error: %v, retrying in 5s", err)
				select {
				case <-s.ctx.Done():
					return
				case <-time.After(5 * time.Second):
				}
			}
		}
	}()

	// Start the HTTP API server
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/apps", s.handleListApps)
	mux.HandleFunc("/api/v1/apps/", s.handleGetApp)
	mux.HandleFunc("/api/v1/permissions", s.handleListPermissions)
	mux.HandleFunc("/api/v1/permissions/check", s.handleCheckPermission)
	mux.HandleFunc("/api/v1/settings", s.handleSettings)
	mux.HandleFunc("/api/v1/nginx/reload", s.handleNginxReload)
	mux.HandleFunc("/healthz", s.handleHealth)
	// Default handler: reverse proxy to installed apps based on Host header
	mux.HandleFunc("/", s.handleAppProxy)

	srv := &http.Server{
		Addr:         s.cfg.ListenAddr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	log.Printf("system-server listening on %s", s.cfg.ListenAddr)
	return srv.ListenAndServe()
}

// watchApplications watches Application CRDs and generates nginx configs.
func (s *Server) watchApplications() error {
	resource := s.dynClient.Resource(ApplicationGVR)

	var watcher watch.Interface
	var err error

	if s.cfg.WatchNamespace != "" {
		watcher, err = resource.Namespace(s.cfg.WatchNamespace).Watch(s.ctx, metav1.ListOptions{})
	} else {
		watcher, err = resource.Watch(s.ctx, metav1.ListOptions{})
	}
	if err != nil {
		return fmt.Errorf("watch applications: %w", err)
	}
	defer watcher.Stop()

	// Reconcile existing
	if err := s.reconcileExistingApps(); err != nil {
		log.Printf("reconcile existing apps: %v", err)
	}

	for {
		select {
		case <-s.ctx.Done():
			return s.ctx.Err()
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return fmt.Errorf("watch channel closed")
			}

			if event.Object == nil {
				continue
			}

			unObj, ok := event.Object.(*unstructured.Unstructured)
			if !ok {
				continue
			}

			app, err := s.parseApplication(unObj)
			if err != nil {
				log.Printf("parse application: %v", err)
				continue
			}

			switch event.Type {
			case watch.Added, watch.Modified:
				s.handleAppCreateOrUpdate(app)
			case watch.Deleted:
				s.handleAppDelete(app)
			}
		}
	}
}

func (s *Server) reconcileExistingApps() error {
	resource := s.dynClient.Resource(ApplicationGVR)

	var list *unstructured.UnstructuredList
	var err error

	if s.cfg.WatchNamespace != "" {
		list, err = resource.Namespace(s.cfg.WatchNamespace).List(s.ctx, metav1.ListOptions{})
	} else {
		list, err = resource.List(s.ctx, metav1.ListOptions{})
	}
	if err != nil {
		return fmt.Errorf("list applications: %w", err)
	}

	for i := range list.Items {
		app, err := s.parseApplication(&list.Items[i])
		if err != nil {
			log.Printf("parse existing application: %v", err)
			continue
		}
		s.handleAppCreateOrUpdate(app)
	}

	return nil
}

func (s *Server) parseApplication(obj *unstructured.Unstructured) (*Application, error) {
	data, err := json.Marshal(obj.Object)
	if err != nil {
		return nil, err
	}

	var app Application
	if err := json.Unmarshal(data, &app); err != nil {
		return nil, err
	}

	return &app, nil
}

func (s *Server) handleAppCreateOrUpdate(app *Application) {
	key := app.Namespace + "/" + app.Name
	log.Printf("app create/update: %s (entrances=%d)", key, len(app.Spec.Entrances))

	s.appsMu.Lock()
	s.apps[key] = app
	s.appsMu.Unlock()

	// Generate nginx configs
	if err := s.nginx.GenerateForApp(app); err != nil {
		log.Printf("generate nginx for %s: %v", key, err)
		return
	}

	// Inject Envoy sidecar for per-app auth (if not already injected)
	if err := s.injectEnvoySidecar(app); err != nil {
		log.Printf("inject envoy sidecar for %s: %v", key, err)
		// Non-fatal — app still works via proxy auth
	}

	// Register permissions
	if len(app.Spec.Permissions) > 0 {
		s.perms.Register(app.Spec.Name, app.Spec.Permissions)
	}

	// Reload nginx
	if err := s.nginx.Reload(); err != nil {
		log.Printf("nginx reload after %s: %v", key, err)
	}
}

func (s *Server) handleAppDelete(app *Application) {
	key := app.Namespace + "/" + app.Name
	log.Printf("app delete: %s", key)

	s.appsMu.Lock()
	delete(s.apps, key)
	s.appsMu.Unlock()

	// Remove nginx configs
	if err := s.nginx.RemoveForApp(app.Spec.Name); err != nil {
		log.Printf("remove nginx for %s: %v", key, err)
	}

	// Unregister permissions
	s.perms.Unregister(app.Spec.Name)

	// Reload nginx
	if err := s.nginx.Reload(); err != nil {
		log.Printf("nginx reload after delete %s: %v", key, err)
	}
}

// HTTP API handlers

func (s *Server) handleListApps(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONResponse(w, http.StatusMethodNotAllowed, map[string]interface{}{"error": "method not allowed"})
		return
	}

	s.appsMu.RLock()
	apps := make([]map[string]interface{}, 0, len(s.apps))
	for _, app := range s.apps {
		entrances := make([]map[string]interface{}, 0, len(app.Spec.Entrances))
		for _, e := range app.Spec.Entrances {
			entrances = append(entrances, map[string]interface{}{
				"name":      e.Name,
				"host":      e.Host,
				"port":      e.Port,
				"title":     e.Title,
				"icon":      e.Icon,
				"authLevel": e.AuthLevel,
			})
		}
		apps = append(apps, map[string]interface{}{
			"name":       app.Spec.Name,
			"namespace":  app.Spec.Namespace,
			"owner":      app.Spec.Owner,
			"entrances":  entrances,
			"state":      app.Status.State,
		})
	}
	s.appsMu.RUnlock()

	writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"code": 0,
		"data": apps,
	})
}

func (s *Server) handleGetApp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONResponse(w, http.StatusMethodNotAllowed, map[string]interface{}{"error": "method not allowed"})
		return
	}

	// Extract app name from path: /api/v1/apps/{name}
	appName := r.URL.Path[len("/api/v1/apps/"):]
	if appName == "" {
		writeJSONResponse(w, http.StatusBadRequest, map[string]interface{}{"error": "app name required"})
		return
	}

	s.appsMu.RLock()
	var found *Application
	for _, app := range s.apps {
		if app.Spec.Name == appName {
			found = app
			break
		}
	}
	s.appsMu.RUnlock()

	if found == nil {
		writeJSONResponse(w, http.StatusNotFound, map[string]interface{}{"error": "app not found"})
		return
	}

	writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"code": 0,
		"data": found,
	})
}

func (s *Server) handleListPermissions(w http.ResponseWriter, r *http.Request) {
	apps := s.perms.ListApps()
	result := make(map[string]interface{})
	for _, app := range apps {
		perms, _ := s.perms.GetPermissions(app)
		result[app] = perms
	}

	writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"code": 0,
		"data": result,
	})
}

func (s *Server) handleCheckPermission(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONResponse(w, http.StatusMethodNotAllowed, map[string]interface{}{"error": "method not allowed"})
		return
	}

	var req struct {
		App      string `json:"app"`
		Group    string `json:"group"`
		DataType string `json:"dataType"`
		Version  string `json:"version"`
		Op       string `json:"op"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONResponse(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid request"})
		return
	}

	if err := s.perms.CheckPermission(req.App, req.Group, req.DataType, req.Version, req.Op); err != nil {
		writeJSONResponse(w, http.StatusForbidden, map[string]interface{}{
			"code":    403,
			"message": err.Error(),
		})
		return
	}

	writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"code":    0,
		"message": "allowed",
	})
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSONResponse(w, http.StatusOK, map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"user_zone": s.cfg.UserZone,
				"username":  s.cfg.Username,
				"namespace": s.cfg.UserNamespace,
			},
		})
	default:
		writeJSONResponse(w, http.StatusMethodNotAllowed, map[string]interface{}{"error": "method not allowed"})
	}
}

func (s *Server) handleNginxReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONResponse(w, http.StatusMethodNotAllowed, map[string]interface{}{"error": "method not allowed"})
		return
	}

	if err := s.nginx.Reload(); err != nil {
		writeJSONResponse(w, http.StatusInternalServerError, map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"code":    0,
		"message": "nginx reloaded",
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSONResponse(w, http.StatusOK, map[string]interface{}{"status": "ok"})
}

// Stop gracefully stops the server.
func (s *Server) Stop() {
	s.cancel()
}

func writeJSONResponse(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}

// handleAppProxy reverse-proxies requests to installed apps based on the Host header.
// The main nginx proxy sends *.laurs.olares.local traffic here.
// We extract the app name from the subdomain and proxy to the app's K8s service.
func (s *Server) handleAppProxy(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	// Extract app name from subdomain: {appname}.laurs.olares.local
	parts := strings.SplitN(host, ".", 2)
	if len(parts) < 2 {
		http.Error(w, "unknown host", http.StatusNotFound)
		return
	}
	appName := parts[0]

	// Look up the app in our registry (apps are stored with namespace/name key)
	s.appsMu.RLock()
	var app *Application
	var exists bool
	for _, a := range s.apps {
		if a.Spec.Name == appName {
			app = a
			exists = true
			break
		}
	}
	s.appsMu.RUnlock()

	if !exists {
		http.Error(w, fmt.Sprintf("app %q not found", appName), http.StatusNotFound)
		return
	}

	// Find the upstream URL from the app's entrances
	if len(app.Spec.Entrances) == 0 {
		http.Error(w, "no entrances configured", http.StatusBadGateway)
		return
	}

	entrance := app.Spec.Entrances[0]
	// Build upstream URL: {servicename}.{namespace}.svc.cluster.local:{port}
	upstream := fmt.Sprintf("http://%s.%s.svc.cluster.local:%d",
		app.Spec.Name, app.Namespace, entrance.Port)

	// Reverse proxy
	target, err := url.Parse(upstream)
	if err != nil {
		http.Error(w, "bad upstream", http.StatusBadGateway)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("proxy error for %s: %v", appName, err)
		http.Error(w, "upstream error", http.StatusBadGateway)
	}

	r.Host = target.Host
	proxy.ServeHTTP(w, r)
}
