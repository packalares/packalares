package appservice

import (
	"encoding/json"
	"sync"
	"time"

	"golang.org/x/net/websocket"
	"k8s.io/klog/v2"
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
