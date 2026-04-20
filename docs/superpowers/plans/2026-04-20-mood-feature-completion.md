# Mood Feature Completion — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire the four remaining gaps in the mood check-in feature: `/mood` command, dedup coordination, evening recap prompt, and mood summary in `/stats`.

**Architecture:** All changes are additive wiring in the delivery and scheduler layers — no new domain types, no new DB migrations, no new DI changes. The existing `MoodUsecase` (`GetWeekMoods`, `HasLoggedToday`, `LogMood`) and `MoodRepository` serve all needs. A Redis dedup key `mood_prompt:{chatID}:{date}` coordinates the two prompt sources (delivery layer vs scheduler) so the user only sees it once per day.

**Tech Stack:** Go, go-telegram-bot-api/v5, Redis (via `usecase.Cache`), Uber FX (no changes needed), `internal/i18n`, `internal/format`

---

## File Map

| File | What changes |
|------|-------------|
| `internal/i18n/en.go` | Add `mood.already_logged` key |
| `internal/i18n/ru.go` | Add `mood.already_logged` key |
| `internal/i18n/kz.go` | Add `mood.already_logged` key |
| `internal/i18n/i18n_test.go` | Add test for new key |
| `internal/delivery/telegram/callbacks.go` | Add dedup key check/set in `maybeSendMoodPrompt` |
| `internal/delivery/telegram/commands.go` | Add `handleMood`; append mood summary in `handleStats` |
| `internal/delivery/telegram/handler.go` | Route `case "mood"` in `handleCommand` |
| `internal/delivery/telegram/bot.go` | Add `/mood` to bot command menu |
| `internal/scheduler/scheduler.go` | Append mood prompt after evening recap send |

---

## Task 1: Add `mood.already_logged` i18n key

**Files:**
- Modify: `internal/i18n/en.go`
- Modify: `internal/i18n/ru.go`
- Modify: `internal/i18n/kz.go`
- Modify: `internal/i18n/i18n_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/i18n/i18n_test.go` after `TestStatsKeysExistAllLangs`:

```go
func TestMoodAlreadyLoggedKeyExistsAllLangs(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.RU, i18n.EN, i18n.KZ} {
		got := i18n.T(lang, "mood.already_logged", "😊")
		if got == "mood.already_logged" || got == "" {
			t.Errorf("lang=%s: key mood.already_logged missing or empty (got %q)", lang, got)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/i18n/... -run TestMoodAlreadyLoggedKeyExistsAllLangs -v
```

Expected: `FAIL` — key falls back to raw key string.

- [ ] **Step 3: Add key to all three language files**

In `internal/i18n/en.go`, add after the `mood.burnout_alert` line:

```go
"mood.already_logged": "You logged %s today — want to change it?",
```

In `internal/i18n/ru.go`, add after the `mood.burnout_alert` line:

```go
"mood.already_logged": "Ты уже отметил(а) %s сегодня — изменить?",
```

In `internal/i18n/kz.go`, add after the `mood.burnout_alert` line:

```go
"mood.already_logged": "Бүгін %s деп белгіледіңіз — өзгерту керек пе?",
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/i18n/... -v
```

Expected: all tests `PASS`.

- [ ] **Step 5: Commit**

```bash
git add internal/i18n/en.go internal/i18n/ru.go internal/i18n/kz.go internal/i18n/i18n_test.go
git commit -m "feat: add mood.already_logged i18n key (EN/RU/KZ)"
```

---

## Task 2: Dedup key in `maybeSendMoodPrompt`

**Files:**
- Modify: `internal/delivery/telegram/callbacks.go`

Context: `maybeSendMoodPrompt` fires when all habits are done. We add a Redis dedup key so the scheduler's evening prompt won't double-fire for the same user on the same day.

- [ ] **Step 1: Replace `maybeSendMoodPrompt` in `internal/delivery/telegram/callbacks.go`**

