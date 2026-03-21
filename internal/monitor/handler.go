package monitor

import (
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Handler serves monitoring metrics to the frontend.
type Handler struct {
	prom *PrometheusClient
}

// NewHandler creates a monitoring handler connected to Prometheus.
func NewHandler(prometheusURL string) *Handler {
	return &Handler{
		prom: NewPrometheusClient(prometheusURL),
	}
}

// RegisterRoutes wires up monitoring API routes.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Packalares native endpoints
	mux.HandleFunc("/api/monitoring/cluster", h.handleCluster)
	mux.HandleFunc("/api/monitoring/nodes", h.handleNodes)
	mux.HandleFunc("/api/monitoring/pods", h.handlePods)
	mux.HandleFunc("/api/monitoring/gpu", h.handleGPU)

	// KubeSphere-compatible endpoints
	mux.HandleFunc("/kapis/monitoring.kubesphere.io/v1alpha3/cluster", h.handleKSCluster)
	mux.HandleFunc("/kapis/monitoring.kubesphere.io/v1alpha3/nodes", h.handleKSNodes)
	mux.HandleFunc("/kapis/monitoring.kubesphere.io/v1alpha3/nodes/", h.handleKSNodeByName)
	mux.HandleFunc("/kapis/monitoring.kubesphere.io/v1alpha3/namespaces/", h.handleKSNamespace)
	mux.HandleFunc("/kapis/monitoring.kubesphere.io/v1alpha3/pods", h.handleKSPods)
	mux.HandleFunc("/kapis/monitoring.kubesphere.io/v1alpha3/components/", h.handleKSComponents)
}

// ---------------------------------------------------------------------------
// Packalares native monitoring endpoints
// ---------------------------------------------------------------------------

func (h *Handler) handleCluster(w http.ResponseWriter, r *http.Request) {
	params := parseMonitoringParams(r)

	metrics := []namedQuery{
		{Name: "cluster_cpu_utilisation", Query: `1 - avg(rate(node_cpu_seconds_total{mode="idle"}[5m]))`},
		{Name: "cluster_cpu_usage", Query: `sum(rate(node_cpu_seconds_total{mode!="idle"}[5m]))`},
		{Name: "cluster_cpu_total", Query: `sum(machine_cpu_cores)`},
		{Name: "cluster_memory_utilisation", Query: `1 - sum(node_memory_MemAvailable_bytes) / sum(node_memory_MemTotal_bytes)`},
		{Name: "cluster_memory_available", Query: `sum(node_memory_MemAvailable_bytes)`},
		{Name: "cluster_memory_total", Query: `sum(node_memory_MemTotal_bytes)`},
		{Name: "cluster_memory_usage_wo_cache", Query: `sum(node_memory_MemTotal_bytes - node_memory_MemAvailable_bytes)`},
		{Name: "cluster_net_bytes_transmitted", Query: `sum(rate(node_network_transmit_bytes_total{device!~"lo|docker.*|veth.*|cali.*|flannel.*"}[5m]))`},
		{Name: "cluster_net_bytes_received", Query: `sum(rate(node_network_receive_bytes_total{device!~"lo|docker.*|veth.*|cali.*|flannel.*"}[5m]))`},
		{Name: "cluster_disk_size_usage", Query: `sum(node_filesystem_size_bytes{mountpoint="/"}) - sum(node_filesystem_avail_bytes{mountpoint="/"})`},
		{Name: "cluster_disk_size_capacity", Query: `sum(node_filesystem_size_bytes{mountpoint="/"})`},
		{Name: "cluster_disk_size_available", Query: `sum(node_filesystem_avail_bytes{mountpoint="/"})`},
		{Name: "cluster_disk_size_utilisation", Query: `1 - sum(node_filesystem_avail_bytes{mountpoint="/"}) / sum(node_filesystem_size_bytes{mountpoint="/"})`},
		{Name: "cluster_disk_read_throughput", Query: `sum(rate(node_disk_read_bytes_total[5m]))`},
		{Name: "cluster_disk_write_throughput", Query: `sum(rate(node_disk_written_bytes_total[5m]))`},
		{Name: "cluster_disk_read_iops", Query: `sum(rate(node_disk_reads_completed_total[5m]))`},
		{Name: "cluster_disk_write_iops", Query: `sum(rate(node_disk_writes_completed_total[5m]))`},
		{Name: "cluster_pod_count", Query: `count(kube_pod_info)`},
		{Name: "cluster_pod_running_count", Query: `count(kube_pod_status_phase{phase="Running"} == 1)`},
		{Name: "cluster_node_total", Query: `count(kube_node_info)`},
		{Name: "cluster_node_online", Query: `count(kube_node_status_condition{condition="Ready",status="true"} == 1)`},
		{Name: "cluster_load1", Query: `avg(node_load1)`},
		{Name: "cluster_load5", Query: `avg(node_load5)`},
		{Name: "cluster_load15", Query: `avg(node_load15)`},
	}

	result := h.queryMetrics(metrics, params)
	writeJSON(w, result)
}

