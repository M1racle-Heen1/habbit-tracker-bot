# Onboarding Simplification + Custom Habit Wizard Bug Fix — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Simplify new-user onboarding to 2 steps (language → template) and fix the custom habit wizard silently stalling after name entry.

**Architecture:** All changes live in `handler.go` (delivery layer) and the 3 i18n files. No new files, no schema changes. Keyboard sender functions gain `lang` and `error` return; a new `resendCurrentStep` helper powers a `default` case in `handleText` so stuck users always see the next step.

**Tech Stack:** Go, `github.com/go-telegram-bot-api/telegram-bot-api/v5`, Redis (via usecase.Cache), PostgreSQL (via usecase interfaces)

---

## File Map

| File | What changes |
|---|---|
| `internal/i18n/ru.go` | Add `onboarding.welcome_screen` key |
| `internal/i18n/en.go` | Add `onboarding.welcome_screen` key |
| `internal/i18n/kz.go` | Add `onboarding.welcome_screen` key |
| `internal/i18n/i18n_test.go` | Add test for new key |
| `internal/delivery/telegram/handler.go` | All logic changes (see tasks below) |

---

### Task 1: Add `onboarding.welcome_screen` i18n key

**Files:**
- Modify: `internal/i18n/ru.go`
- Modify: `internal/i18n/en.go`
- Modify: `internal/i18n/kz.go`
- Modify: `internal/i18n/i18n_test.go`

- [ ] **Step 1: Write failing test**

Add to `internal/i18n/i18n_test.go`:

```go
func TestWelcomeScreenKeyExistsAllLangs(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.RU, i18n.EN, i18n.KZ} {
		got := i18n.T(lang, "onboarding.welcome_screen")
		if got == "onboarding.welcome_screen" {
			t.Errorf("lang %s: key onboarding.welcome_screen is missing", lang)
		}
		if got == "" {
			t.Errorf("lang %s: key onboarding.welcome_screen is empty", lang)
		}
	}
}
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
go test ./internal/i18n/... -run TestWelcomeScreenKeyExistsAllLangs -v
```

Expected: FAIL — `lang ru: key onboarding.welcome_screen is missing`

- [ ] **Step 3: Add key to `ru.go`**

In `internal/i18n/ru.go`, add after the `"onboarding.add_later"` line:

```go
"onboarding.welcome_screen": "Я помогу тебе строить полезные привычки — напомню, отслежу прогресс и отмечу стрики.\n\nВыбери первую привычку:",
```

- [ ] **Step 4: Add key to `en.go`**

In `internal/i18n/en.go`, add after the `"onboarding.add_later"` line:

```go
"onboarding.welcome_screen": "I'll help you build good habits — send reminders, track progress, and celebrate streaks.\n\nPick your first habit:",
```

- [ ] **Step 5: Add key to `kz.go`**

In `internal/i18n/kz.go`, add after the `"onboarding.add_later"` line:

```go
"onboarding.welcome_screen": "Мен сізге пайдалы әдеттер қалыптастыруға көмектесемін.\n\nБірінші әдетті таңда:",
```

- [ ] **Step 6: Run test to confirm it passes**

```bash
go test ./internal/i18n/... -v
```

Expected: all tests PASS

- [ ] **Step 7: Commit**

```bash
git add internal/i18n/ru.go internal/i18n/en.go internal/i18n/kz.go internal/i18n/i18n_test.go
git commit -m "feat: add onboarding.welcome_screen i18n key (ru/en/kz)"
```

---

### Task 2: Make keyboard senders return `error` and use i18n, fix all callers

**Files:**
- Modify: `internal/delivery/telegram/handler.go`

This task changes 4 keyboard helper functions and 5 callers. Do them together — the build won't compile until all callers are updated.

- [ ] **Step 1: Replace `sendIntervalKeyboard`**

Find and replace the entire `sendIntervalKeyboard` function (currently at ~line 1499):

