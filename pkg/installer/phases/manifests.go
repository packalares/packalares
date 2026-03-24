package phases

import (
	"fmt"
	"strings"
)

func imageRef(registry, defaultImage string) string {
	if registry == "" {
		return defaultImage
	}
	return registryImage(registry, defaultImage)
}

func generatePlatformManifest(name, namespace, registry string) string {
	switch name {
	case "citus":
		return generateCitusManifest(namespace, registry)
	case "kvrocks":
		return generateKVRocksManifest(namespace, registry)
	case "nats":
		return generateNATSManifest(namespace, registry)
	case "lldap":
		return generateLLDAPManifest(namespace, registry)
	case "opa":
		return generateOPAManifest(namespace, registry)
	default:
		return ""
	}
}

func generateFrameworkManifest(name, namespace string, opts *InstallOptions) string {
	switch name {
	case "auth-service":
		return generateAuthManifest(namespace, opts)
	case "bfl":
		return generateBFLManifest(namespace, opts)
	case "app-service":
		return generateAppServiceManifest(namespace, opts)
	case "system-server":
		return generateSystemServerManifest(namespace, opts)
	case "files-service":
		return generateFilesManifest(namespace, opts)
	case "market-service":
		return generateMarketManifest(namespace, opts)
	case "backup-service":
		return generateBackupManifest(namespace, opts)
	default:
		return ""
	}
}

func generateAppManifest(name, namespace string, opts *InstallOptions) string {
	switch name {
	case "desktop":
		return generateDesktopManifest(namespace, opts)
	case "wizard":
		return generateWizardManifest(namespace, opts)
	default:
		return ""
	}
}

// --- Platform Manifests ---

func generateCitusManifest(namespace, registry string) string {
	img := imageRef(registry, "citusdata/citus:12.1")
	return fmt.Sprintf(`---
apiVersion: v1
kind: Namespace
metadata:
  name: %s
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: citus-config
  namespace: %s
data:
  POSTGRES_DB: "olares"
  POSTGRES_USER: "olares"
---
apiVersion: v1
kind: Secret
metadata:
  name: citus-secret
  namespace: %s
type: Opaque
stringData:
  POSTGRES_PASSWORD: "packalares-pg-secret"
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: citus-coordinator
  namespace: %s
  labels:
    app: citus
    role: coordinator
spec:
  serviceName: citus-coordinator
  replicas: 1
  selector:
    matchLabels:
      app: citus
      role: coordinator
  template:
    metadata:
      labels:
        app: citus
        role: coordinator
    spec:
      containers:
      - name: citus
        image: %s
        ports:
        - containerPort: 5432
          name: postgres
        envFrom:
        - configMapRef:
            name: citus-config
        - secretRef:
            name: citus-secret
        volumeMounts:
        - name: data
          mountPath: /var/lib/postgresql/data
        resources:
          requests:
            cpu: 100m
            memory: 256Mi
          limits:
            cpu: "2"
            memory: 2Gi
        readinessProbe:
          exec:
            command: ["pg_isready", "-U", "olares"]
          initialDelaySeconds: 10
          periodSeconds: 5
  volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes: ["ReadWriteOnce"]
      resources:
        requests:
          storage: 10Gi
---
apiVersion: v1
kind: Service
metadata:
  name: citus-coordinator-svc
  namespace: %s
spec:
  selector:
    app: citus
    role: coordinator
  ports:
  - port: 5432
    targetPort: 5432
  clusterIP: None
`, namespace, namespace, namespace, namespace, img, namespace)
}

func generateKVRocksManifest(namespace, registry string) string {
	img := imageRef(registry, "ghcr.io/packalares/kvrocks:2.15.0")
	return fmt.Sprintf(`---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: kvrocks
  namespace: %s
  labels:
    app: kvrocks
spec:
  serviceName: kvrocks
  replicas: 1
  selector:
    matchLabels:
      app: kvrocks
  template:
    metadata:
      labels:
        app: kvrocks
    spec:
      containers:
      - name: kvrocks
        image: %s
        ports:
        - containerPort: 6666
          name: kvrocks
        volumeMounts:
        - name: data
          mountPath: /kvrocks/data
        resources:
          requests:
            cpu: 50m
            memory: 128Mi
          limits:
            cpu: "1"
            memory: 1Gi
        readinessProbe:
          tcpSocket:
            port: 6666
          initialDelaySeconds: 5
          periodSeconds: 5
  volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes: ["ReadWriteOnce"]
      resources:
        requests:
          storage: 5Gi
---
apiVersion: v1
kind: Service
metadata:
  name: kvrocks-svc
  namespace: %s
spec:
  selector:
    app: kvrocks
  ports:
  - port: 6666
    targetPort: 6666
  clusterIP: None
`, namespace, img, namespace)
}

