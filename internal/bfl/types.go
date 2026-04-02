package bfl

import (
	"fmt"

	"github.com/packalares/packalares/pkg/config"
)

// ---------------------------------------------------------------------------
// Response envelope
// ---------------------------------------------------------------------------

type APIResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type ListResult struct {
	Items      any `json:"items"`
	TotalCount int `json:"totalItems"`
}

func NewListResult(items any, total int) ListResult {
	return ListResult{Items: items, TotalCount: total}
}

// ---------------------------------------------------------------------------
// Backend v1 types
// ---------------------------------------------------------------------------

type UserInfo struct {
	Name           string `json:"name"`
	OwnerRole      string `json:"owner_role"`
	TerminusName   string `json:"terminusName"`
	IsEphemeral    bool   `json:"is_ephemeral"`
	Zone           string `json:"zone"`
	CreatedUser    string `json:"created_user"`
	WizardComplete bool   `json:"wizard_complete"`
	AccessLevel    *int   `json:"access_level,omitempty"`
	ServerIP       string `json:"server_ip,omitempty"`
}

type TerminusInfo struct {
	TerminusName    string       `json:"terminusName"`
	WizardStatus    WizardStatus `json:"wizardStatus"`
	Selfhosted      bool         `json:"selfhosted"`
	TailScaleEnable bool         `json:"tailScaleEnable"`
	OsVersion       string       `json:"osVersion"`
	LoginBackground string       `json:"loginBackground"`
	Avatar          string       `json:"avatar"`
	TerminusID      string       `json:"terminusId"`
	UserDID         string       `json:"did"`
	ReverseProxy    string       `json:"reverseProxy"`
	Terminusd       string       `json:"terminusd"`
	Style           string       `json:"style"`
}

type OlaresInfo struct {
	OlaresID           string       `json:"olaresId"`
	WizardStatus       WizardStatus `json:"wizardStatus"`
	EnableReverseProxy bool         `json:"enableReverseProxy"`
	TailScaleEnable    bool         `json:"tailScaleEnable"`
	OsVersion          string       `json:"osVersion"`
	LoginBackground    string       `json:"loginBackground"`
	Avatar             string       `json:"avatar"`
	ID                 string       `json:"id"`
	UserDID            string       `json:"did"`
	Olaresd            string       `json:"olaresd"`
	Style              string       `json:"style"`
}

type MyAppsParam struct {
	IsLocal bool `json:"isLocal"`
}

// ---------------------------------------------------------------------------
// IAM v1alpha1 types
// ---------------------------------------------------------------------------

type IAMUserInfo struct {
	UID               string   `json:"uid"`
	Name              string   `json:"name"`
	DisplayName       string   `json:"display_name"`
	Description       string   `json:"description"`
	Email             string   `json:"email"`
	State             string   `json:"state"`
	LastLoginTime     *int64   `json:"last_login_time"`
	CreationTimestamp int64    `json:"creation_timestamp"`
	Avatar            string   `json:"avatar"`
	TerminusName      string   `json:"terminusName"`
	WizardComplete    bool     `json:"wizard_complete"`
	Roles             []string `json:"roles"`
	MemoryLimit       string   `json:"memory_limit"`
	CpuLimit          string   `json:"cpu_limit"`
}

type PasswordReset struct {
	CurrentPassword string `json:"current_password"`
	Password        string `json:"password"`
}

// ---------------------------------------------------------------------------
// Settings v1alpha1 types
// ---------------------------------------------------------------------------

type PostTerminusName struct {
	JWSSignature string `json:"jws_signature"`
	DID          string `json:"did"`
}

type ActivateRequest struct {
	Language string `json:"language"`
	Location string `json:"location"`
	Theme    string `json:"theme"`
}

type LauncherAccessPolicy struct {
	AccessLevel uint64   `json:"access_level"`
	AuthPolicy  string   `json:"auth_policy"`
	AllowCIDRs  []string `json:"allow_cidrs,omitempty"`
}

type PublicDomainAccessPolicy struct {
	DenyAll int `json:"deny_all"`
}

// ---------------------------------------------------------------------------
// Cluster metrics
// ---------------------------------------------------------------------------

type MetricV struct {
	Usage float64 `json:"usage"`
	Total float64 `json:"total"`
	Ratio float64 `json:"ratio"`
	Unit  string  `json:"unit"`
}

type NetMetric struct {
	Transmitted float64 `json:"transmitted"`
	Received    float64 `json:"received"`
}

type ClusterMetrics struct {
	CPU    MetricV   `json:"cpu"`
	Memory MetricV   `json:"memory"`
	Disk   MetricV   `json:"disk"`
	Net    NetMetric `json:"net"`
}

// ---------------------------------------------------------------------------
// Wizard status state machine
// ---------------------------------------------------------------------------

type WizardStatus string