```go
func (h *Handler) sendIntervalKeyboard(chatID int64, lang i18n.Lang) error {
	m := tgbotapi.NewMessage(chatID, i18n.T(lang, "habit.choose_interval"))
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("30 мин", "interval:30"),
			tgbotapi.NewInlineKeyboardButtonData("1 час", "interval:60"),
			tgbotapi.NewInlineKeyboardButtonData("2 часа", "interval:120"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("3 часа", "interval:180"),
			tgbotapi.NewInlineKeyboardButtonData("Раз в день", "interval:1440"),
		),
	)
	_, err := h.api.Send(m)
	if err != nil {
		h.logger.Error("send interval keyboard", zap.Error(err))
	}
	return err
}
```

- [ ] **Step 2: Replace `sendStartHourKeyboard`**

Find and replace the entire `sendStartHourKeyboard` function:

```go
func (h *Handler) sendStartHourKeyboard(chatID int64, lang i18n.Lang) error {
	m := tgbotapi.NewMessage(chatID, i18n.T(lang, "habit.choose_start"))
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("5:00", "start_hour:5"),
			tgbotapi.NewInlineKeyboardButtonData("6:00", "start_hour:6"),
			tgbotapi.NewInlineKeyboardButtonData("7:00", "start_hour:7"),
			tgbotapi.NewInlineKeyboardButtonData("8:00", "start_hour:8"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("9:00", "start_hour:9"),
			tgbotapi.NewInlineKeyboardButtonData("10:00", "start_hour:10"),
			tgbotapi.NewInlineKeyboardButtonData("11:00", "start_hour:11"),
			tgbotapi.NewInlineKeyboardButtonData("12:00", "start_hour:12"),
		),
	)
	_, err := h.api.Send(m)
	if err != nil {
		h.logger.Error("send start hour keyboard", zap.Error(err))
	}
	return err
}
```

- [ ] **Step 3: Replace `sendEndHourKeyboard`**

Find and replace the entire `sendEndHourKeyboard` function (note: parameter order changes — `lang` added before `minHour`):

```go
func (h *Handler) sendEndHourKeyboard(chatID int64, lang i18n.Lang, minHour int) error {
	allHours := []int{14, 16, 18, 20, 21, 22, 23}
	var validHours []int
	for _, hr := range allHours {
		if hr > minHour {
			validHours = append(validHours, hr)
		}
	}
	if len(validHours) == 0 {
		h.send(chatID, i18n.T(lang, "error.generic"))
		h.clearState(chatID)
		return nil
	}
	var rows [][]tgbotapi.InlineKeyboardButton
	var row []tgbotapi.InlineKeyboardButton
	for _, hr := range validHours {
		row = append(row, tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("%d:00", hr), fmt.Sprintf("end_hour:%d", hr)))
		if len(row) == 4 {
			rows = append(rows, row)
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	m := tgbotapi.NewMessage(chatID, i18n.T(lang, "habit.choose_end"))
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	_, err := h.api.Send(m)
	if err != nil {
		h.logger.Error("send end hour keyboard", zap.Error(err))
	}
	return err
}
```

- [ ] **Step 4: Replace `sendGoalKeyboard`**

Find and replace the entire `sendGoalKeyboard` function:

```go
func (h *Handler) sendGoalKeyboard(chatID int64, lang i18n.Lang) error {
	m := tgbotapi.NewMessage(chatID, i18n.T(lang, "habit.choose_goal"))
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("21 день", "add_goal:21"),
			tgbotapi.NewInlineKeyboardButtonData("30 дней", "add_goal:30"),
			tgbotapi.NewInlineKeyboardButtonData("66 дней", "add_goal:66"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("100 дней", "add_goal:100"),
			tgbotapi.NewInlineKeyboardButtonData("Пропустить", "add_goal:0"),
		),
	)
	_, err := h.api.Send(m)
	if err != nil {
		h.logger.Error("send goal keyboard", zap.Error(err))
	}
	return err
}
```

