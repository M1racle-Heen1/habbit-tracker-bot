package telegram

import (
	"context"
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"

	"github.com/saidakmal/habbit-tracker-bot/internal/domain"
	"github.com/saidakmal/habbit-tracker-bot/internal/gamification"
	"github.com/saidakmal/habbit-tracker-bot/internal/i18n"
	"github.com/saidakmal/habbit-tracker-bot/internal/usecase"
)

func (h *Handler) handleStart(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	lang := h.lang(user)

	isNew := user.Language == ""
	if isNew {
		m := tgbotapi.NewMessage(msg.Chat.ID, i18n.T(i18n.RU, "language.choose"))
		m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🇷🇺 Русский", "lang:ru"),
			tgbotapi.NewInlineKeyboardButtonData("🇬🇧 English", "lang:en"),
			tgbotapi.NewInlineKeyboardButtonData("🇰🇿 Қазақша", "lang:kz"),
		))
		if _, err := h.api.Send(m); err != nil {
			h.logger.Error("send start language picker", zap.Error(err))
		}
		return
	}

	habits, _ := h.habitUC.ListHabits(ctx, user.ID)
	if len(habits) == 0 {
		m := tgbotapi.NewMessage(msg.Chat.ID, i18n.T(lang, "onboarding.welcome_new", user.FirstName))
		m.ReplyMarkup = templateKeyboard()
		if _, err := h.api.Send(m); err != nil {
			h.logger.Error("send start", zap.Error(err))
		}
		return
	}

	h.send(msg.Chat.ID, i18n.T(lang, "onboarding.welcome_returning", user.FirstName))
}

func (h *Handler) handleLanguage(msg *tgbotapi.Message) {
	m := tgbotapi.NewMessage(msg.Chat.ID, "Выбери язык / Choose language / Тіл таңда:")
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🇷🇺 Русский", "lang:ru"),
		tgbotapi.NewInlineKeyboardButtonData("🇬🇧 English", "lang:en"),
		tgbotapi.NewInlineKeyboardButtonData("🇰🇿 Қазақша", "lang:kz"),
	))
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send language keyboard", zap.Error(err))
	}
}

func (h *Handler) handleTimezone(msg *tgbotapi.Message, user *domain.User) {
	h.sendTimezoneKeyboard(msg.Chat.ID, h.lang(user), "tz:")
}

func (h *Handler) startAddHabit(msg *tgbotapi.Message, user *domain.User) {
	lang := h.lang(user)
	h.setState(msg.From.ID, &convState{Step: stepIdle, Lang: lang})
	m := tgbotapi.NewMessage(msg.Chat.ID, i18n.T(lang, "habit.choose_template"))
	m.ReplyMarkup = templateKeyboard()
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send template keyboard", zap.Error(err))
	}
}

func (h *Handler) handleDone(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	lang := h.lang(user)
	habits, err := h.habitUC.ListHabits(ctx, user.ID)
	if err != nil || len(habits) == 0 {
		h.send(msg.Chat.ID, i18n.T(lang, "habit.none_for_done"))
		return
	}
	now := time.Now()
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, habit := range habits {
		if habit.IsPaused {
			continue
		}
		label := habit.Name
		if usecase.IsDoneToday(habit, now) {
			label = "✅ " + label
		} else {
			label = "○ " + label
			if habit.Streak > 0 {
				label += fmt.Sprintf(" (%d🔥)", habit.Streak)
			}
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, fmt.Sprintf("done:%d", habit.ID)),
		))
	}
	if len(rows) == 0 {
		h.send(msg.Chat.ID, i18n.T(lang, "habit.all_done_today"))
		return
	}
	m := tgbotapi.NewMessage(msg.Chat.ID, i18n.T(lang, "habit.choose"))
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send done keyboard", zap.Error(err))
	}
}

