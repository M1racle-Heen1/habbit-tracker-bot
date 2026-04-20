package usecase

import (
	"context"
	"time"

	"github.com/saidakmal/habbit-tracker-bot/internal/domain"
)

type MoodUsecase struct {
	moodRepo MoodRepository
}

func NewMoodUsecase(repo MoodRepository) *MoodUsecase {
	return &MoodUsecase{moodRepo: repo}
}

func (u *MoodUsecase) LogMood(ctx context.Context, userID int64, date time.Time, mood int) error {
	return u.moodRepo.Save(ctx, userID, date, mood)
}

func (u *MoodUsecase) HasLoggedToday(ctx context.Context, userID int64) (bool, error) {
	return u.moodRepo.HasLoggedToday(ctx, userID, time.Now())
}

func (u *MoodUsecase) GetWeekMoods(ctx context.Context, userID int64, from, to time.Time) ([]domain.MoodLog, error) {
	return u.moodRepo.GetByUserAndDateRange(ctx, userID, from, to)
}
