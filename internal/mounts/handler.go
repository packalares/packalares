package mounts

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// allowedNFSOptions are the only mount options permitted for NFS mounts.
var allowedNFSOptions = map[string]bool{
	"ro": true, "rw": true, "soft": true, "hard": true,
	"intr": true, "nolock": true, "tcp": true, "udp": true,
	"noatime": true, "nodiratime": true, "relatime": true,
}

// allowedNFSPrefixes are option prefixes that accept a value (e.g. nfsvers=4).
var allowedNFSPrefixes = []string{
	"nfsvers=", "vers=", "timeo=", "retrans=", "rsize=", "wsize=",
	"port=", "mountport=", "proto=", "sec=",
}

// allowedSMBOptions are the only mount options permitted for SMB mounts.
var allowedSMBOptions = map[string]bool{
	"ro": true, "rw": true, "noatime": true, "nodiratime": true,
	"guest": true, "noperm": true, "file_mode=0644": true, "dir_mode=0755": true,
}

// allowedSMBPrefixes are option prefixes that accept a value.
var allowedSMBPrefixes = []string{
	"vers=", "sec=", "uid=", "gid=", "file_mode=", "dir_mode=",
	"iocharset=", "domain=", "workgroup=",
}

// allowedRcloneFlags are the only flags permitted for rclone mounts.
var allowedRcloneFlags = map[string]bool{
	"--read-only":  true,
	"--no-modtime": true,
	"--no-checksum": true,
}

// allowedRclonePrefixes are flag prefixes that accept a value.
var allowedRclonePrefixes = []string{
	"--bwlimit=", "--buffer-size=", "--vfs-read-chunk-size=",
	"--vfs-cache-max-age=", "--vfs-cache-max-size=", "--transfers=",
}

// sanitizeMountOptions filters options against a whitelist.
func sanitizeMountOptions(raw string, allowed map[string]bool, prefixes []string) (string, error) {
	if raw == "" {
		return "", nil
	}
	var result []string
	for _, opt := range strings.Split(raw, ",") {
		opt = strings.TrimSpace(opt)
		if opt == "" {
			continue
		}
		if allowed[opt] {
			result = append(result, opt)
			continue
		}
		ok := false
		for _, p := range prefixes {
			if strings.HasPrefix(opt, p) {
				ok = true
				break
			}
		}
		if !ok {
			return "", fmt.Errorf("mount option %q not allowed", opt)
		}
		result = append(result, opt)
	}
	return strings.Join(result, ","), nil
}

// sanitizeRcloneFlags filters rclone flags against a whitelist.
func sanitizeRcloneFlags(raw string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}
	var result []string
	for _, flag := range strings.Split(raw, " ") {
		flag = strings.TrimSpace(flag)
		if flag == "" {
			continue
		}
		if allowedRcloneFlags[flag] {
			result = append(result, flag)
			continue
		}
		ok := false
		for _, p := range allowedRclonePrefixes {
			if strings.HasPrefix(flag, p) {
				ok = true
				break
			}
		}
		if !ok {
			return nil, fmt.Errorf("rclone flag %q not allowed", flag)
		}
		result = append(result, flag)
	}
	return result, nil
}

// MountType represents the type of remote storage.
type MountType string

const (
	MountTypeSMB    MountType = "smb"
	MountTypeNFS    MountType = "nfs"
	MountTypeRclone MountType = "rclone"
)

// MountConfig describes a mount to create.
type MountConfig struct {
	Name     string    `json:"name"`
	Type     MountType `json:"type"`
	Address  string    `json:"address"`
	Share    string    `json:"share"`
	User     string    `json:"user,omitempty"`
	Password string    `json:"password,omitempty"`
	Remote   string    `json:"remote,omitempty"`   // rclone remote spec, e.g. "s3:mybucket"
	Options  string    `json:"options,omitempty"`   // extra mount options
}

// MountInfo describes an active mount.
type MountInfo struct {
	Name      string    `json:"name"`
	Type      MountType `json:"type"`
	Address   string    `json:"address"`
	Share     string    `json:"share"`
	MountPath string    `json:"mountPath"`
	Status    string    `json:"status"`
}

// Handler manages storage mounts.
type Handler struct {
	basePath string
	mu       sync.RWMutex
	mounts   map[string]*MountInfo
}

// NewHandler creates a mount handler. basePath is where mounts appear (e.g., /packalares/mounts).
func NewHandler(basePath string) *Handler {
	h := &Handler{
		basePath: basePath,
		mounts:   make(map[string]*MountInfo),
	}
	os.MkdirAll(basePath, 0755)
	return h
}

// RegisterRoutes wires up mount API routes.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/mounts", h.handleMounts)
	mux.HandleFunc("/api/mounts/", h.handleMountByName)
}

