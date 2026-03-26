package app

import (
	"context"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	redisc "github.com/redis/go-redis/v9"
	"go.uber.org/fx"
	"go.uber.org/zap"

	"github.com/saidakmal/habbit-tracker-bot/internal/delivery/telegram"
	"github.com/saidakmal/habbit-tracker-bot/internal/gamification"
	"github.com/saidakmal/habbit-tracker-bot/internal/infrastructure/config"
	"github.com/saidakmal/habbit-tracker-bot/internal/infrastructure/logger"
	postgresrepo "github.com/saidakmal/habbit-tracker-bot/internal/repository/postgres"
	redisrepo "github.com/saidakmal/habbit-tracker-bot/internal/repository/redis"
	"github.com/saidakmal/habbit-tracker-bot/internal/scheduler"
	"github.com/saidakmal/habbit-tracker-bot/internal/usecase"
)

func New() *fx.App {
	return fx.New(
		fx.Provide(
			config.New,
			newLogger,
			newDB,
			newRedisClient,
			newTelegramAPI,
			fx.Annotate(postgresrepo.NewUserRepository, fx.As(new(usecase.UserRepository))),
			fx.Annotate(postgresrepo.NewHabitRepository, fx.As(new(usecase.HabitRepository))),
			fx.Annotate(postgresrepo.NewActivityRepository, fx.As(new(usecase.ActivityRepository))),
			fx.Annotate(redisrepo.NewCache, fx.As(new(usecase.Cache))),
			usecase.NewUserUsecase,
			usecase.NewHabitUsecase,
			telegram.NewHandler,
			telegram.NewBot,
			newLocation,
			scheduler.New,
		),
		fx.Invoke(registerHooks),
	fx.Invoke(func(habitUC *usecase.HabitUsecase, userUC *usecase.UserUsecase, activityRepo usecase.ActivityRepository, api *tgbotapi.BotAPI, log *zap.Logger) {
		habitUC.SetGamificationNotifier(func(ctx context.Context, userID int64, _ int64, streak int) {
			user, err := userUC.GetByID(ctx, userID)
			if err != nil {
				log.Warn("gamification GetByID", zap.Error(err))
				return
			}
			gamification.Run(ctx, user, streak, activityRepo, api, log, userUC)
		})
	}),
	)
}

func newLogger() (*zap.Logger, error) {
	return logger.New()
}

func newDB(lc fx.Lifecycle, cfg *config.Config) (*pgxpool.Pool, error) {
	pool, err := postgresrepo.NewPool(context.Background(), cfg.DBDSN)
	if err != nil {
		return nil, err
	}
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			pool.Close()
			return nil
		},
	})
	return pool, nil
}

func newRedisClient(cfg *config.Config) *redisc.Client {
	return redisrepo.NewClient(cfg.RedisAddr)
}

func newTelegramAPI(cfg *config.Config) (*tgbotapi.BotAPI, error) {
	return tgbotapi.NewBotAPI(cfg.TelegramToken)
}

func newLocation(cfg *config.Config) (*time.Location, error) {
	return time.LoadLocation(cfg.Timezone)
}

func registerHooks(lc fx.Lifecycle, bot *telegram.Bot, sched *scheduler.Scheduler, log *zap.Logger) {
	ctx, cancel := context.WithCancel(context.Background())
	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			log.Info("bot starting")
			go bot.Start()
			go sched.Start(ctx)
			return nil
		},
		OnStop: func(_ context.Context) error {
			log.Info("bot stopping")
			cancel()
			return bot.Stop()
		},
	})
}