- [ ] **Step 5: Update `handleText` — `stepAwaitName` case**

Replace the `stepAwaitName` case in `handleText`:

```go
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
```

- [ ] **Step 6: Update `cbTemplate` — custom path**

Replace the `if arg == "custom"` block inside `cbTemplate`:

```go
if arg == "custom" {
	user, err := h.userUC.GetOrCreateUser(ctx, cq.From.ID, cq.From.UserName, cq.From.FirstName)
	if err != nil {
		h.send(chatID, i18n.T(i18n.RU, "error.generic"))
		return
	}
	lang := h.lang(user)
	h.clearState(cq.From.ID)
	h.setState(cq.From.ID, &convState{Step: stepAwaitName})
	h.removeKeyboard(chatID, msgID)
	h.send(chatID, i18n.T(lang, "habit.enter_name"))
	return
}
```

- [ ] **Step 7: Update `cbInterval`**

Replace the entire `cbInterval` function:

```go
func (h *Handler) cbInterval(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, msgID int, arg string) {
	minutes, err := strconv.Atoi(arg)
	if err != nil {
		return
	}
	state := h.getState(cq.From.ID)
	if state == nil || state.Step != stepAwaitInterval {
		h.removeKeyboard(chatID, msgID)
		return
	}
	user, err := h.userUC.GetOrCreateUser(ctx, cq.From.ID, cq.From.UserName, cq.From.FirstName)
	if err != nil {
		h.send(chatID, i18n.T(i18n.RU, "error.generic"))
		return
	}
	lang := h.lang(user)
	state.IntervalMinutes = minutes
	state.Step = stepAwaitStartHour
	h.setState(cq.From.ID, state)
	h.editMsg(chatID, msgID, fmt.Sprintf("⏱ Интервал: %s ✓", formatInterval(minutes)))
	if err := h.sendStartHourKeyboard(chatID, lang); err != nil {
		h.clearState(cq.From.ID)
		h.send(chatID, i18n.T(lang, "error.generic"))
	}
}
```

- [ ] **Step 8: Update `cbStartHour`**

Replace the entire `cbStartHour` function:

```go
func (h *Handler) cbStartHour(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, msgID int, arg string) {
	hour, err := strconv.Atoi(arg)
	if err != nil {
		return
	}
	state := h.getState(cq.From.ID)
	if state == nil || state.Step != stepAwaitStartHour {
		h.removeKeyboard(chatID, msgID)
		return
	}
	user, err := h.userUC.GetOrCreateUser(ctx, cq.From.ID, cq.From.UserName, cq.From.FirstName)
	if err != nil {
		h.send(chatID, i18n.T(i18n.RU, "error.generic"))
		return
	}
	lang := h.lang(user)
	state.StartHour = hour
	state.Step = stepAwaitEndHour
	h.setState(cq.From.ID, state)
	h.editMsg(chatID, msgID, fmt.Sprintf("🕐 Начало: %d:00 ✓", hour))
	if err := h.sendEndHourKeyboard(chatID, lang, hour+1); err != nil {
		h.clearState(cq.From.ID)
		h.send(chatID, i18n.T(lang, "error.generic"))
	}
}
```

- [ ] **Step 9: Update `cbEndHour`**

Replace the entire `cbEndHour` function:

```go
func (h *Handler) cbEndHour(ctx context.Context, cq *tgbotapi.CallbackQuery, chatID int64, msgID int, arg string) {
	endHour, err := strconv.Atoi(arg)
	if err != nil {
		return
	}
	state := h.getState(cq.From.ID)
	if state == nil || state.Step != stepAwaitEndHour || state.HabitName == "" {
		h.removeKeyboard(chatID, msgID)
		return
	}
	user, err := h.userUC.GetOrCreateUser(ctx, cq.From.ID, cq.From.UserName, cq.From.FirstName)
	if err != nil {
		h.send(chatID, i18n.T(i18n.RU, "error.generic"))
		return
	}
	lang := h.lang(user)
	h.editMsg(chatID, msgID, fmt.Sprintf("🕕 Конец: %d:00 ✓", endHour))
	h.setState(cq.From.ID, &convState{
		Step:            stepAwaitGoal,
		HabitName:       state.HabitName,
		IntervalMinutes: state.IntervalMinutes,
		StartHour:       state.StartHour,
		EditHabitID:     int64(endHour),
	})
	if err := h.sendGoalKeyboard(chatID, lang); err != nil {
		h.clearState(cq.From.ID)
		h.send(chatID, i18n.T(lang, "error.generic"))
	}
}
```

- [ ] **Step 10: Verify build compiles**

```bash
go build ./...
```

Expected: no errors

- [ ] **Step 11: Run tests**

```bash
go test ./...
```

Expected: PASS

- [ ] **Step 12: Commit**

```bash
git add internal/delivery/telegram/handler.go
git commit -m "fix: keyboard senders return error + use i18n; all callers handle failure"
```

---

### Task 3: Add `resendCurrentStep` helper + `handleText` default case

**Files:**
- Modify: `internal/delivery/telegram/handler.go`

- [ ] **Step 1: Add `resendCurrentStep` method**

Add this function before the `send` helper at the bottom of `handler.go`:

```go
func (h *Handler) resendCurrentStep(chatID int64, lang i18n.Lang, state *convState) error {
	switch state.Step {
	case stepAwaitInterval:
		return h.sendIntervalKeyboard(chatID, lang)
	case stepAwaitStartHour:
		return h.sendStartHourKeyboard(chatID, lang)
	case stepAwaitEndHour:
		return h.sendEndHourKeyboard(chatID, lang, state.StartHour+1)
	case stepAwaitGoal:
		return h.sendGoalKeyboard(chatID, lang)
	default:
		return nil
	}
}
```

- [ ] **Step 2: Add `default` case to `handleText`**

In the `switch state.Step` block inside `handleText`, add a `default` case after the existing `case stepEditAwaitName:` block:

```go
	default:
		if err := h.resendCurrentStep(msg.Chat.ID, h.lang(user), state); err != nil {
			h.clearState(msg.From.ID)
			h.send(msg.Chat.ID, i18n.T(h.lang(user), "error.generic"))
		}
```

The full `handleText` switch should now look like:

```go
switch state.Step {
case stepAwaitName:
	// ... (already updated in Task 2)
case stepEditAwaitName:
	// ... (unchanged)
default:
	if err := h.resendCurrentStep(msg.Chat.ID, h.lang(user), state); err != nil {
		h.clearState(msg.From.ID)
		h.send(msg.Chat.ID, i18n.T(h.lang(user), "error.generic"))
	}
}
```

- [ ] **Step 3: Build and test**

