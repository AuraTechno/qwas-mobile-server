package handlers

import (
	"strings"
	"time"

	"github.com/AuraTechno/qwas-mobile-server/internal/db"
	"github.com/gofiber/fiber/v2"
)

type UsersHandler struct {
	DB *db.DB
}

func NewUsersHandler(d *db.DB) *UsersHandler {
	return &UsersHandler{DB: d}
}

// GET /api/v1/users/search?q=...
func (h *UsersHandler) Search(c *fiber.Ctx) error {
	q := strings.ToLower(strings.TrimSpace(c.Query("q", "")))
	if len(q) < 2 {
		return c.JSON(fiber.Map{"ok": true, "users": []fiber.Map{}})
	}
	rows, err := h.DB.Pool.Query(c.Context(), `
		SELECT id, username, COALESCE(display_name,''), COALESCE(avatar_url,''), COALESCE(avatar_color,''), is_online, last_seen
		FROM users
		WHERE LOWER(username) LIKE $1 OR LOWER(display_name) LIKE $1
		ORDER BY is_online DESC, last_seen DESC
		LIMIT 30
	`, q+"%")
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"ok": false, "error": "DB error"})
	}
	defer rows.Close()

	var users []fiber.Map
	for rows.Next() {
		var id int64
		var username, displayName, avatarURL, avatarColor string
		var isOnline bool
		var lastSeen time.Time
		if err := rows.Scan(&id, &username, &displayName, &avatarURL, &avatarColor, &isOnline, &lastSeen); err == nil {
			users = append(users, fiber.Map{
				"id":          id,
				"username":    username,
				"displayName": displayName,
				"avatarUrl":   avatarURL,
				"avatarColor": avatarColor,
				"isOnline":    isOnline,
				"lastSeen":    lastSeen,
			})
		}
	}
	if users == nil {
		users = []fiber.Map{}
	}
	return c.JSON(fiber.Map{"ok": true, "users": users})
}

// GET /api/v1/users/:username
func (h *UsersHandler) GetByUsername(c *fiber.Ctx) error {
	username := strings.ToLower(strings.TrimSpace(c.Params("username")))
	var id int64
	var uname, displayName, bio, avatarURL, avatarColor string
	var isOnline bool
	var lastSeen time.Time
	err := h.DB.Pool.QueryRow(c.Context(), `
		SELECT id, username, COALESCE(display_name,''), COALESCE(bio,''), COALESCE(avatar_url,''), COALESCE(avatar_color,''), is_online, last_seen
		FROM users WHERE LOWER(username)=$1
	`, username).Scan(&id, &uname, &displayName, &bio, &avatarURL, &avatarColor, &isOnline, &lastSeen)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"ok": false, "error": "User not found"})
	}
	return c.JSON(fiber.Map{
		"ok":          true,
		"id":          id,
		"username":    uname,
		"displayName": displayName,
		"bio":         bio,
		"avatarUrl":   avatarURL,
		"avatarColor": avatarColor,
		"isOnline":    isOnline,
		"lastSeen":    lastSeen,
	})
}

type updateMeReq struct {
	DisplayName *string `json:"displayName"`
	Bio         *string `json:"bio"`
	AvatarURL   *string `json:"avatarUrl"`
}

// PATCH /api/v1/users/me
func (h *UsersHandler) UpdateMe(c *fiber.Ctx) error {
	userID := c.Locals("userId").(int64)
	var req updateMeReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"ok": false, "error": "Invalid body"})
	}

	if req.DisplayName != nil {
		if len(*req.DisplayName) == 0 || len(*req.DisplayName) > 128 {
			return c.JSON(fiber.Map{"ok": false, "error": "Display name must be 1-128 chars"})
		}
		_, err := h.DB.Pool.Exec(c.Context(), `UPDATE users SET display_name=$1 WHERE id=$2`, *req.DisplayName, userID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"ok": false, "error": "DB error"})
		}
	}
	if req.Bio != nil {
		if len(*req.Bio) > 500 {
			return c.JSON(fiber.Map{"ok": false, "error": "Bio too long (max 500)"})
		}
		_, _ = h.DB.Pool.Exec(c.Context(), `UPDATE users SET bio=$1 WHERE id=$2`, *req.Bio, userID)
	}
	if req.AvatarURL != nil {
		_, _ = h.DB.Pool.Exec(c.Context(), `UPDATE users SET avatar_url=$1 WHERE id=$2`, *req.AvatarURL, userID)
	}
	return c.JSON(fiber.Map{"ok": true})
}
