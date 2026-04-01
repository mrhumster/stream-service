package queue

import (
	"sync"

	"github.com/google/uuid"
	"golang.org/x/net/websocket"
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