func (h *Handler) handleListHabits(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	lang := h.lang(user)
	habits, err := h.habitUC.ListHabits(ctx, user.ID)
	if err != nil {
		h.logger.Error("ListHabits", zap.Int64("user_id", user.ID), zap.Error(err))
		h.send(msg.Chat.ID, i18n.T(lang, "error.generic"))
		return
	}
	if len(habits) == 0 {
		h.send(msg.Chat.ID, i18n.T(lang, "habit.list_empty"))
		return
	}

	now := time.Now()
	var sb strings.Builder
	sb.WriteString(i18n.T(lang, "habit.list_header"))

	var undoneRows [][]tgbotapi.InlineKeyboardButton
	for _, habit := range habits {
		done := usecase.IsDoneToday(habit, now)
		mark := "○"
		if done {
			mark = "✅"
		}
		if habit.IsPaused {
			mark = "⏸"
		}

		streakStr := ""
		if habit.Streak > 0 {
			streakStr = fmt.Sprintf(" 🔥%d", habit.Streak)
		}

		goalStr := ""
		if habit.GoalDays > 0 {
			goalStr = i18n.T(lang, "habit.goal_progress", habit.Streak, habit.GoalDays)
		}

		sb.WriteString(fmt.Sprintf("%s %s%s%s\n   %s, %d:00–%d:00\n\n",
			mark, habit.Name, streakStr, goalStr,
			formatInterval(habit.IntervalMinutes, lang), habit.StartHour, habit.EndHour,
		))

		if !done && !habit.IsPaused {
			undoneRows = append(undoneRows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("✅ "+habit.Name, fmt.Sprintf("done:%d", habit.ID)),
			))
		}
	}

	m := tgbotapi.NewMessage(msg.Chat.ID, sb.String())
	if len(undoneRows) > 0 {
		m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(undoneRows...)
	}
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send list", zap.Error(err))
	}
}

func (h *Handler) handleDeleteHabit(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	lang := h.lang(user)
	habits, err := h.habitUC.ListHabits(ctx, user.ID)
	if err != nil || len(habits) == 0 {
		h.send(msg.Chat.ID, i18n.T(lang, "habit.none_for_delete"))
		return
	}
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, habit := range habits {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🗑 "+habit.Name, fmt.Sprintf("pre_delete:%d", habit.ID)),
		))
	}
	m := tgbotapi.NewMessage(msg.Chat.ID, i18n.T(lang, "habit.choose_delete"))
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send delete keyboard", zap.Error(err))
	}
}

func (h *Handler) handleEditHabit(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	lang := h.lang(user)
	habits, err := h.habitUC.ListHabits(ctx, user.ID)
	if err != nil || len(habits) == 0 {
		h.send(msg.Chat.ID, i18n.T(lang, "habit.none_for_edit"))
		return
	}
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, habit := range habits {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✏️ "+habit.Name, fmt.Sprintf("edit:%d", habit.ID)),
		))
	}
	m := tgbotapi.NewMessage(msg.Chat.ID, i18n.T(lang, "habit.choose_edit"))
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send edit keyboard", zap.Error(err))
	}
}

func (h *Handler) handlePauseHabit(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	h.sendHabitPickerKeyboard(ctx, msg.Chat.ID, user, "pause", "⏸ ")
}

func (h *Handler) handleResumeHabit(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	h.sendHabitPickerKeyboard(ctx, msg.Chat.ID, user, "resume", "▶️ ")
}

func (h *Handler) sendHabitPickerKeyboard(ctx context.Context, chatID int64, user *domain.User, action, prefix string) {
	lang := h.lang(user)
	habits, err := h.habitUC.ListHabits(ctx, user.ID)
	if err != nil || len(habits) == 0 {
		h.send(chatID, i18n.T(lang, "habit.none"))
		return
	}
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, habit := range habits {
		label := prefix + habit.Name
		if habit.IsPaused {
			label += " (⏸)"
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, fmt.Sprintf("%s:%d", action, habit.ID)),
		))
	}
	m := tgbotapi.NewMessage(chatID, i18n.T(lang, "habit.choose"))
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send picker keyboard", zap.Error(err))
	}
}

