package monitor

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// SystemMetrics is the JSON response for GET /api/metrics.
type SystemMetrics struct {
	CPUUsage float64       `json:"cpu_usage"`
	Memory   MemoryMetrics `json:"memory"`
	Disk     DiskMetrics   `json:"disk"`
	Uptime   float64       `json:"uptime"`
	Load     [3]float64    `json:"load"`
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

// collectSystemMetrics gathers CPU, memory, disk, uptime, and load data
// from the local /proc filesystem and syscall.Statfs.
func collectSystemMetrics() (*SystemMetrics, error) {
	cpuUsage, err := readCPUUsage()
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

	uptime, err := readUptime()
	if err != nil {
		return nil, fmt.Errorf("uptime: %w", err)
	}

	load, err := readLoadAvg()
	if err != nil {
		return nil, fmt.Errorf("load: %w", err)
	}

	return &SystemMetrics{
		CPUUsage: cpuUsage,
		Memory:   MemoryMetrics{Used: memUsed, Total: memTotal},
		Disk:     DiskMetrics{Used: diskUsed, Total: diskTotal},
		Uptime:   uptime,
		Load:     load,
	}, nil
}

// readCPUUsage samples /proc/stat twice (200ms apart) and computes the
// overall CPU usage percentage.
func readCPUUsage() (float64, error) {
	idle1, total1, err := readCPUSample()
	if err != nil {
		return 0, err
	}

	time.Sleep(200 * time.Millisecond)

	idle2, total2, err := readCPUSample()
	if err != nil {
		return 0, err
	}

	idleDelta := float64(idle2 - idle1)
	totalDelta := float64(total2 - total1)
	if totalDelta == 0 {
		return 0, nil
	}
	usage := (1.0 - idleDelta/totalDelta) * 100.0
	return usage, nil
}

// readCPUSample reads the aggregate "cpu" line from /proc/stat and returns
// (idle_ticks, total_ticks).
func readCPUSample() (uint64, uint64, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			return 0, 0, fmt.Errorf("unexpected /proc/stat cpu line: %s", line)
		}
		// fields: cpu user nice system idle iowait irq softirq steal guest guest_nice
		var total, idle uint64
		for i := 1; i < len(fields); i++ {
			v, err := strconv.ParseUint(fields[i], 10, 64)
			if err != nil {
				continue
			}
			total += v
			if i == 4 { // idle is the 4th value (index 4 in fields)
				idle = v
			}
		}
		return idle, total, nil
	}
	return 0, 0, fmt.Errorf("/proc/stat: no cpu line found")
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
