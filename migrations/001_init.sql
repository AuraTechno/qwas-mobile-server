-- 001_init.sql
-- Initial schema for QWAS Mobile

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ============= USERS =============
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(32) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    display_name VARCHAR(128),
    bio TEXT,
    avatar_url TEXT,
    avatar_color VARCHAR(16),
    is_online BOOLEAN NOT NULL DEFAULT false,
    last_seen TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_users_username_lower ON users (LOWER(username));
CREATE INDEX IF NOT EXISTS idx_users_last_seen ON users (last_seen DESC);

-- ============= SESSIONS =============
CREATE TABLE IF NOT EXISTS sessions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(255) UNIQUE NOT NULL,
    device_info TEXT,
    ip INET,
    last_active TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions (user_id, last_active DESC);
CREATE INDEX IF NOT EXISTS idx_sessions_token ON sessions (token_hash);

-- ============= CHATS =============
CREATE TABLE IF NOT EXISTS chats (
    id BIGSERIAL PRIMARY KEY,
    type VARCHAR(16) NOT NULL DEFAULT 'private', -- private, group, channel
    name VARCHAR(128),
    description TEXT,
    avatar_url TEXT,
    avatar_color VARCHAR(16),
    owner_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    pinned_message_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_chats_type ON chats (type);
CREATE INDEX IF NOT EXISTS idx_chats_updated ON chats (updated_at DESC);

-- ============= CHAT MEMBERS =============
CREATE TABLE IF NOT EXISTS chat_members (
    chat_id BIGINT NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role VARCHAR(16) NOT NULL DEFAULT 'member', -- owner, admin, member
    last_read_message_id BIGINT,
    notifications_enabled BOOLEAN NOT NULL DEFAULT true,
    is_muted BOOLEAN NOT NULL DEFAULT false,
    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (chat_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_chat_members_user ON chat_members (user_id);
CREATE INDEX IF NOT EXISTS idx_chat_members_chat ON chat_members (chat_id, last_read_message_id DESC);

-- ============= MESSAGES =============
CREATE TABLE IF NOT EXISTS messages (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    sender_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type VARCHAR(16) NOT NULL DEFAULT 'text', -- text, image, video, voice, file, system
    content TEXT,
    media_url TEXT,
    media_meta JSONB,
    reply_to_id BIGINT REFERENCES messages(id) ON DELETE SET NULL,
    forwarded_from_id BIGINT REFERENCES messages(id) ON DELETE SET NULL,
    edited_at TIMESTAMPTZ,
    is_deleted BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_messages_chat ON messages (chat_id, created_at DESC) WHERE is_deleted = false;
CREATE INDEX IF NOT EXISTS idx_messages_sender ON messages (sender_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_messages_reply ON messages (reply_to_id) WHERE reply_to_id IS NOT NULL;

-- ============= REACTIONS =============
CREATE TABLE IF NOT EXISTS reactions (
    message_id BIGINT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    emoji VARCHAR(16) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (message_id, user_id, emoji)
);

CREATE INDEX IF NOT EXISTS idx_reactions_message ON reactions (message_id);

-- ============= ATTACHMENTS =============
CREATE TABLE IF NOT EXISTS attachments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    message_id BIGINT REFERENCES messages(id) ON DELETE CASCADE,
    uploader_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    url TEXT NOT NULL,
    mime_type VARCHAR(128) NOT NULL,
    size_bytes BIGINT NOT NULL,
    width INT,
    height INT,
    duration_sec INT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_attachments_message ON attachments (message_id);
CREATE INDEX IF NOT EXISTS idx_attachments_uploader ON attachments (uploader_id, created_at DESC);

-- ============= CALLS =============
CREATE TABLE IF NOT EXISTS calls (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    chat_id BIGINT NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    initiator_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type VARCHAR(8) NOT NULL DEFAULT 'audio', -- audio, video
    status VARCHAR(16) NOT NULL DEFAULT 'ringing', -- ringing, active, ended, missed, rejected
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    answered_at TIMESTAMPTZ,
    ended_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_calls_chat ON calls (chat_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_calls_initiator ON calls (initiator_id, started_at DESC);

-- ============= UPDATED_AT TRIGGER =============
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_users_updated ON users;
CREATE TRIGGER trg_users_updated BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

DROP TRIGGER IF EXISTS trg_chats_updated ON chats;
CREATE TRIGGER trg_chats_updated BEFORE UPDATE ON chats
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
