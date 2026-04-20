package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/saidakmal/habbit-tracker-bot/internal/domain"
)

type HabitRepository struct {
	pool *pgxpool.Pool
}

func NewHabitRepository(pool *pgxpool.Pool) *HabitRepository {
	return &HabitRepository{pool: pool}
}

// habitCols is the ordered column list for SELECT queries on the habits table.
const habitCols = `id, user_id, name, description, interval_minutes, start_hour, end_hour,
	last_done_at, last_notified_at, streak, best_streak, is_paused, goal_days, motivation, snooze_until, created_at`

type scanner interface {
	Scan(dest ...any) error
}

func scanHabit(s scanner, h *domain.Habit) error {
	return s.Scan(
		&h.ID, &h.UserID, &h.Name, &h.Description,
		&h.IntervalMinutes, &h.StartHour, &h.EndHour,
		&h.LastDoneAt, &h.LastNotifiedAt,
		&h.Streak, &h.BestStreak, &h.IsPaused, &h.GoalDays, &h.Motivation, &h.SnoozeUntil,
		&h.CreatedAt,
	)
}

func scanHabitRow(row pgx.Row, h *domain.Habit) error    { return scanHabit(row, h) }
func scanHabitRows(rows pgx.Rows, h *domain.Habit) error { return scanHabit(rows, h) }

func (r *HabitRepository) Create(ctx context.Context, habit *domain.Habit) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO habits (user_id, name, interval_minutes, start_hour, end_hour, goal_days, motivation)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, created_at`,
		habit.UserID, habit.Name, habit.IntervalMinutes, habit.StartHour, habit.EndHour, habit.GoalDays, habit.Motivation,
	).Scan(&habit.ID, &habit.CreatedAt)
}

func (r *HabitRepository) ListByUserID(ctx context.Context, userID int64) ([]*domain.Habit, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+habitCols+` FROM habits WHERE user_id = $1 ORDER BY created_at`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var habits []*domain.Habit
	for rows.Next() {
		h := &domain.Habit{}
		if err := scanHabitRows(rows, h); err != nil {
			return nil, err
		}
		habits = append(habits, h)
	}
	return habits, rows.Err()
}

func (r *HabitRepository) GetByID(ctx context.Context, id int64) (*domain.Habit, error) {
	h := &domain.Habit{}
	err := scanHabitRow(
		r.pool.QueryRow(ctx, `SELECT `+habitCols+` FROM habits WHERE id = $1`, id),
		h,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return h, nil
}

func (r *HabitRepository) Update(ctx context.Context, habit *domain.Habit) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE habits
		 SET last_done_at = $1, last_notified_at = $2,
		     streak = $3, best_streak = GREATEST(best_streak, $3)
		 WHERE id = $4`,
		habit.LastDoneAt, habit.LastNotifiedAt, habit.Streak, habit.ID,
	)
	return err
}

