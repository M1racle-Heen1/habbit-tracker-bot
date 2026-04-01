package telegram

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"

	"github.com/saidakmal/habbit-tracker-bot/internal/domain"
	"github.com/saidakmal/habbit-tracker-bot/internal/i18n"
	"github.com/saidakmal/habbit-tracker-bot/internal/usecase"
)

func undoKey(habitID int64) string { return fmt.Sprintf("undo:%d", habitID) }

func timerKey(habitID, userID int64) string {
	return fmt.Sprintf("timer:%d:%d", habitID, userID)
}

func (h *Handler) cbLanguage(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, msgID int, arg string) {
	if arg != "ru" && arg != "en" && arg != "kz" {
		return
	}
	user, err := h.userUC.GetOrCreateUser(ctx, cq.From.ID, cq.From.UserName, cq.From.FirstName)
	if err != nil {
		h.send(chatID, i18n.T(i18n.RU, "error.generic"))
		return
	}
	isOnboarding := user.Language == ""
	if err := h.userUC.SetLanguage(ctx, user.ID, arg); err != nil {
		h.logger.Error("SetLanguage", zap.Error(err))
		h.send(chatID, i18n.T(arg, "error.generic"))
		return
	}
	labels := map[string]string{"ru": "🇷🇺 Русский", "en": "🇬🇧 English", "kz": "🇰🇿 Қазақша"}
	h.editMsg(chatID, msgID, "✅ "+labels[arg])

	if isOnboarding {
		h.sendTimezoneKeyboard(chatID, i18n.Lang(arg), "tz_ob:")
	}
}

func (h *Handler) cbTimezone(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, msgID int, tz string) {
	if _, err := time.LoadLocation(tz); err != nil {
		h.send(chatID, i18n.T(i18n.RU, "timezone.invalid"))
		return
	}
	user, _, err := h.getUserFromCallback(ctx, cq)
	if err != nil {
		return
	}
	if err := h.userUC.SetTimezone(ctx, user.ID, tz); err != nil {
		h.logger.Error("SetTimezone", zap.Error(err))
		h.send(chatID, i18n.T(h.lang(user), "error.update"))
		return
	}
	h.editMsg(chatID, msgID, i18n.T(h.lang(user), "timezone.set", tz))
}

func (h *Handler) cbTemplate(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, msgID int, arg string) {
	user, lang, err := h.getUserFromCallback(ctx, cq)
	if err != nil {
		return
	}

	if arg == "custom" {
		h.setState(cq.From.ID, &convState{Step: stepAwaitName, Lang: lang})
		h.removeKeyboard(chatID, msgID)
		h.send(chatID, i18n.T(lang, "habit.enter_name"))
		return
	}
	tmpl, ok := habitTemplates[arg]
	if !ok {
		return
	}
	h.clearState(cq.From.ID)
	habit, err := h.habitUC.CreateHabit(ctx, user.ID, tmpl.Name, tmpl.Interval, tmpl.Start, tmpl.End, 0)
	if err != nil {
		h.logger.Error("CreateHabit template", zap.Error(err))
		h.send(chatID, i18n.T(lang, "error.generic"))
		return
	}
	h.editMsg(chatID, msgID, i18n.T(lang, "habit.created",
		habit.Name, formatInterval(habit.IntervalMinutes, lang), habit.StartHour, habit.EndHour,
	))
}

func (h *Handler) cbInterval(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, msgID int, arg string) {
	minutes, err := strconv.Atoi(arg)
	if err != nil {
		return
	}
	state := h.getState(cq.From.ID)
	if state == nil || state.Step != stepAwaitInterval {
		h.removeKeyboard(chatID, msgID)
		return
	}
	_, lang, err := h.getUserFromCallback(ctx, cq)
	if err != nil {
		return
	}
	state.IntervalMinutes = minutes
	state.Step = stepAwaitStartHour
	h.setState(cq.From.ID, state)
	h.editMsg(chatID, msgID, i18n.T(lang, "wizard.interval_set", formatInterval(minutes, lang)))
	if err := h.sendStartHourKeyboard(chatID, lang); err != nil {
		h.clearState(cq.From.ID)
		h.send(chatID, i18n.T(lang, "error.generic"))
	}
}

