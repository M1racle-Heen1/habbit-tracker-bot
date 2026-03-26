package scheduler

import (
	"context"
	"fmt"
	"strings"
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
	cache    usecase.Cache
}

func New(habitUC *usecase.HabitUsecase, api *tgbotapi.BotAPI, logger *zap.Logger, loc *time.Location, cache usecase.Cache) *Scheduler {
	return &Scheduler{habitUC: habitUC, api: api, logger: logger, location: loc, cache: cache}
}

func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			s.tick(ctx, t)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context, now time.Time) {
	habits, err := s.habitUC.ListAllForScheduler(ctx)
	if err != nil {
		s.logger.Error("ListAllForScheduler", zap.Error(err))
		return
	}

	// Midnight check in default timezone — reset streaks and notify
	defaultNow := now.In(s.location)
	if defaultNow.Hour() == 0 && defaultNow.Minute() == 0 {
		s.resetStreaksAndNotify(ctx)
	}

	// Group habits by user
	type userGroup struct {
		telegramID    int64
		firstName     string
		userTimezone  string
		habits        []*domain.HabitWithTelegramID
	}
	groups := make(map[int64]*userGroup)
	for _, hw := range habits {
		if _, ok := groups[hw.UserID]; !ok {
			groups[hw.UserID] = &userGroup{
				telegramID:   hw.TelegramID,
				firstName:    hw.UserFirstName,
				userTimezone: hw.UserTimezone,
			}
		}
		groups[hw.UserID].habits = append(groups[hw.UserID].habits, hw)
	}

	for _, g := range groups {
		loc, err := time.LoadLocation(g.userTimezone)
		if err != nil {
			loc = s.location
		}
		userNow := now.In(loc)

		// Morning digest at 8:00
		if userNow.Hour() == 8 && userNow.Minute() == 0 {
			s.maybeSendMorningDigest(ctx, g.telegramID, g.firstName, g.habits, userNow)
		}

		// Weekly digest on Sundays at 20:00
		if userNow.Weekday() == time.Sunday && userNow.Hour() == 20 && userNow.Minute() == 0 {
			s.maybeSendWeeklyDigest(ctx, g.telegramID, g.habits[0].UserID, g.habits, userNow)
		}

		// Per-habit reminders
		for _, hw := range g.habits {
			if hw.IsPaused {
				continue
			}
			if hw.SnoozeUntil != nil && now.Before(*hw.SnoozeUntil) {
				continue
			}
			if !usecase.IsInActiveHours(&hw.Habit, userNow) {
				continue
			}
			if usecase.IsDoneToday(&hw.Habit, userNow) {
				continue
			}
			if !usecase.IsFinalReminder(&hw.Habit, userNow) && !usecase.ShouldSendInterval(&hw.Habit, userNow) {
				continue
			}
			s.sendReminder(ctx, hw.TelegramID, &hw.Habit)
		}
	}
}

func (s *Scheduler) sendReminder(ctx context.Context, telegramID int64, h *domain.Habit) {
	streakText := ""
	if h.Streak > 0 {
		streakText = fmt.Sprintf("\nСтрик: %d дней подряд 🔥", h.Streak)
	}
	text := fmt.Sprintf("⏰ «%s»%s", h.Name, streakText)

	msg := tgbotapi.NewMessage(telegramID, text)
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Выполнено", fmt.Sprintf("done:%d", h.ID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⏰ +30 мин", fmt.Sprintf("snooze:%d:30", h.ID)),
			tgbotapi.NewInlineKeyboardButtonData("⏰ +1 час", fmt.Sprintf("snooze:%d:60", h.ID)),
			tgbotapi.NewInlineKeyboardButtonData("⏰ +2 часа", fmt.Sprintf("snooze:%d:120", h.ID)),
		),
	)
	if _, err := s.api.Send(msg); err != nil {
		s.logger.Error("send reminder", zap.Int64("telegram_id", telegramID), zap.Error(err))
		return
	}
	if err := s.habitUC.UpdateNotified(ctx, h.ID); err != nil {
		s.logger.Error("UpdateNotified", zap.Int64("habit_id", h.ID), zap.Error(err))
	}
}

