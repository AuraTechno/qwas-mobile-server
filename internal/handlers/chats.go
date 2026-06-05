package handlers

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/AuraTechno/qwas-mobile-server/internal/db"
	"github.com/gofiber/fiber/v2"
)

type ChatsHandler struct {
	DB *db.DB
}

func NewChatsHandler(d *db.DB) *ChatsHandler {
	return &ChatsHandler{DB: d}
}

type createChatReq struct {
	Type        string  `json:"type"`        // private, group, channel
	Name        string  `json:"name"`
	Description string  `json:"description"`
	UserIDs     []int64 `json:"userIds"`
	Username    string  `json:"username"`    // for private chat - target user
}

type chatResp struct {
	OK       bool        `json:"ok"`
	Chat     interface{} `json:"chat,omitempty"`
	Chats    interface{} `json:"chats,omitempty"`
	Messages interface{} `json:"messages,omitempty"`
	Error    string      `json:"error,omitempty"`
}

// GET /api/v1/chats
func (h *ChatsHandler) List(c *fiber.Ctx) error {
	userID := c.Locals("userId").(int64)

	rows, err := h.DB.Pool.Query(c.Context(), `
		SELECT c.id, c.type, COALESCE(c.name,''), COALESCE(c.description,''), COALESCE(c.avatar_url,''), COALESCE(c.avatar_color,''),
		       c.owner_id, c.pinned_message_id, c.created_at, c.updated_at,
		       (SELECT m.id FROM messages m WHERE m.chat_id=c.id AND m.is_deleted=false ORDER BY m.id DESC LIMIT 1) AS last_msg_id,
		       (SELECT m.content FROM messages m WHERE m.chat_id=c.id AND m.is_deleted=false ORDER BY m.id DESC LIMIT 1) AS last_msg_content,
		       (SELECT m.type FROM messages m WHERE m.chat_id=c.id AND m.is_deleted=false ORDER BY m.id DESC LIMIT 1) AS last_msg_type,
		       (SELECT m.created_at FROM messages m WHERE m.chat_id=c.id AND m.is_deleted=false ORDER BY m.id DESC LIMIT 1) AS last_msg_at,
		       (SELECT m.sender_id FROM messages m WHERE m.chat_id=c.id AND m.is_deleted=false ORDER BY m.id DESC LIMIT 1) AS last_msg_sender,
		       (SELECT u.display_name FROM messages m JOIN users u ON u.id=m.sender_id WHERE m.chat_id=c.id AND m.is_deleted=false ORDER BY m.id DESC LIMIT 1) AS last_msg_sender_name,
		       COALESCE(cm.last_read_message_id, 0),
		       cm.is_muted, cm.notifications_enabled
		FROM chats c
		JOIN chat_members cm ON cm.chat_id=c.id
		WHERE cm.user_id=$1
		ORDER BY c.updated_at DESC
	`, userID)
	if err != nil {
		log.Printf("chats.List: query error: %v", err)
		return c.Status(500).JSON(chatResp{Error: "DB error"})
	}
	defer rows.Close()

	type chatItem struct {
		ID                int64
		Type              string
		Name              string
		Description       string
		AvatarURL         string
		AvatarColor       string
		OwnerID           *int64
		PinnedMsgID       *int64
		CreatedAt         time.Time
		UpdatedAt         time.Time
		LastMsgID         *int64
		LastMsgContent    *string
		LastMsgType       *string
		LastMsgAt         *time.Time
		LastMsgSender     *int64
		LastMsgSenderName *string
		LastReadMsgID     int64
		IsMuted           bool
		NotifsEnabled     bool
	}

	var chats []fiber.Map
	for rows.Next() {
		var ci chatItem
		if err := rows.Scan(&ci.ID, &ci.Type, &ci.Name, &ci.Description, &ci.AvatarURL, &ci.AvatarColor, &ci.OwnerID, &ci.PinnedMsgID, &ci.CreatedAt, &ci.UpdatedAt,
			&ci.LastMsgID, &ci.LastMsgContent, &ci.LastMsgType, &ci.LastMsgAt, &ci.LastMsgSender, &ci.LastMsgSenderName,
			&ci.LastReadMsgID, &ci.IsMuted, &ci.NotifsEnabled); err != nil {
			log.Printf("chats.List: scan error: %v", err)
			continue
		}

		// Get unread count
		var unread int64
		if ci.LastMsgID != nil {
			_ = h.DB.Pool.QueryRow(c.Context(), `
				SELECT COUNT(*) FROM messages
				WHERE chat_id=$1 AND is_deleted=false AND id > $2 AND sender_id <> $3
			`, ci.ID, ci.LastReadMsgID, userID).Scan(&unread)
		}

		// Get members
		members := h.getMembers(c.UserContext(), ci.ID)
		// Get title (for private, use other user's name)
		title := ci.Name
		avatarURL := ci.AvatarURL
		avatarColor := ci.AvatarColor
		if ci.Type == "private" {
			for _, m := range members {
				if id, ok := m["id"].(int64); ok && id != userID {
					title = m["displayName"].(string)
					if title == "" {
						title = m["username"].(string)
					}
					avatarURL, _ = m["avatarUrl"].(string)
					avatarColor, _ = m["avatarColor"].(string)
					break
				}
			}
		}

		chats = append(chats, fiber.Map{
			"id":              ci.ID,
			"type":            ci.Type,
			"name":            title,
			"description":     ci.Description,
			"avatarUrl":       avatarURL,
			"avatarColor":     avatarColor,
			"ownerId":         ci.OwnerID,
			"pinnedMessageId": ci.PinnedMsgID,
			"createdAt":       ci.CreatedAt,
			"updatedAt":       ci.UpdatedAt,
			"lastMessage": fiber.Map{
				"id":         ci.LastMsgID,
				"content":    ci.LastMsgContent,
				"type":       ci.LastMsgType,
				"createdAt":  ci.LastMsgAt,
				"senderId":   ci.LastMsgSender,
				"senderName": ci.LastMsgSenderName,
			},
			"unreadCount":         unread,
			"isMuted":             ci.IsMuted,
			"notificationsEnabled": ci.NotifsEnabled,
			"members":             members,
		})
	}
	if chats == nil {
		chats = []fiber.Map{}
	}
	return c.JSON(chatResp{OK: true, Chats: chats})
}

