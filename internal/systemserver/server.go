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

// Server is the system server that manages apps, permissions, and reverse proxying.
type Server struct {
	cfg         *Config
	perms       *PermissionManager
	providerReg *ProviderRegistry
	reconciler  *ConsumerReconciler
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

	providerReg := NewProviderRegistry()
	reconciler := NewConsumerReconciler(kubeClient, providerReg)

	return &Server{
		cfg:         cfg,
		perms:       NewPermissionManager(),
		providerReg: providerReg,
		reconciler:  reconciler,
		kubeClient:  kubeClient,
		dynClient:   dynClient,
		apps:        make(map[string]*Application),
		ctx:         ctx,
		cancel:      cancel,
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
	mux.HandleFunc("/api/v1/providers", s.handleListProviders)
	mux.HandleFunc("/api/v1/providers/", s.handleGetProviders)
	mux.HandleFunc("/api/v1/settings", s.handleSettings)
	mux.HandleFunc("/healthz", s.handleHealth)
	// Default handler: reverse proxy to installed apps based on Host header
	mux.HandleFunc("/", s.handleAppProxy)

	srv := &http.Server{
		Addr:         s.cfg.ListenAddr,
		Handler:      mux,
		ReadTimeout:  0,
		WriteTimeout: 0,
	}

	log.Printf("system-server listening on %s", s.cfg.ListenAddr)
	return srv.ListenAndServe()
}

// watchApplications watches Application CRDs and updates the in-memory app registry.
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
	log.Printf("app create/update: %s (entrances=%d, sharedEntrances=%d)", key, len(app.Spec.Entrances), len(app.Spec.SharedEntrances))

	s.appsMu.Lock()
	s.apps[key] = app
	s.appsMu.Unlock()

	// Register permissions
	if len(app.Spec.Permissions) > 0 {
		s.perms.Register(app.Spec.Name, app.Spec.Permissions)
	}

	// Register provider if app has sharedEntrances
	if len(app.Spec.SharedEntrances) > 0 {
		s.providerReg.RegisterProvider(app)

		// Notify consumers that depend on this provider's groups
		s.appsMu.RLock()
		appsCopy := make(map[string]*Application, len(s.apps))
		for k, v := range s.apps {
			appsCopy[k] = v
		}
		s.appsMu.RUnlock()

		s.reconciler.OnProviderChanged(app, appsCopy)
	}

	// If this app is a consumer, reconcile it now to pick up existing providers
	if app.Spec.Permission != nil && len(app.Spec.Permission.SysData) > 0 {
		s.appsMu.RLock()
		appsCopy := make(map[string]*Application, len(s.apps))
		for k, v := range s.apps {
			appsCopy[k] = v
		}
		s.appsMu.RUnlock()

		s.reconciler.Reconcile(app, appsCopy)
	}

	// Note: No nginx config generation needed. The Go reverse proxy
	// (handleAppProxy) routes requests based on Host header, which
	// replaces the per-app nginx server blocks entirely.
}

func (s *Server) handleAppDelete(app *Application) {
	key := app.Namespace + "/" + app.Name
	log.Printf("app delete: %s", key)

	// Unregister provider before removing from apps map so consumers
	// can still be found.
	if len(app.Spec.SharedEntrances) > 0 {
		s.providerReg.UnregisterProvider(app.Spec.Name)

		// Reconcile consumers that depended on this provider
		s.appsMu.RLock()
		appsCopy := make(map[string]*Application, len(s.apps))
		for k, v := range s.apps {
			appsCopy[k] = v
		}
		s.appsMu.RUnlock()

		s.reconciler.OnProviderChanged(app, appsCopy)
	}

	s.appsMu.Lock()
	delete(s.apps, key)
	s.appsMu.Unlock()

	// Unregister permissions
	s.perms.Unregister(app.Spec.Name)
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

// handleListProviders returns all registered providers grouped by group name.
// GET /api/v1/providers
func (s *Server) handleListProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONResponse(w, http.StatusMethodNotAllowed, map[string]interface{}{"error": "method not allowed"})
		return
	}

	// Only handle the exact path /api/v1/providers, not /api/v1/providers/{group}
	if r.URL.Path != "/api/v1/providers" {
		s.handleGetProviders(w, r)
		return
	}

	providers := s.providerReg.GetAllProviders()

	writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"code": 0,
		"data": providers,
	})
}