func (s *Scheduler) maybeSendMorningDigest(ctx context.Context, telegramID int64, firstName string, habits []*domain.HabitWithTelegramID, now time.Time) {
	key := fmt.Sprintf("morning:%d:%s", telegramID, now.Format("2006-01-02"))
	if _, err := s.cache.Get(ctx, key); err == nil {
		return // already sent today
	}
	if err := s.cache.Set(ctx, key, "1", 25*time.Hour); err != nil {
		s.logger.Warn("morning digest cache set", zap.Error(err))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("☀️ Доброе утро, %s!\n\nПривычки на сегодня:\n\n", firstName))

	var doneButtons [][]tgbotapi.InlineKeyboardButton
	for _, hw := range habits {
		if hw.IsPaused {
			continue
		}
		if usecase.IsDoneToday(&hw.Habit, now) {
			sb.WriteString(fmt.Sprintf("✅ %s\n", hw.Name))
		} else {
			streakStr := ""
			if hw.Streak > 0 {
				streakStr = fmt.Sprintf(" (стрик: %d)", hw.Streak)
			}
			sb.WriteString(fmt.Sprintf("○ %s%s\n", hw.Name, streakStr))
			doneButtons = append(doneButtons, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("✅ "+hw.Name, fmt.Sprintf("done:%d", hw.ID)),
			))
		}
	}

	msg := tgbotapi.NewMessage(telegramID, sb.String())
	if len(doneButtons) > 0 {
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(doneButtons...)
	}
	if _, err := s.api.Send(msg); err != nil {
		s.logger.Error("send morning digest", zap.Int64("telegram_id", telegramID), zap.Error(err))
	}
}

func (s *Scheduler) maybeSendWeeklyDigest(ctx context.Context, telegramID int64, userID int64, habits []*domain.HabitWithTelegramID, now time.Time) {
	year, week := now.ISOWeek()
	key := fmt.Sprintf("weekly:%d:%d:%d", telegramID, year, week)
	if _, err := s.cache.Get(ctx, key); err == nil {
		return // already sent this week
	}
	if err := s.cache.Set(ctx, key, "1", 8*24*time.Hour); err != nil {
		s.logger.Warn("weekly digest cache set", zap.Error(err))
	}

	// Fetch stats once for this user (not inside the habit loop)
	stats, err := s.habitUC.GetStats(ctx, userID, 7)
	if err != nil {
		s.logger.Error("weekly digest GetStats", zap.Int64("user_id", userID), zap.Error(err))
		return
	}
	statsByID := make(map[int64]*usecase.HabitStats, len(stats))
	for _, st := range stats {
		statsByID[st.Habit.ID] = st
	}

	from := now.AddDate(0, 0, -6)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 Итоги недели (%s – %s)\n\n",
		from.Format("02.01"), now.Format("02.01")))

	totalDone, totalPossible := 0, 0
	for _, hw := range habits {
		if hw.IsPaused {
			continue
		}
		st, ok := statsByID[hw.ID]
		if !ok {
			continue
		}
		totalDone += st.CompletedDays
		totalPossible += 7
		icon := "✅"
		if st.CompletionPct < 50 {
			icon = "⚠️"
		}
		sb.WriteString(fmt.Sprintf("%s %s: %d/7 (%d%%)\n", icon, hw.Name, st.CompletedDays, st.CompletionPct))
	}

	if totalPossible == 0 {
		return
	}
	overall := totalDone * 100 / totalPossible
	sb.WriteString(fmt.Sprintf("\nОбщий результат: %d/%d (%d%%)", totalDone, totalPossible, overall))

	if _, err := s.api.Send(tgbotapi.NewMessage(telegramID, sb.String())); err != nil {
		s.logger.Error("send weekly digest", zap.Int64("telegram_id", telegramID), zap.Error(err))
	}
}

func (s *Scheduler) resetStreaksAndNotify(ctx context.Context) {
	// Get habits whose streaks will be broken BEFORE resetting
	toNotify, err := s.habitUC.ListStreaksToBeReset(ctx)
	if err != nil {
		s.logger.Error("ListStreaksToBeReset", zap.Error(err))
	}

	if err := s.habitUC.ResetStreaks(ctx); err != nil {
		s.logger.Error("ResetStreaks", zap.Error(err))
	}

	for _, hw := range toNotify {
		text := fmt.Sprintf(
			"😔 Стрик «%s» прервался (был %d дней).\nНе сдавайся! Начни снова сегодня.",
			hw.Name, hw.Streak,
		)
		msg := tgbotapi.NewMessage(hw.TelegramID, text)
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("✅ Выполнить сейчас", fmt.Sprintf("done:%d", hw.ID)),
			),
		)
		if _, err := s.api.Send(msg); err != nil {
			s.logger.Error("send streak break", zap.Int64("telegram_id", hw.TelegramID), zap.Error(err))
		}
	}
}
