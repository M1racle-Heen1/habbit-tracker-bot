package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"

	"github.com/saidakmal/habbit-tracker-bot/internal/domain"
	"github.com/saidakmal/habbit-tracker-bot/internal/gamification"
	"github.com/saidakmal/habbit-tracker-bot/internal/i18n"
	"github.com/saidakmal/habbit-tracker-bot/internal/usecase"
)

const handlerTimeout = 10 * time.Second
const stateTTL = 30 * time.Minute

type step int

const (
	stepIdle step = iota
	stepAwaitName
	stepAwaitInterval
	stepAwaitStartHour
	stepAwaitEndHour
	stepAwaitGoal
	stepEditAwaitName
	stepEditAwaitEndHour // separate from stepAwaitEndHour to prevent cross-flow conflicts
	stepOnboardTimezone  // new user: waiting for timezone after language chosen
	stepOnboardHabit     // new user: waiting for Yes/Later on first habit
)

type convState struct {
	Step            step        `json:"step"`
	HabitName       string      `json:"habit_name"`
	IntervalMinutes int         `json:"interval_minutes"`
	StartHour       int         `json:"start_hour"`
	EditHabitID     int64       `json:"edit_habit_id"`
	Lang            i18n.Lang   `json:"lang"`
}

var habitTemplates = map[string]struct {
	Name     string
	Interval int
	Start    int
	End      int
}{
	"water":    {"💧 Пить воду", 60, 8, 22},
	"exercise": {"🏃 Зарядка", 180, 7, 10},
	"read":     {"📚 Читать", 480, 20, 23},
	"sleep":    {"😴 Режим сна", 480, 21, 23},
	"meditate": {"🧘 Медитация", 480, 7, 10},
}

var commonTimezones = []struct {
	Label string
	Value string
}{
	{"UTC", "UTC"},
	{"Москва (UTC+3)", "Europe/Moscow"},
	{"Ташкент (UTC+5)", "Asia/Tashkent"},
	{"Алматы (UTC+5)", "Asia/Almaty"},
	{"Дубай (UTC+4)", "Asia/Dubai"},
	{"Берлин (UTC+1)", "Europe/Berlin"},
	{"Лондон (UTC+0)", "Europe/London"},
	{"Нью-Йорк (UTC-5)", "America/New_York"},
	{"Токио (UTC+9)", "Asia/Tokyo"},
}

type Handler struct {
	habitUC *usecase.HabitUsecase
	userUC  *usecase.UserUsecase
	api     *tgbotapi.BotAPI
	logger  *zap.Logger
	cache   usecase.Cache
}

func NewHandler(habitUC *usecase.HabitUsecase, userUC *usecase.UserUsecase, api *tgbotapi.BotAPI, logger *zap.Logger, cache usecase.Cache) *Handler {
	return &Handler{
		habitUC: habitUC,
		userUC:  userUC,
		api:     api,
		logger:  logger,
		cache:   cache,
	}
}

func (h *Handler) lang(user *domain.User) i18n.Lang {
	if user.Language == "" {
		return i18n.RU
	}
	return user.Language
}

// ── State helpers (Redis-backed) ─────────────────────────────────────────────

func stateKey(telegramID int64) string { return fmt.Sprintf("state:%d", telegramID) }

func (h *Handler) getState(telegramID int64) *convState {
	data, err := h.cache.Get(context.Background(), stateKey(telegramID))
	if err != nil || data == "" {
		return nil
	}
	var s convState
	if err := json.Unmarshal([]byte(data), &s); err != nil {
		return nil
	}
	return &s
}

func (h *Handler) setState(telegramID int64, s *convState) {
	data, _ := json.Marshal(s)
	_ = h.cache.Set(context.Background(), stateKey(telegramID), string(data), stateTTL)
}

func (h *Handler) clearState(telegramID int64) {
	_ = h.cache.Delete(context.Background(), stateKey(telegramID))
}

// ── Message editing helpers ───────────────────────────────────────────────────

// removeKeyboard replaces the inline keyboard on a message with an empty one.
// This prevents users from re-clicking buttons from old messages.
func (h *Handler) removeKeyboard(chatID int64, messageID int) {
	edit := tgbotapi.NewEditMessageReplyMarkup(chatID, messageID, tgbotapi.InlineKeyboardMarkup{})
	if _, err := h.api.Request(edit); err != nil {
		h.logger.Warn("remove keyboard", zap.Error(err))
	}
}

// editMsg updates the text of an existing message and removes its keyboard.
func (h *Handler) editMsg(chatID int64, messageID int, text string) {
	edit := tgbotapi.NewEditMessageText(chatID, messageID, text)
	if _, err := h.api.Request(edit); err != nil {
		h.logger.Warn("edit message text", zap.Error(err))
	}
}

// ── Entry point ───────────────────────────────────────────────────────────────

func (h *Handler) HandleUpdate(update tgbotapi.Update) {
	if update.CallbackQuery != nil {
		h.handleCallback(update.CallbackQuery)
		return
	}
	if update.Message == nil {
		return
	}

	msg := update.Message
	ctx, cancel := context.WithTimeout(context.Background(), handlerTimeout)
	defer cancel()

	user, err := h.userUC.GetOrCreateUser(ctx, msg.From.ID, msg.From.UserName, msg.From.FirstName)
	if err != nil {
		h.logger.Error("GetOrCreateUser", zap.Error(err))
		h.send(msg.Chat.ID, "Произошла ошибка, попробуй позже.")
		return
	}

	if msg.IsCommand() {
		h.handleCommand(ctx, msg, user)
	} else {
		h.handleText(ctx, msg, user)
	}
}

// ── Command router ────────────────────────────────────────────────────────────

