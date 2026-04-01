# Main Navigation & UX Redesign — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a persistent reply keyboard for main navigation, simplify the add-habit wizard to 2 steps, consolidate habit management into an inline action menu, add a Settings screen, and fix the onboarding timezone step.

**Architecture:** All changes live in `internal/delivery/telegram/` and `internal/i18n/`. New i18n keys are added first so later tasks can reference them. Keyboard helpers are centralized in `keyboards.go`. Handler logic is split across `commands.go`, `callbacks.go`, and `handler.go` following the existing pattern.

**Tech Stack:** Go, go-telegram-bot-api/v5, Redis (conversation state), i18n package.

---

## File Map

| File | Changes |
|------|---------|
| `internal/i18n/en.go` | Add nav, settings, habit-menu, onboarding keys |
| `internal/i18n/ru.go` | Same |
| `internal/i18n/kz.go` | Same |
| `internal/i18n/i18n_test.go` | Add key-coverage test |
| `internal/delivery/telegram/keyboards.go` | Add `mainNavKeyboard`, `sendMainNav`, `sendTimezoneKeyboard`; refactor `handleTimezone` |
| `internal/delivery/telegram/handler.go` | Add nav routing in `handleText`; add `habit_menu`, `tz_ob`, `settings`, `onboard_skip` to `handleCallback` switch |
| `internal/delivery/telegram/commands.go` | Update `handleStart` (returning users); add `handleSettings`; update `handleListHabits` |
| `internal/delivery/telegram/callbacks.go` | Update `cbTemplate` (add nav after creation); update `cbLanguage` (onboarding tz step); add `cbTimezoneOnboard`, `cbHabitMenu`, `cbSettings`, `cbOnboardSkip` |
| `internal/delivery/telegram/bot.go` | Trim command menu |

---

## Task 1: Add i18n keys (all three languages) and test coverage

**Files:**
- Modify: `internal/i18n/en.go`
- Modify: `internal/i18n/ru.go`
- Modify: `internal/i18n/kz.go`
- Modify: `internal/i18n/i18n_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/i18n/i18n_test.go` after the existing tests:

```go
func TestNewNavKeysExistAllLangs(t *testing.T) {
	keys := []string{
		"nav.today", "nav.my_habits", "nav.add_habit", "nav.stats", "nav.settings",
		"nav.menu_hint",
		"habit.created_with_defaults",
		"settings.header", "settings.lang_btn", "settings.tz_btn",
		"onboarding.skip_btn",
		"habit.pause_btn", "habit.resume_btn", "habit.done_btn", "habit.delete_btn",
		"history.btn",
	}
	for _, key := range keys {
		for _, lang := range []Lang{RU, EN, KZ} {
			got := T(lang, key)
			if got == key {
				t.Errorf("lang %s: key %q is missing", lang, key)
			}
			if got == "" {
				t.Errorf("lang %s: key %q is empty", lang, key)
			}
		}
	}
}
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
go test ./internal/i18n/... -run TestNewNavKeysExistAllLangs -v
```

Expected: FAIL — keys missing in all languages.

- [ ] **Step 3: Add keys to `internal/i18n/en.go`**

Add inside `enMessages` (before the closing `}`):

```go
	"nav.today":       "📋 Today",
	"nav.my_habits":   "🗂 My Habits",
	"nav.add_habit":   "➕ Add Habit",
	"nav.stats":       "📊 Stats",
	"nav.settings":    "⚙️ Settings",
	"nav.menu_hint":   "Use the buttons below to navigate 👇",
	"habit.created_with_defaults": "✅ «%s» created!\n⏰ Reminders every 2h, 8:00–22:00.\n\nWant to adjust? Open 🗂 My Habits → tap the habit → ✏️ Edit.",
	"settings.header":   "⚙️ Settings",
	"settings.lang_btn": "🌐 Language",
	"settings.tz_btn":   "🕐 Timezone",
	"onboarding.skip_btn": "⏭ Later",
	"habit.pause_btn":   "⏸ Pause",
	"habit.resume_btn":  "▶️ Resume",
	"habit.done_btn":    "✅ Mark Done",
	"habit.delete_btn":  "🗑 Delete",
	"history.btn":       "📅 History",
```

- [ ] **Step 4: Add keys to `internal/i18n/ru.go`**

Add inside `ruMessages`:

```go
	"nav.today":       "📋 Сегодня",
	"nav.my_habits":   "🗂 Мои привычки",
	"nav.add_habit":   "➕ Добавить привычку",
	"nav.stats":       "📊 Статистика",
	"nav.settings":    "⚙️ Настройки",
	"nav.menu_hint":   "Используй кнопки ниже для навигации 👇",
	"habit.created_with_defaults": "✅ Привычка «%s» добавлена!\n⏰ Напоминания каждые 2 ч, 8:00–22:00.\n\nХочешь изменить? Открой 🗂 Мои привычки → нажми на привычку → ✏️ Изменить.",
	"settings.header":   "⚙️ Настройки",
	"settings.lang_btn": "🌐 Язык",
	"settings.tz_btn":   "🕐 Часовой пояс",
	"onboarding.skip_btn": "⏭ Позже",
	"habit.pause_btn":   "⏸ Пауза",
	"habit.resume_btn":  "▶️ Возобновить",
	"habit.done_btn":    "✅ Выполнить",
	"habit.delete_btn":  "🗑 Удалить",
	"history.btn":       "📅 История",
```

- [ ] **Step 5: Add keys to `internal/i18n/kz.go`**

Add inside `kzMessages`:

```go
	"nav.today":       "📋 Бүгін",
	"nav.my_habits":   "🗂 Әдеттерім",
	"nav.add_habit":   "➕ Әдет қосу",
	"nav.stats":       "📊 Статистика",
	"nav.settings":    "⚙️ Баптаулар",
	"nav.menu_hint":   "Навигация үшін төмендегі батырмаларды пайдаланыңыз 👇",
	"habit.created_with_defaults": "✅ «%s» әдеті қосылды!\n⏰ Еске салу әр 2 сағатта, 8:00–22:00.\n\nӨзгерту керек пе? 🗂 Әдеттерім → әдетке басыңыз → ✏️ Өңдеу.",
	"settings.header":   "⚙️ Баптаулар",
	"settings.lang_btn": "🌐 Тіл",
	"settings.tz_btn":   "🕐 Уақыт белдеуі",
	"onboarding.skip_btn": "⏭ Кейінірек",
	"habit.pause_btn":   "⏸ Кідірту",
	"habit.resume_btn":  "▶️ Жалғастыру",
	"habit.done_btn":    "✅ Орындау",
	"habit.delete_btn":  "🗑 Жою",
	"history.btn":       "📅 Тарих",
```

- [ ] **Step 6: Run test to confirm it passes**

```bash
go test ./internal/i18n/... -run TestNewNavKeysExistAllLangs -v
```

Expected: PASS

- [ ] **Step 7: Run all i18n tests**

```bash
go test ./internal/i18n/... -v
```

Expected: all tests PASS

- [ ] **Step 8: Commit**

```bash
git add internal/i18n/en.go internal/i18n/ru.go internal/i18n/kz.go internal/i18n/i18n_test.go
git commit -m "feat: add i18n keys for main nav, settings, and habit menu"
```

---

## Task 2: Keyboard helpers — mainNavKeyboard, sendMainNav, sendTimezoneKeyboard

**Files:**
- Modify: `internal/delivery/telegram/keyboards.go`
- Modify: `internal/delivery/telegram/commands.go` (refactor `handleTimezone`)

- [ ] **Step 1: Add `mainNavKeyboard` and `sendMainNav` to `keyboards.go`**

Add after the `templateKeyboard()` function (around line 29):

```go
func mainNavKeyboard(lang i18n.Lang) tgbotapi.ReplyKeyboardMarkup {
	kb := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(i18n.T(lang, "nav.today")),
			tgbotapi.NewKeyboardButton(i18n.T(lang, "nav.my_habits")),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(i18n.T(lang, "nav.add_habit")),
			tgbotapi.NewKeyboardButton(i18n.T(lang, "nav.stats")),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(i18n.T(lang, "nav.settings")),
		),
	)
	kb.ResizeKeyboard = true
	return kb
}

func (h *Handler) sendMainNav(chatID int64, lang i18n.Lang) {
	m := tgbotapi.NewMessage(chatID, i18n.T(lang, "nav.menu_hint"))
	m.ReplyMarkup = mainNavKeyboard(lang)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send main nav", zap.Error(err))
	}
}
```

- [ ] **Step 2: Add `sendTimezoneKeyboard` helper to `keyboards.go`**

Add after `sendMainNav` (still in keyboards.go):

