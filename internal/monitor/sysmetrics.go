package monitor

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// SystemMetrics is the JSON response for GET /api/metrics.
type SystemMetrics struct {
	CPUUsage  float64       `json:"cpu_usage"`
	CPUCores  []float64     `json:"cpu_cores,omitempty"`
	CPUFreqMHz float64      `json:"cpu_freq_mhz"`
	Memory    MemoryMetrics `json:"memory"`
	Swap      SwapMetrics   `json:"swap"`
	Disk      DiskMetrics   `json:"disk"`
	DiskIO    DiskIOMetrics `json:"disk_io"`
	Network   NetMetrics    `json:"network"`
	Power     PowerMetrics  `json:"power"`
	Temps     TempMetrics   `json:"temps"`
	Fans      FanMetrics    `json:"fans"`
	GPU       *GPUMetrics   `json:"gpu,omitempty"`
	Uptime    float64       `json:"uptime"`
	Load      [3]float64    `json:"load"`
	Hostname  string        `json:"hostname,omitempty"`
	OSVersion string        `json:"os_version,omitempty"`
	Kernel    string        `json:"kernel,omitempty"`
	Arch      string        `json:"arch,omitempty"`
	CPUModel  string        `json:"cpu_model,omitempty"`
	CPUCount  int           `json:"cpu_count,omitempty"`
}

// TempMetrics reports temperature readings in degrees Celsius.
type TempMetrics struct {
	CPU  float64 `json:"cpu"`
	GPU  float64 `json:"gpu,omitempty"`
	NVMe float64 `json:"nvme,omitempty"`
}

// FanMetrics reports fan speeds in RPM.
type FanMetrics struct {
	Fans []FanReading `json:"fans,omitempty"`
}

// FanReading is a single fan sensor reading.
type FanReading struct {
	Name string  `json:"name"`
	RPM  float64 `json:"rpm"`
}

// SwapMetrics reports swap usage in bytes.
type SwapMetrics struct {
	Used  uint64 `json:"used"`
	Total uint64 `json:"total"`
}

// GPUMetrics reports GPU utilization from Prometheus DCGM.
type GPUMetrics struct {
	Name        string  `json:"name"`
	Utilization int     `json:"utilization"`
	MemUsedMB   uint64  `json:"mem_used_mb"`
	MemTotalMB  uint64  `json:"mem_total_mb"`
	Temperature int     `json:"temperature"`
	PowerDraw   float64 `json:"power_draw"`
	PowerLimit  float64 `json:"power_limit"`
}

// MemoryMetrics reports used and total memory in bytes.
type MemoryMetrics struct {
	Used  uint64 `json:"used"`
	Total uint64 `json:"total"`
}

// DiskMetrics reports used and total disk in bytes.
type DiskMetrics struct {
	Used  uint64 `json:"used"`
	Total uint64 `json:"total"`
}

// DiskIOMetrics reports disk read/write bytes per second.
type DiskIOMetrics struct {
	ReadBytesPerSec  float64 `json:"read_bytes_per_sec"`
	WriteBytesPerSec float64 `json:"write_bytes_per_sec"`
}

// NetMetrics reports network bytes per second.
type NetMetrics struct {
	RxBytesPerSec float64 `json:"rx_bytes_per_sec"`
	TxBytesPerSec float64 `json:"tx_bytes_per_sec"`
}

// PowerMetrics reports power consumption in watts.
type PowerMetrics struct {
	CPUWatts   float64 `json:"cpu_watts"`
	GPUWatts   float64 `json:"gpu_watts"`
	TotalWatts float64 `json:"total_watts"`
}

