ALTER TABLE habits
    DROP COLUMN IF EXISTS is_paused,
    DROP COLUMN IF EXISTS goal_days,
    DROP COLUMN IF EXISTS best_streak,
    DROP COLUMN IF EXISTS snooze_until;

ALTER TABLE users
    DROP COLUMN IF EXISTS timezone;
