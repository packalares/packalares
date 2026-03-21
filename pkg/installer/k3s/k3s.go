package k3s

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	etcdCertDir = "/etc/ssl/etcd/ssl"
)

func Install(baseDir, registry string) error {
	// Verify binary exists
	if _, err := os.Stat("/usr/local/bin/k3s"); os.IsNotExist(err) {
		return fmt.Errorf("k3s binary not found — run download phase first")
	}

	// Create symlinks
	for _, link := range []string{"kubectl", "crictl", "ctr"} {
		target := "/usr/local/bin/" + link
		if _, err := os.Stat(target); os.IsNotExist(err) {
			os.Symlink("/usr/local/bin/k3s", target)
		}
	}

	// Generate K3s service file
	serviceContent := generateK3sService(registry)
	if err := os.WriteFile("/etc/systemd/system/k3s.service", []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("write k3s service: %w", err)
	}

	// Generate K3s env file
	envContent := generateK3sEnv()
	os.MkdirAll("/etc/systemd/system/k3s.service.d", 0755)
	if err := os.WriteFile("/etc/systemd/system/k3s.service.d/env.conf", []byte(envContent), 0644); err != nil {
		return fmt.Errorf("write k3s env: %w", err)
	}

	// Create K3s config
	configContent := generateK3sConfig(registry)
	os.MkdirAll("/etc/rancher/k3s", 0755)
	if err := os.WriteFile("/etc/rancher/k3s/config.yaml", []byte(configContent), 0644); err != nil {
		return fmt.Errorf("write k3s config: %w", err)
	}

	// Enable and start
	cmds := [][]string{
		{"systemctl", "daemon-reload"},
		{"systemctl", "enable", "k3s"},
		{"systemctl", "start", "k3s"},
	}

	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("run %v: %s\n%w", args, string(out), err)
		}
	}

	// Wait for K3s to be ready
	fmt.Println("  Waiting for K3s to be ready ...")
	kubeconfig := "/etc/rancher/k3s/k3s.yaml"
	for i := 0; i < 60; i++ {
		cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "get", "nodes")
		if err := cmd.Run(); err == nil {
			break
		}
		time.Sleep(5 * time.Second)
	}

	// Set up default kubeconfig
	os.MkdirAll(os.Getenv("HOME")+"/.kube", 0755)
	exec.Command("cp", kubeconfig, os.Getenv("HOME")+"/.kube/config").Run()
	exec.Command("chmod", "600", os.Getenv("HOME")+"/.kube/config").Run()

	fmt.Println("  K3s installed and running")
	return nil
}

func generateK3sService(registry string) string {
	return `[Unit]
Description=Lightweight Kubernetes (K3s)
Documentation=https://k3s.io
After=network-online.target etcd.service containerd.service
Wants=network-online.target

[Service]
Type=notify
EnvironmentFile=-/etc/systemd/system/k3s.service.d/env.conf
ExecStart=/usr/local/bin/k3s server --config /etc/rancher/k3s/config.yaml
KillMode=process
Delegate=yes
LimitNOFILE=1048576
LimitNPROC=infinity
LimitCORE=infinity
TasksMax=infinity
TimeoutStartSec=0
Restart=always
RestartSec=5s

[Install]
WantedBy=multi-user.target
`
}

func generateK3sEnv() string {
	return `[Service]
Environment="K3S_KUBECONFIG_MODE=644"
`
}

func generateK3sConfig(registry string) string {
	var b strings.Builder
	b.WriteString("# K3s configuration for Packalares\n")
	b.WriteString("write-kubeconfig-mode: '0644'\n")
	b.WriteString("disable:\n")
	b.WriteString("  - traefik\n")
	b.WriteString("  - servicelb\n")
	b.WriteString("  - local-storage\n")
	b.WriteString("flannel-backend: none\n")
	b.WriteString("disable-network-policy: true\n")
	b.WriteString("cluster-cidr: '10.233.64.0/18'\n")
	b.WriteString("service-cidr: '10.233.0.0/18'\n")
	b.WriteString("cluster-dns: '10.233.0.10'\n")

	// External etcd
	b.WriteString(fmt.Sprintf("datastore-endpoint: 'https://127.0.0.1:2379'\n"))
	b.WriteString(fmt.Sprintf("datastore-cafile: '%s/ca.pem'\n", etcdCertDir))
	b.WriteString(fmt.Sprintf("datastore-certfile: '%s/server.pem'\n", etcdCertDir))
	b.WriteString(fmt.Sprintf("datastore-keyfile: '%s/server-key.pem'\n", etcdCertDir))

	// Kubelet args
	b.WriteString("kubelet-arg:\n")
	b.WriteString("  - 'max-pods=220'\n")
	b.WriteString("  - 'serialize-image-pulls=false'\n")
	b.WriteString("  - 'image-gc-high-threshold=85'\n")
	b.WriteString("  - 'image-gc-low-threshold=80'\n")

	// Private registry mirror
	if registry != "" {
		b.WriteString(fmt.Sprintf("# Private registry: %s\n", registry))
	}

	return b.String()
}
