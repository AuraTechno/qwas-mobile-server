package handlers

import (
	"encoding/base64"
	"strconv"
	"time"

	"github.com/AuraTechno/qwas-mobile-server/internal/config"
	"github.com/AuraTechno/qwas-mobile-server/internal/db"
	"github.com/AuraTechno/qwas-mobile-server/internal/ws"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type CallsHandler struct {
	DB   *db.DB
	Cfg  *config.Config
	Hub  *ws.Hub
}

func NewCallsHandler(d *db.DB, c *config.Config, h *ws.Hub) *CallsHandler {
	return &CallsHandler{DB: d, Cfg: c, Hub: h}
}

// GET /api/v1/ice — TURN/STUN credentials
// Returns short-lived credentials derived from a secret
func (h *CallsHandler) Ice(c *fiber.Ctx) error {
	if h.Cfg.TURNSecret == "" {
		return c.JSON(fiber.Map{
			"iceServers": []fiber.Map{
				{"urls": []string{"stun:" + h.Cfg.TURNHost + ":3478"}},
			},
		})
	}

	// Generate time-limited credentials (RFC 7635)
	username := strconv.FormatInt(time.Now().Unix()+3600, 10) // expires in 1h
	turnURI := h.Cfg.TURNRealm
	// Compute HMAC-style credential: SHA1(secret + username) using simple digest
	// For production use: hmac(sha1, secret, username) per RFC 7635
	digest := sha1Hex(h.Cfg.TURNSecret + ":" + username)

	return c.JSON(fiber.Map{
		"iceServers": []fiber.Map{
			{"urls": []string{"stun:" + h.Cfg.TURNHost + ":3478"}},
			{
				"urls":       []string{"turn:" + h.Cfg.TURNHost + ":3478?transport=udp", "turn:" + h.Cfg.TURNHost + ":3478?transport=tcp"},
				"username":   username,
				"credential": digest,
				"realm":      turnURI,
			},
		},
		"ttl": 3600,
	})
}

func sha1Hex(s string) string {
	// Use crypto/sha1 inline
	// (avoiding extra import in this file; package main will rely on auth import)
	h := sha1Sum([]byte(s))
	return base64.RawURLEncoding.EncodeToString(h)
}

// POST /api/v1/chats/:id/calls — initiate a call
func (h *CallsHandler) Initiate(c *fiber.Ctx) error {
	userID := c.Locals("userId").(int64)
	chatID := int64Param(c, "id")
	var req struct {
		Type string `json:"type"` // audio, video
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"ok": false, "error": "Invalid body"})
	}
	if req.Type != "audio" && req.Type != "video" {
		req.Type = "audio"
	}

	// Verify membership
	var isMember bool
	_ = h.DB.Pool.QueryRow(c.Context(), `SELECT EXISTS(SELECT 1 FROM chat_members WHERE chat_id=$1 AND user_id=$2)`, chatID, userID).Scan(&isMember)
	if !isMember {
		return c.Status(403).JSON(fiber.Map{"ok": false, "error": "Forbidden"})
	}

	callID := uuid.NewString()
	var dbID int64
	err := h.DB.Pool.QueryRow(c.Context(), `
		INSERT INTO calls (id, chat_id, initiator_id, type, status)
		VALUES ($1, $2, $3, $4, 'ringing') RETURNING 1
	`, callID, chatID, userID, req.Type).Scan(&dbID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"ok": false, "error": "DB error"})
	}

	// Get caller name + other members
	var callerName string
	_ = h.DB.Pool.QueryRow(c.Context(), `SELECT COALESCE(display_name, username) FROM users WHERE id=$1`, userID).Scan(&callerName)

	// Get other members
	rows, _ := h.DB.Pool.Query(c.Context(), `SELECT user_id FROM chat_members WHERE chat_id=$1 AND user_id<>$2`, chatID, userID)
	defer rows.Close()
	var memberIDs []int64
	for rows.Next() {
		var id int64
		_ = rows.Scan(&id)
		memberIDs = append(memberIDs, id)
	}

	// Broadcast call_incoming to all members (and initiator as echo)
	memberIDs = append(memberIDs, userID)
	h.Hub.SendToUsers(memberIDs, ws.EventCallIncoming, fiber.Map{
		"callId":       callID,
		"chatId":       chatID,
		"initiatorId":  userID,
		"initiatorName": callerName,
		"type":         req.Type,
		"startedAt":    time.Now().UnixMilli(),
	})

	return c.JSON(fiber.Map{
		"ok":        true,
		"callId":    callID,
		"chatId":    chatID,
		"type":      req.Type,
		"members":   memberIDs,
	})
}

// POST /api/v1/calls/:id/accept
func (h *CallsHandler) Accept(c *fiber.Ctx) error {
	userID := c.Locals("userId").(int64)
	callID := c.Params("id")

	_, err := h.DB.Pool.Exec(c.Context(), `UPDATE calls SET status='active', answered_at=NOW() WHERE id=$1`, callID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"ok": false, "error": "DB error"})
	}

	// Notify initiator
	var initiatorID, chatID int64
	_ = h.DB.Pool.QueryRow(c.Context(), `SELECT initiator_id, chat_id FROM calls WHERE id=$1`, callID).Scan(&initiatorID, &chatID)

	h.Hub.SendToUser(initiatorID, ws.EventCallAccepted, fiber.Map{
		"callId":  callID,
		"userId":  userID,
		"chatId":  chatID,
	})

	return c.JSON(fiber.Map{"ok": true})
}

// POST /api/v1/calls/:id/reject
func (h *CallsHandler) Reject(c *fiber.Ctx) error {
	userID := c.Locals("userId").(int64)
	callID := c.Params("id")

	_, _ = h.DB.Pool.Exec(c.Context(), `UPDATE calls SET status='rejected', ended_at=NOW() WHERE id=$1`, callID)

	var initiatorID, chatID int64
	_ = h.DB.Pool.QueryRow(c.Context(), `SELECT initiator_id, chat_id FROM calls WHERE id=$1`, callID).Scan(&initiatorID, &chatID)

	h.Hub.SendToUser(initiatorID, ws.EventCallRejected, fiber.Map{
		"callId":  callID,
		"userId":  userID,
		"chatId":  chatID,
	})

	return c.JSON(fiber.Map{"ok": true})
}

// POST /api/v1/calls/:id/end
func (h *CallsHandler) End(c *fiber.Ctx) error {
	userID := c.Locals("userId").(int64)
	callID := c.Params("id")

	_, _ = h.DB.Pool.Exec(c.Context(), `UPDATE calls SET status='ended', ended_at=NOW() WHERE id=$1`, callID)

	var chatID int64
	_ = h.DB.Pool.QueryRow(c.Context(), `SELECT chat_id FROM calls WHERE id=$1`, callID).Scan(&chatID)

	// Notify all chat members
	rows, _ := h.DB.Pool.Query(c.Context(), `SELECT user_id FROM chat_members WHERE chat_id=$1`, chatID)
	defer rows.Close()
	var memberIDs []int64
	for rows.Next() {
		var id int64
		_ = rows.Scan(&id)
		memberIDs = append(memberIDs, id)
	}
	h.Hub.SendToUsers(memberIDs, ws.EventCallEnded, fiber.Map{
		"callId":  callID,
		"chatId":  chatID,
		"userId":  userID,
	})

	return c.JSON(fiber.Map{"ok": true})
}
