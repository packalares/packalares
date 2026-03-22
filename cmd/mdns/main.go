package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/packalares/packalares/internal/mdns"
)

func main() {
	server, err := mdns.NewServer()
	if err != nil {
		log.Fatalf("Failed to start mDNS server: %v", err)
	}
	defer server.Close()

	log.Println("mDNS agent running")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("mDNS agent shutting down")
}
