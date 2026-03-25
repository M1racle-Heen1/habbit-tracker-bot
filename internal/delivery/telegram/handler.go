package telegram

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"

	"github.com/saidakmal/habbit-tracker-bot/internal/domain"
	"github.com/saidakmal/habbit-tracker-bot/internal/usecase"
)

const handlerTimeout = 10 * time.Second

type step int

const (
	stepIdle step = iota
	stepAwaitName
	stepAwaitInterval
	stepAwaitStartHour
	stepAwaitEndHour
)

type convState struct {
	step            step
	habitName       string
	intervalMinutes int
	startHour       int
}

type Handler struct {
	habitUC *usecase.HabitUsecase
	userUC  *usecase.UserUsecase
	api     *tgbotapi.BotAPI
	logger  *zap.Logger
	states  map[int64]*convState
	mu      sync.Mutex
}

func NewHandler(habitUC *usecase.HabitUsecase, userUC *usecase.UserUsecase, api *tgbotapi.BotAPI, logger *zap.Logger) *Handler {
	return &Handler{
		habitUC: habitUC,
		userUC:  userUC,
		api:     api,
		logger:  logger,
		states:  make(map[int64]*convState),
	}
}

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

func (h *Handler) handleCommand(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	switch msg.Command() {
	case "start":
		h.handleStart(msg, user)
	case "list_habits", "habits":
		h.handleListHabits(ctx, msg, user)
	case "add_habit", "add":
		h.startAddHabit(msg)
	case "done":
		h.handleDone(ctx, msg, user)
	case "delete_habit":
		h.handleDeleteHabit(ctx, msg, user)
	case "health":
		h.send(msg.Chat.ID, "OK")
	}
}

func (h *Handler) handleText(_ context.Context, msg *tgbotapi.Message, _ *domain.User) {
	h.mu.Lock()
	state, ok := h.states[msg.From.ID]
	h.mu.Unlock()

	if !ok || state.step != stepAwaitName {
		return
	}

	name := strings.TrimSpace(msg.Text)
	if name == "" {
		h.send(msg.Chat.ID, "Название не может быть пустым. Введи название:")
		return
	}

	h.mu.Lock()
	state.habitName = name
	state.step = stepAwaitInterval
	h.mu.Unlock()

	h.sendIntervalKeyboard(msg.Chat.ID)
}

func (h *Handler) handleCallback(cq *tgbotapi.CallbackQuery) {
	ctx, cancel := context.WithTimeout(context.Background(), handlerTimeout)
	defer cancel()

	if _, err := h.api.Request(tgbotapi.NewCallback(cq.ID, "")); err != nil {
		h.logger.Warn("answer callback", zap.Error(err))
	}

	chatID := cq.Message.Chat.ID
	parts := strings.SplitN(cq.Data, ":", 2)
	if len(parts) != 2 {
		return
	}
	action, arg := parts[0], parts[1]

	switch action {
	case "interval":
		minutes, err := strconv.Atoi(arg)
		if err != nil {
			return
		}
		h.mu.Lock()
		state, ok := h.states[cq.From.ID]
		if ok && state.step == stepAwaitInterval {
			state.intervalMinutes = minutes
			state.step = stepAwaitStartHour
		}
		h.mu.Unlock()
		if ok {
			h.sendStartHourKeyboard(chatID)
		}

	case "start_hour":
		hour, err := strconv.Atoi(arg)
		if err != nil {
			return
		}
		h.mu.Lock()
		state, ok := h.states[cq.From.ID]
		if ok && state.step == stepAwaitStartHour {
			state.startHour = hour
			state.step = stepAwaitEndHour
		}
		h.mu.Unlock()
		if ok {
			h.sendEndHourKeyboard(chatID)
		}

	case "end_hour":
		endHour, err := strconv.Atoi(arg)
		if err != nil {
			return
		}
		h.mu.Lock()
		state, ok := h.states[cq.From.ID]
		if !ok || state.step != stepAwaitEndHour {
			h.mu.Unlock()
			return
		}
		s := *state
		delete(h.states, cq.From.ID)
		h.mu.Unlock()

		user, err := h.userUC.GetOrCreateUser(ctx, cq.From.ID, cq.From.UserName, cq.From.FirstName)
		if err != nil {
			h.logger.Error("GetOrCreateUser", zap.Error(err))
			h.send(chatID, "Произошла ошибка, попробуй позже.")
			return
		}
		habit, err := h.habitUC.CreateHabit(ctx, user.ID, s.habitName, s.intervalMinutes, s.startHour, endHour)
		if err != nil {
			h.logger.Error("CreateHabit", zap.Error(err))
			h.send(chatID, "Не удалось создать привычку, попробуй позже.")
			return
		}
		h.send(chatID, fmt.Sprintf(
			"✅ Привычка «%s» создана!\nНапоминания каждые %d мин, %d:00–%d:00",
			habit.Name, habit.IntervalMinutes, habit.StartHour, habit.EndHour,
		))

	case "done":
		habitID, err := strconv.ParseInt(arg, 10, 64)
		if err != nil {
			return
		}
		user, err := h.userUC.GetOrCreateUser(ctx, cq.From.ID, cq.From.UserName, cq.From.FirstName)
		if err != nil {
			h.logger.Error("GetOrCreateUser", zap.Error(err))
			return
		}
		if err := h.habitUC.MarkDone(ctx, user.ID, habitID); err != nil {
			if errors.Is(err, domain.ErrAlreadyDone) {
				h.send(chatID, "Привычка уже выполнена сегодня ✓")
				return
			}
			h.logger.Error("MarkDone", zap.Error(err))
			h.send(chatID, "Ошибка, попробуй позже.")
			return
		}
		habit, err := h.habitUC.GetHabit(ctx, habitID)
		if err == nil {
			h.send(chatID, fmt.Sprintf("✅ «%s» выполнена! Стрик: %d дней 🔥", habit.Name, habit.Streak))
		} else {
			h.send(chatID, "✅ Выполнено!")
		}

	case "delete":
		habitID, err := strconv.ParseInt(arg, 10, 64)
		if err != nil {
			return
		}
		user, err := h.userUC.GetOrCreateUser(ctx, cq.From.ID, cq.From.UserName, cq.From.FirstName)
		if err != nil {
			h.logger.Error("GetOrCreateUser", zap.Error(err))
			return
		}
		if err := h.habitUC.DeleteHabit(ctx, user.ID, habitID); err != nil {
			h.logger.Error("DeleteHabit", zap.Error(err))
			h.send(chatID, "Ошибка при удалении.")
			return
		}
		h.send(chatID, "🗑 Привычка удалена.")
	}
}

