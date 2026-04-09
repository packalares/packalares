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
	lokiURL := envOr("LOKI_URL", "http://loki-svc."+config.MonitoringNamespace()+":3100")
	port := envOr("PORT", "8000")

	handler := monitor.NewHandler(prometheusURL, lokiURL)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Health check
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Start metrics pusher to KVRocks
	go startMetricsPublisher(prometheusURL)

	// Start weather fetcher (IP geolocation + Open-Meteo, 30min refresh)
	monitor.StartWeatherLoop()

	addr := ":" + port
	log.Printf("monitoring-server starting on %s (prometheus: %s)", addr, prometheusURL)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

func startMetricsPublisher(prometheusURL string) {
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
		} else if i == 29 {
			log.Printf("warning: KVRocks not reachable after 60s: %v", err)
		}
		time.Sleep(2 * time.Second)
	}

	if redisPass == "" {
		log.Printf("warning: REDIS_PASSWORD is empty — KVRocks may reject writes")
	}
	log.Printf("metrics publisher started (5s interval, redis: %s)", redisAddr)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		metrics, err := monitor.CollectSystemMetrics(prometheusURL)
		if err != nil {
			continue
		}
		metrics.CPUUsage = math.Round(metrics.CPUUsage*10) / 10
		metrics.Uptime = math.Round(metrics.Uptime)

		// Update Prometheus exporter cache
		monitor.UpdateLatestMetrics(metrics)

		data, err := json.Marshal(metrics)
		if err != nil {
			continue
		}
		// SET with 15s TTL — if monitoring-server dies, stale data expires
		rdb.Set(ctx, "packalares:metrics", data, 15*time.Second)

		// Weather (cached, updates every 30min — just write latest to KVRocks)
		if w := monitor.GetWeather(); w != nil {
			if wData, err := json.Marshal(w); err == nil {
				rdb.Set(ctx, "packalares:weather", wData, 35*time.Minute)
			}
		}
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