func (h *Handler) handleCommand(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	// Any command cancels an active wizard (except /cancel itself which just cancels)
	switch msg.Command() {
	case "cancel":
		h.clearState(msg.From.ID)
		h.send(msg.Chat.ID, "❌ Действие отменено.")
		return
	}

	// Clear any pending wizard state when user runs a different command
	h.clearState(msg.From.ID)

	switch msg.Command() {
	case "start":
		h.handleStart(ctx, msg, user)
	case "list_habits", "habits":
		h.handleListHabits(ctx, msg, user)
	case "add_habit", "add":
		h.startAddHabit(msg, user)
	case "done":
		h.handleDone(ctx, msg, user)
	case "delete_habit":
		h.handleDeleteHabit(ctx, msg, user)
	case "edit_habit":
		h.handleEditHabit(ctx, msg, user)
	case "pause_habit":
		h.handlePauseHabit(ctx, msg, user)
	case "resume_habit":
		h.handleResumeHabit(ctx, msg, user)
	case "stats":
		h.handleStats(ctx, msg, user)
	case "history":
		h.handleHistory(ctx, msg, user)
	case "timezone":
		h.handleTimezone(msg)
	case "language":
		h.handleLanguage(msg)
	case "today":
		h.handleToday(ctx, msg, user)
	case "achievements":
		h.handleAchievements(ctx, msg, user)
	case "health":
		h.send(msg.Chat.ID, "OK")
	}
}

// ── Text input handler ────────────────────────────────────────────────────────

func (h *Handler) handleText(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	state := h.getState(msg.From.ID)
	if state == nil {
		return
	}

	switch state.Step {
	case stepAwaitName:
		name := strings.TrimSpace(msg.Text)
		if name == "" {
			h.send(msg.Chat.ID, i18n.T(h.lang(user), "habit.name_empty"))
			return
		}
		state.HabitName = name
		state.Step = stepAwaitInterval
		h.setState(msg.From.ID, state)
		if err := h.sendIntervalKeyboard(msg.Chat.ID, h.lang(user)); err != nil {
			h.clearState(msg.From.ID)
			h.send(msg.Chat.ID, i18n.T(h.lang(user), "error.generic"))
		}

	case stepEditAwaitName:
		lang := h.lang(user)
		name := strings.TrimSpace(msg.Text)
		if name == "" {
			h.send(msg.Chat.ID, i18n.T(lang, "habit.name_empty"))
			return
		}
		habit, err := h.habitUC.GetHabit(ctx, state.EditHabitID)
		if err != nil {
			h.send(msg.Chat.ID, i18n.T(lang, "habit.not_found"))
			h.clearState(msg.From.ID)
			return
		}
		if _, err := h.habitUC.EditHabit(ctx, user.ID, state.EditHabitID, name, habit.IntervalMinutes, habit.StartHour, habit.EndHour); err != nil {
			h.logger.Error("EditHabit name", zap.Error(err))
			h.send(msg.Chat.ID, i18n.T(lang, "error.generic"))
		} else {
			h.send(msg.Chat.ID, i18n.T(lang, "habit.edit_name_done", name))
		}
		h.clearState(msg.From.ID)

	default:
		if err := h.resendCurrentStep(msg.Chat.ID, h.lang(user), state); err != nil {
			h.clearState(msg.From.ID)
			h.send(msg.Chat.ID, i18n.T(h.lang(user), "error.generic"))
		}
	}
}

// ── Callback router ───────────────────────────────────────────────────────────

func (h *Handler) handleCallback(cq *tgbotapi.CallbackQuery) {
	ctx, cancel := context.WithTimeout(context.Background(), handlerTimeout)
	defer cancel()

	if _, err := h.api.Request(tgbotapi.NewCallback(cq.ID, "")); err != nil {
		h.logger.Warn("answer callback", zap.Error(err))
	}

	chatID := cq.Message.Chat.ID
	msgID := cq.Message.MessageID
	parts := strings.SplitN(cq.Data, ":", 2)
	if len(parts) != 2 {
		return
	}
	action, arg := parts[0], parts[1]

	switch action {
	case "interval":
		h.cbInterval(ctx, cq, chatID, msgID, arg)
	case "start_hour":
		h.cbStartHour(ctx, cq, chatID, msgID, arg)
	case "end_hour":
		h.cbEndHour(ctx, cq, chatID, msgID, arg)
	case "add_goal":
		h.cbAddGoal(ctx, cq, chatID, msgID, arg)
	case "template":
		h.cbTemplate(ctx, cq, chatID, msgID, arg)
	case "done":
		h.cbDone(ctx, cq, chatID, msgID, arg)
	case "done_all":
		h.cbDoneAll(ctx, cq, chatID, msgID)
	case "undo":
		h.cbUndo(ctx, cq, chatID, msgID, arg)
	case "timer_start":
		h.cbTimerStart(ctx, cq, chatID, msgID, arg)
	case "timer_set":
		h.cbTimerSet(ctx, cq, chatID, msgID, arg)
	case "pre_delete":
		h.cbPreDelete(ctx, cq, chatID, arg)
	case "confirm_delete":
		h.cbConfirmDelete(ctx, cq, chatID, msgID, arg)
	case "cancel_delete":
		h.cbCancelDelete(ctx, cq, chatID, msgID)
	case "snooze":
		h.cbSnooze(ctx, chatID, msgID, arg)
	case "pause":
		h.cbPauseResume(ctx, cq, chatID, msgID, arg, true)
	case "resume":
		h.cbPauseResume(ctx, cq, chatID, msgID, arg, false)
	case "lang":
		h.cbLanguage(ctx, cq, chatID, msgID, arg)
	case "tz":
		h.cbTimezone(ctx, cq, chatID, msgID, arg)
	case "tz_ob":
		h.cbOnboardTimezone(ctx, cq, chatID, msgID, arg)
	case "onboard_habit":
		h.cbOnboardHabit(ctx, cq, chatID, msgID, arg)
	case "history":
		h.cbHistory(ctx, cq, chatID, arg)
	case "edit":
		h.cbEditMenu(ctx, chatID, arg)
	case "edit_name":
		h.cbEditName(cq, chatID, msgID, arg)
	case "edit_interval":
		h.cbEditInterval(ctx, cq, chatID, msgID, arg)
	case "edit_start":
		h.cbEditStart(cq, chatID, msgID, arg)
	case "edit_end":
		h.cbEditEnd(ctx, cq, chatID, msgID, arg)
	case "set_goal":
		h.cbSetGoal(ctx, cq, chatID, msgID, arg)
	case "goal_menu":
		h.cbGoalMenu(chatID, arg)
	}
}

