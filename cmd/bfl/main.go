package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/packalares/packalares/internal/bfl"
	"k8s.io/klog/v2"
)

func main() {
	klog.InitFlags(nil)

	listenAddr := os.Getenv("BFL_LISTEN_ADDR")
	if listenAddr == "" {
		listenAddr = ":8080"
	}

	server, err := bfl.NewServer(listenAddr)
	if err != nil {
		klog.Fatalf("failed to create BFL server: %v", err)
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

	if err := server.Run(ctx); err != nil {
		klog.Fatalf("BFL server error: %v", err)
	}
}
