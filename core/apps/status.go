package apps

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type InstalledApp struct {
	Name       string    `json:"name"`
	Release    string    `json:"release"`
	Status     string    `json:"status"`
	Version    string    `json:"version"`
	AppVersion string    `json:"app_version"`
	Updated    string    `json:"updated"`
	Pods       []PodInfo `json:"pods"`
}

type NodeInfo struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Roles   string `json:"roles"`
	Version string `json:"version"`
}

type ServiceInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
	ClusterIP string `json:"cluster_ip"`
	Ports     string `json:"ports"`
}

type SystemStatus struct {
	Nodes    []NodeInfo    `json:"nodes"`
	Services []ServiceInfo `json:"services"`
}

func ListInstalledWithStatus() ([]InstalledApp, error) {
	releases, err := ListInstalled()
	if err != nil {
		return nil, err
	}

	var apps []InstalledApp
	for _, r := range releases {
		name := r.Name
		if strings.HasPrefix(name, "pack-") {
			name = name[5:]
		}

		pods := getPodsForRelease(r.Name, r.Namespace)

		apps = append(apps, InstalledApp{
			Name:       name,
			Release:    r.Name,
			Status:     r.Status,
			Version:    r.Chart,
			AppVersion: r.AppVersion,
			Updated:    r.Updated,
			Pods:       pods,
		})
	}

	return apps, nil
}

func GetSystemStatus() (*SystemStatus, error) {
	status := &SystemStatus{}

	nodeCmd := exec.Command("kubectl", "get", "nodes", "-o", "json")
	nodeOut, err := nodeCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("get nodes: %w", err)
	}

	var nodeList struct {
		Items []struct {
			Metadata struct {
				Name   string            `json:"name"`
				Labels map[string]string `json:"labels"`
			} `json:"metadata"`
			Status struct {
				Conditions []struct {
					Type   string `json:"type"`
					Status string `json:"status"`
				} `json:"conditions"`
				NodeInfo struct {
					KubeletVersion string `json:"kubeletVersion"`
				} `json:"nodeInfo"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(nodeOut, &nodeList); err != nil {
		return nil, fmt.Errorf("parse nodes: %w", err)
	}

	for _, n := range nodeList.Items {
		nodeStatus := "Unknown"
		for _, c := range n.Status.Conditions {
			if c.Type == "Ready" {
				if c.Status == "True" {
					nodeStatus = "Ready"
				} else {
					nodeStatus = "NotReady"
				}
			}
		}

		var roles []string
		for k := range n.Metadata.Labels {
			if strings.HasPrefix(k, "node-role.kubernetes.io/") {
				role := strings.TrimPrefix(k, "node-role.kubernetes.io/")
				roles = append(roles, role)
			}
		}
		roleStr := strings.Join(roles, ",")
		if roleStr == "" {
			roleStr = "worker"
		}

		status.Nodes = append(status.Nodes, NodeInfo{
			Name:    n.Metadata.Name,
			Status:  nodeStatus,
			Roles:   roleStr,
			Version: n.Status.NodeInfo.KubeletVersion,
		})
	}

	svcCmd := exec.Command("kubectl", "get", "services", "--all-namespaces", "-o", "json")
	svcOut, err := svcCmd.Output()
	if err == nil {
		var svcList struct {
			Items []struct {
				Metadata struct {
					Name      string `json:"name"`
					Namespace string `json:"namespace"`
				} `json:"metadata"`
				Spec struct {
					Type      string `json:"type"`
					ClusterIP string `json:"clusterIP"`
					Ports     []struct {
						Port     int    `json:"port"`
						Protocol string `json:"protocol"`
					} `json:"ports"`
				} `json:"spec"`
			} `json:"items"`
		}
		if err := json.Unmarshal(svcOut, &svcList); err == nil {
			for _, s := range svcList.Items {
				var ports []string
				for _, p := range s.Spec.Ports {
					ports = append(ports, fmt.Sprintf("%d/%s", p.Port, p.Protocol))
				}
				status.Services = append(status.Services, ServiceInfo{
					Name:      s.Metadata.Name,
					Namespace: s.Metadata.Namespace,
					Type:      s.Spec.Type,
					ClusterIP: s.Spec.ClusterIP,
					Ports:     strings.Join(ports, ","),
				})
			}
		}
	}

	return status, nil
}