func (h *Handler) handleNodes(w http.ResponseWriter, r *http.Request) {
	params := parseMonitoringParams(r)

	metrics := []namedQuery{
		{Name: "node_cpu_utilisation", Query: `1 - avg by(node) (rate(node_cpu_seconds_total{mode="idle"}[5m]))`},
		{Name: "node_cpu_usage", Query: `sum by(node) (rate(node_cpu_seconds_total{mode!="idle"}[5m]))`},
		{Name: "node_cpu_total", Query: `sum by(node) (machine_cpu_cores)`},
		{Name: "node_memory_utilisation", Query: `1 - sum by(node) (node_memory_MemAvailable_bytes) / sum by(node) (node_memory_MemTotal_bytes)`},
		{Name: "node_memory_usage_wo_cache", Query: `sum by(node) (node_memory_MemTotal_bytes - node_memory_MemAvailable_bytes)`},
		{Name: "node_memory_available", Query: `sum by(node) (node_memory_MemAvailable_bytes)`},
		{Name: "node_memory_total", Query: `sum by(node) (node_memory_MemTotal_bytes)`},
		{Name: "node_net_bytes_transmitted", Query: `sum by(node) (rate(node_network_transmit_bytes_total{device!~"lo|docker.*|veth.*|cali.*|flannel.*"}[5m]))`},
		{Name: "node_net_bytes_received", Query: `sum by(node) (rate(node_network_receive_bytes_total{device!~"lo|docker.*|veth.*|cali.*|flannel.*"}[5m]))`},
		{Name: "node_disk_size_capacity", Query: `sum by(node) (node_filesystem_size_bytes{mountpoint="/"})`},
		{Name: "node_disk_size_available", Query: `sum by(node) (node_filesystem_avail_bytes{mountpoint="/"})`},
		{Name: "node_disk_size_usage", Query: `sum by(node) (node_filesystem_size_bytes{mountpoint="/"}) - sum by(node) (node_filesystem_avail_bytes{mountpoint="/"})`},
		{Name: "node_disk_size_utilisation", Query: `1 - sum by(node) (node_filesystem_avail_bytes{mountpoint="/"}) / sum by(node) (node_filesystem_size_bytes{mountpoint="/"})`},
		{Name: "node_load1", Query: `avg by(node) (node_load1)`},
		{Name: "node_load5", Query: `avg by(node) (node_load5)`},
		{Name: "node_load15", Query: `avg by(node) (node_load15)`},
		{Name: "node_pod_count", Query: `count by(node) (kube_pod_info)`},
	}

	result := h.queryMetrics(metrics, params)
	writeJSON(w, result)
}

