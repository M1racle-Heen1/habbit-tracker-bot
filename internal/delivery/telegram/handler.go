package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"

	"github.com/saidakmal/habbit-tracker-bot/internal/domain"
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
)

type convState struct {
	Step            step      `json:"step"`
	HabitName       string    `json:"habit_name"`
	IntervalMinutes int       `json:"interval_minutes"`
	StartHour       int       `json:"start_hour"`
	EndHour         int       `json:"end_hour"`
	EditHabitID     int64     `json:"edit_habit_id"`
	Lang            i18n.Lang `json:"lang"`
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

// ── State helpers ─────────────────────────────────────────────────────────────

func stateKey(telegramID int64) string { return fmt.Sprintf("state:%d", telegramID) }

func (h *Handler) getState(telegramID int64) *convState {
	data, err := h.cache.Get(context.Background(), stateKey(telegramID))
	if err != nil || data == "" {
		return nil
	}
	var s convState
	if err := json.Unmarshal([]byte(data), &s); err != nil {
		h.logger.Error("getState: unmarshal failed", zap.Int64("telegramID", telegramID), zap.Error(err))
		return nil
	}
	return &s
}

func (h *Handler) setState(telegramID int64, s *convState) {
	data, _ := json.Marshal(s)
	if err := h.cache.Set(context.Background(), stateKey(telegramID), string(data), stateTTL); err != nil {
		h.logger.Error("setState: redis set failed", zap.Int64("telegramID", telegramID), zap.Error(err))
	}
}

func (h *Handler) clearState(telegramID int64) {
	_ = h.cache.Delete(context.Background(), stateKey(telegramID))
}

// ── Message helpers ───────────────────────────────────────────────────────────

func (h *Handler) removeKeyboard(chatID int64, messageID int) {
	edit := tgbotapi.NewEditMessageReplyMarkup(chatID, messageID, tgbotapi.InlineKeyboardMarkup{})
	if _, err := h.api.Request(edit); err != nil {
		h.logger.Warn("remove keyboard", zap.Error(err))
	}
}

func (h *Handler) editMsg(chatID int64, messageID int, text string) {
	edit := tgbotapi.NewEditMessageText(chatID, messageID, text)
	if _, err := h.api.Request(edit); err != nil {
		h.logger.Warn("edit message text", zap.Error(err))
	}
}

func (h *Handler) send(chatID int64, text string) {
	if _, err := h.api.Send(tgbotapi.NewMessage(chatID, text)); err != nil {
		h.logger.Error("send message", zap.Int64("chat_id", chatID), zap.Error(err))
	}
}

// getUserFromCallback loads the user from a callback query.
// On error it sends an error message and returns a non-nil error.
func (h *Handler) getUserFromCallback(ctx context.Context, cq *tgbotapi.CallbackQuery) (*domain.User, i18n.Lang, error) {
	user, err := h.userUC.GetOrCreateUser(ctx, cq.From.ID, cq.From.UserName, cq.From.FirstName)
	if err != nil {
		h.logger.Error("GetOrCreateUser", zap.Error(err))
		h.send(cq.Message.Chat.ID, i18n.T(i18n.RU, "error.generic"))
		return nil, i18n.RU, err
	}
	return user, h.lang(user), nil
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
		h.send(msg.Chat.ID, i18n.T(i18n.RU, "error.generic"))
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
	switch msg.Command() {
	case "cancel":
		h.clearState(msg.From.ID)
		h.send(msg.Chat.ID, i18n.T(h.lang(user), "action.cancelled"))
		return
	}

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
		h.handleTimezone(msg, user)
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
		h.cbSnooze(ctx, cq, chatID, msgID, arg)
	case "pause":
		h.cbPauseResume(ctx, cq, chatID, msgID, arg, true)
	case "resume":
		h.cbPauseResume(ctx, cq, chatID, msgID, arg, false)
	case "lang":
		h.cbLanguage(ctx, cq, chatID, msgID, arg)
	case "tz":
		h.cbTimezone(ctx, cq, chatID, msgID, arg)
	case "history":
		h.cbHistory(ctx, cq, chatID, arg)
	case "edit":
		h.cbEditMenu(ctx, cq, chatID, arg)
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
		h.cbGoalMenu(ctx, cq, chatID, arg)
	}
}
