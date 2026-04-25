package appservice

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"

	"k8s.io/klog/v2"
)

// cmInterface abstracts the few ConfigMap operations PublicDomainSync needs,
// so tests can substitute an in-memory fake. The default implementation uses
// kubectl shell-out (matching the pattern in netpolicy.go).
type cmInterface interface {
	get(ctx context.Context, namespace, name string) (string, bool, error)
	apply(ctx context.Context, namespace, name, body string) error
}

// kubectlCM is the default cmInterface backed by `kubectl`.
type kubectlCM struct{}

func (kubectlCM) get(ctx context.Context, namespace, name string) (string, bool, error) {
	cmd := exec.CommandContext(ctx, "kubectl", "get", "configmap", name, "-n", namespace, "-o", "json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		s := string(out)
		if strings.Contains(s, "NotFound") || strings.Contains(s, "not found") {
			return "", false, nil
		}
		return "", false, fmt.Errorf("kubectl get configmap %s/%s: %s (%w)", namespace, name, strings.TrimSpace(s), err)
	}
	var parsed struct {
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return "", true, fmt.Errorf("decode configmap json: %w", err)
	}
	return parsed.Data["domains.txt"], true, nil
}

func (kubectlCM) apply(ctx context.Context, namespace, name, body string) error {
	manifest := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
			"labels": map[string]string{
				"app.kubernetes.io/managed-by": "packalares-appservice",
			},
		},
		"data": map[string]string{
			"domains.txt": body,
		},
	}
	buf, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("encode configmap manifest: %w", err)
	}
	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(string(buf))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl apply configmap %s/%s: %s (%w)", namespace, name, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// PublicDomainSync owns the per-app set of public domains and writes them
// to a shared ConfigMap consumed by the auth-backend.
type PublicDomainSync struct {
	cm        cmInterface
	namespace string
	name      string

	mu       sync.Mutex
	byApp    map[string][]string
	lastBody string
}

// NewPublicDomainSync creates a sync writing to the given ConfigMap.
func NewPublicDomainSync(namespace, name string) *PublicDomainSync {
	return &PublicDomainSync{
		cm:        kubectlCM{},
		namespace: namespace,
		name:      name,
		byApp:     make(map[string][]string),
	}
}

// Sync sets/clears the host list for one app and pushes the merged list to the
// ConfigMap. When enabled is false, the app's entries are removed.
func (s *PublicDomainSync) Sync(ctx context.Context, appName string, enabled bool, hosts []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if enabled {
		s.byApp[appName] = uniqSorted(hosts)
	} else {
		delete(s.byApp, appName)
	}
	return s.flush(ctx)
}

// Reconcile rebuilds state from the given list of records and overwrites the
// ConfigMap. Called on service startup.
func (s *PublicDomainSync) Reconcile(ctx context.Context, recs []*AppRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byApp = make(map[string][]string, len(recs))
	for _, rec := range recs {
		if rec == nil || !rec.PublicAccess {
			continue
		}
		if rec.State == StateUninstalled {
			continue
		}
		hosts := hostsForApp(rec)
		if len(hosts) == 0 {
			continue
		}
		s.byApp[rec.Name] = uniqSorted(hosts)
	}
	return s.flush(ctx)
}

// flush rebuilds the merged sorted list and writes it to the ConfigMap; idempotent.
func (s *PublicDomainSync) flush(ctx context.Context) error {
	all := make(map[string]struct{})
	for _, hs := range s.byApp {
		for _, h := range hs {
			if h == "" {
				continue
			}
			all[h] = struct{}{}
		}
	}
	out := make([]string, 0, len(all))
	for h := range all {
		out = append(out, h)
	}
	sort.Strings(out)
	body := strings.Join(out, "\n")
	if body == s.lastBody {
		return nil
	}
	if err := s.cm.apply(ctx, s.namespace, s.name, body); err != nil {
		return err
	}
	s.lastBody = body
	klog.V(2).Infof("public-domains: wrote %d host(s) to configmap %s/%s", len(out), s.namespace, s.name)
	return nil
}

// uniqSorted returns hosts sorted and deduped.
func uniqSorted(hosts []string) []string {
	if len(hosts) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(hosts))
	out := make([]string, 0, len(hosts))
	for _, h := range hosts {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		if _, ok := seen[h]; ok {
			continue
		}
		seen[h] = struct{}{}
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}

// hostsForApp returns every reachable hostname for an app's entrances. Each
// entrance produces one host on the user zone, plus one on CUSTOM_DOMAIN if set.
func hostsForApp(rec *AppRecord) []string {
	if rec == nil {
		return nil
	}
	zone := os.Getenv("USER_ZONE")
	custom := os.Getenv("CUSTOM_DOMAIN")
	if len(rec.Entrances) == 0 {
		// Fallback to the app name as the entrance name.
		return collectHosts(rec.Name, zone, custom)
	}
	var out []string
	for _, e := range rec.Entrances {
		name := e.Name
		if name == "" {
			name = rec.Name
		}
		out = append(out, collectHosts(name, zone, custom)...)
	}
	return out
}

func collectHosts(entranceName, zone, custom string) []string {
	var hosts []string
	if zone != "" {
		hosts = append(hosts, entranceName+"."+zone)
	}
	if custom != "" {
		hosts = append(hosts, entranceName+"."+custom)
	}
	return hosts
}
