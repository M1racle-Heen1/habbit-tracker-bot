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
		{Command: "list_habits", Description: "Список привычек"},
		{Command: "add_habit", Description: "Добавить привычку"},
		{Command: "done", Description: "Отметить выполнение"},
		{Command: "delete_habit", Description: "Удалить привычку"},
		{Command: "health", Description: "Статус бота"},
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
