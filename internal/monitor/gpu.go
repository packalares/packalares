package monitor

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// GPUInfo describes a single detected GPU.
type GPUInfo struct {
	Index             int     `json:"index"`
	Name              string  `json:"name"`
	UUID              string  `json:"uuid"`
	Vendor            string  `json:"vendor"`
	MemoryTotalMB     uint64  `json:"memory_total_mb"`
	MemoryUsedMB      uint64  `json:"memory_used_mb"`
	MemoryFreeMB      uint64  `json:"memory_free_mb"`
	GPUUtilization    int     `json:"gpu_utilization"`
	MemoryUtilization int     `json:"memory_utilization"`
	Temperature       int     `json:"temperature"`
	PowerDrawW        float64 `json:"power_draw_w"`
	DriverVersion     string  `json:"driver_version"`
}

// GPUListResponse is the JSON response for the GPU detection endpoint.
type GPUListResponse struct {
	GPUs            []GPUInfo `json:"gpus"`
	DriverInstalled bool      `json:"driver_installed"`
	CUDAVersion     string    `json:"cuda_version,omitempty"`
	GPUCount        int       `json:"gpu_count"`
}

// detectGPUs gathers GPU information from nvidia-smi and sysfs.
func detectGPUs() GPUListResponse {
	resp := GPUListResponse{
		GPUs: []GPUInfo{},
	}

	// Try NVIDIA GPUs first via nvidia-smi.
	nvidiaGPUs, cudaVersion, driverInstalled := detectNVIDIAGPUs()
	resp.DriverInstalled = driverInstalled
	resp.CUDAVersion = cudaVersion
	resp.GPUs = append(resp.GPUs, nvidiaGPUs...)

	// Try AMD GPUs via sysfs.
	amdGPUs := detectAMDGPUs(len(resp.GPUs))
	resp.GPUs = append(resp.GPUs, amdGPUs...)

	resp.GPUCount = len(resp.GPUs)
	return resp
}

// detectNVIDIAGPUs runs nvidia-smi and parses the CSV output.
// Returns the list of GPUs, the CUDA version string, and whether the driver is installed.
func detectNVIDIAGPUs() ([]GPUInfo, string, bool) {
	nvidiaSmi, err := exec.LookPath("nvidia-smi")
	if err != nil {
		return nil, "", false
	}
	_ = nvidiaSmi

	// Query per-GPU details.
	out, err := exec.Command("nvidia-smi",
		"--query-gpu=name,uuid,memory.total,memory.used,memory.free,utilization.gpu,utilization.memory,temperature.gpu,power.draw,driver_version",
		"--format=csv,noheader,nounits",
	).Output()
	if err != nil {
		log.Printf("gpu: nvidia-smi query failed: %v", err)
		return nil, "", false
	}

	// Get CUDA version from nvidia-smi header output.
	cudaVersion := detectCUDAVersion()

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var gpus []GPUInfo
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, ", ")
		if len(fields) < 10 {
			log.Printf("gpu: nvidia-smi line %d: expected 10 fields, got %d: %s", i, len(fields), line)
			continue
		}

		gpu := GPUInfo{
			Index:         i,
			Vendor:        "NVIDIA",
			Name:          strings.TrimSpace(fields[0]),
			UUID:          strings.TrimSpace(fields[1]),
			DriverVersion: strings.TrimSpace(fields[9]),
		}

		gpu.MemoryTotalMB = parseUint64Field(fields[2])
		gpu.MemoryUsedMB = parseUint64Field(fields[3])
		gpu.MemoryFreeMB = parseUint64Field(fields[4])
		gpu.GPUUtilization = parseIntField(fields[5])
		gpu.MemoryUtilization = parseIntField(fields[6])
		gpu.Temperature = parseIntField(fields[7])
		gpu.PowerDrawW = parseFloatField(fields[8])

		gpus = append(gpus, gpu)
	}

	return gpus, cudaVersion, true
}

// detectCUDAVersion runs nvidia-smi (no args) and parses the CUDA Version from its output.
func detectCUDAVersion() string {
	out, err := exec.Command("nvidia-smi").Output()
	if err != nil {
		return ""
	}
	// The output contains a line like: "CUDA Version: 12.3"
	for _, line := range strings.Split(string(out), "\n") {
		if idx := strings.Index(line, "CUDA Version:"); idx >= 0 {
			part := strings.TrimSpace(line[idx+len("CUDA Version:"):])
			// Take up to the first space or pipe.
			for i, c := range part {
				if c == ' ' || c == '|' {
					return part[:i]
				}
			}
			return part
		}
	}
	return ""
}