```go
func (h *Handler) sendTimezoneKeyboard(chatID int64, lang i18n.Lang, callbackPrefix string) {
	var rows [][]tgbotapi.InlineKeyboardButton
	for i := 0; i < len(commonTimezones); i += 2 {
		row := []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(commonTimezones[i].Label, callbackPrefix+commonTimezones[i].Value),
		}
		if i+1 < len(commonTimezones) {
			row = append(row, tgbotapi.NewInlineKeyboardButtonData(commonTimezones[i+1].Label, callbackPrefix+commonTimezones[i+1].Value))
		}
		rows = append(rows, row)
	}
	m := tgbotapi.NewMessage(chatID, i18n.T(lang, "timezone.choose"))
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send timezone keyboard", zap.Error(err))
	}
}
```

- [ ] **Step 3: Refactor `handleTimezone` in `commands.go` to use `sendTimezoneKeyboard`**

Replace the existing `handleTimezone` function (lines 60–77 of commands.go) with:

```go
func (h *Handler) handleTimezone(msg *tgbotapi.Message, user *domain.User) {
	h.sendTimezoneKeyboard(msg.Chat.ID, h.lang(user), "tz:")
}
```

- [ ] **Step 4: Build to verify no compile errors**

```bash
go build ./...
```

Expected: no errors

- [ ] **Step 5: Commit**

```bash
git add internal/delivery/telegram/keyboards.go internal/delivery/telegram/commands.go
git commit -m "feat: add mainNavKeyboard, sendMainNav, sendTimezoneKeyboard helpers"
```

---

## Task 3: handleText — nav button routing

**Files:**
- Modify: `internal/delivery/telegram/handler.go`

The nav buttons send plain text messages (not commands). `handleText` must check these before the conversation state machine. Nav taps also clear any active wizard state.

- [ ] **Step 1: Replace the body of `handleText` in `handler.go`**

The current `handleText` starts at the line `func (h *Handler) handleText(...)`. Replace the entire function body with:

```go
func (h *Handler) handleText(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	lang := h.lang(user)
	switch msg.Text {
	case i18n.T(lang, "nav.today"):
		h.clearState(msg.From.ID)
		h.handleToday(ctx, msg, user)
		return
	case i18n.T(lang, "nav.my_habits"):
		h.clearState(msg.From.ID)
		h.handleListHabits(ctx, msg, user)
		return
	case i18n.T(lang, "nav.add_habit"):
		h.clearState(msg.From.ID)
		h.startAddHabit(msg, user)
		return
	case i18n.T(lang, "nav.stats"):
		h.clearState(msg.From.ID)
		h.handleStats(ctx, msg, user)
		return
	case i18n.T(lang, "nav.settings"):
		h.clearState(msg.From.ID)
		h.handleSettings(ctx, msg, user)
		return
	}

	state := h.getState(msg.From.ID)
	if state == nil {
		return
	}

	switch state.Step {
	case stepAwaitName:
		lang := h.lang(user)
		name := strings.TrimSpace(msg.Text)
		if name == "" {
			h.send(msg.Chat.ID, i18n.T(lang, "habit.name_empty"))
			return
		}
		h.clearState(msg.From.ID)
		habit, err := h.habitUC.CreateHabit(ctx, user.ID, name, 120, 8, 22, 0)
		if err != nil {
			h.logger.Error("CreateHabit custom", zap.Error(err))
			h.send(msg.Chat.ID, i18n.T(lang, "error.generic"))
			return
		}
		h.send(msg.Chat.ID, i18n.T(lang, "habit.created_with_defaults", habit.Name))
		h.sendMainNav(msg.Chat.ID, lang)

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
```

- [ ] **Step 2: Build to verify no compile errors**

```bash
go build ./...
```

Expected: no errors (note: `handleSettings` is not defined yet — if you get that error, define a stub in commands.go temporarily: `func (h *Handler) handleSettings(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {}`)

- [ ] **Step 3: Commit**

```bash
git add internal/delivery/telegram/handler.go
git commit -m "feat: route nav button taps in handleText; simplify add-habit to 2 steps"
```

---

## Task 4: handleStart — returning users get nav keyboard

**Files:**
- Modify: `internal/delivery/telegram/commands.go`
- Modify: `internal/i18n/en.go`, `ru.go`, `kz.go`

The welcome-back message no longer needs to list slash commands — the nav keyboard replaces that.

- [ ] **Step 1: Update `onboarding.welcome_returning` in all three language files**