func (h *Handler) cbStartHour(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, msgID int, arg string) {
	hour, err := strconv.Atoi(arg)
	if err != nil {
		return
	}
	state := h.getState(cq.From.ID)
	if state == nil || state.Step != stepAwaitStartHour {
		h.removeKeyboard(chatID, msgID)
		return
	}
	_, lang, err := h.getUserFromCallback(ctx, cq)
	if err != nil {
		return
	}
	state.StartHour = hour
	state.Step = stepAwaitEndHour
	h.setState(cq.From.ID, state)
	h.editMsg(chatID, msgID, i18n.T(lang, "wizard.start_set", hour))
	if err := h.sendEndHourKeyboard(chatID, lang, hour+1); err != nil {
		h.clearState(cq.From.ID)
		h.send(chatID, i18n.T(lang, "error.generic"))
	}
}

func (h *Handler) cbEndHour(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, msgID int, arg string) {
	endHour, err := strconv.Atoi(arg)
	if err != nil {
		return
	}
	state := h.getState(cq.From.ID)
	if state == nil || state.Step != stepAwaitEndHour || state.HabitName == "" {
		h.removeKeyboard(chatID, msgID)
		return
	}
	_, lang, err := h.getUserFromCallback(ctx, cq)
	if err != nil {
		return
	}
	h.editMsg(chatID, msgID, i18n.T(lang, "wizard.end_set", endHour))
	h.setState(cq.From.ID, &convState{
		Step:            stepAwaitGoal,
		HabitName:       state.HabitName,
		IntervalMinutes: state.IntervalMinutes,
		StartHour:       state.StartHour,
		EndHour:         endHour,
	})
	if err := h.sendGoalKeyboard(chatID, lang); err != nil {
		h.clearState(cq.From.ID)
		h.send(chatID, i18n.T(lang, "error.generic"))
	}
}

func (h *Handler) cbAddGoal(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, msgID int, arg string) {
	goalDays, err := strconv.Atoi(arg)
	if err != nil {
		return
	}
	state := h.getState(cq.From.ID)
	if state == nil || state.Step != stepAwaitGoal {
		h.removeKeyboard(chatID, msgID)
		return
	}
	endHour := state.EndHour
	h.clearState(cq.From.ID)

	user, lang, err := h.getUserFromCallback(ctx, cq)
	if err != nil {
		return
	}
	habit, err := h.habitUC.CreateHabit(ctx, user.ID, state.HabitName, state.IntervalMinutes, state.StartHour, endHour, goalDays)
	if err != nil {
		h.logger.Error("CreateHabit", zap.Error(err))
		h.send(chatID, i18n.T(lang, "error.generic"))
		return
	}
	result := i18n.T(lang, "habit.created",
		habit.Name, formatInterval(habit.IntervalMinutes, lang), habit.StartHour, habit.EndHour)
	if goalDays > 0 {
		result += "\n" + i18n.T(lang, "habit.goal_set", goalDays)
	}
	h.editMsg(chatID, msgID, result)
}