Find the existing function (around line 279) and replace it entirely:

```go
// maybeSendMoodPrompt checks if all non-paused habits are done today and
// the user hasn't logged their mood yet or been prompted today; if so, sends the mood prompt.
func (h *Handler) maybeSendMoodPrompt(ctx context.Context, chatID int64, user *domain.User, lang i18n.Lang) {
	habits, err := h.habitUC.ListHabits(ctx, user.ID)
	if err != nil || len(habits) == 0 {
		return
	}
	now := time.Now()
	for _, hab := range habits {
		if !hab.IsPaused && !usecase.IsDoneToday(hab, now) {
			return
		}
	}
	logged, err := h.moodUC.HasLoggedToday(ctx, user.ID)
	if err != nil || logged {
		return
	}
	moodKey := fmt.Sprintf("mood_prompt:%d:%s", chatID, now.Format("2006-01-02"))
	if _, err := h.cache.Get(ctx, moodKey); err == nil {
		return
	}
	h.sendMoodPrompt(chatID, lang)
	_ = h.cache.Set(ctx, moodKey, "1", 25*time.Hour)
}
```

- [ ] **Step 2: Build to verify no compile errors**

```bash
go build ./...
```

Expected: exits 0 with no output.

- [ ] **Step 3: Commit**

```bash
git add internal/delivery/telegram/callbacks.go
git commit -m "feat: add dedup key to maybeSendMoodPrompt"
```

---

## Task 3: `/mood` command

**Files:**
- Modify: `internal/delivery/telegram/commands.go`
- Modify: `internal/delivery/telegram/handler.go`
- Modify: `internal/delivery/telegram/bot.go`

- [ ] **Step 1: Add `handleMood` to `internal/delivery/telegram/commands.go`**

Add this function anywhere after `handleStats` (before or after `handleHistory` is fine):

```go
func (h *Handler) handleMood(ctx context.Context, msg *tgbotapi.Message, user *domain.User) {
	lang := h.lang(user)
	today := time.Now()
	moods, err := h.moodUC.GetWeekMoods(ctx, user.ID, today, today.AddDate(0, 0, 1))
	if err != nil {
		h.logger.Error("handleMood GetWeekMoods", zap.Error(err))
		h.send(msg.Chat.ID, i18n.T(lang, "error.generic"))
		return
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "mood.great"), "mood:3"),
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "mood.okay"), "mood:2"),
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "mood.tough"), "mood:1"),
		),
	)

	if len(moods) > 0 {
		moodEmojis := map[int]string{1: "😞", 2: "😐", 3: "😊"}
		emoji := moodEmojis[moods[0].Mood]
		m := tgbotapi.NewMessage(msg.Chat.ID, i18n.T(lang, "mood.already_logged", emoji))
		m.ReplyMarkup = keyboard
		if _, err := h.api.Send(m); err != nil {
			h.logger.Error("send mood already logged", zap.Error(err))
		}
		return
	}

	h.sendMoodPrompt(msg.Chat.ID, lang)
}
```

- [ ] **Step 2: Wire the command in `internal/delivery/telegram/handler.go`**

In `handleCommand`, add `case "mood"` after the `case "settings"` line:

```go
case "mood":
    h.handleMood(ctx, msg, user)
```

- [ ] **Step 3: Add `/mood` to bot command menu in `internal/delivery/telegram/bot.go`**

In the `commands` slice in `bot.go`, add after the `"today"` entry:

```go
{Command: "mood", Description: "Log mood / Настроение дня"},
```

- [ ] **Step 4: Build to verify no compile errors**

```bash
go build ./...
```

Expected: exits 0 with no output.

- [ ] **Step 5: Commit**

```bash
git add internal/delivery/telegram/commands.go internal/delivery/telegram/handler.go internal/delivery/telegram/bot.go
git commit -m "feat: add /mood command with already-logged update flow"
```

---

## Task 4: Evening mood prompt via scheduler