func (h *Handler) handleStart(msg *tgbotapi.Message, user *domain.User) {
	h.send(msg.Chat.ID, fmt.Sprintf(
		"Привет, %s! 👋\n\nКоманды:\n/list_habits — список привычек\n/add_habit — добавить привычку\n/done — отметить выполнение\n/delete_habit — удалить привычку",
		user.FirstName,
	))
}

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
	for _, habit := range habits {
		done := "○"
		if usecase.IsDoneToday(habit, now) {
			done = "✅"
		}
		sb.WriteString(fmt.Sprintf("%s %s (стрик: %d)\n", done, habit.Name, habit.Streak))
		sb.WriteString(fmt.Sprintf("   каждые %d мин, %d:00–%d:00\n\n", habit.IntervalMinutes, habit.StartHour, habit.EndHour))
	}
	h.send(msg.Chat.ID, sb.String())
}

func (h *Handler) startAddHabit(msg *tgbotapi.Message) {
	h.mu.Lock()
	h.states[msg.From.ID] = &convState{step: stepAwaitName}
	h.mu.Unlock()
	h.send(msg.Chat.ID, "Введи название привычки:")
}

func (h *Handler) handleDone(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	habits, err := h.habitUC.ListHabits(ctx, user.ID)
	if err != nil || len(habits) == 0 {
		h.send(msg.Chat.ID, "Нет привычек для отметки.")
		return
	}
	now := time.Now()
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, habit := range habits {
		label := habit.Name
		if usecase.IsDoneToday(habit, now) {
			label = "✅ " + label
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, fmt.Sprintf("done:%d", habit.ID)),
		))
	}
	m := tgbotapi.NewMessage(msg.Chat.ID, "Выбери привычку:")
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send done keyboard", zap.Error(err))
	}
}

func (h *Handler) handleDeleteHabit(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	habits, err := h.habitUC.ListHabits(ctx, user.ID)
	if err != nil || len(habits) == 0 {
		h.send(msg.Chat.ID, "Нет привычек для удаления.")
		return
	}
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, habit := range habits {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🗑 "+habit.Name, fmt.Sprintf("delete:%d", habit.ID)),
		))
	}
	m := tgbotapi.NewMessage(msg.Chat.ID, "Выбери привычку для удаления:")
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send delete keyboard", zap.Error(err))
	}
}

func (h *Handler) sendIntervalKeyboard(chatID int64) {
	m := tgbotapi.NewMessage(chatID, "Как часто напоминать?")
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("30 мин", "interval:30"),
			tgbotapi.NewInlineKeyboardButtonData("1 час", "interval:60"),
			tgbotapi.NewInlineKeyboardButtonData("2 часа", "interval:120"),
			tgbotapi.NewInlineKeyboardButtonData("3 часа", "interval:180"),
		),
	)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send interval keyboard", zap.Error(err))
	}
}

func (h *Handler) sendStartHourKeyboard(chatID int64) {
	m := tgbotapi.NewMessage(chatID, "Начало активного времени:")
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("7:00", "start_hour:7"),
			tgbotapi.NewInlineKeyboardButtonData("8:00", "start_hour:8"),
			tgbotapi.NewInlineKeyboardButtonData("9:00", "start_hour:9"),
			tgbotapi.NewInlineKeyboardButtonData("10:00", "start_hour:10"),
		),
	)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send start hour keyboard", zap.Error(err))
	}
}

func (h *Handler) sendEndHourKeyboard(chatID int64) {
	m := tgbotapi.NewMessage(chatID, "Конец активного времени:")
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("20:00", "end_hour:20"),
			tgbotapi.NewInlineKeyboardButtonData("21:00", "end_hour:21"),
			tgbotapi.NewInlineKeyboardButtonData("22:00", "end_hour:22"),
			tgbotapi.NewInlineKeyboardButtonData("23:00", "end_hour:23"),
		),
	)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send end hour keyboard", zap.Error(err))
	}
}

func (h *Handler) send(chatID int64, text string) {
	if _, err := h.api.Send(tgbotapi.NewMessage(chatID, text)); err != nil {
		h.logger.Error("send message", zap.Int64("chat_id", chatID), zap.Error(err))
	}
}
