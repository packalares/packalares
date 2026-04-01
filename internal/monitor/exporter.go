package monitor

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
)

// latestMetrics caches the most recent metrics for the Prometheus exporter.
// Updated by CollectSystemMetrics callers (the metrics publisher goroutine).
var (
	latestMetricsMu sync.RWMutex
	latestMetrics   *SystemMetrics
)

// UpdateLatestMetrics stores the most recent metrics snapshot for the exporter.
func UpdateLatestMetrics(m *SystemMetrics) {
	latestMetricsMu.Lock()
	latestMetrics = m
	latestMetricsMu.Unlock()
}

// HandlePrometheusMetrics serves metrics in Prometheus text exposition format.
func HandlePrometheusMetrics(w http.ResponseWriter, r *http.Request) {
	latestMetricsMu.RLock()
	m := latestMetrics
	latestMetricsMu.RUnlock()

	if m == nil {
		http.Error(w, "no metrics available", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	var sb strings.Builder

	// CPU
	gauge(&sb, "packalares_cpu_usage_percent", "CPU usage percentage", m.CPUUsage)
	gauge(&sb, "packalares_cpu_freq_mhz", "Average CPU frequency in MHz", m.CPUFreqMHz)
	for i, core := range m.CPUCores {
		sb.WriteString(fmt.Sprintf("packalares_cpu_core_usage_percent{core=\"%d\"} %g\n", i, core))
	}

	// Memory
	gauge(&sb, "packalares_memory_used_bytes", "Memory used in bytes", float64(m.Memory.Used))
	gauge(&sb, "packalares_memory_total_bytes", "Memory total in bytes", float64(m.Memory.Total))

	// Swap
	gauge(&sb, "packalares_swap_used_bytes", "Swap used in bytes", float64(m.Swap.Used))
	gauge(&sb, "packalares_swap_total_bytes", "Swap total in bytes", float64(m.Swap.Total))

	// Disk
	gauge(&sb, "packalares_disk_used_bytes", "Disk used in bytes", float64(m.Disk.Used))
	gauge(&sb, "packalares_disk_total_bytes", "Disk total in bytes", float64(m.Disk.Total))

	// Disk I/O
	gauge(&sb, "packalares_disk_read_bytes_per_sec", "Disk read bytes per second", m.DiskIO.ReadBytesPerSec)
	gauge(&sb, "packalares_disk_write_bytes_per_sec", "Disk write bytes per second", m.DiskIO.WriteBytesPerSec)

	// Network I/O
	gauge(&sb, "packalares_network_rx_bytes_per_sec", "Network receive bytes per second", m.Network.RxBytesPerSec)
	gauge(&sb, "packalares_network_tx_bytes_per_sec", "Network transmit bytes per second", m.Network.TxBytesPerSec)

	// Power
	gauge(&sb, "packalares_power_cpu_watts", "CPU power consumption in watts", m.Power.CPUWatts)
	gauge(&sb, "packalares_power_gpu_watts", "GPU power consumption in watts", m.Power.GPUWatts)
	gauge(&sb, "packalares_power_total_watts", "Total power consumption in watts", m.Power.TotalWatts)

	// Temperature
	gauge(&sb, "packalares_temp_cpu_celsius", "CPU temperature in Celsius", m.Temps.CPU)
	if m.Temps.GPU > 0 {
		gauge(&sb, "packalares_temp_gpu_celsius", "GPU temperature in Celsius", m.Temps.GPU)
	}
	if m.Temps.NVMe > 0 {
		gauge(&sb, "packalares_temp_nvme_celsius", "NVMe temperature in Celsius", m.Temps.NVMe)
	}

	// Fans
	for _, f := range m.Fans.Fans {
		sb.WriteString(fmt.Sprintf("packalares_fan_rpm{name=\"%s\"} %g\n", f.Name, f.RPM))
	}

	// System
	gauge(&sb, "packalares_uptime_seconds", "System uptime in seconds", m.Uptime)
	gauge(&sb, "packalares_load_1m", "1-minute load average", m.Load[0])
	gauge(&sb, "packalares_load_5m", "5-minute load average", m.Load[1])
	gauge(&sb, "packalares_load_15m", "15-minute load average", m.Load[2])

	w.Write([]byte(sb.String()))
}

func gauge(sb *strings.Builder, name, help string, value float64) {
	sb.WriteString(fmt.Sprintf("# HELP %s %s\n# TYPE %s gauge\n%s %g\n", name, help, name, name, value))
}