// ── Add habit flow ─────────────────────────────────────────────────────────────

func (h *Handler) handleStart(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	lang := h.lang(user)

	isNew := time.Since(user.CreatedAt) < 60*time.Second
	if isNew {
		h.setState(msg.From.ID, &convState{Step: stepOnboardTimezone})
		m := tgbotapi.NewMessage(msg.Chat.ID, i18n.T(lang, "language.choose"))
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
		text := i18n.T(lang, "onboarding.welcome_new", user.FirstName)
		m := tgbotapi.NewMessage(msg.Chat.ID, text)
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

func (h *Handler) cbLanguage(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, msgID int, arg string) {
	if arg != "ru" && arg != "en" && arg != "kz" {
		return
	}
	user, err := h.userUC.GetOrCreateUser(ctx, cq.From.ID, cq.From.UserName, cq.From.FirstName)
	if err != nil {
		h.send(chatID, i18n.T(i18n.RU, "error.generic"))
		return
	}
	if err := h.userUC.SetLanguage(ctx, user.ID, arg); err != nil {
		h.logger.Error("SetLanguage", zap.Error(err))
		h.send(chatID, i18n.T(arg, "error.generic"))
		return
	}
	labels := map[string]string{"ru": "🇷🇺 Русский", "en": "🇬🇧 English", "kz": "🇰🇿 Қазақша"}
	h.editMsg(chatID, msgID, "✅ "+labels[arg])

	state := h.getState(cq.From.ID)
	if state != nil && state.Step == stepOnboardTimezone {
		h.sendOnboardTimezone(chatID, arg)
	}
}

func (h *Handler) sendOnboardTimezone(chatID int64, lang string) {
	m := tgbotapi.NewMessage(chatID, i18n.T(lang, "timezone.choose"))
	var rows [][]tgbotapi.InlineKeyboardButton
	for i := 0; i < len(commonTimezones); i += 2 {
		row := []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(commonTimezones[i].Label, "tz_ob:"+commonTimezones[i].Value),
		}
		if i+1 < len(commonTimezones) {
			row = append(row, tgbotapi.NewInlineKeyboardButtonData(commonTimezones[i+1].Label, "tz_ob:"+commonTimezones[i+1].Value))
		}
		rows = append(rows, row)
	}
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send onboard timezone", zap.Error(err))
	}
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

func (h *Handler) cbTemplate(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, msgID int, arg string) {
	user, err := h.userUC.GetOrCreateUser(ctx, cq.From.ID, cq.From.UserName, cq.From.FirstName)
	if err != nil {
		h.send(chatID, i18n.T(i18n.RU, "error.generic"))
		return
	}
	lang := h.lang(user)

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
		habit.Name, formatInterval(habit.IntervalMinutes), habit.StartHour, habit.EndHour,
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
	user, err := h.userUC.GetOrCreateUser(ctx, cq.From.ID, cq.From.UserName, cq.From.FirstName)
	if err != nil {
		h.send(chatID, i18n.T(i18n.RU, "error.generic"))
		return
	}
	lang := h.lang(user)
	state.IntervalMinutes = minutes
	state.Step = stepAwaitStartHour
	h.setState(cq.From.ID, state)
	h.editMsg(chatID, msgID, fmt.Sprintf("⏱ Интервал: %s ✓", formatInterval(minutes)))
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
	user, err := h.userUC.GetOrCreateUser(ctx, cq.From.ID, cq.From.UserName, cq.From.FirstName)
	if err != nil {
		h.send(chatID, i18n.T(i18n.RU, "error.generic"))
		return
	}
	lang := h.lang(user)
	state.StartHour = hour
	state.Step = stepAwaitEndHour
	h.setState(cq.From.ID, state)
	h.editMsg(chatID, msgID, fmt.Sprintf("🕐 Начало: %d:00 ✓", hour))
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
	user, err := h.userUC.GetOrCreateUser(ctx, cq.From.ID, cq.From.UserName, cq.From.FirstName)
	if err != nil {
		h.send(chatID, i18n.T(i18n.RU, "error.generic"))
		return
	}
	lang := h.lang(user)
	h.editMsg(chatID, msgID, fmt.Sprintf("🕕 Конец: %d:00 ✓", endHour))
	h.setState(cq.From.ID, &convState{
		Step:            stepAwaitGoal,
		HabitName:       state.HabitName,
		IntervalMinutes: state.IntervalMinutes,
		StartHour:       state.StartHour,
		EditHabitID:     int64(endHour),
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
	endHour := int(state.EditHabitID)
	h.clearState(cq.From.ID)

	user, err := h.userUC.GetOrCreateUser(ctx, cq.From.ID, cq.From.UserName, cq.From.FirstName)
	if err != nil {
		h.send(chatID, i18n.T(i18n.RU, "error.generic"))
		return
	}
	lang := h.lang(user)
	habit, err := h.habitUC.CreateHabit(ctx, user.ID, state.HabitName, state.IntervalMinutes, state.StartHour, endHour, goalDays)
	if err != nil {
		h.logger.Error("CreateHabit", zap.Error(err))
		h.send(chatID, i18n.T(lang, "error.generic"))
		return
	}
	result := i18n.T(lang, "habit.created",
		habit.Name, formatInterval(habit.IntervalMinutes), habit.StartHour, habit.EndHour)
	if goalDays > 0 {
		result += "\n" + i18n.T(lang, "habit.goal_set", goalDays)
	}
	h.editMsg(chatID, msgID, result)
}

// ── Done ──────────────────────────────────────────────────────────────────────

func (h *Handler) handleDone(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	habits, err := h.habitUC.ListHabits(ctx, user.ID)
	if err != nil || len(habits) == 0 {
		h.send(msg.Chat.ID, "Нет привычек для отметки.")
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
		h.send(msg.Chat.ID, "Все активные привычки уже выполнены или на паузе!")
		return
	}
	m := tgbotapi.NewMessage(msg.Chat.ID, "Выбери привычку:")
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send done keyboard", zap.Error(err))
	}
}

func undoKey(habitID int64) string { return fmt.Sprintf("undo:%d", habitID) }

func (h *Handler) cbDone(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, msgID int, arg string) {
	habitID, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return
	}
	user, err := h.userUC.GetOrCreateUser(ctx, cq.From.ID, cq.From.UserName, cq.From.FirstName)
	if err != nil {
		h.logger.Error("GetOrCreateUser", zap.Error(err))
		return
	}
	lang := h.lang(user)

	// Capture state before marking done so we can undo
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

	// Store undo state in Redis for 5 minutes
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
	user, err := h.userUC.GetOrCreateUser(ctx, cq.From.ID, cq.From.UserName, cq.From.FirstName)
	if err != nil {
		h.logger.Error("GetOrCreateUser cbUndo", zap.Error(err))
		return
	}
	lang := h.lang(user)

	val, err := h.cache.Get(ctx, undoKey(habitID))
	if err != nil {
		// TTL expired or never set
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
	user, err := h.userUC.GetOrCreateUser(ctx, cq.From.ID, cq.From.UserName, cq.From.FirstName)
	if err != nil {
		h.logger.Error("GetOrCreateUser cbDoneAll", zap.Error(err))
		return
	}
	lang := h.lang(user)
	habits, err := h.habitUC.ListHabits(ctx, user.ID)
	if err != nil {
		h.editMsg(chatID, msgID, i18n.T(lang, "error.generic"))
		return
	}
	now := time.Now()
	marked := 0
	for _, habit := range habits {
		if habit.IsPaused || usecase.IsDoneToday(habit, now) {
			continue
		}
		if err := h.habitUC.MarkDone(ctx, user.ID, habit.ID); err == nil {
			marked++
		}
	}
	h.editMsg(chatID, msgID, i18n.T(lang, "today.all_done"))
	_ = marked
}

// ── Timer ─────────────────────────────────────────────────────────────────────

func timerKey(habitID, userID int64) string {
	return fmt.Sprintf("timer:%d:%d", habitID, userID)
}

func (h *Handler) cbTimerStart(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, msgID int, arg string) {
	habitID, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return
	}
	user, err := h.userUC.GetOrCreateUser(ctx, cq.From.ID, cq.From.UserName, cq.From.FirstName)
	if err != nil {
		return
	}
	lang := h.lang(user)

	// Check if timer already running
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
			tgbotapi.NewInlineKeyboardButtonData("15 мин", fmt.Sprintf("timer_set:%d:15", habitID)),
			tgbotapi.NewInlineKeyboardButtonData("30 мин", fmt.Sprintf("timer_set:%d:30", habitID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("45 мин", fmt.Sprintf("timer_set:%d:45", habitID)),
			tgbotapi.NewInlineKeyboardButtonData("60 мин", fmt.Sprintf("timer_set:%d:60", habitID)),
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
	user, err := h.userUC.GetOrCreateUser(ctx, cq.From.ID, cq.From.UserName, cq.From.FirstName)
	if err != nil {
		return
	}
	lang := h.lang(user)
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

// ── List habits ───────────────────────────────────────────────────────────────

func (h *Handler) handleListHabits(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	habits, err := h.habitUC.ListHabits(ctx, user.ID)
	if err != nil {
		h.logger.Error("ListHabits", zap.Int64("user_id", user.ID), zap.Error(err))
		h.send(msg.Chat.ID, "Не удалось загрузить привычки, попробуй позже.")
		return
	}
	if len(habits) == 0 {
		h.send(msg.Chat.ID, "У тебя пока нет привычек. Добавь: /add_habit")
		return
	}

	now := time.Now()
	var sb strings.Builder
	sb.WriteString("Твои привычки:\n\n")

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
			pct := habit.Streak * 100 / habit.GoalDays
			if pct > 100 {
				pct = 100
			}
			goalStr = fmt.Sprintf(" [цель: %d/%d дней]", habit.Streak, habit.GoalDays)
		}

		sb.WriteString(fmt.Sprintf("%s %s%s%s\n   %s, %d:00–%d:00\n\n",
			mark, habit.Name, streakStr, goalStr,
			formatInterval(habit.IntervalMinutes), habit.StartHour, habit.EndHour,
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

// ── Delete ────────────────────────────────────────────────────────────────────

func (h *Handler) handleDeleteHabit(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	habits, err := h.habitUC.ListHabits(ctx, user.ID)
	if err != nil || len(habits) == 0 {
		h.send(msg.Chat.ID, "Нет привычек для удаления.")
		return
	}
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, habit := range habits {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🗑 "+habit.Name, fmt.Sprintf("pre_delete:%d", habit.ID)),
		))
	}
	m := tgbotapi.NewMessage(msg.Chat.ID, "Выбери привычку для удаления:")
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send delete keyboard", zap.Error(err))
	}
}

func (h *Handler) cbPreDelete(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, arg string) {
	habitID, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return
	}
	user, err := h.userUC.GetOrCreateUser(ctx, cq.From.ID, cq.From.UserName, cq.From.FirstName)
	if err != nil {
		h.send(chatID, i18n.T(i18n.RU, "error.generic"))
		return
	}
	lang := h.lang(user)
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
	user, err := h.userUC.GetOrCreateUser(ctx, cq.From.ID, cq.From.UserName, cq.From.FirstName)
	if err != nil {
		h.send(chatID, i18n.T(i18n.RU, "error.generic"))
		return
	}
	lang := h.lang(user)
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

// ── Edit habit ────────────────────────────────────────────────────────────────

func (h *Handler) handleEditHabit(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	habits, err := h.habitUC.ListHabits(ctx, user.ID)
	if err != nil || len(habits) == 0 {
		h.send(msg.Chat.ID, "Нет привычек для редактирования.")
		return
	}
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, habit := range habits {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✏️ "+habit.Name, fmt.Sprintf("edit:%d", habit.ID)),
		))
	}
	m := tgbotapi.NewMessage(msg.Chat.ID, "Выбери привычку для редактирования:")
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send edit keyboard", zap.Error(err))
	}
}

