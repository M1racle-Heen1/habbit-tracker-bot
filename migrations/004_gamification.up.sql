ALTER TABLE users
    ADD COLUMN IF NOT EXISTS language           VARCHAR(5)   NOT NULL DEFAULT 'ru',
    ADD COLUMN IF NOT EXISTS xp                INT          NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS level             INT          NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS streak_shields    INT          NOT NULL DEFAULT 3,
    ADD COLUMN IF NOT EXISTS evening_recap_hour INT         NOT NULL DEFAULT 21;

CREATE TABLE IF NOT EXISTS user_achievements (
    id               BIGSERIAL PRIMARY KEY,
    user_id          BIGINT       NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    achievement_code VARCHAR(64)  NOT NULL,
    unlocked_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, achievement_code)
);

CREATE INDEX IF NOT EXISTS idx_user_achievements_user_id ON user_achievements(user_id);
