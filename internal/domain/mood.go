package domain

import "time"

type MoodLog struct {
	ID        int64
	UserID    int64
	Date      time.Time
	Mood      int // 1=Tough, 2=Okay, 3=Great
	CreatedAt time.Time
}