// CollectSystemMetrics gathers CPU, memory, disk, network, power, uptime, and load
// from the local /proc filesystem and syscall.Statfs.
func CollectSystemMetrics(prometheusURL string) (*SystemMetrics, error) {
	cpuUsage, cpuCores, err := readCPUUsageWithCores()
	if err != nil {
		return nil, fmt.Errorf("cpu: %w", err)
	}

	memUsed, memTotal, err := readMemInfo()
	if err != nil {
		return nil, fmt.Errorf("memory: %w", err)
	}

	diskUsed, diskTotal, err := readDiskUsage("/")
	if err != nil {
		return nil, fmt.Errorf("disk: %w", err)
	}

	diskIO := readDiskIO()
	net := readNetworkIO()
	power := readPowerConsumption(prometheusURL)

	uptime, err := readUptime()
	if err != nil {
		return nil, fmt.Errorf("uptime: %w", err)
	}

	load, err := readLoadAvg()
	if err != nil {
		return nil, fmt.Errorf("load: %w", err)
	}

	sysInfo := readSystemInfo()

	return &SystemMetrics{
		CPUUsage:   cpuUsage,
		CPUCores:   cpuCores,
		CPUFreqMHz: readCPUFreq(),
		Memory:     MemoryMetrics{Used: memUsed, Total: memTotal},
		Swap:       readSwap(),
		Disk:       DiskMetrics{Used: diskUsed, Total: diskTotal},
		DiskIO:     diskIO,
		Network:    net,
		Power:      power,
		Temps:      readTemperatures(),
		Fans:       readFanSpeeds(),
		GPU:        readGPUMetrics(prometheusURL),
		Uptime:     uptime,
		Load:      load,
		Hostname:  sysInfo.hostname,
		OSVersion: sysInfo.osVersion,
		Kernel:    sysInfo.kernel,
		Arch:      sysInfo.arch,
		CPUModel:  sysInfo.cpuModel,
		CPUCount:  len(cpuCores),
	}, nil
}

// readCPUUsageWithCores samples /proc/stat twice (200ms apart) and returns
// overall CPU usage + per-core usage percentages.
func readCPUUsageWithCores() (float64, []float64, error) {
	samples1, err := readAllCPUSamples()
	if err != nil {
		return 0, nil, err
	}

	time.Sleep(200 * time.Millisecond)

	samples2, err := readAllCPUSamples()
	if err != nil {
		return 0, nil, err
	}

	// Overall CPU (first entry "cpu")
	var overall float64
	if len(samples1) > 0 && len(samples2) > 0 {
		idleDelta := float64(samples2[0].idle - samples1[0].idle)
		totalDelta := float64(samples2[0].total - samples1[0].total)
		if totalDelta > 0 {
			overall = (1.0 - idleDelta/totalDelta) * 100.0
		}
	}

	// Per-core (entries "cpu0", "cpu1", ...)
	var cores []float64
	for i := 1; i < len(samples1) && i < len(samples2); i++ {
		idleDelta := float64(samples2[i].idle - samples1[i].idle)
		totalDelta := float64(samples2[i].total - samples1[i].total)
		if totalDelta > 0 {
			cores = append(cores, (1.0-idleDelta/totalDelta)*100.0)
		} else {
			cores = append(cores, 0)
		}
	}

	return overall, cores, nil
}

type cpuSample struct {
	idle, total uint64
}

// readAllCPUSamples reads all "cpu" and "cpuN" lines from /proc/stat.
func readAllCPUSamples() ([]cpuSample, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var samples []cpuSample
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "cpu") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		var total, idle uint64
		for i := 1; i < len(fields); i++ {
			v, _ := strconv.ParseUint(fields[i], 10, 64)
			total += v
			if i == 4 {
				idle = v
			}
		}
		samples = append(samples, cpuSample{idle: idle, total: total})
	}
	if len(samples) == 0 {
		return nil, fmt.Errorf("/proc/stat: no cpu lines found")
	}
	return samples, nil
}

// readMemInfo reads /proc/meminfo and returns (used_bytes, total_bytes).
func readMemInfo() (uint64, uint64, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	var memTotal, memAvailable uint64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			memTotal = parseMemInfoValue(line)
		} else if strings.HasPrefix(line, "MemAvailable:") {
			memAvailable = parseMemInfoValue(line)
		}
	}
	if memTotal == 0 {
		return 0, 0, fmt.Errorf("/proc/meminfo: MemTotal not found")
	}
	used := memTotal - memAvailable
	return used, memTotal, nil
}

// parseMemInfoValue extracts the kB value from a /proc/meminfo line and converts to bytes.
func parseMemInfoValue(line string) uint64 {
	// Format: "MemTotal:       16384000 kB"
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	v, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0
	}
	return v * 1024 // kB to bytes
}

// readDiskUsage uses syscall.Statfs to report disk usage for the given path.
func readDiskUsage(path string) (uint64, uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}
	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
	used := total - free
	return used, total, nil
}

// readUptime reads /proc/uptime and returns the system uptime in seconds.
func readUptime() (float64, error) {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return 0, fmt.Errorf("/proc/uptime: empty")
	}
	uptime, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, err
	}
	return uptime, nil
}

