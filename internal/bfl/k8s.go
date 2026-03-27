package bfl

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	pkgconfig "github.com/packalares/packalares/pkg/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

// iamUserGVR is the GVR for the iam.kubesphere.io/v1alpha2 User CRD.
var iamUserGVR = schema.GroupVersionResource{
	Group:    "iam.kubesphere.io",
	Version:  "v1alpha2",
	Resource: "users",
}

// K8sClient wraps the Kubernetes API interactions needed by BFL.
type K8sClient struct {
	Clientset     kubernetes.Interface
	DynamicClient dynamic.Interface
	Namespace     string
	Username      string
}

// NewK8sClient creates a K8sClient from in-cluster config.
func NewK8sClient() (*K8sClient, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("in-cluster config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create clientset: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create dynamic client: %w", err)
	}

	ns := os.Getenv("BFL_NAMESPACE")
	if ns == "" {
		ns = os.Getenv("NAMESPACE")
	}
	if ns == "" {
		// Derive from username
		user := os.Getenv("OWNER")
		if user == "" {
			user = os.Getenv("USER_NAME")
		}
		if user != "" {
			ns = fmt.Sprintf("%s-%s", DefaultUserNamespacePrefix, user)
		} else {
			ns = pkgconfig.UserNamespace(pkgconfig.Username())
		}
	}

	username := os.Getenv("OWNER")
	if username == "" {
		username = os.Getenv("USER_NAME")
	}
	if username == "" {
		username = "admin"
	}

	return &K8sClient{
		Clientset:     clientset,
		DynamicClient: dynClient,
		Namespace:     ns,
		Username:      username,
	}, nil
}

// ---------------------------------------------------------------------------
// User CRD helpers
// ---------------------------------------------------------------------------

// GetUser fetches the User CRD. If name is empty, uses k.Username.
func (k *K8sClient) GetUser(ctx context.Context, name string) (*unstructured.Unstructured, error) {
	if name == "" {
		name = k.Username
	}
	return k.DynamicClient.Resource(iamUserGVR).Get(ctx, name, metav1.GetOptions{})
}

// ListUsers returns all User CRDs.
func (k *K8sClient) ListUsers(ctx context.Context) (*unstructured.UnstructuredList, error) {
	return k.DynamicClient.Resource(iamUserGVR).List(ctx, metav1.ListOptions{})
}

// UpdateUser writes back the User CRD.
func (k *K8sClient) UpdateUser(ctx context.Context, user *unstructured.Unstructured) error {
	_, err := k.DynamicClient.Resource(iamUserGVR).Update(ctx, user, metav1.UpdateOptions{})
	return err
}

// GetUserAnnotation returns a single annotation value from a User object.
func GetUserAnnotation(user *unstructured.Unstructured, key string) string {
	annos := user.GetAnnotations()
	if annos == nil {
		return ""
	}
	return annos[key]
}

// SetUserAnnotation sets a single annotation.
func SetUserAnnotation(user *unstructured.Unstructured, key, value string) {
	annos := user.GetAnnotations()
	if annos == nil {
		annos = make(map[string]string)
	}
	annos[key] = value
	user.SetAnnotations(annos)
}

// DeleteUserAnnotation removes an annotation.
func DeleteUserAnnotation(user *unstructured.Unstructured, key string) {
	annos := user.GetAnnotations()
	if annos == nil {
		return
	}
	delete(annos, key)
	user.SetAnnotations(annos)
}

// GetTerminusName returns the terminus-name annotation.
func GetTerminusName(user *unstructured.Unstructured) string {
	return GetUserAnnotation(user, AnnoTerminusName)
}

// GetUserZone returns the zone annotation.
func GetUserZone(user *unstructured.Unstructured) string {
	return GetUserAnnotation(user, AnnoZone)
}

// GetWizardStatus returns the current wizard status.
func GetWizardStatus(user *unstructured.Unstructured) WizardStatus {
	s := GetUserAnnotation(user, AnnoWizardStatus)
	if s == "" {
		return WaitActivateVault
	}
	return WizardStatus(s)
}

// IsWizardComplete returns true if wizard is completed or waiting for password reset.
func IsWizardComplete(user *unstructured.Unstructured) bool {
	s := GetWizardStatus(user)
	return s == Completed || s == WaitResetPassword
}

