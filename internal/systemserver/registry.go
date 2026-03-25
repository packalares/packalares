package systemserver

import (
	"fmt"
	"log"
	"sync"
)

// ProviderRegistry tracks provider apps and their shared entrances,
// indexed by group name (e.g. "api.ollama") for fast consumer lookups.
type ProviderRegistry struct {
	mu        sync.RWMutex
	providers map[string][]ProviderEndpoint // group -> endpoints
}

// ProviderEndpoint describes a single provider's network endpoint.
type ProviderEndpoint struct {
	AppName   string `json:"appName"`   // e.g. "vllmqwen330ba3binstruct4bitv2"
	Host      string `json:"host"`      // K8s service hostname
	Port      int    `json:"port"`
	Title     string `json:"title"`     // friendly name e.g. "Qwen3 30B"
	Namespace string `json:"namespace"`
}

// NewProviderRegistry creates an empty provider registry.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string][]ProviderEndpoint),
	}
}

// RegisterProvider extracts sharedEntrances from an Application and registers
// each one as a provider endpoint. The group key is derived from the entrance
// name (e.g. "api.ollama"). The K8s service hostname is built as:
//
//	{svc}.{namespace}.svc.cluster.local:{port}
//
// where svc is the SharedEntrance.Host field.
func (r *ProviderRegistry) RegisterProvider(app *Application) {
	if len(app.Spec.SharedEntrances) == 0 {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	ns := app.Spec.Namespace
	if ns == "" {
		ns = app.Namespace
	}

	registered := 0
	for _, se := range app.Spec.SharedEntrances {
		group := se.Name // e.g. "api.ollama"
		if group == "" {
			continue
		}

		host := se.Host
		if host == "" {
			host = app.Spec.Name
		}

		// Build fully qualified K8s service hostname
		svcHost := fmt.Sprintf("%s.%s.svc.cluster.local", host, ns)

		ep := ProviderEndpoint{
			AppName:   app.Spec.Name,
			Host:      svcHost,
			Port:      se.Port,
			Title:     se.Title,
			Namespace: ns,
		}

		// Avoid duplicate registrations for the same app+group
		existing := r.providers[group]
		found := false
		for i, e := range existing {
			if e.AppName == app.Spec.Name {
				existing[i] = ep
				found = true
				break
			}
		}
		if !found {
			r.providers[group] = append(r.providers[group], ep)
		}
		registered++
	}

	if registered > 0 {
		log.Printf("provider-registry: registered %d entrances for app %q", registered, app.Spec.Name)
	}
}

// UnregisterProvider removes all provider entries for the given app name.
func (r *ProviderRegistry) UnregisterProvider(appName string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	removed := 0
	for group, eps := range r.providers {
		filtered := eps[:0]
		for _, ep := range eps {
			if ep.AppName != appName {
				filtered = append(filtered, ep)
			} else {
				removed++
			}
		}
		if len(filtered) == 0 {
			delete(r.providers, group)
		} else {
			r.providers[group] = filtered
		}
	}

	if removed > 0 {
		log.Printf("provider-registry: unregistered %d entries for app %q", removed, appName)
	}
}

// GetProviders returns all provider endpoints for a given group (e.g. "api.ollama").
func (r *ProviderRegistry) GetProviders(group string) []ProviderEndpoint {
	r.mu.RLock()
	defer r.mu.RUnlock()

	eps := r.providers[group]
	if eps == nil {
		return nil
	}

	// Return a copy to avoid races
	result := make([]ProviderEndpoint, len(eps))
	copy(result, eps)
	return result
}

// GetAllProviders returns a snapshot of all registered providers grouped by group name.
func (r *ProviderRegistry) GetAllProviders() map[string][]ProviderEndpoint {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string][]ProviderEndpoint, len(r.providers))
	for group, eps := range r.providers {
		copied := make([]ProviderEndpoint, len(eps))
		copy(copied, eps)
		result[group] = copied
	}
	return result
}

// FindConsumers returns all applications that consume the given provider group
// via their permission.sysData entries.
func (r *ProviderRegistry) FindConsumers(apps map[string]*Application, providerGroup string) []*Application {
	var consumers []*Application
	for _, app := range apps {
		if app.Spec.Permission == nil {
			continue
		}
		for _, sd := range app.Spec.Permission.SysData {
			if sd.Group == providerGroup {
				consumers = append(consumers, app)
				break
			}
		}
	}
	return consumers
}
