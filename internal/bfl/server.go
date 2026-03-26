package bfl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/packalares/packalares/pkg/config"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
)

// Server is the BFL API gateway server.
type Server struct {
	K8s        *K8sClient
	ListenAddr string
	mux        *http.ServeMux
}

// NewServer creates a new BFL server.
func NewServer(listenAddr string) (*Server, error) {
	k8s, err := NewK8sClient()
	if err != nil {
		return nil, fmt.Errorf("init k8s client: %w", err)
	}

	s := &Server{
		K8s:        k8s,
		ListenAddr: listenAddr,
		mux:        http.NewServeMux(),
	}
	s.registerRoutes()
	return s, nil
}

// Run starts the HTTP server.
func (s *Server) Run(ctx context.Context) error {
	srv := &http.Server{
		Addr:    s.ListenAddr,
		Handler: s.mux,
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
	}
	klog.Infof("BFL server listening on %s", s.ListenAddr)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

// ---------------------------------------------------------------------------
// Route registration
// ---------------------------------------------------------------------------

func (s *Server) registerRoutes() {
	// Backend v1
	s.mux.HandleFunc("/bfl/backend/v1/user-info", s.handleUserInfo)
	s.mux.HandleFunc("/bfl/backend/v1/terminus-info", s.handleTerminusInfo)
	s.mux.HandleFunc("/bfl/backend/v1/olares-info", s.handleOlaresInfo)
	s.mux.HandleFunc("/bfl/backend/v1/re-download-cert", s.handleReDownloadCert)
	s.mux.HandleFunc("/bfl/backend/v1/myapps", s.handleMyApps)
	s.mux.HandleFunc("/bfl/backend/v1/cluster", s.handleClusterMetrics)
	s.mux.HandleFunc("/bfl/backend/v1/config-system", s.handleGetSysConfig)

	// Info v1 (wizard info endpoint)
	s.mux.HandleFunc("/bfl/info/v1/olares-info", s.handleOlaresInfo)

	// Settings v1alpha1
	s.mux.HandleFunc("/bfl/settings/v1alpha1/activate", s.handleActivate)
	s.mux.HandleFunc("/bfl/settings/v1alpha1/binding-zone", s.handleBindingZone)
	s.mux.HandleFunc("/bfl/settings/v1alpha1/unbind-zone", s.handleUnbindZone)
	s.mux.HandleFunc("/bfl/settings/v1alpha1/config-system", s.handleConfigSystem)
	s.mux.HandleFunc("/api/settings/tailscale", s.handleTailscaleSettings)
	s.mux.HandleFunc("/api/settings/ssh", s.handleSSHSettings)
	s.mux.HandleFunc("/api/settings/updates", s.handleUpdates)
	s.mux.HandleFunc("/api/settings/updates/", s.handleUpdateRestart)

	// IAM v1alpha1
	s.mux.HandleFunc("/bfl/iam/v1alpha1/users", s.handleListUsers)
	s.mux.HandleFunc("/bfl/iam/v1alpha1/users/", s.handleUserRoutes)
	s.mux.HandleFunc("/bfl/iam/v1alpha1/roles", s.handleListRoles)

	// Health check
	s.mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
}

// ---------------------------------------------------------------------------
// JSON helpers
// ---------------------------------------------------------------------------

func respondJSON(w http.ResponseWriter, code int, data any) {
	resp := APIResponse{
		Code:    0,
		Message: "success",
		Data:    data,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(resp)
}

func respondError(w http.ResponseWriter, msg string) {
	resp := APIResponse{
		Code:    1,
		Message: msg,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK) // BFL always returns 200 with code=1 for errors
	json.NewEncoder(w).Encode(resp)
}

func respondSuccess(w http.ResponseWriter) {
	resp := APIResponse{
		Code:    0,
		Message: "success",
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// ---------------------------------------------------------------------------
// Backend v1 handlers
// ---------------------------------------------------------------------------

func (s *Server) handleUserInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, "method not allowed")
		return
	}
	ctx := r.Context()

	user, err := s.K8s.GetUser(ctx, "")
	if err != nil {
		respondError(w, fmt.Sprintf("get user: %v", err))
		return
	}

	terminusName := GetTerminusName(user)
	zone := GetUserZone(user)
	role := GetUserAnnotation(user, AnnoOwnerRole)
	createdUser := GetUserAnnotation(user, AnnoCreator)

	isEphemeral := false
	if v := GetUserAnnotation(user, AnnoIsEphemeral); v != "" {
		isEphemeral, _ = strconv.ParseBool(v)
	}

	// If creator is "cli", resolve to owner user
	if createdUser == "cli" {
		ownerUser, _ := s.findOwnerUser(ctx)
		if ownerUser != nil {
			createdUser = ownerUser.GetName()
		}
	}

	var accessLevel *int
	if level := GetUserAnnotation(user, AnnoAccessLevel); level != "" {
		if l, err := strconv.Atoi(level); err == nil {
			accessLevel = &l
		}
	}

	info := UserInfo{
		Name:           s.K8s.Username,
		OwnerRole:      role,
		TerminusName:   terminusName,
		IsEphemeral:    isEphemeral,
		Zone:           zone,
		CreatedUser:    createdUser,
		WizardComplete: IsWizardComplete(user),
		AccessLevel:    accessLevel,
	}

	respondJSON(w, http.StatusOK, info)
}

func (s *Server) handleTerminusInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, "method not allowed")
		return
	}
	ctx := r.Context()

	user, err := s.K8s.GetUser(ctx, "")
	if err != nil {
		respondError(w, fmt.Sprintf("get user: %v", err))
		return
	}

	status := GetWizardStatus(user)
	denyAllAnno := GetUserAnnotation(user, AnnoDenyAll)
	denyAll, _ := strconv.Atoi(denyAllAnno)

	info := TerminusInfo{
		TerminusName:    GetTerminusName(user),
		WizardStatus:    status,
		Selfhosted:      true, // always true in Packalares
		TailScaleEnable: denyAll == 1,
		OsVersion:       s.K8s.GetOSVersion(ctx),
		LoginBackground: GetUserAnnotation(user, AnnoLoginBackground),
		Avatar:          GetUserAnnotation(user, AnnoAvatar),
		TerminusID:      "", // no cloud ID in Packalares
		UserDID:         GetUserAnnotation(user, AnnoUserDID),
		ReverseProxy:    GetUserAnnotation(user, AnnoReverseProxyType),
		Terminusd:       "0",
		Style:           GetUserAnnotation(user, AnnoLoginBGStyle),
	}

	respondJSON(w, http.StatusOK, info)
}