func generateNATSManifest(namespace, registry string) string {
	img := imageRef(registry, "nats:2.10-alpine")
	return fmt.Sprintf(`---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nats
  namespace: %s
  labels:
    app: nats
spec:
  replicas: 1
  selector:
    matchLabels:
      app: nats
  template:
    metadata:
      labels:
        app: nats
    spec:
      containers:
      - name: nats
        image: %s
        ports:
        - containerPort: 4222
          name: client
        - containerPort: 8222
          name: monitoring
        args: ["-js", "-m", "8222"]
        resources:
          requests:
            cpu: 50m
            memory: 64Mi
          limits:
            cpu: 500m
            memory: 512Mi
        readinessProbe:
          httpGet:
            path: /healthz
            port: 8222
          initialDelaySeconds: 5
          periodSeconds: 5
---
apiVersion: v1
kind: Service
metadata:
  name: nats-svc
  namespace: %s
spec:
  selector:
    app: nats
  ports:
  - name: client
    port: 4222
    targetPort: 4222
  - name: monitoring
    port: 8222
    targetPort: 8222
`, namespace, img, namespace)
}

func generateLLDAPManifest(namespace, registry string) string {
	img := imageRef(registry, "lldap/lldap:v0.5")
	return fmt.Sprintf(`---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: lldap
  namespace: %s
  labels:
    app: lldap
spec:
  replicas: 1
  selector:
    matchLabels:
      app: lldap
  template:
    metadata:
      labels:
        app: lldap
    spec:
      containers:
      - name: lldap
        image: %s
        ports:
        - containerPort: 3890
          name: ldap
        - containerPort: 17170
          name: http
        env:
        - name: LLDAP_JWT_SECRET
          value: "packalares-lldap-jwt-secret"
        - name: LLDAP_LDAP_USER_PASS
          value: "packalares-lldap-admin"
        - name: LLDAP_LDAP_BASE_DN
          value: "dc=olares,dc=local"
        resources:
          requests:
            cpu: 50m
            memory: 64Mi
          limits:
            cpu: 500m
            memory: 256Mi
        readinessProbe:
          tcpSocket:
            port: 3890
          initialDelaySeconds: 5
          periodSeconds: 5
---
apiVersion: v1
kind: Service
metadata:
  name: lldap-svc
  namespace: %s
spec:
  selector:
    app: lldap
  ports:
  - name: ldap
    port: 3890
    targetPort: 3890
  - name: http
    port: 17170
    targetPort: 17170
`, namespace, img, namespace)
}

func generateOPAManifest(namespace, registry string) string {
	img := imageRef(registry, "openpolicyagent/opa:0.62.1")
	return fmt.Sprintf(`---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: opa
  namespace: %s
  labels:
    app: opa
spec:
  replicas: 1
  selector:
    matchLabels:
      app: opa
  template:
    metadata:
      labels:
        app: opa
    spec:
      containers:
      - name: opa
        image: %s
        ports:
        - containerPort: 8181
          name: http
        args: ["run", "--server", "--log-level=info"]
        resources:
          requests:
            cpu: 50m
            memory: 64Mi
          limits:
            cpu: 500m
            memory: 256Mi
        readinessProbe:
          httpGet:
            path: /health
            port: 8181
          initialDelaySeconds: 5
          periodSeconds: 5
---
apiVersion: v1
kind: Service
metadata:
  name: opa-svc
  namespace: %s
spec:
  selector:
    app: opa
  ports:
  - port: 8181
    targetPort: 8181
`, namespace, img, namespace)
}

// --- Framework Manifests ---

func frameworkImage(registry, component string) string {
	defaultImg := fmt.Sprintf("ghcr.io/packalares/%s:latest", component)
	return imageRef(registry, defaultImg)
}

