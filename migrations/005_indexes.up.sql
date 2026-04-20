-- Used in ListStreaksToBeReset and ResetStreaksForInactive every midnight
CREATE INDEX IF NOT EXISTS idx_habits_streak_last_done
    ON habits(streak, last_done_at) WHERE streak > 0;

-- Used in GetHistory and CountsByHabitsAndDateRange
CREATE INDEX IF NOT EXISTS idx_activities_habit_date
    ON activities(habit_id, date);

-- Used in HasAchievement and ListAchievements
CREATE INDEX IF NOT EXISTS idx_user_achievements_code
    ON user_achievements(achievement_code);
