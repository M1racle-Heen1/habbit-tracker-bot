package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/saidakmal/habbit-tracker-bot/internal/domain"
)

type MoodRepository struct {
	pool *pgxpool.Pool
}

func NewMoodRepository(pool *pgxpool.Pool) *MoodRepository {
	return &MoodRepository{pool: pool}
}

func (r *MoodRepository) Save(ctx context.Context, userID int64, date time.Time, mood int) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO mood_logs (user_id, date, mood)
		 VALUES ($1, $2::date, $3)
		 ON CONFLICT (user_id, date) DO UPDATE SET mood = EXCLUDED.mood`,
		userID, date, mood,
	)
	if err != nil {
		return fmt.Errorf("mood save: %w", err)
	}
	return nil
}

func (r *MoodRepository) GetByUserAndDateRange(ctx context.Context, userID int64, from, to time.Time) ([]domain.MoodLog, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, date, mood, created_at FROM mood_logs
		 WHERE user_id = $1 AND date >= $2::date AND date < $3::date
		 ORDER BY date`,
		userID, from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("mood range query: %w", err)
	}
	defer rows.Close()

	var logs []domain.MoodLog
	for rows.Next() {
		var m domain.MoodLog
		if err := rows.Scan(&m.ID, &m.UserID, &m.Date, &m.Mood, &m.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, m)
	}
	return logs, rows.Err()
}

func (r *MoodRepository) HasLoggedToday(ctx context.Context, userID int64, date time.Time) (bool, error) {
	var count int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM mood_logs WHERE user_id = $1 AND date = $2::date`,
		userID, date,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("mood has logged: %w", err)
	}
	return count > 0, nil
}