**Files:**
- Modify: `internal/scheduler/scheduler.go`

Context: `maybeSendEveningRecap` (around line 365) sends the nightly recap. After the `s.api.Send` call that sends the recap, we append a mood prompt if the user hasn't logged today and hasn't been prompted already.

- [ ] **Step 1: Add mood prompt block to `maybeSendEveningRecap` in `internal/scheduler/scheduler.go`**

Find this block near the end of `maybeSendEveningRecap` (around line 415):

```go
	if _, err := s.api.Send(tgbotapi.NewMessage(telegramID, sb.String())); err != nil {
		s.logger.Error("send evening recap", zap.Int64("telegram_id", telegramID), zap.Error(err))
	}
}
```

Replace it with:

```go
	if _, err := s.api.Send(tgbotapi.NewMessage(telegramID, sb.String())); err != nil {
		s.logger.Error("send evening recap", zap.Int64("telegram_id", telegramID), zap.Error(err))
		return
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
```

Note: `return` is added on send error so we don't attempt the mood prompt after a failed recap send. The closing `}` closes `maybeSendEveningRecap`.

- [ ] **Step 2: Build to verify no compile errors**

```bash
go build ./...
```

Expected: exits 0 with no output.

- [ ] **Step 3: Commit**

```bash
git add internal/scheduler/scheduler.go
git commit -m "feat: send mood prompt after evening recap if not yet logged"
```

---

## Task 5: Mood summary in `/stats`

**Files:**
- Modify: `internal/delivery/telegram/commands.go`

Context: `handleStats` builds a `strings.Builder` (`sb`) and sends it. We append a 7-day mood summary at the end of `sb`, before the `m := tgbotapi.NewMessage(...)` call.

- [ ] **Step 1: Add mood summary to `handleStats` in `internal/delivery/telegram/commands.go`**

Find this block in `handleStats` (around line 329):

```go
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
```

Replace it with:

```go
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

	// 7-day mood summary
	weekFrom := now.AddDate(0, 0, -6)
	moods, err := h.moodUC.GetWeekMoods(ctx, user.ID, weekFrom, now.AddDate(0, 0, 1))
	if err != nil {
		h.logger.Warn("handleStats GetWeekMoods", zap.Error(err))
	} else if len(moods) > 0 {
		sb.WriteString(format.BuildMoodSummary(moods, lang))
	}

	// Per-habit buttons (tapping opens history)
```

Make sure `format` is imported at the top of `commands.go`. Check the existing imports — it should already be there since `handleInsights` uses it. Verify with:

```bash
grep '"github.com/saidakmal/habbit-tracker-bot/internal/format"' internal/delivery/telegram/commands.go
```

If missing, add it to the import block.

- [ ] **Step 2: Build to verify no compile errors**

```bash
go build ./...
```

Expected: exits 0 with no output.

- [ ] **Step 3: Run all tests**

```bash
go test ./...
```

Expected: all tests `PASS` (or `ok` with no failures).

- [ ] **Step 4: Commit**

```bash
git add internal/delivery/telegram/commands.go
git commit -m "feat: append 7-day mood summary to /stats"
```

---

## Verification Checklist

After all tasks are done, manually verify in Telegram (with the bot running locally via `make run`):

- [ ] `/mood` when no mood logged today → shows 😊/😐/😞 picker
- [ ] Tap a mood option → confirmation message, keyboard cleared
- [ ] `/mood` again → shows "You logged 😊 today — want to change it?" with picker; tapping a different option updates it
- [ ] Mark all habits done → mood prompt fires once; sending `/mood` again shows already-logged state
- [ ] `/stats` → mood summary line appears at the bottom if any moods logged in the past 7 days
- [ ] At `EveningRecapHour` (default 21:00 user timezone) → evening recap sent, mood prompt follows if not yet logged
- [ ] If mood prompt already sent (all-habits-done path), evening recap does NOT send a second prompt