func (h *Handler) handleMounts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listMounts(w, r)
	case http.MethodPost:
		h.createMount(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleMountByName(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/mounts/")
	if name == "" {
		h.handleMounts(w, r)
		return
	}

	switch r.Method {
	case http.MethodDelete:
		h.removeMount(w, r, name)
	case http.MethodGet:
		h.getMount(w, r, name)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) listMounts(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]MountInfo, 0, len(h.mounts))
	for _, m := range h.mounts {
		result = append(result, *m)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (h *Handler) getMount(w http.ResponseWriter, r *http.Request, name string) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	m, ok := h.mounts[name]
	if !ok {
		http.Error(w, "mount not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(m)
}

func (h *Handler) createMount(w http.ResponseWriter, r *http.Request) {
	var cfg MountConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if cfg.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	// Sanitize name to prevent path traversal
	safeName := filepath.Base(cfg.Name)
	if safeName != cfg.Name || safeName == "." || safeName == ".." {
		http.Error(w, "invalid mount name", http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.mounts[cfg.Name]; exists {
		http.Error(w, "mount already exists", http.StatusConflict)
		return
	}

	mountPoint := filepath.Join(h.basePath, cfg.Name)
	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		http.Error(w, fmt.Sprintf("failed to create mount point: %v", err), http.StatusInternalServerError)
		return
	}

	var err error
	switch cfg.Type {
	case MountTypeSMB:
		err = mountSMB(cfg, mountPoint)
	case MountTypeNFS:
		err = mountNFS(cfg, mountPoint)
	case MountTypeRclone:
		err = mountRclone(cfg, mountPoint)
	default:
		os.Remove(mountPoint)
		http.Error(w, "unsupported mount type", http.StatusBadRequest)
		return
	}

	if err != nil {
		os.Remove(mountPoint)
		http.Error(w, fmt.Sprintf("mount failed: %v", err), http.StatusInternalServerError)
		return
	}

	info := &MountInfo{
		Name:      cfg.Name,
		Type:      cfg.Type,
		Address:   cfg.Address,
		Share:     cfg.Share,
		MountPath: mountPoint,
		Status:    "mounted",
	}
	h.mounts[cfg.Name] = info

	log.Printf("mount created: %s (%s) at %s", cfg.Name, cfg.Type, mountPoint)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(info)
}

func (h *Handler) removeMount(w http.ResponseWriter, r *http.Request, name string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	m, ok := h.mounts[name]
	if !ok {
		http.Error(w, "mount not found", http.StatusNotFound)
		return
	}

	// Unmount
	if err := unmount(m.MountPath); err != nil {
		log.Printf("warning: unmount %s failed: %v", m.MountPath, err)
	}

	// Remove mount point directory
	os.Remove(m.MountPath)

	delete(h.mounts, name)

	log.Printf("mount removed: %s", name)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ensureCIFSModules loads kernel modules required for CIFS mounts.
func ensureCIFSModules() {
	for _, mod := range []string{"cifs", "cmac", "aes"} {
		exec.Command("modprobe", mod).Run()
	}
}

// mountSMB mounts an SMB/CIFS share.
func mountSMB(cfg MountConfig, mountPoint string) error {
	ensureCIFSModules()
	source := fmt.Sprintf("//%s/%s", cfg.Address, cfg.Share)

	args := []string{"-t", "cifs", source, mountPoint}

	opts := []string{}
	if cfg.User != "" {
		// Escape commas in username to prevent mount option injection
		opts = append(opts, fmt.Sprintf("username=%s", strings.ReplaceAll(cfg.User, ",", "\\,")))
	}
	if cfg.Password != "" {
		// Escape commas in password to prevent mount option injection
		opts = append(opts, fmt.Sprintf("password=%s", strings.ReplaceAll(cfg.Password, ",", "\\,")))
	}
	if cfg.Options != "" {
		sanitized, err := sanitizeMountOptions(cfg.Options, allowedSMBOptions, allowedSMBPrefixes)
		if err != nil {
			return fmt.Errorf("smb options: %w", err)
		}
		if sanitized != "" {
			opts = append(opts, sanitized)
		}
	}
	// Default to SMB 3.0 if no version specified
	hasVers := false
	for _, o := range opts {
		if strings.HasPrefix(o, "vers=") {
			hasVers = true
			break
		}
	}
	if !hasVers {
		opts = append(opts, "vers=3.0")
	}
	if len(opts) == 1 && opts[0] == "vers=3.0" && cfg.User == "" {
		opts = append(opts, "guest")
	}

	args = append(args, "-o", strings.Join(opts, ","))

	cmd := exec.Command("mount", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mount.cifs failed: %s: %w", string(output), err)
	}
	return nil
}

// mountNFS mounts an NFS share.
func mountNFS(cfg MountConfig, mountPoint string) error {
	source := fmt.Sprintf("%s:%s", cfg.Address, cfg.Share)

	args := []string{"-t", "nfs", source, mountPoint}

	if cfg.Options != "" {
		sanitized, err := sanitizeMountOptions(cfg.Options, allowedNFSOptions, allowedNFSPrefixes)
		if err != nil {
			return fmt.Errorf("nfs options: %w", err)
		}
		if sanitized != "" {
			args = append(args, "-o", sanitized)
		}
	}

	cmd := exec.Command("mount", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mount.nfs failed: %s: %w", string(output), err)
	}
	return nil
}

// mountRclone mounts cloud storage via rclone.
func mountRclone(cfg MountConfig, mountPoint string) error {
	remote := cfg.Remote
	if remote == "" {
		remote = fmt.Sprintf("%s:%s", cfg.Address, cfg.Share)
	}

	args := []string{"mount", remote, mountPoint,
		"--daemon",
		"--vfs-cache-mode", "full",
		"--allow-other",
		"--log-level", "INFO",
	}

	if cfg.Options != "" {
		flags, err := sanitizeRcloneFlags(cfg.Options)
		if err != nil {
			return fmt.Errorf("rclone options: %w", err)
		}
		args = append(args, flags...)
	}

	cmd := exec.Command("rclone", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rclone mount failed: %s: %w", string(output), err)
	}
	return nil
}

// unmount removes a mount point.
func unmount(mountPoint string) error {
	// Try fusermount first (for rclone mounts)
	cmd := exec.Command("fusermount", "-u", mountPoint)
	if err := cmd.Run(); err != nil {
		// Fall back to regular umount
		cmd = exec.Command("umount", mountPoint)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("umount failed: %s: %w", string(output), err)
		}
	}
	return nil
}