func (h *Handler) cbEditMenu(ctx context.Context, chatID int64, arg string) {
	habitID, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return
	}
	habit, err := h.habitUC.GetHabit(ctx, habitID)
	if err != nil {
		h.send(chatID, "Привычка не найдена.")
		return
	}
	text := fmt.Sprintf("✏️ «%s»\nЧто изменить?", habit.Name)
	m := tgbotapi.NewMessage(chatID, text)
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📝 Название", fmt.Sprintf("edit_name:%d", habitID)),
			tgbotapi.NewInlineKeyboardButtonData("⏱ Интервал", fmt.Sprintf("edit_interval:%d:menu", habitID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🕐 Часы", fmt.Sprintf("edit_start:%d:menu", habitID)),
			tgbotapi.NewInlineKeyboardButtonData("🎯 Цель", fmt.Sprintf("goal_menu:%d", habitID)),
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
	h.clearState(cq.From.ID)
	h.setState(cq.From.ID, &convState{Step: stepEditAwaitName, EditHabitID: habitID})
	h.removeKeyboard(chatID, msgID)
	h.send(chatID, "Введи новое название привычки:")
}

func (h *Handler) cbEditInterval(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, msgID int, arg string) {
	subparts := strings.SplitN(arg, ":", 2)
	habitID, err := strconv.ParseInt(subparts[0], 10, 64)
	if err != nil {
		return
	}
	if len(subparts) == 1 || subparts[1] == "menu" {
		h.sendEditIntervalKeyboard(chatID, habitID)
		return
	}
	minutes, err := strconv.Atoi(subparts[1])
	if err != nil {
		return
	}
	user, err := h.userUC.GetOrCreateUser(ctx, cq.From.ID, cq.From.UserName, cq.From.FirstName)
	if err != nil {
		h.send(chatID, "Ошибка, попробуй позже.")
		return
	}
	habit, err := h.habitUC.GetHabit(ctx, habitID)
	if err != nil {
		h.send(chatID, "Привычка не найдена.")
		return
	}
	if _, err := h.habitUC.EditHabit(ctx, user.ID, habitID, habit.Name, minutes, habit.StartHour, habit.EndHour); err != nil {
		h.logger.Error("EditHabit interval", zap.Error(err))
		h.send(chatID, "Ошибка обновления.")
		return
	}
	h.editMsg(chatID, msgID, fmt.Sprintf("✅ Интервал изменён: %s", formatInterval(minutes)))
}

func (h *Handler) cbEditStart(cq *tgbotapi.CallbackQuery, chatID int64, msgID int, arg string) {
	subparts := strings.SplitN(arg, ":", 2)
	habitID, err := strconv.ParseInt(subparts[0], 10, 64)
	if err != nil {
		return
	}
	if len(subparts) == 1 || subparts[1] == "menu" {
		h.sendEditStartHourKeyboard(chatID, habitID)
		return
	}
	startHour, err := strconv.Atoi(subparts[1])
	if err != nil {
		return
	}
	// Use stepEditAwaitEndHour (distinct from stepAwaitEndHour used by ADD flow)
	// This prevents old `end_hour:X` buttons from the ADD flow from interfering
	h.clearState(cq.From.ID)
	h.setState(cq.From.ID, &convState{
		Step:        stepEditAwaitEndHour,
		EditHabitID: habitID,
		StartHour:   startHour,
	})
	h.editMsg(chatID, msgID, fmt.Sprintf("🕐 Начало: %d:00 ✓", startHour))
	h.sendEditEndHourKeyboard(chatID, habitID, startHour+1)
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
	// Must be in EDIT end-hour flow specifically
	if state == nil || state.Step != stepEditAwaitEndHour || state.EditHabitID != habitID {
		h.removeKeyboard(chatID, msgID)
		return
	}
	startHour := state.StartHour
	h.clearState(cq.From.ID)

	user, err := h.userUC.GetOrCreateUser(ctx, cq.From.ID, cq.From.UserName, cq.From.FirstName)
	if err != nil {
		h.send(chatID, "Ошибка, попробуй позже.")
		return
	}
	habit, err := h.habitUC.GetHabit(ctx, habitID)
	if err != nil {
		h.send(chatID, "Привычка не найдена.")
		return
	}
	if _, err := h.habitUC.EditHabit(ctx, user.ID, habitID, habit.Name, habit.IntervalMinutes, startHour, endHour); err != nil {
		h.logger.Error("EditHabit hours", zap.Error(err))
		h.send(chatID, "Ошибка обновления.")
		return
	}
	h.editMsg(chatID, msgID, fmt.Sprintf("✅ Часы обновлены: %d:00–%d:00", startHour, endHour))
}

// ── Pause / Resume ────────────────────────────────────────────────────────────

func (h *Handler) handlePauseHabit(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	h.sendHabitPickerKeyboard(ctx, msg.Chat.ID, user, "pause", "⏸ ")
}

func (h *Handler) handleResumeHabit(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	h.sendHabitPickerKeyboard(ctx, msg.Chat.ID, user, "resume", "▶️ ")
}

func (h *Handler) sendHabitPickerKeyboard(ctx context.Context, chatID int64, user *domain.User, action, prefix string) {
	habits, err := h.habitUC.ListHabits(ctx, user.ID)
	if err != nil || len(habits) == 0 {
		h.send(chatID, "Нет привычек.")
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
	m := tgbotapi.NewMessage(chatID, "Выбери привычку:")
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send picker keyboard", zap.Error(err))
	}
}

func (h *Handler) cbPauseResume(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, msgID int, arg string, pause bool) {
	habitID, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return
	}
	user, err := h.userUC.GetOrCreateUser(ctx, cq.From.ID, cq.From.UserName, cq.From.FirstName)
	if err != nil {
		h.send(chatID, "Ошибка, попробуй позже.")
		return
	}
	habit, err := h.habitUC.GetHabit(ctx, habitID)
	if err != nil {
		h.send(chatID, "Привычка не найдена.")
		return
	}
	var result string
	if pause {
		if err := h.habitUC.PauseHabit(ctx, user.ID, habitID); err != nil {
			h.logger.Error("PauseHabit", zap.Error(err))
			h.send(chatID, "Ошибка.")
			return
		}
		result = fmt.Sprintf("⏸ «%s» поставлена на паузу.", habit.Name)
	} else {
		if err := h.habitUC.ResumeHabit(ctx, user.ID, habitID); err != nil {
			h.logger.Error("ResumeHabit", zap.Error(err))
			h.send(chatID, "Ошибка.")
			return
		}
		result = fmt.Sprintf("▶️ «%s» возобновлена!", habit.Name)
	}
	h.editMsg(chatID, msgID, result)
}