func (h *Handler) cbDone(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, msgID int, arg string) {
	habitID, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return
	}
	user, lang, err := h.getUserFromCallback(ctx, cq)
	if err != nil {
		return
	}

	prevHabit, _ := h.habitUC.GetHabit(ctx, habitID)

	if err := h.habitUC.MarkDone(ctx, user.ID, habitID); err != nil {
		if errors.Is(err, domain.ErrAlreadyDone) {
			h.editMsg(chatID, msgID, i18n.T(lang, "habit.already_done"))
			return
		}
		h.logger.Error("MarkDone", zap.Error(err))
		h.send(chatID, i18n.T(lang, "error.generic"))
		return
	}

	if prevHabit != nil {
		var prevTS int64
		if prevHabit.LastDoneAt != nil {
			prevTS = prevHabit.LastDoneAt.Unix()
		}
		undoVal := fmt.Sprintf("%d|%d", prevHabit.Streak, prevTS)
		_ = h.cache.Set(ctx, undoKey(habitID), undoVal, 5*time.Minute)
	}

	habit, err := h.habitUC.GetHabit(ctx, habitID)
	doneMsg := i18n.T(lang, "habit.done_simple")
	if err == nil {
		doneMsg = doneMessage(habit.Name, habit.Streak, habit.GoalDays, lang)
	}
	edit := tgbotapi.NewEditMessageText(chatID, msgID, doneMsg)
	if prevHabit != nil {
		kb := tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "habit.undo_btn"), fmt.Sprintf("undo:%d", habitID)),
		))
		edit.ReplyMarkup = &kb
	}
	if _, err := h.api.Send(edit); err != nil {
		h.logger.Error("edit done msg", zap.Error(err))
	}
}

func (h *Handler) cbUndo(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, msgID int, arg string) {
	habitID, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return
	}
	user, lang, err := h.getUserFromCallback(ctx, cq)
	if err != nil {
		return
	}

	val, err := h.cache.Get(ctx, undoKey(habitID))
	if err != nil {
		h.editMsg(chatID, msgID, i18n.T(lang, "habit.undo_expired"))
		return
	}
	_ = h.cache.Delete(ctx, undoKey(habitID))

	parts := strings.SplitN(val, "|", 2)
	if len(parts) != 2 {
		h.editMsg(chatID, msgID, i18n.T(lang, "error.generic"))
		return
	}
	prevStreak, _ := strconv.Atoi(parts[0])
	prevTS, _ := strconv.ParseInt(parts[1], 10, 64)
	var prevLastDoneAt *time.Time
	if prevTS != 0 {
		t := time.Unix(prevTS, 0)
		prevLastDoneAt = &t
	}

	if err := h.habitUC.UndoMarkDone(ctx, user.ID, habitID, prevStreak, prevLastDoneAt); err != nil {
		h.logger.Error("UndoMarkDone", zap.Error(err))
		h.editMsg(chatID, msgID, i18n.T(lang, "error.generic"))
		return
	}
	h.editMsg(chatID, msgID, i18n.T(lang, "habit.undo_done"))
}

func (h *Handler) cbDoneAll(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, msgID int) {
	user, lang, err := h.getUserFromCallback(ctx, cq)
	if err != nil {
		return
	}
	habits, err := h.habitUC.ListHabits(ctx, user.ID)
	if err != nil {
		h.editMsg(chatID, msgID, i18n.T(lang, "error.generic"))
		return
	}
	now := time.Now()
	for _, habit := range habits {
		if habit.IsPaused || usecase.IsDoneToday(habit, now) {
			continue
		}
		_ = h.habitUC.MarkDone(ctx, user.ID, habit.ID)
	}
	h.editMsg(chatID, msgID, i18n.T(lang, "today.all_done"))
}

