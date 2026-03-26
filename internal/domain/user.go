package domain

import "time"

type User struct {
	ID               int64
	TelegramID       int64
	Username         string
	FirstName        string
	Timezone         string
	Language         string
	XP               int
	Level            int
	StreakShields    int
	EveningRecapHour int
	CreatedAt        time.Time
}

type UserAchievement struct {
	Code       string
	UnlockedAt time.Time
}
