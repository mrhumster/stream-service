package wss

import (
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Hub struct {
	clients map[uuid.UUID][]*websocket.Conn
	mu      sync.Mutex
}

func NewHub() *Hub {
	return &Hub{clients: make(map[uuid.UUID][]*websocket.Conn)}
}

func (h *Hub) Register(userID uuid.UUID, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[userID] = append(h.clients[userID], conn)
}

func (h *Hub) Unregister(userID uuid.UUID, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	conns := h.clients[userID]
	for i, c := range conns {
		if c == conn {
			h.clients[userID] = append(conns[:i], conns[i+1:]...)
		}
	}
	if len(h.clients[userID]) == 0 {
		delete(h.clients, userID)
	}
}

func (h *Hub) SendProgress(userID uuid.UUID, data interface{}) {
	h.mu.Lock()
	conns := h.clients[userID]
	h.mu.Unlock()
	for _, conn := range conns {
		_ = conn.WriteJSON(data)
	}
}