func (s *Server) handleOlaresInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, "method not allowed")
		return
	}
	ctx := r.Context()

	user, err := s.K8s.GetUser(ctx, "")
	if err != nil {
		respondError(w, fmt.Sprintf("get user: %v", err))
		return
	}

	status := GetWizardStatus(user)
	denyAllAnno := GetUserAnnotation(user, AnnoDenyAll)
	denyAll, _ := strconv.Atoi(denyAllAnno)

	info := OlaresInfo{
		OlaresID:           GetTerminusName(user),
		WizardStatus:       status,
		EnableReverseProxy: true, // always selfhosted
		TailScaleEnable:    denyAll == 1,
		OsVersion:          s.K8s.GetOSVersion(ctx),
		LoginBackground:    GetUserAnnotation(user, AnnoLoginBackground),
		Avatar:             GetUserAnnotation(user, AnnoAvatar),
		ID:                 "", // no cloud terminus ID
		UserDID:            GetUserAnnotation(user, AnnoUserDID),
		Olaresd:            "0",
		Style:              GetUserAnnotation(user, AnnoLoginBGStyle),
	}

	respondJSON(w, http.StatusOK, info)
}

func (s *Server) handleReDownloadCert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, "method not allowed")
		return
	}
	ctx := r.Context()

	force := r.URL.Query().Get("force")
	from := r.Header.Get("X-FROM-CRONJOB")

	if from == "" && force != "true" {
		respondError(w, "re-download certificate: not allowed")
		return
	}

	user, err := s.K8s.GetUser(ctx, s.K8s.Username)
	if err != nil {
		respondError(w, fmt.Sprintf("re-download cert: get user: %v", err))
		return
	}

	terminusName := GetTerminusName(user)
	if terminusName == "" {
		respondError(w, "no olares name bound")
		return
	}

	tn := TerminusName(terminusName)
	zone := tn.UserZone()

	certPEM, keyPEM, err := GenerateSelfSignedCert(zone, tn)
	if err != nil {
		respondError(w, fmt.Sprintf("generate cert: %v", err))
		return
	}

	if err := s.K8s.EnsureSSLConfigMap(ctx, zone, certPEM, keyPEM); err != nil {
		respondError(w, fmt.Sprintf("update ssl configmap: %v", err))
		return
	}

	klog.Info("re-download (regenerate) cert successfully")
	respondSuccess(w)
}

