package config

import (
	"os"
	"strconv"
)

var (
	BFLNodePort      = envInt("PACKALARES_BFL_PORT", 30883)
	IngressHTTPPort  = envInt("PACKALARES_INGRESS_HTTP_PORT", 30083)
	IngressHTTPSPort = envInt("PACKALARES_INGRESS_HTTPS_PORT", 30082)
	WizardPort       = envInt("PACKALARES_WIZARD_PORT", 30180)
)

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}
