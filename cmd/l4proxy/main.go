package main

import (
	"context"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/packalares/packalares/internal/l4proxy"
	"k8s.io/klog/v2"
)

func main() {
	klog.InitFlags(nil)

	listenPort := 443
	if v := os.Getenv("L4_PROXY_LISTEN_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			listenPort = p
		}
	}

	bflPort := 443
	if v := os.Getenv("BFL_INGRESS_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			bflPort = p
		}
	}

	userNSPrefix := os.Getenv("USER_NAMESPACE_PREFIX")
	if userNSPrefix == "" {
		userNSPrefix = "user-space"
	}

	localDomain := os.Getenv("OLARES_LOCAL_DOMAIN")
	if localDomain == "" {
		localDomain = "olares.local"
	}

	cfg := l4proxy.Cfg{
		ListenPort:     listenPort,
		BFLServicePort: bflPort,
		UserNSPrefix:   userNSPrefix,
		LocalDomain:    localDomain,
	}

	proxy, err := l4proxy.NewProxy(cfg)
	if err != nil {
		klog.Fatalf("failed to create L4 proxy: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		klog.Infof("received signal %v, shutting down", sig)
		cancel()
	}()

	if err := proxy.Run(ctx); err != nil {
		klog.Fatalf("L4 proxy error: %v", err)
	}
}
