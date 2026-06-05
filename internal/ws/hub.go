package ws

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gofiber/contrib/websocket"
)

// Hub manages connected WebSocket clients, indexed by user ID
type Hub struct {
	mu      sync.RWMutex
	clients map[int64]map[*websocket.Conn]struct{}
	online  map[int64]time.Time
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[int64]map[*websocket.Conn]struct{}),
		online:  make(map[int64]time.Time),
	}
}

func (h *Hub) Register(userID int64, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[userID] == nil {
		h.clients[userID] = make(map[*websocket.Conn]struct{})
	}
	h.clients[userID][conn] = struct{}{}
	h.online[userID] = time.Now()
	log.Printf("ws: user %d connected, %d total connections", userID, len(h.clients[userID]))
}

func (h *Hub) Unregister(userID int64, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if conns, ok := h.clients[userID]; ok {
		delete(conns, conn)
		if len(conns) == 0 {
			delete(h.clients, userID)
			delete(h.online, userID)
		}
	}
	log.Printf("ws: user %d disconnected, %d total connections", userID, len(h.clients[userID]))
}

func (h *Hub) IsOnline(userID int64) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.online[userID]
	return ok
}

func (h *Hub) OnlineUserIDs() []int64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	ids := make([]int64, 0, len(h.online))
	for id := range h.online {
		ids = append(ids, id)
	}
	return ids
}

// SendToUser sends an event to all connections of a user
func (h *Hub) SendToUser(userID int64, event string, payload interface{}) {
	h.mu.RLock()
	conns := make([]*websocket.Conn, 0, len(h.clients[userID]))
	for c := range h.clients[userID] {
		conns = append(conns, c)
	}
	h.mu.RUnlock()

	data := map[string]interface{}{
		"event":   event,
		"payload": payload,
		"ts":      time.Now().UnixMilli(),
	}
	b, err := json.Marshal(data)
	if err != nil {
		log.Printf("ws: marshal error: %v", err)
		return
	}

	for _, c := range conns {
		if err := c.WriteMessage(websocket.TextMessage, b); err != nil {
			log.Printf("ws: write to user %d error: %v", userID, err)
		}
	}
}

// SendToUsers sends the same event to multiple users
func (h *Hub) SendToUsers(userIDs []int64, event string, payload interface{}) {
	for _, id := range userIDs {
		h.SendToUser(id, event, payload)
	}
}

// BroadcastToChat sends an event to all members of a chat (caller resolves member IDs)
func (h *Hub) BroadcastToChat(memberIDs []int64, event string, payload interface{}) {
	h.SendToUsers(memberIDs, event, payload)
}

// Standard event names
const (
	EventNewMessage       = "new_message"
	EventMessageEdited    = "message_edited"
	EventMessageDeleted   = "message_deleted"
	EventMessageReaction  = "message_reaction"
	EventTyping           = "typing"
	EventUserOnline       = "user_online"
	EventUserOffline      = "user_offline"
	EventChatUpdated      = "chat_updated"
	EventPinnedUpdated    = "pinned_updated"
	EventCallIncoming     = "call_incoming"
	EventCallAccepted     = "call_accepted"
	EventCallRejected     = "call_rejected"
	EventCallEnded        = "call_ended"
	EventSignalOffer      = "signal_offer"
	EventSignalAnswer     = "signal_answer"
	EventSignalIce        = "signal_ice"
)