func generateAuthManifest(namespace string, opts *InstallOptions) string {
	img := frameworkImage(opts.Registry, "auth-service")
	return fmt.Sprintf(`---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: auth-service
  namespace: %s
  labels:
    app: auth-service
    tier: framework
spec:
  replicas: 1
  selector:
    matchLabels:
      app: auth-service
  template:
    metadata:
      labels:
        app: auth-service
    spec:
      containers:
      - name: auth-service
        image: %s
        ports:
        - containerPort: 8080
        env:
        - name: DOMAIN
          value: "%s"
        - name: CERT_MODE
          value: "%s"
        resources:
          requests:
            cpu: 50m
            memory: 64Mi
          limits:
            cpu: 500m
            memory: 256Mi
---
apiVersion: v1
kind: Service
metadata:
  name: auth-service-svc
  namespace: %s
spec:
  selector:
    app: auth-service
  ports:
  - port: 8080
    targetPort: 8080
`, namespace, img, opts.Domain, opts.CertMode, namespace)
}

func generateBFLManifest(namespace string, opts *InstallOptions) string {
	img := frameworkImage(opts.Registry, "bfl")
	return fmt.Sprintf(`---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: bfl
  namespace: %s
  labels:
    app: bfl
    tier: framework
spec:
  replicas: 1
  selector:
    matchLabels:
      app: bfl
  template:
    metadata:
      labels:
        app: bfl
    spec:
      serviceAccountName: bfl
      containers:
      - name: bfl
        image: %s
        ports:
        - containerPort: 8080
        env:
        - name: DOMAIN
          value: "%s"
        - name: ADMIN_USER
          value: "%s"
        resources:
          requests:
            cpu: 50m
            memory: 64Mi
          limits:
            cpu: 500m
            memory: 256Mi
---
apiVersion: v1
kind: Service
metadata:
  name: bfl-svc
  namespace: %s
spec:
  selector:
    app: bfl
  ports:
  - port: 8080
    targetPort: 8080
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: bfl
  namespace: %s
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: bfl-admin
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-admin
subjects:
- kind: ServiceAccount
  name: bfl
  namespace: %s
`, namespace, img, opts.Domain, opts.Username, namespace, namespace, namespace)
}

func generateAppServiceManifest(namespace string, opts *InstallOptions) string {
	img := frameworkImage(opts.Registry, "app-service")
	return fmt.Sprintf(`---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: app-service
  namespace: %s
  labels:
    app: app-service
    tier: framework
spec:
  replicas: 1
  selector:
    matchLabels:
      app: app-service
  template:
    metadata:
      labels:
        app: app-service
    spec:
      serviceAccountName: app-service
      containers:
      - name: app-service
        image: %s
        ports:
        - containerPort: 8080
        resources:
          requests:
            cpu: 50m
            memory: 64Mi
          limits:
            cpu: "1"
            memory: 512Mi
---
apiVersion: v1
kind: Service
metadata:
  name: app-service-svc
  namespace: %s
spec:
  selector:
    app: app-service
  ports:
  - port: 8080
    targetPort: 8080
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: app-service
  namespace: %s
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: app-service-admin
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-admin
subjects:
- kind: ServiceAccount
  name: app-service
  namespace: %s
`, namespace, img, namespace, namespace, namespace)
}

func generateSystemServerManifest(namespace string, opts *InstallOptions) string {
	img := frameworkImage(opts.Registry, "system-server")
	return fmt.Sprintf(`---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: system-server
  namespace: %s
  labels:
    app: system-server
    tier: framework
spec:
  replicas: 1
  selector:
    matchLabels:
      app: system-server
  template:
    metadata:
      labels:
        app: system-server
    spec:
      serviceAccountName: system-server
      containers:
      - name: system-server
        image: %s
        ports:
        - containerPort: 8080
        resources:
          requests:
            cpu: 50m
            memory: 64Mi
          limits:
            cpu: 500m
            memory: 256Mi
---
apiVersion: v1
kind: Service
metadata:
  name: system-server-svc
  namespace: %s
spec:
  selector:
    app: system-server
  ports:
  - port: 8080
    targetPort: 8080
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: system-server
  namespace: %s
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: system-server-admin
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-admin
subjects:
- kind: ServiceAccount
  name: system-server
  namespace: %s
`, namespace, img, namespace, namespace, namespace)
}