func (s *Server) handleMyApps(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, "method not allowed")
		return
	}
	ctx := r.Context()

	apps, err := s.K8s.ListUserApps(ctx)
	if err != nil {
		respondError(w, fmt.Sprintf("list apps: %v", err))
		return
	}
	if apps == nil {
		apps = []AppInfo{}
	}
	respondJSON(w, http.StatusOK, NewListResult(apps, len(apps)))
}

func (s *Server) handleClusterMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, "method not allowed")
		return
	}
	metrics := s.K8s.GetClusterMetrics(r.Context())
	respondJSON(w, http.StatusOK, metrics)
}

func (s *Server) handleGetSysConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, "method not allowed")
		return
	}
	ctx := r.Context()

	user, err := s.K8s.GetUser(ctx, s.K8s.Username)
	if err != nil {
		respondError(w, fmt.Sprintf("get sys config: %v", err))
		return
	}

	cfg := GetSysConfig(user)
	respondJSON(w, http.StatusOK, cfg)
}

// ---------------------------------------------------------------------------
// Settings v1alpha1 handlers
// ---------------------------------------------------------------------------

func (s *Server) handleBindingZone(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, "method not allowed")
		return
	}
	ctx := r.Context()

	var post PostTerminusName
	if err := json.NewDecoder(r.Body).Decode(&post); err != nil {
		respondError(w, fmt.Sprintf("binding zone: %v", err))
		return
	}

	user, err := s.K8s.GetUser(ctx, s.K8s.Username)
	if err != nil {
		respondError(w, fmt.Sprintf("binding zone: get user: %v", err))
		return
	}

	// Check wizard status
	status := GetWizardStatus(user)
	if status != WaitActivateVault && status != "" {
		respondError(w, fmt.Sprintf("user wizard status invalid: %s", status))
		return
	}

	domain, err := s.K8s.GetDomain(ctx)
	if err != nil {
		respondError(w, fmt.Sprintf("get domain: %v", err))
		return
	}

	tn := NewTerminusName(s.K8s.Username, domain)
	SetUserAnnotation(user, AnnoTerminusName, string(tn))

	if post.JWSSignature != "" {
		SetUserAnnotation(user, AnnoJWSToken, post.JWSSignature)
	}
	if post.DID != "" {
		SetUserAnnotation(user, AnnoCertManagerDID, post.DID)
	}

	SetUserAnnotation(user, AnnoWizardStatus, string(WaitActivateSystem))

	if err := s.K8s.UpdateUser(ctx, user); err != nil {
		respondError(w, fmt.Sprintf("binding zone: update user: %v", err))
		return
	}

	respondSuccess(w)
}

func (s *Server) handleUnbindZone(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, "method not allowed")
		return
	}
	ctx := r.Context()

	user, err := s.K8s.GetUser(ctx, "")
	if err != nil {
		respondError(w, fmt.Sprintf("unbind zone: get user: %v", err))
		return
	}

	// Remove annotations
	DeleteUserAnnotation(user, AnnoTerminusName)
	DeleteUserAnnotation(user, AnnoZone)
	DeleteUserAnnotation(user, AnnoTaskEnableSSL)

	if err := s.K8s.UpdateUser(ctx, user); err != nil {
		respondError(w, fmt.Sprintf("unbind zone: update user: %v", err))
		return
	}

	// Delete SSL configmap (best effort)
	_ = s.K8s.Clientset.CoreV1().ConfigMaps(s.K8s.Namespace).Delete(ctx, SSLConfigMapName, deleteOpts())

	respondSuccess(w)
}

