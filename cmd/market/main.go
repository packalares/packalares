package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/packalares/packalares/internal/market"
	"k8s.io/klog/v2"
)

func main() {
	klog.InitFlags(nil)
	flag.Parse()

	cfg := market.DefaultConfig()

	klog.Infof("starting market backend listen=%s market=%s", cfg.ListenAddr, cfg.MarketURL)

	catalog := market.NewCatalog(cfg.MarketURL, cfg.CatalogPath)
	if err := catalog.Load(); err != nil {
		klog.Warningf("initial catalog load: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start background refresh
	done := make(chan struct{})
	go catalog.StartRefreshLoop(done)

	// Set up the chart sync manager with the Olares source
	dataDir := os.Getenv("MARKET_DATA_DIR")
	if dataDir == "" {
		dataDir = "/data/market"
	}
	syncMgr := market.NewChartSyncManager(dataDir, catalog)
	syncMgr.RegisterSource(market.NewOlaresSource())

	handler := market.NewHandler(catalog)
	handler.SetSyncManager(syncMgr)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	server := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigCh
		klog.Info("shutting down market backend")
		cancel()
		close(done)
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	_ = ctx // used in shutdown

	klog.Infof("market backend listening on %s", cfg.ListenAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		klog.Fatalf("market server: %v", err)
	}
}