In `internal/i18n/en.go`, replace:
```go
"onboarding.welcome_returning": "Hi, %s! 👋\n\nCommands:\n/add_habit — add a habit\n/list_habits — habits with progress\n/done — mark as done\n/today — today's habits\n/edit_habit — edit a habit\n/pause_habit — pause\n/resume_habit — resume\n/stats — statistics\n/history — completion history\n/timezone — set timezone\n/achievements — achievements\n/language — change language\n/delete_habit — delete a habit\n/cancel — cancel current action",
```
with:
```go
"onboarding.welcome_returning": "Welcome back, %s! 👋",
```

In `internal/i18n/ru.go`, replace:
```go
"onboarding.welcome_returning": "Привет, %s! 👋\n\nКоманды:\n/add_habit — добавить привычку\n/list_habits — список с прогрессом\n/done — отметить выполнение\n/today — привычки на сегодня\n/edit_habit — редактировать\n/pause_habit — пауза\n/resume_habit — снять с паузы\n/stats — статистика\n/history — история\n/timezone — часовой пояс\n/achievements — достижения\n/language — язык\n/delete_habit — удалить\n/cancel — отменить",
```
with:
```go
"onboarding.welcome_returning": "С возвращением, %s! 👋",
```

In `internal/i18n/kz.go`, replace:
```go
"onboarding.welcome_returning": "Сәлем, %s! 👋\n\nКомандалар:\n/add_habit — әдет қосу\n/list_habits — тізім\n/done — орындалды деп белгілеу\n/today — бүгінгі әдеттер\n/edit_habit — өңдеу\n/pause_habit — кідірту\n/resume_habit — жалғастыру\n/stats — статистика\n/history — тарих\n/timezone — уақыт белдеуі\n/achievements — жетістіктер\n/language — тіл\n/delete_habit — жою\n/cancel — болдырмау",
```
with:
```go
"onboarding.welcome_returning": "Қайтып келдіңіз, %s! 👋",
```

- [ ] **Step 2: Update `handleStart` in `commands.go` — returning users branch**

Replace the returning-users branch of `handleStart` (the `else` branch starting with `h.send(msg.Chat.ID, i18n.T(lang, "onboarding.welcome_returning"...)`):

```go
	// returning user with habits
	m := tgbotapi.NewMessage(msg.Chat.ID, i18n.T(lang, "onboarding.welcome_returning", user.FirstName))
	m.ReplyMarkup = mainNavKeyboard(lang)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send start returning", zap.Error(err))
	}
```

The full updated `handleStart` should look like:

```go
func (h *Handler) handleStart(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	lang := h.lang(user)

	isNew := user.Language == ""
	if isNew {
		m := tgbotapi.NewMessage(msg.Chat.ID, i18n.T(i18n.RU, "language.choose"))
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
		m := tgbotapi.NewMessage(msg.Chat.ID, i18n.T(lang, "onboarding.welcome_new", user.FirstName))
		m.ReplyMarkup = templateKeyboard()
		if _, err := h.api.Send(m); err != nil {
			h.logger.Error("send start", zap.Error(err))
		}
		return
	}

	m := tgbotapi.NewMessage(msg.Chat.ID, i18n.T(lang, "onboarding.welcome_returning", user.FirstName))
	m.ReplyMarkup = mainNavKeyboard(lang)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send start returning", zap.Error(err))
	}
}
```

- [ ] **Step 3: Build**

```bash
go build ./...
```

Expected: no errors

- [ ] **Step 4: Run all tests**

```bash
go test ./...
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/delivery/telegram/commands.go internal/i18n/en.go internal/i18n/ru.go internal/i18n/kz.go
git commit -m "feat: send nav keyboard to returning users on /start; simplify welcome-back message"
```

---

## Task 5: Settings screen

**Files:**
- Modify: `internal/delivery/telegram/commands.go` (add `handleSettings`)
- Modify: `internal/delivery/telegram/callbacks.go` (add `cbSettings`)
- Modify: `internal/delivery/telegram/handler.go` (add `settings` to callback router)

- [ ] **Step 1: Add `handleSettings` to `commands.go`**

Add at the end of `commands.go`:

```go
func (h *Handler) handleSettings(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	lang := h.lang(user)
	m := tgbotapi.NewMessage(msg.Chat.ID, i18n.T(lang, "settings.header"))
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "settings.lang_btn"), "settings:language"),
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "settings.tz_btn"), "settings:timezone"),
		),
	)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send settings", zap.Error(err))
	}
}
```

