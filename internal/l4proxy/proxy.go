package l4proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	ctrl "k8s.io/client-go/rest"
)

var iamUserGVR = schema.GroupVersionResource{
	Group:    "iam.kubesphere.io",
	Version:  "v1alpha2",
	Resource: "users",
}

// Proxy is the L4 TLS SNI-based proxy server.
type Proxy struct {
	cfg    Cfg
	client dynamic.Interface

	mu    sync.RWMutex
	users Users
}

// NewProxy creates a new L4 proxy.
func NewProxy(cfg Cfg) (*Proxy, error) {
	config, err := ctrl.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("in-cluster config: %w", err)
	}
	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("dynamic client: %w", err)
	}
	if cfg.LocalDomain == "" {
		cfg.LocalDomain = os.Getenv("OLARES_LOCAL_DOMAIN")
		if cfg.LocalDomain == "" {
			cfg.LocalDomain = defaultLocalDomain()
		}
	}
	if cfg.UserNSPrefix == "" {
		cfg.UserNSPrefix = "user-space"
	}
	if cfg.BFLServicePort == 0 {
		cfg.BFLServicePort = 443 // BFL ingress HTTPS port
	}
	if cfg.ListenPort == 0 {
		cfg.ListenPort = 443
	}

	return &Proxy{
		cfg:    cfg,
		client: dynClient,
	}, nil
}

// Run starts the proxy server and user watcher.
func (p *Proxy) Run(ctx context.Context) error {
	// Initial user load
	if err := p.refreshUsers(ctx); err != nil {
		klog.Warningf("initial user load failed: %v", err)
	}

	// Watch for user changes
	go p.watchUsers(ctx)

	// Periodic refresh as backup
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := p.refreshUsers(ctx); err != nil {
					klog.V(2).Infof("periodic user refresh: %v", err)
				}
			}
		}
	}()

	// Start TCP listener
	addr := fmt.Sprintf(":%d", p.cfg.ListenPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}
	defer ln.Close()
	klog.Infof("L4 proxy listening on %s", addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				klog.Errorf("accept: %v", err)
				continue
			}
		}
		go p.handleConn(conn)
	}
}

// handleConn processes a single incoming TLS connection.
func (p *Proxy) handleConn(clientConn net.Conn) {
	defer clientConn.Close()

	// Set read deadline for SNI extraction
	clientConn.SetReadDeadline(time.Now().Add(10 * time.Second))

	serverName, helloBytes, err := readClientHello(clientConn)
	if err != nil {
		klog.V(2).Infof("read client hello from %s: %v", clientConn.RemoteAddr(), err)
		return
	}

	// Clear deadline for proxy phase
	clientConn.SetReadDeadline(time.Time{})

	remoteAddr := clientConn.RemoteAddr().(*net.TCPAddr).IP.String()

	p.mu.RLock()
	users := p.users
	p.mu.RUnlock()

	var matchedUser *User

	if serverName == "" {
		// No SNI — IP access fallback: route to default (first) user
		klog.Infof("no SNI from %s, using default user", remoteAddr)
		if len(users) > 0 {
			matchedUser = &users[0]
		}
	} else {
		klog.V(2).Infof("SNI=%s from %s", serverName, remoteAddr)
		matchedUser = matchUser(serverName, users)

		// Fallback to default user for single-user setups
		if matchedUser == nil && len(users) > 0 {
			klog.Infof("no match for %s, falling back to default user", serverName)
			matchedUser = &users[0]
		}
	}

	if matchedUser == nil {
		klog.Warningf("no users configured, dropping connection from %s", remoteAddr)
		return
	}

	// Deny filter
	if !denyFilter(matchedUser, serverName, remoteAddr) {
		klog.Infof("denied %s for %s", remoteAddr, serverName)
		return
	}

	// Connect to upstream BFL ingress
	upstream := fmt.Sprintf("%s:%d", matchedUser.BFLIngressSvcHost, matchedUser.BFLIngressSvcPort)
	upstreamConn, err := net.DialTimeout("tcp", upstream, 10*time.Second)
	if err != nil {
		klog.Errorf("connect upstream %s: %v", upstream, err)
		return
	}
	defer upstreamConn.Close()

	// Replay the ClientHello
	if _, err := upstreamConn.Write(helloBytes); err != nil {
		klog.Errorf("write hello to upstream: %v", err)
		return
	}

	// Bidirectional proxy
	done := make(chan struct{}, 2)
	go func() {
		io.Copy(upstreamConn, clientConn)
		done <- struct{}{}
	}()
	go func() {
		io.Copy(clientConn, upstreamConn)
		done <- struct{}{}
	}()
	<-done
}

