package postgres

import (
	"context"
	"errors"
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

func (r *HabitRepository) Create(ctx context.Context, habit *domain.Habit) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO habits (user_id, name, interval_minutes, start_hour, end_hour)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, created_at`,
		habit.UserID, habit.Name, habit.IntervalMinutes, habit.StartHour, habit.EndHour,
	).Scan(&habit.ID, &habit.CreatedAt)
}

func (r *HabitRepository) ListByUserID(ctx context.Context, userID int64) ([]*domain.Habit, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, name, description, interval_minutes, start_hour, end_hour,
		        last_done_at, last_notified_at, streak, created_at
		 FROM habits WHERE user_id = $1 ORDER BY created_at`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var habits []*domain.Habit
	for rows.Next() {
		h := &domain.Habit{}
		if err := scanHabit(rows, h); err != nil {
			return nil, err
		}
		habits = append(habits, h)
	}
	return habits, rows.Err()
}

func (r *HabitRepository) GetByID(ctx context.Context, id int64) (*domain.Habit, error) {
	h := &domain.Habit{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, name, description, interval_minutes, start_hour, end_hour,
		        last_done_at, last_notified_at, streak, created_at
		 FROM habits WHERE id = $1`,
		id,
	).Scan(&h.ID, &h.UserID, &h.Name, &h.Description,
		&h.IntervalMinutes, &h.StartHour, &h.EndHour,
		&h.LastDoneAt, &h.LastNotifiedAt, &h.Streak, &h.CreatedAt)
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
		`UPDATE habits SET last_done_at = $1, last_notified_at = $2, streak = $3 WHERE id = $4`,
		habit.LastDoneAt, habit.LastNotifiedAt, habit.Streak, habit.ID,
	)
	return err
}

func (r *HabitRepository) Delete(ctx context.Context, habitID, userID int64) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM habits WHERE id = $1 AND user_id = $2`,
		habitID, userID,
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
		`UPDATE habits SET last_notified_at = $1 WHERE id = $2`,
		t, habitID,
	)
	return err
}

func (r *HabitRepository) ListAllWithTelegramID(ctx context.Context) ([]*domain.HabitWithTelegramID, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT h.id, h.user_id, h.name, h.description, h.interval_minutes, h.start_hour, h.end_hour,
		        h.last_done_at, h.last_notified_at, h.streak, h.created_at, u.telegram_id
		 FROM habits h
		 JOIN users u ON u.id = h.user_id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var habits []*domain.HabitWithTelegramID
	for rows.Next() {
		hw := &domain.HabitWithTelegramID{}
		if err := rows.Scan(
			&hw.ID, &hw.UserID, &hw.Name, &hw.Description,
			&hw.IntervalMinutes, &hw.StartHour, &hw.EndHour,
			&hw.LastDoneAt, &hw.LastNotifiedAt, &hw.Streak, &hw.CreatedAt,
			&hw.TelegramID,
		); err != nil {
			return nil, err
		}
		habits = append(habits, hw)
	}
	return habits, rows.Err()
}

func (r *HabitRepository) ResetStreaksForInactive(ctx context.Context) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE habits SET streak = 0
		 WHERE streak > 0
		   AND (last_done_at IS NULL OR last_done_at::date < CURRENT_DATE - INTERVAL '1 day')`,
	)
	return err
}

// scanHabit scans a row from ListByUserID queries.
func scanHabit(rows pgx.Rows, h *domain.Habit) error {
	return rows.Scan(
		&h.ID, &h.UserID, &h.Name, &h.Description,
		&h.IntervalMinutes, &h.StartHour, &h.EndHour,
		&h.LastDoneAt, &h.LastNotifiedAt, &h.Streak, &h.CreatedAt,
	)
}

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
