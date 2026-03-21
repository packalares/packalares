package monitor

// clusterQueries returns PromQL queries for cluster-level metrics
// matching the KubeSphere ClusterMetrics naming convention.
func clusterQueries() []namedQuery {
	return []namedQuery{
		{Name: "cluster_cpu_utilisation", Query: `1 - avg(rate(node_cpu_seconds_total{mode="idle"}[5m]))`},
		{Name: "cluster_cpu_usage", Query: `sum(rate(node_cpu_seconds_total{mode!="idle"}[5m]))`},
		{Name: "cluster_cpu_total", Query: `sum(machine_cpu_cores)`},
		{Name: "cluster_memory_utilisation", Query: `1 - sum(node_memory_MemAvailable_bytes) / sum(node_memory_MemTotal_bytes)`},
		{Name: "cluster_memory_available", Query: `sum(node_memory_MemAvailable_bytes)`},
		{Name: "cluster_memory_total", Query: `sum(node_memory_MemTotal_bytes)`},
		{Name: "cluster_memory_usage_wo_cache", Query: `sum(node_memory_MemTotal_bytes - node_memory_MemAvailable_bytes)`},
		{Name: "cluster_net_utilisation", Query: `sum(rate(node_network_transmit_bytes_total{device!~"lo|docker.*|veth.*|cali.*|flannel.*"}[5m])) + sum(rate(node_network_receive_bytes_total{device!~"lo|docker.*|veth.*|cali.*|flannel.*"}[5m]))`},
		{Name: "cluster_net_bytes_transmitted", Query: `sum(rate(node_network_transmit_bytes_total{device!~"lo|docker.*|veth.*|cali.*|flannel.*"}[5m]))`},
		{Name: "cluster_net_bytes_received", Query: `sum(rate(node_network_receive_bytes_total{device!~"lo|docker.*|veth.*|cali.*|flannel.*"}[5m]))`},
		{Name: "cluster_disk_read_iops", Query: `sum(rate(node_disk_reads_completed_total[5m]))`},
		{Name: "cluster_disk_write_iops", Query: `sum(rate(node_disk_writes_completed_total[5m]))`},
		{Name: "cluster_disk_read_throughput", Query: `sum(rate(node_disk_read_bytes_total[5m]))`},
		{Name: "cluster_disk_write_throughput", Query: `sum(rate(node_disk_written_bytes_total[5m]))`},
		{Name: "cluster_disk_size_usage", Query: `sum(node_filesystem_size_bytes{mountpoint="/"}) - sum(node_filesystem_avail_bytes{mountpoint="/"})`},
		{Name: "cluster_disk_size_utilisation", Query: `1 - sum(node_filesystem_avail_bytes{mountpoint="/"}) / sum(node_filesystem_size_bytes{mountpoint="/"})`},
		{Name: "cluster_disk_size_capacity", Query: `sum(node_filesystem_size_bytes{mountpoint="/"})`},
		{Name: "cluster_disk_size_available", Query: `sum(node_filesystem_avail_bytes{mountpoint="/"})`},
		{Name: "cluster_disk_inode_total", Query: `sum(node_filesystem_files{mountpoint="/"})`},
		{Name: "cluster_disk_inode_usage", Query: `sum(node_filesystem_files{mountpoint="/"}) - sum(node_filesystem_files_free{mountpoint="/"})`},
		{Name: "cluster_disk_inode_utilisation", Query: `1 - sum(node_filesystem_files_free{mountpoint="/"}) / sum(node_filesystem_files{mountpoint="/"})`},
		{Name: "cluster_namespace_count", Query: `count(kube_namespace_created)`},
		{Name: "cluster_pod_count", Query: `count(kube_pod_info)`},
		{Name: "cluster_pod_quota", Query: `sum(kube_node_status_allocatable{resource="pods"})`},
		{Name: "cluster_pod_utilisation", Query: `count(kube_pod_info) / sum(kube_node_status_allocatable{resource="pods"})`},
		{Name: "cluster_pod_running_count", Query: `count(kube_pod_status_phase{phase="Running"} == 1)`},
		{Name: "cluster_pod_succeeded_count", Query: `count(kube_pod_status_phase{phase="Succeeded"} == 1)`},
		{Name: "cluster_pod_abnormal_count", Query: `count(kube_pod_status_phase{phase=~"Failed|Unknown"} == 1)`},
		{Name: "cluster_node_online", Query: `count(kube_node_status_condition{condition="Ready",status="true"} == 1)`},
		{Name: "cluster_node_offline", Query: `count(kube_node_status_condition{condition="Ready",status="true"} == 0)`},
		{Name: "cluster_node_total", Query: `count(kube_node_info)`},
		{Name: "cluster_cronjob_count", Query: `count(kube_cronjob_info)`},
		{Name: "cluster_pvc_count", Query: `count(kube_persistentvolumeclaim_info)`},
		{Name: "cluster_daemonset_count", Query: `count(kube_daemonset_created)`},
		{Name: "cluster_deployment_count", Query: `count(kube_deployment_created)`},
		{Name: "cluster_statefulset_count", Query: `count(kube_statefulset_created)`},
		{Name: "cluster_service_count", Query: `count(kube_service_info)`},
		{Name: "cluster_load1", Query: `avg(node_load1)`},
		{Name: "cluster_load5", Query: `avg(node_load5)`},
		{Name: "cluster_load15", Query: `avg(node_load15)`},
		{Name: "cluster_pod_abnormal_ratio", Query: `count(kube_pod_status_phase{phase=~"Failed|Unknown"} == 1) / count(kube_pod_info)`},
		{Name: "cluster_node_offline_ratio", Query: `count(kube_node_status_condition{condition="Ready",status="true"} == 0) / count(kube_node_info)`},
	}
}

