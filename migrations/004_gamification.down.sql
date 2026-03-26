DROP TABLE IF EXISTS user_achievements;

ALTER TABLE users
    DROP COLUMN IF EXISTS language,
    DROP COLUMN IF EXISTS xp,
    DROP COLUMN IF EXISTS level,
    DROP COLUMN IF EXISTS streak_shields,
    DROP COLUMN IF EXISTS evening_recap_hour;
