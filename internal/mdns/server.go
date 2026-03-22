package mdns

import (
	"fmt"
	"log"
	"net"
	"os"
	"strings"

	"github.com/hashicorp/mdns"
)

// Server announces the packalares instance on the local network via mDNS.
type Server struct {
	server *mdns.Server
}

// NewServer creates and starts an mDNS server that announces:
//   - _packalares._tcp — main web UI
//   - _smb._tcp — Samba file sharing (if enabled)
func NewServer() (*Server, error) {
	hostname, _ := os.Hostname()
	serviceName := os.Getenv("MDNS_SERVICE_NAME")
	if serviceName == "" {
		serviceName = "Packalares"
	}

	port := 443
	if p := os.Getenv("MDNS_PORT"); p != "" {
		fmt.Sscanf(p, "%d", &port)
	}

	ip := getHostIP()
	if ip == nil {
		return nil, fmt.Errorf("could not determine host IP")
	}

	// Create mDNS service entry
	info := []string{
		"packalares=true",
		"version=1.0.0",
	}

	service, err := mdns.NewMDNSService(
		serviceName,         // instance name
		"_packalares._tcp",  // service type
		"",                  // domain (default .local)
		hostname+".",        // host name
		port,                // port
		[]net.IP{ip},        // IPs
		info,                // TXT records
	)
	if err != nil {
		return nil, fmt.Errorf("create mdns service: %w", err)
	}

	server, err := mdns.NewServer(&mdns.Config{
		Zone: service,
	})
	if err != nil {
		return nil, fmt.Errorf("start mdns server: %w", err)
	}

	log.Printf("mDNS: announcing %s on %s port %d", serviceName, ip.String(), port)
	return &Server{server: server}, nil
}

// Close stops the mDNS server.
func (s *Server) Close() {
	if s.server != nil {
		s.server.Shutdown()
	}
}

// getHostIP returns the first non-loopback IPv4 address.
func getHostIP() net.IP {
	// Try environment variable first
	if ip := os.Getenv("SERVER_IP"); ip != "" {
		parsed := net.ParseIP(ip)
		if parsed != nil {
			return parsed
		}
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		// Skip virtual/container interfaces
		if strings.HasPrefix(iface.Name, "veth") ||
			strings.HasPrefix(iface.Name, "cali") ||
			strings.HasPrefix(iface.Name, "docker") ||
			strings.HasPrefix(iface.Name, "flannel") ||
			strings.HasPrefix(iface.Name, "cni") {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
				return ipnet.IP
			}
		}
	}
	return nil
}
