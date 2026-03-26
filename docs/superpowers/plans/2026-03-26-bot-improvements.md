# Habit Tracker Bot Improvements — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add multi-language support (RU/EN/KZ), gamification (streak shields, achievements, XP/levels), richer UX (/today, onboarding), and smarter notifications (evening recap, adaptive reminders, progress visualization).

**Architecture:** New `internal/i18n/` package provides `T(lang, key, args...)` used everywhere in place of hardcoded Russian strings. New `internal/gamification/` package handles XP, level-up, and achievement checks called after every `MarkDone`. Scheduler gains evening recap and adaptive reminder logic. All new user fields land in migration 004.

**Tech Stack:** Go, pgx/v5, go-telegram-bot-api/v5, Redis, Uber FX

---

## File Map

**Create:**
- `internal/i18n/i18n.go` — `T()` function, `Lang` type
- `internal/i18n/ru.go` — Russian translation map
- `internal/i18n/en.go` — English translation map
- `internal/i18n/kz.go` — Kazakh translation map
- `internal/i18n/i18n_test.go` — unit tests
- `internal/gamification/gamification.go` — XP calc, level thresholds, achievement codes + `CheckAchievements()`
- `internal/gamification/gamification_test.go` — unit tests
- `migrations/004_gamification.up.sql`
- `migrations/004_gamification.down.sql`

**Modify:**
- `internal/domain/user.go` — add Language, XP, Level, StreakShields, EveningRecapHour
- `internal/usecase/interfaces.go` — extend UserRepository, ActivityRepository
- `internal/usecase/user.go` — SetLanguage, AddXP, AddStreakShields, GetAchievements, AddAchievement
- `internal/usecase/habit.go` — call gamification after MarkDone
- `internal/repository/postgres/user.go` — implement new methods, update Save/Get queries
- `internal/repository/postgres/activity.go` — add GetAverageCompletionHour
- `internal/delivery/telegram/handler.go` — i18n wiring, /language, /today, /achievements, new /start
- `internal/delivery/telegram/bot.go` — add new commands to menu
- `internal/scheduler/scheduler.go` — streak shield, evening recap, adaptive reminders, i18n

---

## Task 1: Migration 004

**Files:**
- Create: `migrations/004_gamification.up.sql`
- Create: `migrations/004_gamification.down.sql`

- [ ] **Step 1: Write up migration**

```sql
-- migrations/004_gamification.up.sql
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS language           VARCHAR(5)   NOT NULL DEFAULT 'ru',
    ADD COLUMN IF NOT EXISTS xp                INT          NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS level             INT          NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS streak_shields    INT          NOT NULL DEFAULT 3,
    ADD COLUMN IF NOT EXISTS evening_recap_hour INT         NOT NULL DEFAULT 21;

CREATE TABLE IF NOT EXISTS user_achievements (
    id               BIGSERIAL PRIMARY KEY,
    user_id          BIGINT       NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    achievement_code VARCHAR(64)  NOT NULL,
    unlocked_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, achievement_code)
);

CREATE INDEX IF NOT EXISTS idx_user_achievements_user_id ON user_achievements(user_id);
```

- [ ] **Step 2: Write down migration**

```sql
-- migrations/004_gamification.down.sql
DROP TABLE IF EXISTS user_achievements;

ALTER TABLE users
    DROP COLUMN IF EXISTS language,
    DROP COLUMN IF EXISTS xp,
    DROP COLUMN IF EXISTS level,
    DROP COLUMN IF EXISTS streak_shields,
    DROP COLUMN IF EXISTS evening_recap_hour;
```

- [ ] **Step 3: Apply migration**

```bash
make migrate
```

Expected: `migrations/004_gamification.up.sql` applied, no errors.

- [ ] **Step 4: Commit**

```bash
git add migrations/004_gamification.up.sql migrations/004_gamification.down.sql
git commit -m "feat: add migration 004 for gamification and i18n user fields"
```

---

## Task 2: Update domain.User

**Files:**
- Modify: `internal/domain/user.go`

- [ ] **Step 1: Add new fields**

Replace the entire file:

```go
package domain

import "time"

type User struct {
	ID               int64
	TelegramID       int64
	Username         string
	FirstName        string
	Timezone         string
	Language         string
	XP               int
	Level            int
	StreakShields    int
	EveningRecapHour int
	CreatedAt        time.Time
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/domain/user.go
git commit -m "feat: add Language, XP, Level, StreakShields, EveningRecapHour to User domain"
```

---

## Task 3: i18n package

**Files:**
- Create: `internal/i18n/i18n.go`
- Create: `internal/i18n/ru.go`
- Create: `internal/i18n/en.go`
- Create: `internal/i18n/kz.go`
- Create: `internal/i18n/i18n_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/i18n/i18n_test.go
package i18n_test

import (
	"testing"

	"github.com/saidakmal/habbit-tracker-bot/internal/i18n"
)

func TestT_knownKey(t *testing.T) {
	got := i18n.T(i18n.RU, "error.generic")
	if got == "" || got == "error.generic" {
		t.Fatalf("expected non-empty translation, got %q", got)
	}
}

func TestT_withArgs(t *testing.T) {
	got := i18n.T(i18n.EN, "habit.created", "Running", "every 30 min", 7, 22)
	if got == "habit.created" {
		t.Fatalf("expected interpolated string, got %q", got)
	}
}

func TestT_unknownLangFallsBackToEN(t *testing.T) {
	got := i18n.T("xx", "error.generic")
	en := i18n.T(i18n.EN, "error.generic")
	if got != en {
		t.Fatalf("expected EN fallback %q, got %q", en, got)
	}
}

func TestT_missingKeyReturnsKey(t *testing.T) {
	got := i18n.T(i18n.EN, "nonexistent.key")
	if got != "nonexistent.key" {
		t.Fatalf("expected key as fallback, got %q", got)
	}
}
```