func (h *ChatsHandler) getMembers(ctx context.Context, chatID int64) []fiber.Map {
	rows, err := h.DB.Pool.Query(ctx, `
		SELECT u.id, u.username, COALESCE(u.display_name,''), COALESCE(u.avatar_url,''), COALESCE(u.avatar_color,''), u.is_online, u.last_seen, cm.role
		FROM chat_members cm
		JOIN users u ON u.id=cm.user_id
		WHERE cm.chat_id=$1
		ORDER BY cm.joined_at ASC
	`, chatID)
	if err != nil {
		return []fiber.Map{}
	}
	defer rows.Close()

	var members []fiber.Map
	for rows.Next() {
		var id int64
		var username, displayName, avatarURL, avatarColor, role string
		var isOnline bool
		var lastSeen time.Time
		if err := rows.Scan(&id, &username, &displayName, &avatarURL, &avatarColor, &isOnline, &lastSeen, &role); err == nil {
			members = append(members, fiber.Map{
				"id":          id,
				"username":    username,
				"displayName": displayName,
				"avatarUrl":   avatarURL,
				"avatarColor": avatarColor,
				"isOnline":    isOnline,
				"lastSeen":    lastSeen,
				"role":        role,
			})
		}
	}
	return members
}

// POST /api/v1/chats
func (h *ChatsHandler) Create(c *fiber.Ctx) error {
	userID := c.Locals("userId").(int64)
	var req createChatReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(chatResp{Error: "Invalid body"})
	}

	if req.Type == "" {
		req.Type = "private"
	}
	if req.Type != "private" && req.Type != "group" && req.Type != "channel" {
		return c.JSON(chatResp{Error: "Invalid type"})
	}

	// Private chat: find or create with target user
	if req.Type == "private" {
		if req.Username == "" {
			return c.JSON(chatResp{Error: "username required for private chat"})
		}
		target := strings.ToLower(strings.TrimSpace(req.Username))

		var targetID int64
		err := h.DB.Pool.QueryRow(c.Context(), `SELECT id FROM users WHERE LOWER(username)=$1`, target).Scan(&targetID)
		if err != nil {
			return c.JSON(chatResp{Error: "User not found"})
		}
		if targetID == userID {
			return c.JSON(chatResp{Error: "Cannot create chat with yourself"})
		}

		// Check existing private chat
		var existingID int64
		err = h.DB.Pool.QueryRow(c.Context(), `
			SELECT c.id FROM chats c
			JOIN chat_members m1 ON m1.chat_id=c.id AND m1.user_id=$1
			JOIN chat_members m2 ON m2.chat_id=c.id AND m2.user_id=$2
			WHERE c.type='private'
			LIMIT 1
		`, userID, targetID).Scan(&existingID)
		if err == nil {
			return c.JSON(chatResp{OK: true, Chat: fiber.Map{"id": existingID, "existing": true}})
		}

		// Create
		var chatID int64
		err = h.DB.Pool.QueryRow(c.Context(), `
			INSERT INTO chats (type) VALUES ('private') RETURNING id
		`).Scan(&chatID)
		if err != nil {
			return c.Status(500).JSON(chatResp{Error: "DB error"})
		}
		_, err = h.DB.Pool.Exec(c.Context(), `
			INSERT INTO chat_members (chat_id, user_id, role) VALUES ($1, $2, 'member'), ($1, $3, 'member')
		`, chatID, userID, targetID)
		if err != nil {
			return c.Status(500).JSON(chatResp{Error: "DB error"})
		}
		return c.JSON(chatResp{OK: true, Chat: fiber.Map{"id": chatID, "existing": false}})
	}

	// Group/channel
	if req.Name == "" {
		return c.JSON(chatResp{Error: "Name required"})
	}
	if len(req.Name) > 128 {
		return c.JSON(chatResp{Error: "Name too long"})
	}
	color := pickColor(req.Name)

	tx, err := h.DB.Pool.Begin(c.Context())
	if err != nil {
		return c.Status(500).JSON(chatResp{Error: "DB error"})
	}
	defer tx.Rollback(c.Context())

	var chatID int64
	err = tx.QueryRow(c.Context(), `
		INSERT INTO chats (type, name, description, owner_id, avatar_color) VALUES ($1, $2, $3, $4, $5) RETURNING id
	`, req.Type, req.Name, req.Description, userID, color).Scan(&chatID)
	if err != nil {
		return c.Status(500).JSON(chatResp{Error: "DB error"})
	}

	// Add creator as owner
	_, err = tx.Exec(c.Context(), `INSERT INTO chat_members (chat_id, user_id, role) VALUES ($1, $2, 'owner')`, chatID, userID)
	if err != nil {
		return c.Status(500).JSON(chatResp{Error: "DB error"})
	}

	// Add other members
	for _, uid := range req.UserIDs {
		if uid == userID {
			continue
		}
		_, _ = tx.Exec(c.Context(), `INSERT INTO chat_members (chat_id, user_id, role) VALUES ($1, $2, 'member') ON CONFLICT DO NOTHING`, chatID, uid)
	}

	if err := tx.Commit(c.Context()); err != nil {
		return c.Status(500).JSON(chatResp{Error: "DB error"})
	}

	return c.JSON(chatResp{OK: true, Chat: fiber.Map{"id": chatID, "type": req.Type, "name": req.Name, "avatarColor": color}})
}

