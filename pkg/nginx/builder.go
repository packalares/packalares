package nginx

import (
	"fmt"
	"strings"
)

// Params holds all values needed to generate the nginx config from template.
type Params struct {
	Zone         string
	ServerIP     string
	VPNIP  string
	CustomDomain string
	FrameworkNS  string
	UserNS       string
	Resolver     string
}

// Build replaces all placeholders in a template string with computed values.
func Build(tmpl string, p Params) string {
	domains := []string{p.Zone}
	if p.CustomDomain != "" {
		domains = append(domains, p.CustomDomain)
	}

	var ipNames []string
	if p.ServerIP != "" {
		ipNames = append(ipNames, p.ServerIP)
	}
	if p.VPNIP != "" {
		ipNames = append(ipNames, p.VPNIP)
	}
	if len(ipNames) == 0 {
		ipNames = append(ipNames, "_")
	}

	replacements := map[string]string{
		"{{ZONE}}":                  p.Zone,
		"{{SERVER_NAMES_IP}}":       strings.Join(ipNames, " "),
		"{{SERVER_NAMES_ZONE}}":     joinWithPrefix("", domains),
		"{{SERVER_NAMES_API}}":      joinWithPrefix("api.", domains),
		"{{SERVER_NAMES_DESKTOP}}":  joinWithPrefix("desktop.", domains),
		"{{SERVER_NAMES_AUTH}}":     joinWithPrefix("auth.", domains),
		"{{SERVER_NAMES_SYSAPPS}}":  buildSysAppNames(domains),
		"{{SERVER_NAMES_WILDCARD}}": joinWithPrefix("*.", domains),
		"{{CORS_ENTRIES}}":          buildCORSEntries(p),
		"{{REDIRECT_MAPS}}":        buildRedirectMaps(p),
		"{{AUTH_REDIRECT}}":         authRedirect(p),
		"{{DESKTOP_REDIRECT}}":      desktopRedirect(p),
		"{{FRAMEWORK_NAMESPACE}}":   p.FrameworkNS,
		"{{USER_NAMESPACE}}":        p.UserNS,
		"{{RESOLVER}}":              p.Resolver,
	}

	result := tmpl
	for k, v := range replacements {
		result = strings.ReplaceAll(result, k, v)
	}
	return result
}

func joinWithPrefix(prefix string, domains []string) string {
	parts := make([]string, len(domains))
	for i, d := range domains {
		parts[i] = prefix + d
	}
	return strings.Join(parts, " ")
}

func buildSysAppNames(domains []string) string {
	var parts []string
	for _, d := range domains {
		parts = append(parts, "market."+d, "settings."+d, "files."+d, "dashboard."+d)
	}
	return strings.Join(parts, " ")
}

func buildCORSEntries(p Params) string {
	var lines []string
	lines = append(lines, fmt.Sprintf(`"https://%s" $http_origin;`, p.Zone))
	lines = append(lines, fmt.Sprintf(`~^https://[a-zA-Z0-9\-]+\.%s$ $http_origin;`, escDots(p.Zone)))
	if p.ServerIP != "" {
		lines = append(lines, fmt.Sprintf(`"https://%s" $http_origin;`, p.ServerIP))
	}
	if p.VPNIP != "" {
		lines = append(lines, fmt.Sprintf(`"https://%s" $http_origin;`, p.VPNIP))
	}
	if p.CustomDomain != "" {
		lines = append(lines, fmt.Sprintf(`"https://%s" $http_origin;`, p.CustomDomain))
		lines = append(lines, fmt.Sprintf(`~^https://[a-zA-Z0-9\-]+\.%s$ $http_origin;`, escDots(p.CustomDomain)))
	}
	return "    " + strings.Join(lines, "\n    ")
}

func buildRedirectMaps(p Params) string {
	if p.CustomDomain == "" {
		return ""
	}
	escaped := escDots(p.CustomDomain)
	return fmt.Sprintf(`  map $host $auth_domain {
    default                      auth.%s;
    ~\.%s$                       auth.%s;
    %s                           auth.%s;
  }
  map $host $desktop_domain {
    default                      desktop.%s;
    ~\.%s$                       desktop.%s;
    %s                           desktop.%s;
  }`,
		p.Zone, escaped, p.CustomDomain, p.CustomDomain, p.CustomDomain,
		p.Zone, escaped, p.CustomDomain, p.CustomDomain, p.CustomDomain)
}

func authRedirect(p Params) string {
	if p.CustomDomain != "" {
		return "$auth_domain"
	}
	return "auth." + p.Zone
}

func desktopRedirect(p Params) string {
	if p.CustomDomain != "" {
		return "$desktop_domain"
	}
	return "desktop." + p.Zone
}

func escDots(s string) string {
	return strings.ReplaceAll(s, ".", "\\.")
}