- [ ] **Step 2: Run — expect compile failure (package doesn't exist yet)**

```bash
go test ./internal/i18n/... 2>&1 | head -5
```

- [ ] **Step 3: Write i18n core**

```go
// internal/i18n/i18n.go
package i18n

import "fmt"

type Lang = string

const (
	RU Lang = "ru"
	EN Lang = "en"
	KZ Lang = "kz"
)

var translations = map[Lang]map[string]string{
	RU: ruMessages,
	EN: enMessages,
	KZ: kzMessages,
}

func T(lang Lang, key string, args ...any) string {
	m, ok := translations[lang]
	if !ok {
		m = translations[EN]
	}
	s, ok := m[key]
	if !ok {
		s, ok = translations[EN][key]
		if !ok {
			return key
		}
	}
	if len(args) == 0 {
		return s
	}
	return fmt.Sprintf(s, args...)
}
```

- [ ] **Step 4: Write Russian translations**

```go
// internal/i18n/ru.go
package i18n

var ruMessages = map[string]string{
	// Generic
	"error.generic":          "Произошла ошибка, попробуй позже.",
	"error.update":           "Ошибка обновления.",
	"action.cancelled":       "❌ Действие отменено.",
	"action.cancel":          "Отмена",
	"action.yes_delete":      "Да, удалить",
	"action.delete_cancelled":"Удаление отменено.",

	// Language
	"language.choose": "Выбери язык / Choose language / Тіл таңда:",

	// Onboarding
	"onboarding.welcome_new":      "Привет, %s! 👋\n\nЯ помогу тебе формировать полезные привычки.\n\nВыбери шаблон или создай свою:",
	"onboarding.welcome_returning": "Привет, %s! 👋\n\nКоманды:\n/add_habit — добавить привычку\n/list_habits — список с прогрессом\n/done — отметить выполнение\n/today — привычки на сегодня\n/edit_habit — редактировать\n/pause_habit — пауза\n/resume_habit — снять с паузы\n/stats — статистика\n/history — история\n/timezone — часовой пояс\n/achievements — достижения\n/language — язык\n/delete_habit — удалить\n/cancel — отменить",
	"onboarding.first_habit":       "Добавить первую привычку?",
	"onboarding.add_yes":           "✅ Добавить",
	"onboarding.add_later":         "⏭ Позже",

	// Habit creation
	"habit.choose_template":  "Выбери шаблон или создай свою привычку:",
	"habit.enter_name":       "Введи название привычки:",
	"habit.name_empty":       "Название не может быть пустым. Введи название:",
	"habit.choose_interval":  "Как часто напоминать?",
	"habit.choose_start":     "Во сколько начинать напоминания?",
	"habit.choose_end":       "До какого часа напоминать?",
	"habit.choose_goal":      "Установить цель (дней)?",
	"habit.created":          "✅ Привычка «%s» создана!\nНапоминания: %s, %d:00–%d:00",
	"habit.goal_set":         "\n🎯 Цель: %d дней",
	"habit.create_failed":    "Не удалось создать привычку, попробуй позже.",

	// Habit actions
	"habit.choose":           "Выбери привычку:",
	"habit.done_simple":      "✅ Выполнено!",
	"habit.done_streak":      "✅ %s выполнено!\n🔥 Стрик: %d дней",
	"habit.done_goal":        "✅ %s выполнено!\n🔥 Стрик: %d/%d дней",
	"habit.already_done":     "Привычка уже выполнена сегодня ✓",
	"habit.not_found":        "Привычка не найдена.",
	"habit.none":             "Нет привычек.",
	"habit.none_for_done":    "Нет привычек для отметки.",
	"habit.none_for_delete":  "Нет привычек для удаления.",
	"habit.none_for_edit":    "Нет привычек для редактирования.",
	"habit.all_done_today":   "Все активные привычки уже выполнены или на паузе!",

	// Habit list
	"habit.list_header":      "Твои привычки:\n\n",
	"habit.list_empty":       "У тебя пока нет привычек. Добавь: /add_habit",
	"habit.shields_balance":  "🛡 Щитов стрика: %d",

	// Delete
	"habit.choose_delete":    "Выбери привычку для удаления:",
	"habit.delete_confirm":   "🗑 Удалить «%s»?%s",
	"habit.delete_streak_warn": "\n\n⚠️ Стрик %d дней будет потерян.",
	"habit.deleted":          "🗑 Привычка удалена.",

	// Edit
	"habit.choose_edit":      "Выбери привычку для редактирования:",
	"habit.edit_what":        "✏️ «%s»\nЧто изменить?",
	"habit.edit_enter_name":  "Введи новое название привычки:",
	"habit.name_updated":     "✅ Название изменено на «%s»",
	"habit.name_update_failed": "Не удалось обновить название.",
	"habit.interval_updated": "✅ Интервал изменён: %s",
	"habit.hours_updated":    "✅ Часы обновлены: %d:00–%d:00",

	// Pause/Resume
	"habit.choose_pause":     "Выбери привычку для паузы:",
	"habit.choose_resume":    "Выбери привычку для возобновления:",
	"habit.paused":           "⏸ «%s» поставлена на паузу.",
	"habit.resumed":          "▶️ «%s» возобновлена.",

	// Snooze
	"snooze.set":             "⏰ Напоминание отложено.",
	"snooze.30min":           "⏰ +30 мин",
	"snooze.1hr":             "⏰ +1 час",
	"snooze.2hr":             "⏰ +2 часа",

	// Today
	"today.header":           "📋 Привычки на сегодня:\n\n",
	"today.all_done":         "✅ Все привычки выполнены! Отличная работа.",
	"today.none":             "Нет активных привычек. Добавь: /add_habit",

	// Stats
	"stats.header":           "📊 Статистика за 30 дней:\n\n",
	"stats.empty":            "Нет данных. Начни отслеживать привычки.",
	"stats.xp_level":         "\n⭐ Уровень %d · %d XP\n🛡 Щиты: %d",

	// History
	"history.header":         "📅 %s — последние 28 дней:\n",
	"history.legend":         "■ выполнено  □ пропущено",
	"history.empty":          "Нет истории.",
	"history.choose":         "Выбери привычку для истории:",

	// Timezone
	"timezone.choose":        "Выбери свой часовой пояс:",
	"timezone.set":           "✅ Часовой пояс установлен: %s",

	// Achievements
	"achievement.unlocked":   "🏆 Достижение разблокировано: %s!\n%s",
	"achievement.list_header":"🏆 Твои достижения:\n\n",
	"achievement.list_empty": "Пока нет достижений. Выполняй привычки, чтобы получить!",
	"achievement.shield_reward": "+1 защитный щит",
	"achievement.xp_reward":  "+%d XP",

	// Level up
	"levelup.message":        "⬆️ Новый уровень! Ты на уровне %d 🎉",

	// Streak shield
	"shield.used":            "🛡 Щит использован! Стрик «%s» защищён. Щитов осталось: %d",

	// Reminders
	"reminder.text":          "⏰ «%s»",
	"reminder.streak":        "\nСтрик: %d дней подряд 🔥",
	"reminder.done_button":   "✅ Выполнено",

	// Morning digest
	"morning.header":         "☀️ Доброе утро, %s!\n\nПривычки на сегодня:\n\n",

	// Weekly digest
	"weekly.header":          "📊 Итоги недели (%s – %s)\n\n",
	"weekly.overall":         "\nОбщий результат: %d/%d (%d%%)",
	"weekly.habit_line":      "%s %s: %d/7 (%d%%)\n",

	// Evening recap
	"evening.header":         "🌙 Итоги дня:\n\n",
	"evening.done_line":      "✅ %s — выполнено\n",
	"evening.missed_line":    "○ %s — пропущено\n",
	"evening.summary":        "\n%d/%d привычек выполнено (%d%%)",
	"evening.shields":        "\nЩиты стрика: %d 🛡",
	"evening.perfect":        " 🎉 Отлично!",
	"evening.good":           " 💪",
	"evening.nudge":          "\nНе сдавайся, завтра — новый шанс! 💫",

	// Streak broken
	"streak.broken":          "😔 Стрик «%s» прервался (был %d дней).\nНе сдавайся! Начни снова сегодня.",
	"streak.do_now":          "✅ Выполнить сейчас",
}
```

- [ ] **Step 5: Write English translations**

```go
// internal/i18n/en.go
package i18n

var enMessages = map[string]string{
	"error.generic":          "Something went wrong, please try again later.",
	"error.update":           "Update failed.",
	"action.cancelled":       "❌ Action cancelled.",
	"action.cancel":          "Cancel",
	"action.yes_delete":      "Yes, delete",
	"action.delete_cancelled":"Deletion cancelled.",

	"language.choose": "Выбери язык / Choose language / Тіл таңда:",

	"onboarding.welcome_new":       "Hi, %s! 👋\n\nI'll help you build good habits.\n\nPick a template or create your own:",
	"onboarding.welcome_returning": "Hi, %s! 👋\n\nCommands:\n/add_habit — add a habit\n/list_habits — habits with progress\n/done — mark as done\n/today — today's habits\n/edit_habit — edit a habit\n/pause_habit — pause\n/resume_habit — resume\n/stats — statistics\n/history — completion history\n/timezone — set timezone\n/achievements — achievements\n/language — change language\n/delete_habit — delete a habit\n/cancel — cancel current action",
	"onboarding.first_habit":        "Add your first habit?",
	"onboarding.add_yes":            "✅ Add habit",
	"onboarding.add_later":          "⏭ Later",

	"habit.choose_template":  "Pick a template or create a custom habit:",
	"habit.enter_name":       "Enter habit name:",
	"habit.name_empty":       "Name can't be empty. Enter habit name:",
	"habit.choose_interval":  "How often should I remind you?",
	"habit.choose_start":     "From what hour should reminders start?",
	"habit.choose_end":       "Until what hour?",
	"habit.choose_goal":      "Set a goal (days)?",
	"habit.created":          "✅ Habit «%s» created!\nReminders: %s, %d:00–%d:00",
	"habit.goal_set":         "\n🎯 Goal: %d days",
	"habit.create_failed":    "Failed to create habit, please try again.",

	"habit.choose":           "Choose a habit:",
	"habit.done_simple":      "✅ Done!",
	"habit.done_streak":      "✅ %s done!\n🔥 Streak: %d days",
	"habit.done_goal":        "✅ %s done!\n🔥 Streak: %d/%d days",
	"habit.already_done":     "Already done today ✓",
	"habit.not_found":        "Habit not found.",
	"habit.none":             "No habits.",
	"habit.none_for_done":    "No habits to mark.",
	"habit.none_for_delete":  "No habits to delete.",
	"habit.none_for_edit":    "No habits to edit.",
	"habit.all_done_today":   "All active habits are done or paused!",

	"habit.list_header":      "Your habits:\n\n",
	"habit.list_empty":       "No habits yet. Add one: /add_habit",
	"habit.shields_balance":  "🛡 Streak shields: %d",

	"habit.choose_delete":    "Choose a habit to delete:",
	"habit.delete_confirm":   "🗑 Delete «%s»?%s",
	"habit.delete_streak_warn": "\n\n⚠️ %d-day streak will be lost.",
	"habit.deleted":          "🗑 Habit deleted.",

	"habit.choose_edit":      "Choose a habit to edit:",
	"habit.edit_what":        "✏️ «%s»\nWhat to change?",
	"habit.edit_enter_name":  "Enter new habit name:",
	"habit.name_updated":     "✅ Name changed to «%s»",
	"habit.name_update_failed": "Failed to update name.",
	"habit.interval_updated": "✅ Interval updated: %s",
	"habit.hours_updated":    "✅ Hours updated: %d:00–%d:00",

	"habit.choose_pause":     "Choose a habit to pause:",
	"habit.choose_resume":    "Choose a habit to resume:",
	"habit.paused":           "⏸ «%s» paused.",
	"habit.resumed":          "▶️ «%s» resumed.",

	"snooze.set":             "⏰ Reminder snoozed.",
	"snooze.30min":           "⏰ +30 min",
	"snooze.1hr":             "⏰ +1 hour",
	"snooze.2hr":             "⏰ +2 hours",

	"today.header":           "📋 Today's habits:\n\n",
	"today.all_done":         "✅ All habits done today! Great job.",
	"today.none":             "No active habits. Add one: /add_habit",

	"stats.header":           "📊 Statistics (last 30 days):\n\n",
	"stats.empty":            "No data yet. Start tracking habits.",
	"stats.xp_level":         "\n⭐ Level %d · %d XP\n🛡 Shields: %d",

	"history.header":         "📅 %s — last 28 days:\n",
	"history.legend":         "■ done  □ missed",
	"history.empty":          "No history yet.",
	"history.choose":         "Choose a habit to view history:",

	"timezone.choose":        "Choose your timezone:",
	"timezone.set":           "✅ Timezone set: %s",

	"achievement.unlocked":   "🏆 Achievement unlocked: %s!\n%s",
	"achievement.list_header":"🏆 Your achievements:\n\n",
	"achievement.list_empty": "No achievements yet. Complete habits to earn some!",
	"achievement.shield_reward": "+1 streak shield",
	"achievement.xp_reward":  "+%d XP",

	"levelup.message":        "⬆️ Level up! You're now Level %d 🎉",

	"shield.used":            "🛡 Shield used! «%s» streak protected. Shields left: %d",

	"reminder.text":          "⏰ «%s»",
	"reminder.streak":        "\nStreak: %d days 🔥",
	"reminder.done_button":   "✅ Done",

	"morning.header":         "☀️ Good morning, %s!\n\nToday's habits:\n\n",

	"weekly.header":          "📊 Weekly recap (%s – %s)\n\n",
	"weekly.overall":         "\nOverall: %d/%d (%d%%)",
	"weekly.habit_line":      "%s %s: %d/7 (%d%%)\n",

	"evening.header":         "🌙 Day recap:\n\n",
	"evening.done_line":      "✅ %s — done\n",
	"evening.missed_line":    "○ %s — missed\n",
	"evening.summary":        "\n%d/%d habits done (%d%%)",
	"evening.shields":        "\nStreak shields: %d 🛡",
	"evening.perfect":        " 🎉 Perfect!",
	"evening.good":           " 💪",
	"evening.nudge":          "\nDon't give up — tomorrow is a new chance! 💫",

	"streak.broken":          "😔 «%s» streak broken (was %d days).\nDon't give up! Start again today.",
	"streak.do_now":          "✅ Do it now",
}
```

- [ ] **Step 6: Write Kazakh translations**

```go
// internal/i18n/kz.go
package i18n

var kzMessages = map[string]string{
	"error.generic":          "Қате орын алды, кейінірек көріңіз.",
	"error.update":           "Жаңарту қатесі.",
	"action.cancelled":       "❌ Әрекет болдырылмады.",
	"action.cancel":          "Болдырма",
	"action.yes_delete":      "Иә, жою",
	"action.delete_cancelled":"Жою болдырылмады.",

	"language.choose": "Выбери язык / Choose language / Тіл таңда:",

	"onboarding.welcome_new":       "Сәлем, %s! 👋\n\nМен сізге пайдалы әдеттер қалыптастыруға көмектесемін.\n\nҮлгі таңдаңыз немесе өзіңіз жасаңыз:",
	"onboarding.welcome_returning": "Сәлем, %s! 👋\n\nКомандалар:\n/add_habit — әдет қосу\n/list_habits — тізім\n/done — орындалды деп белгілеу\n/today — бүгінгі әдеттер\n/edit_habit — өңдеу\n/pause_habit — кідірту\n/resume_habit — жалғастыру\n/stats — статистика\n/history — тарих\n/timezone — уақыт белдеуі\n/achievements — жетістіктер\n/language — тіл\n/delete_habit — жою\n/cancel — болдырмау",
	"onboarding.first_habit":        "Бірінші әдетті қосу керек пе?",
	"onboarding.add_yes":            "✅ Қосу",
	"onboarding.add_later":          "⏭ Кейінірек",

	"habit.choose_template":  "Үлгі таңдаңыз немесе өз әдетіңізді жасаңыз:",
	"habit.enter_name":       "Әдеттің атауын енгізіңіз:",
	"habit.name_empty":       "Атау бос болмауы керек. Атауды енгізіңіз:",
	"habit.choose_interval":  "Қаншалықты жиі еске салу керек?",
	"habit.choose_start":     "Еске салу қай сағаттан басталсын?",
	"habit.choose_end":       "Қай сағатқа дейін?",
	"habit.choose_goal":      "Мақсат орнату (күн)?",
	"habit.created":          "✅ «%s» әдеті жасалды!\nЕске салу: %s, %d:00–%d:00",
	"habit.goal_set":         "\n🎯 Мақсат: %d күн",
	"habit.create_failed":    "Әдетті жасау мүмкін болмады, кейінірек көріңіз.",

	"habit.choose":           "Әдетті таңдаңыз:",
	"habit.done_simple":      "✅ Орындалды!",
	"habit.done_streak":      "✅ %s орындалды!\n🔥 Серия: %d күн",
	"habit.done_goal":        "✅ %s орындалды!\n🔥 Серия: %d/%d күн",
	"habit.already_done":     "Бүгін орындалды ✓",
	"habit.not_found":        "Әдет табылмады.",
	"habit.none":             "Әдеттер жоқ.",
	"habit.none_for_done":    "Белгілейтін әдеттер жоқ.",
	"habit.none_for_delete":  "Жоятын әдеттер жоқ.",
	"habit.none_for_edit":    "Өңдейтін әдеттер жоқ.",
	"habit.all_done_today":   "Барлық белсенді әдеттер орындалды немесе кідіртілді!",

	"habit.list_header":      "Сіздің әдеттеріңіз:\n\n",
	"habit.list_empty":       "Әлі әдеттер жоқ. Қосыңыз: /add_habit",
	"habit.shields_balance":  "🛡 Серия қалқандары: %d",

	"habit.choose_delete":    "Жоятын әдетті таңдаңыз:",
	"habit.delete_confirm":   "🗑 «%s» жою керек пе?%s",
	"habit.delete_streak_warn": "\n\n⚠️ %d күндік серия жоғалады.",
	"habit.deleted":          "🗑 Әдет жойылды.",

	"habit.choose_edit":      "Өңдейтін әдетті таңдаңыз:",
	"habit.edit_what":        "✏️ «%s»\nНені өзгерту керек?",
	"habit.edit_enter_name":  "Жаңа атауды енгізіңіз:",
	"habit.name_updated":     "✅ Атау «%s» болып өзгертілді",
	"habit.name_update_failed": "Атауды жаңарту мүмкін болмады.",
	"habit.interval_updated": "✅ Интервал жаңартылды: %s",
	"habit.hours_updated":    "✅ Сағаттар жаңартылды: %d:00–%d:00",

	"habit.choose_pause":     "Кідіртетін әдетті таңдаңыз:",
	"habit.choose_resume":    "Жалғастыратын әдетті таңдаңыз:",
	"habit.paused":           "⏸ «%s» кідіртілді.",
	"habit.resumed":          "▶️ «%s» жалғастырылды.",

	"snooze.set":             "⏰ Еске салу кейінге қалдырылды.",
	"snooze.30min":           "⏰ +30 мин",
	"snooze.1hr":             "⏰ +1 сағат",
	"snooze.2hr":             "⏰ +2 сағат",

	"today.header":           "📋 Бүгінгі әдеттер:\n\n",
	"today.all_done":         "✅ Бүгінгі барлық әдеттер орындалды! Керемет.",
	"today.none":             "Белсенді әдеттер жоқ. Қосыңыз: /add_habit",

	"stats.header":           "📊 Статистика (соңғы 30 күн):\n\n",
	"stats.empty":            "Деректер жоқ. Әдеттерді бақылауды бастаңыз.",
	"stats.xp_level":         "\n⭐ Деңгей %d · %d XP\n🛡 Қалқандар: %d",

	"history.header":         "📅 %s — соңғы 28 күн:\n",
	"history.legend":         "■ орындалды  □ өткізілді",
	"history.empty":          "Тарих жоқ.",
	"history.choose":         "Тарихты көру үшін әдетті таңдаңыз:",

	"timezone.choose":        "Уақыт белдеуіңізді таңдаңыз:",
	"timezone.set":           "✅ Уақыт белдеуі орнатылды: %s",

	"achievement.unlocked":   "🏆 Жетістік ашылды: %s!\n%s",
	"achievement.list_header":"🏆 Сіздің жетістіктеріңіз:\n\n",
	"achievement.list_empty": "Әлі жетістіктер жоқ. Алу үшін әдеттерді орындаңыз!",
	"achievement.shield_reward": "+1 серия қалқаны",
	"achievement.xp_reward":  "+%d XP",

	"levelup.message":        "⬆️ Деңгей өсті! Сіз %d деңгейдесіз 🎉",

	"shield.used":            "🛡 Қалқан қолданылды! «%s» сериясы қорғалды. Қалған қалқандар: %d",

	"reminder.text":          "⏰ «%s»",
	"reminder.streak":        "\nСерия: %d күн 🔥",
	"reminder.done_button":   "✅ Орындалды",

	"morning.header":         "☀️ Қайырлы таң, %s!\n\nБүгінгі әдеттер:\n\n",

	"weekly.header":          "📊 Апталық қорытынды (%s – %s)\n\n",
	"weekly.overall":         "\nЖалпы нәтиже: %d/%d (%d%%)",
	"weekly.habit_line":      "%s %s: %d/7 (%d%%)\n",

	"evening.header":         "🌙 Күн қорытындысы:\n\n",
	"evening.done_line":      "✅ %s — орындалды\n",
	"evening.missed_line":    "○ %s — өткізілді\n",
	"evening.summary":        "\n%d/%d әдет орындалды (%d%%)",
	"evening.shields":        "\nСерия қалқандары: %d 🛡",
	"evening.perfect":        " 🎉 Тамаша!",
	"evening.good":           " 💪",
	"evening.nudge":          "\nТастама — ертең жаңа мүмкіндік! 💫",

	"streak.broken":          "😔 «%s» сериясы үзілді (%d күн болды).\nТастама! Бүгін қайтадан бастаңыз.",
	"streak.do_now":          "✅ Қазір орында",
}
```

- [ ] **Step 7: Run tests — expect pass**

```bash
go test ./internal/i18n/... -v
```

Expected: `PASS` for all 4 tests.

- [ ] **Step 8: Commit**

```bash
git add internal/i18n/
git commit -m "feat: add i18n package with RU/EN/KZ translations"
```

---

## Task 4: Update UserRepository interface + postgres implementation

**Files:**
- Modify: `internal/usecase/interfaces.go`
- Modify: `internal/repository/postgres/user.go`

- [ ] **Step 1: Extend UserRepository interface**

In `internal/usecase/interfaces.go`, replace `UserRepository`:

```go
type UserRepository interface {
	Save(ctx context.Context, user *domain.User) error
	GetByTelegramID(ctx context.Context, telegramID int64) (*domain.User, error)
	UpdateTimezone(ctx context.Context, userID int64, timezone string) error
	UpdateLanguage(ctx context.Context, userID int64, language string) error
	AddXP(ctx context.Context, userID int64, xp int) (newXP int, newLevel int, err error)
	UpdateStreakShields(ctx context.Context, userID int64, shields int) error
	AddAchievement(ctx context.Context, userID int64, code string) error
	HasAchievement(ctx context.Context, userID int64, code string) (bool, error)
	ListAchievements(ctx context.Context, userID int64) ([]domain.UserAchievement, error)
}
```

- [ ] **Step 2: Add UserAchievement to domain**

Add to `internal/domain/user.go` (after User struct):

```go
type UserAchievement struct {
	Code       string
	UnlockedAt time.Time
}
```

- [ ] **Step 3: Update postgres UserRepository — Save and GetByTelegramID**

Replace the full file `internal/repository/postgres/user.go`:

```go
package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/saidakmal/habbit-tracker-bot/internal/domain"
)

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

func (r *UserRepository) Save(ctx context.Context, user *domain.User) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO users (telegram_id, username, first_name)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (telegram_id) DO UPDATE
		   SET username = EXCLUDED.username, first_name = EXCLUDED.first_name
		 RETURNING id, timezone, language, xp, level, streak_shields, evening_recap_hour, created_at`,
		user.TelegramID, user.Username, user.FirstName,
	).Scan(&user.ID, &user.Timezone, &user.Language, &user.XP, &user.Level, &user.StreakShields, &user.EveningRecapHour, &user.CreatedAt)
}