// detectAMDGPUs scans sysfs for AMD GPU devices.
// startIndex is the GPU index offset (to continue numbering after NVIDIA GPUs).
func detectAMDGPUs(startIndex int) []GPUInfo {
	// Look for DRM card devices with AMD vendor ID (0x1002).
	matches, err := filepath.Glob("/sys/class/drm/card[0-9]*/device/vendor")
	if err != nil {
		return nil
	}

	var gpus []GPUInfo
	idx := startIndex
	for _, vendorPath := range matches {
		data, err := os.ReadFile(vendorPath)
		if err != nil {
			continue
		}
		vendor := strings.TrimSpace(string(data))
		if vendor != "0x1002" {
			continue
		}

		deviceDir := filepath.Dir(vendorPath)
		cardDir := filepath.Dir(deviceDir)

		gpu := GPUInfo{
			Index:  idx,
			Vendor: "AMD",
			Name:   readAMDGPUName(deviceDir, cardDir),
		}

		// Read memory info if available (amdgpu driver exposes these).
		gpu.MemoryTotalMB = readAMDGPUMemTotal(deviceDir)
		gpu.MemoryUsedMB = readAMDGPUMemUsed(deviceDir)
		if gpu.MemoryTotalMB > 0 && gpu.MemoryUsedMB <= gpu.MemoryTotalMB {
			gpu.MemoryFreeMB = gpu.MemoryTotalMB - gpu.MemoryUsedMB
		}

		// Read GPU utilization if available.
		gpu.GPUUtilization = readAMDGPUBusyPercent(deviceDir)

		// Read temperature from hwmon.
		gpu.Temperature = readAMDGPUTemperature(cardDir)

		gpus = append(gpus, gpu)
		idx++
	}

	return gpus
}

// readAMDGPUName tries to read a human-readable name for an AMD GPU.
func readAMDGPUName(deviceDir, cardDir string) string {
	// Try reading the product name from the device directory.
	for _, nameFile := range []string{
		filepath.Join(deviceDir, "product_name"),
		filepath.Join(deviceDir, "label"),
	} {
		data, err := os.ReadFile(nameFile)
		if err == nil {
			name := strings.TrimSpace(string(data))
			if name != "" {
				return name
			}
		}
	}

	// Fall back to the card directory name.
	return fmt.Sprintf("AMD GPU (%s)", filepath.Base(cardDir))
}

// readAMDGPUMemTotal reads total VRAM in MB from amdgpu sysfs.
func readAMDGPUMemTotal(deviceDir string) uint64 {
	// amdgpu exposes mem_info_vram_total in bytes.
	data, err := os.ReadFile(filepath.Join(deviceDir, "mem_info_vram_total"))
	if err != nil {
		return 0
	}
	val, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0
	}
	return val / (1024 * 1024)
}

// readAMDGPUMemUsed reads used VRAM in MB from amdgpu sysfs.
func readAMDGPUMemUsed(deviceDir string) uint64 {
	data, err := os.ReadFile(filepath.Join(deviceDir, "mem_info_vram_used"))
	if err != nil {
		return 0
	}
	val, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0
	}
	return val / (1024 * 1024)
}

// readAMDGPUBusyPercent reads GPU utilization from gpu_busy_percent sysfs file.
func readAMDGPUBusyPercent(deviceDir string) int {
	data, err := os.ReadFile(filepath.Join(deviceDir, "gpu_busy_percent"))
	if err != nil {
		return 0
	}
	val, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return val
}

// readAMDGPUTemperature reads the GPU temperature from hwmon.
func readAMDGPUTemperature(cardDir string) int {
	// hwmon devices are under the device directory.
	hwmonDirs, err := filepath.Glob(filepath.Join(cardDir, "device", "hwmon", "hwmon*"))
	if err != nil || len(hwmonDirs) == 0 {
		return 0
	}
	// Read temp1_input (millidegrees Celsius).
	data, err := os.ReadFile(filepath.Join(hwmonDirs[0], "temp1_input"))
	if err != nil {
		return 0
	}
	val, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return val / 1000 // Convert from millidegrees to degrees.
}

// ---------------------------------------------------------------------------
// Field parsing helpers
// ---------------------------------------------------------------------------

func parseUint64Field(s string) uint64 {
	s = strings.TrimSpace(s)
	v, _ := strconv.ParseUint(s, 10, 64)
	return v
}

func parseIntField(s string) int {
	s = strings.TrimSpace(s)
	v, _ := strconv.Atoi(s)
	return v
}

func parseFloatField(s string) float64 {
	s = strings.TrimSpace(s)
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
