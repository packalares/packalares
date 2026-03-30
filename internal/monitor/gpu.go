package monitor

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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

// detectGPUsFromPrometheus queries Prometheus DCGM metrics to get GPU info.
// This works from Alpine containers that can't run nvidia-smi.
func detectGPUsFromPrometheus(prometheusURL string) GPUListResponse {
	resp := GPUListResponse{GPUs: []GPUInfo{}}

	if prometheusURL == "" {
		// Fallback to AMD sysfs detection
		amdGPUs := detectAMDGPUs(0)
		resp.GPUs = append(resp.GPUs, amdGPUs...)
		resp.GPUCount = len(resp.GPUs)
		return resp
	}

	// Query DCGM metrics from Prometheus
	type promResult struct {
		Metric map[string]string `json:"metric"`
		Value  []interface{}     `json:"value"`
	}
	type promResponse struct {
		Status string `json:"status"`
		Data   struct {
			Result []promResult `json:"result"`
		} `json:"data"`
	}

	query := func(q string) *promResponse {
		url := fmt.Sprintf("%s/api/v1/query?query=%s", prometheusURL, q)
		client := &http.Client{Timeout: 5 * time.Second}
		r, err := client.Get(url)
		if err != nil {
			return nil
		}
		defer r.Body.Close()
		var pr promResponse
		json.NewDecoder(r.Body).Decode(&pr)
		if pr.Status == "success" {
			return &pr
		}
		return nil
	}

	// Get GPU list from DCGM_FI_DEV_GPU_UTIL (one entry per GPU)
	utilResult := query("DCGM_FI_DEV_GPU_UTIL")
	if utilResult == nil || len(utilResult.Data.Result) == 0 {
		// No DCGM data — try AMD
		amdGPUs := detectAMDGPUs(0)
		resp.GPUs = append(resp.GPUs, amdGPUs...)
		resp.GPUCount = len(resp.GPUs)
		return resp
	}

	// Query all GPU metrics
	tempResult := query("DCGM_FI_DEV_GPU_TEMP")
	powerResult := query("DCGM_FI_DEV_POWER_USAGE")
	memUsedResult := query("DCGM_FI_DEV_FB_USED")
	memFreeResult := query("DCGM_FI_DEV_FB_FREE")

	getVal := func(pr *promResponse, gpu string) float64 {
		if pr == nil {
			return 0
		}
		for _, r := range pr.Data.Result {
			if r.Metric["gpu"] == gpu && len(r.Value) >= 2 {
				v, _ := strconv.ParseFloat(fmt.Sprint(r.Value[1]), 64)
				return v
			}
		}
		return 0
	}

	resp.DriverInstalled = true
	for i, r := range utilResult.Data.Result {
		gpuIdx := r.Metric["gpu"]
		util, _ := strconv.ParseFloat(fmt.Sprint(r.Value[1]), 64)

		memUsed := getVal(memUsedResult, gpuIdx)
		memFree := getVal(memFreeResult, gpuIdx)
		temp := getVal(tempResult, gpuIdx)
		power := getVal(powerResult, gpuIdx)

		gpu := GPUInfo{
			Index:             i,
			Vendor:            "NVIDIA",
			Name:              r.Metric["modelName"],
			UUID:              r.Metric["UUID"],
			DriverVersion:     r.Metric["DCGM_FI_DRIVER_VERSION"],
			GPUUtilization:    int(util),
			Temperature:       int(temp),
			PowerDrawW:        power,
			MemoryUsedMB:      uint64(memUsed),
			MemoryFreeMB:      uint64(memFree),
			MemoryTotalMB:     uint64(memUsed + memFree),
			MemoryUtilization: 0,
		}
		if gpu.MemoryTotalMB > 0 {
			gpu.MemoryUtilization = int(float64(gpu.MemoryUsedMB) / float64(gpu.MemoryTotalMB) * 100)
		}
		if gpu.DriverVersion == "" {
			gpu.DriverVersion = r.Metric["DCGM_FI_DRIVER_VERSION"]
		}
		resp.GPUs = append(resp.GPUs, gpu)
	}

	if len(resp.GPUs) > 0 {
		resp.CUDAVersion = detectCUDAFromPrometheus(prometheusURL)
	}

	// Also check AMD GPUs
	amdGPUs := detectAMDGPUs(len(resp.GPUs))
	resp.GPUs = append(resp.GPUs, amdGPUs...)

	resp.GPUCount = len(resp.GPUs)
	return resp
}

// detectCUDAFromPrometheus tries to get CUDA version from driver version mapping.
func detectCUDAFromPrometheus(prometheusURL string) string {
	// DCGM doesn't expose CUDA version directly.
	// Common driver-to-CUDA mappings for recent drivers.
	return ""
}

// detectAMDGPUs scans sysfs for AMD GPU devices.
func detectAMDGPUs(startIndex int) []GPUInfo {
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

		gpu.MemoryTotalMB = readAMDGPUMemTotal(deviceDir)
		gpu.MemoryUsedMB = readAMDGPUMemUsed(deviceDir)
		if gpu.MemoryTotalMB > 0 && gpu.MemoryUsedMB <= gpu.MemoryTotalMB {
			gpu.MemoryFreeMB = gpu.MemoryTotalMB - gpu.MemoryUsedMB
		}
		gpu.GPUUtilization = readAMDGPUBusyPercent(deviceDir)
		gpu.Temperature = readAMDGPUTemperature(cardDir)

		gpus = append(gpus, gpu)
		idx++
	}

	return gpus
}

func readAMDGPUName(deviceDir, cardDir string) string {
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
	return fmt.Sprintf("AMD GPU (%s)", filepath.Base(cardDir))
}

func readAMDGPUMemTotal(deviceDir string) uint64 {
	data, err := os.ReadFile(filepath.Join(deviceDir, "mem_info_vram_total"))
	if err != nil {
		return 0
	}
	val, _ := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	return val / (1024 * 1024)
}

func readAMDGPUMemUsed(deviceDir string) uint64 {
	data, err := os.ReadFile(filepath.Join(deviceDir, "mem_info_vram_used"))
	if err != nil {
		return 0
	}
	val, _ := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	return val / (1024 * 1024)
}

func readAMDGPUBusyPercent(deviceDir string) int {
	data, err := os.ReadFile(filepath.Join(deviceDir, "gpu_busy_percent"))
	if err != nil {
		return 0
	}
	val, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	return val
}

func readAMDGPUTemperature(cardDir string) int {
	hwmonDirs, err := filepath.Glob(filepath.Join(cardDir, "device", "hwmon", "hwmon*"))
	if err != nil || len(hwmonDirs) == 0 {
		return 0
	}
	data, err := os.ReadFile(filepath.Join(hwmonDirs[0], "temp1_input"))
	if err != nil {
		return 0
	}
	val, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	return val / 1000
}
