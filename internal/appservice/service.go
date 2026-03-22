package appservice

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/packalares/packalares/pkg/config"
	"k8s.io/klog/v2"
)

// Service is the main app-service controller that orchestrates installs,
// uninstalls, suspends, and resumes.
type Service struct {
	helm       *HelmClient
	store      *AppStore
	k8s        *K8sClient
	lldap      *LLDAPClient
	genMgr     *GeneratedAppManager
	chartDL    *ChartDownloader
	owner      string
	namespace  string
	chartRepo  string
}

// Config holds configuration for the app-service.
type Config struct {
	DataDir      string // directory for persistent state
	Namespace    string // k8s namespace for app deployments
	ChartRepoURL string // default chart repository URL
	Owner        string // default user/owner name
	ListenAddr   string // HTTP server listen address
}

// DefaultConfig returns config populated from environment variables.
func DefaultConfig() *Config {
	cfg := &Config{
		DataDir:      "/var/lib/packalares/appservice",
		Namespace:    "user-space-admin",
		ChartRepoURL: config.ChartRepoURL(),
		Owner:        "admin",
		ListenAddr:   ":6755",
	}

	if v := os.Getenv("APP_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv("APP_NAMESPACE"); v != "" {
		cfg.Namespace = v
	}
	if v := os.Getenv("CHART_REPO_URL"); v != "" {
		cfg.ChartRepoURL = v
	}
	if v := os.Getenv("APP_OWNER"); v != "" {
		cfg.Owner = v
	}
	if v := os.Getenv("LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}

	return cfg
}

// NewService creates a new app-service instance.
func NewService(cfg *Config) (*Service, error) {
	store, err := NewAppStore(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("init app store: %w", err)
	}

	helm := NewHelmClient(cfg.Namespace, cfg.ChartRepoURL)
	k8s := NewK8sClient()
	lldap := NewLLDAPClient()

	svc := &Service{
		helm:      helm,
		store:     store,
		k8s:       k8s,
		lldap:     lldap,
		chartDL:   NewChartDownloader(),
		owner:     cfg.Owner,
		namespace: cfg.Namespace,
		chartRepo: cfg.ChartRepoURL,
	}

	svc.genMgr = NewGeneratedAppManager(helm, store, cfg.Owner)

	return svc, nil
}

// Start initializes the service: installs generated apps, starts background loops.
func (s *Service) Start(ctx context.Context) error {
	// Install generated apps
	if err := s.genMgr.EnsureGeneratedApps(ctx); err != nil {
		klog.Errorf("ensure generated apps: %v", err)
	}

	// Start LLDAP sync
	go s.lldap.StartUserSyncLoop(ctx)

	// Start status sync loop
	go s.statusSyncLoop(ctx)

	return nil
}

// statusSyncLoop periodically syncs helm release status into the app store.
func (s *Service) statusSyncLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.syncStatuses(ctx)
		}
	}
}

func (s *Service) syncStatuses(ctx context.Context) {
	releases, err := s.helm.ListReleases(ctx)
	if err != nil {
		klog.V(2).Infof("sync statuses: list releases: %v", err)
		return
	}

	releaseMap := make(map[string]*HelmRelease)
	for i := range releases {
		releaseMap[releases[i].Name] = &releases[i]
	}

	for _, rec := range s.store.List(ctx) {
		rel, ok := releaseMap[rec.ReleaseName]
		if !ok {
			// Release gone, mark as uninstalled if not already
			if rec.State != StateUninstalled && rec.State != StateUninstalling {
				rec.State = StateUninstalled
				_ = s.store.Put(ctx, rec)
			}
			continue
		}

		// Map helm status to our state
		switch rel.Status {
		case "deployed":
			if rec.State == StateInstalling || rec.State == StatePending || rec.State == StateResuming {
				rec.State = StateRunning
				_ = s.store.Put(ctx, rec)
			}
		case "failed":
			if rec.State == StateInstalling {
				rec.State = StateInstallFailed
				_ = s.store.Put(ctx, rec)
			}
		case "uninstalled":
			rec.State = StateUninstalled
			_ = s.store.Put(ctx, rec)
		}
	}
}