// readLoadAvg reads /proc/loadavg and returns [load1, load5, load15].
func readLoadAvg() ([3]float64, error) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return [3]float64{}, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return [3]float64{}, fmt.Errorf("/proc/loadavg: not enough fields")
	}
	var load [3]float64
	for i := 0; i < 3; i++ {
		load[i], err = strconv.ParseFloat(fields[i], 64)
		if err != nil {
			return [3]float64{}, fmt.Errorf("/proc/loadavg field %d: %w", i, err)
		}
	}
	return load, nil
}

// --- Network I/O ---

var prevNetRx, prevNetTx uint64
var prevNetTime time.Time

// readNetworkIO reads /proc/net/dev and computes bytes/sec since last call.
func readNetworkIO() NetMetrics {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return NetMetrics{}
	}
	defer f.Close()

	var totalRx, totalTx uint64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		iface := strings.TrimSpace(parts[0])
		// Skip loopback and virtual interfaces (Calico, Docker, flannel, etc.)
		if iface == "lo" || strings.HasPrefix(iface, "cali") || strings.HasPrefix(iface, "veth") ||
			strings.HasPrefix(iface, "flannel") || strings.HasPrefix(iface, "vxlan") ||
			strings.HasPrefix(iface, "docker") || strings.HasPrefix(iface, "tunl") {
			continue
		}
		fields := strings.Fields(parts[1])
		if len(fields) < 10 {
			continue
		}
		rx, _ := strconv.ParseUint(fields[0], 10, 64)
		tx, _ := strconv.ParseUint(fields[8], 10, 64)
		totalRx += rx
		totalTx += tx
	}

	now := time.Now()
	result := NetMetrics{}
	if !prevNetTime.IsZero() {
		elapsed := now.Sub(prevNetTime).Seconds()
		if elapsed > 0 {
			result.RxBytesPerSec = float64(totalRx-prevNetRx) / elapsed
			result.TxBytesPerSec = float64(totalTx-prevNetTx) / elapsed
		}
	}
	prevNetRx = totalRx
	prevNetTx = totalTx
	prevNetTime = now
	return result
}

// --- Disk I/O ---

var prevDiskRead, prevDiskWrite uint64
var prevDiskTime time.Time

// readDiskIO reads /proc/diskstats and computes bytes/sec since last call.
func readDiskIO() DiskIOMetrics {
	f, err := os.Open("/proc/diskstats")
	if err != nil {
		return DiskIOMetrics{}
	}
	defer f.Close()

	var totalRead, totalWrite uint64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 14 {
			continue
		}
		name := fields[2]
		// Only count whole disks (sda, vda, nvme0n1), not partitions
		if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") {
			continue
		}
		// Skip partitions (sda1, vda2, nvme0n1p1)
		last := name[len(name)-1]
		if last >= '0' && last <= '9' && !strings.Contains(name, "nvme") {
			continue
		}
		if strings.Contains(name, "p") && strings.Contains(name, "nvme") {
			continue
		}
		// fields[5] = sectors read, fields[9] = sectors written (512 bytes each)
		r, _ := strconv.ParseUint(fields[5], 10, 64)
		w, _ := strconv.ParseUint(fields[9], 10, 64)
		totalRead += r * 512
		totalWrite += w * 512
	}

	now := time.Now()
	result := DiskIOMetrics{}
	if !prevDiskTime.IsZero() {
		elapsed := now.Sub(prevDiskTime).Seconds()
		if elapsed > 0 {
			result.ReadBytesPerSec = float64(totalRead-prevDiskRead) / elapsed
			result.WriteBytesPerSec = float64(totalWrite-prevDiskWrite) / elapsed
		}
	}
	prevDiskRead = totalRead
	prevDiskWrite = totalWrite
	prevDiskTime = now
	return result
}

// --- Power Consumption ---

