package appservice

import (
	"time"
)

// ApplicationManagerState represents the lifecycle state of an app.
type ApplicationManagerState string

const (
	StatePending        ApplicationManagerState = "pending"
	StateDownloading    ApplicationManagerState = "downloading"
	StateInstalling     ApplicationManagerState = "installing"
	StateInitializing   ApplicationManagerState = "initializing"
	StateRunning        ApplicationManagerState = "running"
	StateUpgrading      ApplicationManagerState = "upgrading"
	StateStopping       ApplicationManagerState = "stopping"
	StateStopped        ApplicationManagerState = "stopped"
	StateResuming       ApplicationManagerState = "resuming"
	StateUninstalling   ApplicationManagerState = "uninstalling"
	StateUninstalled    ApplicationManagerState = "uninstalled"
	StateInstallFailed  ApplicationManagerState = "installFailed"
	StateUninstallFailed ApplicationManagerState = "uninstallFailed"
	StateResumeFailed   ApplicationManagerState = "resumeFailed"
	StateUpgradeFailed  ApplicationManagerState = "upgradeFailed"
	StateStopFailed     ApplicationManagerState = "stopFailed"
	StateFailed         ApplicationManagerState = "failed"
)

func (s ApplicationManagerState) String() string { return string(s) }

// OpType represents the operation being performed.
type OpType string

const (
	OpInstall   OpType = "install"
	OpUninstall OpType = "uninstall"
	OpUpgrade   OpType = "upgrade"
	OpStop      OpType = "stop"
	OpResume    OpType = "resume"
	OpCancel    OpType = "cancel"
)

// AppSource describes where an app comes from.
type AppSource string

const (
	SourceMarket  AppSource = "market"
	SourceCustom  AppSource = "custom"
	SourceDevBox  AppSource = "devbox"
	SourceSystem  AppSource = "system"
	SourceUnknown AppSource = "unknown"
)

// Entrance describes a user-facing entry point for an app.
type Entrance struct {
	Name       string `json:"name" yaml:"name"`
	Host       string `json:"host" yaml:"host"`
	Port       int32  `json:"port" yaml:"port"`
	Icon       string `json:"icon,omitempty" yaml:"icon,omitempty"`
	Title      string `json:"title,omitempty" yaml:"title,omitempty"`
	AuthLevel  string `json:"authLevel,omitempty" yaml:"authLevel,omitempty"`
	Invisible  bool   `json:"invisible,omitempty" yaml:"invisible,omitempty"`
	URL        string `json:"url,omitempty" yaml:"url,omitempty"`
	OpenMethod string `json:"openMethod,omitempty" yaml:"openMethod,omitempty"`
}

// SharedEntrance describes a shared API endpoint exposed by a provider app.
type SharedEntrance struct {
	Name      string `json:"name" yaml:"name"`
	Host      string `json:"host" yaml:"host"`
	Port      int32  `json:"port" yaml:"port"`
	Title     string `json:"title,omitempty" yaml:"title,omitempty"`
	Icon      string `json:"icon,omitempty" yaml:"icon,omitempty"`
	AuthLevel string `json:"authLevel,omitempty" yaml:"authLevel,omitempty"`
	Invisible bool   `json:"invisible,omitempty" yaml:"invisible,omitempty"`
}

// ServicePort defines an exposed port for an app.
type ServicePort struct {
	Name       string `json:"name" yaml:"name"`
	Host       string `json:"host" yaml:"host"`
	Port       int32  `json:"port" yaml:"port"`
	ExposePort int32  `json:"exposePort,omitempty" yaml:"exposePort,omitempty"`
	Protocol   string `json:"protocol,omitempty" yaml:"protocol,omitempty"`
}

