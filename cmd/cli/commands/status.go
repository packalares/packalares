package commands

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"

	"github.com/packalares/packalares/pkg/config"
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show Packalares system status",
		Run: func(cmd *cobra.Command, args []string) {
			if err := showStatus(); err != nil {
				log.Fatalf("status check failed: %v", err)
			}
		},
	}

	return cmd
}

func showStatus() error {
	fmt.Println("=== Packalares System Status ===")
	fmt.Println()

	// Check systemd services
	services := []string{"k3s", "etcd"}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "SERVICE\tSTATUS")
	fmt.Fprintln(w, "-------\t------")
	for _, svc := range services {
		status := getServiceStatus(svc)
		fmt.Fprintf(w, "%s\t%s\n", svc, status)
	}
	w.Flush()
	fmt.Println()

	// Check Kubernetes pods if kubectl available
	kubectlPath := findKubectl()
	if kubectlPath == "" {
		fmt.Println("kubectl not found — cannot check pod status")
		return nil
	}

	fmt.Println("=== Kubernetes Pods ===")
	fmt.Println()

	ctx := context.Background()
	namespaces := []string{
		config.PlatformNamespace(),
		config.FrameworkNamespace(),
		config.MonitoringNamespace(),
		config.UserNamespace(config.Username()),
		"kube-system",
	}

	for _, ns := range namespaces {
		out, err := exec.CommandContext(ctx, kubectlPath,
			"get", "pods", "-n", ns,
			"--no-headers",
			"-o", "custom-columns=NAME:.metadata.name,STATUS:.status.phase,READY:.status.conditions[?(@.type=='Ready')].status",
		).CombinedOutput()
		if err != nil {
			continue
		}
		lines := strings.TrimSpace(string(out))
		if lines == "" {
			continue
		}
		fmt.Printf("--- %s ---\n", ns)
		fmt.Println(lines)
		fmt.Println()
	}

	// Show release info
	if data, err := os.ReadFile("/etc/packalares/release"); err == nil {
		fmt.Println("=== Release Info ===")
		fmt.Println(string(data))
	}

	return nil
}

func getServiceStatus(name string) string {
	out, err := exec.Command("systemctl", "is-active", name).CombinedOutput()
	if err != nil {
		return "not installed"
	}
	return strings.TrimSpace(string(out))
}

func findKubectl() string {
	paths := []string{
		"/usr/local/bin/kubectl",
		"/usr/bin/kubectl",
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if p, err := exec.LookPath("kubectl"); err == nil {
		return p
	}
	return ""
}
