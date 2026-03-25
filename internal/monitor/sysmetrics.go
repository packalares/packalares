package monitor

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// SystemMetrics is the JSON response for GET /api/metrics.
type SystemMetrics struct {
	CPUUsage float64       `json:"cpu_usage"`
	CPUCores []float64     `json:"cpu_cores,omitempty"`
	Memory   MemoryMetrics `json:"memory"`
	Disk     DiskMetrics   `json:"disk"`
	DiskIO   DiskIOMetrics `json:"disk_io"`
	Network  NetMetrics    `json:"network"`
	Power    PowerMetrics  `json:"power"`
	Uptime   float64       `json:"uptime"`
	Load     [3]float64    `json:"load"`
	Hostname string        `json:"hostname,omitempty"`
	OSVersion string       `json:"os_version,omitempty"`
	Kernel   string        `json:"kernel,omitempty"`
	Arch     string        `json:"arch,omitempty"`
	CPUModel string        `json:"cpu_model,omitempty"`
	CPUCount int           `json:"cpu_count,omitempty"`
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
func CollectSystemMetrics() (*SystemMetrics, error) {
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
	power := readPowerConsumption()

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
		CPUUsage:  cpuUsage,
		CPUCores:  cpuCores,
		Memory:    MemoryMetrics{Used: memUsed, Total: memTotal},
		Disk:      DiskMetrics{Used: diskUsed, Total: diskTotal},
		DiskIO:    diskIO,
		Network:   net,
		Power:     power,
		Uptime:    uptime,
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
		if iface == "lo" {
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

// readPowerConsumption reads CPU power from Intel RAPL and GPU power from nvidia-smi.
func readPowerConsumption() PowerMetrics {
	var p PowerMetrics

	// CPU power from Intel RAPL
	data, err := os.ReadFile("/sys/class/powercap/intel-rapl:0/energy_uj")
	if err == nil {
		uj, _ := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
		p.CPUWatts = raplToWatts(uj)
	}

	// GPU power from nvidia-smi
	out, err := execCommand("nvidia-smi", "--query-gpu=power.draw", "--format=csv,noheader,nounits")
	if err == nil {
		watts, _ := strconv.ParseFloat(strings.TrimSpace(out), 64)
		p.GPUWatts = watts
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

	// Hostname
	if data, err := os.ReadFile("/proc/sys/kernel/hostname"); err == nil {
		info.hostname = strings.TrimSpace(string(data))
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