// handleGetProviders returns providers for a specific group.
// GET /api/v1/providers/{group}
func (s *Server) handleGetProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONResponse(w, http.StatusMethodNotAllowed, map[string]interface{}{"error": "method not allowed"})
		return
	}

	// Extract group from path: /api/v1/providers/{group}
	group := strings.TrimPrefix(r.URL.Path, "/api/v1/providers/")
	group = strings.TrimSuffix(group, "/")
	if group == "" {
		writeJSONResponse(w, http.StatusBadRequest, map[string]interface{}{"error": "group name required"})
		return
	}

	providers := s.providerReg.GetProviders(group)
	if providers == nil {
		providers = []ProviderEndpoint{}
	}

	writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"code": 0,
		"data": providers,
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
// The main nginx proxy sends *.{user.zone} traffic here.
// We extract the app name from the subdomain and proxy to the app's K8s service.
func (s *Server) handleAppProxy(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	// Extract app name from subdomain: {appname}.{user.zone}
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
	var sharedEntrance *SharedEntrance
	var matchedEntrance *Entrance
	for _, a := range s.apps {
		if a.Spec.Name == appName {
			app = a
			exists = true
			break
		}
		for i, e := range a.Spec.Entrances {
			if e.Name == appName {
				app = a
				matchedEntrance = &a.Spec.Entrances[i]
				exists = true
				break
			}
		}
		if exists {
			break
		}
	}
	// If no direct match, check shared entrances (dynamic subdomains like ComfyUI instances)
	if !exists {
		for _, a := range s.apps {
			if len(a.Spec.SharedEntrances) > 0 {
				app = a
				sharedEntrance = &a.Spec.SharedEntrances[0]
				exists = true
				break
			}
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

	// Pick the best entrance: use shared entrance if matched, otherwise prefer non-internal
	var entrance Entrance
	if sharedEntrance != nil {
		entrance = Entrance{
			Name: sharedEntrance.Name, Host: sharedEntrance.Host,
			Port: sharedEntrance.Port, AuthLevel: sharedEntrance.AuthLevel,
		}
	} else if matchedEntrance != nil {
		entrance = *matchedEntrance
	} else {
		entrance = app.Spec.Entrances[0]
		for _, e := range app.Spec.Entrances {
			if e.AuthLevel != "internal" && !e.Invisible {
				entrance = e
				break
			}
		}
	}
	// Build upstream URL from the entrance host (which is the K8s service name)
	// and the app's namespace
	svcHost := entrance.Host
	if svcHost == "" {
		svcHost = app.Spec.Name
	}
	// If the host doesn't contain dots, it's a short service name — qualify it
	ns := app.Namespace
	if app.Spec.Namespace != "" {
		ns = app.Spec.Namespace
	}
	if !strings.Contains(svcHost, ".") {
		svcHost = fmt.Sprintf("%s.%s.svc.cluster.local", svcHost, ns)
	}
	upstream := fmt.Sprintf("http://%s:%d", svcHost, entrance.Port)

	// Reverse proxy
	target, err := url.Parse(upstream)
	if err != nil {
		http.Error(w, "bad upstream", http.StatusBadGateway)
		return
	}

	// The public host for this app (e.g. ipfs.admin.olares.local)
	publicHost := appName + "." + s.cfg.UserZone

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 3 * time.Second,
		}).DialContext,
	}
	proxy.FlushInterval = -1 // flush immediately (required for SSE/streaming)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("proxy error for %s: %v", appName, err)
		http.Error(w, "upstream error", http.StatusBadGateway)
	}
	// Rewrite Location headers from internal hostnames to public subdomain
	proxy.ModifyResponse = func(resp *http.Response) error {
		loc := resp.Header.Get("Location")
		if loc != "" {
			// Replace internal service hostname with public hostname
			loc = strings.Replace(loc, "http://"+svcHost, "https://"+publicHost, 1)
			loc = strings.Replace(loc, "http://"+entrance.Host, "https://"+publicHost, 1)
			resp.Header.Set("Location", loc)
		}
		return nil
	}

	// Preserve the original browser host for apps that build URLs from it (e.g. Gradio)
	if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
		r.Host = fwdHost
	} else {
		r.Host = publicHost
	}
	proxy.ServeHTTP(w, r)
}
