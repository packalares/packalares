package appservice

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/packalares/packalares/pkg/config"
	"github.com/redis/go-redis/v9"
	"golang.org/x/net/websocket"
	"k8s.io/klog/v2"
)

const (
	sessionCookieName = "packalares_session"
)

// WSMessage is the envelope for all WebSocket messages sent to clients.
type WSMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// wsClient represents a single connected WebSocket client.
type wsClient struct {
	conn *websocket.Conn
	send chan []byte
}

// WSHub maintains the set of active WebSocket clients and broadcasts
// messages to all of them. It is safe for concurrent use.
type WSHub struct {
	mu      sync.RWMutex
	clients map[*wsClient]struct{}
}

// defaultHub is the package-level singleton hub that other parts of
// the codebase use to broadcast WebSocket notifications.
var defaultHub = &WSHub{
	clients: make(map[*wsClient]struct{}),
}

// GetWSHub returns the global WebSocket hub singleton.
func GetWSHub() *WSHub {
	return defaultHub
}

// register adds a client to the hub.
func (hub *WSHub) register(c *wsClient) {
	hub.mu.Lock()
	hub.clients[c] = struct{}{}
	hub.mu.Unlock()
	klog.V(2).Infof("ws: client registered (%d total)", hub.clientCount())
}

// unregister removes a client from the hub and closes its send channel.
func (hub *WSHub) unregister(c *wsClient) {
	hub.mu.Lock()
	if _, ok := hub.clients[c]; ok {
		delete(hub.clients, c)
		close(c.send)
	}
	hub.mu.Unlock()
	klog.V(2).Infof("ws: client unregistered (%d total)", hub.clientCount())
}

// clientCount returns the current number of connected clients.
func (hub *WSHub) clientCount() int {
	hub.mu.RLock()
	defer hub.mu.RUnlock()
	return len(hub.clients)
}

// Broadcast sends a message to every connected client. It is safe to
// call from any goroutine and will never block the caller for long;
// messages are dropped for clients whose send buffer is full.
func (hub *WSHub) Broadcast(msg WSMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		klog.Errorf("ws: marshal broadcast: %v", err)
		return
	}

	hub.mu.RLock()
	defer hub.mu.RUnlock()

	for c := range hub.clients {
		select {
		case c.send <- data:
		default:
			// Client too slow; drop the message to avoid blocking.
			klog.V(3).Infof("ws: dropping message for slow client")
		}
	}
}

// BroadcastAppState is a convenience wrapper that broadcasts an
// app_state notification for the given app name and state.
func (hub *WSHub) BroadcastAppState(name string, state ApplicationManagerState) {
	hub.Broadcast(WSMessage{
		Type: "app_state",
		Data: map[string]string{
			"name":  name,
			"state": string(state),
		},
	})
}

// BroadcastInstallProgress broadcasts detailed install progress.
func (hub *WSHub) BroadcastInstallProgress(name string, state ApplicationManagerState, step, totalSteps int, detail string, bytesDownloaded, bytesTotal int64) {
	hub.Broadcast(WSMessage{
		Type: "install_progress",
		Data: map[string]interface{}{
			"name":             name,
			"state":            string(state),
			"step":             step,
			"totalSteps":       totalSteps,
			"detail":           detail,
			"bytesDownloaded":  bytesDownloaded,
			"bytesTotal":       bytesTotal,
		},
	})
}

// BroadcastAlert broadcasts a system alert to all connected clients.
func (hub *WSHub) BroadcastAlert(level, message string) {
	hub.Broadcast(WSMessage{
		Type: "alert",
		Data: map[string]string{
			"level":   level,
			"message": message,
		},
	})
}

// StartMetricsPusher runs a background goroutine that reads system metrics
// from KVRocks (written by monitoring-server) and broadcasts to all connected
// WebSocket clients every 5 seconds.
func StartMetricsPusher() {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = config.KVRocksHost() + ":" + config.KVRocksPort()
	}
	redisPass := os.Getenv("REDIS_PASSWORD")

	rdb := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: redisPass,
	})

	ctx := context.Background()

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			hub := GetWSHub()
			if hub.clientCount() == 0 {
				continue
			}

			data, err := rdb.Get(ctx, "packalares:metrics").Bytes()
			if err != nil {
				continue
			}

			var metrics json.RawMessage = data
			hub.Broadcast(WSMessage{Type: "metrics", Data: metrics})
		}
	}()

	klog.Infof("ws: metrics pusher started (5s interval, reading from KVRocks)")
}

