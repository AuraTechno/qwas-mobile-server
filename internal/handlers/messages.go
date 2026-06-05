package handlers

import (
	"strconv"
	"time"

	"github.com/AuraTechno/qwas-mobile-server/internal/db"
	"github.com/AuraTechno/qwas-mobile-server/internal/ws"
	"github.com/gofiber/fiber/v2"
)

type MessagesHandler struct {
	DB  *db.DB
	Hub *ws.Hub
}

func NewMessagesHandler(d *db.DB, hub *ws.Hub) *MessagesHandler {
	return &MessagesHandler{DB: d, Hub: hub}
}

type sendMessageReq struct {
	Type      string  `json:"type"`      // text, image, video, voice, file, system
	Content   string  `json:"content"`
	MediaURL  string  `json:"mediaUrl"`
	MediaMeta *string `json:"mediaMeta"` // JSON string
	ReplyToID *int64  `json:"replyToId"`
}

// GET /api/v1/chats/:id/messages?limit=50&before=ID
func (h *MessagesHandler) List(c *fiber.Ctx) error {
	userID := c.Locals("userId").(int64)
	chatID := int64Param(c, "id")

	var isMember bool
	err := h.DB.Pool.QueryRow(c.Context(), `SELECT EXISTS(SELECT 1 FROM chat_members WHERE chat_id=$1 AND user_id=$2)`, chatID, userID).Scan(&isMember)
	if err != nil || !isMember {
		return c.Status(403).JSON(fiber.Map{"ok": false, "error": "Forbidden"})
	}

	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	before, _ := strconv.ParseInt(c.Query("before", "0"), 10, 64)

	q := `
		SELECT m.id, m.chat_id, m.sender_id, COALESCE(u.username,''), COALESCE(u.display_name,''), COALESCE(u.avatar_color,''),
		       m.type, COALESCE(m.content,''), COALESCE(m.media_url,''), COALESCE(m.media_meta::text,''),
		       m.reply_to_id, m.created_at, m.edited_at
		FROM messages m
		JOIN users u ON u.id=m.sender_id
		WHERE m.chat_id=$1 AND m.is_deleted=false
	`
	args := []interface{}{chatID}
	if before > 0 {
		q += ` AND m.id < $2 ORDER BY m.id DESC LIMIT $3`
		args = append(args, before, limit)
	} else {
		q += ` ORDER BY m.id DESC LIMIT $2`
		args = append(args, limit)
	}

	rows, err := h.DB.Pool.Query(c.Context(), q, args...)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"ok": false, "error": "DB error"})
	}
	defer rows.Close()

	var messages []fiber.Map
	for rows.Next() {
		var id, chatID, senderID int64
		var replyToPtr *int64
		var senderUsername, senderName, senderColor, msgType, content, mediaURL, mediaMeta string
		var createdAt time.Time
		var editedAt *time.Time
		if err := rows.Scan(&id, &chatID, &senderID, &senderUsername, &senderName, &senderColor,
			&msgType, &content, &mediaURL, &mediaMeta, &replyToPtr, &createdAt, &editedAt); err != nil {
			continue
		}
		messages = append(messages, fiber.Map{
			"id":          id,
			"chatId":      chatID,
			"senderId":    senderID,
			"senderName":  senderName,
			"senderColor": senderColor,
			"type":        msgType,
			"content":     content,
			"mediaUrl":    mediaURL,
			"mediaMeta":   mediaMeta,
			"replyToId":   replyToPtr,
			"createdAt":   createdAt,
			"editedAt":    editedAt,
		})
	}
	if messages == nil {
		messages = []fiber.Map{}
	}

	// Reverse to chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return c.JSON(fiber.Map{"ok": true, "messages": messages})
}

