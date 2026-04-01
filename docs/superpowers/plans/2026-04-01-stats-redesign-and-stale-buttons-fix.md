# Stats Redesign & Stale Buttons Fix — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix stale inline keyboard buttons across all definitive-action callbacks, and redesign `/stats` to show a summary header (Today / Week / Month) plus tappable per-habit rows.

**Architecture:** Two independent changes. (1) A new `editMsgAndClearMarkup` helper replaces bare `editMsg` calls at every site where a conversation is definitively concluded — the helper attaches an empty `InlineKeyboardMarkup` so old buttons stop being clickable. (2) `handleStats` is rewritten to make three usecase calls (`ListHabits`, `GetStats(7)`, `GetStats(30)`), aggregate the numbers, and render a summary + per-habit inline keyboard.

**Tech Stack:** Go, go-telegram-bot-api/v5, Redis-backed conversation state, i18n package (`i18n.T(lang, key, args...)`).

---

## File Map

| File | Change |
|------|--------|
| `internal/delivery/telegram/handler.go` | Add `editMsgAndClearMarkup` after `editMsg` (line ~138) |
| `internal/delivery/telegram/callbacks.go` | Replace `editMsg` → `editMsgAndClearMarkup` at 13 definitive-action sites; fix `cbDone` else-branch |
| `internal/delivery/telegram/commands.go` | Rewrite `handleStats` (currently lines 258–287) |
| `internal/i18n/en.go` | Add 5 stats keys |
| `internal/i18n/ru.go` | Add 5 stats keys |
| `internal/i18n/kz.go` | Add 5 stats keys |
| `internal/i18n/i18n_test.go` | Add test for the 5 new stats i18n keys |

---

## Task 1: Add `editMsgAndClearMarkup` helper

**Files:**
- Modify: `internal/delivery/telegram/handler.go` (after line 139, the closing `}` of `editMsg`)

- [ ] **Step 1: Add the helper function**

In `internal/delivery/telegram/handler.go`, insert immediately after the closing `}` of `editMsg` (currently ends around line 139):

```go
func (h *Handler) editMsgAndClearMarkup(chatID int64, messageID int, text string) {
	edit := tgbotapi.NewEditMessageText(chatID, messageID, text)
	empty := tgbotapi.NewInlineKeyboardMarkup()
	edit.ReplyMarkup = &empty
	if _, err := h.api.Request(edit); err != nil {
		h.logger.Warn("edit message and clear markup", zap.Error(err))
	}
}
```

- [ ] **Step 2: Verify the code compiles**

```bash
go build ./internal/delivery/telegram/...
```

Expected: no output (successful build).

- [ ] **Step 3: Commit**

```bash
git add internal/delivery/telegram/handler.go
git commit -m "feat: add editMsgAndClearMarkup helper"
```

---

## Task 2: Replace `editMsg` with `editMsgAndClearMarkup` at all definitive-action sites

**Files:**
- Modify: `internal/delivery/telegram/callbacks.go`

This task makes 13 targeted replacements in `callbacks.go`. Each replacement is listed with the current text to find and the new text. Work through them sequentially.

- [ ] **Step 1: Fix `cbTemplate` (habit created from template)**

Find (line ~88):
```go
	h.editMsg(chatID, msgID, i18n.T(lang, "habit.created",
		habit.Name, formatInterval(habit.IntervalMinutes, lang), habit.StartHour, habit.EndHour,
	))
```
Replace with:
```go
	h.editMsgAndClearMarkup(chatID, msgID, i18n.T(lang, "habit.created",
		habit.Name, formatInterval(habit.IntervalMinutes, lang), habit.StartHour, habit.EndHour,
	))
```

- [ ] **Step 2: Fix `cbAddGoal` (custom wizard habit created)**

Find (line ~198):
```go
	h.editMsg(chatID, msgID, result)
```
Replace with:
```go
	h.editMsgAndClearMarkup(chatID, msgID, result)
```

- [ ] **Step 3: Fix `cbDone` — clear keyboard when undo is unavailable**

