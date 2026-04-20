package usecase

import (
	"context"
	"time"

	"github.com/saidakmal/habbit-tracker-bot/internal/domain"
)

type GamificationNotifier func(ctx context.Context, userID int64, habitID int64, streak int)

type HabitUsecase struct {
	habitRepo    HabitRepository
	activityRepo ActivityRepository
	onDone       GamificationNotifier
}

func NewHabitUsecase(habitRepo HabitRepository, activityRepo ActivityRepository) *HabitUsecase {
	return &HabitUsecase{habitRepo: habitRepo, activityRepo: activityRepo}
}

func (u *HabitUsecase) SetGamificationNotifier(fn GamificationNotifier) {
	u.onDone = fn
}

type HabitStats struct {
	Habit         *domain.Habit
	TotalDays     int
	CompletedDays int
	CompletionPct int
}

func (u *HabitUsecase) CreateHabit(ctx context.Context, userID int64, name string, intervalMinutes, startHour, endHour, goalDays int, motivation string) (*domain.Habit, error) {
	habit := &domain.Habit{
		UserID:          userID,
		Name:            name,
		IntervalMinutes: intervalMinutes,
		StartHour:       startHour,
		EndHour:         endHour,
		GoalDays:        goalDays,
		Motivation:      motivation,
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

	if habit.LastDoneAt != nil && sameDay(habit.LastDoneAt.AddDate(0, 0, 1), now) {
		habit.Streak++
	} else {
		habit.Streak = 1
	}
	habit.LastDoneAt = &now

	activity := &domain.Activity{
		UserID:  userID,
		HabitID: habitID,
		Date:    now,
	}
	// Atomically update habit + insert activity + clear snooze in one transaction.
	if err := u.habitRepo.MarkDoneWithActivity(ctx, habit, activity); err != nil {
		return err
	}

	if u.onDone != nil {
		streak := habit.Streak
		go u.onDone(context.Background(), userID, habitID, streak)
	}
	return nil
}

func (u *HabitUsecase) DeleteHabit(ctx context.Context, userID, habitID int64) error {
	return u.habitRepo.Delete(ctx, habitID, userID)
}

func (u *HabitUsecase) EditHabit(ctx context.Context, userID, habitID int64, name string, intervalMinutes, startHour, endHour int) (*domain.Habit, error) {
	habit, err := u.habitRepo.GetByID(ctx, habitID)
	if err != nil {
		return nil, err
	}
	if habit.UserID != userID {
		return nil, domain.ErrForbidden
	}
	habit.Name = name
	habit.IntervalMinutes = intervalMinutes
	habit.StartHour = startHour
	habit.EndHour = endHour
	// Motivation is preserved from the fetched habit (not overwritten).
	if err := u.habitRepo.UpdateSettings(ctx, habit); err != nil {
		return nil, err
	}
	return habit, nil
}

func (u *HabitUsecase) SetMotivation(ctx context.Context, userID, habitID int64, motivation string) error {
	habit, err := u.habitRepo.GetByID(ctx, habitID)
	if err != nil {
		return err
	}
	if habit.UserID != userID {
		return domain.ErrForbidden
	}
	habit.Motivation = motivation
	return u.habitRepo.UpdateSettings(ctx, habit)
}

func (u *HabitUsecase) GetDayOfWeekStats(ctx context.Context, userID int64, timezone string) (map[int]int, error) {
	now := time.Now()
	from := now.AddDate(0, -3, 0)
	return u.activityRepo.GetDayOfWeekCounts(ctx, userID, timezone, from, now.AddDate(0, 0, 1))
}

func (u *HabitUsecase) PauseHabit(ctx context.Context, userID, habitID int64) error {
	habit, err := u.habitRepo.GetByID(ctx, habitID)
	if err != nil {
		return err
	}
	if habit.UserID != userID {
		return domain.ErrForbidden
	}
	habit.IsPaused = true
	return u.habitRepo.UpdateSettings(ctx, habit)
}

func (u *HabitUsecase) ResumeHabit(ctx context.Context, userID, habitID int64) error {
	habit, err := u.habitRepo.GetByID(ctx, habitID)
	if err != nil {
		return err
	}
	if habit.UserID != userID {
		return domain.ErrForbidden
	}
	habit.IsPaused = false
	return u.habitRepo.UpdateSettings(ctx, habit)
}

func (u *HabitUsecase) SnoozeHabit(ctx context.Context, habitID int64, minutes int) error {
	t := time.Now().Add(time.Duration(minutes) * time.Minute)
	return u.habitRepo.SetSnoozeUntil(ctx, habitID, &t)
}

func (u *HabitUsecase) SetGoal(ctx context.Context, userID, habitID int64, days int) error {
	habit, err := u.habitRepo.GetByID(ctx, habitID)
	if err != nil {
		return err
	}
	if habit.UserID != userID {
		return domain.ErrForbidden
	}
	habit.GoalDays = days
	return u.habitRepo.UpdateSettings(ctx, habit)
}

func (u *HabitUsecase) GetStats(ctx context.Context, userID int64, days int) ([]*HabitStats, error) {
	habits, err := u.habitRepo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(habits) == 0 {
		return nil, nil
	}

	now := time.Now()
	from := now.AddDate(0, 0, -days)
	to := now.AddDate(0, 0, 1)

	habitIDs := make([]int64, len(habits))
	for i, h := range habits {
		habitIDs[i] = h.ID
	}

	counts, err := u.activityRepo.CountsByHabitsAndDateRange(ctx, habitIDs, from, to)
	if err != nil {
		return nil, err
	}

	stats := make([]*HabitStats, 0, len(habits))
	for _, h := range habits {
		count := counts[h.ID]
		daysActive := int(now.Sub(h.CreatedAt).Hours()/24) + 1
		if daysActive > days {
			daysActive = days
		}
		pct := 0
		if daysActive > 0 {
			pct = count * 100 / daysActive
		}
		stats = append(stats, &HabitStats{
			Habit:         h,
			TotalDays:     daysActive,
			CompletedDays: count,
			CompletionPct: pct,
		})
	}
	return stats, nil
}

func (u *HabitUsecase) GetHistory(ctx context.Context, userID, habitID int64, from, to time.Time) ([]time.Time, error) {
	habit, err := u.habitRepo.GetByID(ctx, habitID)
	if err != nil {
		return nil, err
	}
	if habit.UserID != userID {
		return nil, domain.ErrForbidden
	}
	return u.activityRepo.ListDatesByHabitAndDateRange(ctx, habitID, from, to)
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

func (u *HabitUsecase) ResetHabitStreak(ctx context.Context, habitID int64) error {
	return u.habitRepo.ResetHabitStreak(ctx, habitID)
}

func (u *HabitUsecase) ListStreaksToBeReset(ctx context.Context) ([]*domain.HabitWithTelegramID, error) {
	return u.habitRepo.ListStreaksToBeReset(ctx)
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

// IsInActiveHoursFrom reports whether now falls within [startHour, h.EndHour).
func IsInActiveHoursFrom(h *domain.Habit, now time.Time, startHour int) bool {
	return now.Hour() >= startHour && now.Hour() < h.EndHour
}

// ShouldSendInterval reports whether enough time has passed since the last notification.
func ShouldSendInterval(h *domain.Habit, now time.Time) bool {
	if h.LastNotifiedAt == nil {
		return true
	}
	return now.Sub(*h.LastNotifiedAt) >= time.Duration(h.IntervalMinutes)*time.Minute
}

// IsFinalReminder reports whether it's within the last 30 minutes of the active window.
// Habits with interval >= 480 min (daily) are excluded from forced final reminders.
func IsFinalReminder(h *domain.Habit, now time.Time) bool {
	if h.IntervalMinutes >= 480 {
		return false
	}
	return now.Hour() == h.EndHour-1 && now.Minute() >= 30
}

func sameDay(a, b time.Time) bool {
	a = a.In(b.Location())
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// GetActivityAverageHour returns the average completion hour for a habit over the last 30 days.
func (u *HabitUsecase) GetActivityAverageHour(ctx context.Context, habitID int64) (int, bool, error) {
	return u.activityRepo.GetAverageCompletionHour(ctx, habitID)
}

// UndoMarkDone reverts a MarkDone by deleting today's activity and restoring
// the previous streak and last_done_at values.
func (u *HabitUsecase) UndoMarkDone(ctx context.Context, userID, habitID int64, prevStreak int, prevLastDoneAt *time.Time) error {
	habit, err := u.habitRepo.GetByID(ctx, habitID)
	if err != nil {
		return err
	}
	if habit.UserID != userID {
		return domain.ErrForbidden
	}
	if err := u.activityRepo.DeleteTodayActivity(ctx, habitID, time.Now()); err != nil {
		return err
	}
	habit.Streak = prevStreak
	habit.LastDoneAt = prevLastDoneAt
	return u.habitRepo.Update(ctx, habit)
}
