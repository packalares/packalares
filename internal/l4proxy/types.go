package l4proxy

import "fmt"

const (
	annotationGroup = "bytetrade.io"

	annoZone         = annotationGroup + "/zone"
	annoDID          = annotationGroup + "/did"
	annoOwnerRole    = annotationGroup + "/owner-role"
	annoAccessLevel  = annotationGroup + "/launcher-access-level"
	annoAllowCIDR    = annotationGroup + "/launcher-allow-cidr"
	annoDenyAll      = annotationGroup + "/deny-all"
	annoIsEphemeral  = annotationGroup + "/is-ephemeral"
	annoCreator      = annotationGroup + "/creator"
	annoLocalDNS     = annotationGroup + "/local-domain-dns-record"
	annoAllowDomains = annotationGroup + "/allowed-domains"
)

// User represents a backend user for L4 routing.
type User struct {
	Name                 string   `json:"name"`
	Namespace            string   `json:"namespace"`
	DID                  string   `json:"did"`
	Zone                 string   `json:"zone"`
	IsEphemeral          string   `json:"is_ephemeral"`
	BFLIngressSvcHost    string   `json:"bfl_ingress_svc_host"`
	BFLIngressSvcPort    int      `json:"bfl_ingress_svc_port"`
	AccessLevel          uint64   `json:"access_level"`
	AllowCIDRs           []string `json:"allow_cidrs"`
	DenyAll              int      `json:"deny_all"`
	AllowedDomains       []string `json:"allowed_domains"`
	NgxServerNameDomains []string `json:"ngx_server_name_domains"`
	CreateTimestamp      int64    `json:"create_timestamp"`
	LocalDomainIP        string   `json:"local_domain_ip"`
	LocalDomain          string   `json:"local_domain"`
}

// Users is a sortable list of User (newest first).
type Users []User

func (u Users) Len() int           { return len(u) }
func (u Users) Less(i, j int) bool { return u[i].CreateTimestamp > u[j].CreateTimestamp }
func (u Users) Swap(i, j int)      { u[i], u[j] = u[j], u[i] }

// StreamServer represents a TCP/UDP port proxy entry.
type StreamServer struct {
	Protocol string `json:"protocol"`
	Port     int32  `json:"port"`
	BflHost  string `json:"bfl_host"`
}

// Cfg holds the L4 proxy configuration.
type Cfg struct {
	ListenPort     int
	BFLServicePort int
	UserNSPrefix   string
	LocalDomain    string
}

func defaultLocalDomain() string {
	return "olares.local"
}

func bflServiceName(userNSPrefix, userName string) string {
	return fmt.Sprintf("bfl.%s-%s", userNSPrefix, userName)
}