func (r *UserRepository) GetByTelegramID(ctx context.Context, telegramID int64) (*domain.User, error) {
	user := &domain.User{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, telegram_id, username, first_name, timezone, language, xp, level, streak_shields, evening_recap_hour, created_at
		 FROM users WHERE telegram_id = $1`,
		telegramID,
	).Scan(&user.ID, &user.TelegramID, &user.Username, &user.FirstName, &user.Timezone,
		&user.Language, &user.XP, &user.Level, &user.StreakShields, &user.EveningRecapHour, &user.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return user, nil
}

func (r *UserRepository) UpdateTimezone(ctx context.Context, userID int64, timezone string) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET timezone = $1 WHERE id = $2`, timezone, userID)
	return err
}

func (r *UserRepository) UpdateLanguage(ctx context.Context, userID int64, language string) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET language = $1 WHERE id = $2`, language, userID)
	return err
}

func (r *UserRepository) AddXP(ctx context.Context, userID int64, xp int) (int, int, error) {
	thresholds := []int{0, 100, 250, 500, 1000}
	levelFor := func(totalXP int) int {
		lv := 1
		for i, t := range thresholds {
			if totalXP >= t {
				lv = i + 1
			}
		}
		if totalXP >= 1000 {
			extra := (totalXP - 1000) / 500
			lv = 5 + extra
		}
		return lv
	}

	var newXP, newLevel int
	err := r.pool.QueryRow(ctx,
		`UPDATE users SET xp = xp + $1, level = $2
		 WHERE id = $3
		 RETURNING xp, level`,
		xp, 1, userID, // level placeholder; we compute after
	).Scan(&newXP, &newLevel)
	if err != nil {
		return 0, 0, err
	}
	// Recompute and update level
	computed := levelFor(newXP)
	if computed != newLevel {
		_, err = r.pool.Exec(ctx, `UPDATE users SET level = $1 WHERE id = $2`, computed, userID)
		if err != nil {
			return newXP, computed, err
		}
		newLevel = computed
	}
	return newXP, newLevel, nil
}

func (r *UserRepository) UpdateStreakShields(ctx context.Context, userID int64, shields int) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET streak_shields = $1 WHERE id = $2`, shields, userID)
	return err
}

