package handlers

import (
	"context"
	"errors"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/AuraTechno/qwas-mobile-server/internal/auth"
	"github.com/AuraTechno/qwas-mobile-server/internal/db"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

var usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]{3,32}$`)

type AuthHandler struct {
	DB   *db.DB
	Auth *auth.Manager
}

func NewAuthHandler(d *db.DB, a *auth.Manager) *AuthHandler {
	return &AuthHandler{DB: d, Auth: a}
}

type registerReq struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"displayName"`
}

type authResp struct {
	OK       bool   `json:"ok"`
	Token    string `json:"token,omitempty"`
	UserID   int64  `json:"userId,omitempty"`
	Username string `json:"username,omitempty"`
	Error    string `json:"error,omitempty"`
}

func (h *AuthHandler) CheckUsername(c *fiber.Ctx) error {
	username := strings.ToLower(strings.TrimSpace(c.Query("username")))
	if !usernameRegex.MatchString(username) {
		return c.JSON(fiber.Map{"ok": false, "available": false, "error": "Invalid format (3-32 chars, a-z 0-9 _)"})
	}

	var exists bool
	err := h.DB.Pool.QueryRow(c.Context(), `SELECT EXISTS(SELECT 1 FROM users WHERE LOWER(username)=$1)`, username).Scan(&exists)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"ok": false, "error": "DB error"})
	}
	return c.JSON(fiber.Map{"ok": true, "available": !exists})
}