func (h *Handler) handlePods(w http.ResponseWriter, r *http.Request) {
	params := parseMonitoringParams(r)

	namespace := r.URL.Query().Get("namespace")
	podFilter := r.URL.Query().Get("pod")

	nsFilter := ""
	if namespace != "" {
		nsFilter = `namespace="` + namespace + `"`
	}
	podLabel := ""
	if podFilter != "" {
		podLabel = `pod=~"` + podFilter + `"`
	}
	labels := joinLabels(nsFilter, podLabel)

	metrics := []namedQuery{
		{Name: "pod_cpu_usage", Query: `sum by(namespace, pod) (rate(container_cpu_usage_seconds_total{` + labels + `container!=""}[5m]))`},
		{Name: "pod_memory_usage", Query: `sum by(namespace, pod) (container_memory_usage_bytes{` + labels + `container!=""})`},
		{Name: "pod_memory_usage_wo_cache", Query: `sum by(namespace, pod) (container_memory_working_set_bytes{` + labels + `container!=""})`},
		{Name: "pod_net_bytes_transmitted", Query: `sum by(namespace, pod) (rate(container_network_transmit_bytes_total{` + labels + `}[5m]))`},
		{Name: "pod_net_bytes_received", Query: `sum by(namespace, pod) (rate(container_network_receive_bytes_total{` + labels + `}[5m]))`},
	}

	result := h.queryMetrics(metrics, params)
	writeJSON(w, result)
}

func (h *Handler) handleGPU(w http.ResponseWriter, r *http.Request) {
	params := parseMonitoringParams(r)

	metrics := []namedQuery{
		{Name: "gpu_utilization", Query: `avg(DCGM_FI_DEV_GPU_UTIL)`},
		{Name: "gpu_memory_utilization", Query: `avg(DCGM_FI_DEV_FB_USED) / avg(DCGM_FI_DEV_FB_FREE + DCGM_FI_DEV_FB_USED) * 100`},
		{Name: "gpu_memory_used", Query: `sum(DCGM_FI_DEV_FB_USED) * 1048576`},
		{Name: "gpu_memory_total", Query: `sum(DCGM_FI_DEV_FB_FREE + DCGM_FI_DEV_FB_USED) * 1048576`},
		{Name: "gpu_temperature", Query: `avg(DCGM_FI_DEV_GPU_TEMP)`},
		{Name: "gpu_power_usage", Query: `sum(DCGM_FI_DEV_POWER_USAGE)`},
		{Name: "gpu_count", Query: `count(DCGM_FI_DEV_GPU_UTIL)`},
	}

	result := h.queryMetrics(metrics, params)
	writeJSON(w, result)
}

// ---------------------------------------------------------------------------
// KubeSphere-compatible monitoring endpoints
// ---------------------------------------------------------------------------

func (h *Handler) handleKSCluster(w http.ResponseWriter, r *http.Request) {
	params := parseMonitoringParams(r)
	metricsFilter := params.metricsFilter
	if metricsFilter == "" {
		metricsFilter = ".*"
	}

	re, err := regexp.Compile(metricsFilter)
	if err != nil {
		re = regexp.MustCompile(".*")
	}

	metrics := clusterQueries()
	filtered := filterMetrics(metrics, re)
	result := h.queryMetrics(filtered, params)
	writeJSON(w, result)
}

func (h *Handler) handleKSNodes(w http.ResponseWriter, r *http.Request) {
	params := parseMonitoringParams(r)
	metricsFilter := params.metricsFilter
	if metricsFilter == "" {
		metricsFilter = ".*"
	}

	re, err := regexp.Compile(metricsFilter)
	if err != nil {
		re = regexp.MustCompile(".*")
	}

	metrics := nodeQueries()
	filtered := filterMetrics(metrics, re)
	result := h.queryMetrics(filtered, params)
	writeJSON(w, result)
}

func (h *Handler) handleKSNodeByName(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/kapis/monitoring.kubesphere.io/v1alpha3/nodes/")
	parts := strings.SplitN(path, "/", 2)
	nodeName := parts[0]
	_ = nodeName

	params := parseMonitoringParams(r)
	metricsFilter := params.metricsFilter
	if metricsFilter == "" {
		metricsFilter = ".*"
	}

	re, err := regexp.Compile(metricsFilter)
	if err != nil {
		re = regexp.MustCompile(".*")
	}

	metrics := nodeQueries()
	filtered := filterMetrics(metrics, re)
	result := h.queryMetrics(filtered, params)
	writeJSON(w, result)
}

