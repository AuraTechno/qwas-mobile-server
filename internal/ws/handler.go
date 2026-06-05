package ws

import (
	"database/sql"
	"encoding/json"
	"log"
	"time"

	"github.com/AuraTechno/qwas-mobile-server/internal/auth"
	"github.com/gofiber/contrib/websocket"
)

type Handler struct {
	Hub  *Hub
	Auth *auth.Manager
	DB   *sql.DB
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
	// Validate JWT from query token (UpgradeMiddleware is defined but not wired up)
	token := c.Query("token")
	if token == "" {
		_ = c.WriteJSON(map[string]interface{}{"type": "error", "error": "No token"})
		_ = c.Close()
		return
	}
	claims, err := h.Auth.ParseToken(token)
	if err != nil {
		_ = c.WriteJSON(map[string]interface{}{"type": "error", "error": "Invalid token"})
		_ = c.Close()
		return
	}
	userID := claims.UserID
	c.Locals("userId", userID)
	c.Locals("username", claims.Username)

	h.Hub.Register(userID, c)
	defer h.Hub.Unregister(userID, c)

	// Send welcome event so client knows we're connected
	_ = c.WriteJSON(map[string]interface{}{
		"event":   "connected",
		"payload": map[string]interface{}{"userId": userID, "ts": time.Now().UnixMilli()},
		"ts":      time.Now().UnixMilli(),
	})

	// Read messages (pongs/keepalive/typing/call signaling)
	c.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.SetPongHandler(func(string) error {
		c.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			time.Sleep(100 * time.Millisecond)
			select {
			case <-ticker.C:
				if err := c.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
					return
				}
			default:
				if c.Conn == nil {
					return
				}
			}
			if _, ok := h.Hub.IsConnected(userID); !ok {
				return
			}
		}
	}()

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
			var p struct {
				ChatID   int64 `json:"chatId"`
				IsTyping bool  `json:"isTyping"`
			}
			if err := json.Unmarshal(msg.Payload, &p); err != nil {
				continue
			}
			if p.ChatID == 0 {
				continue
			}
			if h.DB == nil {
				continue
			}
			rows, err := h.DB.Query("SELECT user_id FROM chat_members WHERE chat_id = $1", p.ChatID)
			if err != nil {
				continue
			}
			var memberIDs []int64
			for rows.Next() {
				var uid int64
				if err := rows.Scan(&uid); err == nil {
					memberIDs = append(memberIDs, uid)
				}
			}
			rows.Close()
			ev := map[string]interface{}{
				"chatId":   p.ChatID,
				"userId":   userID,
				"isTyping": p.IsTyping,
			}
			for _, uid := range memberIDs {
				if uid == userID {
					continue
				}
				h.Hub.SendToUser(uid, "user_typing", ev)
			}
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
