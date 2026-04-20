package scheduler

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"

	"github.com/saidakmal/habbit-tracker-bot/internal/domain"
	"github.com/saidakmal/habbit-tracker-bot/internal/format"
	"github.com/saidakmal/habbit-tracker-bot/internal/i18n"
	"github.com/saidakmal/habbit-tracker-bot/internal/usecase"
)

const midnightLockKey = "scheduler:midnight:lock"

type Scheduler struct {
	habitUC  *usecase.HabitUsecase
	userUC   *usecase.UserUsecase
	moodUC   *usecase.MoodUsecase
	api      *tgbotapi.BotAPI
	logger   *zap.Logger
	location *time.Location
	cache    usecase.Cache
	locCache map[string]*time.Location
	locMu    sync.RWMutex
}

func New(habitUC *usecase.HabitUsecase, userUC *usecase.UserUsecase, moodUC *usecase.MoodUsecase, api *tgbotapi.BotAPI, logger *zap.Logger, loc *time.Location, cache usecase.Cache) *Scheduler {
	return &Scheduler{
		habitUC:  habitUC,
		userUC:   userUC,
		moodUC:   moodUC,
		api:      api,
		logger:   logger,
		location: loc,
		cache:    cache,
		locCache: make(map[string]*time.Location),
	}
}

// loadLocation returns a cached *time.Location for tz, falling back to the default location on error.
func (s *Scheduler) loadLocation(tz string) *time.Location {
	s.locMu.RLock()
	if loc, ok := s.locCache[tz]; ok {
		s.locMu.RUnlock()
		return loc
	}
	s.locMu.RUnlock()

	loc, err := time.LoadLocation(tz)
	if err != nil {
		return s.location
	}

	s.locMu.Lock()
	s.locCache[tz] = loc
	s.locMu.Unlock()
	return loc
}