func (h *Handler) handleKSNamespace(w http.ResponseWriter, r *http.Request) {
	params := parseMonitoringParams(r)
	metricsFilter := params.metricsFilter
	if metricsFilter == "" {
		metricsFilter = ".*"
	}

	re, err := regexp.Compile(metricsFilter)
	if err != nil {
		re = regexp.MustCompile(".*")
	}

	metrics := namespaceQueries()
	filtered := filterMetrics(metrics, re)
	result := h.queryMetrics(filtered, params)
	writeJSON(w, result)
}

func (h *Handler) handleKSPods(w http.ResponseWriter, r *http.Request) {
	h.handlePods(w, r)
}

func (h *Handler) handleKSComponents(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/kapis/monitoring.kubesphere.io/v1alpha3/components/")
	component := strings.TrimSuffix(path, "/")

	params := parseMonitoringParams(r)

	var metrics []namedQuery
	switch component {
	case "etcd":
		metrics = etcdQueries()
	case "apiserver":
		metrics = apiserverQueries()
	case "scheduler":
		metrics = schedulerQueries()
	default:
		writeJSON(w, KSMetricsResponse{Results: []KSMetric{}})
		return
	}

	result := h.queryMetrics(metrics, params)
	writeJSON(w, result)
}

// ---------------------------------------------------------------------------
// Query helpers
// ---------------------------------------------------------------------------

type monitoringParams struct {
	start         time.Time
	end           time.Time
	step          time.Duration
	time_         time.Time
	metricsFilter string
	isRange       bool
}

func parseMonitoringParams(r *http.Request) monitoringParams {
	p := monitoringParams{}
	q := r.URL.Query()

	p.metricsFilter = q.Get("metrics_filter")

	startStr := q.Get("start")
	endStr := q.Get("end")
	stepStr := q.Get("step")
	timeStr := q.Get("time")

	if startStr != "" && endStr != "" {
		p.isRange = true
		startInt, _ := strconv.ParseInt(startStr, 10, 64)
		p.start = time.Unix(startInt, 0)
		endInt, _ := strconv.ParseInt(endStr, 10, 64)
		p.end = time.Unix(endInt, 0)
		if stepStr != "" {
			p.step, _ = time.ParseDuration(stepStr)
		}
		if p.step == 0 {
			p.step = 10 * time.Minute
		}
	} else {
		p.isRange = false
		if timeStr != "" {
			timeInt, _ := strconv.ParseInt(timeStr, 10, 64)
			p.time_ = time.Unix(timeInt, 0)
		} else {
			p.time_ = time.Now()
		}
	}

	return p
}

type namedQuery struct {
	Name  string
	Query string
}

func (h *Handler) queryMetrics(metrics []namedQuery, params monitoringParams) KSMetricsResponse {
	var results []KSMetric

	for _, m := range metrics {
		var metric KSMetric
		metric.MetricName = m.Name

		var err error
		if params.isRange {
			metric.MetricData, err = h.prom.QueryRange(m.Query, params.start, params.end, params.step)
		} else {
			metric.MetricData, err = h.prom.Query(m.Query, params.time_)
		}

		if err != nil {
			log.Printf("prometheus query error for %s: %v", m.Name, err)
			metric.Error = err.Error()
		}

		results = append(results, metric)
	}

	return KSMetricsResponse{Results: results}
}

func filterMetrics(metrics []namedQuery, re *regexp.Regexp) []namedQuery {
	var filtered []namedQuery
	for _, m := range metrics {
		if re.MatchString(m.Name) {
			filtered = append(filtered, m)
		}
	}
	return filtered
}

func joinLabels(labels ...string) string {
	var parts []string
	for _, l := range labels {
		if l != "" {
			parts = append(parts, l)
		}
	}
	return strings.Join(parts, ",")
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