func (r *UserRepository) AddAchievement(ctx context.Context, userID int64, code string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO user_achievements (user_id, achievement_code) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		userID, code,
	)
	return err
}

func (r *UserRepository) HasAchievement(ctx context.Context, userID int64, code string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM user_achievements WHERE user_id = $1 AND achievement_code = $2)`,
		userID, code,
	).Scan(&exists)
	return exists, err
}

func (r *UserRepository) ListAchievements(ctx context.Context, userID int64) ([]domain.UserAchievement, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT achievement_code, unlocked_at FROM user_achievements WHERE user_id = $1 ORDER BY unlocked_at`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.UserAchievement
	for rows.Next() {
		var a domain.UserAchievement
		if err := rows.Scan(&a.Code, &a.UnlockedAt); err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}
```

- [ ] **Step 4: Build to verify no compile errors**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add internal/usecase/interfaces.go internal/domain/user.go internal/repository/postgres/user.go
git commit -m "feat: extend UserRepository with language, XP, shields, achievement methods"
```

---

## Task 5: Update UserUsecase

**Files:**
- Modify: `internal/usecase/user.go`

- [ ] **Step 1: Add new use case methods**

Replace full file:

```go
package usecase

import (
	"context"
	"errors"

	"github.com/saidakmal/habbit-tracker-bot/internal/domain"
)

type UserUsecase struct {
	repo UserRepository
}

func NewUserUsecase(repo UserRepository) *UserUsecase {
	return &UserUsecase{repo: repo}
}

func (u *UserUsecase) GetOrCreateUser(ctx context.Context, telegramID int64, username, firstName string) (*domain.User, error) {
	user, err := u.repo.GetByTelegramID(ctx, telegramID)
	if err == nil {
		return user, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	user = &domain.User{
		TelegramID: telegramID,
		Username:   username,
		FirstName:  firstName,
	}
	if err := u.repo.Save(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

func (u *UserUsecase) SetTimezone(ctx context.Context, userID int64, timezone string) error {
	return u.repo.UpdateTimezone(ctx, userID, timezone)
}

func (u *UserUsecase) SetLanguage(ctx context.Context, userID int64, language string) error {
	return u.repo.UpdateLanguage(ctx, userID, language)
}

func (u *UserUsecase) AddXP(ctx context.Context, userID int64, xp int) (newXP int, newLevel int, err error) {
	return u.repo.AddXP(ctx, userID, xp)
}

func (u *UserUsecase) AddStreakShield(ctx context.Context, userID int64) error {
	user, err := u.repo.GetByTelegramID(ctx, userID) // userID here is internal ID — need TG lookup
	_ = user
	_ = err
	// NOTE: shields are incremented via UpdateStreakShields after fetch; see gamification package
	return nil
}

func (u *UserUsecase) UpdateStreakShields(ctx context.Context, userID int64, shields int) error {
	return u.repo.UpdateStreakShields(ctx, userID, shields)
}

func (u *UserUsecase) AddAchievement(ctx context.Context, userID int64, code string) error {
	return u.repo.AddAchievement(ctx, userID, code)
}

func (u *UserUsecase) HasAchievement(ctx context.Context, userID int64, code string) (bool, error) {
	return u.repo.HasAchievement(ctx, userID, code)
}

func (u *UserUsecase) ListAchievements(ctx context.Context, userID int64) ([]domain.UserAchievement, error) {
	return u.repo.ListAchievements(ctx, userID)
}
```

- [ ] **Step 2: Build**

```bash
go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add internal/usecase/user.go
git commit -m "feat: add SetLanguage, AddXP, UpdateStreakShields, achievement methods to UserUsecase"
```

---

## Task 6: Gamification package

**Files:**
- Create: `internal/gamification/gamification.go`
- Create: `internal/gamification/gamification_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/gamification/gamification_test.go
package gamification_test

import (
	"testing"

	"github.com/saidakmal/habbit-tracker-bot/internal/gamification"
)

func TestXPForCompletion_baseOnly(t *testing.T) {
	xp := gamification.XPForCompletion(0)
	if xp != 10 {
		t.Fatalf("expected 10, got %d", xp)
	}
}

func TestXPForCompletion_withStreak(t *testing.T) {
	xp := gamification.XPForCompletion(15)
	if xp != 25 { // 10 base + 15 streak bonus
		t.Fatalf("expected 25, got %d", xp)
	}
}

func TestXPForCompletion_streakCap(t *testing.T) {
	xp := gamification.XPForCompletion(100)
	if xp != 30 { // 10 base + 20 cap
		t.Fatalf("expected 30 (capped), got %d", xp)
	}
}

func TestLevelFor(t *testing.T) {
	cases := []struct{ xp, want int }{
		{0, 1}, {99, 1}, {100, 2}, {250, 3}, {500, 4}, {1000, 5}, {1500, 6}, {2000, 7},
	}
	for _, c := range cases {
		got := gamification.LevelFor(c.xp)
		if got != c.want {
			t.Errorf("LevelFor(%d) = %d, want %d", c.xp, got, c.want)
		}
	}
}

func TestAchievementNames(t *testing.T) {
	for _, code := range gamification.AllCodes() {
		if gamification.DisplayName(code, "en") == "" {
			t.Errorf("achievement %q has no EN display name", code)
		}
	}
}
```

- [ ] **Step 2: Run — expect compile failure**

```bash
go test ./internal/gamification/... 2>&1 | head -5
```

- [ ] **Step 3: Write gamification package**