func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			s.tick(ctx, t)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context, now time.Time) {
	habits, err := s.habitUC.ListAllForScheduler(ctx)
	if err != nil {
		s.logger.Error("ListAllForScheduler", zap.Error(err))
		return
	}

	// Midnight check in default timezone — reset streaks and notify.
	// Redis lock prevents double-execution on restart at midnight.
	defaultNow := now.In(s.location)
	if defaultNow.Hour() == 0 && defaultNow.Minute() == 0 {
		if ok, err := s.cache.SetNX(ctx, midnightLockKey, "1", 2*time.Hour); err == nil && ok {
			s.resetStreaksAndNotify(ctx)
		}
	}

	// Group habits by user
	type userGroup struct {
		telegramID       int64
		firstName        string
		userTimezone     string
		userLang         string
		userID           int64
		eveningRecapHour int
		habits           []*domain.HabitWithTelegramID
	}
	groups := make(map[int64]*userGroup)
	for _, hw := range habits {
		if _, ok := groups[hw.UserID]; !ok {
			groups[hw.UserID] = &userGroup{
				telegramID:       hw.TelegramID,
				firstName:        hw.UserFirstName,
				userTimezone:     hw.UserTimezone,
				userLang:         hw.UserLanguage,
				userID:           hw.UserID,
				eveningRecapHour: hw.EveningRecapHour,
			}
		}
		groups[hw.UserID].habits = append(groups[hw.UserID].habits, hw)
	}

	for _, g := range groups {
		userNow := now.In(s.loadLocation(g.userTimezone))

		lang := g.userLang
		if lang == "" {
			lang = i18n.RU
		}

		// Morning digest at 8:00
		if userNow.Hour() == 8 && userNow.Minute() < 5 {
			s.maybeSendMorningDigest(ctx, g.telegramID, g.firstName, g.habits, userNow, lang)
		}

		// Weekly digest on Sundays at 20:00
		if userNow.Weekday() == time.Sunday && userNow.Hour() == 20 && userNow.Minute() < 5 {
			s.maybeSendWeeklyDigest(ctx, g.telegramID, g.userID, g.habits, userNow, lang)
		}

		// Streak-at-risk alert at 20:00
		if userNow.Hour() == 20 && userNow.Minute() < 5 {
			s.maybeSendStreakRisk(ctx, g.telegramID, lang, g.habits, userNow)
		}

		// Evening recap at user-configured hour
		if g.eveningRecapHour > 0 && userNow.Hour() == g.eveningRecapHour && userNow.Minute() < 5 {
			s.maybeSendEveningRecap(ctx, g.telegramID, g.userID, lang, g.habits, userNow)
		}

		// Per-habit reminders + timer expiry
		for _, hw := range g.habits {
			if hw.IsPaused {
				continue
			}

			// Check if a timer has expired for this habit
			timerKey := fmt.Sprintf("timer:%d:%d", hw.ID, hw.UserID)
			if val, err := s.cache.Get(ctx, timerKey); err == nil {
				expiry, _ := strconv.ParseInt(val, 10, 64)
				if time.Now().Unix() >= expiry {
					_ = s.cache.Delete(ctx, timerKey)
					if !usecase.IsDoneToday(&hw.Habit, userNow) {
						if err := s.habitUC.MarkDone(ctx, hw.UserID, hw.ID); err == nil {
							s.api.Send(tgbotapi.NewMessage(hw.TelegramID, i18n.T(lang, "timer.done", hw.Name))) //nolint:errcheck
						}
					}
				}
			}

			if hw.SnoozeUntil != nil && now.Before(*hw.SnoozeUntil) {
				continue
			}

			adaptiveHour, hasAdaptive, err := s.habitUC.GetActivityAverageHour(ctx, hw.ID)
			if err != nil {
				adaptiveHour = hw.StartHour
				hasAdaptive = false
			}
			effectiveStartHour := hw.StartHour
			if hasAdaptive && adaptiveHour > hw.StartHour && adaptiveHour < hw.EndHour {
				effectiveStartHour = adaptiveHour - 1
				if effectiveStartHour < hw.StartHour {
					effectiveStartHour = hw.StartHour
				}
			}
			if !usecase.IsInActiveHoursFrom(&hw.Habit, userNow, effectiveStartHour) {
				continue
			}

			if usecase.IsDoneToday(&hw.Habit, userNow) {
				continue
			}
			if !usecase.IsFinalReminder(&hw.Habit, userNow) && !usecase.ShouldSendInterval(&hw.Habit, userNow) {
				continue
			}
			s.sendReminder(ctx, hw.TelegramID, &hw.Habit, lang)
		}
	}
}

func (s *Scheduler) sendReminder(ctx context.Context, telegramID int64, h *domain.Habit, lang string) {
	streakText := ""
	if h.Streak > 0 {
		streakText = i18n.T(lang, "reminder.streak", h.Streak)
	}
	text := i18n.T(lang, "reminder.text", h.Name) + streakText
	if h.Motivation != "" {
		text += "\n" + i18n.T(lang, "reminder.motivation", h.Motivation)
	}

	msg := tgbotapi.NewMessage(telegramID, text)
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "reminder.done_button"), fmt.Sprintf("done:%d", h.ID)),
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "timer.btn"), fmt.Sprintf("timer_start:%d", h.ID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "snooze.30min"), fmt.Sprintf("snooze:%d:30", h.ID)),
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "snooze.1hr"), fmt.Sprintf("snooze:%d:60", h.ID)),
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "snooze.2hr"), fmt.Sprintf("snooze:%d:120", h.ID)),
		),
	)
	if _, err := s.api.Send(msg); err != nil {
		s.logger.Error("send reminder", zap.Int64("telegram_id", telegramID), zap.Error(err))
		return
	}
	if err := s.habitUC.UpdateNotified(ctx, h.ID); err != nil {
		s.logger.Error("UpdateNotified", zap.Int64("habit_id", h.ID), zap.Error(err))
	}
}