// ---------------------------------------------------------------------------
// User matching (port of server.lua match_user)
// ---------------------------------------------------------------------------

func matchUser(serverName string, users Users) *User {
	// Separate ephemeral and admin users
	var ephemeral, admin []User
	for _, u := range users {
		if u.IsEphemeral == "yes" {
			ephemeral = append(ephemeral, u)
		} else {
			admin = append(admin, u)
		}
	}

	// Match ephemeral users: pattern {appid}-{username}.{zone}
	for i := range ephemeral {
		u := &ephemeral[i]
		pattern := fmt.Sprintf(`^[A-Za-z0-9]+-%s\.%s$`, regexp.QuoteMeta(u.Name), regexp.QuoteMeta(u.Zone))
		if matched, _ := regexp.MatchString(pattern, serverName); matched {
			return u
		}
		// Also check local domain
		ldomain := u.LocalDomain
		if ldomain == "" {
			ldomain = defaultLocalDomain()
		}
		pattern = fmt.Sprintf(`^[A-Za-z0-9]+-%s\.%s$`, regexp.QuoteMeta(u.Name), regexp.QuoteMeta(ldomain))
		if matched, _ := regexp.MatchString(pattern, serverName); matched {
			return u
		}
	}

	// Match admin users against their ngx_server_name_domains
	for i := range admin {
		u := &admin[i]
		for _, domain := range u.NgxServerNameDomains {
			if serverName == domain {
				return u
			}
			// Match subdomains: {prefix}.{domain}
			escaped := regexp.QuoteMeta(domain)
			pattern := fmt.Sprintf(`^[a-zA-Z0-9-]+\.?%s$`, escaped)
			if matched, _ := regexp.MatchString(pattern, serverName); matched {
				return u
			}
		}
	}

	return nil
}

// denyFilter implements the deny-all logic from server.lua.
func denyFilter(user *User, serverName, remoteAddr string) bool {
	// Always allow local domain IP
	if remoteAddr == user.LocalDomainIP {
		return true
	}

	// Always allow VPN CIDR
	vpnCIDR := "100.64.0.0/10"
	_, vpnNet, _ := net.ParseCIDR(vpnCIDR)
	if vpnNet != nil && vpnNet.Contains(net.ParseIP(remoteAddr)) {
		return true
	}

	// Always allow private ranges
	privateRanges := []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "127.0.0.0/8"}
	for _, cidr := range privateRanges {
		_, network, _ := net.ParseCIDR(cidr)
		if network != nil && network.Contains(net.ParseIP(remoteAddr)) {
			return true
		}
	}

	if user.DenyAll == 0 {
		return true
	}

	// Check allowed domains
	for _, d := range user.AllowedDomains {
		if d == serverName {
			return true
		}
	}

	return false
}

// ---------------------------------------------------------------------------
// User discovery from Kubernetes
// ---------------------------------------------------------------------------

func (p *Proxy) refreshUsers(ctx context.Context) error {
	users, err := p.listUsers(ctx)
	if err != nil {
		return err
	}

	p.mu.Lock()
	p.users = users
	p.mu.Unlock()

	klog.V(2).Infof("refreshed %d users", len(users))
	return nil
}

