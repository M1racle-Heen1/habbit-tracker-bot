-- Feature 1: Habit motivation
ALTER TABLE habits ADD COLUMN IF NOT EXISTS motivation TEXT NOT NULL DEFAULT '';

-- Feature 2: Daily mood check-in
CREATE TABLE IF NOT EXISTS mood_logs (
    id         BIGSERIAL   PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    date       DATE        NOT NULL,
    mood       SMALLINT    NOT NULL CHECK (mood BETWEEN 1 AND 3),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, date)
);
CREATE INDEX IF NOT EXISTS idx_mood_logs_user_date ON mood_logs(user_id, date);
