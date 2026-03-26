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
		 RETURNING id, timezone, created_at`,
		user.TelegramID, user.Username, user.FirstName,
	).Scan(&user.ID, &user.Timezone, &user.CreatedAt)
}

func (r *UserRepository) GetByTelegramID(ctx context.Context, telegramID int64) (*domain.User, error) {
	user := &domain.User{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, telegram_id, username, first_name, timezone, created_at FROM users WHERE telegram_id = $1`,
		telegramID,
	).Scan(&user.ID, &user.TelegramID, &user.Username, &user.FirstName, &user.Timezone, &user.CreatedAt)
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