// nodeQueries returns PromQL queries for node-level metrics.
func nodeQueries() []namedQuery {
	return []namedQuery{
		{Name: "node_cpu_utilisation", Query: `1 - avg by(instance) (rate(node_cpu_seconds_total{mode="idle"}[5m]))`},
		{Name: "node_cpu_total", Query: `count by(instance) (node_cpu_seconds_total{mode="idle"})`},
		{Name: "node_cpu_usage", Query: `sum by(instance) (rate(node_cpu_seconds_total{mode!="idle"}[5m]))`},
		{Name: "node_memory_utilisation", Query: `1 - node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes`},
		{Name: "node_memory_usage_wo_cache", Query: `node_memory_MemTotal_bytes - node_memory_MemAvailable_bytes`},
		{Name: "node_memory_available", Query: `node_memory_MemAvailable_bytes`},
		{Name: "node_memory_total", Query: `node_memory_MemTotal_bytes`},
		{Name: "node_net_utilisation", Query: `sum by(instance) (rate(node_network_transmit_bytes_total{device!~"lo|docker.*|veth.*|cali.*|flannel.*"}[5m])) + sum by(instance) (rate(node_network_receive_bytes_total{device!~"lo|docker.*|veth.*|cali.*|flannel.*"}[5m]))`},
		{Name: "node_net_bytes_transmitted", Query: `sum by(instance) (rate(node_network_transmit_bytes_total{device!~"lo|docker.*|veth.*|cali.*|flannel.*"}[5m]))`},
		{Name: "node_net_bytes_received", Query: `sum by(instance) (rate(node_network_receive_bytes_total{device!~"lo|docker.*|veth.*|cali.*|flannel.*"}[5m]))`},
		{Name: "node_disk_read_iops", Query: `sum by(instance) (rate(node_disk_reads_completed_total[5m]))`},
		{Name: "node_disk_write_iops", Query: `sum by(instance) (rate(node_disk_writes_completed_total[5m]))`},
		{Name: "node_disk_read_throughput", Query: `sum by(instance) (rate(node_disk_read_bytes_total[5m]))`},
		{Name: "node_disk_write_throughput", Query: `sum by(instance) (rate(node_disk_written_bytes_total[5m]))`},
		{Name: "node_disk_size_capacity", Query: `sum by(instance) (node_filesystem_size_bytes{mountpoint="/"})`},
		{Name: "node_disk_size_available", Query: `sum by(instance) (node_filesystem_avail_bytes{mountpoint="/"})`},
		{Name: "node_disk_size_usage", Query: `sum by(instance) (node_filesystem_size_bytes{mountpoint="/"}) - sum by(instance) (node_filesystem_avail_bytes{mountpoint="/"})`},
		{Name: "node_disk_size_utilisation", Query: `1 - sum by(instance) (node_filesystem_avail_bytes{mountpoint="/"}) / sum by(instance) (node_filesystem_size_bytes{mountpoint="/"})`},
		{Name: "node_disk_inode_total", Query: `sum by(instance) (node_filesystem_files{mountpoint="/"})`},
		{Name: "node_disk_inode_usage", Query: `sum by(instance) (node_filesystem_files{mountpoint="/"}) - sum by(instance) (node_filesystem_files_free{mountpoint="/"})`},
		{Name: "node_disk_inode_utilisation", Query: `1 - sum by(instance) (node_filesystem_files_free{mountpoint="/"}) / sum by(instance) (node_filesystem_files{mountpoint="/"})`},
		{Name: "node_pod_count", Query: `count by(node) (kube_pod_info)`},
		{Name: "node_pod_running_count", Query: `count by(node) (kube_pod_status_phase{phase="Running"} == 1)`},
		{Name: "node_load1", Query: `node_load1`},
		{Name: "node_load5", Query: `node_load5`},
		{Name: "node_load15", Query: `node_load15`},
	}
}