func (s *Server) handleActivate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, "method not allowed")
		return
	}
	ctx := r.Context()

	user, err := s.K8s.GetUser(ctx, "")
	if err != nil {
		respondError(w, fmt.Sprintf("activate: get user: %v", err))
		return
	}

	terminusName := GetTerminusName(user)
	if terminusName == "" {
		respondError(w, "activate: no olares name bound")
		return
	}

	// If already activated or activating, return success (idempotent)
	zone := GetUserAnnotation(user, AnnoZone)
	status := GetWizardStatus(user)
	if zone != "" || status == NetworkActivating {
		respondSuccess(w)
		return
	}

	// If previously failed, retry
	if status == NetworkActivateFailed {
		SetUserAnnotation(user, AnnoWizardStatus, string(NetworkActivating))
		if err := s.K8s.UpdateUser(ctx, user); err != nil {
			respondError(w, fmt.Sprintf("activate: update status: %v", err))
			return
		}
		respondSuccess(w)
		return
	}

	// Parse request
	var payload ActivateRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		respondError(w, fmt.Sprintf("activate: parse request: %v", err))
		return
	}

	// Save locale settings
	if payload.Language != "" {
		SetUserAnnotation(user, AnnoLanguage, payload.Language)
	}
	if payload.Location != "" {
		SetUserAnnotation(user, AnnoLocation, payload.Location)
	}
	if payload.Theme == "" {
		payload.Theme = "light"
	}
	SetUserAnnotation(user, AnnoTheme, payload.Theme)

	// Generate self-signed cert
	tn := TerminusName(terminusName)
	userZone := tn.UserZone()

	certPEM, keyPEM, err := GenerateSelfSignedCert(userZone, tn)
	if err != nil {
		respondError(w, fmt.Sprintf("activate: generate cert: %v", err))
		return
	}

	// Store cert in ConfigMap
	if err := s.K8s.EnsureSSLConfigMap(ctx, userZone, certPEM, keyPEM); err != nil {
		respondError(w, fmt.Sprintf("activate: store cert: %v", err))
		return
	}

	// Set zone annotation
	SetUserAnnotation(user, AnnoZone, userZone)

	// Set access level defaults
	if GetUserAnnotation(user, AnnoAccessLevel) == "" {
		SetUserAnnotation(user, AnnoAccessLevel, "1") // WorldWide by default
		SetUserAnnotation(user, AnnoAllowCIDR, "0.0.0.0/0")
		SetUserAnnotation(user, AnnoAuthPolicy, DefaultAuthPolicy)
	}

	// Mark as activating -> then completed (since we do local cert, no async needed)
	SetUserAnnotation(user, AnnoWizardStatus, string(WaitResetPassword))

	if err := s.K8s.UpdateUser(ctx, user); err != nil {
		respondError(w, fmt.Sprintf("activate: update user: %v", err))
		return
	}

	klog.Infof("system activated for user %s, zone=%s", s.K8s.Username, userZone)
	respondSuccess(w)
}

