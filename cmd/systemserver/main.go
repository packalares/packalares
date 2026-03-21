package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/packalares/packalares/internal/systemserver"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("packalares-system-server starting")

	cfg, err := systemserver.LoadConfig()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	srv, err := systemserver.NewServer(cfg)
	if err != nil {
		log.Fatalf("create server: %v", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("shutting down")
		srv.Stop()
		os.Exit(0)
	}()

	if err := srv.Run(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