func generateFilesManifest(namespace string, opts *InstallOptions) string {
	img := frameworkImage(opts.Registry, "files-service")
	return fmt.Sprintf(`---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: files-service
  namespace: %s
  labels:
    app: files-service
    tier: framework
spec:
  replicas: 1
  selector:
    matchLabels:
      app: files-service
  template:
    metadata:
      labels:
        app: files-service
    spec:
      containers:
      - name: files-service
        image: %s
        ports:
        - containerPort: 8080
        resources:
          requests:
            cpu: 50m
            memory: 64Mi
          limits:
            cpu: 500m
            memory: 256Mi
---
apiVersion: v1
kind: Service
metadata:
  name: files-service-svc
  namespace: %s
spec:
  selector:
    app: files-service
  ports:
  - port: 8080
    targetPort: 8080
`, namespace, img, namespace)
}

func generateMarketManifest(namespace string, opts *InstallOptions) string {
	img := frameworkImage(opts.Registry, "market-service")
	return fmt.Sprintf(`---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: market-service
  namespace: %s
  labels:
    app: market-service
    tier: framework
spec:
  replicas: 1
  selector:
    matchLabels:
      app: market-service
  template:
    metadata:
      labels:
        app: market-service
    spec:
      containers:
      - name: market-service
        image: %s
        ports:
        - containerPort: 8080
        resources:
          requests:
            cpu: 50m
            memory: 64Mi
          limits:
            cpu: 500m
            memory: 256Mi
---
apiVersion: v1
kind: Service
metadata:
  name: market-service-svc
  namespace: %s
spec:
  selector:
    app: market-service
  ports:
  - port: 8080
    targetPort: 8080
`, namespace, img, namespace)
}

func generateBackupManifest(namespace string, opts *InstallOptions) string {
	img := frameworkImage(opts.Registry, "backup-service")
	return fmt.Sprintf(`---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: backup-service
  namespace: %s
  labels:
    app: backup-service
    tier: framework
spec:
  replicas: 1
  selector:
    matchLabels:
      app: backup-service
  template:
    metadata:
      labels:
        app: backup-service
    spec:
      containers:
      - name: backup-service
        image: %s
        ports:
        - containerPort: 8080
        resources:
          requests:
            cpu: 50m
            memory: 64Mi
          limits:
            cpu: 500m
            memory: 256Mi
---
apiVersion: v1
kind: Service
metadata:
  name: backup-service-svc
  namespace: %s
spec:
  selector:
    app: backup-service
  ports:
  - port: 8080
    targetPort: 8080
`, namespace, img, namespace)
}

// --- App Manifests ---

func generateDesktopManifest(namespace string, opts *InstallOptions) string {
	img := frameworkImage(opts.Registry, "desktop")
	return fmt.Sprintf(`---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: desktop
  namespace: %s
  labels:
    app: desktop
    tier: user-app
spec:
  replicas: 1
  selector:
    matchLabels:
      app: desktop
  template:
    metadata:
      labels:
        app: desktop
    spec:
      containers:
      - name: desktop
        image: %s
        ports:
        - containerPort: 80
        resources:
          requests:
            cpu: 50m
            memory: 64Mi
          limits:
            cpu: 500m
            memory: 256Mi
---
apiVersion: v1
kind: Service
metadata:
  name: desktop-svc
  namespace: %s
spec:
  selector:
    app: desktop
  ports:
  - port: 80
    targetPort: 80
`, namespace, img, namespace)
}

func generateWizardManifest(namespace string, opts *InstallOptions) string {
	img := frameworkImage(opts.Registry, "wizard")
	return fmt.Sprintf(`---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: wizard
  namespace: %s
  labels:
    app: wizard
    tier: user-app
spec:
  replicas: 1
  selector:
    matchLabels:
      app: wizard
  template:
    metadata:
      labels:
        app: wizard
    spec:
      containers:
      - name: wizard
        image: %s
        ports:
        - containerPort: 80
        resources:
          requests:
            cpu: 50m
            memory: 64Mi
          limits:
            cpu: 500m
            memory: 256Mi
---
apiVersion: v1
kind: Service
metadata:
  name: wizard-svc
  namespace: %s
spec:
  selector:
    app: wizard
  ports:
  - port: 80
    targetPort: 80
`, namespace, img, namespace)
}