// ── Snooze ────────────────────────────────────────────────────────────────────

func (h *Handler) cbSnooze(ctx context.Context, chatID int64, msgID int, arg string) {
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
	if err := h.habitUC.SnoozeHabit(ctx, habitID, minutes); err != nil {
		h.logger.Error("SnoozeHabit", zap.Error(err))
		h.send(chatID, "Ошибка снуза.")
		return
	}
	h.editMsg(chatID, msgID, fmt.Sprintf("⏰ Напомню через %d мин.", minutes))
}

// ── Stats ─────────────────────────────────────────────────────────────────────

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

// ── History ───────────────────────────────────────────────────────────────────

func (h *Handler) handleHistory(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	habits, err := h.habitUC.ListHabits(ctx, user.ID)
	if err != nil || len(habits) == 0 {
		h.send(msg.Chat.ID, "Нет привычек для просмотра истории.")
		return
	}
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, habit := range habits {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📅 "+habit.Name, fmt.Sprintf("history:%d", habit.ID)),
		))
	}
	m := tgbotapi.NewMessage(msg.Chat.ID, "Выбери привычку:")
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send history keyboard", zap.Error(err))
	}
}

func (h *Handler) cbHistory(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, arg string) {
	habitID, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return
	}
	user, err := h.userUC.GetOrCreateUser(ctx, cq.From.ID, cq.From.UserName, cq.From.FirstName)
	if err != nil {
		h.send(chatID, i18n.T(i18n.RU, "error.generic"))
		return
	}
	lang := h.lang(user)
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
	text := buildHeatmap(habit.Name, from, now, doneSet, lang)
	h.send(chatID, text)
}

