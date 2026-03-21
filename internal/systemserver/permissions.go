package systemserver

import (
	"fmt"
	"log"
	"sync"
)

// PermissionManager tracks which apps have which permissions.
type PermissionManager struct {
	mu          sync.RWMutex
	permissions map[string]*AppPermissionSet // keyed by app name
}

type AppPermissionSet struct {
	AppName string
	AppID   string
	Key     string
	Secret  string
	Perms   []AppPermission
}

func NewPermissionManager() *PermissionManager {
	return &PermissionManager{
		permissions: make(map[string]*AppPermissionSet),
	}
}

// Register records an app's permissions.
func (pm *PermissionManager) Register(app string, perms []AppPermission) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.permissions[app] = &AppPermissionSet{
		AppName: app,
		Perms:   perms,
	}
	log.Printf("registered permissions for app %q: %d rules", app, len(perms))
}

// Unregister removes an app's permissions.
func (pm *PermissionManager) Unregister(app string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	delete(pm.permissions, app)
	log.Printf("unregistered permissions for app %q", app)
}

// CheckPermission validates that an app has a specific permission.
func (pm *PermissionManager) CheckPermission(app, group, dataType, version, op string) error {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	permSet, ok := pm.permissions[app]
	if !ok {
		return fmt.Errorf("app %q has no registered permissions", app)
	}

	for _, p := range permSet.Perms {
		if p.Group == group && p.DataType == dataType && p.Version == version {
			for _, allowedOp := range p.Ops {
				if allowedOp == op {
					return nil
				}
			}
		}
	}

	return fmt.Errorf("app %q does not have permission for %s/%s/%s/%s", app, group, dataType, version, op)
}

// ListApps returns all registered app names.
func (pm *PermissionManager) ListApps() []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var apps []string
	for name := range pm.permissions {
		apps = append(apps, name)
	}
	return apps
}

// GetPermissions returns permissions for a specific app.
func (pm *PermissionManager) GetPermissions(app string) ([]AppPermission, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	permSet, ok := pm.permissions[app]
	if !ok {
		return nil, false
	}
	return permSet.Perms, true
}