func (h *Handler) cbTimerStart(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, msgID int, arg string) {
	habitID, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return
	}
	user, lang, err := h.getUserFromCallback(ctx, cq)
	if err != nil {
		return
	}

	if val, err := h.cache.Get(ctx, timerKey(habitID, user.ID)); err == nil {
		expiry, _ := strconv.ParseInt(val, 10, 64)
		remaining := int(time.Until(time.Unix(expiry, 0)).Minutes()) + 1
		if remaining > 0 {
			h.editMsg(chatID, msgID, i18n.T(lang, "timer.already_running", remaining))
			return
		}
	}

	habit, err := h.habitUC.GetHabit(ctx, habitID)
	if err != nil {
		h.send(chatID, i18n.T(lang, "error.generic"))
		return
	}

	m := tgbotapi.NewEditMessageText(chatID, msgID, i18n.T(lang, "timer.choose_duration", habit.Name))
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "timer.min_btn", 15), fmt.Sprintf("timer_set:%d:15", habitID)),
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "timer.min_btn", 30), fmt.Sprintf("timer_set:%d:30", habitID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "timer.min_btn", 45), fmt.Sprintf("timer_set:%d:45", habitID)),
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "timer.min_btn", 60), fmt.Sprintf("timer_set:%d:60", habitID)),
		),
	)
	m.ReplyMarkup = &kb
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send timer duration picker", zap.Error(err))
	}
}

func (h *Handler) cbTimerSet(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, msgID int, arg string) {
	parts := strings.SplitN(arg, ":", 2)
	if len(parts) != 2 {
		return
	}
	habitID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return
	}
	minutes, err := strconv.Atoi(parts[1])
	if err != nil {
		return
	}
	user, lang, err := h.getUserFromCallback(ctx, cq)
	if err != nil {
		return
	}
	habit, err := h.habitUC.GetHabit(ctx, habitID)
	if err != nil {
		h.send(chatID, i18n.T(lang, "error.generic"))
		return
	}

	expiry := time.Now().Add(time.Duration(minutes) * time.Minute)
	ttl := time.Duration(minutes+2) * time.Minute
	_ = h.cache.Set(ctx, timerKey(habitID, user.ID), strconv.FormatInt(expiry.Unix(), 10), ttl)

	h.editMsg(chatID, msgID, i18n.T(lang, "timer.started", minutes, habit.Name))
}

func (h *Handler) cbPreDelete(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, arg string) {
	habitID, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return
	}
	_, lang, err := h.getUserFromCallback(ctx, cq)
	if err != nil {
		return
	}
	habit, err := h.habitUC.GetHabit(ctx, habitID)
	if err != nil {
		h.send(chatID, i18n.T(lang, "habit.not_found"))
		return
	}
	warning := ""
	if habit.Streak > 0 {
		warning = i18n.T(lang, "habit.delete_streak_warn", habit.Streak)
	}
	m := tgbotapi.NewMessage(chatID, i18n.T(lang, "habit.delete_confirm", habit.Name, warning))
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "action.yes_delete"), fmt.Sprintf("confirm_delete:%d", habitID)),
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "action.cancel"), "cancel_delete:0"),
		),
	)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send confirm delete", zap.Error(err))
	}
}

func (h *Handler) cbConfirmDelete(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, msgID int, arg string) {
	habitID, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return
	}
	user, lang, err := h.getUserFromCallback(ctx, cq)
	if err != nil {
		return
	}
	if err := h.habitUC.DeleteHabit(ctx, user.ID, habitID); err != nil {
		h.logger.Error("DeleteHabit", zap.Error(err))
		h.send(chatID, i18n.T(lang, "error.generic"))
		return
	}
	h.editMsg(chatID, msgID, i18n.T(lang, "habit.deleted"))
}

func (h *Handler) cbCancelDelete(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, msgID int) {
	user, err := h.userUC.GetOrCreateUser(ctx, cq.From.ID, cq.From.UserName, cq.From.FirstName)
	lang := i18n.Lang(i18n.RU)
	if err == nil {
		lang = h.lang(user)
	}
	h.editMsg(chatID, msgID, i18n.T(lang, "action.delete_cancelled"))
}

