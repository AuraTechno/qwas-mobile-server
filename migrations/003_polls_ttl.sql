-- 003 — polls and self-destructing messages
ALTER TABLE messages ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_messages_expires ON messages (expires_at) WHERE expires_at IS NOT NULL;

CREATE TABLE IF NOT EXISTS polls (
    id BIGSERIAL PRIMARY KEY,
    message_id BIGINT NOT NULL UNIQUE REFERENCES messages(id) ON DELETE CASCADE,
    question TEXT NOT NULL,
    is_anonymous BOOLEAN NOT NULL DEFAULT false,
    is_multiple BOOLEAN NOT NULL DEFAULT false,
    closes_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_polls_message ON polls (message_id);

CREATE TABLE IF NOT EXISTS poll_options (
    id BIGSERIAL PRIMARY KEY,
    poll_id BIGINT NOT NULL REFERENCES polls(id) ON DELETE CASCADE,
    text TEXT NOT NULL,
    sort_order INT NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_poll_options_poll ON poll_options (poll_id, sort_order);

CREATE TABLE IF NOT EXISTS poll_votes (
    poll_id BIGINT NOT NULL REFERENCES polls(id) ON DELETE CASCADE,
    option_id BIGINT NOT NULL REFERENCES poll_options(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    voted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (poll_id, option_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_poll_votes_user ON poll_votes (user_id, poll_id);
