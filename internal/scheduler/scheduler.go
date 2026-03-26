package scheduler

import (
	"context"
	"fmt"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"

	"github.com/saidakmal/habbit-tracker-bot/internal/domain"
	"github.com/saidakmal/habbit-tracker-bot/internal/usecase"
)

type Scheduler struct {
	habitUC  *usecase.HabitUsecase
	api      *tgbotapi.BotAPI
	logger   *zap.Logger
	location *time.Location
}

func New(habitUC *usecase.HabitUsecase, api *tgbotapi.BotAPI, logger *zap.Logger, loc *time.Location) *Scheduler {
	return &Scheduler{habitUC: habitUC, api: api, logger: logger, location: loc}
}

func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			s.tick(ctx, t.In(s.location))
		}
	}
}

func (s *Scheduler) tick(ctx context.Context, now time.Time) {
	if now.Hour() == 0 && now.Minute() == 0 {
		if err := s.habitUC.ResetStreaks(ctx); err != nil {
			s.logger.Error("ResetStreaks", zap.Error(err))
		}
	}

	habits, err := s.habitUC.ListAllForScheduler(ctx)
	if err != nil {
		s.logger.Error("ListAllForScheduler", zap.Error(err))
		return
	}

	for _, hw := range habits {
		if !usecase.IsInActiveHours(&hw.Habit, now) {
			continue
		}
		if usecase.IsDoneToday(&hw.Habit, now) {
			continue
		}

		force := usecase.IsFinalReminder(now, hw.EndHour)
		if !force && !usecase.ShouldSendInterval(&hw.Habit, now) {
			continue
		}

		s.sendReminder(ctx, hw.TelegramID, &hw.Habit)
	}
}

func (s *Scheduler) sendReminder(ctx context.Context, telegramID int64, h *domain.Habit) {
	text := fmt.Sprintf("⏰ «%s»\nСтрик: %d дней подряд", h.Name, h.Streak)
	if _, err := s.api.Send(tgbotapi.NewMessage(telegramID, text)); err != nil {
		s.logger.Error("send reminder", zap.Int64("telegram_id", telegramID), zap.Error(err))
		return
	}
	if err := s.habitUC.UpdateNotified(ctx, h.ID); err != nil {
		s.logger.Error("UpdateNotified", zap.Int64("habit_id", h.ID), zap.Error(err))
	}
}
