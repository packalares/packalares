package redis

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const redisService = `[Unit]
Description=Redis In-Memory Data Store
After=network.target

[Service]
ExecStart=/usr/bin/redis-server /etc/redis/redis.conf
ExecStop=/usr/bin/redis-cli shutdown
Restart=always
RestartSec=3
LimitNOFILE=65536
Type=notify

[Install]
WantedBy=multi-user.target
`

func Install(baseDir string) error {
	// Check if redis-server is available
	redisPath, err := exec.LookPath("redis-server")
	if err != nil {
		// Try to install via package manager
		fmt.Println("  Installing redis-server via package manager ...")
		if err := installRedisPackage(); err != nil {
			return fmt.Errorf("redis-server not found and could not be installed: %w", err)
		}
		redisPath = "/usr/bin/redis-server"
	}
	_ = redisPath

	// Generate password
	pwBytes := make([]byte, 16)
	if _, err := rand.Read(pwBytes); err != nil {
		return fmt.Errorf("generate redis password: %w", err)
	}
	password := hex.EncodeToString(pwBytes)

	// Save password
	os.MkdirAll(filepath.Join(baseDir, "state"), 0700)
	if err := os.WriteFile(filepath.Join(baseDir, "state", "redis_password"), []byte(password), 0600); err != nil {
		return fmt.Errorf("save redis password: %w", err)
	}

	// Generate config
	config := generateRedisConfig(password)
	os.MkdirAll("/etc/redis", 0755)
	os.MkdirAll("/var/lib/redis", 0755)
	os.MkdirAll("/var/log/redis", 0755)
	if err := os.WriteFile("/etc/redis/redis.conf", []byte(config), 0644); err != nil {
		return fmt.Errorf("write redis config: %w", err)
	}

	// Write systemd unit
	if err := os.WriteFile("/etc/systemd/system/redis-server.service", []byte(redisService), 0644); err != nil {
		return fmt.Errorf("write redis service: %w", err)
	}

	// Stop any existing redis
	exec.Command("systemctl", "stop", "redis-server").Run()
	exec.Command("systemctl", "stop", "redis").Run()

	// Enable and start
	cmds := [][]string{
		{"systemctl", "daemon-reload"},
		{"systemctl", "enable", "redis-server"},
		{"systemctl", "start", "redis-server"},
	}

	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("run %v: %s\n%w", args, string(out), err)
		}
	}

	// Wait for redis
	fmt.Println("  Waiting for Redis to be ready ...")
	for i := 0; i < 20; i++ {
		cmd := exec.Command("redis-cli", "-a", password, "ping")
		out, err := cmd.CombinedOutput()
		if err == nil && len(out) >= 4 {
			if string(out[:4]) == "PONG" {
				fmt.Println("  Redis installed and running")
				return nil
			}
		}
		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("redis did not become ready in time")
}

func installRedisPackage() error {
	// Try apt first (Ubuntu/Debian)
	if _, err := exec.LookPath("apt-get"); err == nil {
		cmd := exec.Command("apt-get", "install", "-y", "redis-server")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("apt-get install redis: %s\n%w", string(out), err)
		}
		return nil
	}

	// Try dnf/yum (CentOS/RHEL/Fedora)
	for _, mgr := range []string{"dnf", "yum"} {
		if _, err := exec.LookPath(mgr); err == nil {
			cmd := exec.Command(mgr, "install", "-y", "redis")
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%s install redis: %s\n%w", mgr, string(out), err)
			}
			return nil
		}
	}

	return fmt.Errorf("no supported package manager found (tried apt-get, dnf, yum)")
}

func generateRedisConfig(password string) string {
	return fmt.Sprintf(`# Packalares Redis configuration
bind 127.0.0.1 -::1
port 6379
requirepass %s
dir /var/lib/redis
dbfilename dump.rdb

# Persistence
save 900 1
save 300 10
save 60 10000

# Memory
maxmemory 256mb
maxmemory-policy allkeys-lru

# Security
protected-mode yes
rename-command FLUSHALL ""
rename-command FLUSHDB ""

# Logging
loglevel notice
logfile /var/log/redis/redis-server.log

# Performance
tcp-keepalive 300
timeout 0
tcp-backlog 511

# Supervised by systemd
supervised systemd
`, password)
}
