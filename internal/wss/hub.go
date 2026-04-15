//go:generate mockgen -source=hub.go -destination=./mock/hub_mock.go -package=mock
package wss

import (
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Hub interface {
	Register(userID uuid.UUID, conn *websocket.Conn)
	Unregister(userID uuid.UUID, conn *websocket.Conn)
	SendMessgeToOwner(userID uuid.UUID, data any)
}