- [ ] **Step 2: Add `settings` case to `handleCommand` router in `handler.go`**

In the `switch msg.Command()` block, add:

```go
	case "settings":
		h.handleSettings(ctx, msg, user)
```

- [ ] **Step 3: Add `cbSettings` to `callbacks.go`**

Add at the end of `callbacks.go`:

```go
func (h *Handler) cbSettings(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, arg string) {
	_, lang, err := h.getUserFromCallback(ctx, cq)
	if err != nil {
		return
	}
	switch arg {
	case "language":
		m := tgbotapi.NewMessage(chatID, "Выбери язык / Choose language / Тіл таңда:")
		m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🇷🇺 Русский", "lang:ru"),
			tgbotapi.NewInlineKeyboardButtonData("🇬🇧 English", "lang:en"),
			tgbotapi.NewInlineKeyboardButtonData("🇰🇿 Қазақша", "lang:kz"),
		))
		if _, err := h.api.Send(m); err != nil {
			h.logger.Error("send language from settings", zap.Error(err))
		}
	case "timezone":
		h.sendTimezoneKeyboard(chatID, lang, "tz:")
	}
}
```

- [ ] **Step 4: Add `settings` case to `handleCallback` router in `handler.go`**

In the `switch action` block of `handleCallback`, add:

```go
	case "settings":
		h.cbSettings(ctx, cq, chatID, arg)
```

- [ ] **Step 5: Build**

```bash
go build ./...
```

Expected: no errors

- [ ] **Step 6: Commit**

```bash
git add internal/delivery/telegram/commands.go internal/delivery/telegram/callbacks.go internal/delivery/telegram/handler.go
git commit -m "feat: add Settings screen accessible from nav and /settings command"
```

---

## Task 6: Onboarding — restore timezone step

**Files:**
- Modify: `internal/delivery/telegram/callbacks.go` (update `cbLanguage`, add `cbTimezoneOnboard`, `cbOnboardSkip`)
- Modify: `internal/delivery/telegram/keyboards.go` (add `onboardTemplateKeyboard`)
- Modify: `internal/delivery/telegram/handler.go` (add `tz_ob`, `onboard_skip` to callback router)

- [ ] **Step 1: Add `onboardTemplateKeyboard` to `keyboards.go`**

Add after `templateKeyboard()`:

```go
func onboardTemplateKeyboard(lang i18n.Lang) tgbotapi.InlineKeyboardMarkup {
	base := templateKeyboard()
	base.InlineKeyboard = append(base.InlineKeyboard, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "onboarding.skip_btn"), "onboard_skip:1"),
	))
	return base
}
```

- [ ] **Step 2: Update the `isOnboarding` branch of `cbLanguage` in `callbacks.go`**

Find the `isOnboarding` block inside `cbLanguage`:

```go
	if isOnboarding {
		if err := h.userUC.SetTimezone(ctx, user.ID, "Asia/Almaty"); err != nil {
			h.logger.Error("SetTimezone onboard", zap.Error(err))
		}
		h.send(chatID, i18n.T(arg, "onboarding.welcome_screen"))
		m := tgbotapi.NewMessage(chatID, i18n.T(arg, "habit.choose_template"))
		m.ReplyMarkup = templateKeyboard()
		if _, err := h.api.Send(m); err != nil {
			h.logger.Error("send template keyboard onboard", zap.Error(err))
		}
	}
```

Replace it with:

```go
	if isOnboarding {
		h.sendTimezoneKeyboard(chatID, i18n.Lang(arg), "tz_ob:")
	}
```

- [ ] **Step 3: Add `cbTimezoneOnboard` to `callbacks.go`**

Add at the end of `callbacks.go`:

```go
func (h *Handler) cbTimezoneOnboard(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, msgID int, tz string) {
	if _, err := time.LoadLocation(tz); err != nil {
		h.send(chatID, i18n.T(i18n.RU, "timezone.invalid"))
		return
	}
	user, lang, err := h.getUserFromCallback(ctx, cq)
	if err != nil {
		return
	}
	if err := h.userUC.SetTimezone(ctx, user.ID, tz); err != nil {
		h.logger.Error("SetTimezone onboard", zap.Error(err))
		h.send(chatID, i18n.T(lang, "error.generic"))
		return
	}
	h.editMsg(chatID, msgID, i18n.T(lang, "timezone.set", tz))
	h.send(chatID, i18n.T(lang, "onboarding.welcome_screen"))
	m := tgbotapi.NewMessage(chatID, i18n.T(lang, "habit.choose_template"))
	m.ReplyMarkup = onboardTemplateKeyboard(lang)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send template keyboard onboard", zap.Error(err))
	}
}
```