// readPowerConsumption reads CPU power from Intel RAPL and GPU power from Prometheus DCGM.
func readPowerConsumption(prometheusURL string) PowerMetrics {
	var p PowerMetrics

	// CPU power from Intel RAPL
	data, err := os.ReadFile("/sys/class/powercap/intel-rapl:0/energy_uj")
	if err == nil {
		uj, _ := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
		p.CPUWatts = raplToWatts(uj)
	}

	// GPU power from Prometheus DCGM metrics
	if prometheusURL != "" {
		url := fmt.Sprintf("%s/api/v1/query?query=DCGM_FI_DEV_POWER_USAGE", prometheusURL)
		client := &http.Client{Timeout: 3 * time.Second}
		resp, err := client.Get(url)
		if err == nil {
			defer resp.Body.Close()
			var pr struct {
				Status string `json:"status"`
				Data   struct {
					Result []struct {
						Value []interface{} `json:"value"`
					} `json:"result"`
				} `json:"data"`
			}
			if json.NewDecoder(resp.Body).Decode(&pr) == nil && pr.Status == "success" {
				for _, r := range pr.Data.Result {
					if len(r.Value) >= 2 {
						w, _ := strconv.ParseFloat(fmt.Sprint(r.Value[1]), 64)
						p.GPUWatts += w
					}
				}
			}
		}
	}

	p.TotalWatts = p.CPUWatts + p.GPUWatts
	// Add base system power estimate if we have any readings
	if p.CPUWatts > 0 || p.GPUWatts > 0 {
		p.TotalWatts += 10 // ~10W base (motherboard, fans, SSD)
	}

	return p
}

var prevRAPLEnergy uint64
var prevRAPLTime time.Time

func raplToWatts(energyUJ uint64) float64 {
	now := time.Now()
	var watts float64
	if !prevRAPLTime.IsZero() && energyUJ > prevRAPLEnergy {
		elapsed := now.Sub(prevRAPLTime).Seconds()
		if elapsed > 0 {
			deltaJ := float64(energyUJ-prevRAPLEnergy) / 1_000_000.0
			watts = deltaJ / elapsed
		}
	}
	prevRAPLEnergy = energyUJ
	prevRAPLTime = now
	return watts
}

// --- Temperature ---

func readTemperatures() TempMetrics {
	var t TempMetrics

	// CPU temperature from thermal zones
	// Look for zones with type containing "cpu", "x86_pkg", "coretemp", or "acpitz"
	matches, _ := filepath.Glob("/sys/class/thermal/thermal_zone*/temp")
	for _, tempFile := range matches {
		zoneDir := filepath.Dir(tempFile)
		typeData, err := os.ReadFile(filepath.Join(zoneDir, "type"))
		if err != nil {
			continue
		}
		zoneType := strings.TrimSpace(strings.ToLower(string(typeData)))

		data, err := os.ReadFile(tempFile)
		if err != nil {
			continue
		}
		val, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
		if err != nil {
			continue
		}
		tempC := val / 1000.0

		if strings.Contains(zoneType, "x86_pkg") ||
			strings.Contains(zoneType, "coretemp") ||
			strings.Contains(zoneType, "cpu") {
			if tempC > t.CPU {
				t.CPU = tempC
			}
		}
	}

	// Fallback: hwmon for CPU temp
	if t.CPU == 0 {
		hwmonMatches, _ := filepath.Glob("/sys/class/hwmon/hwmon*/temp1_input")
		for _, f := range hwmonMatches {
			nameFile := filepath.Join(filepath.Dir(f), "name")
			nameData, _ := os.ReadFile(nameFile)
			name := strings.TrimSpace(strings.ToLower(string(nameData)))
			if strings.Contains(name, "coretemp") || strings.Contains(name, "k10temp") || strings.Contains(name, "cpu") {
				data, err := os.ReadFile(f)
				if err == nil {
					val, _ := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
					t.CPU = val / 1000.0
				}
				break
			}
		}
	}

	// NVMe temperature from hwmon
	hwmonMatches, _ := filepath.Glob("/sys/class/hwmon/hwmon*/temp1_input")
	for _, f := range hwmonMatches {
		nameFile := filepath.Join(filepath.Dir(f), "name")
		nameData, _ := os.ReadFile(nameFile)
		name := strings.TrimSpace(strings.ToLower(string(nameData)))
		if strings.Contains(name, "nvme") {
			data, err := os.ReadFile(f)
			if err == nil {
				val, _ := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
				t.NVMe = val / 1000.0
			}
			break
		}
	}

	// GPU temperature comes from DCGM (already in gpu.go), skip here

	return t
}

// --- GPU Metrics (from Prometheus DCGM) ---