```bash
go build ./... && go test ./...
```

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/delivery/telegram/handler.go
git commit -m "fix: add resendCurrentStep + handleText default — stuck users see the missing keyboard"
```

---

### Task 4: Simplify `handleStart` — change `isNew` detection

**Files:**
- Modify: `internal/delivery/telegram/handler.go`

- [ ] **Step 1: Update `handleStart`**

In `handleStart`, replace:

```go
isNew := time.Since(user.CreatedAt) < 60*time.Second
if isNew {
	h.setState(msg.From.ID, &convState{Step: stepOnboardTimezone})
	m := tgbotapi.NewMessage(msg.Chat.ID, i18n.T(lang, "language.choose"))
```

With:

```go
isNew := user.Language == ""
if isNew {
	m := tgbotapi.NewMessage(msg.Chat.ID, i18n.T(i18n.RU, "language.choose"))
```

The rest of the `if isNew` block stays the same (show language picker, return). The `h.setState(...)` line is removed — no state is needed during onboarding.

- [ ] **Step 2: Build**

```bash
go build ./...
```

Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add internal/delivery/telegram/handler.go
git commit -m "fix: detect new users via Language==\"\" instead of 60s creation window"
```

---

### Task 5: Simplify `cbLanguage` — onboarding path sets timezone + shows welcome

**Files:**
- Modify: `internal/delivery/telegram/handler.go`

- [ ] **Step 1: Replace `cbLanguage`**

Replace the entire `cbLanguage` function with:

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
	isOnboarding := user.Language == ""
	if err := h.userUC.SetLanguage(ctx, user.ID, arg); err != nil {
		h.logger.Error("SetLanguage", zap.Error(err))
		h.send(chatID, i18n.T(arg, "error.generic"))
		return
	}
	labels := map[string]string{"ru": "🇷🇺 Русский", "en": "🇬🇧 English", "kz": "🇰🇿 Қазақша"}
	h.editMsg(chatID, msgID, "✅ "+labels[arg])

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
}
```

- [ ] **Step 2: Build and test**

```bash
go build ./... && go test ./...
```

Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/delivery/telegram/handler.go
git commit -m "feat: simplify onboarding — language pick sets Almaty timezone and shows welcome + template"
```

---

### Task 6: Remove dead onboarding code

**Files:**
- Modify: `internal/delivery/telegram/handler.go`

- [ ] **Step 1: Remove step constants**

In the `step` iota block, remove `stepOnboardTimezone` and `stepOnboardHabit`:

```go
const (
	stepIdle step = iota
	stepAwaitName
	stepAwaitInterval
	stepAwaitStartHour
	stepAwaitEndHour
	stepAwaitGoal
	stepEditAwaitName
	stepEditAwaitEndHour
)
```

- [ ] **Step 2: Remove `sendOnboardTimezone` function**

Delete the entire `sendOnboardTimezone` function (currently after `cbLanguage`).

- [ ] **Step 3: Remove `cbOnboardTimezone` and `cbOnboardHabit` functions**

Delete both functions in their entirety (currently near line 1433–1478).

- [ ] **Step 4: Remove dead cases from `handleCallback`**

In the `handleCallback` switch, remove:

```go
case "tz_ob":
	h.cbOnboardTimezone(ctx, cq, chatID, msgID, arg)
case "onboard_habit":
	h.cbOnboardHabit(ctx, cq, chatID, msgID, arg)
```

- [ ] **Step 5: Build**

```bash
go build ./...
```

Expected: no errors. If there are "undefined" errors, a reference to a removed function was missed — search for `cbOnboardTimezone`, `cbOnboardHabit`, `sendOnboardTimezone`, `stepOnboardTimezone`, `stepOnboardHabit` and remove any remaining references.

- [ ] **Step 6: Run all tests**

```bash
go test ./...
```

Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/delivery/telegram/handler.go
git commit -m "refactor: remove stepOnboardTimezone/Habit, cbOnboardTimezone/Habit, sendOnboardTimezone"
```

---

### Task 7: Final verification

- [ ] **Step 1: Full build + test**

```bash
go build ./... && go test ./...
```

Expected: PASS

- [ ] **Step 2: Grep for leftover hardcoded Russian strings in the wizard flow**

```bash
grep -n "Введи название\|Как часто\|Во сколько\|До какого\|Установить цель\|Название не может" internal/delivery/telegram/handler.go
```

Expected: no output (all replaced with i18n calls)

- [ ] **Step 3: Grep for removed symbols to confirm nothing references them**

```bash
grep -n "stepOnboardTimezone\|stepOnboardHabit\|cbOnboardTimezone\|cbOnboardHabit\|sendOnboardTimezone" internal/delivery/telegram/handler.go
```

Expected: no output

- [ ] **Step 4: Final commit if any cleanup needed, then done**

```bash
git log --oneline -6
```

Expected: 6 commits since the spec commit covering all tasks above.