func (p *Proxy) listUsers(ctx context.Context) (Users, error) {
	list, err := p.client.Resource(iamUserGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}

	users := make(Users, 0)

	for _, item := range list.Items {
		user := &item
		isEphemeralAnno := getUserAnno(user, annoIsEphemeral)

		did := getUserAnno(user, annoDID)
		zone := getUserAnno(user, annoZone)

		if did == "" && zone == "" && isEphemeralAnno == "" {
			continue
		}

		isEphemeralDomain := "no"
		if ok, err := strconv.ParseBool(isEphemeralAnno); err == nil && ok {
			isEphemeralDomain = "yes"
		}

		var (
			accLevel    string
			allowCIDR   string
			denyAll     string
			localDNSIP  string
			serverNames []string
		)

		if isEphemeralDomain == "no" {
			accLevel = getUserAnno(user, annoAccessLevel)
			allowCIDR = getUserAnno(user, annoAllowCIDR)
			denyAll = getUserAnno(user, annoDenyAll)
			localDNSIP = getUserAnno(user, annoLocalDNS)
			serverNames = []string{zone, fmt.Sprintf("%s.%s", user.GetName(), p.cfg.LocalDomain)}
		} else {
			// Ephemeral user — inherit from creator
			creator := getUserAnno(user, annoCreator)
			ownerUser := p.findUser(list, creator)
			if ownerUser == nil {
				continue
			}
			did = getUserAnno(ownerUser, annoDID)
			zone = getUserAnno(ownerUser, annoZone)
			accLevel = getUserAnno(ownerUser, annoAccessLevel)
			allowCIDR = getUserAnno(ownerUser, annoAllowCIDR)
			denyAll = getUserAnno(ownerUser, annoDenyAll)
		}

		accessLevel, _ := strconv.ParseUint(accLevel, 10, 64)
		denyAllInt, _ := strconv.Atoi(denyAll)

		// Resolve BFL ingress service host
		svcName := bflServiceName(p.cfg.UserNSPrefix, user.GetName())
		addr, err := lookupHost(svcName)
		if err != nil {
			klog.V(2).Infof("lookup %s: %v", svcName, err)
			continue
		}

		// Build allowed domains list
		allowedDomains := []string{zone} // always include zone
		if denyAll == "1" {
			// Add zone itself, any public apps would be added dynamically
		}

		u := User{
			Name:                 user.GetName(),
			Namespace:            fmt.Sprintf("%s-%s", p.cfg.UserNSPrefix, user.GetName()),
			BFLIngressSvcHost:    addr,
			BFLIngressSvcPort:    p.cfg.BFLServicePort,
			DID:                  did,
			Zone:                 zone,
			IsEphemeral:          isEphemeralDomain,
			NgxServerNameDomains: serverNames,
			AccessLevel:          accessLevel,
			AllowCIDRs:           strings.Split(allowCIDR, ","),
			DenyAll:              denyAllInt,
			AllowedDomains:       allowedDomains,
			CreateTimestamp:      user.GetCreationTimestamp().Unix(),
			LocalDomainIP:        localDNSIP,
			LocalDomain:          p.cfg.LocalDomain,
		}
		users = append(users, u)
	}

	sort.Sort(users)
	return users, nil
}

func (p *Proxy) findUser(list *unstructured.UnstructuredList, name string) *unstructured.Unstructured {
	for i := range list.Items {
		if list.Items[i].GetName() == name {
			return &list.Items[i]
		}
		// "cli" creator means owner
		if name == "cli" && getUserAnno(&list.Items[i], annoOwnerRole) == "owner" {
			return &list.Items[i]
		}
	}
	return nil
}

func getUserAnno(user *unstructured.Unstructured, key string) string {
	annos := user.GetAnnotations()
	if annos == nil {
		return ""
	}
	return annos[key]
}

func lookupHost(svc string) (string, error) {
	for retry := 0; retry < 5; retry++ {
		addrs, err := net.LookupHost(svc)
		if err == nil && len(addrs) > 0 {
			return addrs[0], nil
		}
		time.Sleep(time.Second)
	}
	return "", fmt.Errorf("no host lookup for %s", svc)
}

// ---------------------------------------------------------------------------
// Watch users
// ---------------------------------------------------------------------------

func (p *Proxy) watchUsers(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		w, err := p.client.Resource(iamUserGVR).Watch(ctx, metav1.ListOptions{})
		if err != nil {
			klog.Warningf("watch users: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		func() {
			defer w.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case event, ok := <-w.ResultChan():
					if !ok {
						return
					}
					switch event.Type {
					case watch.Added, watch.Modified, watch.Deleted:
						if err := p.refreshUsers(ctx); err != nil {
							klog.V(2).Infof("refresh after watch event: %v", err)
						}
					}
				}
			}
		}()

		time.Sleep(time.Second)
	}
}

// Ensure bytes import is used (for the io.Copy in handleConn via the buffered reader).
var _ = bytes.NewReader