// verifySession checks the packalares_session cookie against the auth
// service. Returns nil if the session is valid, an error otherwise.
func verifySession(r *http.Request) error {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return fmt.Errorf("missing %s cookie", sessionCookieName)
	}

	verifyURL := os.Getenv("AUTH_VERIFY_URL")
	if verifyURL == "" {
		verifyURL = "http://" + config.AuthDNS() + ":9091/api/verify"
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, verifyURL, nil)
	if err != nil {
		return fmt.Errorf("create verify request: %w", err)
	}
	req.AddCookie(cookie)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("auth verify request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("auth verify returned %d", resp.StatusCode)
	}
	return nil
}

// AuthWebSocketHandler returns an http.Handler that validates the session
// cookie before upgrading to a WebSocket connection. This allows the
// /ws endpoint to be served without nginx auth_request, avoiding issues
// with WebSocket upgrade requests and auth subrequests.
//
// It uses websocket.Server with a custom Handshake function that skips
// the default Origin header check. Auth is already enforced by verifying
// the session cookie, so Origin checking is unnecessary.
func AuthWebSocketHandler() http.Handler {
	wsHandler := WebSocketHandler()
	zone := config.UserZone()
	customDomain := os.Getenv("CUSTOM_DOMAIN")
	wsServer := websocket.Server{
		Handler: wsHandler,
		Handshake: func(cfg *websocket.Config, r *http.Request) error {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return nil
			}
			host := strings.TrimPrefix(strings.TrimPrefix(origin, "https://"), "http://")
			if isAllowedOrigin(host, zone, customDomain) {
				return nil
			}
			serverIP := os.Getenv("SERVER_IP")
			if serverIP != "" && host == serverIP {
				return nil
			}
			return fmt.Errorf("origin %q not allowed", origin)
		},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := verifySession(r); err != nil {
			klog.V(2).Infof("ws: auth rejected: %v", err)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		wsServer.ServeHTTP(w, r)
	})
}

// WebSocketHandler returns a websocket.Handler that can be used with
// http.Handle. Each connection is registered with the hub, receives a
// welcome message, and gets periodic heartbeat pings.
func WebSocketHandler() websocket.Handler {
	return func(ws *websocket.Conn) {
		client := &wsClient{
			conn: ws,
			send: make(chan []byte, 64),
		}

		defaultHub.register(client)
		defer func() {
			defaultHub.unregister(client)
			ws.Close()
		}()

		// Send the welcome message immediately.
		welcome := WSMessage{
			Type: "connected",
			Data: map[string]string{
				"version": "1.0.0",
			},
		}
		welcomeData, _ := json.Marshal(welcome)
		if _, err := ws.Write(welcomeData); err != nil {
			klog.V(2).Infof("ws: failed to send welcome: %v", err)
			return
		}

		// Writer goroutine: drains the send channel and writes to the
		// connection. Also sends heartbeat pings on a timer.
		done := make(chan struct{})
		go func() {
			defer close(done)
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case msg, ok := <-client.send:
					if !ok {
						// Hub closed the channel; client was unregistered.
						return
					}
					if _, err := ws.Write(msg); err != nil {
						klog.V(2).Infof("ws: write error: %v", err)
						return
					}
				case <-ticker.C:
					ping := WSMessage{
						Type: "ping",
						Data: map[string]int64{
							"timestamp": time.Now().Unix(),
						},
					}
					data, _ := json.Marshal(ping)
					if _, err := ws.Write(data); err != nil {
						klog.V(2).Infof("ws: ping error: %v", err)
						return
					}
				}
			}
		}()

		// Reader loop: read and discard incoming messages (we only need
		// to detect when the client disconnects).
		buf := make([]byte, 1024)
		for {
			if _, err := ws.Read(buf); err != nil {
				// Client disconnected or read error.
				klog.V(3).Infof("ws: read error (client disconnect): %v", err)
				break
			}
		}

		// Wait for the writer goroutine to finish.
		<-done
	}
}

// isAllowedOrigin checks if a host matches the zone or custom domain (including subdomains).
func isAllowedOrigin(host, zone, customDomain string) bool {
	if host == zone || strings.HasSuffix(host, "."+zone) {
		return true
	}
	if customDomain != "" && (host == customDomain || strings.HasSuffix(host, "."+customDomain)) {
		return true
	}
	return false
}