// GET /api/v1/chats/:id
func (h *ChatsHandler) Get(c *fiber.Ctx) error {
	userID := c.Locals("userId").(int64)
	chatID := int64Param(c, "id")

	var isMember bool
	err := h.DB.Pool.QueryRow(c.Context(), `SELECT EXISTS(SELECT 1 FROM chat_members WHERE chat_id=$1 AND user_id=$2)`, chatID, userID).Scan(&isMember)
	if err != nil || !isMember {
		return c.Status(403).JSON(chatResp{Error: "Forbidden"})
	}

	row := h.DB.Pool.QueryRow(c.Context(), `
		SELECT id, type, COALESCE(name,''), COALESCE(description,''), COALESCE(avatar_url,''), COALESCE(avatar_color,''),
		       owner_id, pinned_message_id, created_at, updated_at
		FROM chats WHERE id=$1
	`, chatID)
	var id int64
	var ctype, name, description, avatarURL, avatarColor string
	var ownerID int64
	var pinnedMsgID *int64
	var createdAt, updatedAt time.Time
	if err := row.Scan(&id, &ctype, &name, &description, &avatarURL, &avatarColor, &ownerID, &pinnedMsgID, &createdAt, &updatedAt); err != nil {
		return c.Status(404).JSON(chatResp{Error: "Chat not found"})
	}

	members := h.getMembers(c.UserContext(), chatID)
	_ = members
	return c.JSON(chatResp{OK: true, Chat: fiber.Map{
		"id":              id,
		"type":            ctype,
		"name":            name,
		"description":     description,
		"avatarUrl":       avatarURL,
		"avatarColor":     avatarColor,
		"ownerId":         ownerID,
		"pinnedMessageId": pinnedMsgID,
		"createdAt":       createdAt,
		"updatedAt":       updatedAt,
		"members":         members,
	}})
}

