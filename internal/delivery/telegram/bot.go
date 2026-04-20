package telegram

import (
	"context"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"

	"github.com/saidakmal/habbit-tracker-bot/internal/usecase"
)

const (
	pollingLockKey           = "bot:polling:lock"
	pollingLockTTL           = 30 * time.Second
	pollingLockRenewInterval = 10 * time.Second
)

type Bot struct {
	api      *tgbotapi.BotAPI
	handler  *Handler
	logger   *zap.Logger
	cache    usecase.Cache
	stopLock chan struct{}
	stopOnce sync.Once
}

func NewBot(api *tgbotapi.BotAPI, handler *Handler, logger *zap.Logger, cache usecase.Cache) *Bot {
	return &Bot{
		api:      api,
		handler:  handler,
		logger:   logger,
		cache:    cache,
		stopLock: make(chan struct{}),
	}
}

func (b *Bot) Start() {
	ctx := context.Background()

	// Acquire distributed lock so only one instance polls Telegram at a time.
	// Two instances with the same token split updates — users see intermittent responses.
	acquired, err := b.cache.SetNX(ctx, pollingLockKey, "1", pollingLockTTL)
	if err != nil {
		b.logger.Warn("could not check polling lock (Redis unavailable); proceeding without lock", zap.Error(err))
	} else if !acquired {
		b.logger.Fatal("another bot instance is already running; this instance will not start to prevent update conflicts")
	}

	if err == nil {
		go b.renewLock(ctx)
	}

	commands := []tgbotapi.BotCommand{
		{Command: "start", Description: "Start / Начать"},
		{Command: "today", Description: "Today's habits / Привычки на сегодня"},
		{Command: "stats", Description: "Statistics / Статистика"},
		{Command: "mood", Description: "Log mood / Настроение дня"},
		{Command: "achievements", Description: "Achievements / Достижения"},
		{Command: "settings", Description: "Settings / Настройки"},
		{Command: "cancel", Description: "Cancel current action / Отменить"},
	}
	if _, err := b.api.Request(tgbotapi.NewSetMyCommands(commands...)); err != nil {
		b.logger.Warn("failed to set commands menu", zap.Error(err))
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	b.logger.Info("polling started", zap.String("bot", b.api.Self.UserName))
	for update := range b.api.GetUpdatesChan(u) {
		update := update
		go func() {
			defer func() {
				if r := recover(); r != nil {
					b.logger.Error("panic in HandleUpdate", zap.Any("recover", r))
				}
			}()
			b.handler.HandleUpdate(update)
		}()
	}
}

func (b *Bot) renewLock(ctx context.Context) {
	ticker := time.NewTicker(pollingLockRenewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-b.stopLock:
			return
		case <-ticker.C:
			if err := b.cache.Set(ctx, pollingLockKey, "1", pollingLockTTL); err != nil {
				b.logger.Error("failed to renew polling lock", zap.Error(err))
			}
		}
	}
}

func (b *Bot) Stop() error {
	b.stopOnce.Do(func() { close(b.stopLock) })
	b.api.StopReceivingUpdates()
	_ = b.cache.Delete(context.Background(), pollingLockKey)
	return nil
}
