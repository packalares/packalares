package containerd

import (
	"fmt"
	"os"
	"os/exec"
)

const containerdConfig = `version = 2

[plugins]
  [plugins."io.containerd.grpc.v1.cri"]
    sandbox_image = "registry.k8s.io/pause:3.9"
    [plugins."io.containerd.grpc.v1.cri".containerd]
      [plugins."io.containerd.grpc.v1.cri".containerd.runtimes]
        [plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc]
          runtime_type = "io.containerd.runc.v2"
          [plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc.options]
            SystemdCgroup = true
    [plugins."io.containerd.grpc.v1.cri".registry]
      [plugins."io.containerd.grpc.v1.cri".registry.mirrors]
        [plugins."io.containerd.grpc.v1.cri".registry.mirrors."docker.io"]
          endpoint = ["https://registry-1.docker.io"]
`

const containerdService = `[Unit]
Description=containerd container runtime
Documentation=https://containerd.io
After=network.target local-fs.target

[Service]
ExecStart=/usr/local/bin/containerd
Restart=always
RestartSec=5
LimitNPROC=infinity
LimitCORE=infinity
LimitNOFILE=1048576
TasksMax=infinity
OOMScoreAdjust=-999

[Install]
WantedBy=multi-user.target
`

func Install(baseDir string) error {
	// containerd binary should already be in /usr/local/bin from download phase
	if _, err := os.Stat("/usr/local/bin/containerd"); os.IsNotExist(err) {
		return fmt.Errorf("containerd binary not found — run download phase first")
	}

	// Write config
	if err := os.MkdirAll("/etc/containerd", 0755); err != nil {
		return fmt.Errorf("create containerd config dir: %w", err)
	}
	if err := os.WriteFile("/etc/containerd/config.toml", []byte(containerdConfig), 0644); err != nil {
		return fmt.Errorf("write containerd config: %w", err)
	}

	// Write systemd unit
	if err := os.WriteFile("/etc/systemd/system/containerd.service", []byte(containerdService), 0644); err != nil {
		return fmt.Errorf("write containerd service: %w", err)
	}

	// Enable and start
	cmds := [][]string{
		{"systemctl", "daemon-reload"},
		{"systemctl", "enable", "containerd"},
		{"systemctl", "start", "containerd"},
	}

	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("run %v: %s\n%w", args, string(out), err)
		}
	}

	fmt.Println("  containerd installed and running")
	return nil
}
