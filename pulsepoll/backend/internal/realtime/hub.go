package realtime

import (
	"encoding/json"
	"github.com/gorilla/websocket"
	"sync"
	"time"
)

type Event struct {
	Type   string           `json:"type"`
	PollID string           `json:"poll_id"`
	Counts map[string]int64 `json:"counts,omitempty"`
	Status string           `json:"status,omitempty"`
}

type Hub struct {
	mu      sync.RWMutex
	clients map[string]map[*websocket.Conn]struct{}
}

func NewHub() *Hub { return &Hub{clients: make(map[string]map[*websocket.Conn]struct{})} }
func (h *Hub) Add(pollID string, c *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[pollID] == nil {
		h.clients[pollID] = map[*websocket.Conn]struct{}{}
	}
	h.clients[pollID][c] = struct{}{}
}
func (h *Hub) Remove(pollID string, c *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients[pollID], c)
	if len(h.clients[pollID]) == 0 {
		delete(h.clients, pollID)
	}
}
func (h *Hub) Broadcast(e Event) {
	data, _ := json.Marshal(e)
	h.mu.RLock()
	targets := make([]*websocket.Conn, 0, len(h.clients[e.PollID]))
	for c := range h.clients[e.PollID] {
		targets = append(targets, c)
	}
	h.mu.RUnlock()
	for _, c := range targets {
		_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := c.WriteMessage(websocket.TextMessage, data); err != nil {
			h.Remove(e.PollID, c)
			_ = c.Close()
		}
	}
}