// POST /api/v1/chats/:id/messages
func (h *MessagesHandler) Send(c *fiber.Ctx) error {
	userID := c.Locals("userId").(int64)
	chatID := int64Param(c, "id")

	var isMember bool
	err := h.DB.Pool.QueryRow(c.Context(), `SELECT EXISTS(SELECT 1 FROM chat_members WHERE chat_id=$1 AND user_id=$2)`, chatID, userID).Scan(&isMember)
	if err != nil || !isMember {
		return c.Status(403).JSON(fiber.Map{"ok": false, "error": "Forbidden"})
	}

	var req sendMessageReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"ok": false, "error": "Invalid body"})
	}

	if req.Type == "" {
		req.Type = "text"
	}
	if req.Type != "text" && req.Type != "image" && req.Type != "video" && req.Type != "voice" && req.Type != "file" && req.Type != "system" {
		return c.JSON(fiber.Map{"ok": false, "error": "Invalid message type"})
	}
	if req.Type == "text" && len(req.Content) == 0 {
		return c.JSON(fiber.Map{"ok": false, "error": "Empty message"})
	}
	if len(req.Content) > 8000 {
		return c.JSON(fiber.Map{"ok": false, "error": "Message too long (max 8000)"})
	}

	tx, err := h.DB.Pool.Begin(c.Context())
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"ok": false, "error": "DB error"})
	}
	defer tx.Rollback(c.Context())

	var msgID int64
	err = tx.QueryRow(c.Context(), `
		INSERT INTO messages (chat_id, sender_id, type, content, media_url, media_meta, reply_to_id)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7)
		RETURNING id
	`, chatID, userID, req.Type, req.Content, req.MediaURL, req.MediaMeta, req.ReplyToID).Scan(&msgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"ok": false, "error": "DB error"})
	}

	// Update chat's updated_at
	_, _ = tx.Exec(c.Context(), `UPDATE chats SET updated_at=NOW() WHERE id=$1`, chatID)

	// Fetch the full message
	var id, chatID2, senderID int64
	var senderUsername, senderName, senderColor, msgType, content, mediaURL, mediaMeta string
	var replyToID *int64
	var createdAt time.Time
	var editedAt *time.Time
	err = tx.QueryRow(c.Context(), `
		SELECT m.id, m.chat_id, m.sender_id, u.username, u.display_name, u.avatar_color,
		       m.type, m.content, COALESCE(m.media_url,''), COALESCE(m.media_meta::text,''), m.reply_to_id, m.created_at, m.edited_at
		FROM messages m JOIN users u ON u.id=m.sender_id
		WHERE m.id=$1
	`, msgID).Scan(&id, &chatID2, &senderID, &senderUsername, &senderName, &senderColor,
		&msgType, &content, &mediaURL, &mediaMeta, &replyToID, &createdAt, &editedAt)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"ok": false, "error": "DB error"})
	}

	if err := tx.Commit(c.Context()); err != nil {
		return c.Status(500).JSON(fiber.Map{"ok": false, "error": "DB error"})
	}

	// Get all members for broadcast
	memberRows, _ := h.DB.Pool.Query(c.Context(), `SELECT user_id FROM chat_members WHERE chat_id=$1`, chatID)
	defer memberRows.Close()
	var memberIDs []int64
	for memberRows.Next() {
		var id int64
		if err := memberRows.Scan(&id); err == nil {
			memberIDs = append(memberIDs, id)
		}
	}

	msgPayload := fiber.Map{
		"id":          id,
		"chatId":      chatID2,
		"senderId":    senderID,
		"senderName":  senderName,
		"senderColor": senderColor,
		"type":        msgType,
		"content":     content,
		"mediaUrl":    mediaURL,
		"mediaMeta":   mediaMeta,
		"replyToId":   replyToID,
		"createdAt":   createdAt,
		"editedAt":    editedAt,
	}
	for _, mid := range memberIDs {
		if h.Hub != nil {
			h.Hub.SendToUser(mid, "new_message", msgPayload)
		}
	}

	return c.JSON(fiber.Map{
		"ok":         true,
		"id":         id,
		"chatId":     chatID2,
		"senderId":   senderID,
		"senderName": senderName,
		"type":       msgType,
		"content":    content,
		"mediaUrl":   mediaURL,
		"mediaMeta":  mediaMeta,
		"replyToId":  replyToID,
		"createdAt":  createdAt,
		"members":    memberIDs,
	})
}

// DELETE /api/v1/messages/:id
func (h *MessagesHandler) Delete(c *fiber.Ctx) error {
	userID := c.Locals("userId").(int64)
	msgID := int64Param(c, "id")

	res, err := h.DB.Pool.Exec(c.Context(), `UPDATE messages SET is_deleted=true, content='', media_url='' WHERE id=$1 AND sender_id=$2`, msgID, userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"ok": false, "error": "DB error"})
	}
	if res.RowsAffected() == 0 {
		return c.Status(403).JSON(fiber.Map{"ok": false, "error": "Cannot delete"})
	}
	return c.JSON(fiber.Map{"ok": true})
}

// PATCH /api/v1/messages/:id (edit text)
func (h *MessagesHandler) Edit(c *fiber.Ctx) error {
	userID := c.Locals("userId").(int64)
	msgID := int64Param(c, "id")
	var req struct {
		Content string `json:"content"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"ok": false, "error": "Invalid body"})
	}
	if len(req.Content) == 0 || len(req.Content) > 8000 {
		return c.JSON(fiber.Map{"ok": false, "error": "Invalid content"})
	}
	res, err := h.DB.Pool.Exec(c.Context(), `UPDATE messages SET content=$1, edited_at=NOW() WHERE id=$2 AND sender_id=$3 AND type='text' AND is_deleted=false`, req.Content, msgID, userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"ok": false, "error": "DB error"})
	}
	if res.RowsAffected() == 0 {
		return c.Status(403).JSON(fiber.Map{"ok": false, "error": "Cannot edit"})
	}
	return c.JSON(fiber.Map{"ok": true})
}
