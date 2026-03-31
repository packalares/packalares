package cni

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"
)

const calicoVersion = "v3.27.2"

func DeployCalico(registry string, w io.Writer) error {
	fmt.Fprintln(w, "  Deploying Calico CNI ...")

	// Apply Calico operator
	operatorURL := fmt.Sprintf("https://raw.githubusercontent.com/projectcalico/calico/%s/manifests/tigera-operator.yaml", calicoVersion)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "kubectl", "apply", "--server-side", "--force-conflicts", "-f", operatorURL)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("apply calico operator: %s\n%w", string(out), err)
	}

	// Wait for CRDs to be registered by the operator
	fmt.Fprintln(w, "  Waiting for Calico CRDs ...")
	for i := 0; i < 60; i++ {
		crdCheck := exec.CommandContext(ctx, "kubectl", "get", "crd", "installations.operator.tigera.io", "--no-headers")
		if out, err := crdCheck.CombinedOutput(); err == nil && len(out) > 0 {
			break
		}
		if i == 59 {
			return fmt.Errorf("calico CRDs not registered after 5 minutes")
		}
		time.Sleep(5 * time.Second)
	}

	// Apply custom resource with our CIDR
	calicoCustomResource := `apiVersion: operator.tigera.io/v1
kind: Installation
metadata:
  name: default
spec:
  calicoNetwork:
    ipPools:
    - blockSize: 24
      cidr: 10.233.64.0/18
      encapsulation: VXLANCrossSubnet
      natOutgoing: Enabled
      nodeSelector: all()
---
apiVersion: operator.tigera.io/v1
kind: APIServer
metadata:
  name: default
spec: {}
`

	cmd2 := exec.CommandContext(ctx, "kubectl", "apply", "-f", "-")
	cmd2.Stdin = nil
	cmd2Stdin, err := cmd2.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	go func() {
		defer cmd2Stdin.Close()
		cmd2Stdin.Write([]byte(calicoCustomResource))
	}()

	if out, err := cmd2.CombinedOutput(); err != nil {
		return fmt.Errorf("apply calico CR: %s\n%w", string(out), err)
	}

	// Wait for calico to be ready
	fmt.Fprintln(w, "  Waiting for Calico to be ready ...")
	for i := 0; i < 60; i++ {
		cmd := exec.Command("kubectl", "get", "pods", "-n", "calico-system", "--no-headers")
		out, err := cmd.CombinedOutput()
		if err == nil && len(out) > 0 {
			// Check if all pods are running
			lines := string(out)
			if !containsNotReady(lines) {
				fmt.Fprintln(w, "  Calico CNI deployed")
				return nil
			}
		}
		time.Sleep(5 * time.Second)
	}

	// Don't fail — calico might still be coming up
	fmt.Fprintln(w, "  Warning: Calico not fully ready yet, continuing ...")
	return nil
}

func containsNotReady(output string) bool {
	// Simple check: if there are pods and none have 0/ in ready column
	for _, line := range splitLines(output) {
		if line == "" {
			continue
		}
		if contains(line, "0/") || contains(line, "Pending") || contains(line, "Init") || contains(line, "ContainerCreating") {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && searchString(s, sub)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
