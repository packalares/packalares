// Package config provides centralized configuration for all packalares services.
// Values come from /etc/packalares/config.yaml, environment variables, or defaults.
// No hardcoded namespaces, domains, or service names anywhere else in the codebase.
package config

import (
	"os"
	"strings"
)

// Namespaces
func PlatformNamespace() string { return envOr("PLATFORM_NAMESPACE", "os-system") }
func FrameworkNamespace() string { return envOr("FRAMEWORK_NAMESPACE", "os-framework") }
func UserNamespace(username string) string { return "user-space-" + username }
func MonitoringNamespace() string { return envOr("MONITORING_NAMESPACE", "monitoring") }

// Domain
func Domain() string { return envOr("SYSTEM_DOMAIN", "olares.local") }
func Username() string { return envOr("USERNAME", "laurs") }
func UserZone() string {
	if v := os.Getenv("USER_ZONE"); v != "" {
		return v
	}
	return Username() + "." + Domain()
}

// Service DNS names (all derived from namespaces)
func ServiceDNS(name, namespace string) string {
	return name + "." + namespace + ".svc.cluster.local"
}

// Platform services
func CitusDNS() string { return ServiceDNS("citus-svc", PlatformNamespace()) }
func CitusHost() string { return envOr("PG_HOST", ServiceDNS("citus-svc", PlatformNamespace())) }
func CitusPort() string { return envOr("PG_PORT", "5432") }
func CitusPassword() string { return envOr("PG_PASSWORD", "") }
func CitusUser() string { return envOr("PG_USER", "packalares") }

func KVRocksDNS() string { return ServiceDNS("kvrocks-svc", PlatformNamespace()) }
func KVRocksHost() string { return envOr("KVROCKS_HOST", ServiceDNS("kvrocks-svc", PlatformNamespace())) }
func KVRocksPort() string { return envOr("KVROCKS_PORT", "6379") }

func NATSDns() string { return ServiceDNS("nats-svc", PlatformNamespace()) }
func NATSHost() string { return envOr("NATS_HOST", ServiceDNS("nats-svc", PlatformNamespace())) }

func LLDAPHost() string { return envOr("LLDAP_HOST", ServiceDNS("lldap-svc", PlatformNamespace())) }
func LLDAPPort() string { return envOr("LLDAP_PORT", "17170") }

func InfisicalDNS() string { return ServiceDNS("infisical-svc", FrameworkNamespace()) }

// Chart repo
func ChartRepoDNS() string { return ServiceDNS("chart-repo-service", FrameworkNamespace()) }
func ChartRepoURL() string { return envOr("CHART_REPO_URL", "http://"+ChartRepoDNS()+":82/") }

// Framework services
func AuthDNS() string { return ServiceDNS("auth-svc", FrameworkNamespace()) }
func BFLDNS() string { return ServiceDNS("bfl-svc", FrameworkNamespace()) }
func MonitorDNS() string { return ServiceDNS("monitor-svc", FrameworkNamespace()) }
func MarketDNS() string { return ServiceDNS("market-svc", FrameworkNamespace()) }
func AppserviceDNS() string { return ServiceDNS("appservice-svc", FrameworkNamespace()) }
func DesktopDNS() string { return ServiceDNS("desktop-svc", FrameworkNamespace()) }

// Monitoring
func PrometheusDNS() string { return ServiceDNS("prometheus-svc", MonitoringNamespace()) }
func PrometheusURL() string { return envOr("PROMETHEUS_URL", "http://" + PrometheusDNS() + ":9090") }

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// APISubdomain returns the API gateway subdomain
func APISubdomain() string { return "api." + UserZone() }

// AppSubdomain returns the subdomain for an installed app
func AppSubdomain(appName string) string { return appName + "." + UserZone() }

// SystemSubdomains returns all system app subdomains
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

// SplitZone extracts username and domain from a zone like "laurs.olares.local"
func SplitZone(zone string) (username, domain string) {
	parts := strings.SplitN(zone, ".", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return zone, ""
}
