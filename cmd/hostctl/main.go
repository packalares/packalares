package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// nsenterPrefix is the command prefix to execute in the host's PID 1 namespace.
var nsenterPrefix = []string{"/usr/bin/nsenter", "-t", "1", "-m", "-u", "-n", "-i", "--"}

// cmdTimeout is the maximum duration for any nsenter command.
const cmdTimeout = 30 * time.Second

// SSHStatus represents the current SSH daemon state.
type SSHStatus struct {
	Enabled bool `json:"enabled"`
	Port    int  `json:"port"`
}

func main() {
	token := os.Getenv("HOSTCTL_TOKEN")
	if token == "" {
		log.Fatal("HOSTCTL_TOKEN environment variable is required")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ssh/status", withAuth(token, handleSSHStatus))
	mux.HandleFunc("/ssh/config", withAuth(token, handleSSHConfig))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	srv := &http.Server{
		Addr:         ":9199",
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("received signal %v, shutting down", sig)
		cancel()
	}()

	log.Printf("hostctl listening on %s", srv.Addr)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown error: %v", err)
		}
	case err := <-errCh:
		log.Fatalf("server error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Auth middleware
// ---------------------------------------------------------------------------

func withAuth(token string, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
			respondErr(w, http.StatusUnauthorized, "missing or invalid authorization header")
			return
		}
		if strings.TrimPrefix(auth, "Bearer ") != token {
			respondErr(w, http.StatusForbidden, "invalid token")
			return
		}
		handler(w, r)
	}
}

// ---------------------------------------------------------------------------
// GET /ssh/status
// ---------------------------------------------------------------------------

func handleSSHStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	status := getSSHStatus()
	respondJSON(w, http.StatusOK, status)
}

func getSSHStatus() SSHStatus {
	status := SSHStatus{Port: 22, Enabled: false}

	// Check if sshd is active
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, nsenterPrefix[0], append(nsenterPrefix[1:], "systemctl", "is-active", "sshd")...).CombinedOutput()
	if err == nil && strings.TrimSpace(string(out)) == "active" {
		status.Enabled = true
	}

	// Parse port from sshd_config
	ctx2, cancel2 := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel2()
	cfgOut, err := exec.CommandContext(ctx2, nsenterPrefix[0], append(nsenterPrefix[1:], "cat", "/etc/ssh/sshd_config")...).CombinedOutput()
	if err == nil {
		for _, line := range strings.Split(string(cfgOut), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "#") {
				continue
			}
			if strings.HasPrefix(line, "Port ") || strings.HasPrefix(line, "Port\t") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					if p, err := strconv.Atoi(fields[1]); err == nil {
						status.Port = p
					}
				}
			}
		}
	}

	return status
}

// ---------------------------------------------------------------------------
// POST /ssh/config
// ---------------------------------------------------------------------------

func handleSSHConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
		Port    int  `json:"port"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondErr(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	// Validate port
	if req.Port != 22 && (req.Port < 1024 || req.Port > 65535) {
		respondErr(w, http.StatusBadRequest, "port must be 22 or in range 1024-65535")
		return
	}

	log.Printf("[ssh-config] request: enabled=%v port=%d at %s", req.Enabled, req.Port, time.Now().UTC().Format(time.RFC3339))

	// Update port in sshd_config
	if err := setSSHPort(req.Port); err != nil {
		respondErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to set SSH port: %v", err))
		return
	}

	// Enable or disable sshd
	if req.Enabled {
		if err := nsenterRun("systemctl", "enable", "sshd"); err != nil {
			respondErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to enable sshd: %v", err))
			return
		}
		if err := nsenterRun("systemctl", "restart", "sshd"); err != nil {
			respondErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to restart sshd: %v", err))
			return
		}
		log.Printf("[ssh-config] sshd enabled on port %d at %s", req.Port, time.Now().UTC().Format(time.RFC3339))
	} else {
		if err := nsenterRun("systemctl", "stop", "sshd"); err != nil {
			respondErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to stop sshd: %v", err))
			return
		}
		if err := nsenterRun("systemctl", "disable", "sshd"); err != nil {
			respondErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to disable sshd: %v", err))
			return
		}
		log.Printf("[ssh-config] sshd disabled at %s", time.Now().UTC().Format(time.RFC3339))
	}

	// Return new status
	status := getSSHStatus()
	respondJSON(w, http.StatusOK, status)
}

func setSSHPort(port int) error {
	// Use sed via nsenter to update the Port line in sshd_config.
	// This handles both "Port NNN" and "#Port NNN" lines.
	sedExpr := fmt.Sprintf("s/^#*Port .*/Port %d/", port)
	return nsenterRun("sed", "-i", sedExpr, "/etc/ssh/sshd_config")
}

func nsenterRun(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()
	cmdArgs := append(nsenterPrefix[1:], args...)
	cmd := exec.CommandContext(ctx, nsenterPrefix[0], cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s (%w)", strings.Join(args, " "), strings.TrimSpace(string(out)), err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// JSON helpers
// ---------------------------------------------------------------------------

func respondJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}

func respondErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