func (h *AuthHandler) Register(c *fiber.Ctx) error {
	var req registerReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"ok": false, "error": "Invalid body"})
	}
	req.Username = strings.ToLower(strings.TrimSpace(req.Username))

	if !usernameRegex.MatchString(req.Username) {
		return c.JSON(authResp{Error: "Username 3-32 chars, a-z 0-9 _"})
	}
	if len(req.Password) < 8 {
		return c.JSON(authResp{Error: "Password must be at least 8 characters"})
	}
	if req.DisplayName == "" {
		req.DisplayName = req.Username
	}

	hash, err := h.Auth.HashPassword(req.Password)
	if err != nil {
		return c.JSON(authResp{Error: err.Error()})
	}

	color := pickColor(req.Username)
	tx, err := h.DB.Pool.Begin(c.Context())
	if err != nil {
		return c.Status(500).JSON(authResp{Error: "DB error"})
	}
	defer tx.Rollback(context.Background())

	var userID int64
	err = tx.QueryRow(c.Context(), `
		INSERT INTO users (username, password_hash, display_name, avatar_color)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, req.Username, hash, req.DisplayName, color).Scan(&userID)
	if err != nil {
		if strings.Contains(err.Error(), "unique") {
			return c.JSON(authResp{Error: "Username already taken"})
		}
		return c.Status(500).JSON(authResp{Error: "DB error"})
	}

	tokenID := uuid.NewString()
	deviceInfo := string(c.Context().UserAgent())
	ip := c.IP()
	token, err := h.Auth.IssueToken(userID, req.Username, tokenID)
	if err != nil {
		return c.Status(500).JSON(authResp{Error: "Token error"})
	}
	tokenHash := h.Auth.HashToken(token)

	_, err = tx.Exec(c.Context(), `
		INSERT INTO sessions (id, user_id, token_hash, device_info, ip)
		VALUES ($1, $2, $3, $4, $5)
	`, tokenID, userID, tokenHash, deviceInfo, ip)
	if err != nil {
		return c.Status(500).JSON(authResp{Error: "Session error"})
	}

	if err := tx.Commit(c.Context()); err != nil {
		return c.Status(500).JSON(authResp{Error: "DB error"})
	}

	return c.JSON(authResp{OK: true, Token: token, UserID: userID, Username: req.Username})
}

func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req registerReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"ok": false, "error": "Invalid body"})
	}
	req.Username = strings.ToLower(strings.TrimSpace(req.Username))

	var userID int64
	var hash string
	err := h.DB.Pool.QueryRow(c.Context(), `SELECT id, password_hash FROM users WHERE LOWER(username)=$1`, req.Username).Scan(&userID, &hash)
	if err != nil {
		return c.JSON(authResp{Error: "Invalid username or password"})
	}
	if err := h.Auth.CheckPassword(hash, req.Password); err != nil {
		return c.JSON(authResp{Error: "Invalid username or password"})
	}

	tokenID := uuid.NewString()
	deviceInfo := string(c.Context().UserAgent())
	ip := c.IP()
	token, err := h.Auth.IssueToken(userID, req.Username, tokenID)
	if err != nil {
		return c.Status(500).JSON(authResp{Error: "Token error"})
	}
	tokenHash := h.Auth.HashToken(token)

	_, err = h.DB.Pool.Exec(c.Context(), `
		INSERT INTO sessions (id, user_id, token_hash, device_info, ip)
		VALUES ($1, $2, $3, $4, $5)
	`, tokenID, userID, tokenHash, deviceInfo, ip)
	if err != nil {
		return c.Status(500).JSON(authResp{Error: "Session error"})
	}

	_, _ = h.DB.Pool.Exec(c.Context(), `UPDATE users SET is_online=true, last_seen=NOW() WHERE id=$1`, userID)

	return c.JSON(authResp{OK: true, Token: token, UserID: userID, Username: req.Username})
}

func (h *AuthHandler) Me(c *fiber.Ctx) error {
	userID := c.Locals("userId").(int64)
	row := h.DB.Pool.QueryRow(c.Context(), `
		SELECT id, username, COALESCE(display_name,''), COALESCE(bio,''), COALESCE(avatar_url,''), COALESCE(avatar_color,''), is_online, last_seen, created_at
		FROM users WHERE id=$1
	`, userID)

	var u struct {
		ID, Username, DisplayName, Bio, AvatarURL, AvatarColor, IsOnline, LastSeen, CreatedAt
	}
	err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Bio, &u.AvatarURL, &u.AvatarColor, &u.IsOnline, &u.LastSeen, &u.CreatedAt)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"ok": false, "error": "User not found"})
	}
	return c.JSON(fiber.Map{
		"ok":          true,
		"id":          u.ID,
		"username":    u.Username,
		"displayName": u.DisplayName,
		"bio":         u.Bio,
		"avatarUrl":   u.AvatarURL,
		"avatarColor": u.AvatarColor,
		"isOnline":    u.IsOnline,
		"lastSeen":    u.LastSeen,
		"createdAt":   u.CreatedAt,
	})
}

func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	tokenHash := c.Locals("tokenHash").(string)
	_, err := h.DB.Pool.Exec(c.Context(), `DELETE FROM sessions WHERE token_hash=$1`, tokenHash)
	if err != nil && !errors.Is(err, net.ErrClosed) {
		return c.Status(500).JSON(fiber.Map{"ok": false, "error": "DB error"})
	}
	return c.JSON(fiber.Map{"ok": true})
}

func (h *AuthHandler) GetSessions(c *fiber.Ctx) error {
	userID := c.Locals("userId").(int64)
	rows, err := h.DB.Pool.Query(c.Context(), `
		SELECT id, COALESCE(device_info,''), COALESCE(host(ip),''), last_active, created_at
		FROM sessions WHERE user_id=$1
		ORDER BY last_active DESC
	`, userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"ok": false, "error": "DB error"})
	}
	defer rows.Close()

	var sessions []fiber.Map
	for rows.Next() {
		var s struct {
			ID, DeviceInfo, IP, LastActive, CreatedAt
		}
		if err := rows.Scan(&s.ID, &s.DeviceInfo, &s.IP, &s.LastActive, &s.CreatedAt); err == nil {
			sessions = append(sessions, fiber.Map{
				"id":         s.ID,
				"deviceInfo": s.DeviceInfo,
				"ip":         s.IP,
				"lastActive": s.LastActive,
				"createdAt":  s.CreatedAt,
			})
		}
	}
	return c.JSON(fiber.Map{"ok": true, "sessions": sessions})
}

func (h *AuthHandler) TerminateSession(c *fiber.Ctx) error {
	userID := c.Locals("userId").(int64)
	sessionID := c.Params("id")
	res, err := h.DB.Pool.Exec(c.Context(), `DELETE FROM sessions WHERE id=$1 AND user_id=$2`, sessionID, userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"ok": false, "error": "DB error"})
	}
	if res.RowsAffected() == 0 {
		return c.Status(404).JSON(fiber.Map{"ok": false, "error": "Session not found"})
	}
	return c.JSON(fiber.Map{"ok": true})
}

func (h *AuthHandler) TerminateAllSessions(c *fiber.Ctx) error {
	userID := c.Locals("userId").(int64)
	currentHash := c.Locals("tokenHash").(string)

	_, err := h.DB.Pool.Exec(c.Context(), `DELETE FROM sessions WHERE user_id=$1 AND token_hash<>$2`, userID, currentHash)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"ok": false, "error": "DB error"})
	}
	return c.JSON(fiber.Map{"ok": true})
}

func pickColor(username string) string {
	colors := []string{
		"#e17076", "#7bc862", "#65aadd", "#a695e7", "#ee7aae",
		"#6ec9cb", "#faa774", "#94c47d", "#85a1d4", "#cd7f8e",
	}
	hash := 0
	for _, r := range username {
		hash = (hash*31 + int(r)) & 0x7fffffff
	}
	return colors[hash%len(colors)]
}

func int64Param(c *fiber.Ctx, name string) int64 {
	v, _ := strconv.ParseInt(c.Params(name), 10, 64)
	return v
}
