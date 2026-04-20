package telegram

import (
	"context"
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"

	"github.com/saidakmal/habbit-tracker-bot/internal/domain"
	"github.com/saidakmal/habbit-tracker-bot/internal/format"
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
		m.ReplyMarkup = templateKeyboard(lang)
		if _, err := h.api.Send(m); err != nil {
			h.logger.Error("send start", zap.Error(err))
		}
		return
	}

	m := tgbotapi.NewMessage(msg.Chat.ID, i18n.T(lang, "onboarding.welcome_returning", user.FirstName))
	m.ReplyMarkup = mainNavKeyboard(lang)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send start returning", zap.Error(err))
	}
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

func (h *Handler) handleSettings(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	lang := h.lang(user)
	m := tgbotapi.NewMessage(msg.Chat.ID, i18n.T(lang, "settings.header"))
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "settings.lang_btn"), "settings:language"),
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "settings.tz_btn"), "settings:timezone"),
		),
	)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send settings", zap.Error(err))
	}
}

func (h *Handler) startAddHabit(msg *tgbotapi.Message, user *domain.User) {
	lang := h.lang(user)
	h.setState(msg.From.ID, &convState{Step: stepIdle, Lang: lang})
	m := tgbotapi.NewMessage(msg.Chat.ID, i18n.T(lang, "habit.choose_template"))
	m.ReplyMarkup = templateKeyboard(lang)
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

	var habitRows [][]tgbotapi.InlineKeyboardButton
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

		habitRows = append(habitRows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⚙️ "+habit.Name, fmt.Sprintf("habit_menu:%d", habit.ID)),
		))
	}

	m := tgbotapi.NewMessage(msg.Chat.ID, sb.String())
	if len(habitRows) > 0 {
		m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(habitRows...)
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

	habits, err := h.habitUC.ListHabits(ctx, user.ID)
	if err != nil {
		h.logger.Error("ListHabits stats", zap.Int64("user_id", user.ID), zap.Error(err))
		h.send(msg.Chat.ID, i18n.T(lang, "error.generic"))
		return
	}
	weekStats, err := h.habitUC.GetStats(ctx, user.ID, 7)
	if err != nil {
		h.logger.Error("GetStats week", zap.Error(err))
		h.send(msg.Chat.ID, i18n.T(lang, "error.generic"))
		return
	}
	monthStats, err := h.habitUC.GetStats(ctx, user.ID, 30)
	if err != nil {
		h.logger.Error("GetStats month", zap.Error(err))
		h.send(msg.Chat.ID, i18n.T(lang, "error.generic"))
		return
	}
	if len(monthStats) == 0 {
		h.send(msg.Chat.ID, i18n.T(lang, "stats.empty"))
		return
	}

	// Today done / total (non-paused habits only)
	now := time.Now()
	todayTotal, todayDone := 0, 0
	for _, habit := range habits {
		if habit.IsPaused {
			continue
		}
		todayTotal++
		if usecase.IsDoneToday(habit, now) {
			todayDone++
		}
	}

	// Week aggregation
	weekCompleted, weekTotal := 0, 0
	for _, s := range weekStats {
		weekCompleted += s.CompletedDays
		weekTotal += s.TotalDays
	}
	weekPct := 0
	if weekTotal > 0 {
		weekPct = weekCompleted * 100 / weekTotal
	}

	// Month aggregation
	monthCompleted, monthTotal := 0, 0
	for _, s := range monthStats {
		monthCompleted += s.CompletedDays
		monthTotal += s.TotalDays
	}
	monthPct := 0
	if monthTotal > 0 {
		monthPct = monthCompleted * 100 / monthTotal
	}

	var sb strings.Builder
	sb.WriteString(i18n.T(lang, "stats.header"))
	sb.WriteString("\n")
	sb.WriteString(i18n.T(lang, "stats.today_line", todayDone, todayTotal))
	sb.WriteString("\n")
	sb.WriteString(i18n.T(lang, "stats.week_line", weekPct, weekCompleted, weekTotal))
	sb.WriteString("\n")
	sb.WriteString(i18n.T(lang, "stats.month_line", monthPct, monthCompleted, monthTotal))
	sb.WriteString("\n\n")
	sb.WriteString(i18n.T(lang, "stats.xp_level", user.Level, user.XP, user.StreakShields))

	// 7-day mood summary
	weekFrom := now.AddDate(0, 0, -6)
	moods, err := h.moodUC.GetWeekMoods(ctx, user.ID, weekFrom, now.AddDate(0, 0, 1))
	if err != nil {
		h.logger.Warn("handleStats GetWeekMoods", zap.Error(err))
	} else if len(moods) > 0 {
		sb.WriteString(format.BuildMoodSummary(moods, lang))
	}

	// Per-habit buttons (tapping opens history)
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, s := range monthStats {
		var label string
		if s.Habit.IsPaused {
			label = i18n.T(lang, "stats.habit_btn_paused", s.Habit.Name)
		} else {
			label = i18n.T(lang, "stats.habit_btn", s.Habit.Name, s.Habit.Streak, s.CompletionPct)
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, fmt.Sprintf("history:%d", s.Habit.ID)),
		))
	}

	m := tgbotapi.NewMessage(msg.Chat.ID, sb.String())
	if len(rows) > 0 {
		m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	}
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send stats", zap.Error(err))
	}
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