// POST /api/v1/chats/:id/read
func (h *ChatsHandler) MarkRead(c *fiber.Ctx) error {
	userID := c.Locals("userId").(int64)
	chatID := int64Param(c, "id")

	var maxID int64
	_ = h.DB.Pool.QueryRow(c.Context(), `SELECT COALESCE(MAX(id), 0) FROM messages WHERE chat_id=$1 AND is_deleted=false`, chatID).Scan(&maxID)
	_, err := h.DB.Pool.Exec(c.Context(), `UPDATE chat_members SET last_read_message_id=$1 WHERE chat_id=$2 AND user_id=$3`, maxID, chatID, userID)
	if err != nil {
		return c.Status(500).JSON(chatResp{Error: "DB error"})
	}
	return c.JSON(chatResp{OK: true})
}

// POST /api/v1/chats/:id/typing
func (h *ChatsHandler) Typing(c *fiber.Ctx) error {
	_ = time.Now()
	// Just an event broadcast via WS - handled in main.go via Hub
	return c.JSON(chatResp{OK: true})
}

// PATCH /api/v1/chats/:id
func (h *ChatsHandler) Update(c *fiber.Ctx) error {
	userID := c.Locals("userId").(int64)
	chatID := int64Param(c, "id")

	var ownerID int64
	err := h.DB.Pool.QueryRow(c.Context(), `SELECT COALESCE(owner_id, 0) FROM chats WHERE id=$1`, chatID).Scan(&ownerID)
	if err != nil || (ownerID != userID) {
		var isAdmin bool
		_ = h.DB.Pool.QueryRow(c.Context(), `SELECT EXISTS(SELECT 1 FROM chat_members WHERE chat_id=$1 AND user_id=$2 AND role IN ('owner','admin'))`, chatID, userID).Scan(&isAdmin)
		if !isAdmin {
			return c.Status(403).JSON(chatResp{Error: "Forbidden"})
		}
	}

	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(chatResp{Error: "Invalid body"})
	}
	if req.Name != nil {
		_, _ = h.DB.Pool.Exec(c.Context(), `UPDATE chats SET name=$1 WHERE id=$2`, *req.Name, chatID)
	}
	if req.Description != nil {
		_, _ = h.DB.Pool.Exec(c.Context(), `UPDATE chats SET description=$1 WHERE id=$2`, *req.Description, chatID)
	}
	return c.JSON(chatResp{OK: true})
}

// POST /api/v1/chats/:id/leave
func (h *ChatsHandler) Leave(c *fiber.Ctx) error {
	userID := c.Locals("userId").(int64)
	chatID := int64Param(c, "id")
	_, err := h.DB.Pool.Exec(c.Context(), `DELETE FROM chat_members WHERE chat_id=$1 AND user_id=$2`, chatID, userID)
	if err != nil {
		return c.Status(500).JSON(chatResp{Error: "DB error"})
	}
	return c.JSON(chatResp{OK: true})
}