// AppConfiguration is the Olares-compatible app manifest (OlaresManifest.yaml).
type AppConfiguration struct {
	ConfigVersion   string           `json:"olaresManifest.version" yaml:"olaresManifest.version"`
	ConfigType      string           `json:"olaresManifest.type" yaml:"olaresManifest.type"`
	APIVersion      string           `json:"apiVersion" yaml:"apiVersion"`
	Metadata        AppMetaData      `json:"metadata" yaml:"metadata"`
	Entrances       []Entrance       `json:"entrances" yaml:"entrances"`
	SharedEntrances []SharedEntrance `json:"sharedEntrances,omitempty" yaml:"sharedEntrances,omitempty"`
	Ports           []ServicePort    `json:"ports" yaml:"ports"`
	Spec            AppSpec          `json:"spec" yaml:"spec"`
	Permission      Permission       `json:"permission" yaml:"permission"`
	Options         Options          `json:"options" yaml:"options"`
	Middleware      *MiddlewareSpec  `json:"middleware,omitempty" yaml:"middleware,omitempty"`
}

// MiddlewareSpec declares middleware dependencies for an app.
type MiddlewareSpec struct {
	Postgres *MiddlewarePostgres `json:"postgres,omitempty" yaml:"postgres,omitempty"`
	Redis    *MiddlewareRedis    `json:"redis,omitempty" yaml:"redis,omitempty"`
	MongoDB  *MiddlewareMongoDB  `json:"mongodb,omitempty" yaml:"mongodb,omitempty"`
}

// MiddlewarePostgres declares PostgreSQL database requirements.
type MiddlewarePostgres struct {
	Username  string              `json:"username,omitempty" yaml:"username,omitempty"`
	Password  string              `json:"password,omitempty" yaml:"password,omitempty"`
	Databases []MiddlewareDBEntry `json:"databases,omitempty" yaml:"databases,omitempty"`
}

// MiddlewareRedis declares Redis requirements.
type MiddlewareRedis struct {
	Namespace string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
}

// MiddlewareMongoDB declares MongoDB requirements.
type MiddlewareMongoDB struct {
	Username  string              `json:"username,omitempty" yaml:"username,omitempty"`
	Password  string              `json:"password,omitempty" yaml:"password,omitempty"`
	Databases []MiddlewareDBEntry `json:"databases,omitempty" yaml:"databases,omitempty"`
}

// MiddlewareDBEntry is a single database to provision.
type MiddlewareDBEntry struct {
	Name        string `json:"name" yaml:"name"`
	Distributed bool   `json:"distributed,omitempty" yaml:"distributed,omitempty"`
}

// AppMetaData holds descriptive metadata for an app chart.
type AppMetaData struct {
	Name        string   `json:"name" yaml:"name"`
	Icon        string   `json:"icon" yaml:"icon"`
	Description string   `json:"description" yaml:"description"`
	AppID       string   `json:"appid" yaml:"appid"`
	Title       string   `json:"title" yaml:"title"`
	Version     string   `json:"version" yaml:"version"`
	Categories  []string `json:"categories" yaml:"categories"`
	Rating      float32  `json:"rating" yaml:"rating"`
	Target      string   `json:"target" yaml:"target"`
	Type        string   `json:"type" yaml:"type"`
}

// AppSpec holds detailed specification fields.
type AppSpec struct {
	Namespace      string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	OnlyAdmin      bool   `json:"onlyAdmin,omitempty" yaml:"onlyAdmin,omitempty"`
	VersionName    string `json:"versionName" yaml:"versionName"`
	FullDescription string `json:"fullDescription" yaml:"fullDescription"`
	Developer      string `json:"developer" yaml:"developer"`
	RequiredMemory string `json:"requiredMemory" yaml:"requiredMemory"`
	RequiredDisk   string `json:"requiredDisk" yaml:"requiredDisk"`
	RequiredGPU    string `json:"requiredGpu" yaml:"requiredGpu"`
	RequiredCPU    string `json:"requiredCpu" yaml:"requiredCpu"`
	LimitedMemory  string `json:"limitedMemory" yaml:"limitedMemory"`
	LimitedCPU     string `json:"limitedCPU" yaml:"limitedCPU"`
}

// Permission describes what an app needs access to.
type Permission struct {
	AppData  bool           `json:"appData" yaml:"appData"`
	AppCache bool           `json:"appCache" yaml:"appCache"`
	UserData []string       `json:"userData" yaml:"userData"`
	SysData  []SysDataEntry `json:"sysData,omitempty" yaml:"sysData,omitempty"`
	Provider []ProviderEntry `json:"provider,omitempty" yaml:"provider,omitempty"`
}

