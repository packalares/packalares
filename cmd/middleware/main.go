package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/packalares/packalares/internal/middleware"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("packalares-middleware-operator starting")

	cfg, err := middleware.LoadConfig()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	ctrl, err := middleware.NewController(cfg)
	if err != nil {
		log.Fatalf("create controller: %v", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("shutting down")
		ctrl.Stop()
	}()

	if err := ctrl.Run(); err != nil {
		log.Fatalf("controller error: %v", err)
	}
}
