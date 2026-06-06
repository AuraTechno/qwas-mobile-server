package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/AuraTechno/qwas-mobile-server/internal/auth"
	"github.com/AuraTechno/qwas-mobile-server/internal/config"
	"github.com/AuraTechno/qwas-mobile-server/internal/db"
	"github.com/AuraTechno/qwas-mobile-server/internal/handlers"
	"github.com/AuraTechno/qwas-mobile-server/internal/ws"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// DB
	d, err := db.New(ctx, cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPass, cfg.DBName)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer d.Close()

	// Migrations
	if err := d.RunMigrations(ctx, "./migrations"); err != nil {
		log.Fatalf("migrations: %v", err)
	}

	// Auth
	am := auth.New(cfg.JWTSecret, cfg.JWTTTL)

	// WebSocket hub
	hub := ws.NewHub()
	wsH := &ws.Handler{Hub: hub, Auth: am, DB: d.Pool}

	// Fiber
	app := fiber.New(fiber.Config{
		AppName:               "qwas-mobile-server",
		BodyLimit:             int(cfg.MaxUpload),
		DisableStartupMessage: false,
		ReadTimeout:           30 * time.Second,
		WriteTimeout:          30 * time.Second,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			return c.Status(code).JSON(fiber.Map{"ok": false, "error": err.Error()})
		},
	})

	app.Use(recover.New())
	app.Use(logger.New(logger.Config{
		Format: "${time} ${ip} ${method} ${path} ${status} ${latency}\n",
	}))
	app.Use(cors.New(cors.Config{
		AllowOrigins:     joinStrings(cfg.CORSOrigins, ","),
		AllowMethods:     "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		AllowHeaders:     "Content-Type,Authorization,X-Auth-Token",
		AllowCredentials: false,
		MaxAge:           3600,
	}))

	// Health
	app.Get("/health", func(c *fiber.Ctx) error {
		if err := d.Pool.Ping(c.Context()); err != nil {
			return c.Status(503).JSON(fiber.Map{"ok": false, "error": "db down"})
		}
		return c.JSON(fiber.Map{"ok": true, "service": "qwas-mobile-server", "ts": time.Now().UnixMilli()})
	})

	// Version & APK
	mh := handlers.NewMediaHandler(cfg)
	app.Get("/app/version.json", mh.Version)
	app.Get("/app/latest.apk", mh.LatestApk)
	app.Get("/app/*", func(c *fiber.Ctx) error {
		return c.SendFile(filepath.Join("/var/www/qwas-app-releases", c.Params("*")))
	})

	// Static media (served directly too, for clients that don't go through publicUrl)
	app.Get("/media/*", mh.Serve)

	// WS
	app.Use("/ws", func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})
	app.Get("/ws", websocket.New(wsH.Handle, websocket.Config{
		HandshakeTimeout: 10 * time.Second,
	}))

	// API v1
	api := app.Group("/api/v1", limiter.New(limiter.Config{
		Max:        120,
		Expiration: 1 * time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},
	}))

	// Auth
	ah := handlers.NewAuthHandler(d, am)
	api.Get("/auth/check-username", ah.CheckUsername)
	api.Post("/auth/register", ah.Register)
	api.Post("/auth/login", ah.Login)

	// Authenticated routes
	authed := api.Group("", am.Middleware())
	authed.Post("/auth/logout", ah.Logout)
	authed.Get("/auth/me", ah.Me)
	authed.Get("/auth/sessions", ah.GetSessions)
	authed.Delete("/auth/sessions/:id", ah.TerminateSession)
	authed.Post("/auth/terminate-all", ah.TerminateAllSessions)

	// Users
	uh := handlers.NewUsersHandler(d)
	authed.Get("/users/search", uh.Search)
	authed.Get("/users/:username", uh.GetByUsername)
	authed.Patch("/users/me", uh.UpdateMe)

	// Chats
	ch := handlers.NewChatsHandler(d)
	authed.Get("/chats", ch.List)
	authed.Get("/chats/self", ch.Self)
	authed.Post("/chats", ch.Create)
	authed.Get("/chats/:id", ch.Get)
	authed.Patch("/chats/:id", ch.Update)
	authed.Post("/chats/:id/leave", ch.Leave)
	authed.Post("/chats/:id/read", ch.MarkRead)
	authed.Post("/chats/:id/typing", ch.Typing)
	authed.Post("/chats/:id/mute", ch.Mute)

	// Messages
	mh2 := handlers.NewMessagesHandler(d, hub)
	authed.Get("/chats/:id/messages", mh2.List)
	authed.Post("/chats/:id/messages", mh2.Send)
	authed.Delete("/messages/:id", mh2.Delete)
	authed.Patch("/messages/:id", mh2.Edit)

	// Reactions + Pinned
	rh := handlers.NewReactionsHandler(d, hub)
	ph := handlers.NewPinnedHandler(d)
	authed.Put("/messages/:id/reactions", rh.Toggle)
	authed.Get("/messages/:id/reactions", rh.List)
	authed.Post("/chats/:id/pin-message", ph.Pin)
	authed.Delete("/chats/:id/pin-message", ph.Unpin)

	// Polls
	poh := handlers.NewPollsHandler(d, hub)
	authed.Get("/polls/:id", poh.Get)
	authed.Post("/polls/:id/vote", poh.Vote)

	// Media
	authed.Post("/media/upload", mh.Upload)

	// Calls + ICE
	callH := handlers.NewCallsHandler(d, cfg, hub)
	authed.Get("/ice", callH.Ice)
	authed.Post("/chats/:id/calls", callH.Initiate)
	authed.Post("/calls/:id/accept", callH.Accept)
	authed.Get("/calls", callH.List)
	authed.Post("/calls/:id/reject", callH.Reject)
	authed.Post("/calls/:id/end", callH.End)

	// 404
	app.Use(func(c *fiber.Ctx) error {
		return c.Status(404).JSON(fiber.Map{"ok": false, "error": "Not found"})
	})

	// Graceful shutdown
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		log.Println("shutdown: signal received")
		_ = app.ShutdownWithTimeout(15 * time.Second)
		cancel()
	}()

	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("listening on %s (env=%s)", addr, cfg.Env)
	if err := app.Listen(addr); err != nil {
		log.Fatalf("fiber: %v", err)
	}
}

func joinStrings(arr []string, sep string) string {
	out := ""
	for i, s := range arr {
		if i > 0 {
			out += sep
		}
		out += s
	}
	return out
}
