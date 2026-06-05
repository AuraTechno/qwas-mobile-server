package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AuraTechno/qwas-mobile-server/internal/config"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type MediaHandler struct {
	Cfg *config.Config
}

func NewMediaHandler(c *config.Config) *MediaHandler {
	return &MediaHandler{Cfg: c}
}

func (h *MediaHandler) Upload(c *fiber.Ctx) error {
	userID := c.Locals("userId").(int64)

	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"ok": false, "error": "No file"})
	}
	if file.Size > h.Cfg.MaxUpload {
		return c.Status(413).JSON(fiber.Map{"ok": false, "error": "File too large"})
	}

	src, err := file.Open()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"ok": false, "error": "Cannot read file"})
	}
	defer src.Close()

	// Compute hash + size
	hasher := sha256.New()
	tmp := filepath.Join(os.TempDir(), "qwas-upload-"+uuid.NewString())
	dst, err := os.Create(tmp)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"ok": false, "error": "Temp error"})
	}
	written, err := io.Copy(io.MultiWriter(dst, hasher), src)
	dst.Close()
	if err != nil {
		os.Remove(tmp)
		return c.Status(500).JSON(fiber.Map{"ok": false, "error": "Write error"})
	}
	if written > h.Cfg.MaxUpload {
		os.Remove(tmp)
		return c.Status(413).JSON(fiber.Map{"ok": false, "error": "File too large"})
	}
	hash := hex.EncodeToString(hasher.Sum(nil))[:16]

	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext == "" {
		ext = ".bin"
	}
	// Map mime to ext if needed
	mime := file.Header.Get("Content-Type")
	if mime == "" {
		mime = "application/octet-stream"
	}

	// Organize by date + hash
	dateDir := time.Now().Format("2006/01/02")
	absDir := filepath.Join(h.Cfg.MediaDir, fmt.Sprintf("%d", userID), dateDir)
	if err := os.MkdirAll(absDir, 0755); err != nil {
		os.Remove(tmp)
		return c.Status(500).JSON(fiber.Map{"ok": false, "error": "Dir error"})
	}
	finalName := hash + ext
	finalPath := filepath.Join(absDir, finalName)

	if err := os.Rename(tmp, finalPath); err != nil {
		os.Remove(tmp)
		return c.Status(500).JSON(fiber.Map{"ok": false, "error": "Move error"})
	}

	url := fmt.Sprintf("%s/media/%d/%s/%s", h.Cfg.PublicURL, userID, dateDir, finalName)
	relURL := fmt.Sprintf("/media/%d/%s/%s", userID, dateDir, finalName)

	return c.JSON(fiber.Map{
		"ok":       true,
		"url":      url,
		"relUrl":   relURL,
		"size":     written,
		"mime":     mime,
		"filename": file.Filename,
	})
}

// Serve static files from MediaDir under /media/* path
func (h *MediaHandler) Serve(c *fiber.Ctx) error {
	rel := c.Params("*")
	if rel == "" {
		return c.Status(404).SendString("Not found")
	}
	// Prevent directory traversal
	abs := filepath.Join(h.Cfg.MediaDir, rel)
	relBase, err := filepath.Abs(h.Cfg.MediaDir)
	if err != nil {
		return c.Status(500).SendString("Error")
	}
	absClean, err := filepath.Abs(abs)
	if err != nil {
		return c.Status(400).SendString("Invalid path")
	}
	if !strings.HasPrefix(absClean, relBase) {
		return c.Status(403).SendString("Forbidden")
	}
	if _, err := os.Stat(absClean); err != nil {
		return c.Status(404).SendString("Not found")
	}
	c.Set("Content-Type", mimeByExt(filepath.Ext(absClean)))
	c.Set("Cache-Control", "public, max-age=31536000, immutable")
	return c.SendFile(absClean)
}

func mimeByExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".mp4":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mp3":
		return "audio/mpeg"
	case ".ogg":
		return "audio/ogg"
	case ".m4a":
		return "audio/mp4"
	case ".wav":
		return "audio/wav"
	case ".pdf":
		return "application/pdf"
	case ".txt":
		return "text/plain"
	}
	return "application/octet-stream"
}

// Version endpoint for app auto-update
func (h *MediaHandler) Version(c *fiber.Ctx) error {
	// Read /var/www/qwas-app-releases/version.json if exists
	path := filepath.Join("/var/www/qwas-app-releases", "version.json")
	if data, err := os.ReadFile(path); err == nil {
		c.Set("Content-Type", "application/json")
		c.Set("Cache-Control", "no-store")
		return c.Send(data)
	}
	return c.JSON(fiber.Map{
		"ok":      true,
		"version": "0.0.0",
		"url":     "",
	})
}

// Static fallback for missing /app/latest.apk
func (h *MediaHandler) LatestApk(c *fiber.Ctx) error {
	path := filepath.Join("/var/www/qwas-app-releases", "latest.apk")
	if _, err := os.Stat(path); err != nil {
		return c.Status(404).JSON(fiber.Map{"ok": false, "error": "No APK available yet"})
	}
	c.Set("Content-Type", "application/vnd.android.package-archive")
	c.Set("Content-Disposition", "attachment; filename=\"qwas-mobile.apk\"")
	return c.SendFile(path)
}