func (h *Handler) cbEditMenu(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, arg string) {
	habitID, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return
	}
	_, lang, err := h.getUserFromCallback(ctx, cq)
	if err != nil {
		return
	}
	habit, err := h.habitUC.GetHabit(ctx, habitID)
	if err != nil {
		h.send(chatID, i18n.T(lang, "habit.not_found"))
		return
	}
	m := tgbotapi.NewMessage(chatID, i18n.T(lang, "habit.edit_what", habit.Name))
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "edit.name_btn"), fmt.Sprintf("edit_name:%d", habitID)),
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "edit.interval_btn"), fmt.Sprintf("edit_interval:%d:menu", habitID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "edit.hours_btn"), fmt.Sprintf("edit_start:%d:menu", habitID)),
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "edit.goal_btn"), fmt.Sprintf("goal_menu:%d", habitID)),
		),
	)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send edit menu", zap.Error(err))
	}
}

func (h *Handler) cbEditName(cq *tgbotapi.CallbackQuery, chatID int64, msgID int, arg string) {
	habitID, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return
	}
	// Determine lang without creating user (best-effort)
	lang := i18n.RU
	if user, err := h.userUC.GetOrCreateUser(context.Background(), cq.From.ID, cq.From.UserName, cq.From.FirstName); err == nil {
		lang = h.lang(user)
	}
	h.clearState(cq.From.ID)
	h.setState(cq.From.ID, &convState{Step: stepEditAwaitName, EditHabitID: habitID})
	h.removeKeyboard(chatID, msgID)
	h.send(chatID, i18n.T(lang, "habit.edit_enter_name"))
}

func (h *Handler) cbEditInterval(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, msgID int, arg string) {
	subparts := strings.SplitN(arg, ":", 2)
	habitID, err := strconv.ParseInt(subparts[0], 10, 64)
	if err != nil {
		return
	}
	user, lang, err := h.getUserFromCallback(ctx, cq)
	if err != nil {
		return
	}
	if len(subparts) == 1 || subparts[1] == "menu" {
		h.sendEditIntervalKeyboard(chatID, habitID, lang)
		return
	}
	minutes, err := strconv.Atoi(subparts[1])
	if err != nil {
		return
	}
	habit, err := h.habitUC.GetHabit(ctx, habitID)
	if err != nil {
		h.send(chatID, i18n.T(lang, "habit.not_found"))
		return
	}
	if _, err := h.habitUC.EditHabit(ctx, user.ID, habitID, habit.Name, minutes, habit.StartHour, habit.EndHour); err != nil {
		h.logger.Error("EditHabit interval", zap.Error(err))
		h.send(chatID, i18n.T(lang, "error.update"))
		return
	}
	h.editMsg(chatID, msgID, i18n.T(lang, "habit.interval_updated", formatInterval(minutes, lang)))
}

func (h *Handler) cbEditStart(cq *tgbotapi.CallbackQuery, chatID int64, msgID int, arg string) {
	subparts := strings.SplitN(arg, ":", 2)
	habitID, err := strconv.ParseInt(subparts[0], 10, 64)
	if err != nil {
		return
	}
	lang := i18n.RU
	user, err := h.userUC.GetOrCreateUser(context.Background(), cq.From.ID, cq.From.UserName, cq.From.FirstName)
	if err == nil {
		lang = h.lang(user)
	}
	if len(subparts) == 1 || subparts[1] == "menu" {
		h.sendEditStartHourKeyboard(chatID, habitID, lang)
		return
	}
	startHour, err := strconv.Atoi(subparts[1])
	if err != nil {
		return
	}
	h.clearState(cq.From.ID)
	h.setState(cq.From.ID, &convState{
		Step:        stepEditAwaitEndHour,
		EditHabitID: habitID,
		StartHour:   startHour,
	})
	h.editMsg(chatID, msgID, fmt.Sprintf("🕐 %d:00 ✓", startHour))
	h.sendEditEndHourKeyboard(chatID, habitID, startHour+1, lang)
}