// GetDomain derives the domain from the user's terminus name or env.
func (k *K8sClient) GetDomain(ctx context.Context) (string, error) {
	domain := os.Getenv("OLARES_DOMAIN")
	if domain != "" {
		return domain, nil
	}
	// Try to read from cluster configmap
	cm, err := k.Clientset.CoreV1().ConfigMaps(pkgconfig.PlatformNamespace()).Get(ctx, "user-config", metav1.GetOptions{})
	if err == nil {
		if d, ok := cm.Data["domain"]; ok && d != "" {
			return d, nil
		}
	}
	return "local.packalares.io", nil
}

// GetOSVersion reads OS version from env or configmap.
func (k *K8sClient) GetOSVersion(ctx context.Context) string {
	if v := os.Getenv("OLARES_VERSION"); v != "" {
		return v
	}
	cm, err := k.Clientset.CoreV1().ConfigMaps(pkgconfig.PlatformNamespace()).Get(ctx, "os-config", metav1.GetOptions{})
	if err == nil {
		if v, ok := cm.Data["version"]; ok {
			return v
		}
	}
	return "unknown"
}

// ---------------------------------------------------------------------------
// SSL Secret helpers
// ---------------------------------------------------------------------------

// GetSSLSecret returns the zone-ssl-config Secret data.
func (k *K8sClient) GetSSLSecret(ctx context.Context) (map[string]string, error) {
	secret, err := k.Clientset.CoreV1().Secrets(k.Namespace).Get(ctx, SSLSecretName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	data := make(map[string]string, len(secret.Data))
	for k, v := range secret.Data {
		data[k] = string(v)
	}
	return data, nil
}

// EnsureSSLSecret creates or updates zone-ssl-config Secret with cert data.
func (k *K8sClient) EnsureSSLSecret(ctx context.Context, zone, certPEM, keyPEM string) error {
	expiry := time.Now().AddDate(10, 0, 0).Format("2006-01-02 15:04:05")
	data := map[string][]byte{
		"zone":       []byte(zone),
		"cert":       []byte(certPEM),
		"key":        []byte(keyPEM),
		"expired_at": []byte(expiry),
	}

	existing, err := k.Clientset.CoreV1().Secrets(k.Namespace).Get(ctx, SSLSecretName, metav1.GetOptions{})
	if err != nil {
		// Create
		newSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      SSLSecretName,
				Namespace: k.Namespace,
			},
			Type: corev1.SecretTypeOpaque,
			Data: data,
		}
		_, err = k.Clientset.CoreV1().Secrets(k.Namespace).Create(ctx, newSecret, metav1.CreateOptions{})
		return err
	}

	// Update
	existing.Data = data
	_, err = k.Clientset.CoreV1().Secrets(k.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

// ---------------------------------------------------------------------------
// Reverse proxy config helpers
// ---------------------------------------------------------------------------

// GetReverseProxyConfig reads the reverse-proxy-config ConfigMap.
func (k *K8sClient) GetReverseProxyConfig(ctx context.Context) (map[string]string, error) {
	cm, err := k.Clientset.CoreV1().ConfigMaps(k.Namespace).Get(ctx, ReverseProxyConfigMap, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return cm.Data, nil
}

// WriteReverseProxyConfig creates or updates the reverse-proxy-config ConfigMap.
func (k *K8sClient) WriteReverseProxyConfig(ctx context.Context, data map[string]string) error {
	cm, err := k.Clientset.CoreV1().ConfigMaps(k.Namespace).Get(ctx, ReverseProxyConfigMap, metav1.GetOptions{})
	if err != nil {
		newCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ReverseProxyConfigMap,
				Namespace: k.Namespace,
			},
			Data: data,
		}
		_, err = k.Clientset.CoreV1().ConfigMaps(k.Namespace).Create(ctx, newCM, metav1.CreateOptions{})
		return err
	}
	cm.Data = data
	_, err = k.Clientset.CoreV1().ConfigMaps(k.Namespace).Update(ctx, cm, metav1.UpdateOptions{})
	return err
}

// ---------------------------------------------------------------------------
// User extraction helpers for unstructured User CRDs
// ---------------------------------------------------------------------------

// extractUserSpec extracts typed fields from the User CRD spec.
func extractUserSpec(user *unstructured.Unstructured) (displayName, description, email, state string) {
	spec, ok := user.Object["spec"].(map[string]interface{})
	if !ok {
		return
	}
	if v, ok := spec["displayName"].(string); ok {
		displayName = v
	}
	if v, ok := spec["description"].(string); ok {
		description = v
	}
	if v, ok := spec["email"].(string); ok {
		email = v
	}
	status, ok := user.Object["status"].(map[string]interface{})
	if ok {
		if v, ok := status["state"].(string); ok {
			state = v
		}
	}
	return
}