In `cbDone`, find the block (lines ~237–246):
```go
	edit := tgbotapi.NewEditMessageText(chatID, msgID, doneMsg)
	if prevHabit != nil {
		kb := tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "habit.undo_btn"), fmt.Sprintf("undo:%d", habitID)),
		))
		edit.ReplyMarkup = &kb
	}
	if _, err := h.api.Send(edit); err != nil {
		h.logger.Error("edit done msg", zap.Error(err))
	}
```
Replace with:
```go
	edit := tgbotapi.NewEditMessageText(chatID, msgID, doneMsg)
	if prevHabit != nil {
		kb := tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "habit.undo_btn"), fmt.Sprintf("undo:%d", habitID)),
		))
		edit.ReplyMarkup = &kb
	} else {
		empty := tgbotapi.NewInlineKeyboardMarkup()
		edit.ReplyMarkup = &empty
	}
	if _, err := h.api.Send(edit); err != nil {
		h.logger.Error("edit done msg", zap.Error(err))
	}
```

- [ ] **Step 4: Fix `cbUndo` (mark-done undone)**

Find (line ~284):
```go
	h.editMsg(chatID, msgID, i18n.T(lang, "habit.undo_done"))
```
Replace with:
```go
	h.editMsgAndClearMarkup(chatID, msgID, i18n.T(lang, "habit.undo_done"))
```

Also fix the undo-expired path in `cbUndo`. Find (line ~262):
```go
		h.editMsg(chatID, msgID, i18n.T(lang, "habit.undo_expired"))
```
Replace with:
```go
		h.editMsgAndClearMarkup(chatID, msgID, i18n.T(lang, "habit.undo_expired"))
```

- [ ] **Step 5: Fix `cbDoneAll` (all habits marked done)**

Find (line ~304):
```go
	h.editMsg(chatID, msgID, i18n.T(lang, "today.all_done"))
```
Replace with:
```go
	h.editMsgAndClearMarkup(chatID, msgID, i18n.T(lang, "today.all_done"))
```

- [ ] **Step 6: Fix `cbConfirmDelete` (habit deleted)**

Find (line ~423):
```go
	h.editMsg(chatID, msgID, i18n.T(lang, "habit.deleted"))
```
Replace with:
```go
	h.editMsgAndClearMarkup(chatID, msgID, i18n.T(lang, "habit.deleted"))
```

- [ ] **Step 7: Fix `cbCancelDelete` (delete cancelled)**

Find (line ~432):
```go
	h.editMsg(chatID, msgID, i18n.T(lang, "action.delete_cancelled"))
```
Replace with:
```go
	h.editMsgAndClearMarkup(chatID, msgID, i18n.T(lang, "action.delete_cancelled"))
```

- [ ] **Step 8: Fix `cbPauseResume` (habit paused/resumed)**

Find (line ~599):
```go
		h.editMsg(chatID, msgID, i18n.T(lang, "habit.paused", habit.Name))
```
Replace with:
```go
		h.editMsgAndClearMarkup(chatID, msgID, i18n.T(lang, "habit.paused", habit.Name))
```

Find (line ~606):
```go
		h.editMsg(chatID, msgID, i18n.T(lang, "habit.resumed", habit.Name))
```
Replace with:
```go
		h.editMsgAndClearMarkup(chatID, msgID, i18n.T(lang, "habit.resumed", habit.Name))
```

- [ ] **Step 9: Fix `cbTimezone` (timezone saved)**

Find (line ~62):
```go
	h.editMsg(chatID, msgID, i18n.T(h.lang(user), "timezone.set", tz))
```
Replace with:
```go
	h.editMsgAndClearMarkup(chatID, msgID, i18n.T(h.lang(user), "timezone.set", tz))
```

- [ ] **Step 10: Fix `cbTimezoneOnboard` (timezone saved during onboarding)**

Find (line ~747):
```go
	h.editMsg(chatID, msgID, i18n.T(lang, "timezone.set", tz))
```
Replace with:
```go
	h.editMsgAndClearMarkup(chatID, msgID, i18n.T(lang, "timezone.set", tz))
```

- [ ] **Step 11: Fix `cbEditInterval` (interval updated)**

Find (line ~509):
```go
	h.editMsg(chatID, msgID, i18n.T(lang, "habit.interval_updated", formatInterval(minutes, lang)))
```
Replace with:
```go
	h.editMsgAndClearMarkup(chatID, msgID, i18n.T(lang, "habit.interval_updated", formatInterval(minutes, lang)))
```

- [ ] **Step 12: Fix `cbEditEnd` (hours updated)**

