// Package config provides centralized configuration for all packalares services.
// Priority: /etc/packalares/config.yaml → environment variables → defaults.
// No hardcoded namespaces, domains, or service names anywhere else in the codebase.
package config

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// configData holds the parsed config.yaml contents.
var (
	cfg     map[string]interface{}
	cfgOnce sync.Once
)

// loadConfig reads /etc/packalares/config.yaml once.
func loadConfig() {
	cfgOnce.Do(func() {
		paths := []string{
			"/etc/packalares/config.yaml",
			os.Getenv("PACKALARES_CONFIG"),
		}
		for _, p := range paths {
			if p == "" {
				continue
			}
			data, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			var parsed map[string]interface{}
			if err := yaml.Unmarshal(data, &parsed); err != nil {
				continue
			}
			cfg = parsed
			return
		}
		cfg = make(map[string]interface{})
	})
}

// configGet reads a dot-separated key from config.yaml.
// Example: configGet("system.domain") reads cfg["system"]["domain"]
func configGet(key string) string {
	loadConfig()
	parts := strings.Split(key, ".")
	var current interface{} = cfg

	for _, part := range parts {
		m, ok := current.(map[string]interface{})
		if !ok {
			return ""
		}
		current, ok = m[part]
		if !ok {
			return ""
		}
	}

	return fmt.Sprintf("%v", current)
}

// configOr checks: config.yaml key → env var → default.
func configOr(yamlKey, envKey, fallback string) string {
	if v := configGet(yamlKey); v != "" {
		return v
	}
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	return fallback
}

// ─── Namespaces ─────────────────────────────────────────

func PlatformNamespace() string {
	return configOr("system.namespaces.platform", "PLATFORM_NAMESPACE", "os-system")
}
func FrameworkNamespace() string {
	return configOr("system.namespaces.framework", "FRAMEWORK_NAMESPACE", "os-framework")
}
func UserNamespace(username string) string { return "user-space-" + username }
func MonitoringNamespace() string {
	return configOr("system.namespaces.monitoring", "MONITORING_NAMESPACE", "monitoring")
}

// ─── API Group ──────────────────────────────────────────

func APIGroup() string { return configOr("system.api_group", "API_GROUP", "packalares.io") }

// ─── TLS ────────────────────────────────────────────────

func TLSSecretName() string { return configOr("network.tls_secret_name", "TLS_SECRET_NAME", "zone-tls") }

// ─── Domain ─────────────────────────────────────────────

func Domain() string  { return configOr("system.domain", "SYSTEM_DOMAIN", "olares.local") }
func Username() string {
	if v := configOr("user.name", "USERNAME", ""); v != "" {
		return v
	}
	return os.Getenv("OWNER")
}
func Hostname() string { return configOr("system.hostname", "HOSTNAME", "packalares") }
func Timezone() string { return configOr("system.timezone", "TZ", "UTC") }

func UserZone() string {
	if v := configGet("user.zone"); v != "" {
		return v
	}
	if v := os.Getenv("USER_ZONE"); v != "" {
		return v
	}
	return Username() + "." + Domain()
}

// ─── Service DNS ────────────────────────────────────────

func ServiceDNS(name, namespace string) string {
	return name + "." + namespace + ".svc.cluster.local"
}

// ─── Platform services ─────────────────────────────────

func CitusDNS() string  { return ServiceDNS("citus-coordinator-svc", PlatformNamespace()) }
func CitusHost() string  { return configOr("database.host", "PG_HOST", CitusDNS()) }
func CitusPort() string  { return configOr("database.port", "PG_PORT", "5432") }
func CitusPassword() string { return configOr("database.password", "PG_PASSWORD", "") }
func CitusUser() string  { return configOr("database.user", "PG_USER", "packalares") }

func KVRocksDNS() string  { return ServiceDNS("kvrocks-svc", PlatformNamespace()) }
func KVRocksHost() string { return configOr("redis.host", "KVROCKS_HOST", KVRocksDNS()) }
func KVRocksPort() string { return configOr("redis.port", "KVROCKS_PORT", "6379") }

func NATSDns() string  { return ServiceDNS("nats-svc", PlatformNamespace()) }
func NATSHost() string { return configOr("nats.host", "NATS_HOST", NATSDns()) }

func LLDAPHost() string { return configOr("lldap.host", "LLDAP_HOST", ServiceDNS("lldap-svc", PlatformNamespace())) }
func LLDAPPort() string { return configOr("lldap.port", "LLDAP_PORT", "17170") }

func InfisicalDNS() string { return ServiceDNS("infisical-svc", PlatformNamespace()) }

// ─── Chart repo ─────────────────────────────────────────

func ChartRepoDNS() string { return ServiceDNS("chart-repo-service", FrameworkNamespace()) }
func ChartRepoURL() string {
	return configOr("catalog.chart_repo_url", "CHART_REPO_URL", "http://"+ChartRepoDNS()+":82/")
}

// ─── Framework services ────────────────────────────────

func AuthDNS() string       { return ServiceDNS("auth-backend", FrameworkNamespace()) }
func BFLDNS() string        { return ServiceDNS("bfl", FrameworkNamespace()) }
func MonitorDNS() string    { return ServiceDNS("monitoring-server", FrameworkNamespace()) }
func MarketDNS() string     { return ServiceDNS("market-backend", FrameworkNamespace()) }
func AppserviceDNS() string { return ServiceDNS("app-service", FrameworkNamespace()) }
func SystemServerDNS() string { return ServiceDNS("system-server", FrameworkNamespace()) }
func DesktopDNS() string    { return ServiceDNS("desktop-svc", UserNamespace(Username())) }

// ─── Monitoring ─────────────────────────────────────────

func PrometheusDNS() string { return ServiceDNS("prometheus-svc", MonitoringNamespace()) }
func PrometheusURL() string {
	return configOr("monitoring.prometheus_url", "PROMETHEUS_URL", "http://"+PrometheusDNS()+":9090")
}

// ─── Network ────────────────────────────────────────────

func TailscaleEnabled() bool { return configGet("network.tailscale.enabled") == "true" }
func TailscaleAuthKey() string {
	return configOr("network.tailscale.auth_key", "TS_AUTH_KEY", "")
}
func DNSExposePort53() bool { return configGet("network.dns.expose_port_53") == "true" }

// ─── Storage ────────────────────────────────────────────

func DataPath() string { return configOr("storage.data_path", "DATA_PATH", "/packalares/data") }

// ─── GPU ────────────────────────────────────────────────

func GPUEnabled() string { return configOr("gpu.enabled", "GPU_ENABLED", "auto") }
func GPUSharing() bool   { return configGet("gpu.sharing") != "false" }

// ─── Subdomains ─────────────────────────────────────────

func APISubdomain() string              { return "api." + UserZone() }
func AppSubdomain(appName string) string { return appName + "." + UserZone() }

func SystemSubdomains() map[string]string {
	zone := UserZone()
	return map[string]string{
		"desktop":   "desktop." + zone,
		"market":    "market." + zone,
		"settings":  "settings." + zone,
		"files":     "files." + zone,
		"dashboard": "dashboard." + zone,
		"auth":      "auth." + zone,
		"api":       "api." + zone,
	}
}

// ─── Helpers ────────────────────────────────────────────

func SplitZone(zone string) (username, domain string) {
	parts := strings.SplitN(zone, ".", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return zone, ""
}