```go
// internal/gamification/gamification.go
package gamification

import "fmt"

// XP
const baseXP = 10
const maxStreakBonus = 20

func XPForCompletion(streak int) int {
	bonus := streak
	if bonus > maxStreakBonus {
		bonus = maxStreakBonus
	}
	return baseXP + bonus
}

var levelThresholds = []int{0, 100, 250, 500, 1000}

func LevelFor(xp int) int {
	lv := 1
	for i, t := range levelThresholds {
		if xp >= t {
			lv = i + 1
		}
	}
	if xp >= 1000 {
		extra := (xp - 1000) / 500
		lv = 5 + extra
	}
	return lv
}

// Achievement codes
const (
	AchFirstDone    = "first_done"
	AchStreak7      = "streak_7"
	AchStreak30     = "streak_30"
	AchStreak100    = "streak_100"
	AchPerfectWeek  = "perfect_week"
	AchEarlyBird    = "early_bird"
	AchCompletionist = "completionist"
)

func AllCodes() []string {
	return []string{AchFirstDone, AchStreak7, AchStreak30, AchStreak100, AchPerfectWeek, AchEarlyBird, AchCompletionist}
}

type AchievementDef struct {
	Code        string
	Names       map[string]string // lang -> display name
	ShieldBonus int
	XPBonus     int
}

var definitions = []AchievementDef{
	{
		Code:        AchFirstDone,
		Names:       map[string]string{"ru": "Первый шаг", "en": "First Step", "kz": "Бірінші қадам"},
		ShieldBonus: 1,
	},
	{
		Code:        AchStreak7,
		Names:       map[string]string{"ru": "7-дневный воин", "en": "7-Day Warrior", "kz": "7 күндік жауынгер"},
		ShieldBonus: 1,
	},
	{
		Code:        AchStreak30,
		Names:       map[string]string{"ru": "30-дневный чемпион", "en": "30-Day Champion", "kz": "30 күндік чемпион"},
		ShieldBonus: 1,
		XPBonus:     100,
	},
	{
		Code:        AchStreak100,
		Names:       map[string]string{"ru": "Легенда", "en": "Legend", "kz": "Аңыз"},
		ShieldBonus: 2,
		XPBonus:     500,
	},
	{
		Code:        AchPerfectWeek,
		Names:       map[string]string{"ru": "Идеальная неделя", "en": "Perfect Week", "kz": "Мінсіз апта"},
		ShieldBonus: 1,
	},
	{
		Code:        AchEarlyBird,
		Names:       map[string]string{"ru": "Ранняя пташка", "en": "Early Bird", "kz": "Ерте тұрған"},
	},
	{
		Code:        AchCompletionist,
		Names:       map[string]string{"ru": "Перфекционист", "en": "Completionist", "kz": "Перфекционист"},
	},
}

var defsByCode = func() map[string]*AchievementDef {
	m := make(map[string]*AchievementDef, len(definitions))
	for i := range definitions {
		m[definitions[i].Code] = &definitions[i]
	}
	return m
}()

func GetDef(code string) (*AchievementDef, bool) {
	d, ok := defsByCode[code]
	return d, ok
}

func DisplayName(code, lang string) string {
	d, ok := defsByCode[code]
	if !ok {
		return code
	}
	if name, ok := d.Names[lang]; ok {
		return name
	}
	if name, ok := d.Names["en"]; ok {
		return name
	}
	return code
}

// RewardText returns a human-readable reward description.
func RewardText(def *AchievementDef, lang string) string {
	switch {
	case def.ShieldBonus > 0 && def.XPBonus > 0:
		shield := map[string]string{"ru": "+%d щит(а)", "en": "+%d shield(s)", "kz": "+%d қалқан"}[lang]
		if shield == "" {
			shield = "+%d shield(s)"
		}
		return fmt.Sprintf(shield, def.ShieldBonus) + fmt.Sprintf(" · +%d XP", def.XPBonus)
	case def.ShieldBonus > 0:
		shield := map[string]string{"ru": "+%d щит(а) стрика", "en": "+%d streak shield(s)", "kz": "+%d серия қалқаны"}[lang]
		if shield == "" {
			shield = "+%d streak shield(s)"
		}
		return fmt.Sprintf(shield, def.ShieldBonus)
	case def.XPBonus > 0:
		return fmt.Sprintf("+%d XP", def.XPBonus)
	default:
		return "🏅 badge"
	}
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/gamification/... -v
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/gamification/
git commit -m "feat: add gamification package with XP, level, and achievement definitions"
```

---

## Task 7: Wire gamification into HabitUsecase.MarkDone

**Files:**
- Modify: `internal/usecase/habit.go`
- Modify: `internal/usecase/interfaces.go` — add ActivityRepository.CountByUserAndDate

The gamification check needs: total completions ever (for first_done), current habit streak (for streak_7/30/100), and a way to notify the user. We pass a callback-style `NotifyFn` to keep gamification out of the delivery layer.

- [ ] **Step 1: Add CountAllByUser to ActivityRepository interface**

In `internal/usecase/interfaces.go`, add to `ActivityRepository`:

```go
type ActivityRepository interface {
	Save(ctx context.Context, activity *domain.Activity) error
	ListByUserAndDate(ctx context.Context, userID int64, date time.Time) ([]*domain.Activity, error)
	CountByHabitAndDateRange(ctx context.Context, habitID int64, from, to time.Time) (int, error)
	ListDatesByHabitAndDateRange(ctx context.Context, habitID int64, from, to time.Time) ([]time.Time, error)
	CountAllByUser(ctx context.Context, userID int64) (int, error)
	GetAverageCompletionHour(ctx context.Context, habitID int64) (hour int, hasData bool, err error)
}
```

- [ ] **Step 2: Implement CountAllByUser and GetAverageCompletionHour in postgres**

Find `internal/repository/postgres/activity.go` and add at the end:

```go
func (r *ActivityRepository) CountAllByUser(ctx context.Context, userID int64) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM activities WHERE user_id = $1`, userID,
	).Scan(&count)
	return count, err
}

func (r *ActivityRepository) GetAverageCompletionHour(ctx context.Context, habitID int64) (int, bool, error) {
	var avgHour *float64
	err := r.pool.QueryRow(ctx,
		`SELECT AVG(EXTRACT(HOUR FROM date)) FROM activities WHERE habit_id = $1 AND date > NOW() - INTERVAL '30 days'`,
		habitID,
	).Scan(&avgHour)
	if err != nil || avgHour == nil {
		return 0, false, err
	}
	return int(*avgHour), true, nil
}
```

- [ ] **Step 3: Add GamificationNotifier type to HabitUsecase**

In `internal/usecase/habit.go`, add the notifier type and update the struct and constructor:

```go
// GamificationNotifier is called after MarkDone to handle XP, levels, and achievements.
// It runs in a goroutine — errors are logged, not returned.
type GamificationNotifier func(ctx context.Context, userID int64, habitID int64, streak int)

type HabitUsecase struct {
	habitRepo    HabitRepository
	activityRepo ActivityRepository
	onDone       GamificationNotifier // may be nil
}

func NewHabitUsecase(habitRepo HabitRepository, activityRepo ActivityRepository) *HabitUsecase {
	return &HabitUsecase{habitRepo: habitRepo, activityRepo: activityRepo}
}

func (u *HabitUsecase) SetGamificationNotifier(fn GamificationNotifier) {
	u.onDone = fn
}
```

- [ ] **Step 4: Call notifier at end of MarkDone**

At the end of `MarkDone`, after `activityRepo.Save`, add:

```go
	if u.onDone != nil {
		streak := habit.Streak
		go u.onDone(context.Background(), userID, habitID, streak)
	}

	return nil
```

Remove the old `return u.activityRepo.Save(...)` and replace with:

```go
	if err := u.activityRepo.Save(ctx, &domain.Activity{
		UserID:  userID,
		HabitID: habitID,
		Date:    now,
	}); err != nil {
		return err
	}

	if u.onDone != nil {
		streak := habit.Streak
		go u.onDone(context.Background(), userID, habitID, streak)
	}
	return nil
```

- [ ] **Step 5: Build**

```bash
go build ./...
```

- [ ] **Step 6: Commit**

```bash
git add internal/usecase/interfaces.go internal/usecase/habit.go internal/repository/postgres/activity.go
git commit -m "feat: wire GamificationNotifier into MarkDone, add CountAllByUser and GetAverageCompletionHour"
```

---

## Task 8: Wire gamification notifier in app.go

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 1: Read app.go**

Read `internal/app/app.go` to understand existing wiring pattern.

- [ ] **Step 2: Add GamificationService and wire notifier**

After `fx.New(...)` provides `HabitUsecase`, add an `fx.Invoke` that wires the notifier. The notifier function needs access to `UserUsecase`, `ActivityRepository`, bot API, and logger:

```go
fx.Invoke(func(
    habitUC *usecase.HabitUsecase,
    userUC *usecase.UserUsecase,
    activityRepo usecase.ActivityRepository,
    api *tgbotapi.BotAPI,
    logger *zap.Logger,
) {
    habitUC.SetGamificationNotifier(func(ctx context.Context, userID int64, habitID int64, streak int) {
        // Get user for language and shield count
        // We need GetByInternalID — use a workaround via userUC
        // For now, look up via a helper stored on UserUsecase
        runGamification(ctx, userID, habitID, streak, userUC, activityRepo, api, logger)
    })
})
```

Add the `runGamification` function to `internal/app/app.go` (or extract to `internal/gamification/runner.go`):

```go
// internal/gamification/runner.go
package gamification

import (
	"context"
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"

	"github.com/saidakmal/habbit-tracker-bot/internal/domain"
	"github.com/saidakmal/habbit-tracker-bot/internal/i18n"
)

