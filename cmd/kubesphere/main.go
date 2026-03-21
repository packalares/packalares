package main

import (
	"log"
	"os"

	"github.com/packalares/packalares/internal/kubesphere"
)

func main() {
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":9090"
	}
	log.Printf("Starting KubeSphere API server on %s", addr)
	srv := kubesphere.NewServer()
	if err := srv.ListenAndServe(addr); err != nil {
		log.Fatal(err)
	}
}