func (s *Scheduler) maybeSendMorningDigest(ctx context.Context, telegramID int64, firstName string, habits []*domain.HabitWithTelegramID, now time.Time, lang string) {
	key := fmt.Sprintf("morning:%d:%s", telegramID, now.Format("2006-01-02"))
	if _, err := s.cache.Get(ctx, key); err == nil {
		return // already sent today
	}

	var sb strings.Builder
	sb.WriteString(i18n.T(lang, "morning.header", firstName))

	var doneButtons [][]tgbotapi.InlineKeyboardButton
	for _, hw := range habits {
		if hw.IsPaused {
			continue
		}
		if usecase.IsDoneToday(&hw.Habit, now) {
			sb.WriteString(fmt.Sprintf("✅ %s\n", hw.Name))
		} else {
			streakStr := ""
			if hw.Streak > 0 {
				streakStr = " " + i18n.T(lang, "reminder.streak", hw.Streak)
			}
			sb.WriteString(fmt.Sprintf("○ %s%s\n", hw.Name, streakStr))
			doneButtons = append(doneButtons, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("✅ "+hw.Name, fmt.Sprintf("done:%d", hw.ID)),
			))
		}
	}

	msg := tgbotapi.NewMessage(telegramID, sb.String())
	if len(doneButtons) > 0 {
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(doneButtons...)
	}
	if _, err := s.api.Send(msg); err != nil {
		s.logger.Error("send morning digest", zap.Int64("telegram_id", telegramID), zap.Error(err))
		return
	}
	if err := s.cache.Set(ctx, key, "1", 25*time.Hour); err != nil {
		s.logger.Warn("morning digest cache set", zap.Error(err))
	}
}

func (s *Scheduler) maybeSendWeeklyDigest(ctx context.Context, telegramID int64, userID int64, habits []*domain.HabitWithTelegramID, now time.Time, lang string) {
	year, week := now.ISOWeek()
	key := fmt.Sprintf("weekly:%d:%d:%d", telegramID, year, week)
	if _, err := s.cache.Get(ctx, key); err == nil {
		return // already sent this week
	}

	stats, err := s.habitUC.GetStats(ctx, userID, 7)
	if err != nil {
		s.logger.Error("weekly digest GetStats", zap.Int64("user_id", userID), zap.Error(err))
		return
	}
	statsByID := make(map[int64]*usecase.HabitStats, len(stats))
	for _, st := range stats {
		statsByID[st.Habit.ID] = st
	}

	from := now.AddDate(0, 0, -6)
	var sb strings.Builder
	sb.WriteString(i18n.T(lang, "weekly.header", from.Format("02.01"), now.Format("02.01")))

	totalDone, totalPossible := 0, 0
	for _, hw := range habits {
		if hw.IsPaused {
			continue
		}
		st, ok := statsByID[hw.ID]
		if !ok {
			continue
		}
		totalDone += st.CompletedDays
		totalPossible += 7
		icon := "✅"
		if st.CompletionPct < 50 {
			icon = "⚠️"
		}
		sb.WriteString(i18n.T(lang, "weekly.habit_line", icon, hw.Name, st.CompletedDays, st.CompletionPct))
	}

	if totalPossible == 0 {
		return
	}
	overall := totalDone * 100 / totalPossible
	sb.WriteString(i18n.T(lang, "weekly.overall", totalDone, totalPossible, overall))

	// Mood summary for the week.
	weekFrom := now.AddDate(0, 0, -6)
	moods, err := s.moodUC.GetWeekMoods(ctx, userID, weekFrom, now.AddDate(0, 0, 1))
	if err != nil {
		s.logger.Warn("weekly mood query", zap.Error(err))
	}
	if len(moods) > 0 {
		sb.WriteString(format.BuildMoodSummary(moods, lang))
		if format.CountMood(moods, 1) >= 3 {
			s.sendBurnoutAlert(ctx, telegramID, habits, lang)
		}
	}

	// Day-of-week insights (requires a user timezone lookup).
	if user, err := s.userUC.GetByID(ctx, userID); err == nil {
		dow, err := s.habitUC.GetDayOfWeekStats(ctx, userID, user.Timezone)
		if err == nil && len(dow) > 0 {
			best, worst := format.BestAndWorstDay(dow)
			sb.WriteString(format.BuildDayOfWeekInsight(dow, best, worst, lang))
		}
	}

	if _, err := s.api.Send(tgbotapi.NewMessage(telegramID, sb.String())); err != nil {
		s.logger.Error("send weekly digest", zap.Int64("telegram_id", telegramID), zap.Error(err))
		return
	}
	if err := s.cache.Set(ctx, key, "1", 8*24*time.Hour); err != nil {
		s.logger.Warn("weekly digest cache set", zap.Error(err))
	}
}