// Install installs an app from a chart reference.
//
// The full flow:
//  1. Download the chart from GitHub (beclab/apps) to /tmp/charts/{name}/
//  2. Parse OlaresManifest.yaml for entrances, permissions, metadata
//  3. Create the target namespace if it does not exist
//  4. Run helm install from the local chart directory
//  5. Create the Application CRD so the desktop and other services can discover the app
//  6. Return success immediately; the actual install runs in the background
func (s *Service) Install(ctx context.Context, req *InstallRequest) (*InstallationResponse, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("app name is required")
	}

	if existing, exists := s.store.Get(ctx, req.Name); exists {
		// Allow reinstall if previous attempt failed or was uninstalled
		if existing.State == StateRunning || existing.State == StateInstalling || existing.State == StateDownloading {
			return nil, fmt.Errorf("app %q is already installed (state: %s)", req.Name, existing.State)
		}
		// Clean up the old record for retry
		_ = s.store.Delete(ctx, req.Name)
	}

	source := req.Source
	if source == "" {
		source = SourceMarket
	}

	// Olares charts use Release.Name as base for service DNS names.
	// Release name must be just the app name (not appname-owner).
	releaseName := req.Name
	appID := req.Name
	if !IsSysApp(req.Name) {
		appID = Md5Short(req.Name)
	}

	opID := strconv.FormatInt(time.Now().Unix(), 10)

	rec := &AppRecord{
		Name:        req.Name,
		AppID:       appID,
		Namespace:   s.namespace,
		Owner:       s.owner,
		Source:      string(source),
		State:       StatePending,
		OpType:      OpInstall,
		OpID:        opID,
		ReleaseName: releaseName,
		ChartRef:    req.Name,
		RepoURL:     req.RepoURL,
		Values:      req.Values,
		Version:     req.Version,
		CreatedAt:   time.Now(),
	}

	if err := s.store.Put(ctx, rec); err != nil {
		return nil, fmt.Errorf("save app record: %w", err)
	}

	// Notify connected clients that install has started
	GetWSHub().BroadcastAppState(req.Name, StatePending)

	// Install in background
	go s.doInstall(rec, req)

	return &InstallationResponse{
		Response: Response{Code: 200},
		Data: InstallationResponseData{
			UID:  req.Name,
			OpID: opID,
		},
	}, nil
}

// doInstall performs the actual install in a background goroutine.
func (s *Service) doInstall(rec *AppRecord, req *InstallRequest) {
	bgCtx := context.Background()

	// --- Step 1: Download chart from GitHub ---
	rec.State = StateDownloading
	_ = s.store.Put(bgCtx, rec)
	GetWSHub().BroadcastAppState(rec.Name, StateDownloading)

	chartDir, err := s.chartDL.DownloadChart(bgCtx, req.Name)
	if err != nil {
		klog.Errorf("download chart %s: %v", req.Name, err)
		rec.State = StateInstallFailed
		_ = s.store.Put(bgCtx, rec)
		GetWSHub().BroadcastAppState(rec.Name, StateInstallFailed)
		return
	}
	defer CleanupChart(req.Name)

	// --- Step 2: Parse OlaresManifest.yaml ---
	manifest, err := ParseOlaresManifest(chartDir)
	if err != nil {
		// Manifest is optional for simple charts, log and continue
		klog.V(2).Infof("parse manifest for %s: %v (continuing without it)", req.Name, err)
	}

	// Parse Chart.yaml for version info
	chartVersion, appVersion, chartErr := ParseChartMetadata(chartDir)
	if chartErr != nil {
		klog.V(2).Infof("parse Chart.yaml for %s: %v", req.Name, chartErr)
	}

	// Populate record with metadata from the manifest
	if manifest != nil {
		rec.Title = manifest.Metadata.Title
		rec.Icon = manifest.Metadata.Icon
		rec.Description = manifest.Metadata.Description
		rec.Entrances = BuildEntrancesFromManifest(manifest, req.Name, s.owner, s.namespace)
		if rec.Version == "" {
			rec.Version = manifest.Metadata.Version
		}
	}
	if rec.Version == "" && appVersion != "" {
		rec.Version = appVersion
	}
	if rec.Version == "" && chartVersion != "" {
		rec.Version = chartVersion
	}
	_ = s.store.Put(bgCtx, rec)

	// --- Step 3: Create namespace if needed ---
	if err := s.k8s.CreateNamespace(bgCtx, s.namespace); err != nil {
		klog.V(2).Infof("ensure namespace %s: %v (may already exist)", s.namespace, err)
	}

	// --- Step 4: Write standard values into the chart's values.yaml ---
	// Instead of --set, modify values.yaml directly so ALL template references work.
	// The middleware operator (separate pod) handles DB provisioning via CRDs that
	// the chart's templates create -- we just need to supply connection info.
	zone := config.UserZone()
	domain := config.Domain()

	pgHost := config.CitusHost()
	pgPort := config.CitusPort()
	pgPass := config.CitusPassword()
	pgUser := config.CitusUser()

	redisHost := config.KVRocksHost()
	redisPort := config.KVRocksPort()
	redisPass := os.Getenv("REDIS_PASSWORD")

	middlewareBlock := map[string]interface{}{
		"postgres": map[string]interface{}{
			"host":     pgHost,
			"port":     pgPort,
			"username": pgUser,
			"password": pgPass,
		},
		"redis": map[string]interface{}{
			"host":     redisHost,
			"port":     redisPort,
			"password": redisPass,
		},
	}

	err = injectValuesYaml(chartDir, map[string]interface{}{
		"bfl": map[string]interface{}{
			"username": s.owner,
		},
		"admin": s.owner,
		"user": map[string]interface{}{
			"zone": zone,
		},
		"domain":    domain,
		"namespace": s.namespace,
		"userspace": map[string]interface{}{
			"appData":  "/terminus/userdata/appdata",
			"appCache": "/terminus/userdata/appcache",
			"userData": "/userdata",
		},
		"os": map[string]interface{}{
			"appKey": rec.AppID,
		},
		"dep": map[string]interface{}{
			"namespace":  s.namespace,
			"middleware": middlewareBlock,
		},
		// Also set top-level postgres/redis for old charts
		"postgres": map[string]interface{}{
			"host":     pgHost,
			"port":     pgPort,
			"username": pgUser,
			"password": pgPass,
		},
		"redis": map[string]interface{}{
			"host":     redisHost,
			"port":     redisPort,
			"password": redisPass,
		},
	})
	if err != nil {
		klog.Errorf("inject values.yaml for %s: %v", req.Name, err)
		rec.State = StateInstallFailed
		_ = s.store.Put(bgCtx, rec)
		GetWSHub().BroadcastAppState(rec.Name, StateInstallFailed)
		return
	}

	// --- Step 5: helm install -- NO --wait, NO --set ---
	// Let the chart create MiddlewareRequest CRDs naturally.
	// The middleware operator will provision databases.
	rec.State = StateInstalling
	_ = s.store.Put(bgCtx, rec)
	GetWSHub().BroadcastAppState(rec.Name, StateInstalling)

	if err := s.helm.InstallFromDir(bgCtx, rec.ReleaseName, chartDir, s.namespace); err != nil {
		klog.Errorf("helm install %s: %v", req.Name, err)
		rec.State = StateInstallFailed
		_ = s.store.Put(bgCtx, rec)
		GetWSHub().BroadcastAppState(rec.Name, StateInstallFailed)
		return
	}

	// --- Step 6: Create Application CRD ---
	rec.State = StateRunning
	_ = s.store.Put(bgCtx, rec)

	if err := s.k8s.ApplyApplicationCRD(bgCtx, rec); err != nil {
		klog.Errorf("apply Application CRD for %s: %v", req.Name, err)
	}

	// --- Step 7: Done ---
	GetWSHub().BroadcastAppState(rec.Name, StateRunning)
	klog.Infof("app %s installed successfully (release=%s, namespace=%s)", req.Name, rec.ReleaseName, s.namespace)
}