func (h *Handler) cbEditEnd(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, msgID int, arg string) {
	subparts := strings.SplitN(arg, ":", 2)
	if len(subparts) != 2 {
		return
	}
	habitID, err := strconv.ParseInt(subparts[0], 10, 64)
	if err != nil {
		return
	}
	endHour, err := strconv.Atoi(subparts[1])
	if err != nil {
		return
	}
	state := h.getState(cq.From.ID)
	if state == nil || state.Step != stepEditAwaitEndHour || state.EditHabitID != habitID {
		h.removeKeyboard(chatID, msgID)
		return
	}
	startHour := state.StartHour
	h.clearState(cq.From.ID)

	user, lang, err := h.getUserFromCallback(ctx, cq)
	if err != nil {
		return
	}
	habit, err := h.habitUC.GetHabit(ctx, habitID)
	if err != nil {
		h.send(chatID, i18n.T(lang, "habit.not_found"))
		return
	}
	if _, err := h.habitUC.EditHabit(ctx, user.ID, habitID, habit.Name, habit.IntervalMinutes, startHour, endHour); err != nil {
		h.logger.Error("EditHabit hours", zap.Error(err))
		h.send(chatID, i18n.T(lang, "error.update"))
		return
	}
	h.editMsg(chatID, msgID, i18n.T(lang, "habit.hours_updated", startHour, endHour))
}

func (h *Handler) cbPauseResume(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, msgID int, arg string, pause bool) {
	habitID, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return
	}
	user, lang, err := h.getUserFromCallback(ctx, cq)
	if err != nil {
		return
	}
	habit, err := h.habitUC.GetHabit(ctx, habitID)
	if err != nil {
		h.send(chatID, i18n.T(lang, "habit.not_found"))
		return
	}
	if pause {
		if err := h.habitUC.PauseHabit(ctx, user.ID, habitID); err != nil {
			h.logger.Error("PauseHabit", zap.Error(err))
			h.send(chatID, i18n.T(lang, "error.generic"))
			return
		}
		h.editMsg(chatID, msgID, i18n.T(lang, "habit.paused", habit.Name))
	} else {
		if err := h.habitUC.ResumeHabit(ctx, user.ID, habitID); err != nil {
			h.logger.Error("ResumeHabit", zap.Error(err))
			h.send(chatID, i18n.T(lang, "error.generic"))
			return
		}
		h.editMsg(chatID, msgID, i18n.T(lang, "habit.resumed", habit.Name))
	}
}

func (h *Handler) cbSnooze(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, msgID int, arg string) {
	subparts := strings.SplitN(arg, ":", 2)
	if len(subparts) != 2 {
		return
	}
	habitID, err := strconv.ParseInt(subparts[0], 10, 64)
	if err != nil {
		return
	}
	minutes, err := strconv.Atoi(subparts[1])
	if err != nil {
		return
	}
	_, lang, err := h.getUserFromCallback(ctx, cq)
	if err != nil {
		return
	}
	if err := h.habitUC.SnoozeHabit(ctx, habitID, minutes); err != nil {
		h.logger.Error("SnoozeHabit", zap.Error(err))
		h.send(chatID, i18n.T(lang, "error.generic"))
		return
	}
	h.editMsg(chatID, msgID, i18n.T(lang, "snooze.remind_in", minutes))
}

func (h *Handler) cbHistory(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, arg string) {
	habitID, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return
	}
	user, lang, err := h.getUserFromCallback(ctx, cq)
	if err != nil {
		return
	}
	habit, err := h.habitUC.GetHabit(ctx, habitID)
	if err != nil {
		h.send(chatID, i18n.T(lang, "habit.not_found"))
		return
	}

	now := time.Now()
	from := now.AddDate(0, 0, -27)
	dates, err := h.habitUC.GetHistory(ctx, user.ID, habitID, from, now.AddDate(0, 0, 1))
	if err != nil {
		h.logger.Error("GetHistory", zap.Error(err))
		h.send(chatID, i18n.T(lang, "error.generic"))
		return
	}

	doneSet := make(map[string]bool, len(dates))
	for _, d := range dates {
		doneSet[d.Format("2006-01-02")] = true
	}
	h.send(chatID, buildHeatmap(habit.Name, from, now, doneSet, lang))
}