Find (line ~576):
```go
	h.editMsg(chatID, msgID, i18n.T(lang, "habit.hours_updated", startHour, endHour))
```
Replace with:
```go
	h.editMsgAndClearMarkup(chatID, msgID, i18n.T(lang, "habit.hours_updated", startHour, endHour))
```

- [ ] **Step 13: Fix `cbSetGoal` (goal set/removed)**

Find (line ~809):
```go
		h.editMsg(chatID, msgID, i18n.T(lang, "goal.removed"))
```
Replace with:
```go
		h.editMsgAndClearMarkup(chatID, msgID, i18n.T(lang, "goal.removed"))
```

Find (line ~811):
```go
		h.editMsg(chatID, msgID, i18n.T(lang, "goal.set", days))
```
Replace with:
```go
		h.editMsgAndClearMarkup(chatID, msgID, i18n.T(lang, "goal.set", days))
```

- [ ] **Step 14: Fix `cbSnooze` (snooze set)**

Find (line ~632):
```go
	h.editMsg(chatID, msgID, i18n.T(lang, "snooze.remind_in", minutes))
```
Replace with:
```go
	h.editMsgAndClearMarkup(chatID, msgID, i18n.T(lang, "snooze.remind_in", minutes))
```

- [ ] **Step 15: Verify the code compiles**

```bash
go build ./internal/delivery/telegram/...
```

Expected: no output.

- [ ] **Step 16: Commit**

```bash
git add internal/delivery/telegram/callbacks.go
git commit -m "fix: clear inline keyboard markup after all definitive callback actions"
```

---

## Task 3: Add stats i18n keys

**Files:**
- Modify: `internal/i18n/en.go`
- Modify: `internal/i18n/ru.go`
- Modify: `internal/i18n/kz.go`
- Modify: `internal/i18n/i18n_test.go`

- [ ] **Step 1: Write the failing test**

In `internal/i18n/i18n_test.go`, add after `TestNewNavKeysExistAllLangs`:

```go
func TestStatsKeysExistAllLangs(t *testing.T) {
	keys := []string{
		"stats.today_line",
		"stats.week_line",
		"stats.month_line",
		"stats.habit_btn",
		"stats.habit_btn_paused",
	}
	langs := []i18n.Lang{i18n.RU, i18n.EN, i18n.KZ}
	for _, lang := range langs {
		for _, key := range keys {
			got := i18n.T(lang, key)
			if got == key || got == "" {
				t.Errorf("lang=%s key=%s: missing translation (got %q)", lang, key, got)
			}
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/i18n/... -run TestStatsKeysExistAllLangs -v
```

Expected: FAIL — missing translation for `stats.today_line` etc.

- [ ] **Step 3: Add keys to `en.go`**

In `internal/i18n/en.go`, add before the closing `}` of `enMessages`:

```go
	"stats.today_line":        "Today:      %d/%d done",
	"stats.week_line":         "This week:  %d%% (%d/%d)",
	"stats.month_line":        "This month: %d%% (%d/%d)",
	"stats.habit_btn":         "%s  🔥%d  %d%%",
	"stats.habit_btn_paused":  "%s  ⏸ paused",
```

- [ ] **Step 4: Add keys to `ru.go`**

In `internal/i18n/ru.go`, add before the closing `}` of `ruMessages`:

```go
	"stats.today_line":        "Сегодня:     %d/%d",
	"stats.week_line":         "Эта неделя:  %d%% (%d/%d)",
	"stats.month_line":        "Этот месяц:  %d%% (%d/%d)",
	"stats.habit_btn":         "%s  🔥%d  %d%%",
	"stats.habit_btn_paused":  "%s  ⏸ пауза",
```

- [ ] **Step 5: Add keys to `kz.go`**

In `internal/i18n/kz.go`, add before the closing `}` of `kzMessages`:

```go
	"stats.today_line":        "Бүгін:       %d/%d",
	"stats.week_line":         "Осы апта:    %d%% (%d/%d)",
	"stats.month_line":        "Осы ай:      %d%% (%d/%d)",
	"stats.habit_btn":         "%s  🔥%d  %d%%",
	"stats.habit_btn_paused":  "%s  ⏸ үзіліс",
```

- [ ] **Step 6: Run test to verify it passes**

```bash
go test ./internal/i18n/... -run TestStatsKeysExistAllLangs -v
```

Expected: PASS.

