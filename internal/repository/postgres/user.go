package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/saidakmal/habbit-tracker-bot/internal/domain"
)

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

func (r *UserRepository) Save(ctx context.Context, user *domain.User) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO users (telegram_id, username, first_name)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (telegram_id) DO UPDATE
		   SET username = EXCLUDED.username, first_name = EXCLUDED.first_name
		 RETURNING id, timezone, language, xp, level, streak_shields, evening_recap_hour, created_at`,
		user.TelegramID, user.Username, user.FirstName,
	).Scan(&user.ID, &user.Timezone, &user.Language, &user.XP, &user.Level, &user.StreakShields, &user.EveningRecapHour, &user.CreatedAt)
}

func (r *UserRepository) GetByTelegramID(ctx context.Context, telegramID int64) (*domain.User, error) {
	user := &domain.User{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, telegram_id, username, first_name, timezone, language, xp, level, streak_shields, evening_recap_hour, created_at FROM users WHERE telegram_id = $1`,
		telegramID,
	).Scan(&user.ID, &user.TelegramID, &user.Username, &user.FirstName, &user.Timezone, &user.Language, &user.XP, &user.Level, &user.StreakShields, &user.EveningRecapHour, &user.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return user, nil
}

func (r *UserRepository) UpdateTimezone(ctx context.Context, userID int64, timezone string) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET timezone = $1 WHERE id = $2`, timezone, userID)
	return err
}

func (r *UserRepository) UpdateLanguage(ctx context.Context, userID int64, language string) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET language = $1 WHERE id = $2`, language, userID)
	return err
}

func (r *UserRepository) AddXP(ctx context.Context, userID int64, xp int) (int, int, error) {
	var newXP, newLevel int
	err := r.pool.QueryRow(ctx, `
		UPDATE users
		SET xp    = xp + $1,
		    level = CASE
		              WHEN xp + $1 >= 1000 THEN 5 + (xp + $1 - 1000) / 500
		              WHEN xp + $1 >= 500  THEN 5
		              WHEN xp + $1 >= 250  THEN 4
		              WHEN xp + $1 >= 100  THEN 3
		              WHEN xp + $1 >= 0    THEN 2
		              ELSE 1
		            END
		WHERE id = $2
		RETURNING xp, level`,
		xp, userID,
	).Scan(&newXP, &newLevel)
	return newXP, newLevel, err
}

func (r *UserRepository) UpdateStreakShields(ctx context.Context, userID int64, shields int) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET streak_shields = $1 WHERE id = $2`, shields, userID)
	return err
}

func (r *UserRepository) AddAchievement(ctx context.Context, userID int64, code string) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO user_achievements (user_id, achievement_code) VALUES ($1, $2) ON CONFLICT DO NOTHING`, userID, code)
	return err
}

func (r *UserRepository) HasAchievement(ctx context.Context, userID int64, code string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM user_achievements WHERE user_id = $1 AND achievement_code = $2)`, userID, code).Scan(&exists)
	return exists, err
}

func (r *UserRepository) ListAchievements(ctx context.Context, userID int64) ([]domain.UserAchievement, error) {
	rows, err := r.pool.Query(ctx, `SELECT achievement_code, unlocked_at FROM user_achievements WHERE user_id = $1 ORDER BY unlocked_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.UserAchievement
	for rows.Next() {
		var a domain.UserAchievement
		if err := rows.Scan(&a.Code, &a.UnlockedAt); err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

func (r *UserRepository) GetByID(ctx context.Context, userID int64) (*domain.User, error) {
	user := &domain.User{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, telegram_id, username, first_name, timezone, language, xp, level, streak_shields, evening_recap_hour, created_at FROM users WHERE id = $1`,
		userID,
	).Scan(&user.ID, &user.TelegramID, &user.Username, &user.FirstName, &user.Timezone, &user.Language, &user.XP, &user.Level, &user.StreakShields, &user.EveningRecapHour, &user.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return user, nil
}