func (s *Scheduler) sendBurnoutAlert(ctx context.Context, telegramID int64, habits []*domain.HabitWithTelegramID, lang string) {
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, hw := range habits {
		if !hw.IsPaused {
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("⏸ "+hw.Name, fmt.Sprintf("pause:%d", hw.ID)),
			))
		}
	}
	msg := tgbotapi.NewMessage(telegramID, i18n.T(lang, "mood.burnout_alert"))
	if len(rows) > 0 {
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	}
	if _, err := s.api.Send(msg); err != nil {
		s.logger.Error("send burnout alert", zap.Int64("telegram_id", telegramID), zap.Error(err))
	}
}

func (s *Scheduler) maybeSendEveningRecap(ctx context.Context, telegramID int64, userID int64, lang string, habits []*domain.HabitWithTelegramID, now time.Time) {
	key := fmt.Sprintf("evening:%d:%s", telegramID, now.Format("2006-01-02"))
	if _, err := s.cache.Get(ctx, key); err == nil {
		return
	}

	if lang == "" {
		lang = i18n.RU
	}

	var sb strings.Builder
	sb.WriteString(i18n.T(lang, "evening.header"))

	done, total := 0, 0
	for _, hw := range habits {
		if hw.IsPaused {
			continue
		}
		total++
		if usecase.IsDoneToday(&hw.Habit, now) {
			done++
			sb.WriteString(i18n.T(lang, "evening.done_line", hw.Name))
		} else {
			sb.WriteString(i18n.T(lang, "evening.missed_line", hw.Name))
		}
	}
	if total == 0 {
		return
	}

	pct := done * 100 / total
	sb.WriteString(i18n.T(lang, "evening.summary", done, total, pct))

	user, err := s.userUC.GetByID(ctx, userID)
	if err == nil {
		sb.WriteString(i18n.T(lang, "evening.shields", user.StreakShields))
	}

	switch {
	case pct == 100:
		sb.WriteString(i18n.T(lang, "evening.perfect"))
	case pct >= 50:
		sb.WriteString(i18n.T(lang, "evening.good"))
	default:
		sb.WriteString(i18n.T(lang, "evening.nudge"))
	}

	if _, err := s.api.Send(tgbotapi.NewMessage(telegramID, sb.String())); err != nil {
		s.logger.Error("send evening recap", zap.Int64("telegram_id", telegramID), zap.Error(err))
		return
	}
	if err := s.cache.Set(ctx, key, "1", 25*time.Hour); err != nil {
		s.logger.Warn("evening recap cache set", zap.Error(err))
	}

	moodKey := fmt.Sprintf("mood_prompt:%d:%s", telegramID, now.Format("2006-01-02"))
	if _, err := s.cache.Get(ctx, moodKey); err != nil {
		logged, err := s.moodUC.HasLoggedToday(ctx, userID)
		if err == nil && !logged {
			moodMsg := tgbotapi.NewMessage(telegramID, i18n.T(lang, "mood.check_in"))
			moodMsg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "mood.great"), "mood:3"),
					tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "mood.okay"), "mood:2"),
					tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "mood.tough"), "mood:1"),
				),
			)
			if _, err := s.api.Send(moodMsg); err != nil {
				s.logger.Error("send evening mood prompt", zap.Int64("telegram_id", telegramID), zap.Error(err))
			} else {
				_ = s.cache.Set(ctx, moodKey, "1", 25*time.Hour)
			}
		}
	}
}