func (r *HabitRepository) UpdateSettings(ctx context.Context, habit *domain.Habit) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE habits
		 SET name = $1, interval_minutes = $2, start_hour = $3, end_hour = $4,
		     is_paused = $5, goal_days = $6, motivation = $7
		 WHERE id = $8 AND user_id = $9`,
		habit.Name, habit.IntervalMinutes, habit.StartHour, habit.EndHour,
		habit.IsPaused, habit.GoalDays, habit.Motivation,
		habit.ID, habit.UserID,
	)
	return err
}

func (r *HabitRepository) SetSnoozeUntil(ctx context.Context, habitID int64, t *time.Time) error {
	_, err := r.pool.Exec(ctx, `UPDATE habits SET snooze_until = $1 WHERE id = $2`, t, habitID)
	return err
}

func (r *HabitRepository) Delete(ctx context.Context, habitID, userID int64) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM habits WHERE id = $1 AND user_id = $2`, habitID, userID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *HabitRepository) SetLastNotifiedAt(ctx context.Context, habitID int64, t time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE habits SET last_notified_at = $1 WHERE id = $2`, t, habitID,
	)
	return err
}

const habitsWithUserSelect = `
	SELECT h.id, h.user_id, h.name, h.description, h.interval_minutes, h.start_hour, h.end_hour,
	       h.last_done_at, h.last_notified_at, h.streak, h.best_streak, h.is_paused, h.goal_days, h.motivation, h.snooze_until, h.created_at,
	       u.telegram_id, u.timezone, u.first_name, u.language, u.id, u.evening_recap_hour
	FROM habits h JOIN users u ON u.id = h.user_id`

func (r *HabitRepository) listHabitsWithUser(ctx context.Context, where string, args ...any) ([]*domain.HabitWithTelegramID, error) {
	rows, err := r.pool.Query(ctx, habitsWithUserSelect+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanHabitsWithUser(rows)
}

func (r *HabitRepository) ListAllWithTelegramID(ctx context.Context) ([]*domain.HabitWithTelegramID, error) {
	return r.listHabitsWithUser(ctx, "")
}

func (r *HabitRepository) ResetStreaksForInactive(ctx context.Context) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE habits SET streak = 0
		 WHERE streak > 0
		   AND (last_done_at IS NULL OR last_done_at::date < CURRENT_DATE - INTERVAL '1 day')`,
	)
	return err
}

func (r *HabitRepository) ResetHabitStreak(ctx context.Context, habitID int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE habits SET streak = 0 WHERE id = $1`, habitID)
	return err
}

// MarkDoneWithActivity atomically updates the habit and records the activity in one transaction.
// This prevents the case where habit is marked done but the activity record is missing.
func (r *HabitRepository) MarkDoneWithActivity(ctx context.Context, habit *domain.Habit, activity *domain.Activity) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	_, err = tx.Exec(ctx,
		`UPDATE habits
		 SET last_done_at = $1, streak = $2, best_streak = GREATEST(best_streak, $2), snooze_until = NULL
		 WHERE id = $3`,
		habit.LastDoneAt, habit.Streak, habit.ID,
	)
	if err != nil {
		return fmt.Errorf("update habit: %w", err)
	}

	err = tx.QueryRow(ctx,
		`INSERT INTO activities (user_id, habit_id, date) VALUES ($1, $2, $3) RETURNING id, created_at`,
		activity.UserID, activity.HabitID, activity.Date,
	).Scan(&activity.ID, &activity.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert activity: %w", err)
	}

	return tx.Commit(ctx)
}

func (r *HabitRepository) ListStreaksToBeReset(ctx context.Context) ([]*domain.HabitWithTelegramID, error) {
	return r.listHabitsWithUser(ctx, `
		WHERE h.streak > 0
		  AND (h.last_done_at IS NULL OR h.last_done_at::date < CURRENT_DATE - INTERVAL '1 day')`)
}

func scanHabitsWithUser(rows pgx.Rows) ([]*domain.HabitWithTelegramID, error) {
	var habits []*domain.HabitWithTelegramID
	for rows.Next() {
		hw := &domain.HabitWithTelegramID{}
		if err := rows.Scan(
			&hw.ID, &hw.UserID, &hw.Name, &hw.Description,
			&hw.IntervalMinutes, &hw.StartHour, &hw.EndHour,
			&hw.LastDoneAt, &hw.LastNotifiedAt,
			&hw.Streak, &hw.BestStreak, &hw.IsPaused, &hw.GoalDays, &hw.Motivation, &hw.SnoozeUntil,
			&hw.CreatedAt,
			&hw.TelegramID, &hw.UserTimezone, &hw.UserFirstName,
			&hw.UserLanguage, &hw.UserID, &hw.EveningRecapHour,
		); err != nil {
			return nil, err
		}
		habits = append(habits, hw)
	}
	return habits, rows.Err()
}

// ActivityRepository

type ActivityRepository struct {
	pool *pgxpool.Pool
}

func NewActivityRepository(pool *pgxpool.Pool) *ActivityRepository {
	return &ActivityRepository{pool: pool}
}

func (r *ActivityRepository) Save(ctx context.Context, activity *domain.Activity) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO activities (user_id, habit_id, date) VALUES ($1, $2, $3) RETURNING id, created_at`,
		activity.UserID, activity.HabitID, activity.Date,
	).Scan(&activity.ID, &activity.CreatedAt)
}