// extractLastLoginTime returns the Unix timestamp of lastLoginTime, or nil.
func extractLastLoginTime(user *unstructured.Unstructured) *int64 {
	status, ok := user.Object["status"].(map[string]interface{})
	if !ok {
		return nil
	}
	v, ok := status["lastLoginTime"]
	if !ok || v == nil {
		return nil
	}
	switch val := v.(type) {
	case string:
		t, err := time.Parse(time.RFC3339, val)
		if err != nil {
			return nil
		}
		unix := t.Unix()
		return &unix
	case float64:
		unix := int64(val)
		return &unix
	}
	return nil
}

// ---------------------------------------------------------------------------
// GlobalRoleBinding helpers
// ---------------------------------------------------------------------------

var globalRoleBindingGVR = schema.GroupVersionResource{
	Group:    "iam.kubesphere.io",
	Version:  "v1alpha2",
	Resource: "globalrolebindings",
}

// GetUserRoles returns the global roles bound to a given username.
func (k *K8sClient) GetUserRoles(ctx context.Context, username string) ([]string, error) {
	list, err := k.DynamicClient.Resource(globalRoleBindingGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	roles := make(map[string]struct{})
	for _, item := range list.Items {
		subjects, found, _ := unstructured.NestedSlice(item.Object, "subjects")
		if !found {
			continue
		}
		roleRef, _, _ := unstructured.NestedString(item.Object, "roleRef", "name")
		for _, s := range subjects {
			subj, ok := s.(map[string]interface{})
			if !ok {
				continue
			}
			if name, _ := subj["name"].(string); name == username {
				roles[roleRef] = struct{}{}
			}
		}
	}

	result := make([]string, 0, len(roles))
	for r := range roles {
		result = append(result, r)
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Cluster metrics helpers (reads from kubelet/node metrics)
// ---------------------------------------------------------------------------

// GetClusterMetrics returns basic cluster resource metrics by reading node
// allocatable resources and node conditions. Falls back to stub values if
// the metrics API is unavailable.
func (k *K8sClient) GetClusterMetrics(ctx context.Context) ClusterMetrics {
	metrics := ClusterMetrics{}

	nodes, err := k.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		klog.Warningf("cannot list nodes for metrics: %v", err)
		return metrics
	}

	var totalCPUMillis, totalMemBytes, totalDiskBytes int64

	for _, node := range nodes.Items {
		if cpu, ok := node.Status.Allocatable[corev1.ResourceCPU]; ok {
			totalCPUMillis += cpu.MilliValue()
		}
		if mem, ok := node.Status.Allocatable[corev1.ResourceMemory]; ok {
			totalMemBytes += mem.Value()
		}
		if disk, ok := node.Status.Allocatable[corev1.ResourceEphemeralStorage]; ok {
			totalDiskBytes += disk.Value()
		}
	}

	metrics.CPU.Total = float64(totalCPUMillis) / 1000.0
	metrics.CPU.Unit = "Cores"
	metrics.Memory.Total = float64(totalMemBytes) / 1e9
	metrics.Memory.Unit = "GB"
	metrics.Disk.Total = float64(totalDiskBytes) / 1e9
	metrics.Disk.Unit = "GB"

	// Try reading usage from metrics-server
	usageMetrics := k.readNodeMetrics(ctx)
	metrics.CPU.Usage = usageMetrics.cpuCores
	metrics.Memory.Usage = usageMetrics.memGB

	if metrics.CPU.Total > 0 {
		metrics.CPU.Ratio = float64(int(metrics.CPU.Usage/metrics.CPU.Total*100+0.5))
	}
	if metrics.Memory.Total > 0 {
		metrics.Memory.Ratio = float64(int(metrics.Memory.Usage/metrics.Memory.Total*100+0.5))
	}

	return metrics
}

type nodeMetricsUsage struct {
	cpuCores float64
	memGB    float64
}

func (k *K8sClient) readNodeMetrics(ctx context.Context) nodeMetricsUsage {
	var usage nodeMetricsUsage

	metricsGVR := schema.GroupVersionResource{
		Group:    "metrics.k8s.io",
		Version:  "v1beta1",
		Resource: "nodes",
	}

	list, err := k.DynamicClient.Resource(metricsGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		klog.V(2).Infof("cannot read node metrics: %v", err)
		return usage
	}

	for _, item := range list.Items {
		usageMap, found, _ := unstructured.NestedMap(item.Object, "usage")
		if !found {
			continue
		}
		if cpuStr, ok := usageMap["cpu"].(string); ok {
			usage.cpuCores += parseCPU(cpuStr)
		}
		if memStr, ok := usageMap["memory"].(string); ok {
			usage.memGB += parseMemory(memStr) / 1e9
		}
	}
	return usage
}

func parseCPU(s string) float64 {
	if strings.HasSuffix(s, "n") {
		v, _ := strconv.ParseFloat(strings.TrimSuffix(s, "n"), 64)
		return v / 1e9
	}
	if strings.HasSuffix(s, "m") {
		v, _ := strconv.ParseFloat(strings.TrimSuffix(s, "m"), 64)
		return v / 1000
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func parseMemory(s string) float64 {
	if strings.HasSuffix(s, "Ki") {
		v, _ := strconv.ParseFloat(strings.TrimSuffix(s, "Ki"), 64)
		return v * 1024
	}
	if strings.HasSuffix(s, "Mi") {
		v, _ := strconv.ParseFloat(strings.TrimSuffix(s, "Mi"), 64)
		return v * 1024 * 1024
	}
	if strings.HasSuffix(s, "Gi") {
		v, _ := strconv.ParseFloat(strings.TrimSuffix(s, "Gi"), 64)
		return v * 1024 * 1024 * 1024
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// ---------------------------------------------------------------------------
// App service helpers
// ---------------------------------------------------------------------------

var appGVR = schema.GroupVersionResource{
	Group:    "app." + pkgconfig.APIGroup(),
	Version:  "v1alpha1",
	Resource: "applications",
}

// AppInfo is a simplified representation of a user's installed app.
type AppInfo struct {
	Name      string `json:"name"`
	Appid     string `json:"appid"`
	Namespace string `json:"namespace"`
	Owner     string `json:"owner"`
	URL       string `json:"url"`
	Icon      string `json:"icon"`
	State     string `json:"state"`
}

// ListUserApps returns apps owned by the current user.
func (k *K8sClient) ListUserApps(ctx context.Context) ([]AppInfo, error) {
	list, err := k.DynamicClient.Resource(appGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var apps []AppInfo
	for _, item := range list.Items {
		spec, ok := item.Object["spec"].(map[string]interface{})
		if !ok {
			continue
		}
		owner, _ := spec["owner"].(string)
		if owner != k.Username {
			continue
		}
		name, _ := spec["name"].(string)
		appid, _ := spec["appid"].(string)
		ns, _ := spec["namespace"].(string)
		icon, _ := spec["icon"].(string)

		state := "running"
		if s, ok := item.Object["status"].(map[string]interface{}); ok {
			if st, ok := s["state"].(string); ok {
				state = st
			}
		}

		// Build URL from zone and appid
		zone := ""
		user, _ := k.GetUser(ctx, k.Username)
		if user != nil {
			zone = GetUserAnnotation(user, AnnoZone)
		}
		url := ""
		if zone != "" && appid != "" {
			url = fmt.Sprintf("https://%s.%s", appid, zone)
		}

		apps = append(apps, AppInfo{
			Name:      name,
			Appid:     appid,
			Namespace: ns,
			Owner:     owner,
			URL:       url,
			Icon:      icon,
			State:     state,
		})
	}
	return apps, nil
}

// ---------------------------------------------------------------------------
// Sys config helpers
// ---------------------------------------------------------------------------

type SysConfig struct {
	Language string `json:"language"`
	Location string `json:"location"`
	Theme    string `json:"theme"`
}

// GetSysConfig reads locale/theme info from user annotations.
func GetSysConfig(user *unstructured.Unstructured) SysConfig {
	return SysConfig{
		Language: GetUserAnnotation(user, AnnoLanguage),
		Location: GetUserAnnotation(user, AnnoLocation),
		Theme:    GetUserAnnotation(user, AnnoTheme),
	}
}

// SaveSysConfig persists locale/theme to user annotations.
func SaveSysConfig(user *unstructured.Unstructured, cfg SysConfig) {
	if cfg.Language != "" {
		SetUserAnnotation(user, AnnoLanguage, cfg.Language)
	}
	if cfg.Location != "" {
		SetUserAnnotation(user, AnnoLocation, cfg.Location)
	}
	if cfg.Theme == "" {
		cfg.Theme = "light"
	}
	SetUserAnnotation(user, AnnoTheme, cfg.Theme)
}

// UserSysConfigJSON returns JSON bytes for the sys config from user annotations.
func UserSysConfigJSON(user *unstructured.Unstructured) ([]byte, error) {
	cfg := GetSysConfig(user)
	return json.Marshal(cfg)
}
