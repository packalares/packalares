package appservice

import (
	"context"
	"crypto/md5"
	"fmt"
	"os"
	"strings"

	"k8s.io/klog/v2"
)

// GeneratedAppManager creates "generated apps" from the GENERATED_APPS env var.
// These are per-user system apps like L4 proxy, kvrocks, etc. that get auto-installed.
type GeneratedAppManager struct {
	helm  *HelmClient
	store *AppStore
	owner string
}

// NewGeneratedAppManager creates a manager for auto-generated apps.
func NewGeneratedAppManager(helm *HelmClient, store *AppStore, owner string) *GeneratedAppManager {
	return &GeneratedAppManager{
		helm:  helm,
		store: store,
		owner: owner,
	}
}

// GetGeneratedApps returns the list of generated app names from GENERATED_APPS env.
func GetGeneratedApps() []string {
	env := os.Getenv("GENERATED_APPS")
	if env == "" {
		return nil
	}
	apps := strings.Split(env, ",")
	var result []string
	for _, a := range apps {
		a = strings.TrimSpace(a)
		if a != "" {
			result = append(result, a)
		}
	}
	return result
}

// GetSysApps returns the list of system app names from SYS_APPS env.
func GetSysApps() []string {
	env := os.Getenv("SYS_APPS")
	if env == "" {
		return nil
	}
	apps := strings.Split(env, ",")
	var result []string
	for _, a := range apps {
		a = strings.TrimSpace(a)
		if a != "" {
			result = append(result, a)
		}
	}
	return result
}

// IsSysApp checks if an app is a system app.
func IsSysApp(name string) bool {
	for _, a := range GetSysApps() {
		if a == name {
			return true
		}
	}
	return false
}

// IsGeneratedApp checks if an app name matches a generated app prefix.
func IsGeneratedApp(name string) bool {
	for _, a := range GetGeneratedApps() {
		if strings.HasPrefix(name, a) {
			return true
		}
	}
	return false
}

// EnsureGeneratedApps installs any generated apps that are not yet installed.
func (m *GeneratedAppManager) EnsureGeneratedApps(ctx context.Context) error {
	genApps := GetGeneratedApps()
	if len(genApps) == 0 {
		klog.Info("no generated apps configured")
		return nil
	}

	chartRepoURL := os.Getenv("CHART_REPO_URL")
	if chartRepoURL == "" {
		chartRepoURL = "http://chart-repo-service.os-framework:82/"
	}

	for _, appName := range genApps {
		releaseName := fmt.Sprintf("%s-%s", appName, m.owner)
		_, exists := m.store.Get(ctx, appName)
		if exists {
			continue
		}

		klog.Infof("installing generated app %s for user %s", appName, m.owner)

		appID := appName
		if !IsSysApp(appName) {
			appID = Md5Short(appName)
		}

		rec := &AppRecord{
			Name:        appName,
			AppID:       appID,
			Namespace:   m.helm.Namespace,
			Owner:       m.owner,
			Source:      string(SourceSystem),
			State:       StateInstalling,
			OpType:      OpInstall,
			ReleaseName: releaseName,
			ChartRef:    appName,
			RepoURL:     chartRepoURL,
			IsSysApp:    true,
		}

		if err := m.store.Put(ctx, rec); err != nil {
			klog.Errorf("store generated app %s: %v", appName, err)
			continue
		}

		chartRef := appName
		err := m.helm.Install(ctx, releaseName, chartRef, nil, "")
		if err != nil {
			klog.Errorf("install generated app %s: %v", appName, err)
			rec.State = StateInstallFailed
			_ = m.store.Put(ctx, rec)
			continue
		}

		rec.State = StateRunning
		_ = m.store.Put(ctx, rec)
		klog.Infof("generated app %s installed successfully", appName)
	}

	return nil
}

// Md5Short returns the first 8 chars of the MD5 hex digest of s.
func Md5Short(s string) string {
	h := md5.Sum([]byte(s))
	return fmt.Sprintf("%x", h)[:8]
}
