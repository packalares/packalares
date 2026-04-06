package appservice

import (
	"context"
	"os/exec"
	"strings"

	"k8s.io/klog/v2"
)

const ctrSock = "/run/containerd/containerd.sock"

// purgeContainerImages removes container images and their content store data.
// For each image:
//  1. crictl rmi — removes CRI tag
//  2. ctr images rm — removes containerd image record
//  3. ctr content rm — force-removes distribution cache blobs
func purgeContainerImages(ctx context.Context, images []string) {
	for _, img := range images {
		// Step 1: Remove CRI tag
		exec.CommandContext(ctx, "crictl", "rmi", img).Run()

		// Step 2: Remove containerd image records (tagged + by-digest)
		exec.CommandContext(ctx, "ctr", "-a", ctrSock, "-n", "k8s.io", "images", "rm", img).Run()
		digestOut, _ := exec.CommandContext(ctx, "ctr", "-a", ctrSock, "-n", "k8s.io", "images", "ls", "-q").Output()
		repoPrefix := img
		if idx := strings.LastIndex(repoPrefix, ":"); idx > 0 && !strings.Contains(repoPrefix[idx:], "/") {
			repoPrefix = repoPrefix[:idx]
		}
		for _, line := range strings.Split(string(digestOut), "\n") {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "@sha256:") && strings.HasPrefix(line, repoPrefix) {
				exec.CommandContext(ctx, "ctr", "-a", ctrSock, "-n", "k8s.io", "images", "rm", line).Run()
			}
		}

		// Step 3: Force-remove distribution cache blobs matching this image's repo
		repoRef := repoPrefix
		if idx := strings.Index(repoRef, "/"); idx > 0 && strings.Contains(repoRef[:idx], ".") {
			repoRef = repoRef[idx+1:]
		}
		contentOut, _ := exec.CommandContext(ctx, "ctr", "-a", ctrSock, "-n", "k8s.io", "content", "ls").Output()
		for _, line := range strings.Split(string(contentOut), "\n") {
			if strings.Contains(line, repoRef) {
				fields := strings.Fields(line)
				if len(fields) > 0 {
					exec.CommandContext(ctx, "ctr", "-a", ctrSock, "-n", "k8s.io", "content", "rm", fields[0]).Run()
				}
			}
		}

		klog.Infof("purged image %s", img)
	}

	// Note: Do NOT run "crictl rmi --prune" or "ctr content prune" here.
	// That would remove ALL unused images on the node, including images
	// for apps that are installed but stopped (scaled to 0 replicas).
}