func (s *Server) handleTailscaleSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ns := config.FrameworkNamespace()
	secretName := "tailscale-config"

	switch r.Method {
	case http.MethodGet:
		secret, err := s.K8s.Clientset.CoreV1().Secrets(ns).Get(ctx, secretName, metav1.GetOptions{})
		if err != nil {
			respondJSON(w, http.StatusOK, map[string]interface{}{"auth_key": "", "hostname": "packalares", "control_url": ""})
			return
		}
		// Strip --login-server= prefix from extra-args to return clean URL
		controlURL := string(secret.Data["extra-args"])
		controlURL = strings.TrimPrefix(controlURL, "--login-server=")
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"auth_key":    string(secret.Data["auth-key"]),
			"hostname":    string(secret.Data["hostname"]),
			"control_url": controlURL,
		})

	case http.MethodPost:
		var req struct {
			AuthKey    string `json:"auth_key"`
			Hostname   string `json:"hostname"`
			ControlURL string `json:"control_url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondError(w, "invalid request")
			return
		}

		extraArgs := ""
		if req.ControlURL != "" {
			extraArgs = "--login-server=" + req.ControlURL
		}
		hostname := req.Hostname
		if hostname == "" {
			hostname = "packalares"
		}

		secret, err := s.K8s.Clientset.CoreV1().Secrets(ns).Get(ctx, secretName, metav1.GetOptions{})
		if err != nil {
			// Secret doesn't exist — create it
			newSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: ns},
				Type:       corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"auth-key":   []byte(req.AuthKey),
					"hostname":   []byte(hostname),
					"extra-args": []byte(extraArgs),
				},
			}
			if _, err := s.K8s.Clientset.CoreV1().Secrets(ns).Create(ctx, newSecret, metav1.CreateOptions{}); err != nil {
				respondError(w, fmt.Sprintf("create secret: %v", err))
				return
			}
			// Also create state secret
			stateSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "tailscale-state", Namespace: ns},
				Type:       corev1.SecretTypeOpaque,
			}
			s.K8s.Clientset.CoreV1().Secrets(ns).Create(ctx, stateSecret, metav1.CreateOptions{}) // best effort
		} else {
			// Secret exists — update it
			if secret.Data == nil {
				secret.Data = make(map[string][]byte)
			}
			secret.Data["auth-key"] = []byte(req.AuthKey)
			secret.Data["hostname"] = []byte(hostname)
			secret.Data["extra-args"] = []byte(extraArgs)

			if _, err := s.K8s.Clientset.CoreV1().Secrets(ns).Update(ctx, secret, metav1.UpdateOptions{}); err != nil {
				respondError(w, fmt.Sprintf("update secret: %v", err))
				return
			}
		}

		// Ensure tailscale deployment exists, deploy if missing
		_, depErr := s.K8s.Clientset.AppsV1().Deployments(ns).Get(ctx, "tailscale", metav1.GetOptions{})
		if depErr != nil {
			klog.Infof("Tailscale deployment not found, creating...")
			if err := s.createTailscaleDeployment(ctx, ns); err != nil {
				klog.Errorf("Failed to create tailscale deployment: %v", err)
			}
		} else {
			// Restart to pick up new config
			exec.CommandContext(ctx, "kubectl", "rollout", "restart", "deployment/tailscale", "-n", ns).Run()
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{"status": "OK", "message": "Tailscale config updated, restarting..."})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ---------------------------------------------------------------------------
// SSH Settings — proxies to hostctl service for read/write SSH control
// ---------------------------------------------------------------------------

func (s *Server) handleSSHSettings(w http.ResponseWriter, r *http.Request) {
	hostctlURL := envOrDefault("HOSTCTL_URL", "http://hostctl-svc:9199")
	hostctlToken := os.Getenv("HOSTCTL_TOKEN")

	switch r.Method {
	case http.MethodGet:
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, hostctlURL+"/ssh/status", nil)
		if err != nil {
			respondError(w, fmt.Sprintf("ssh status: %v", err))
			return
		}
		if hostctlToken != "" {
			req.Header.Set("Authorization", "Bearer "+hostctlToken)
		}
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			respondError(w, fmt.Sprintf("ssh status: hostctl unreachable: %v", err))
			return
		}
		defer resp.Body.Close()

		var status struct {
			Enabled bool `json:"enabled"`
			Port    int  `json:"port"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
			respondError(w, fmt.Sprintf("ssh status: decode: %v", err))
			return
		}
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"enabled":   status.Enabled,
			"port":      status.Port,
			"read_only": false,
		})

	case http.MethodPost:
		var body struct {
			Enabled bool `json:"enabled"`
			Port    int  `json:"port"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondError(w, fmt.Sprintf("ssh config: %v", err))
			return
		}

		payload, _ := json.Marshal(body)
		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, hostctlURL+"/ssh/config", bytes.NewReader(payload))
		if err != nil {
			respondError(w, fmt.Sprintf("ssh config: %v", err))
			return
		}
		req.Header.Set("Content-Type", "application/json")
		if hostctlToken != "" {
			req.Header.Set("Authorization", "Bearer "+hostctlToken)
		}
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			respondError(w, fmt.Sprintf("ssh config: hostctl unreachable: %v", err))
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			respondError(w, fmt.Sprintf("ssh config: hostctl error: %s", string(respBody)))
			return
		}

		var result struct {
			Enabled bool `json:"enabled"`
			Port    int  `json:"port"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			respondError(w, fmt.Sprintf("ssh config: decode: %v", err))
			return
		}
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"enabled":   result.Enabled,
			"port":      result.Port,
			"read_only": false,
		})

	default:
		respondError(w, "method not allowed")
	}
}

// ---------------------------------------------------------------------------
// Update management — list deployments and check GHCR for latest tags
// ---------------------------------------------------------------------------

// DeploymentUpdateInfo represents a single deployment's update status.
type DeploymentUpdateInfo struct {
	Name            string `json:"name"`
	Namespace       string `json:"namespace"`
	CurrentImage    string `json:"currentImage"`
	CurrentTag      string `json:"currentTag"`
	CurrentDigest   string `json:"currentDigest"`
	RemoteDigest    string `json:"remoteDigest"`
	UpdateAvailable bool   `json:"updateAvailable"`
}