func readGPUMetrics(prometheusURL string) *GPUMetrics {
	if prometheusURL == "" {
		return nil
	}

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
		client := &http.Client{Timeout: 3 * time.Second}
		r, err := client.Get(url)
		if err != nil {
			return nil
		}
		defer r.Body.Close()
		var pr promResponse
		json.NewDecoder(r.Body).Decode(&pr)
		if pr.Status == "success" && len(pr.Data.Result) > 0 {
			return &pr
		}
		return nil
	}

	getVal := func(pr *promResponse) float64 {
		if pr == nil || len(pr.Data.Result) == 0 || len(pr.Data.Result[0].Value) < 2 {
			return 0
		}
		v, _ := strconv.ParseFloat(fmt.Sprint(pr.Data.Result[0].Value[1]), 64)
		return v
	}

	utilResult := query("DCGM_FI_DEV_GPU_UTIL")
	if utilResult == nil {
		return nil
	}

	name := ""
	if len(utilResult.Data.Result) > 0 {
		name = utilResult.Data.Result[0].Metric["modelName"]
	}

	memUsed := getVal(query("DCGM_FI_DEV_FB_USED"))
	memFree := getVal(query("DCGM_FI_DEV_FB_FREE"))
	temp := getVal(query("DCGM_FI_DEV_GPU_TEMP"))
	power := getVal(query("DCGM_FI_DEV_POWER_USAGE"))
	powerLimit := getVal(query("DCGM_FI_DEV_ENFORCED_POWER_LIMIT"))

	return &GPUMetrics{
		Name:        name,
		Utilization: int(getVal(utilResult)),
		MemUsedMB:   uint64(memUsed),
		MemTotalMB:  uint64(memUsed + memFree),
		Temperature: int(temp),
		PowerDraw:   power,
		PowerLimit:  powerLimit,
	}
}

// --- CPU Frequency ---

func readCPUFreq() float64 {
	// Average current frequency across all cores
	matches, _ := filepath.Glob("/sys/devices/system/cpu/cpu*/cpufreq/scaling_cur_freq")
	if len(matches) == 0 {
		return 0
	}
	var total float64
	var count int
	for _, f := range matches {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		khz, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
		if err != nil {
			continue
		}
		total += khz / 1000.0 // KHz to MHz
		count++
	}
	if count == 0 {
		return 0
	}
	return total / float64(count)
}

// --- Swap ---

func readSwap() SwapMetrics {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return SwapMetrics{}
	}
	var total, free uint64
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		val, _ := strconv.ParseUint(fields[1], 10, 64)
		val *= 1024 // kB to bytes
		switch fields[0] {
		case "SwapTotal:":
			total = val
		case "SwapFree:":
			free = val
		}
	}
	return SwapMetrics{Used: total - free, Total: total}
}

// --- Fan Speeds ---

func readFanSpeeds() FanMetrics {
	var fans []FanReading
	hwmonDirs, _ := filepath.Glob("/sys/class/hwmon/hwmon*")
	for _, dir := range hwmonDirs {
		nameData, _ := os.ReadFile(filepath.Join(dir, "name"))
		sensorName := strings.TrimSpace(string(nameData))

		fanFiles, _ := filepath.Glob(filepath.Join(dir, "fan*_input"))
		for _, f := range fanFiles {
			data, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			rpm, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
			if err != nil || rpm == 0 {
				continue
			}
			// Build label from sensor name + fan index
			base := filepath.Base(f)
			idx := strings.TrimSuffix(strings.TrimPrefix(base, "fan"), "_input")
			label := sensorName
			if label == "" {
				label = "fan"
			}
			label += "_" + idx
			fans = append(fans, FanReading{Name: label, RPM: rpm})
		}
	}
	return FanMetrics{Fans: fans}
}

func execCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	return string(out), err
}

// --- System Info (cached, read once) ---

type sysInfoCache struct {
	hostname  string
	osVersion string
	kernel    string
	arch      string
	cpuModel  string
}

var cachedSysInfo *sysInfoCache

func readSystemInfo() sysInfoCache {
	if cachedSysInfo != nil {
		return *cachedSysInfo
	}

	info := sysInfoCache{}

	// Hostname — prefer NODE_NAME env (from downward API) over container hostname
	info.hostname = os.Getenv("NODE_NAME")
	if info.hostname == "" {
		if data, err := os.ReadFile("/proc/sys/kernel/hostname"); err == nil {
			info.hostname = strings.TrimSpace(string(data))
		}
	}

	// OS version from /etc/os-release
	if f, err := os.Open("/etc/os-release"); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				info.osVersion = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
			}
		}
	}

	// Kernel version
	if out, err := execCommand("uname", "-r"); err == nil {
		info.kernel = strings.TrimSpace(out)
	}

	// Architecture
	if out, err := execCommand("uname", "-m"); err == nil {
		info.arch = strings.TrimSpace(out)
	}

	// CPU model from /proc/cpuinfo
	if f, err := os.Open("/proc/cpuinfo"); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "model name") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					info.cpuModel = strings.TrimSpace(parts[1])
					break
				}
			}
		}
	}

	cachedSysInfo = &info
	return info
}

