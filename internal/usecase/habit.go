package usecase

import (
	"context"
	"time"

	"github.com/saidakmal/habbit-tracker-bot/internal/domain"
)

type HabitUsecase struct {
	habitRepo    HabitRepository
	activityRepo ActivityRepository
}

func NewHabitUsecase(habitRepo HabitRepository, activityRepo ActivityRepository) *HabitUsecase {
	return &HabitUsecase{habitRepo: habitRepo, activityRepo: activityRepo}
}

func (u *HabitUsecase) CreateHabit(ctx context.Context, userID int64, name string, intervalMinutes, startHour, endHour int) (*domain.Habit, error) {
	habit := &domain.Habit{
		UserID:          userID,
		Name:            name,
		IntervalMinutes: intervalMinutes,
		StartHour:       startHour,
		EndHour:         endHour,
	}
	if err := u.habitRepo.Create(ctx, habit); err != nil {
		return nil, err
	}
	return habit, nil
}

func (u *HabitUsecase) ListHabits(ctx context.Context, userID int64) ([]*domain.Habit, error) {
	return u.habitRepo.ListByUserID(ctx, userID)
}

func (u *HabitUsecase) GetHabit(ctx context.Context, id int64) (*domain.Habit, error) {
	return u.habitRepo.GetByID(ctx, id)
}

func (u *HabitUsecase) MarkDone(ctx context.Context, userID, habitID int64) error {
	habit, err := u.habitRepo.GetByID(ctx, habitID)
	if err != nil {
		return err
	}
	if habit.UserID != userID {
		return domain.ErrForbidden
	}

	now := time.Now()
	if IsDoneToday(habit, now) {
		return domain.ErrAlreadyDone
	}

	// streak: +1 if done yesterday, else reset to 1
	if habit.LastDoneAt != nil && sameDay(habit.LastDoneAt.AddDate(0, 0, 1), now) {
		habit.Streak++
	} else {
		habit.Streak = 1
	}
	habit.LastDoneAt = &now

	if err := u.habitRepo.Update(ctx, habit); err != nil {
		return err
	}
	return u.activityRepo.Save(ctx, &domain.Activity{
		UserID:  userID,
		HabitID: habitID,
		Date:    now,
	})
}

func (u *HabitUsecase) DeleteHabit(ctx context.Context, userID, habitID int64) error {
	return u.habitRepo.Delete(ctx, habitID, userID)
}

func (u *HabitUsecase) UpdateNotified(ctx context.Context, habitID int64) error {
	return u.habitRepo.SetLastNotifiedAt(ctx, habitID, time.Now())
}

func (u *HabitUsecase) ListAllForScheduler(ctx context.Context) ([]*domain.HabitWithTelegramID, error) {
	return u.habitRepo.ListAllWithTelegramID(ctx)
}

func (u *HabitUsecase) ResetStreaks(ctx context.Context) error {
	return u.habitRepo.ResetStreaksForInactive(ctx)
}

// IsDoneToday reports whether the habit was completed today.
func IsDoneToday(h *domain.Habit, now time.Time) bool {
	if h.LastDoneAt == nil {
		return false
	}
	return sameDay(*h.LastDoneAt, now)
}

// IsInActiveHours reports whether now falls within the habit's active window.
func IsInActiveHours(h *domain.Habit, now time.Time) bool {
	return now.Hour() >= h.StartHour && now.Hour() < h.EndHour
}

// ShouldSendInterval reports whether enough time has passed since the last notification.
func ShouldSendInterval(h *domain.Habit, now time.Time) bool {
	if h.LastNotifiedAt == nil {
		return true
	}
	return now.Sub(*h.LastNotifiedAt) >= time.Duration(h.IntervalMinutes)*time.Minute
}

// IsFinalReminder reports whether it's within the last 30 minutes of the active window.
func IsFinalReminder(now time.Time, endHour int) bool {
	return now.Hour() == endHour-1 && now.Minute() >= 30
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// TrackHabit is kept for backward compatibility.
func (u *HabitUsecase) TrackHabit(ctx context.Context, userID, habitID int64) error {
	return u.MarkDone(ctx, userID, habitID)
}

