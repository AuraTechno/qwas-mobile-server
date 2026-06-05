package ws

import (
	"encoding/json"
	"log"
	"time"

	"github.com/AuraTechno/qwas-mobile-server/internal/auth"
	"github.com/gofiber/contrib/websocket"
)

type Handler struct {
	Hub  *Hub
	Auth *auth.Manager
}

// WebSocket upgrade middleware (validates JWT from query/header before upgrade)
func (h *Handler) UpgradeMiddleware() func(*websocket.Conn) {
	return func(c *websocket.Conn) {
		// Token from query param
		token := c.Query("token")
		if token == "" {
			_ = c.WriteJSON(map[string]interface{}{"error": "No token"})
			_ = c.Close()
			return
		}
		claims, err := h.Auth.ParseToken(token)
		if err != nil {
			_ = c.WriteJSON(map[string]interface{}{"error": "Invalid token"})
			_ = c.Close()
			return
		}
		c.Locals("userId", claims.UserID)
		c.Locals("username", claims.Username)
	}
}

func (h *Handler) Handle(c *websocket.Conn) {
	userID, _ := c.Locals("userId").(int64)
	if userID == 0 {
		_ = c.Close()
		return
	}

	h.Hub.Register(userID, c)
	defer h.Hub.Unregister(userID, c)

	// Read messages (pongs/keepalive/typing/call signaling)
	for {
		_, data, err := c.ReadMessage()
		if err != nil {
			return
		}

		var msg struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "ping":
			_ = c.WriteJSON(map[string]interface{}{"type": "pong", "ts": time.Now().UnixMilli()})
		case "typing":
			// payload: {chatId, isTyping}
			var p struct {
				ChatID   int64 `json:"chatId"`
				IsTyping bool  `json:"isTyping"`
			}
			if err := json.Unmarshal(msg.Payload, &p); err != nil {
				continue
			}
			// Resolve chat members and broadcast (excluding self)
			// Skipped here for brevity; could query DB if needed
		case "call_offer", "call_answer", "call_ice", "call_end":
			// WebRTC signaling - forward to target user
			var p struct {
				TargetUserID int64           `json:"targetUserId"`
				ChatID       int64           `json:"chatId"`
				Data         json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(msg.Payload, &p); err != nil {
				continue
			}
			h.Hub.SendToUser(p.TargetUserID, "webrtc_"+msg.Type, map[string]interface{}{
				"fromUserId": userID,
				"chatId":     p.ChatID,
				"data":       p.Data,
			})
		}
	}
}

// Silence log spam
var _ = log.Print