// --- /api/system/info — static system information + devices ---

// DeviceInfo describes a detected hardware device.
type DeviceInfo struct {
	Type   string `json:"type"`   // gpu, npu, wifi, ethernet, nvme, thunderbolt, audio, bluetooth
	Name   string `json:"name"`
	Driver string `json:"driver,omitempty"`
}

// SystemInfo is the JSON response for GET /api/system/info. Loaded once.
type SystemInfo struct {
	Hostname    string       `json:"hostname"`
	OSVersion   string       `json:"os_version"`
	Kernel      string       `json:"kernel"`
	Arch        string       `json:"arch"`
	CPUModel    string       `json:"cpu_model"`
	CPUCount    int          `json:"cpu_count"`
	MemoryTotal uint64       `json:"memory_total"`
	DiskTotal   uint64       `json:"disk_total"`
	Devices     []DeviceInfo `json:"devices"`
}

var cachedSystemInfo *SystemInfo

// CollectSystemInfo returns static system info (cached after first call).
func CollectSystemInfo() *SystemInfo {
	if cachedSystemInfo != nil {
		return cachedSystemInfo
	}

	si := readSystemInfo()
	_, memTotal, _ := readMemInfo()
	_, diskTotal, _ := readDiskUsage("/")

	info := &SystemInfo{
		Hostname:    si.hostname,
		OSVersion:   si.osVersion,
		Kernel:      si.kernel,
		Arch:        si.arch,
		CPUModel:    si.cpuModel,
		CPUCount:    runtime.NumCPU(),
		MemoryTotal: memTotal,
		DiskTotal:   diskTotal,
		Devices:     detectDevices(),
	}

	cachedSystemInfo = info
	return info
}

// detectDevices parses lspci -k to find hardware devices and their drivers.
func detectDevices() []DeviceInfo {
	out, err := execCommand("lspci", "-k")
	if err != nil {
		return nil
	}

	var devices []DeviceInfo
	lines := strings.Split(out, "\n")

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if line == "" || line[0] == '\t' || line[0] == ' ' {
			continue
		}

		lower := strings.ToLower(line)
		parts := strings.SplitN(line, ": ", 3)
		if len(parts) < 3 {
			continue
		}
		name := strings.TrimSpace(parts[2])

		// Find driver on next lines
		driver := ""
		for j := i + 1; j < len(lines) && (strings.HasPrefix(lines[j], "\t") || strings.HasPrefix(lines[j], " ")); j++ {
			if strings.Contains(lines[j], "Kernel driver in use:") {
				dParts := strings.SplitN(lines[j], ":", 2)
				if len(dParts) == 2 {
					driver = strings.TrimSpace(dParts[1])
				}
			}
		}

		var devType string
		switch {
		case strings.Contains(lower, "nvidia") && (strings.Contains(lower, "vga") || strings.Contains(lower, "3d controller")):
			devType = "gpu"
		case strings.Contains(lower, "vga") && !strings.Contains(lower, "nvidia"):
			devType = "igpu"
		case strings.Contains(lower, "npu") || strings.Contains(lower, "processing accelerator"):
			devType = "npu"
		case strings.Contains(lower, "wi-fi") || strings.Contains(lower, "wireless") || strings.Contains(lower, "network controller"):
			devType = "wifi"
		case strings.Contains(lower, "ethernet"):
			devType = "ethernet"
		case strings.Contains(lower, "non-volatile memory") || strings.Contains(lower, "nvme"):
			devType = "nvme"
		case strings.Contains(lower, "thunderbolt") && strings.Contains(lower, "usb controller"):
			devType = "thunderbolt"
		case strings.Contains(lower, "audio"):
			devType = "audio"
		default:
			continue
		}

		devices = append(devices, DeviceInfo{
			Type:   devType,
			Name:   name,
			Driver: driver,
		})
	}

	return devices
}
