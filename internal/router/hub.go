package router

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// writeWait is the maximum time a single WebSocket write may take.
	writeWait = 10 * time.Second
	// pongWait is how long we wait for a pong after sending a ping before
	// considering the client dead.
	pongWait = 60 * time.Second
	// pingPeriod is how often we send a ping to keep connections alive and
	// detect dead peers.
	pingPeriod = 30 * time.Second
	// sendBuffer is the per-client outbound queue depth. When a client's
	// queue is full we drop it as "too slow" rather than block the event
	// broadcaster for everyone else.
	sendBuffer = 64
)

// client is a single WebSocket connection plus its outbound queue. All writes
// to conn happen from exactly one goroutine (the write pump) which drains
// sendCh and emits periodic pings, so we never have concurrent writers on a
// connection (gorilla/websocket requirement).
type client struct {
	conn   *websocket.Conn
	sendCh chan []byte
}

type Hub struct {
	mu      sync.Mutex
	clients map[*client]struct{}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		return strings.HasPrefix(origin, "http://127.0.0.1") ||
			strings.HasPrefix(origin, "http://localhost") ||
			strings.HasPrefix(origin, "ws://127.0.0.1") ||
			strings.HasPrefix(origin, "ws://localhost")
	},
}

func NewHub() *Hub {
	return &Hub{clients: make(map[*client]struct{})}
}

func (h *Hub) Add(c *client) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	n := len(h.clients)
	h.mu.Unlock()
	log.Printf("dashboard client connected (%d total)", n)
}

func (h *Hub) Remove(c *client) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
	}
	h.mu.Unlock()
	c.conn.Close()
}

// Send broadcasts a JSON message to all connected clients. It snapshots the
// client set under the lock, then enqueues outside the lock so a slow client
// cannot block others: if a client's queue is full it is closed and removed.
func (h *Hub) Send(msg map[string]any) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("hub marshal: %v", err)
		return
	}

	// Snapshot clients under lock.
	h.mu.Lock()
	snapshot := make([]*client, 0, len(h.clients))
	for c := range h.clients {
		snapshot = append(snapshot, c)
	}
	h.mu.Unlock()

	// Enqueue outside the lock. Non-blocking send: a full queue means the
	// client is too slow, so drop it.
	var failed []*client
	for _, c := range snapshot {
		select {
		case c.sendCh <- data:
		default:
			failed = append(failed, c)
		}
	}

	// Cleanup failed clients under lock.
	if len(failed) > 0 {
		h.mu.Lock()
		for _, c := range failed {
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				c.conn.Close()
			}
		}
		h.mu.Unlock()
	}
}

func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade: %v", err)
		return
	}
	c := &client{conn: conn, sendCh: make(chan []byte, sendBuffer)}
	h.Add(c)

	// Read pump: consumes incoming frames (including control frames, which
	// gorilla handles internally), enforces the pong deadline, and exits when
	// the peer goes away — tearing down the whole connection.
	go func() {
		defer h.Remove(c)
		_ = conn.SetReadDeadline(time.Now().Add(pongWait))
		conn.SetPongHandler(func(string) error {
			return conn.SetReadDeadline(time.Now().Add(pongWait))
		})
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	// Write pump: the ONLY goroutine that writes to conn. Drains the outbound
	// queue and emits periodic pings; on any error the connection is closed
	// and removed.
	go func() {
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()
		defer h.Remove(c)
		for {
			select {
			case msg, ok := <-c.sendCh:
				if !ok {
					return
				}
				_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
				if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
					return
				}
			case <-ticker.C:
				_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()
}
