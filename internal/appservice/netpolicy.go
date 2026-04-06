package appservice

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"k8s.io/klog/v2"
)

const iptablesChain = "PACKALARES-NET"

// hostIptables runs an iptables command on the host via nsenter.
func hostIptables(ctx context.Context, args ...string) ([]byte, error) {
	cmdArgs := append([]string{"--target", "1", "--net", "--mount", "--", "/usr/sbin/iptables"}, args...)
	return exec.CommandContext(ctx, "nsenter", cmdArgs...).CombinedOutput()
}

// EnsureIptablesChain creates the custom chain if it doesn't exist and hooks it into FORWARD.
func EnsureIptablesChain(ctx context.Context) {
	// Create chain (ignore error if exists)
	hostIptables(ctx, "-N", iptablesChain)
	// Hook into FORWARD if not already
	out, _ := hostIptables(ctx, "-C", "FORWARD", "-j", iptablesChain)
	if strings.Contains(string(out), "No chain") || strings.Contains(string(out), "does a matching rule exist") {
		hostIptables(ctx, "-I", "FORWARD", "1", "-j", iptablesChain)
	}
}

// BlockAppInternet adds iptables rules to block internet for all pods of an app.
func (k *K8sClient) BlockAppInternet(ctx context.Context, namespace, releaseName string) error {
	EnsureIptablesChain(ctx)

	podIPs, err := k.getPodIPs(ctx, namespace, releaseName)
	if err != nil {
		return err
	}

	for _, ip := range podIPs {
		// Allow cluster-internal traffic (pod CIDR, service CIDR, local network)
		hostIptables(ctx, "-A", iptablesChain, "-s", ip, "-d", "10.233.0.0/16", "-j", "ACCEPT")
		hostIptables(ctx, "-A", iptablesChain, "-s", ip, "-d", "192.168.0.0/16", "-j", "ACCEPT")
		// Block everything else
		hostIptables(ctx, "-A", iptablesChain, "-s", ip, "-j", "DROP")
		klog.Infof("blocked internet for pod IP %s (app %s)", ip, releaseName)
	}

	return nil
}

// UnblockAppInternet removes iptables rules for an app's pods.
func (k *K8sClient) UnblockAppInternet(ctx context.Context, namespace, releaseName string) error {
	podIPs, err := k.getPodIPs(ctx, namespace, releaseName)
	if err != nil {
		return err
	}

	for _, ip := range podIPs {
		// Remove all rules for this IP (run multiple times to catch all)
		for i := 0; i < 5; i++ {
			hostIptables(ctx, "-D", iptablesChain, "-s", ip, "-d", "10.233.0.0/16", "-j", "ACCEPT")
			hostIptables(ctx, "-D", iptablesChain, "-s", ip, "-d", "192.168.0.0/16", "-j", "ACCEPT")
			hostIptables(ctx, "-D", iptablesChain, "-s", ip, "-j", "DROP")
		}
		klog.Infof("unblocked internet for pod IP %s (app %s)", ip, releaseName)
	}

	return nil
}

// SyncInternetBlocks re-applies iptables rules for all blocked apps.
// Called on startup and when pods restart (new IPs).
func (s *Service) SyncInternetBlocks(ctx context.Context) {
	EnsureIptablesChain(ctx)
	// Flush and rebuild
	hostIptables(ctx, "-F", iptablesChain)

	for _, rec := range s.store.List(ctx) {
		if !rec.InternetBlocked || rec.State == StateUninstalled || rec.State == StateStopped {
			continue
		}
		if err := s.k8s.BlockAppInternet(ctx, rec.Namespace, rec.ReleaseName); err != nil {
			klog.V(2).Infof("sync internet block for %s: %v", rec.Name, err)
		}
	}
}

// getPodIPs returns all pod IPs for an app by label selector.
func (k *K8sClient) getPodIPs(ctx context.Context, namespace, releaseName string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "kubectl", "get", "pods",
		"--namespace", namespace,
		"-l", "app.kubernetes.io/instance="+releaseName,
		"-o", "jsonpath={range .items[*]}{.status.podIP}{\"\\n\"}{end}",
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("get pod IPs: %w", err)
	}

	var ips []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		ip := strings.TrimSpace(line)
		if ip != "" {
			ips = append(ips, ip)
		}
	}
	return ips, nil
}