// Uninstall removes an installed app.
func (s *Service) Uninstall(ctx context.Context, req *UninstallRequest) (*InstallationResponse, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("app name is required")
	}

	rec, exists := s.store.Get(ctx, req.Name)
	if !exists {
		return nil, fmt.Errorf("app %q not found", req.Name)
	}

	if IsSysApp(req.Name) {
		return nil, fmt.Errorf("system app %q cannot be uninstalled", req.Name)
	}

	opID := strconv.FormatInt(time.Now().Unix(), 10)
	rec.State = StateUninstalling
	rec.OpType = OpUninstall
	rec.OpID = opID
	if err := s.store.Put(ctx, rec); err != nil {
		return nil, err
	}

	// Uninstall in background
	go func() {
		bgCtx := context.Background()

		if err := s.helm.Uninstall(bgCtx, rec.ReleaseName); err != nil {
			klog.Errorf("helm uninstall %s: %v", req.Name, err)
			rec.State = StateUninstallFailed
			_ = s.store.Put(bgCtx, rec)
			return
		}

		// Remove Application CRD
		if err := s.k8s.DeleteApplicationCRD(bgCtx, rec.ReleaseName, rec.Namespace); err != nil {
			klog.Errorf("delete Application CRD for %s: %v", req.Name, err)
		}

		rec.State = StateUninstalled
		_ = s.store.Put(bgCtx, rec)

		// Optionally remove the record entirely
		if req.DeleteData {
			_ = s.store.Delete(bgCtx, req.Name)
		}

		klog.Infof("app %s uninstalled successfully", req.Name)
	}()

	return &InstallationResponse{
		Response: Response{Code: 200},
		Data: InstallationResponseData{
			UID:  req.Name,
			OpID: opID,
		},
	}, nil
}

