package domain

import "time"

type Habit struct {
	ID              int64
	UserID          int64
	Name            string
	Description     string
	IntervalMinutes int
	StartHour       int
	EndHour         int
	LastDoneAt      *time.Time
	LastNotifiedAt  *time.Time
	Streak          int
	CreatedAt       time.Time
}

type Activity struct {
	ID        int64
	UserID    int64
	HabitID   int64
	Date      time.Time
	CreatedAt time.Time
}

type HabitWithTelegramID struct {
	Habit
	TelegramID int64
}