// ── Goal ──────────────────────────────────────────────────────────────────────

func (h *Handler) cbGoalMenu(chatID int64, arg string) {
	habitID, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return
	}
	m := tgbotapi.NewMessage(chatID, "🎯 Выбери цель:")
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("21 день", fmt.Sprintf("set_goal:%d:21", habitID)),
			tgbotapi.NewInlineKeyboardButtonData("30 дней", fmt.Sprintf("set_goal:%d:30", habitID)),
			tgbotapi.NewInlineKeyboardButtonData("66 дней", fmt.Sprintf("set_goal:%d:66", habitID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("100 дней", fmt.Sprintf("set_goal:%d:100", habitID)),
			tgbotapi.NewInlineKeyboardButtonData("Без цели", fmt.Sprintf("set_goal:%d:0", habitID)),
		),
	)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send goal menu", zap.Error(err))
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
	user, err := h.userUC.GetOrCreateUser(ctx, cq.From.ID, cq.From.UserName, cq.From.FirstName)
	if err != nil {
		h.send(chatID, "Ошибка, попробуй позже.")
		return
	}
	if err := h.habitUC.SetGoal(ctx, user.ID, habitID, days); err != nil {
		h.logger.Error("SetGoal", zap.Error(err))
		h.send(chatID, "Ошибка.")
		return
	}
	if days == 0 {
		h.editMsg(chatID, msgID, "🎯 Цель снята.")
	} else {
		h.editMsg(chatID, msgID, fmt.Sprintf("🎯 Цель установлена: %d дней!", days))
	}
}

// ── Today ─────────────────────────────────────────────────────────────────────

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

// ── Achievements ──────────────────────────────────────────────────────────────

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

// ── Timezone ──────────────────────────────────────────────────────────────────

func (h *Handler) handleTimezone(msg *tgbotapi.Message) {
	m := tgbotapi.NewMessage(msg.Chat.ID, "🌍 Выбери свой часовой пояс:")
	var rows [][]tgbotapi.InlineKeyboardButton
	for i := 0; i < len(commonTimezones); i += 2 {
		row := []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(commonTimezones[i].Label, "tz:"+commonTimezones[i].Value),
		}
		if i+1 < len(commonTimezones) {
			row = append(row, tgbotapi.NewInlineKeyboardButtonData(commonTimezones[i+1].Label, "tz:"+commonTimezones[i+1].Value))
		}
		rows = append(rows, row)
	}
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send timezone keyboard", zap.Error(err))
	}
}