func (h *Handler) handleMood(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	lang := h.lang(user)
	today := time.Now()
	moods, err := h.moodUC.GetWeekMoods(ctx, user.ID, today, today.AddDate(0, 0, 1))
	if err != nil {
		h.logger.Error("handleMood GetWeekMoods", zap.Error(err))
		h.send(msg.Chat.ID, i18n.T(lang, "error.generic"))
		return
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "mood.great"), "mood:3"),
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "mood.okay"), "mood:2"),
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "mood.tough"), "mood:1"),
		),
	)

	if len(moods) > 0 {
		moodEmojis := map[int]string{1: "😞", 2: "😐", 3: "😊"}
		emoji := moodEmojis[moods[0].Mood]
		m := tgbotapi.NewMessage(msg.Chat.ID, i18n.T(lang, "mood.already_logged", emoji))
		m.ReplyMarkup = keyboard
		if _, err := h.api.Send(m); err != nil {
			h.logger.Error("send mood already logged", zap.Error(err))
		}
		return
	}

	h.sendMoodPrompt(msg.Chat.ID, lang)
}

func (h *Handler) handleToday(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	lang := h.lang(user)
	habits, err := h.habitUC.ListHabits(ctx, user.ID)
	if err != nil {
		h.send(msg.Chat.ID, i18n.T(lang, "error.generic"))
		return
	}
	now := time.Now()

	// Count totals for the progress bar header.
	totalHabits, doneHabits := 0, 0
	for _, habit := range habits {
		if habit.IsPaused {
			continue
		}
		totalHabits++
		if usecase.IsDoneToday(habit, now) {
			doneHabits++
		}
	}

	if totalHabits == 0 {
		h.send(msg.Chat.ID, i18n.T(lang, "today.none"))
		return
	}

	if doneHabits == totalHabits {
		h.send(msg.Chat.ID, i18n.T(lang, "today.all_done"))
		return
	}

	bar := progressBar(doneHabits, totalHabits)
	pct := doneHabits * 100 / totalHabits

	var sb strings.Builder
	sb.WriteString(i18n.T(lang, "today.header_progress", doneHabits, totalHabits, bar, pct))

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

func (h *Handler) handleInsights(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	lang := h.lang(user)
	dow, err := h.habitUC.GetDayOfWeekStats(ctx, user.ID, user.Timezone)
	if err != nil {
		h.logger.Error("GetDayOfWeekStats", zap.Error(err))
		h.send(msg.Chat.ID, i18n.T(lang, "error.generic"))
		return
	}
	if len(dow) == 0 {
		h.send(msg.Chat.ID, i18n.T(lang, "insights.not_enough_data"))
		return
	}
	best, worst := format.BestAndWorstDay(dow)
	h.send(msg.Chat.ID, format.BuildDayOfWeekInsight(dow, best, worst, string(lang)))
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
