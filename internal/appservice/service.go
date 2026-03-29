package appservice

import (
	"context"
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/packalares/packalares/pkg/config"
	"k8s.io/klog/v2"
)

// Service is the main app-service controller that orchestrates installs,
// uninstalls, suspends, and resumes.
type Service struct {
	helm           *HelmClient
	store          *AppStore
	k8s            *K8sClient
	lldap          *LLDAPClient
	genMgr         *GeneratedAppManager
	chartDL        *ChartDownloader
	owner          string
	namespace      string
	chartRepo      string
	modelBackends  map[string]ModelBackend
	activeModelMu  sync.Mutex
	activeModelOps map[string]string // name → state ("downloading", "uninstalling")
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
		Namespace:    config.UserNamespace(config.Username()),
		ChartRepoURL: config.ChartRepoURL(),
		Owner:        config.Username(),
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

	// Initialize model backends
	ollamaURL := fmt.Sprintf("http://ollama.%s:11434", cfg.Namespace)
	svc.modelBackends = map[string]ModelBackend{
		"ollama": NewOllamaBackend(ollamaURL),
		"vllm":   NewVLLMBackend(helm, cfg.Namespace),
	}
	svc.activeModelOps = make(map[string]string)

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
			// Release gone — mark as uninstalled, but preserve failed states
			// so the user can see what went wrong
			if rec.State != StateUninstalled && rec.State != StateUninstalling &&
				rec.State != StateInstallFailed && rec.State != StateUninstallFailed {
				rec.State = StateUninstalled
				_ = s.store.Put(ctx, rec)
			}
			continue
		}

		// Map helm status to our state
		switch rel.Status {
		case "deployed":
			if rec.State != StateRunning && rec.State != StateUninstalling {
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

	// --- Step 1: Download chart ---
	rec.State = StateDownloading
	_ = s.store.Put(bgCtx, rec)
	GetWSHub().BroadcastAppState(rec.Name, StateDownloading)
	GetWSHub().BroadcastInstallProgress(rec.Name, StateDownloading, 1, 6, "Downloading chart...", 0, 0)

	chartDir, err := s.chartDL.DownloadChart(bgCtx, req.Name)
	if err != nil {
		klog.Errorf("download chart %s: %v", req.Name, err)
		rec.State = StateInstallFailed
		_ = s.store.Put(bgCtx, rec)
		GetWSHub().BroadcastAppState(rec.Name, StateInstallFailed)
		GetWSHub().BroadcastInstallProgress(rec.Name, StateInstallFailed, 1, 6, fmt.Sprintf("Download failed: %v", err), 0, 0)
		return
	}
	defer CleanupChart(req.Name)

	GetWSHub().BroadcastInstallProgress(rec.Name, StateDownloading, 2, 6, "Parsing manifest...", 0, 0)

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
		// Use locally cached icon if available, otherwise keep CDN URL
		if manifest.Metadata.Icon != "" {
			rec.Icon = "/api/market/icons/" + req.Name + ".png"
		}
		rec.Description = manifest.Metadata.Description
		rec.Entrances = BuildEntrancesFromManifest(manifest, req.Name, s.owner, s.namespace)
		if rec.Version == "" {
			rec.Version = manifest.Metadata.Version
		}

		// Store sharedEntrances from the manifest
		if len(manifest.SharedEntrances) > 0 {
			rec.SharedEntrances = manifest.SharedEntrances
		}

		// Store permission from the manifest (includes sysData and provider)
		rec.Permission = &manifest.Permission
	}
	if rec.Version == "" && appVersion != "" {
		rec.Version = appVersion
	}
	if rec.Version == "" && chartVersion != "" {
		rec.Version = chartVersion
	}
	_ = s.store.Put(bgCtx, rec)

	GetWSHub().BroadcastInstallProgress(rec.Name, StateDownloading, 3, 6, "Preparing namespace...", 0, 0)

	// --- Step 3: Create namespace if needed ---
	if err := s.k8s.CreateNamespace(bgCtx, s.namespace); err != nil {
		klog.V(2).Infof("ensure namespace %s: %v (may already exist)", s.namespace, err)
	}

	// --- Step 4: Write standard values into the chart's values.yaml ---
	// Instead of --set, modify values.yaml directly so ALL template references work.
	// The middleware operator (separate pod) handles DB provisioning via CRDs that
	// the chart's templates create -- we just need to supply connection info.
	zone := config.UserZone()

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

	// Build domain map: each entrance name maps to its full domain
	// e.g. domain.gitea = "gitea.admin.olares.local"
	// Charts use: .Values.domain.<entranceName>
	domainMap := map[string]interface{}{}
	if manifest != nil {
		for _, e := range manifest.Entrances {
			eName := e.Name
			if eName == "" {
				eName = req.Name
			}
			domainMap[eName] = fmt.Sprintf("%s.%s", eName, zone)
		}
	}
	// Always add a default entry with the app name
	if _, ok := domainMap[req.Name]; !ok {
		domainMap[req.Name] = fmt.Sprintf("%s.%s", req.Name, zone)
	}

	// Database name for the app (Olares pattern: app name as db name)
	dbName := req.Name

	err = injectValuesYaml(chartDir, map[string]interface{}{
		"bfl": map[string]interface{}{
			"username": s.owner,
		},
		"admin": s.owner,
		"user": map[string]interface{}{
			"zone": zone,
		},
		"domain":    domainMap,
		"namespace": s.namespace,
		"sysVersion": "1.12.0",
		"userspace": map[string]interface{}{
			"appData":  "/packalares/data/appdata",
			"appCache": "/packalares/data/appcache",
			"userData": "/packalares/data",
		},
		"os": map[string]interface{}{
			"appKey":    rec.AppID,
			"appSecret": fmt.Sprintf("%x", md5.Sum([]byte(rec.AppID+"secret")))[:16],
		},
		"dep": map[string]interface{}{
			"namespace":  s.namespace,
			"middleware": middlewareBlock,
		},
		"postgres": map[string]interface{}{
			"host":     pgHost,
			"port":     pgPort,
			"username": pgUser,
			"password": pgPass,
			"databases": map[string]interface{}{
				dbName: dbName,
			},
		},
		"redis": map[string]interface{}{
			"host":     redisHost,
			"port":     redisPort,
			"password": redisPass,
		},
		"olaresEnv": map[string]interface{}{
			"OLARES_USER_HUGGINGFACE_SERVICE": os.Getenv("OLARES_USER_HUGGINGFACE_SERVICE"),
			"OLARES_USER_HUGGINGFACE_TOKEN":   os.Getenv("OLARES_USER_HUGGINGFACE_TOKEN"),
		},
		"sharedlib":      "/packalares/data/sharedlib",
		"downloadCdnURL": "https://cdn.olares.com",
		"gpu":            "",
		"GPU":            map[string]interface{}{},
		"mysql": map[string]interface{}{
			"host": pgHost, "port": "3306",
			"username": pgUser, "password": pgPass,
			"databases": map[string]interface{}{dbName: dbName},
		},
		"mongodb": map[string]interface{}{
			"host": "", "port": "27017",
			"username": "", "password": "",
			"databases": map[string]interface{}{dbName: dbName},
		},
		"mariadb": map[string]interface{}{
			"host": pgHost, "port": "3306",
			"username": pgUser, "password": pgPass,
			"databases": map[string]interface{}{dbName: dbName},
		},
		"minio": map[string]interface{}{
			"host": "", "port": "9000",
			"username": "", "password": "",
			"buckets": map[string]interface{}{dbName: dbName},
		},
	})
	if err != nil {
		klog.Errorf("inject values.yaml for %s: %v", req.Name, err)
		rec.State = StateInstallFailed
		_ = s.store.Put(bgCtx, rec)
		GetWSHub().BroadcastAppState(rec.Name, StateInstallFailed)
		return
	}

	// --- Step 4b: Move subcharts into charts/ directory for Helm discovery ---
	restructureSubcharts(chartDir)

	// --- Step 4c: Inject values into subcharts too ---
	// Helm subcharts don't inherit parent values unless explicitly passed.
	// Inject the same values into each subchart's values.yaml.
	chartsSubdir := filepath.Join(chartDir, "charts")
	if subEntries, err := os.ReadDir(chartsSubdir); err == nil {
		for _, entry := range subEntries {
			if entry.IsDir() {
				subValuesOverrides := map[string]interface{}{
					"domain":    domainMap,
					"admin":     s.owner,
					"namespace": s.namespace,
					"bfl":       map[string]interface{}{"username": s.owner},
					"user":      map[string]interface{}{"zone": zone},
					"userspace": map[string]interface{}{
						"appData":  "/packalares/data/appdata",
						"appCache": "/packalares/data/appcache",
						"userData": "/packalares/data",
					},
					"postgres": map[string]interface{}{
						"host": pgHost, "port": pgPort,
						"username": pgUser, "password": pgPass,
						"databases": map[string]interface{}{dbName: dbName},
					},
					"redis": map[string]interface{}{
						"host": redisHost, "port": redisPort, "password": redisPass,
					},
					"olaresEnv": map[string]interface{}{
						"OLARES_USER_HUGGINGFACE_SERVICE": os.Getenv("OLARES_USER_HUGGINGFACE_SERVICE"),
						"OLARES_USER_HUGGINGFACE_TOKEN":   os.Getenv("OLARES_USER_HUGGINGFACE_TOKEN"),
					},
					"sharedlib":      "/packalares/data/sharedlib",
					"downloadCdnURL": "https://cdn.olares.com",
					"gpu":            "",
					"GPU":            map[string]interface{}{},
					"os": map[string]interface{}{
						"appKey":    rec.AppID,
						"appSecret": fmt.Sprintf("%x", md5.Sum([]byte(rec.AppID+"secret")))[:16],
					},
					"mysql": map[string]interface{}{
						"host": pgHost, "port": "3306",
						"username": pgUser, "password": pgPass,
						"databases": map[string]interface{}{dbName: dbName},
					},
					"mongodb": map[string]interface{}{
						"host": "", "port": "27017",
						"username": "", "password": "",
						"databases": map[string]interface{}{dbName: dbName},
					},
					"mariadb": map[string]interface{}{
						"host": pgHost, "port": "3306",
						"username": pgUser, "password": pgPass,
						"databases": map[string]interface{}{dbName: dbName},
					},
					"minio": map[string]interface{}{
						"host": "", "port": "9000",
						"username": "", "password": "",
						"buckets": map[string]interface{}{dbName: dbName},
					},
				}
				subDir := filepath.Join(chartsSubdir, entry.Name())
				if err := injectValuesYaml(subDir, subValuesOverrides); err != nil {
					klog.V(2).Infof("inject subchart values %s: %v", entry.Name(), err)
				}
			}
		}
	}

	// --- Step 5: Pre-pull container images ---
	rec.State = StateInstalling
	_ = s.store.Put(bgCtx, rec)
	GetWSHub().BroadcastAppState(rec.Name, StateInstalling)
	GetWSHub().BroadcastInstallProgress(rec.Name, StateInstalling, 4, 6, "Pre-pulling images...", 0, 0)

	// Try helm template first for accurate image list, fall back to static extraction
	templateImages := ExtractImagesFromTemplateOutput(bgCtx, chartDir, s.namespace)
	if len(templateImages) > 0 {
		klog.Infof("chart %s: helm template found %d images", req.Name, len(templateImages))
		// Merge with statically extracted images
		staticImages := ExtractImagesFromChart(chartDir)
		mergedSet := make(map[string]bool)
		for _, img := range templateImages {
			mergedSet[img] = true
		}
		for _, img := range staticImages {
			mergedSet[img] = true
		}
		allImages := make([]string, 0, len(mergedSet))
		for img := range mergedSet {
			allImages = append(allImages, img)
		}
		missing := FilterMissingImages(bgCtx, allImages)
		if len(missing) > 0 {
			PullImagesWithProgress(bgCtx, missing, func(pulled, total int, currentImage string, bytesDownloaded, bytesTotal int64) {
				short := currentImage
				if idx := strings.LastIndex(short, "/"); idx >= 0 {
					short = short[idx+1:]
				}
				detail := fmt.Sprintf("Pulling images (%d/%d) %s", pulled, total, short)
				GetWSHub().BroadcastInstallProgress(rec.Name, StateInstalling, 4, 6, detail, bytesDownloaded, bytesTotal)
			})
		}
	} else {
		PullImagesForChart(bgCtx, rec.Name, chartDir, 4, 6)
	}

	// --- Step 6: helm install ---
	GetWSHub().BroadcastInstallProgress(rec.Name, StateInstalling, 5, 6, "Running helm install...", 0, 0)

	if err := s.helm.InstallFromDir(bgCtx, rec.ReleaseName, chartDir, s.namespace); err != nil {
		klog.Errorf("helm install %s: %v", req.Name, err)
		rec.State = StateInstallFailed
		_ = s.store.Put(bgCtx, rec)
		GetWSHub().BroadcastAppState(rec.Name, StateInstallFailed)
		GetWSHub().BroadcastInstallProgress(rec.Name, StateInstallFailed, 5, 6, fmt.Sprintf("Install failed: %v", err), 0, 0)
		return
	}

	// --- Step 7: Create Application CRD ---
	GetWSHub().BroadcastInstallProgress(rec.Name, StateInstalling, 6, 6, "Registering app...", 0, 0)
	rec.State = StateRunning
	_ = s.store.Put(bgCtx, rec)

	if err := s.k8s.ApplyApplicationCRD(bgCtx, rec); err != nil {
		klog.Errorf("apply Application CRD for %s: %v", req.Name, err)
	}

	// --- Done ---
	GetWSHub().BroadcastAppState(rec.Name, StateRunning)
	GetWSHub().BroadcastInstallProgress(rec.Name, StateRunning, 6, 6, "Installed successfully", 0, 0)
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

		GetWSHub().BroadcastAppState(rec.Name, StateUninstalling)
		GetWSHub().BroadcastInstallProgress(rec.Name, StateUninstalling, 1, 3, "Removing helm release...", 0, 0)

		// Collect images from running pods AND from helm manifest
		appImages := s.k8s.GetImagesForApp(bgCtx, rec.ReleaseName, rec.Namespace)
		// Also get images from helm manifest (in case pods are already gone)
		manifestImages := s.helm.GetImagesFromManifest(bgCtx, rec.ReleaseName)
		for _, img := range manifestImages {
			found := false
			for _, existing := range appImages {
				if existing == img { found = true; break }
			}
			if !found { appImages = append(appImages, img) }
		}

		if err := s.helm.Uninstall(bgCtx, rec.ReleaseName); err != nil {
			klog.Errorf("helm uninstall %s: %v", req.Name, err)
			rec.State = StateUninstallFailed
			_ = s.store.Put(bgCtx, rec)
			GetWSHub().BroadcastAppState(rec.Name, StateUninstallFailed)
			return
		}

		GetWSHub().BroadcastInstallProgress(rec.Name, StateUninstalling, 2, 3, "Cleaning up...", 0, 0)

		// Remove Application CRD
		if err := s.k8s.DeleteApplicationCRD(bgCtx, rec.ReleaseName, rec.Namespace); err != nil {
			klog.Errorf("delete Application CRD for %s: %v", req.Name, err)
		}

		GetWSHub().BroadcastInstallProgress(rec.Name, StateUninstalling, 3, 4, "Waiting for pods to terminate...", 0, 0)

		// Wait for all app pods to be fully gone before removing images,
		// otherwise kubelet re-pulls them for the terminating pods.
		for i := 0; i < 60; i++ {
			pods := s.k8s.GetPodsForApp(bgCtx, rec.ReleaseName, rec.Namespace)
			if len(pods) == 0 {
				break
			}
			time.Sleep(time.Second)
		}

		GetWSHub().BroadcastInstallProgress(rec.Name, StateUninstalling, 4, 4, "Removing images...", 0, 0)
		purgeContainerImages(bgCtx, appImages)

		rec.State = StateUninstalled
		_ = s.store.Put(bgCtx, rec)
		GetWSHub().BroadcastAppState(rec.Name, StateUninstalled)
		GetWSHub().BroadcastInstallProgress(rec.Name, StateUninstalled, 3, 3, "Uninstalled", 0, 0)

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

	GetWSHub().BroadcastAppState(req.Name, StateStopping)

	go func() {
		bgCtx := context.Background()
		label := "app.kubernetes.io/instance=" + rec.ReleaseName

		err1 := s.k8s.ScaleDeployment(bgCtx, rec.Namespace, label, 0)
		err2 := s.k8s.ScaleStatefulSet(bgCtx, rec.Namespace, label, 0)

		if err1 != nil || err2 != nil {
			klog.Errorf("suspend %s: deployments=%v statefulsets=%v", req.Name, err1, err2)
			rec.State = StateStopFailed
			_ = s.store.Put(bgCtx, rec)
			GetWSHub().BroadcastAppState(req.Name, StateStopFailed)
			return
		}

		rec.State = StateStopped
		_ = s.store.Put(bgCtx, rec)
		GetWSHub().BroadcastAppState(req.Name, StateStopped)
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

	GetWSHub().BroadcastAppState(req.Name, StateResuming)

	go func() {
		bgCtx := context.Background()
		label := "app.kubernetes.io/instance=" + rec.ReleaseName

		err1 := s.k8s.ScaleDeployment(bgCtx, rec.Namespace, label, 1)
		err2 := s.k8s.ScaleStatefulSet(bgCtx, rec.Namespace, label, 1)

		if err1 != nil || err2 != nil {
			klog.Errorf("resume %s: deployments=%v statefulsets=%v", req.Name, err1, err2)
			rec.State = StateResumeFailed
			_ = s.store.Put(bgCtx, rec)
			GetWSHub().BroadcastAppState(req.Name, StateResumeFailed)
			return
		}

		rec.State = StateRunning
		_ = s.store.Put(bgCtx, rec)
		GetWSHub().BroadcastAppState(req.Name, StateRunning)
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
		info := recordToInfo(rec)

		// Check actual pod status — override state if pod isn't ready
		if info.State == "running" {
			pods := s.k8s.GetPodsForApp(ctx, rec.ReleaseName, rec.Namespace)
			if len(pods) == 0 {
				info.State = "pending"
				info.StatusMessage = "No pods found"
			} else {
				for _, p := range pods {
					switch p.Status {
					case "Pending":
						info.State = "pending"
						info.StatusMessage = "Pod pending (possibly insufficient resources)"
					case "CrashLoopBackOff", "Error", "OOMKilled":
						info.State = "failed"
						info.StatusMessage = p.Status
					case "ContainerCreating", "PodInitializing":
						info.State = "starting"
						info.StatusMessage = p.Status
					case "Terminating":
						info.State = "stopping"
					}
				}
			}
		}

		result = append(result, info)
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

// InstallModel installs a model via the appropriate backend.
// The install runs in a background goroutine; progress is broadcast via WebSocket.
func (s *Service) InstallModel(ctx context.Context, spec ModelSpec) error {
	backend, ok := s.modelBackends[spec.Backend]
	if !ok {
		return fmt.Errorf("unknown model backend %q (available: ollama, vllm)", spec.Backend)
	}

	wsHub := GetWSHub()
	wsHub.BroadcastAppState(spec.Name, StateDownloading)
	s.activeModelMu.Lock()
	s.activeModelOps[spec.Name] = string(StateDownloading)
	s.activeModelMu.Unlock()

	go func() {
		defer func() {
			s.activeModelMu.Lock()
			delete(s.activeModelOps, spec.Name)
			s.activeModelMu.Unlock()
		}()
		if err := backend.Install(context.Background(), spec, wsHub); err != nil {
			klog.Errorf("model install %s (%s): %v", spec.Name, spec.Backend, err)
			wsHub.BroadcastAppState(spec.Name, StateInstallFailed)
			wsHub.BroadcastInstallProgress(spec.Name, StateInstallFailed, 1, 1,
				fmt.Sprintf("Install failed: %v", err), 0, 0)
		}
	}()

	return nil
}

// UninstallModel removes a model via the appropriate backend.
func (s *Service) UninstallModel(ctx context.Context, spec ModelSpec) error {
	backend, ok := s.modelBackends[spec.Backend]
	if !ok {
		return fmt.Errorf("unknown model backend %q", spec.Backend)
	}

	wsHub := GetWSHub()
	wsHub.BroadcastAppState(spec.Name, StateUninstalling)
	s.activeModelMu.Lock()
	s.activeModelOps[spec.Name] = string(StateUninstalling)
	s.activeModelMu.Unlock()

	go func() {
		defer func() {
			s.activeModelMu.Lock()
			delete(s.activeModelOps, spec.Name)
			s.activeModelMu.Unlock()
		}()
		if err := backend.Uninstall(context.Background(), spec); err != nil {
			klog.Errorf("model uninstall %s (%s): %v", spec.Name, spec.Backend, err)
			wsHub.BroadcastAppState(spec.Name, StateUninstallFailed)
			return
		}
		wsHub.BroadcastAppState(spec.Name, StateUninstalled)
		klog.Infof("model %s (%s) uninstalled", spec.Name, spec.Backend)
	}()

	return nil
}

// ListInstalledModels returns all installed models across all backends,
// plus any models with active operations (downloading, uninstalling).
// The result is a map of backend name to list of installed models.
// Active operations appear under the "_active" key.
func (s *Service) ListInstalledModels(ctx context.Context) (map[string][]InstalledModel, error) {
	result := make(map[string][]InstalledModel)

	for name, backend := range s.modelBackends {
		models, err := backend.InstalledModels(ctx)
		if err != nil {
			klog.V(2).Infof("list models for %s: %v", name, err)
			continue // backend might not be reachable
		}
		if len(models) > 0 {
			result[name] = models
		}
	}

	// Include active operations so the frontend can show state after refresh
	s.activeModelMu.Lock()
	var active []InstalledModel
	for name, state := range s.activeModelOps {
		active = append(active, InstalledModel{
			Name:     name,
			Modified: state, // reuse Modified field for state
		})
	}
	s.activeModelMu.Unlock()
	if len(active) > 0 {
		result["_active"] = active
	}

	return result, nil
}