func (s *Server) handleUpdates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, "method not allowed")
		return
	}
	ctx := r.Context()
	ns := config.FrameworkNamespace()

	deployments, err := s.K8s.Clientset.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		respondError(w, fmt.Sprintf("list deployments: %v", err))
		return
	}

	// Get running pod image digests
	pods, err := s.K8s.Clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	podDigests := make(map[string]string) // image name -> digest
	if err == nil {
		for _, pod := range pods.Items {
			for _, cs := range pod.Status.ContainerStatuses {
				// imageID is like "ghcr.io/packalares/auth@sha256:abc123..."
				if strings.Contains(cs.ImageID, "@sha256:") {
					parts := strings.SplitN(cs.ImageID, "@", 2)
					podDigests[cs.Image] = parts[1]
				}
			}
		}
	}

	var results []DeploymentUpdateInfo
	httpClient := &http.Client{Timeout: 10 * time.Second}

	for _, dep := range deployments.Items {
		for _, c := range dep.Spec.Template.Spec.Containers {
			image := c.Image
			if !strings.HasPrefix(image, "ghcr.io/packalares/") {
				continue
			}

			currentImage, currentTag := parseImageTag(image)
			shortName := strings.TrimPrefix(currentImage, "ghcr.io/packalares/")

			// Get current running digest from pod status
			currentDigest := podDigests[image]
			if currentDigest == "" {
				currentDigest = "unknown"
			}

			// Get remote digest from GHCR manifest API (requires token)
			remoteDigest := ""
			// First get an anonymous token
			tokenURL := fmt.Sprintf("https://ghcr.io/token?scope=repository:packalares/%s:pull", shortName)
			var ghcrToken string
			if tokenReq, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL, nil); err == nil {
				if tokenResp, err := httpClient.Do(tokenReq); err == nil {
					var tokenBody struct {
						Token string `json:"token"`
					}
					json.NewDecoder(tokenResp.Body).Decode(&tokenBody)
					tokenResp.Body.Close()
					ghcrToken = tokenBody.Token
				}
			}

			manifestURL := fmt.Sprintf("https://ghcr.io/v2/packalares/%s/manifests/%s", shortName, currentTag)
			req, err := http.NewRequestWithContext(ctx, http.MethodHead, manifestURL, nil)
			if err == nil {
				req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")
				if ghcrToken != "" {
					req.Header.Set("Authorization", "Bearer "+ghcrToken)
				}
				resp, err := httpClient.Do(req)
				if err == nil {
					resp.Body.Close()
					if resp.StatusCode == http.StatusOK {
						remoteDigest = resp.Header.Get("Docker-Content-Digest")
					}
				}
			}

			updateAvailable := remoteDigest != "" && currentDigest != "unknown" && remoteDigest != currentDigest

			results = append(results, DeploymentUpdateInfo{
				Name:            dep.Name,
				Namespace:       dep.Namespace,
				CurrentImage:    currentImage,
				CurrentTag:      currentTag,
				CurrentDigest:   currentDigest[:min(len(currentDigest), 19)],
				RemoteDigest:    remoteDigest[:min(len(remoteDigest), 19)],
				UpdateAvailable: updateAvailable,
			})
		}
	}

	if results == nil {
		results = []DeploymentUpdateInfo{}
	}

	respondJSON(w, http.StatusOK, results)
}

func (s *Server) handleUpdateRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, "method not allowed")
		return
	}
	ctx := r.Context()

	// Parse deployment name from URL: /api/settings/updates/{name}
	path := strings.TrimPrefix(r.URL.Path, "/api/settings/updates/")
	name := strings.TrimSuffix(path, "/")
	if name == "" {
		respondError(w, "deployment name is required")
		return
	}

	ns := config.FrameworkNamespace()

	// Verify deployment exists
	_, err := s.K8s.Clientset.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		respondError(w, fmt.Sprintf("deployment not found: %v", err))
		return
	}

	// Trigger rolling restart by patching the pod template annotation
	// This is equivalent to "kubectl rollout restart" but uses the K8s API directly
	dep, err := s.K8s.Clientset.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		respondError(w, fmt.Sprintf("get deployment: %v", err))
		return
	}
	if dep.Spec.Template.Annotations == nil {
		dep.Spec.Template.Annotations = make(map[string]string)
	}
	dep.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)
	_, err = s.K8s.Clientset.AppsV1().Deployments(ns).Update(ctx, dep, metav1.UpdateOptions{})
	if err != nil {
		respondError(w, fmt.Sprintf("rollout restart failed: %v", err))
		return
	}

	klog.Infof("triggered rolling restart for deployment/%s in %s", name, ns)
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"name":    name,
		"status":  "restarting",
		"message": fmt.Sprintf("Rolling restart triggered for %s", name),
	})
}