func (h *Handler) cbTimezone(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, msgID int, tz string) {
	if _, err := time.LoadLocation(tz); err != nil {
		h.send(chatID, "Неизвестный часовой пояс.")
		return
	}
	user, err := h.userUC.GetOrCreateUser(ctx, cq.From.ID, cq.From.UserName, cq.From.FirstName)
	if err != nil {
		h.send(chatID, "Ошибка, попробуй позже.")
		return
	}
	if err := h.userUC.SetTimezone(ctx, user.ID, tz); err != nil {
		h.logger.Error("SetTimezone", zap.Error(err))
		h.send(chatID, "Ошибка сохранения.")
		return
	}
	h.editMsg(chatID, msgID, fmt.Sprintf("✅ Часовой пояс: %s", tz))
}

func (h *Handler) cbOnboardTimezone(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, msgID int, tz string) {
	if _, err := time.LoadLocation(tz); err != nil {
		return
	}
	user, err := h.userUC.GetOrCreateUser(ctx, cq.From.ID, cq.From.UserName, cq.From.FirstName)
	if err != nil {
		h.send(chatID, i18n.T(i18n.RU, "error.generic"))
		return
	}
	if err := h.userUC.SetTimezone(ctx, user.ID, tz); err != nil {
		h.logger.Error("SetTimezone onboard", zap.Error(err))
		h.send(chatID, i18n.T(h.lang(user), "error.generic"))
		return
	}
	lang := h.lang(user)
	h.editMsg(chatID, msgID, i18n.T(lang, "timezone.set", tz))
	h.setState(cq.From.ID, &convState{Step: stepOnboardHabit})
	m := tgbotapi.NewMessage(chatID, i18n.T(lang, "onboarding.first_habit"))
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "onboarding.add_yes"), "onboard_habit:yes"),
		tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "onboarding.add_later"), "onboard_habit:later"),
	))
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send onboard first habit prompt", zap.Error(err))
	}
}

func (h *Handler) cbOnboardHabit(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, msgID int, arg string) {
	user, err := h.userUC.GetOrCreateUser(ctx, cq.From.ID, cq.From.UserName, cq.From.FirstName)
	if err != nil {
		h.send(chatID, i18n.T(i18n.RU, "error.generic"))
		return
	}
	lang := h.lang(user)
	h.clearState(cq.From.ID)
	h.removeKeyboard(chatID, msgID)
	if arg == "yes" {
		m := tgbotapi.NewMessage(chatID, i18n.T(lang, "habit.choose_template"))
		m.ReplyMarkup = templateKeyboard()
		if _, err := h.api.Send(m); err != nil {
			h.logger.Error("send template keyboard onboard", zap.Error(err))
		}
	} else {
		h.send(chatID, i18n.T(lang, "onboarding.welcome_new", user.FirstName))
	}
}

// ── Keyboards ─────────────────────────────────────────────────────────────────

func templateKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("💧 Пить воду", "template:water"),
			tgbotapi.NewInlineKeyboardButtonData("🏃 Зарядка", "template:exercise"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📚 Читать", "template:read"),
			tgbotapi.NewInlineKeyboardButtonData("🧘 Медитация", "template:meditate"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("😴 Режим сна", "template:sleep"),
			tgbotapi.NewInlineKeyboardButtonData("✏️ Своя привычка", "template:custom"),
		),
	)
}

func (h *Handler) sendIntervalKeyboard(chatID int64, lang i18n.Lang) error {
	m := tgbotapi.NewMessage(chatID, i18n.T(lang, "habit.choose_interval"))
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("30 мин", "interval:30"),
			tgbotapi.NewInlineKeyboardButtonData("1 час", "interval:60"),
			tgbotapi.NewInlineKeyboardButtonData("2 часа", "interval:120"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("3 часа", "interval:180"),
			tgbotapi.NewInlineKeyboardButtonData("Раз в день", "interval:1440"),
		),
	)
	_, err := h.api.Send(m)
	return err
}

func (h *Handler) sendStartHourKeyboard(chatID int64, lang i18n.Lang) error {
	m := tgbotapi.NewMessage(chatID, i18n.T(lang, "habit.choose_start"))
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("5:00", "start_hour:5"),
			tgbotapi.NewInlineKeyboardButtonData("6:00", "start_hour:6"),
			tgbotapi.NewInlineKeyboardButtonData("7:00", "start_hour:7"),
			tgbotapi.NewInlineKeyboardButtonData("8:00", "start_hour:8"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("9:00", "start_hour:9"),
			tgbotapi.NewInlineKeyboardButtonData("10:00", "start_hour:10"),
			tgbotapi.NewInlineKeyboardButtonData("11:00", "start_hour:11"),
			tgbotapi.NewInlineKeyboardButtonData("12:00", "start_hour:12"),
		),
	)
	_, err := h.api.Send(m)
	return err
}

func (h *Handler) sendEndHourKeyboard(chatID int64, lang i18n.Lang, minHour int) error {
	allHours := []int{14, 16, 18, 20, 21, 22, 23}
	var validHours []int
	for _, hr := range allHours {
		if hr > minHour {
			validHours = append(validHours, hr)
		}
	}
	if len(validHours) == 0 {
		h.send(chatID, i18n.T(lang, "error.generic"))
		h.clearState(chatID)
		return nil
	}
	var rows [][]tgbotapi.InlineKeyboardButton
	var row []tgbotapi.InlineKeyboardButton
	for _, hr := range validHours {
		row = append(row, tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("%d:00", hr), fmt.Sprintf("end_hour:%d", hr)))
		if len(row) == 4 {
			rows = append(rows, row)
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	m := tgbotapi.NewMessage(chatID, i18n.T(lang, "habit.choose_end"))
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	_, err := h.api.Send(m)
	return err
}

func (h *Handler) sendGoalKeyboard(chatID int64, lang i18n.Lang) error {
	m := tgbotapi.NewMessage(chatID, i18n.T(lang, "habit.choose_goal"))
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("21 день", "add_goal:21"),
			tgbotapi.NewInlineKeyboardButtonData("30 дней", "add_goal:30"),
			tgbotapi.NewInlineKeyboardButtonData("66 дней", "add_goal:66"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("100 дней", "add_goal:100"),
			tgbotapi.NewInlineKeyboardButtonData("Пропустить", "add_goal:0"),
		),
	)
	_, err := h.api.Send(m)
	return err
}