const (
	WaitActivateVault     WizardStatus = "wait_activate_vault"
	WaitActivateSystem    WizardStatus = "wait_activate_system"
	SystemActivating      WizardStatus = "system_activating"
	WaitActivateNetwork   WizardStatus = "wait_activate_network"
	SystemActivateFailed  WizardStatus = "system_activate_failed"
	NetworkActivating     WizardStatus = "network_activating"
	NetworkActivateFailed WizardStatus = "network_activate_failed"
	WaitResetPassword     WizardStatus = "wait_reset_password"
	Completed             WizardStatus = "completed"
)

// ---------------------------------------------------------------------------
// Terminus name helpers (user@domain format)
// ---------------------------------------------------------------------------

type TerminusName string

func NewTerminusName(user, domain string) TerminusName {
	return TerminusName(fmt.Sprintf("%s@%s", user, domain))
}

func (t TerminusName) UserName() string {
	for i, c := range string(t) {
		if c == '@' {
			return string(t[:i])
		}
	}
	return string(t)
}

func (t TerminusName) Domain() string {
	for i, c := range string(t) {
		if c == '@' {
			return string(t[i+1:])
		}
	}
	return ""
}

// UserZone returns user.domain (the zone used for DNS / nginx server_name).
func (t TerminusName) UserZone() string {
	u := t.UserName()
	d := t.Domain()
	if d == "" {
		return u
	}
	return u + "." + d
}

// ---------------------------------------------------------------------------
// Annotation keys
// ---------------------------------------------------------------------------

var AnnotationGroup = config.APIGroup()

func anno(key string) string { return AnnotationGroup + "/" + key }

var (
	AnnoTerminusName      = anno("terminus-name")
	AnnoZone              = anno("zone")
	AnnoOwnerRole         = anno("owner-role")
	AnnoIsEphemeral       = anno("is-ephemeral")
	AnnoCreator           = anno("creator")
	AnnoWizardStatus      = anno("wizard-status")
	AnnoWizardError       = anno("wizard-error")
	AnnoAccessLevel       = anno("launcher-access-level")
	AnnoAllowCIDR         = anno("launcher-allow-cidr")
	AnnoAuthPolicy        = anno("launcher-auth-policy")
	AnnoDenyAll           = anno("deny-all")
	AnnoPublicDomainIP    = anno("public-domain-ip")
	AnnoLocalDomainIP     = anno("local-domain-ip")
	AnnoLocalDNSRecord    = anno("local-domain-dns-record")
	AnnoReverseProxyType  = anno("reverse-proxy-type")
	AnnoLanguage          = anno("language")
	AnnoLocation          = anno("location")
	AnnoTheme             = anno("theme")
	AnnoAvatar            = anno("avatar")
	AnnoLoginBackground   = anno("login-background")
	AnnoLoginBGStyle      = anno("login-background-style")
	AnnoUserDID           = anno("user-did")
	AnnoCPULimit          = anno("user-cpu-limit")
	AnnoMemoryLimit       = anno("user-memory-limit")
	AnnoJWSToken          = anno("jws-token-signature")
	AnnoCertManagerDID    = anno("user-did")
	AnnoAllowedDomainTS   = anno("deny-all-public-update")
	AnnoNatGatewayIP      = anno("nat-gateway-ip")
	AnnoTaskEnableSSL     = anno("task-enable-ssl")
)

// ---------------------------------------------------------------------------
// Network / Tailscale types
// ---------------------------------------------------------------------------

// TailscalePeer represents a single peer in the Tailscale network.
type TailscalePeer struct {
	Name     string `json:"name"`
	IP       string `json:"ip"`
	Online   bool   `json:"online"`
	LastSeen string `json:"lastSeen"`
}

// TailscaleStatusResponse is the API response for /bfl/backend/v1/tailscale/status.
type TailscaleStatusResponse struct {
	Enabled      bool            `json:"enabled"`
	Connected    bool            `json:"connected"`
	IP           string          `json:"ip"`
	Hostname     string          `json:"hostname"`
	Peers        []TailscalePeer `json:"peers"`
	AcceptRoutes bool            `json:"acceptRoutes"`
}

// NetworkInfoResponse is the API response for /bfl/backend/v1/network/info.
type NetworkInfoResponse struct {
	ServerIP     string   `json:"serverIP"`
	VPNIP        string   `json:"vpnIP"`
	TailscaleIP  string   `json:"tailscaleIP"` // backward compat
	Zone         string   `json:"zone"`
	CustomDomain string   `json:"customDomain"`
	CertSANs     []string `json:"certSANs"`
	CertExpiry   string   `json:"certExpiry"`
}

// CustomDomainRequest is the POST body for /bfl/backend/v1/network/domain.
type CustomDomainRequest struct {
	Domain string `json:"domain"`
}

const (
	RoleOwner = "owner"
	RoleAdmin = "admin"

	DefaultAuthPolicy = "two_factor"

	SSLSecretName            = "zone-ssl-config"
	ReverseProxyConfigMap    = "reverse-proxy-config"
	DefaultUserNamespacePrefix = "user-space"
)
