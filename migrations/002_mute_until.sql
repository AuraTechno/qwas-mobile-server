-- 002 — добавляем mute_until для временного мута
ALTER TABLE chat_members ADD COLUMN IF NOT EXISTS mute_until TIMESTAMPTZ;
