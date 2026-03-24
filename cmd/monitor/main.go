package main

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"net/http"
	"os"
	"time"

	"github.com/packalares/packalares/internal/monitor"
	"github.com/packalares/packalares/pkg/config"
	"github.com/packalares/packalares/pkg/secrets"
	"github.com/redis/go-redis/v9"
)

func main() {
	secrets.MustLoadSecrets()
	prometheusURL := envOr("PROMETHEUS_URL", config.PrometheusURL())
	port := envOr("PORT", "8000")

	handler := monitor.NewHandler(prometheusURL)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Health check
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Start metrics pusher to KVRocks
	go startMetricsPublisher()

	addr := ":" + port
	log.Printf("monitoring-server starting on %s (prometheus: %s)", addr, prometheusURL)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

func startMetricsPublisher() {
	redisAddr := config.KVRocksHost() + ":" + config.KVRocksPort()
	redisPass := os.Getenv("REDIS_PASSWORD")

	rdb := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: redisPass,
	})

	ctx := context.Background()

	// Wait for KVRocks to be ready
	for i := 0; i < 30; i++ {
		if err := rdb.Ping(ctx).Err(); err == nil {
			break
		}
		time.Sleep(2 * time.Second)
	}

	log.Printf("metrics publisher started (5s interval, redis: %s)", redisAddr)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		metrics, err := monitor.CollectSystemMetrics()
		if err != nil {
			continue
		}
		metrics.CPUUsage = math.Round(metrics.CPUUsage*10) / 10
		metrics.Uptime = math.Round(metrics.Uptime)

		data, err := json.Marshal(metrics)
		if err != nil {
			continue
		}
		// SET with 15s TTL — if monitoring-server dies, stale data expires
		rdb.Set(ctx, "packalares:metrics", data, 15*time.Second)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