// SysDataEntry describes a system data dependency (e.g. consuming an API provider).
type SysDataEntry struct {
	DataType string   `json:"dataType" yaml:"dataType"`
	AppName  string   `json:"appName" yaml:"appName"`
	Svc      string   `json:"svc" yaml:"svc"`
	Port     int      `json:"port" yaml:"port"`
	Group    string   `json:"group" yaml:"group"`
	Version  string   `json:"version" yaml:"version"`
	Ops      []string `json:"ops" yaml:"ops"`
}

// ProviderEntry describes a provider dependency.
type ProviderEntry struct {
	AppName      string `json:"appName" yaml:"appName"`
	ProviderName string `json:"providerName" yaml:"providerName"`
}

// Options holds optional app behaviour flags.
type Options struct {
	MobileSupported bool         `json:"mobileSupported" yaml:"mobileSupported"`
	Dependencies    []Dependency `json:"dependencies" yaml:"dependencies"`
	Conflicts       []Conflict   `json:"conflicts" yaml:"conflicts"`
}

// Dependency on another app or system component.
type Dependency struct {
	Name    string `json:"name" yaml:"name"`
	Version string `json:"version" yaml:"version"`
	Type    string `json:"type" yaml:"type"`
}

// Conflict with another app.
type Conflict struct {
	Name string `json:"name" yaml:"name"`
	Type string `json:"type" yaml:"type"`
}

// --- API request/response types (Olares-compatible) ---

// Response is the base response envelope.
type Response struct {
	Code int32 `json:"code"`
}

// InstallRequest is the body for POST /app-service/v1/install.
type InstallRequest struct {
	Name    string            `json:"name"`
	RepoURL string            `json:"repoUrl"`
	Source  AppSource         `json:"source"`
	Version string            `json:"version,omitempty"`
	Values  map[string]string `json:"values,omitempty"`
}

// UninstallRequest is the body for POST /app-service/v1/uninstall.
type UninstallRequest struct {
	Name       string `json:"name"`
	All        bool   `json:"all"`
	DeleteData bool   `json:"deleteData"`
}

// SuspendRequest is the body for POST /app-service/v1/suspend.
type SuspendRequest struct {
	Name string `json:"name"`
	All  bool   `json:"all"`
}

// ResumeRequest is the body for POST /app-service/v1/resume.
type ResumeRequest struct {
	Name string `json:"name"`
}

// InstallationResponse is returned after install/uninstall/suspend/resume.
type InstallationResponse struct {
	Response
	Data InstallationResponseData `json:"data"`
}

// InstallationResponseData carries the operation identifier.
type InstallationResponseData struct {
	UID  string `json:"uid"`
	OpID string `json:"opID"`
}

// AppInfo is the detailed information about an installed app.
type AppInfo struct {
	Name        string                  `json:"name"`
	AppID       string                  `json:"appID"`
	Namespace   string                  `json:"namespace"`
	Owner       string                  `json:"owner"`
	Icon        string                  `json:"icon,omitempty"`
	Title       string                  `json:"title,omitempty"`
	Description string                  `json:"description,omitempty"`
	Version     string                  `json:"version,omitempty"`
	State         ApplicationManagerState `json:"state"`
	StatusMessage string                  `json:"statusMessage,omitempty"`
	Source        string                  `json:"source"`
	Entrances     []Entrance              `json:"entrances,omitempty"`
	CreatedAt     time.Time               `json:"createdAt"`
	UpdatedAt     time.Time               `json:"updatedAt"`
}

// AppListResponse is returned for GET /apps.
type AppListResponse struct {
	Response
	Data []AppInfo `json:"data"`
}

// AppDetailResponse is returned for GET /app/{name}.
type AppDetailResponse struct {
	Response
	Data *AppInfo `json:"data"`
}

// PodInfo describes a running pod for an app.
type PodInfo struct {
	Name   string `json:"name"`
	Ready  string `json:"ready"`
	Status string `json:"status"`
	Age    string `json:"age"`
}
