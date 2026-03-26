package usecase

import (
	"context"
	"errors"

	"github.com/saidakmal/habbit-tracker-bot/internal/domain"
)

type UserUsecase struct {
	repo UserRepository
}

func NewUserUsecase(repo UserRepository) *UserUsecase {
	return &UserUsecase{repo: repo}
}

func (u *UserUsecase) GetOrCreateUser(ctx context.Context, telegramID int64, username, firstName string) (*domain.User, error) {
	user, err := u.repo.GetByTelegramID(ctx, telegramID)
	if err == nil {
		return user, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	user = &domain.User{
		TelegramID: telegramID,
		Username:   username,
		FirstName:  firstName,
	}
	if err := u.repo.Save(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

func (u *UserUsecase) SetTimezone(ctx context.Context, userID int64, timezone string) error {
	return u.repo.UpdateTimezone(ctx, userID, timezone)
}