// parseImageTag splits "ghcr.io/packalares/foo:v1.2.3" into image and tag.
func parseImageTag(image string) (string, string) {
	// Handle digest references (image@sha256:...)
	if idx := strings.Index(image, "@"); idx >= 0 {
		return image[:idx], image[idx+1:]
	}
	// Handle tag references (image:tag)
	if idx := strings.LastIndex(image, ":"); idx >= 0 {
		// Make sure the colon is not part of the registry (e.g., ghcr.io:443/...)
		candidate := image[idx+1:]
		if !strings.Contains(candidate, "/") {
			return image[:idx], candidate
		}
	}
	return image, "latest"
}

func (s *Server) handleConfigSystem(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		s.handleGetSysConfig(w, r)
	case http.MethodPost:
		var cfg SysConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			respondError(w, fmt.Sprintf("config system: %v", err))
			return
		}
		user, err := s.K8s.GetUser(ctx, s.K8s.Username)
		if err != nil {
			respondError(w, fmt.Sprintf("config system: get user: %v", err))
			return
		}
		SaveSysConfig(user, cfg)
		if err := s.K8s.UpdateUser(ctx, user); err != nil {
			respondError(w, fmt.Sprintf("config system: update user: %v", err))
			return
		}
		respondSuccess(w)
	default:
		respondError(w, "method not allowed")
	}
}

// ---------------------------------------------------------------------------
// IAM v1alpha1 handlers
// ---------------------------------------------------------------------------

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, "method not allowed")
		return
	}
	ctx := r.Context()

	userList, err := s.K8s.ListUsers(ctx)
	if err != nil {
		respondError(w, fmt.Sprintf("list users: %v", err))
		return
	}

	users := make([]IAMUserInfo, 0)

	for _, item := range userList.Items {
		name := item.GetName()
		displayName, description, email, state := extractUserSpec(&item)

		roles, err := s.K8s.GetUserRoles(ctx, name)
		if err != nil {
			klog.Warningf("get roles for %s: %v", name, err)
			roles = []string{}
		}

		u := IAMUserInfo{
			UID:               string(item.GetUID()),
			Name:              name,
			DisplayName:       displayName,
			Description:       description,
			Email:             email,
			State:             state,
			CreationTimestamp: item.GetCreationTimestamp().Unix(),
			Roles:             roles,
			TerminusName:      GetUserAnnotation(&item, AnnoTerminusName),
			WizardComplete:    IsWizardComplete(&item),
			Avatar:            GetUserAnnotation(&item, AnnoAvatar),
			MemoryLimit:       GetUserAnnotation(&item, AnnoMemoryLimit),
			CpuLimit:          GetUserAnnotation(&item, AnnoCPULimit),
			LastLoginTime:     extractLastLoginTime(&item),
		}

		users = append(users, u)
	}

	respondJSON(w, http.StatusOK, NewListResult(users, len(users)))
}

func (s *Server) handleListRoles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, "method not allowed")
		return
	}
	roles := []string{RoleOwner, RoleAdmin, RoleOwner}
	respondJSON(w, http.StatusOK, NewListResult(roles, len(roles)))
}

// handleUserRoutes dispatches /bfl/iam/v1alpha1/users/{user}/...
func (s *Server) handleUserRoutes(w http.ResponseWriter, r *http.Request) {
	// Parse: /bfl/iam/v1alpha1/users/{user}/password  (PUT)
	// Parse: /bfl/iam/v1alpha1/users/{user}/metrics   (GET)
	// Parse: /bfl/iam/v1alpha1/users/{user}/login-records (GET)
	path := strings.TrimPrefix(r.URL.Path, "/bfl/iam/v1alpha1/users/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 {
		respondError(w, "invalid user route")
		return
	}

	userName := parts[0]
	subPath := parts[1]

	switch {
	case subPath == "password" && r.Method == http.MethodPut:
		s.handleResetPassword(w, r, userName)
	case subPath == "login-records" && r.Method == http.MethodGet:
		// Simplified: return empty list (lldap integration not needed for Packalares)
		respondJSON(w, http.StatusOK, NewListResult([]interface{}{}, 0))
	case subPath == "metrics" && r.Method == http.MethodGet:
		// Return empty metrics
		respondJSON(w, http.StatusOK, map[string]interface{}{})
	default:
		respondError(w, "not found")
	}
}

