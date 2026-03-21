package main

import (
	"log"
	"context"
	"os"

	"github.com/packalares/packalares/internal/kubesphere"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":9090"
	}
	config, err := rest.InClusterConfig()
	if err != nil {
		kc := os.Getenv("KUBECONFIG")
		if kc == "" {
			kc = os.Getenv("HOME") + "/.kube/config"
		}
		config, err = clientcmd.BuildConfigFromFlags("", kc)
		if err != nil {
			log.Fatalf("Failed to get k8s config: %v", err)
		}
	}

	srv, err := kubesphere.NewServer(config, addr)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	log.Printf("Starting KubeSphere API server on %s", addr)
	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		log.Fatal(err)
	}
}