func (h *Handler) handleStats(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	lang := h.lang(user)
	stats, err := h.habitUC.GetStats(ctx, user.ID, 30)
	if err != nil {
		h.logger.Error("GetStats", zap.Error(err))
		h.send(msg.Chat.ID, i18n.T(lang, "error.generic"))
		return
	}
	if len(stats) == 0 {
		h.send(msg.Chat.ID, i18n.T(lang, "stats.empty"))
		return
	}

	var sb strings.Builder
	sb.WriteString(i18n.T(lang, "stats.header"))
	for _, s := range stats {
		bar := progressBar(s.CompletedDays, s.TotalDays)
		sb.WriteString(fmt.Sprintf("%s  %s  %d/%d (%d%%)\n", s.Habit.Name, bar, s.CompletedDays, s.TotalDays, s.CompletionPct))
		if s.Habit.GoalDays > 0 {
			goalBar := progressBar(s.Habit.Streak, s.Habit.GoalDays)
			sb.WriteString(fmt.Sprintf("   🎯 %s %d/%d days\n", goalBar, s.Habit.Streak, s.Habit.GoalDays))
		}
		if s.Habit.Streak > 0 || s.Habit.BestStreak > 0 {
			sb.WriteString(i18n.T(lang, "stats.streak_line", s.Habit.Streak, s.Habit.BestStreak))
		}
		sb.WriteString("\n")
	}
	sb.WriteString(i18n.T(lang, "stats.xp_level", user.Level, user.XP, user.StreakShields))
	h.send(msg.Chat.ID, sb.String())
}

func (h *Handler) handleHistory(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	lang := h.lang(user)
	habits, err := h.habitUC.ListHabits(ctx, user.ID)
	if err != nil || len(habits) == 0 {
		h.send(msg.Chat.ID, i18n.T(lang, "habit.none_for_history"))
		return
	}
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, habit := range habits {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📅 "+habit.Name, fmt.Sprintf("history:%d", habit.ID)),
		))
	}
	m := tgbotapi.NewMessage(msg.Chat.ID, i18n.T(lang, "history.choose"))
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send history keyboard", zap.Error(err))
	}
}

func (h *Handler) handleToday(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	lang := h.lang(user)
	habits, err := h.habitUC.ListHabits(ctx, user.ID)
	if err != nil {
		h.send(msg.Chat.ID, i18n.T(lang, "error.generic"))
		return
	}
	now := time.Now()
	var sb strings.Builder
	sb.WriteString(i18n.T(lang, "today.header"))
	var rows [][]tgbotapi.InlineKeyboardButton
	pending := 0
	for _, habit := range habits {
		if habit.IsPaused {
			continue
		}
		if usecase.IsDoneToday(habit, now) {
			sb.WriteString(fmt.Sprintf("✅ %s\n", habit.Name))
		} else {
			sb.WriteString(fmt.Sprintf("○ %s\n", habit.Name))
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("✅ "+habit.Name, fmt.Sprintf("done:%d", habit.ID)),
				tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "timer.btn"), fmt.Sprintf("timer_start:%d", habit.ID)),
			))
			pending++
		}
	}
	if pending == 0 && len(habits) == 0 {
		h.send(msg.Chat.ID, i18n.T(lang, "today.none"))
		return
	}
	if pending == 0 {
		h.send(msg.Chat.ID, i18n.T(lang, "today.all_done"))
		return
	}
	if pending >= 2 {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "today.done_all_btn"), "done_all:1"),
		))
	}
	m := tgbotapi.NewMessage(msg.Chat.ID, sb.String())
	if len(rows) > 0 {
		m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	}
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send today", zap.Error(err))
	}
}

func (h *Handler) handleAchievements(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	lang := h.lang(user)
	achievements, err := h.userUC.ListAchievements(ctx, user.ID)
	if err != nil {
		h.send(msg.Chat.ID, i18n.T(lang, "error.generic"))
		return
	}
	if len(achievements) == 0 {
		h.send(msg.Chat.ID, i18n.T(lang, "achievement.list_empty"))
		return
	}
	var sb strings.Builder
	sb.WriteString(i18n.T(lang, "achievement.list_header"))
	for _, a := range achievements {
		sb.WriteString(fmt.Sprintf("🏆 %s — %s\n", gamification.DisplayName(a.Code, lang), a.UnlockedAt.Format("02.01.2006")))
	}
	h.send(msg.Chat.ID, sb.String())
}