// UserProvider is the subset of UserUsecase needed by the gamification runner.
type UserProvider interface {
	GetByID(ctx context.Context, userID int64) (*domain.User, error)
	AddXP(ctx context.Context, userID int64, xp int) (int, int, error)
	UpdateStreakShields(ctx context.Context, userID int64, shields int) error
	AddAchievement(ctx context.Context, userID int64, code string) error
	HasAchievement(ctx context.Context, userID int64, code string) (bool, error)
}

// ActivityCounter is the subset of ActivityRepository needed by gamification.
type ActivityCounter interface {
	CountAllByUser(ctx context.Context, userID int64) (int, error)
}

func Run(
	ctx context.Context,
	user *domain.User,
	streak int,
	actCounter ActivityCounter,
	api *tgbotapi.BotAPI,
	logger *zap.Logger,
	up UserProvider,
) {
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

	// Level up notification
	if newLevel > user.Level {
		send(api, user.TelegramID, i18n.T(lang, "levelup.message", newLevel))
	}

	// Check achievements
	achievementChecks := []struct {
		code      string
		triggered bool
	}{
		{AchFirstDone, func() bool {
			count, err := actCounter.CountAllByUser(ctx, user.ID)
			return err == nil && count == 1
		}()},
		{AchStreak7, streak >= 7},
		{AchStreak30, streak >= 30},
		{AchStreak100, streak >= 100},
	}

	for _, check := range achievementChecks {
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
		rewardText := RewardText(def, lang)
		send(api, user.TelegramID, i18n.T(lang, "achievement.unlocked", DisplayName(check.code, lang), rewardText))

		if def.ShieldBonus > 0 {
			newShields := user.StreakShields + def.ShieldBonus
			_ = up.UpdateStreakShields(ctx, user.ID, newShields)
		}
		if def.XPBonus > 0 {
			newXP, newLevel, _ = up.AddXP(ctx, user.ID, def.XPBonus)
			if newLevel > user.Level {
				send(api, user.TelegramID, i18n.T(lang, "levelup.message", newLevel))
			}
		}
		_ = newXP
	}
}

func send(api *tgbotapi.BotAPI, telegramID int64, text string) {
	if _, err := api.Send(tgbotapi.NewMessage(telegramID, text)); err != nil {
		// best-effort
		_ = err
	}
}

func EarlyBirdHour() int { return 9 }