// Suspend scales down all deployments/statefulsets for an app.
func (s *Service) Suspend(ctx context.Context, req *SuspendRequest) (*InstallationResponse, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("app name is required")
	}

	rec, exists := s.store.Get(ctx, req.Name)
	if !exists {
		return nil, fmt.Errorf("app %q not found", req.Name)
	}

	if IsSysApp(req.Name) {
		return nil, fmt.Errorf("system app %q cannot be suspended", req.Name)
	}

	if rec.State != StateRunning {
		return nil, fmt.Errorf("app %q is not running (state: %s)", req.Name, rec.State)
	}

	opID := strconv.FormatInt(time.Now().Unix(), 10)
	rec.State = StateStopping
	rec.OpType = OpStop
	rec.OpID = opID
	if err := s.store.Put(ctx, rec); err != nil {
		return nil, err
	}

	go func() {
		bgCtx := context.Background()
		label := "app.kubernetes.io/instance=" + rec.ReleaseName

		err1 := s.k8s.ScaleDeployment(bgCtx, rec.Namespace, label, 0)
		err2 := s.k8s.ScaleStatefulSet(bgCtx, rec.Namespace, label, 0)

		if err1 != nil || err2 != nil {
			klog.Errorf("suspend %s: deployments=%v statefulsets=%v", req.Name, err1, err2)
			rec.State = StateStopFailed
			_ = s.store.Put(bgCtx, rec)
			return
		}

		rec.State = StateStopped
		_ = s.store.Put(bgCtx, rec)
		klog.Infof("app %s suspended", req.Name)
	}()

	return &InstallationResponse{
		Response: Response{Code: 200},
		Data: InstallationResponseData{
			UID:  req.Name,
			OpID: opID,
		},
	}, nil
}

// Resume scales back up all deployments/statefulsets for a suspended app.
func (s *Service) Resume(ctx context.Context, req *ResumeRequest) (*InstallationResponse, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("app name is required")
	}

	rec, exists := s.store.Get(ctx, req.Name)
	if !exists {
		return nil, fmt.Errorf("app %q not found", req.Name)
	}

	if rec.State != StateStopped {
		return nil, fmt.Errorf("app %q is not suspended (state: %s)", req.Name, rec.State)
	}

	opID := strconv.FormatInt(time.Now().Unix(), 10)
	rec.State = StateResuming
	rec.OpType = OpResume
	rec.OpID = opID
	if err := s.store.Put(ctx, rec); err != nil {
		return nil, err
	}

	go func() {
		bgCtx := context.Background()
		label := "app.kubernetes.io/instance=" + rec.ReleaseName

		err1 := s.k8s.ScaleDeployment(bgCtx, rec.Namespace, label, 1)
		err2 := s.k8s.ScaleStatefulSet(bgCtx, rec.Namespace, label, 1)

		if err1 != nil || err2 != nil {
			klog.Errorf("resume %s: deployments=%v statefulsets=%v", req.Name, err1, err2)
			rec.State = StateResumeFailed
			_ = s.store.Put(bgCtx, rec)
			return
		}

		rec.State = StateRunning
		_ = s.store.Put(bgCtx, rec)
		klog.Infof("app %s resumed", req.Name)
	}()

	return &InstallationResponse{
		Response: Response{Code: 200},
		Data: InstallationResponseData{
			UID:  req.Name,
			OpID: opID,
		},
	}, nil
}

// ListApps returns all installed apps with their current state.
func (s *Service) ListApps(ctx context.Context) []AppInfo {
	records := s.store.List(ctx)
	result := make([]AppInfo, 0, len(records))

	for _, rec := range records {
		if rec.State == StateUninstalled {
			continue // skip uninstalled apps
		}
		result = append(result, recordToInfo(rec))
	}

	return result
}

// GetApp returns a single app's info.
func (s *Service) GetApp(ctx context.Context, name string) (*AppInfo, error) {
	rec, exists := s.store.Get(ctx, name)
	if !exists {
		return nil, fmt.Errorf("app %q not found", name)
	}

	info := recordToInfo(rec)

	// Fetch live pod status
	pods := s.k8s.GetPodsForApp(ctx, rec.ReleaseName, rec.Namespace)
	_ = pods // Pod info available via separate endpoint if needed

	return &info, nil
}

func recordToInfo(rec *AppRecord) AppInfo {
	return AppInfo{
		Name:        rec.Name,
		AppID:       rec.AppID,
		Namespace:   rec.Namespace,
		Owner:       rec.Owner,
		Icon:        rec.Icon,
		Title:       rec.Title,
		Description: rec.Description,
		Version:     rec.Version,
		State:       rec.State,
		Source:      rec.Source,
		Entrances:   rec.Entrances,
		CreatedAt:   rec.CreatedAt,
		UpdatedAt:   rec.UpdatedAt,
	}
}