- [ ] **Step 7: Run all i18n tests**

```bash
go test ./internal/i18n/... -v
```

Expected: all tests PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/i18n/en.go internal/i18n/ru.go internal/i18n/kz.go internal/i18n/i18n_test.go
git commit -m "feat: add stats i18n keys for today/week/month summary and habit buttons"
```

---

## Task 4: Rewrite `handleStats`

**Files:**
- Modify: `internal/delivery/telegram/commands.go` (replace `handleStats`, lines ~258–287)

- [ ] **Step 1: Replace the entire `handleStats` function**

In `internal/delivery/telegram/commands.go`, find the existing `handleStats` function body and replace it entirely with:

```go
func (h *Handler) handleStats(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	lang := h.lang(user)

	habits, err := h.habitUC.ListHabits(ctx, user.ID)
	if err != nil {
		h.logger.Error("ListHabits stats", zap.Int64("user_id", user.ID), zap.Error(err))
		h.send(msg.Chat.ID, i18n.T(lang, "error.generic"))
		return
	}
	weekStats, err := h.habitUC.GetStats(ctx, user.ID, 7)
	if err != nil {
		h.logger.Error("GetStats week", zap.Error(err))
		h.send(msg.Chat.ID, i18n.T(lang, "error.generic"))
		return
	}
	monthStats, err := h.habitUC.GetStats(ctx, user.ID, 30)
	if err != nil {
		h.logger.Error("GetStats month", zap.Error(err))
		h.send(msg.Chat.ID, i18n.T(lang, "error.generic"))
		return
	}
	if len(monthStats) == 0 {
		h.send(msg.Chat.ID, i18n.T(lang, "stats.empty"))
		return
	}

	// Today done / total (non-paused habits only)
	now := time.Now()
	todayTotal, todayDone := 0, 0
	for _, habit := range habits {
		if habit.IsPaused {
			continue
		}
		todayTotal++
		if usecase.IsDoneToday(habit, now) {
			todayDone++
		}
	}

	// Week aggregation
	weekCompleted, weekTotal := 0, 0
	for _, s := range weekStats {
		weekCompleted += s.CompletedDays
		weekTotal += s.TotalDays
	}
	weekPct := 0
	if weekTotal > 0 {
		weekPct = weekCompleted * 100 / weekTotal
	}

	// Month aggregation
	monthCompleted, monthTotal := 0, 0
	for _, s := range monthStats {
		monthCompleted += s.CompletedDays
		monthTotal += s.TotalDays
	}
	monthPct := 0
	if monthTotal > 0 {
		monthPct = monthCompleted * 100 / monthTotal
	}

	var sb strings.Builder
	sb.WriteString(i18n.T(lang, "stats.header"))
	sb.WriteString("\n")
	sb.WriteString(i18n.T(lang, "stats.today_line", todayDone, todayTotal))
	sb.WriteString("\n")
	sb.WriteString(i18n.T(lang, "stats.week_line", weekPct, weekCompleted, weekTotal))
	sb.WriteString("\n")
	sb.WriteString(i18n.T(lang, "stats.month_line", monthPct, monthCompleted, monthTotal))
	sb.WriteString("\n\n")
	sb.WriteString(i18n.T(lang, "stats.xp_level", user.Level, user.XP, user.StreakShields))

	// Per-habit buttons (tapping opens history)
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, s := range monthStats {
		var label string
		if s.Habit.IsPaused {
			label = i18n.T(lang, "stats.habit_btn_paused", s.Habit.Name)
		} else {
			label = i18n.T(lang, "stats.habit_btn", s.Habit.Name, s.Habit.Streak, s.CompletionPct)
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, fmt.Sprintf("history:%d", s.Habit.ID)),
		))
	}

	m := tgbotapi.NewMessage(msg.Chat.ID, sb.String())
	if len(rows) > 0 {
		m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	}
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send stats", zap.Error(err))
	}
}
```

Ensure `time` is imported in `commands.go` (it already is — `handleToday` uses it).

- [ ] **Step 2: Verify the code compiles**

```bash
go build ./internal/delivery/telegram/...
```

Expected: no output.

- [ ] **Step 3: Run all tests**

```bash
go test ./...
```

Expected: all tests PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/delivery/telegram/commands.go
git commit -m "feat: redesign /stats — today/week/month summary + tappable per-habit buttons"
```