func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request, userName string) {
	ctx := r.Context()

	var pr PasswordReset
	body, err := io.ReadAll(r.Body)
	if err != nil {
		respondError(w, fmt.Sprintf("reset password: read body: %v", err))
		return
	}
	if err := json.Unmarshal(body, &pr); err != nil {
		respondError(w, fmt.Sprintf("reset password: %v", err))
		return
	}

	if pr.Password == "" {
		respondError(w, "reset password: new password is empty")
		return
	}
	if pr.Password == pr.CurrentPassword {
		respondError(w, "reset password: passwords must be different")
		return
	}

	token := r.Header.Get("X-Authorization")

	// Update wizard status if not completed
	user, err := s.K8s.GetUser(ctx, userName)
	if err != nil {
		respondError(w, fmt.Sprintf("reset password: get user: %v", err))
		return
	}

	if !IsWizardComplete(user) {
		SetUserAnnotation(user, AnnoWizardStatus, string(Completed))
		if err := s.K8s.UpdateUser(ctx, user); err != nil {
			respondError(w, fmt.Sprintf("reset password: update user: %v", err))
			return
		}
	}

	// Call auth service to reset the password
	autheliaURL := fmt.Sprintf("http://%s:9091/api/reset/%s/password", config.AuthDNS(), userName)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, autheliaURL, bytes.NewReader(body))
	if err != nil {
		respondError(w, fmt.Sprintf("reset password: create request: %v", err))
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Authorization", token)
	httpReq.Header.Set("X-BFL-USER", userName)

	httpClient := &http.Client{Timeout: 30 * time.Second}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		respondError(w, fmt.Sprintf("reset password: authelia request: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		respondError(w, fmt.Sprintf("reset password: authelia: %s", string(respBody)))
		return
	}

	klog.Infof("password reset for user %s", userName)
	respondSuccess(w)
}

// ---------------------------------------------------------------------------
// Tailscale deployment creation
// ---------------------------------------------------------------------------

func (s *Server) createTailscaleDeployment(ctx context.Context, ns string) error {
	replicas := int32(1)
	hostNetwork := true
	labels := map[string]string{"app": "tailscale"}
	optionalSecret := true

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "tailscale", Namespace: ns, Labels: labels},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					HostNetwork:        hostNetwork,
					DNSPolicy:          corev1.DNSClusterFirstWithHostNet,
					ServiceAccountName: "packalares-admin",
					Containers: []corev1.Container{{
						Name:  "tailscale",
						Image: "ghcr.io/packalares/tailscale:stable",
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"NET_ADMIN", "NET_RAW"}},
						},
						Env: []corev1.EnvVar{
							{Name: "TS_KUBE_SECRET", Value: "tailscale-state"},
							{Name: "TS_USERSPACE", Value: "false"},
							{Name: "TS_AUTH_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: "tailscale-config"}, Key: "auth-key", Optional: &optionalSecret,
							}}},
							{Name: "TS_HOSTNAME", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: "tailscale-config"}, Key: "hostname", Optional: &optionalSecret,
							}}},
							{Name: "TS_EXTRA_ARGS", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: "tailscale-config"}, Key: "extra-args", Optional: &optionalSecret,
							}}},
						},
						VolumeMounts: []corev1.VolumeMount{
							{Name: "dev-tun", MountPath: "/dev/net/tun"},
							{Name: "state", MountPath: "/var/lib/tailscale"},
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("10m"), corev1.ResourceMemory: resource.MustParse("32Mi")},
							Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("200m"), corev1.ResourceMemory: resource.MustParse("128Mi")},
						},
					}},
					Volumes: []corev1.Volume{
						{Name: "dev-tun", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/dev/net/tun"}}},
						{Name: "state", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/var/lib/packalares/tailscale-state"}}},
					},
				},
			},
		},
	}

	_, err := s.K8s.Clientset.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{})
	return err
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (s *Server) findOwnerUser(ctx context.Context) (*unstructured.Unstructured, error) {
	list, err := s.K8s.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	for _, item := range list.Items {
		if GetUserAnnotation(&item, AnnoOwnerRole) == RoleOwner {
			return &item, nil
		}
	}
	return nil, fmt.Errorf("no owner user found")
}

func deleteOpts() metav1.DeleteOptions {
	return metav1.DeleteOptions{}
}

// envOrDefault returns an env var value or a default.
func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
