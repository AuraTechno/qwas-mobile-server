package handlers

import (
	"context"
	"time"

	"github.com/AuraTechno/qwas-mobile-server/internal/db"
	"github.com/AuraTechno/qwas-mobile-server/internal/ws"
	"github.com/gofiber/fiber/v2"
)

type ReactionsHandler struct {
	DB  *db.DB
	Hub *ws.Hub
}

func NewReactionsHandler(d *db.DB, hub *ws.Hub) *ReactionsHandler {
	return &ReactionsHandler{DB: d, Hub: hub}
}

// PUT /api/v1/messages/:id/reactions - toggle reaction
func (h *ReactionsHandler) Toggle(c *fiber.Ctx) error {
	userID := c.Locals("userId").(int64)
	msgID := int64Param(c, "id")
	var req struct {
		Emoji string `json:"emoji"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"ok": false, "error": "Invalid body"})
	}
	if req.Emoji == "" || len(req.Emoji) > 16 {
		return c.JSON(fiber.Map{"ok": false, "error": "Invalid emoji"})
	}

	var chatID int64
	_ = h.DB.Pool.QueryRow(c.Context(), `SELECT chat_id FROM messages WHERE id=$1`, msgID).Scan(&chatID)
	if chatID == 0 {
		return c.Status(404).JSON(fiber.Map{"ok": false, "error": "Message not found"})
	}

	// Check existing
	var exists bool
	_ = h.DB.Pool.QueryRow(c.Context(), `
		SELECT EXISTS(SELECT 1 FROM reactions WHERE message_id=$1 AND user_id=$2 AND emoji=$3)
	`, msgID, userID, req.Emoji).Scan(&exists)

	if exists {
		_, _ = h.DB.Pool.Exec(c.Context(), `DELETE FROM reactions WHERE message_id=$1 AND user_id=$2 AND emoji=$3`, msgID, userID, req.Emoji)
		if h.Hub != nil {
			h.broadcastReaction(chatID, msgID, userID, req.Emoji, false)
		}
		return c.JSON(fiber.Map{"ok": true, "active": false})
	}

	_, err := h.DB.Pool.Exec(c.Context(), `INSERT INTO reactions (message_id, user_id, emoji) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, msgID, userID, req.Emoji)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"ok": false, "error": "DB error"})
	}
	if h.Hub != nil {
		h.broadcastReaction(chatID, msgID, userID, req.Emoji, true)
	}
	return c.JSON(fiber.Map{"ok": true, "active": true})
}

func (h *ReactionsHandler) broadcastReaction(chatID, msgID, userID int64, emoji string, active bool) {
	memberRows, _ := h.DB.Pool.Query(context.Background(), `SELECT user_id FROM chat_members WHERE chat_id=$1`, chatID)
	defer memberRows.Close()
	var memberIDs []int64
	for memberRows.Next() {
		var id int64
		if err := memberRows.Scan(&id); err == nil {
			memberIDs = append(memberIDs, id)
		}
	}
	for _, mid := range memberIDs {
		h.Hub.SendToUser(mid, "message_reaction", fiber.Map{
			"id":        msgID,
			"chatId":    chatID,
			"userId":    userID,
			"emoji":     emoji,
			"active":    active,
			"createdAt": time.Now(),
		})
	}
}

// GET /api/v1/messages/:id/reactions
func (h *ReactionsHandler) List(c *fiber.Ctx) error {
	msgID := int64Param(c, "id")
	rows, err := h.DB.Pool.Query(c.Context(), `
		SELECT message_id, user_id, emoji, created_at
		FROM reactions WHERE message_id=$1
		ORDER BY created_at ASC
	`, msgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"ok": false, "error": "DB error"})
	}
	defer rows.Close()

	var reactions []fiber.Map
	for rows.Next() {
		var messageID, userID int64
		var emoji string
		var createdAt time.Time
		if err := rows.Scan(&messageID, &userID, &emoji, &createdAt); err == nil {
			reactions = append(reactions, fiber.Map{
				"messageId": messageID,
				"userId":    userID,
				"emoji":     emoji,
				"createdAt": createdAt,
			})
		}
	}
	if reactions == nil {
		reactions = []fiber.Map{}
	}
	return c.JSON(fiber.Map{"ok": true, "reactions": reactions})
}

type PinnedHandler struct {
	DB *db.DB
}

func NewPinnedHandler(d *db.DB) *PinnedHandler {
	return &PinnedHandler{DB: d}
}

// POST /api/v1/chats/:id/pin-message
func (h *PinnedHandler) Pin(c *fiber.Ctx) error {
	userID := c.Locals("userId").(int64)
	chatID := int64Param(c, "id")
	var req struct {
		MessageID int64 `json:"messageId"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"ok": false, "error": "Invalid body"})
	}

	var isMember bool
	_ = h.DB.Pool.QueryRow(c.Context(), `SELECT EXISTS(SELECT 1 FROM chat_members WHERE chat_id=$1 AND user_id=$2)`, chatID, userID).Scan(&isMember)
	if !isMember {
		return c.Status(403).JSON(fiber.Map{"ok": false, "error": "Forbidden"})
	}

	_, err := h.DB.Pool.Exec(c.Context(), `UPDATE chats SET pinned_message_id=$1 WHERE id=$2`, req.MessageID, chatID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"ok": false, "error": "DB error"})
	}
	return c.JSON(fiber.Map{"ok": true})
}

// DELETE /api/v1/chats/:id/pin-message
func (h *PinnedHandler) Unpin(c *fiber.Ctx) error {
	userID := c.Locals("userId").(int64)
	chatID := int64Param(c, "id")
	var isMember bool
	_ = h.DB.Pool.QueryRow(c.Context(), `SELECT EXISTS(SELECT 1 FROM chat_members WHERE chat_id=$1 AND user_id=$2)`, chatID, userID).Scan(&isMember)
	if !isMember {
		return c.Status(403).JSON(fiber.Map{"ok": false, "error": "Forbidden"})
	}
	_, err := h.DB.Pool.Exec(c.Context(), `UPDATE chats SET pinned_message_id=NULL WHERE id=$1`, chatID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"ok": false, "error": "DB error"})
	}
	return c.JSON(fiber.Map{"ok": true})
}
