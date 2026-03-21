package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/packalares/packalares/internal/auth"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("packalares-auth starting")

	cfg, err := auth.LoadConfig()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	srv := auth.NewServer(cfg)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("shutting down")
		os.Exit(0)
	}()

	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