func (s *Scheduler) maybeSendStreakRisk(ctx context.Context, telegramID int64, lang string, habits []*domain.HabitWithTelegramID, now time.Time) {
	key := fmt.Sprintf("streak_risk:%d:%s", telegramID, now.Format("2006-01-02"))
	if _, err := s.cache.Get(ctx, key); err == nil {
		return
	}

	type atRiskEntry struct {
		name   string
		streak int
	}
	var atRisk []atRiskEntry
	for _, hw := range habits {
		if hw.IsPaused || hw.Streak == 0 {
			continue
		}
		if !usecase.IsDoneToday(&hw.Habit, now) {
			atRisk = append(atRisk, atRiskEntry{hw.Name, hw.Streak})
		}
	}
	if len(atRisk) == 0 {
		return
	}

	if lang == "" {
		lang = i18n.RU
	}
	var sb strings.Builder
	sb.WriteString(i18n.T(lang, "streak.risk_header"))
	for _, r := range atRisk {
		sb.WriteString(i18n.T(lang, "streak.risk_line", r.name, r.streak))
	}
	sb.WriteString(i18n.T(lang, "streak.risk_footer"))

	if _, err := s.api.Send(tgbotapi.NewMessage(telegramID, sb.String())); err != nil {
		s.logger.Error("send streak risk", zap.Int64("telegram_id", telegramID), zap.Error(err))
		return
	}
	if err := s.cache.Set(ctx, key, "1", 25*time.Hour); err != nil {
		s.logger.Warn("streak risk cache set", zap.Error(err))
	}
}

type userShieldRecord struct {
	lang       string
	protected  bool
	newShields int
}

// resetStreaksAndNotify resets streaks for inactive habits, respecting streak shields.
// Shields are checked BEFORE the reset: a user with shields has ALL their habits
// protected for the night (one shield consumed per user, not per habit).
func (s *Scheduler) resetStreaksAndNotify(ctx context.Context) {
	toReset, err := s.habitUC.ListStreaksToBeReset(ctx)
	if err != nil {
		s.logger.Error("ListStreaksToBeReset", zap.Error(err))
		return
	}

	// Resolve each user's shield state once to avoid repeated DB calls and
	// to ensure 1 shield protects ALL habits for that user this night.
	userRecords := make(map[int64]*userShieldRecord, len(toReset))
	for _, hw := range toReset {
		if _, seen := userRecords[hw.UserID]; seen {
			continue
		}
		rec := &userShieldRecord{lang: i18n.RU}
		user, err := s.userUC.GetByID(ctx, hw.UserID)
		if err == nil {
			if user.Language != "" {
				rec.lang = user.Language
			}
			if user.StreakShields > 0 {
				rec.protected = true
				rec.newShields = user.StreakShields - 1
				if err := s.userUC.UpdateStreakShields(ctx, hw.UserID, rec.newShields); err != nil {
					s.logger.Warn("UpdateStreakShields", zap.Error(err))
				}
			}
		}
		userRecords[hw.UserID] = rec
	}

	for _, hw := range toReset {
		rec := userRecords[hw.UserID]

		if rec.protected {
			// Shield active: streak is preserved, just notify the user.
			text := i18n.T(rec.lang, "shield.used", hw.Name, rec.newShields)
			if _, err := s.api.Send(tgbotapi.NewMessage(hw.TelegramID, text)); err != nil {
				s.logger.Error("send shield used", zap.Int64("telegram_id", hw.TelegramID), zap.Error(err))
			}
			continue
		}

		// No shield: reset this habit's streak in DB, then notify.
		if err := s.habitUC.ResetHabitStreak(ctx, hw.ID); err != nil {
			s.logger.Error("ResetHabitStreak", zap.Int64("habit_id", hw.ID), zap.Error(err))
		}

		text := i18n.T(rec.lang, "streak.broken", hw.Name, hw.Streak)
		msg := tgbotapi.NewMessage(hw.TelegramID, text)
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(i18n.T(rec.lang, "streak.do_now"), fmt.Sprintf("done:%d", hw.ID)),
			),
		)
		if _, err := s.api.Send(msg); err != nil {
			s.logger.Error("send streak break", zap.Int64("telegram_id", hw.TelegramID), zap.Error(err))
		}
	}
}