- [ ] **Step 4: Add `cbOnboardSkip` to `callbacks.go`**

Add at the end of `callbacks.go`:

```go
func (h *Handler) cbOnboardSkip(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, msgID int) {
	_, lang, err := h.getUserFromCallback(ctx, cq)
	if err != nil {
		return
	}
	h.removeKeyboard(chatID, msgID)
	h.sendMainNav(chatID, lang)
}
```

- [ ] **Step 5: Add `tz_ob` and `onboard_skip` to callback router in `handler.go`**

In `handleCallback`'s `switch action` block, add:

```go
	case "tz_ob":
		h.cbTimezoneOnboard(ctx, cq, chatID, msgID, arg)
	case "onboard_skip":
		h.cbOnboardSkip(ctx, cq, chatID, msgID)
```

- [ ] **Step 6: Build**

```bash
go build ./...
```

Expected: no errors

- [ ] **Step 7: Commit**

```bash
git add internal/delivery/telegram/callbacks.go internal/delivery/telegram/keyboards.go internal/delivery/telegram/handler.go
git commit -m "feat: restore onboarding timezone step; add Later button to skip first habit"
```

---

## Task 7: Simplified add-habit wizard — send nav after template creation

**Files:**
- Modify: `internal/delivery/telegram/callbacks.go` (update `cbTemplate`)

The `stepAwaitName` → create-with-defaults path was already handled in Task 3. This task adds `sendMainNav` after template-path creation.

- [ ] **Step 1: Update `cbTemplate` to call `sendMainNav` after template habit creation**

In `cbTemplate`, find the end of the template-path success branch:

```go
	h.editMsg(chatID, msgID, i18n.T(lang, "habit.created",
		habit.Name, formatInterval(habit.IntervalMinutes, lang), habit.StartHour, habit.EndHour,
	))
```

Add `h.sendMainNav(chatID, lang)` immediately after:

```go
	h.editMsg(chatID, msgID, i18n.T(lang, "habit.created",
		habit.Name, formatInterval(habit.IntervalMinutes, lang), habit.StartHour, habit.EndHour,
	))
	h.sendMainNav(chatID, lang)
```

- [ ] **Step 2: Build**

```bash
go build ./...
```

Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add internal/delivery/telegram/callbacks.go
git commit -m "feat: show main nav keyboard after habit creation from template"
```

---

## Task 8: My Habits — ⚙️ buttons + habit action menu

**Files:**
- Modify: `internal/delivery/telegram/commands.go` (update `handleListHabits`)
- Modify: `internal/delivery/telegram/callbacks.go` (add `cbHabitMenu`)
- Modify: `internal/delivery/telegram/handler.go` (add `habit_menu` to callback router)

- [ ] **Step 1: Update `handleListHabits` in `commands.go`**

Replace the current `handleListHabits` function with:

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

	var habitRows [][]tgbotapi.InlineKeyboardButton
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
			goalStr = i18n.T(lang, "habit.goal_progress", habit.Streak, habit.GoalDays)
		}

		sb.WriteString(fmt.Sprintf("%s %s%s%s\n   %s, %d:00–%d:00\n\n",
			mark, habit.Name, streakStr, goalStr,
			formatInterval(habit.IntervalMinutes, lang), habit.StartHour, habit.EndHour,
		))

		habitRows = append(habitRows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⚙️ "+habit.Name, fmt.Sprintf("habit_menu:%d", habit.ID)),
		))
	}

	m := tgbotapi.NewMessage(msg.Chat.ID, sb.String())
	if len(habitRows) > 0 {
		m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(habitRows...)
	}
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send list", zap.Error(err))
	}
}
```

- [ ] **Step 2: Add `cbHabitMenu` to `callbacks.go`**

Add at the end of `callbacks.go`:

```go
func (h *Handler) cbHabitMenu(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, arg string) {
	habitID, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return
	}
	_, lang, err := h.getUserFromCallback(ctx, cq)
	if err != nil {
		return
	}
	habit, err := h.habitUC.GetHabit(ctx, habitID)
	if err != nil {
		h.send(chatID, i18n.T(lang, "habit.not_found"))
		return
	}

	pauseLabel := i18n.T(lang, "habit.pause_btn")
	pauseAction := fmt.Sprintf("pause:%d", habitID)
	if habit.IsPaused {
		pauseLabel = i18n.T(lang, "habit.resume_btn")
		pauseAction = fmt.Sprintf("resume:%d", habitID)
	}

	m := tgbotapi.NewMessage(chatID, habit.Name)
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "habit.done_btn"), fmt.Sprintf("done:%d", habitID)),
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "edit.name_btn"), fmt.Sprintf("edit:%d", habitID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(pauseLabel, pauseAction),
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "history.btn"), fmt.Sprintf("history:%d", habitID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "habit.delete_btn"), fmt.Sprintf("pre_delete:%d", habitID)),
		),
	)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send habit menu", zap.Error(err))
	}
}
```

- [ ] **Step 3: Add `habit_menu` to callback router in `handler.go`**

In `handleCallback`'s `switch action` block, add:

```go
	case "habit_menu":
		h.cbHabitMenu(ctx, cq, chatID, arg)
```

- [ ] **Step 4: Build**

```bash
go build ./...
```

Expected: no errors

- [ ] **Step 5: Run all tests**

```bash
go test ./...
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/delivery/telegram/commands.go internal/delivery/telegram/callbacks.go internal/delivery/telegram/handler.go
git commit -m "feat: add per-habit action menu in My Habits list"
```

---

## Task 9: bot.go — trim command menu

**Files:**
- Modify: `internal/delivery/telegram/bot.go`

Remove the management commands from the bot command menu. Users discover these via the nav keyboard and inline actions. Slash commands still work but don't clutter the menu.

- [ ] **Step 1: Update `SetMyCommands` in `bot.go`**

Replace the `commands` slice in `Start()` with:

```go
	commands := []tgbotapi.BotCommand{
		{Command: "start", Description: "Start / Начать"},
		{Command: "today", Description: "Today's habits / Привычки на сегодня"},
		{Command: "stats", Description: "Statistics / Статистика"},
		{Command: "achievements", Description: "Achievements / Достижения"},
		{Command: "settings", Description: "Settings / Настройки"},
		{Command: "cancel", Description: "Cancel current action / Отменить"},
	}
```

- [ ] **Step 2: Build**

```bash
go build ./...
```

Expected: no errors

- [ ] **Step 3: Run all tests**

```bash
go test ./...
```

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/delivery/telegram/bot.go
git commit -m "feat: trim bot command menu — management via nav and inline menus"
```

---

## Self-Review Checklist

### Spec coverage

| Spec requirement | Covered by |
|---|---|
| Persistent reply keyboard (5 buttons, 2+2+1) | Task 2 (mainNavKeyboard) |
| Nav routing in handleText | Task 3 |
| Nav keyboard sent to returning users | Task 4 |
| Simplified add-habit: template → instant, custom → name only | Task 3 (stepAwaitName) + Task 7 (cbTemplate) |
| Confirmation message with edit hint | Task 1 (habit.created_with_defaults key) + Task 3 |
| i18n for all new strings (ru/en/kz) | Task 1 |
| My Habits: ⚙️ per-habit button | Task 8 (handleListHabits) |
| Habit action menu (Done/Edit/Pause/History/Delete) | Task 8 (cbHabitMenu) |
| Settings screen | Task 5 |
| Onboarding timezone step restored | Task 6 |
| "Later" skip button in onboarding | Task 6 (cbOnboardSkip) |
| bot command menu trimmed | Task 9 |

### Type/signature consistency

- `mainNavKeyboard(lang i18n.Lang) tgbotapi.ReplyKeyboardMarkup` — used in Task 2, called in Task 4
- `sendMainNav(chatID int64, lang i18n.Lang)` — defined Task 2, called in Tasks 3, 6, 7
- `sendTimezoneKeyboard(chatID int64, lang i18n.Lang, callbackPrefix string)` — defined Task 2, called in Tasks 5 and 6
- `onboardTemplateKeyboard(lang i18n.Lang) tgbotapi.InlineKeyboardMarkup` — defined Task 6, called in Task 6
- `handleSettings(ctx, msg, user)` — defined Task 5, called in Tasks 3 and 5
- All callback handlers follow existing `(ctx, cq, chatID, [msgID,] arg)` signature pattern ✓
