package handlers

import (
	"encoding/json"
	"strings"
	"sync"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"

	"venus_backend/internal/middleware"
)

// WSHub is a lightweight realtime hub. Clients connect with a valid JWT at
// /ws?token=... and are registered under their user id. Broadcast(userID, ...)
// delivers an event only to that subscriber's sockets; userID 0 targets admin
// sockets (role=admin) for cross-subscriber events such as new tickets.
type WSHub struct {
	mu      sync.RWMutex
	writeMu sync.Mutex // serializes WriteMessage: websocket conns are not thread-safe
	clients map[int64]map[*websocket.Conn]bool
}

// NewWSHub creates an empty hub.
func NewWSHub() *WSHub {
	return &WSHub{clients: make(map[int64]map[*websocket.Conn]bool)}
}

// Event is the envelope pushed to clients.
type Event struct {
	Channel string      `json:"channel"` // inventory | sales | debts | tickets | admin
	Data    interface{} `json:"data"`
}

// Broadcast sends an event to every socket of the given user (0 = admins).
func (h *WSHub) Broadcast(userID int64, channel string, data interface{}) {
	evt, err := json.Marshal(Event{Channel: channel, Data: data})
	if err != nil {
		return
	}

	h.mu.RLock()
	sockets := make([]*websocket.Conn, 0, len(h.clients[userID]))
	for conn := range h.clients[userID] {
		sockets = append(sockets, conn)
	}
	h.mu.RUnlock()

	h.writeMu.Lock()
	for _, conn := range sockets {
		if err := conn.WriteMessage(websocket.TextMessage, evt); err != nil {
			h.remove(userID, conn)
			conn.Close()
		}
	}
	h.writeMu.Unlock()
}

func (h *WSHub) remove(userID int64, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if set, ok := h.clients[userID]; ok {
		delete(set, conn)
		if len(set) == 0 {
			delete(h.clients, userID)
		}
	}
}

// Handler upgrades /ws connections. The JWT may arrive via ?token= query
// param (browsers cannot set headers on WebSocket) or Authorization header.
func (h *WSHub) Handler(jwtSecret string) fiber.Handler {
	return websocket.New(func(conn *websocket.Conn) {
		token := conn.Query("token")
		if token == "" {
			token = strings.TrimPrefix(conn.Headers("Authorization"), "Bearer ")
		}
		claims, err := middleware.ParseToken(jwtSecret, token)
		if err != nil {
			conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "invalid token"))
			conn.Close()
			return
		}
		userID := claims.UserID
		if claims.Role == "admin" {
			userID = 0 // admins receive cross-subscriber admin events
		}

		h.mu.Lock()
		if h.clients[userID] == nil {
			h.clients[userID] = make(map[*websocket.Conn]bool)
		}
		h.clients[userID][conn] = true
		h.mu.Unlock()

		defer func() {
			h.remove(userID, conn)
			conn.Close()
		}()

		// Read loop keeps the connection alive and drains client messages.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
}