func (h *Handler) sendEditIntervalKeyboard(chatID, habitID int64) {
	id := strconv.FormatInt(habitID, 10)
	m := tgbotapi.NewMessage(chatID, "Новый интервал напоминаний:")
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("30 мин", "edit_interval:"+id+":30"),
			tgbotapi.NewInlineKeyboardButtonData("1 час", "edit_interval:"+id+":60"),
			tgbotapi.NewInlineKeyboardButtonData("2 часа", "edit_interval:"+id+":120"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("3 часа", "edit_interval:"+id+":180"),
			tgbotapi.NewInlineKeyboardButtonData("Раз в день", "edit_interval:"+id+":1440"),
		),
	)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send edit interval keyboard", zap.Error(err))
	}
}

func (h *Handler) sendEditStartHourKeyboard(chatID, habitID int64) {
	id := strconv.FormatInt(habitID, 10)
	m := tgbotapi.NewMessage(chatID, "Новое начало активного времени:")
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("5:00", "edit_start:"+id+":5"),
			tgbotapi.NewInlineKeyboardButtonData("6:00", "edit_start:"+id+":6"),
			tgbotapi.NewInlineKeyboardButtonData("7:00", "edit_start:"+id+":7"),
			tgbotapi.NewInlineKeyboardButtonData("8:00", "edit_start:"+id+":8"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("9:00", "edit_start:"+id+":9"),
			tgbotapi.NewInlineKeyboardButtonData("10:00", "edit_start:"+id+":10"),
			tgbotapi.NewInlineKeyboardButtonData("11:00", "edit_start:"+id+":11"),
			tgbotapi.NewInlineKeyboardButtonData("12:00", "edit_start:"+id+":12"),
		),
	)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send edit start hour keyboard", zap.Error(err))
	}
}

func (h *Handler) sendEditEndHourKeyboard(chatID, habitID int64, minHour int) {
	id := strconv.FormatInt(habitID, 10)
	allHours := []int{14, 16, 18, 20, 21, 22, 23}
	var rows [][]tgbotapi.InlineKeyboardButton
	var row []tgbotapi.InlineKeyboardButton
	for _, hr := range allHours {
		if hr <= minHour {
			continue
		}
		row = append(row, tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("%d:00", hr),
			fmt.Sprintf("edit_end:%s:%d", id, hr),
		))
		if len(row) == 4 {
			rows = append(rows, row)
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	m := tgbotapi.NewMessage(chatID, "Новый конец активного времени:")
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send edit end hour keyboard", zap.Error(err))
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (h *Handler) resendCurrentStep(chatID int64, lang i18n.Lang, state *convState) error {
	switch state.Step {
	case stepAwaitInterval:
		return h.sendIntervalKeyboard(chatID, lang)
	case stepAwaitStartHour:
		return h.sendStartHourKeyboard(chatID, lang)
	case stepAwaitEndHour:
		return h.sendEndHourKeyboard(chatID, lang, state.StartHour+1)
	case stepAwaitGoal:
		return h.sendGoalKeyboard(chatID, lang)
	default:
		return nil
	}
}

func (h *Handler) send(chatID int64, text string) {
	if _, err := h.api.Send(tgbotapi.NewMessage(chatID, text)); err != nil {
		h.logger.Error("send message", zap.Int64("chat_id", chatID), zap.Error(err))
	}
}

func doneMessage(name string, streak, goalDays int, lang i18n.Lang) string {
	if goalDays > 0 && streak > 0 {
		return i18n.T(lang, "habit.done_goal", name, streak, goalDays)
	}
	if streak > 0 {
		return i18n.T(lang, "habit.done_streak", name, streak)
	}
	return i18n.T(lang, "habit.done_simple")
}

func formatInterval(minutes int) string {
	switch {
	case minutes >= 1440:
		return "раз в день"
	case minutes >= 60:
		hours := minutes / 60
		if hours == 1 {
			return "каждый час"
		}
		return fmt.Sprintf("каждые %d ч", hours)
	default:
		return fmt.Sprintf("каждые %d мин", minutes)
	}
}

func progressBar(done, total int) string {
	const width = 10
	if total == 0 {
		return strings.Repeat("░", width)
	}
	filled := done * width / total
	if filled > width {
		filled = width
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func buildHeatmap(habitName string, from, to time.Time, doneSet map[string]bool, lang string) string {
	weekdays := []string{"Mo", "Tu", "We", "Th", "Fr", "Sa", "Su"}
	start := from
	for start.Weekday() != time.Monday {
		start = start.AddDate(0, 0, -1)
	}
	grid := [7][4]string{}
	for row := 0; row < 7; row++ {
		for col := 0; col < 4; col++ {
			day := start.AddDate(0, 0, col*7+row)
			if day.After(to) {
				grid[row][col] = " "
				continue
			}
			if doneSet[day.Format("2006-01-02")] {
				grid[row][col] = "■"
			} else {
				grid[row][col] = "□"
			}
		}
	}
	var sb strings.Builder
	sb.WriteString(i18n.T(lang, "history.header", habitName))
	sb.WriteString("\n")
	for row := 0; row < 7; row++ {
		sb.WriteString(weekdays[row] + " ")
		for col := 0; col < 4; col++ {
			sb.WriteString(grid[row][col] + " ")
		}
		sb.WriteString("\n")
	}
	sb.WriteString(i18n.T(lang, "history.legend"))
	return sb.String()
}