func (r *ActivityRepository) ListByUserAndDate(ctx context.Context, userID int64, date time.Time) ([]*domain.Activity, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, habit_id, date, created_at FROM activities WHERE user_id = $1 AND date::date = $2::date`,
		userID, date,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var activities []*domain.Activity
	for rows.Next() {
		a := &domain.Activity{}
		if err := rows.Scan(&a.ID, &a.UserID, &a.HabitID, &a.Date, &a.CreatedAt); err != nil {
			return nil, err
		}
		activities = append(activities, a)
	}
	return activities, rows.Err()
}

func (r *ActivityRepository) CountByHabitAndDateRange(ctx context.Context, habitID int64, from, to time.Time) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(DISTINCT date::date) FROM activities WHERE habit_id = $1 AND date >= $2 AND date < $3`,
		habitID, from, to,
	).Scan(&count)
	return count, err
}

func (r *ActivityRepository) CountsByHabitsAndDateRange(ctx context.Context, habitIDs []int64, from, to time.Time) (map[int64]int, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT habit_id, COUNT(DISTINCT date::date)
		 FROM activities
		 WHERE habit_id = ANY($1) AND date >= $2 AND date < $3
		 GROUP BY habit_id`,
		habitIDs, from, to,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[int64]int, len(habitIDs))
	for rows.Next() {
		var habitID int64
		var count int
		if err := rows.Scan(&habitID, &count); err != nil {
			return nil, err
		}
		counts[habitID] = count
	}
	return counts, rows.Err()
}

func (r *ActivityRepository) ListDatesByHabitAndDateRange(ctx context.Context, habitID int64, from, to time.Time) ([]time.Time, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT date::date FROM activities WHERE habit_id = $1 AND date >= $2 AND date < $3 ORDER BY 1`,
		habitID, from, to,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dates []time.Time
	for rows.Next() {
		var d time.Time
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		dates = append(dates, d)
	}
	return dates, rows.Err()
}

func (r *ActivityRepository) CountAllByUser(ctx context.Context, userID int64) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM activities WHERE user_id = $1`, userID).Scan(&count)
	return count, err
}

func (r *ActivityRepository) GetAverageCompletionHour(ctx context.Context, habitID int64) (int, bool, error) {
	var avgHour *float64
	err := r.pool.QueryRow(ctx,
		`SELECT AVG(EXTRACT(HOUR FROM date)) FROM activities WHERE habit_id = $1 AND date > NOW() - INTERVAL '30 days'`,
		habitID,
	).Scan(&avgHour)
	if err != nil || avgHour == nil {
		return 0, false, err
	}
	return int(*avgHour), true, nil
}

func (r *ActivityRepository) DeleteTodayActivity(ctx context.Context, habitID int64, date time.Time) error {
	dayStart := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	dayEnd := dayStart.AddDate(0, 0, 1)
	_, err := r.pool.Exec(ctx,
		`DELETE FROM activities WHERE habit_id = $1 AND date >= $2 AND date < $3`,
		habitID, dayStart, dayEnd,
	)
	return err
}

func (r *ActivityRepository) GetDayOfWeekCounts(ctx context.Context, userID int64, timezone string, from, to time.Time) (map[int]int, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT EXTRACT(DOW FROM date AT TIME ZONE $1)::int AS dow,
		       COUNT(DISTINCT date::date)
		FROM activities
		WHERE user_id = $2 AND date >= $3 AND date < $4
		GROUP BY dow`,
		timezone, userID, from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("GetDayOfWeekCounts: %w", err)
	}
	defer rows.Close()

	counts := make(map[int]int)
	for rows.Next() {
		var dow, count int
		if err := rows.Scan(&dow, &count); err != nil {
			return nil, err
		}
		counts[dow] = count
	}
	return counts, rows.Err()
}
