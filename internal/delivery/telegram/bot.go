package telegram

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"
)

type Bot struct {
	api     *tgbotapi.BotAPI
	handler *Handler
	logger  *zap.Logger
}

func NewBot(api *tgbotapi.BotAPI, handler *Handler, logger *zap.Logger) *Bot {
	return &Bot{api: api, handler: handler, logger: logger}
}

func (b *Bot) Start() {
	commands := []tgbotapi.BotCommand{
		{Command: "list_habits", Description: "Habits with progress / Список привычек"},
		{Command: "today", Description: "Today's habits / Сегодня"},
		{Command: "done", Description: "Mark as done / Отметить выполнение"},
		{Command: "add_habit", Description: "Add habit / Добавить привычку"},
		{Command: "achievements", Description: "Achievements / Достижения"},
		{Command: "stats", Description: "Statistics / Статистика"},
		{Command: "history", Description: "History / История"},
		{Command: "edit_habit", Description: "Edit habit / Редактировать"},
		{Command: "pause_habit", Description: "Pause / Пауза"},
		{Command: "resume_habit", Description: "Resume / Возобновить"},
		{Command: "language", Description: "Language / Язык / Тіл"},
		{Command: "timezone", Description: "Timezone / Часовой пояс"},
		{Command: "delete_habit", Description: "Delete habit / Удалить"},
		{Command: "cancel", Description: "Cancel / Отмена"},
	}
	if _, err := b.api.Request(tgbotapi.NewSetMyCommands(commands...)); err != nil {
		b.logger.Warn("failed to set commands menu", zap.Error(err))
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	b.logger.Info("polling started", zap.String("bot", b.api.Self.UserName))
	for update := range b.api.GetUpdatesChan(u) {
		update := update
		go b.handler.HandleUpdate(update)
	}
}

func (b *Bot) Stop() error {
	b.api.StopReceivingUpdates()
	return nil
}