// --- Infrastructure Manifests ---

func generateMonitoringManifest(registry string) string {
	promImg := imageRef(registry, "prom/prometheus:v2.51.0")
	nodeExImg := imageRef(registry, "prom/node-exporter:v1.7.0")
	ksmetricsImg := imageRef(registry, "registry.k8s.io/kube-state-metrics/kube-state-metrics:v2.11.0")

	return fmt.Sprintf(`---
apiVersion: v1
kind: Namespace
metadata:
  name: monitoring
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: prometheus
  namespace: monitoring
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: prometheus
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-admin
subjects:
- kind: ServiceAccount
  name: prometheus
  namespace: monitoring
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: prometheus
  namespace: monitoring
  labels:
    app: prometheus
spec:
  replicas: 1
  selector:
    matchLabels:
      app: prometheus
  template:
    metadata:
      labels:
        app: prometheus
    spec:
      serviceAccountName: prometheus
      containers:
      - name: prometheus
        image: %s
        ports:
        - containerPort: 9090
        args:
        - "--config.file=/etc/prometheus/prometheus.yml"
        - "--storage.tsdb.retention.time=15d"
        resources:
          requests:
            cpu: 100m
            memory: 256Mi
          limits:
            cpu: "1"
            memory: 1Gi
---
apiVersion: v1
kind: Service
metadata:
  name: prometheus-svc
  namespace: monitoring
spec:
  selector:
    app: prometheus
  ports:
  - port: 9090
    targetPort: 9090
---
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: node-exporter
  namespace: monitoring
  labels:
    app: node-exporter
spec:
  selector:
    matchLabels:
      app: node-exporter
  template:
    metadata:
      labels:
        app: node-exporter
    spec:
      hostNetwork: true
      hostPID: true
      containers:
      - name: node-exporter
        image: %s
        ports:
        - containerPort: 9100
        args:
        - "--path.rootfs=/host"
        volumeMounts:
        - name: rootfs
          mountPath: /host
          readOnly: true
        resources:
          requests:
            cpu: 50m
            memory: 32Mi
          limits:
            cpu: 200m
            memory: 128Mi
      volumes:
      - name: rootfs
        hostPath:
          path: /
---
apiVersion: v1
kind: Service
metadata:
  name: node-exporter-svc
  namespace: monitoring
spec:
  selector:
    app: node-exporter
  ports:
  - port: 9100
    targetPort: 9100
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kube-state-metrics
  namespace: monitoring
  labels:
    app: kube-state-metrics
spec:
  replicas: 1
  selector:
    matchLabels:
      app: kube-state-metrics
  template:
    metadata:
      labels:
        app: kube-state-metrics
    spec:
      serviceAccountName: prometheus
      containers:
      - name: kube-state-metrics
        image: %s
        ports:
        - containerPort: 8080
        - containerPort: 8081
        resources:
          requests:
            cpu: 50m
            memory: 64Mi
          limits:
            cpu: 200m
            memory: 256Mi
---
apiVersion: v1
kind: Service
metadata:
  name: kube-state-metrics-svc
  namespace: monitoring
spec:
  selector:
    app: kube-state-metrics
  ports:
  - name: metrics
    port: 8080
    targetPort: 8080
  - name: telemetry
    port: 8081
    targetPort: 8081
`, promImg, nodeExImg, ksmetricsImg)
}

func generateKubeBlocksManifest(registry string) string {
	_ = strings.TrimSpace // avoid unused import
	img := imageRef(registry, "apecloud/kubeblocks:0.8.2")
	return fmt.Sprintf(`---
apiVersion: v1
kind: Namespace
metadata:
  name: kubeblocks-system
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kubeblocks
  namespace: kubeblocks-system
  labels:
    app: kubeblocks
spec:
  replicas: 1
  selector:
    matchLabels:
      app: kubeblocks
  template:
    metadata:
      labels:
        app: kubeblocks
    spec:
      containers:
      - name: kubeblocks
        image: %s
        ports:
        - containerPort: 8080
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 512Mi
---
apiVersion: v1
kind: Service
metadata:
  name: kubeblocks-svc
  namespace: kubeblocks-system
spec:
  selector:
    app: kubeblocks
  ports:
  - port: 8080
    targetPort: 8080
`, img)
}
