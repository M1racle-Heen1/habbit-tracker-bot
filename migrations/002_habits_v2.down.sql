ALTER TABLE habits
    DROP COLUMN IF EXISTS interval_minutes,
    DROP COLUMN IF EXISTS start_hour,
    DROP COLUMN IF EXISTS end_hour,
    DROP COLUMN IF EXISTS last_done_at,
    DROP COLUMN IF EXISTS last_notified_at,
    DROP COLUMN IF EXISTS streak;
