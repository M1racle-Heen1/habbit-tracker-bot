package usecase

import (
	"context"
	"time"

	"github.com/saidakmal/habbit-tracker-bot/internal/domain"
)

type UserRepository interface {
	Save(ctx context.Context, user *domain.User) error
	GetByTelegramID(ctx context.Context, telegramID int64) (*domain.User, error)
	UpdateTimezone(ctx context.Context, userID int64, timezone string) error
}

type HabitRepository interface {
	Create(ctx context.Context, habit *domain.Habit) error
	ListByUserID(ctx context.Context, userID int64) ([]*domain.Habit, error)
	GetByID(ctx context.Context, id int64) (*domain.Habit, error)
	Update(ctx context.Context, habit *domain.Habit) error
	UpdateSettings(ctx context.Context, habit *domain.Habit) error
	SetSnoozeUntil(ctx context.Context, habitID int64, t *time.Time) error
	Delete(ctx context.Context, habitID, userID int64) error
	SetLastNotifiedAt(ctx context.Context, habitID int64, t time.Time) error
	ListAllWithTelegramID(ctx context.Context) ([]*domain.HabitWithTelegramID, error)
	ResetStreaksForInactive(ctx context.Context) error
	ListStreaksToBeReset(ctx context.Context) ([]*domain.HabitWithTelegramID, error)
}

type ActivityRepository interface {
	Save(ctx context.Context, activity *domain.Activity) error
	ListByUserAndDate(ctx context.Context, userID int64, date time.Time) ([]*domain.Activity, error)
	CountByHabitAndDateRange(ctx context.Context, habitID int64, from, to time.Time) (int, error)
	ListDatesByHabitAndDateRange(ctx context.Context, habitID int64, from, to time.Time) ([]time.Time, error)
}

type Cache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}
