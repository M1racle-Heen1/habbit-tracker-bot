ALTER TABLE habits
    ADD COLUMN IF NOT EXISTS interval_minutes  INT         NOT NULL DEFAULT 60,
    ADD COLUMN IF NOT EXISTS start_hour        INT         NOT NULL DEFAULT 9,
    ADD COLUMN IF NOT EXISTS end_hour          INT         NOT NULL DEFAULT 23,
    ADD COLUMN IF NOT EXISTS last_done_at      TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_notified_at  TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS streak            INT         NOT NULL DEFAULT 0;
