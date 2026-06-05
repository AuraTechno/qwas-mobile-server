# QWAS Mobile Server

Go (Fiber v2) backend for the QWAS Mobile React Native app.

## Features

- **Auth**: username + password, JWT, bcrypt, multi-session management
- **Real-time**: WebSocket hub (online status, typing, new messages, WebRTC signaling)
- **Chats**: private, group, channel types; auto-resolve titles for private chats
- **Messages**: text, image, video, voice, file, system; pagination, edit, delete, reply
- **Reactions**: per-message emoji with toggle
- **Pinned messages**: per-chat, restricted to members
- **Media**: chunked upload to local FS, served from `/media/*`
- **Calls (WebRTC)**: TURN/STUN credentials via `/api/v1/ice`, signaling over WS
- **App auto-update**: serves `/app/version.json` and `/app/latest.apk`
- **Migrations**: SQL files in `migrations/` applied on startup

## Tech Stack

- Go 1.22, Fiber v2
- PostgreSQL 16 (pgx/v5 pool)
- gorilla/websocket (via gofiber/contrib/websocket)
- bcrypt + JWT (HS256)

## Quick Start (production)

```bash
# On a fresh Ubuntu 24.04 server:
apt update && apt install -y postgresql coturn ufw curl wget git
# Install Go from https://go.dev/dl/

# Setup DB
sudo -u postgres psql -c "CREATE USER qwas_app WITH PASSWORD '...';"
sudo -u postgres psql -c "CREATE DATABASE qwas_app OWNER qwas_app;"

# Clone & install
git clone https://github.com/AuraTechno/qwas-mobile-server.git /opt/qwas-mobile-server
cd /opt/qwas-mobile-server
go build -o qwas-app-server ./cmd/server
cp scripts/qwas-app.service /etc/systemd/system/
cp scripts/update.sh /usr/local/bin/qwas-app-update
systemctl enable --now qwas-app
```

## Update

```bash
qwas-app-update
```

This script: `git pull` → `go build` → restart service → health check.

## Environment

Copy `.env.example` to `.env` and fill in. The systemd unit reads from `/etc/qwas-app.env`.

Required: `QWAS_DB_PASS`, `QWAS_JWT_SECRET`. Optional: `QWAS_TURN_*`, `QWAS_PUBLIC_URL`.

## API Endpoints

### Public
- `GET /health`
- `GET /app/version.json` — app auto-update manifest
- `GET /app/latest.apk` — APK download
- `GET /media/*` — uploaded media

### Auth
- `GET /api/v1/auth/check-username?username=...`
- `POST /api/v1/auth/register` — `{username, password, displayName}`
- `POST /api/v1/auth/login` — `{username, password}`
- `GET /api/v1/auth/me`
- `POST /api/v1/auth/logout`
- `GET /api/v1/auth/sessions`
- `DELETE /api/v1/auth/sessions/:id`
- `POST /api/v1/auth/terminate-all`

### Users
- `GET /api/v1/users/search?q=...`
- `GET /api/v1/users/:username`
- `PATCH /api/v1/users/me`

### Chats
- `GET /api/v1/chats`
- `POST /api/v1/chats` — `{type, name, userIds[], username?}`
- `GET /api/v1/chats/:id`
- `PATCH /api/v1/chats/:id`
- `POST /api/v1/chats/:id/leave`
- `POST /api/v1/chats/:id/read`
- `POST /api/v1/chats/:id/typing`

### Messages
- `GET /api/v1/chats/:id/messages?limit=50&before=ID`
- `POST /api/v1/chats/:id/messages` — `{type, content, mediaUrl?, mediaMeta?, replyToId?}`
- `PATCH /api/v1/messages/:id` — edit
- `DELETE /api/v1/messages/:id`

### Reactions
- `PUT /api/v1/messages/:id/reactions` — `{emoji}`
- `GET /api/v1/messages/:id/reactions`

### Pinned
- `POST /api/v1/chats/:id/pin-message` — `{messageId}`
- `DELETE /api/v1/chats/:id/pin-message`

### Media
- `POST /api/v1/media/upload` — multipart `file`

### Calls (WebRTC)
- `GET /api/v1/ice` — TURN/STUN credentials
- `POST /api/v1/chats/:id/calls` — `{type: audio|video}`
- `POST /api/v1/calls/:id/accept`
- `POST /api/v1/calls/:id/reject`
- `POST /api/v1/calls/:id/end`

## WebSocket

Connect to `wss://api-qwas.academinctools.pw/ws?token=<JWT>`.

**Client → Server:**
- `{type: "ping"}`
- `{type: "typing", payload: {chatId, isTyping}}`
- `{type: "call_offer|call_answer|call_ice|call_end", payload: {targetUserId, chatId, data}}`

**Server → Client:**
- `{event: "new_message", payload: {...}}`
- `{event: "message_edited", payload: {...}}`
- `{event: "message_deleted", payload: {...}}`
- `{event: "message_reaction", payload: {...}}`
- `{event: "chat_updated", payload: {...}}`
- `{event: "pinned_updated", payload: {...}}`
- `{event: "call_incoming", payload: {...}}`
- `{event: "call_accepted|rejected|ended", payload: {...}}`
- `{event: "webrtc_call_offer|answer|ice", payload: {fromUserId, chatId, data}}`

## License

Private.
