package gamification

import (
	"context"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"

	"github.com/saidakmal/habbit-tracker-bot/internal/domain"
	"github.com/saidakmal/habbit-tracker-bot/internal/i18n"
)

type UserProvider interface {
	GetByID(ctx context.Context, userID int64) (*domain.User, error)
	AddXP(ctx context.Context, userID int64, xp int) (int, int, error)
	UpdateStreakShields(ctx context.Context, userID int64, shields int) error
	AddAchievement(ctx context.Context, userID int64, code string) error
	HasAchievement(ctx context.Context, userID int64, code string) (bool, error)
}

type ActivityCounter interface {
	CountAllByUser(ctx context.Context, userID int64) (int, error)
}

func Run(ctx context.Context, user *domain.User, streak int, actCounter ActivityCounter, api *tgbotapi.BotAPI, logger *zap.Logger, up UserProvider) {
	lang := user.Language
	if lang == "" {
		lang = i18n.RU
	}

	xp := XPForCompletion(streak)
	newXP, newLevel, err := up.AddXP(ctx, user.ID, xp)
	if err != nil {
		logger.Warn("gamification AddXP", zap.Error(err))
		return
	}
	if newLevel > user.Level {
		sendMsg(api, user.TelegramID, i18n.T(lang, "levelup.message", newLevel))
	}

	checks := []struct {
		code      string
		triggered bool
	}{
		{AchFirstDone, func() bool {
			c, err := actCounter.CountAllByUser(ctx, user.ID)
			return err == nil && c == 1
		}()},
		{AchStreak7, streak >= 7},
		{AchStreak30, streak >= 30},
		{AchStreak100, streak >= 100},
	}
	for _, check := range checks {
		if !check.triggered {
			continue
		}
		has, err := up.HasAchievement(ctx, user.ID, check.code)
		if err != nil || has {
			continue
		}
		def, ok := GetDef(check.code)
		if !ok {
			continue
		}
		if err := up.AddAchievement(ctx, user.ID, check.code); err != nil {
			logger.Warn("gamification AddAchievement", zap.Error(err))
			continue
		}
		sendMsg(api, user.TelegramID, i18n.T(lang, "achievement.unlocked", DisplayName(check.code, lang), RewardText(def, lang)))
		if def.ShieldBonus > 0 {
			_ = up.UpdateStreakShields(ctx, user.ID, user.StreakShields+def.ShieldBonus)
		}
		if def.XPBonus > 0 {
			newXP, newLevel, _ = up.AddXP(ctx, user.ID, def.XPBonus)
			if newLevel > user.Level {
				sendMsg(api, user.TelegramID, i18n.T(lang, "levelup.message", newLevel))
			}
		}
		_ = newXP
	}
}

func sendMsg(api *tgbotapi.BotAPI, telegramID int64, text string) {
	_, _ = api.Send(tgbotapi.NewMessage(telegramID, text))
}

func EarlyBirdHour() int { return 9 }