// namespaceQueries returns PromQL queries for namespace-level metrics.
func namespaceQueries() []namedQuery {
	return []namedQuery{
		{Name: "namespace_cpu_usage", Query: `sum by(namespace) (rate(container_cpu_usage_seconds_total{container!=""}[5m]))`},
		{Name: "namespace_memory_usage", Query: `sum by(namespace) (container_memory_usage_bytes{container!=""})`},
		{Name: "namespace_memory_usage_wo_cache", Query: `sum by(namespace) (container_memory_working_set_bytes{container!=""})`},
		{Name: "namespace_net_bytes_transmitted", Query: `sum by(namespace) (rate(container_network_transmit_bytes_total[5m]))`},
		{Name: "namespace_net_bytes_received", Query: `sum by(namespace) (rate(container_network_receive_bytes_total[5m]))`},
		{Name: "namespace_pod_count", Query: `count by(namespace) (kube_pod_info)`},
		{Name: "namespace_pod_running_count", Query: `count by(namespace) (kube_pod_status_phase{phase="Running"} == 1)`},
	}
}

// etcdQueries returns PromQL queries for etcd component metrics.
func etcdQueries() []namedQuery {
	return []namedQuery{
		{Name: "etcd_server_list", Query: `label_replace(up{job="etcd"}, "node_ip", "$1", "instance", "(.*):.*")`},
		{Name: "etcd_server_total", Query: `count(up{job="etcd"})`},
		{Name: "etcd_server_up_total", Query: `count(up{job="etcd"} == 1)`},
		{Name: "etcd_server_has_leader", Query: `etcd_server_has_leader`},
		{Name: "etcd_server_leader_changes", Query: `max(etcd_server_leader_changes_seen_total)`},
		{Name: "etcd_mvcc_db_size", Query: `etcd_mvcc_db_total_size_in_bytes`},
		{Name: "etcd_network_client_grpc_received_bytes", Query: `sum(rate(etcd_network_client_grpc_received_bytes_total[5m]))`},
		{Name: "etcd_network_client_grpc_sent_bytes", Query: `sum(rate(etcd_network_client_grpc_sent_bytes_total[5m]))`},
		{Name: "etcd_disk_wal_fsync_duration", Query: `histogram_quantile(0.99, sum(rate(etcd_disk_wal_fsync_duration_seconds_bucket[5m])) by(instance, le))`},
		{Name: "etcd_disk_backend_commit_duration", Query: `histogram_quantile(0.99, sum(rate(etcd_disk_backend_commit_duration_seconds_bucket[5m])) by(instance, le))`},
	}
}

// apiserverQueries returns PromQL queries for API server metrics.
func apiserverQueries() []namedQuery {
	return []namedQuery{
		{Name: "apiserver_up_sum", Query: `count(up{job="apiserver"} == 1)`},
		{Name: "apiserver_request_rate", Query: `sum(rate(apiserver_request_total[5m]))`},
		{Name: "apiserver_request_by_verb_rate", Query: `sum by(verb) (rate(apiserver_request_total[5m]))`},
		{Name: "apiserver_request_latencies", Query: `histogram_quantile(0.99, sum(rate(apiserver_request_duration_seconds_bucket[5m])) by(le))`},
	}
}

// schedulerQueries returns PromQL queries for scheduler metrics.
func schedulerQueries() []namedQuery {
	return []namedQuery{
		{Name: "scheduler_up_sum", Query: `count(up{job="kube-scheduler"} == 1)`},
		{Name: "scheduler_schedule_attempts", Query: `sum(scheduler_schedule_attempts_total)`},
		{Name: "scheduler_schedule_attempt_rate", Query: `sum(rate(scheduler_schedule_attempts_total[5m])) by(result)`},
		{Name: "scheduler_e2e_scheduling_latency", Query: `histogram_quantile(0.99, sum(rate(scheduler_e2e_scheduling_duration_seconds_bucket[5m])) by(le))`},
	}
}