func (h *Handler) cbGoalMenu(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, arg string) {
	habitID, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return
	}
	_, lang, err := h.getUserFromCallback(ctx, cq)
	if err != nil {
		return
	}
	m := tgbotapi.NewMessage(chatID, i18n.T(lang, "goal.choose"))
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "goal.days_btn", 21), fmt.Sprintf("set_goal:%d:21", habitID)),
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "goal.days_btn", 30), fmt.Sprintf("set_goal:%d:30", habitID)),
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "goal.days_btn", 66), fmt.Sprintf("set_goal:%d:66", habitID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "goal.days_btn", 100), fmt.Sprintf("set_goal:%d:100", habitID)),
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "goal.no_goal"), fmt.Sprintf("set_goal:%d:0", habitID)),
		),
	)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send goal menu", zap.Error(err))
	}
}

func (h *Handler) cbTimezoneOnboard(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, msgID int, tz string) {
	if _, err := time.LoadLocation(tz); err != nil {
		h.send(chatID, i18n.T(i18n.RU, "timezone.invalid"))
		return
	}
	user, lang, err := h.getUserFromCallback(ctx, cq)
	if err != nil {
		return
	}
	if err := h.userUC.SetTimezone(ctx, user.ID, tz); err != nil {
		h.logger.Error("SetTimezone onboard", zap.Error(err))
		h.send(chatID, i18n.T(lang, "error.generic"))
		return
	}
	h.editMsg(chatID, msgID, i18n.T(lang, "timezone.set", tz))
	h.send(chatID, i18n.T(lang, "onboarding.welcome_screen"))
	m := tgbotapi.NewMessage(chatID, i18n.T(lang, "habit.choose_template"))
	m.ReplyMarkup = onboardTemplateKeyboard(lang)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send template keyboard onboard", zap.Error(err))
	}
}

func (h *Handler) cbOnboardSkip(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, msgID int) {
	_, lang, err := h.getUserFromCallback(ctx, cq)
	if err != nil {
		return
	}
	h.removeKeyboard(chatID, msgID)
	h.sendMainNav(chatID, lang)
}

func (h *Handler) cbSettings(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, arg string) {
	_, lang, err := h.getUserFromCallback(ctx, cq)
	if err != nil {
		return
	}
	switch arg {
	case "language":
		m := tgbotapi.NewMessage(chatID, "Выбери язык / Choose language / Тіл таңда:")
		m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🇷🇺 Русский", "lang:ru"),
			tgbotapi.NewInlineKeyboardButtonData("🇬🇧 English", "lang:en"),
			tgbotapi.NewInlineKeyboardButtonData("🇰🇿 Қазақша", "lang:kz"),
		))
		if _, err := h.api.Send(m); err != nil {
			h.logger.Error("send language from settings", zap.Error(err))
		}
	case "timezone":
		h.sendTimezoneKeyboard(chatID, lang, "tz:")
	}
}

func (h *Handler) cbSetGoal(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, msgID int, arg string) {
	subparts := strings.SplitN(arg, ":", 2)
	if len(subparts) != 2 {
		return
	}
	habitID, err := strconv.ParseInt(subparts[0], 10, 64)
	if err != nil {
		return
	}
	days, err := strconv.Atoi(subparts[1])
	if err != nil {
		return
	}
	user, lang, err := h.getUserFromCallback(ctx, cq)
	if err != nil {
		return
	}
	if err := h.habitUC.SetGoal(ctx, user.ID, habitID, days); err != nil {
		h.logger.Error("SetGoal", zap.Error(err))
		h.send(chatID, i18n.T(lang, "error.generic"))
		return
	}
	if days == 0 {
		h.editMsg(chatID, msgID, i18n.T(lang, "goal.removed"))
	} else {
		h.editMsg(chatID, msgID, i18n.T(lang, "goal.set", days))
	}
}
