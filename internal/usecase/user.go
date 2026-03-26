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

func (u *UserUsecase) SetLanguage(ctx context.Context, userID int64, language string) error {
	return u.repo.UpdateLanguage(ctx, userID, language)
}

func (u *UserUsecase) AddXP(ctx context.Context, userID int64, xp int) (int, int, error) {
	return u.repo.AddXP(ctx, userID, xp)
}

func (u *UserUsecase) UpdateStreakShields(ctx context.Context, userID int64, shields int) error {
	return u.repo.UpdateStreakShields(ctx, userID, shields)
}

func (u *UserUsecase) AddAchievement(ctx context.Context, userID int64, code string) error {
	return u.repo.AddAchievement(ctx, userID, code)
}

func (u *UserUsecase) HasAchievement(ctx context.Context, userID int64, code string) (bool, error) {
	return u.repo.HasAchievement(ctx, userID, code)
}

func (u *UserUsecase) ListAchievements(ctx context.Context, userID int64) ([]domain.UserAchievement, error) {
	return u.repo.ListAchievements(ctx, userID)
}

func (u *UserUsecase) GetByID(ctx context.Context, userID int64) (*domain.User, error) {
	return u.repo.GetByID(ctx, userID)
}