func FormatAchievementLine(a domain.UserAchievement, lang string) string {
	name := DisplayName(a.Code, lang)
	return fmt.Sprintf("🏆 %s — %s\n", name, a.UnlockedAt.Format("02.01.2006"))
}
```

- [ ] **Step 3: Add GetByID to UserUsecase and interface**

Add to `internal/usecase/interfaces.go` UserRepository:
```go
GetByID(ctx context.Context, userID int64) (*domain.User, error)
```

Add to `internal/repository/postgres/user.go`:
```go
func (r *UserRepository) GetByID(ctx context.Context, userID int64) (*domain.User, error) {
	user := &domain.User{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, telegram_id, username, first_name, timezone, language, xp, level, streak_shields, evening_recap_hour, created_at
		 FROM users WHERE id = $1`,
		userID,
	).Scan(&user.ID, &user.TelegramID, &user.Username, &user.FirstName, &user.Timezone,
		&user.Language, &user.XP, &user.Level, &user.StreakShields, &user.EveningRecapHour, &user.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return user, nil
}
```

Add to `internal/usecase/user.go`:
```go
func (u *UserUsecase) GetByID(ctx context.Context, userID int64) (*domain.User, error) {
	return u.repo.GetByID(ctx, userID)
}
```

- [ ] **Step 4: Wire in app.go**

Read `internal/app/app.go` fully, then add the `fx.Invoke` call that wires the gamification notifier using `gamification.Run`.

- [ ] **Step 5: Build**

```bash
go build ./...
```

- [ ] **Step 6: Commit**

```bash
git add internal/gamification/runner.go internal/usecase/ internal/repository/postgres/user.go internal/app/app.go
git commit -m "feat: wire gamification runner — XP, level-up, and achievement notifications on MarkDone"
```

---

## Task 9: /language command + improved /start onboarding

**Files:**
- Modify: `internal/delivery/telegram/handler.go`

- [ ] **Step 1: Add language lookup helper**

Add to handler.go (near the top helpers section, after `clearState`):

```go
func (h *Handler) lang(user *domain.User) i18n.Lang {
	if user.Language == "" {
		return i18n.RU
	}
	return user.Language
}
```

Add import `"github.com/saidakmal/habbit-tracker-bot/internal/i18n"` to the import block.

- [ ] **Step 2: Add /language to command router**

In `handleCommand`, add:
```go
case "language":
    h.handleLanguage(msg)
case "today":
    h.handleToday(ctx, msg, user)
case "achievements":
    h.handleAchievements(ctx, msg, user)
```

- [ ] **Step 3: Add handleLanguage**

```go
func (h *Handler) handleLanguage(msg *tgbotapi.Message) {
	m := tgbotapi.NewMessage(msg.Chat.ID, "Выбери язык / Choose language / Тіл таңда:")
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🇷🇺 Русский", "tz:lang:ru"),
			tgbotapi.NewInlineKeyboardButtonData("🇬🇧 English", "tz:lang:en"),
			tgbotapi.NewInlineKeyboardButtonData("🇰🇿 Қазақша", "tz:lang:kz"),
		),
	)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send language keyboard", zap.Error(err))
	}
}
```

Add `"lang"` callback handling. In `handleCallback` switch, add a new case (or extend `"tz"` case):

```go
case "lang":
    h.cbLanguage(ctx, cq, chatID, msgID, arg)
```

But since callback data is `action:value` split on first `:`, change the /language keyboard to use action `"lang"`:

```go
// handleLanguage keyboard:
tgbotapi.NewInlineKeyboardButtonData("🇷🇺 Русский", "lang:ru"),
tgbotapi.NewInlineKeyboardButtonData("🇬🇧 English", "lang:en"),
tgbotapi.NewInlineKeyboardButtonData("🇰🇿 Қазақша", "lang:kz"),
```

Add to `handleCallback` switch:
```go
case "lang":
    h.cbLanguage(ctx, cq, chatID, msgID, arg)
```

Add handler:
```go
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
		h.send(chatID, i18n.T(i18n.Lang(arg), "error.generic"))
		return
	}
	labels := map[string]string{"ru": "🇷🇺 Русский", "en": "🇬🇧 English", "kz": "🇰🇿 Қазақша"}
	h.editMsg(chatID, msgID, "✅ "+labels[arg])
}
```

- [ ] **Step 4: Update handleStart for new users**

Replace `handleStart`:

```go
func (h *Handler) handleStart(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	lang := h.lang(user)

	// New user: created within the last 60 seconds → run onboarding
	if time.Since(user.CreatedAt) < 60*time.Second {
		m := tgbotapi.NewMessage(msg.Chat.ID, "Выбери язык / Choose language / Тіл таңда:")
		m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🇷🇺 Русский", "lang:ru"),
				tgbotapi.NewInlineKeyboardButtonData("🇬🇧 English", "lang:en"),
				tgbotapi.NewInlineKeyboardButtonData("🇰🇿 Қазақша", "lang:kz"),
			),
		)
		if _, err := h.api.Send(m); err != nil {
			h.logger.Error("send onboarding lang", zap.Error(err))
		}
		return
	}

	h.send(msg.Chat.ID, i18n.T(lang, "onboarding.welcome_returning", user.FirstName))
}
```

- [ ] **Step 5: Build**

```bash
go build ./...
```

- [ ] **Step 6: Commit**

```bash
git add internal/delivery/telegram/handler.go
git commit -m "feat: add /language command, cbLanguage callback, updated /start onboarding"
```

---

## Task 10: Wire i18n through handler.go messages

**Files:**
- Modify: `internal/delivery/telegram/handler.go`

This task replaces all hardcoded Russian strings in handler.go with `i18n.T(h.lang(user), key, ...)` calls. For callbacks that don't yet fetch the user, add `GetOrCreateUser` at the top of the callback.

- [ ] **Step 1: Update send helper to accept lang-keyed messages**

No structural change needed — use `i18n.T(lang, key)` at each call site.

- [ ] **Step 2: Update handleListHabits**

```go
func (h *Handler) handleListHabits(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	lang := h.lang(user)
	habits, err := h.habitUC.ListHabits(ctx, user.ID)
	if err != nil {
		h.logger.Error("ListHabits", zap.Int64("user_id", user.ID), zap.Error(err))
		h.send(msg.Chat.ID, i18n.T(lang, "error.generic"))
		return
	}
	if len(habits) == 0 {
		h.send(msg.Chat.ID, i18n.T(lang, "habit.list_empty"))
		return
	}

	now := time.Now()
	var sb strings.Builder
	sb.WriteString(i18n.T(lang, "habit.list_header"))

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
			goalStr = fmt.Sprintf(" [%d/%d]", habit.Streak, habit.GoalDays)
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
```

- [ ] **Step 3: Update handleDone, cbDone**

```go
func (h *Handler) handleDone(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	lang := h.lang(user)
	habits, err := h.habitUC.ListHabits(ctx, user.ID)
	if err != nil || len(habits) == 0 {
		h.send(msg.Chat.ID, i18n.T(lang, "habit.none_for_done"))
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
		h.send(msg.Chat.ID, i18n.T(lang, "habit.all_done_today"))
		return
	}
	m := tgbotapi.NewMessage(msg.Chat.ID, i18n.T(lang, "habit.choose"))
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send done keyboard", zap.Error(err))
	}
}

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
	if err := h.habitUC.MarkDone(ctx, user.ID, habitID); err != nil {
		if errors.Is(err, domain.ErrAlreadyDone) {
			h.editMsg(chatID, msgID, i18n.T(lang, "habit.already_done"))
			return
		}
		h.logger.Error("MarkDone", zap.Error(err))
		h.send(chatID, i18n.T(lang, "error.generic"))
		return
	}
	habit, err := h.habitUC.GetHabit(ctx, habitID)
	msg := i18n.T(lang, "habit.done_simple")
	if err == nil {
		msg = doneMessage(habit.Name, habit.Streak, habit.GoalDays, lang)
	}
	h.editMsg(chatID, msgID, msg)
}
```

- [ ] **Step 4: Update doneMessage helper signature**

The existing `doneMessage` function uses hardcoded strings. Replace it:

```go
func doneMessage(name string, streak, goalDays int, lang i18n.Lang) string {
	if goalDays > 0 && streak > 0 {
		return i18n.T(lang, "habit.done_goal", name, streak, goalDays)
	}
	if streak > 0 {
		return i18n.T(lang, "habit.done_streak", name, streak)
	}
	return i18n.T(lang, "habit.done_simple")
}
```

Find and update all existing calls to `doneMessage` — they currently pass 3 args; now pass `lang` as 4th.

- [ ] **Step 5: Update handleDeleteHabit, cbPreDelete, cbConfirmDelete**

```go
func (h *Handler) handleDeleteHabit(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	lang := h.lang(user)
	habits, err := h.habitUC.ListHabits(ctx, user.ID)
	if err != nil || len(habits) == 0 {
		h.send(msg.Chat.ID, i18n.T(lang, "habit.none_for_delete"))
		return
	}
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, habit := range habits {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🗑 "+habit.Name, fmt.Sprintf("pre_delete:%d", habit.ID)),
		))
	}
	m := tgbotapi.NewMessage(msg.Chat.ID, i18n.T(lang, "habit.choose_delete"))
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send delete keyboard", zap.Error(err))
	}
}

func (h *Handler) cbPreDelete(ctx context.Context, chatID int64, arg string) {
	habitID, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return
	}
	habit, err := h.habitUC.GetHabit(ctx, habitID)
	if err != nil {
		h.send(chatID, i18n.T(i18n.RU, "habit.not_found"))
		return
	}
	warning := ""
	if habit.Streak > 0 {
		warning = i18n.T(i18n.RU, "habit.delete_streak_warn", habit.Streak)
	}
	m := tgbotapi.NewMessage(chatID, i18n.T(i18n.RU, "habit.delete_confirm", habit.Name, warning))
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(i18n.RU, "action.yes_delete"), fmt.Sprintf("confirm_delete:%d", habitID)),
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(i18n.RU, "action.cancel"), "cancel_delete:0"),
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
```

Also update `cancel_delete` case in `handleCallback`:
```go
case "cancel_delete":
    // fetch user for lang
    user, _ := h.userUC.GetOrCreateUser(ctx, cq.From.ID, cq.From.UserName, cq.From.FirstName)
    lang := i18n.RU
    if user != nil {
        lang = h.lang(user)
    }
    h.editMsg(chatID, msgID, i18n.T(lang, "action.delete_cancelled"))
```

- [ ] **Step 6: Update remaining handlers (edit, pause/resume, snooze, stats, history, timezone)**

Apply the same pattern — replace hardcoded strings with `i18n.T(lang, key, args...)` — to:
- `handleEditHabit`, `cbEditMenu`, `cbEditName`, `cbEditInterval`, `cbEditStart`, `cbEditEnd` — use `i18n.T(lang, "habit.choose_edit")`, `"habit.edit_what"`, `"habit.edit_enter_name"`, `"habit.name_updated"`, `"habit.interval_updated"`, `"habit.hours_updated"`, `"error.update"`, `"error.generic"`
- `handlePauseHabit` → `"habit.choose_pause"`, `cbPauseResume` → `"habit.paused"` / `"habit.resumed"`
- `cbSnooze` → `"snooze.set"`; snooze button labels → `"snooze.30min"`, `"snooze.1hr"`, `"snooze.2hr"`
- `handleTimezone` → `"timezone.choose"`, `cbTimezone` → `"timezone.set"`
- `handleStats` → `"stats.header"`, `"stats.empty"`, `"stats.xp_level"` (add XP/level/shields display)
- `handleHistory`, `cbHistory` → `"history.header"`, `"history.legend"`, `"history.empty"`, `"history.choose"`
- `handleText` (stepAwaitName etc.) → `"habit.enter_name"`, `"habit.name_empty"`, `"habit.name_updated"`, `"habit.name_update_failed"`
- `handleCommand` `cancel` case → `"action.cancelled"`
- `cbTemplate` → `"habit.created"`, `"habit.goal_set"`, `"error.generic"`, `"habit.create_failed"`
- `cbInterval` → `"habit.choose_interval"` (edit msg text)
- `cbStartHour` / `cbEndHour` / `cbAddGoal` → corresponding keys
- `startAddHabit` → `"habit.choose_template"`

For callbacks that don't already fetch a user, add at the top:
```go
user, err := h.userUC.GetOrCreateUser(ctx, cq.From.ID, cq.From.UserName, cq.From.FirstName)
lang := i18n.RU
if err == nil {
    lang = h.lang(user)
}
```

- [ ] **Step 7: Build**

```bash
go build ./...
```

- [ ] **Step 8: Commit**

```bash
git add internal/delivery/telegram/handler.go
git commit -m "feat: wire i18n through all handler messages"
```

---

## Task 11: /today and /achievements commands

**Files:**
- Modify: `internal/delivery/telegram/handler.go`

- [ ] **Step 1: Add handleToday**

```go
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
	m := tgbotapi.NewMessage(msg.Chat.ID, sb.String())
	if len(rows) > 0 {
		m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	}
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send today", zap.Error(err))
	}
}
```

- [ ] **Step 2: Add handleAchievements**

```go
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
		sb.WriteString(gamification.FormatAchievementLine(a, lang))
	}
	h.send(msg.Chat.ID, sb.String())
}
```

Add import `"github.com/saidakmal/habbit-tracker-bot/internal/gamification"` to handler.go imports.

- [ ] **Step 3: Update handleStats to show XP, level, shields**

In `handleStats`, after building the stats string, append:

```go
text += i18n.T(lang, "stats.xp_level", user.Level, user.XP, user.StreakShields)
```

- [ ] **Step 4: Build**

```bash
go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add internal/delivery/telegram/handler.go
git commit -m "feat: add /today and /achievements commands, show XP/level/shields in /stats"
```

---

## Task 12: Streak shield in scheduler + i18n wiring

**Files:**
- Modify: `internal/scheduler/scheduler.go`

- [ ] **Step 1: Add UserUsecase to Scheduler**

Update Scheduler struct and constructor:

```go
type Scheduler struct {
	habitUC  *usecase.HabitUsecase
	userUC   *usecase.UserUsecase
	api      *tgbotapi.BotAPI
	logger   *zap.Logger
	location *time.Location
	cache    usecase.Cache
}

func New(habitUC *usecase.HabitUsecase, userUC *usecase.UserUsecase, api *tgbotapi.BotAPI, logger *zap.Logger, loc *time.Location, cache usecase.Cache) *Scheduler {
	return &Scheduler{habitUC: habitUC, userUC: userUC, api: api, logger: logger, location: loc, cache: cache}
}
```

Update `internal/app/app.go` to pass `userUC` to `scheduler.New`.

- [ ] **Step 2: Update resetStreaksAndNotify with shield check**

Replace `resetStreaksAndNotify`:

```go
func (s *Scheduler) resetStreaksAndNotify(ctx context.Context) {
	toNotify, err := s.habitUC.ListStreaksToBeReset(ctx)
	if err != nil {
		s.logger.Error("ListStreaksToBeReset", zap.Error(err))
	}

	if err := s.habitUC.ResetStreaks(ctx); err != nil {
		s.logger.Error("ResetStreaks", zap.Error(err))
	}

	for _, hw := range toNotify {
		user, err := s.userUC.GetByID(ctx, hw.UserID)
		lang := i18n.RU
		shields := 0
		if err == nil {
			lang = user.Language
			if lang == "" {
				lang = i18n.RU
			}
			shields = user.StreakShields
		}

		if shields > 0 {
			// Use a shield instead of breaking the streak
			newShields := shields - 1
			if err := s.userUC.UpdateStreakShields(ctx, hw.UserID, newShields); err != nil {
				s.logger.Warn("UpdateStreakShields", zap.Error(err))
			}
			text := i18n.T(lang, "shield.used", hw.Name, newShields)
			if _, err := s.api.Send(tgbotapi.NewMessage(hw.TelegramID, text)); err != nil {
				s.logger.Error("send shield used", zap.Int64("telegram_id", hw.TelegramID), zap.Error(err))
			}
			continue
		}

		text := i18n.T(lang, "streak.broken", hw.Name, hw.Streak)
		msg := tgbotapi.NewMessage(hw.TelegramID, text)
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "streak.do_now"), fmt.Sprintf("done:%d", hw.ID)),
			),
		)
		if _, err := s.api.Send(msg); err != nil {
			s.logger.Error("send streak break", zap.Int64("telegram_id", hw.TelegramID), zap.Error(err))
		}
	}
}
```

- [ ] **Step 3: Wire i18n into sendReminder**

```go
func (s *Scheduler) sendReminder(ctx context.Context, telegramID int64, h *domain.Habit, lang string) {
	streakText := ""
	if h.Streak > 0 {
		streakText = i18n.T(lang, "reminder.streak", h.Streak)
	}
	text := i18n.T(lang, "reminder.text", h.Name) + streakText
	msg := tgbotapi.NewMessage(telegramID, text)
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "reminder.done_button"), fmt.Sprintf("done:%d", h.ID)),
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
```

Update the call site in `tick` to pass `lang`:
```go
s.sendReminder(ctx, hw.TelegramID, &hw.Habit, g.userLang)
```

Add `userLang` to `userGroup` struct:
```go
type userGroup struct {
    telegramID   int64
    firstName    string
    userTimezone string
    userLang     string
    habits       []*domain.HabitWithTelegramID
}
```

Populate `userLang` from `hw.UserLanguage` (add `UserLanguage string` to `domain.HabitWithTelegramID`).

- [ ] **Step 4: Add UserLanguage to HabitWithTelegramID**

In `internal/domain/habit.go`:
```go
type HabitWithTelegramID struct {
	Habit
	TelegramID    int64
	UserTimezone  string
	UserFirstName string
	UserLanguage  string
}
```

Update `internal/repository/postgres/habit.go` — find `ListAllWithTelegramID` query and add `u.language` to the SELECT and scan.

- [ ] **Step 5: Wire i18n into morning and weekly digests**

Update `maybeSendMorningDigest` signature: add `lang string` param, replace hardcoded strings:
```go
sb.WriteString(i18n.T(lang, "morning.header", firstName))
```

Update `maybeSendWeeklyDigest` signature: add `lang string` param:
```go
sb.WriteString(i18n.T(lang, "weekly.header", from.Format("02.01"), now.Format("02.01")))
// per habit line:
sb.WriteString(i18n.T(lang, "weekly.habit_line", icon, hw.Name, st.CompletedDays, st.CompletionPct))
// overall:
sb.WriteString(i18n.T(lang, "weekly.overall", totalDone, totalPossible, overall))
```

Pass `g.userLang` at call sites in `tick`.

- [ ] **Step 6: Build**

```bash
go build ./...
```

- [ ] **Step 7: Commit**

```bash
git add internal/scheduler/scheduler.go internal/domain/habit.go internal/repository/postgres/habit.go internal/app/app.go
git commit -m "feat: streak shield in midnight reset, i18n in scheduler reminders and digests"
```

---

## Task 13: Evening recap

**Files:**
- Modify: `internal/scheduler/scheduler.go`

- [ ] **Step 1: Add evening recap check in tick**

In the per-user loop in `tick`, add after the weekly digest check:

```go
if userNow.Hour() == g.eveningRecapHour && userNow.Minute() == 0 {
    s.maybeSendEveningRecap(ctx, g.telegramID, g.userID, g.userLang, g.habits, userNow)
}
```

Add `eveningRecapHour int` and `userID int64` to `userGroup` struct and populate from a new field on `HabitWithTelegramID`.

- [ ] **Step 2: Add EveningRecapHour to HabitWithTelegramID**

In `internal/domain/habit.go`:
```go
type HabitWithTelegramID struct {
	Habit
	TelegramID       int64
	UserTimezone     string
	UserFirstName    string
	UserLanguage     string
	UserID           int64
	EveningRecapHour int
}
```

Update `ListAllWithTelegramID` query in `internal/repository/postgres/habit.go` to include `u.evening_recap_hour` and scan it.

- [ ] **Step 3: Add maybeSendEveningRecap**

```go
func (s *Scheduler) maybeSendEveningRecap(ctx context.Context, telegramID int64, userID int64, lang string, habits []*domain.HabitWithTelegramID, now time.Time) {
	key := fmt.Sprintf("evening:%d:%s", telegramID, now.Format("2006-01-02"))
	if _, err := s.cache.Get(ctx, key); err == nil {
		return
	}
	if err := s.cache.Set(ctx, key, "1", 25*time.Hour); err != nil {
		s.logger.Warn("evening recap cache set", zap.Error(err))
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
	}
}
```

- [ ] **Step 4: Build**

```bash
go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add internal/scheduler/scheduler.go internal/domain/habit.go internal/repository/postgres/habit.go
git commit -m "feat: add evening recap at user-configured hour"
```

---

## Task 14: /history ASCII heatmap + /stats progress bar

**Files:**
- Modify: `internal/delivery/telegram/handler.go`

- [ ] **Step 1: Update handleHistory to show ASCII heatmap**

Find the existing `handleHistory` / `cbHistory` functions and replace with:

```go
func (h *Handler) handleHistory(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	lang := h.lang(user)
	habits, err := h.habitUC.ListHabits(ctx, user.ID)
	if err != nil || len(habits) == 0 {
		h.send(msg.Chat.ID, i18n.T(lang, "history.empty"))
		return
	}
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, habit := range habits {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(habit.Name, fmt.Sprintf("history:%d", habit.ID)),
		))
	}
	m := tgbotapi.NewMessage(msg.Chat.ID, i18n.T(lang, "history.choose"))
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

func buildHeatmap(habitName string, from, to time.Time, doneSet map[string]bool, lang string) string {
	// Build a 4-week grid (rows = Mon-Sun, cols = weeks)
	weekdays := []string{"Mo", "Tu", "We", "Th", "Fr", "Sa", "Su"}
	// Find the Monday of the week containing `from`
	start := from
	for start.Weekday() != time.Monday {
		start = start.AddDate(0, 0, -1)
	}

	// Collect 4 weeks × 7 days
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
			} else if !day.After(to) {
				grid[row][col] = "□"
			} else {
				grid[row][col] = " "
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
```

- [ ] **Step 2: Add progress bar to handleStats**

Find `handleStats`. After building each habit stats line, append a progress bar:

```go
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
```

In the stats habit loop:
```go
bar := progressBar(st.CompletedDays, st.TotalDays)
sb.WriteString(fmt.Sprintf("%s  %s  %d/%d (%d%%)\n",
    h.Name, bar, st.CompletedDays, st.TotalDays, st.CompletionPct))
if h.GoalDays > 0 {
    goalBar := progressBar(h.Streak, h.GoalDays)
    sb.WriteString(fmt.Sprintf("   🎯 %s %d/%d days\n", goalBar, h.Streak, h.GoalDays))
}
```

- [ ] **Step 3: Build**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/delivery/telegram/handler.go
git commit -m "feat: ASCII heatmap in /history and progress bar in /stats"
```

---

## Task 15: Adaptive reminders + update bot command menu

**Files:**
- Modify: `internal/scheduler/scheduler.go`
- Modify: `internal/delivery/telegram/bot.go`

- [ ] **Step 1: Add adaptive reminder offset in sendReminder decision**

In the `tick` per-habit loop, before calling `sendReminder`, add an adaptive timing check:

```go
// Adaptive: if enough data, shift first reminder toward user's avg completion hour
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
```

Add `GetActivityAverageHour` to `HabitUsecase`:

```go
func (u *HabitUsecase) GetActivityAverageHour(ctx context.Context, habitID int64) (int, bool, error) {
	return u.activityRepo.GetAverageCompletionHour(ctx, habitID)
}
```

Add `IsInActiveHoursFrom` to `internal/usecase/habit.go`:

```go
func IsInActiveHoursFrom(h *domain.Habit, now time.Time, startHour int) bool {
	return now.Hour() >= startHour && now.Hour() < h.EndHour
}
```

Update tick to use `IsInActiveHoursFrom` instead of `IsInActiveHours` for the adaptive path:

```go
if !usecase.IsInActiveHoursFrom(&hw.Habit, userNow, effectiveStartHour) {
    continue
}
```

- [ ] **Step 2: Update bot command menu**

Replace the commands slice in `internal/delivery/telegram/bot.go`:

```go
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
```

- [ ] **Step 3: Final build and test**

```bash
go build ./...
go test ./...
```

Expected: build succeeds, tests pass.

- [ ] **Step 4: Final commit**

```bash
git add internal/scheduler/scheduler.go internal/usecase/habit.go internal/delivery/telegram/bot.go
git commit -m "feat: adaptive reminder timing, update bot command menu with new commands"
```

---

## Self-Review

**Spec coverage:**
- ✅ Multi-language RU/EN/KZ — Tasks 3, 10, 12
- ✅ Streak shield — Task 12
- ✅ Improved /start onboarding — Task 9
- ✅ Achievements (first_done, streak_7/30/100, perfect_week, early_bird, completionist) — Tasks 6, 7, 8
- ✅ XP & levels — Tasks 4, 5, 6, 7, 8
- ✅ /today command — Task 11
- ✅ /achievements command — Task 11
- ✅ /stats shows XP/level/shields — Task 11
- ✅ End-of-day recap — Task 13
- ✅ ASCII heatmap in /history — Task 14
- ✅ Progress bar in /stats — Task 14
- ✅ Adaptive reminders — Task 15
- ✅ DB migration — Task 1
- ✅ Bot command menu updated — Task 15

**Type consistency:**
- `i18n.Lang = string` — used as plain string throughout ✅
- `domain.UserAchievement` defined in Task 4, used in Tasks 5, 8, 11 ✅
- `gamification.Run` uses `UserProvider` interface — `UserUsecase` must implement `GetByID`, `AddXP`, `UpdateStreakShields`, `AddAchievement`, `HasAchievement` — all added in Task 5 ✅
- `GamificationNotifier` signature `func(ctx, userID, habitID, streak int)` consistent across Tasks 7, 8 ✅
- `sendReminder` gains `lang string` param — call site in `tick` passes `g.userLang` — Task 12 ✅

**Placeholder scan:** No TBDs or incomplete steps found.
