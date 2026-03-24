package main

import (
	"context"
	"log"
	"os"

	"github.com/packalares/packalares/internal/tapr"
)

func main() {
	listenAddr := os.Getenv("LISTEN_ADDR")
	if listenAddr == "" {
		listenAddr = ":8080"
	}

	log.Println("tapr: secrets gateway starting")

	ctx := context.Background()
	if err := tapr.SeedAndStart(ctx, listenAddr, nil); err != nil {
		log.Fatalf("tapr: %v", err)
	}
}
