# Stats Redesign & Stale Buttons Fix

**Date:** 2026-04-01  
**Scope:** Redesign `/stats` output for readability; fix stale inline keyboard buttons across all handlers.

---

## 1. Stale Buttons Fix

### Problem

`editMsg` calls `NewEditMessageText` which edits only the message text — the existing `InlineKeyboardMarkup` is preserved by the Telegram API. This means all inline keyboard buttons remain clickable after a definitive action (habit created, marked done, deleted, etc.), allowing users to re-fire the same action from old messages in chat history.

### Solution

Add a new helper `editMsgAndClearMarkup(chatID int64, messageID int, text string)` to `handler.go`:

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

Replace `editMsg` with `editMsgAndClearMarkup` at every **definitive action** site:

| File | Callback | Action |
|------|----------|--------|
| `callbacks.go` | `cbTemplate` | Habit created from template |
| `callbacks.go` | `cbAddGoal` | Habit created (custom wizard) |
| `callbacks.go` | `cbDone` | Habit marked done |
| `callbacks.go` | `cbUndo` | Mark-done undone |
| `callbacks.go` | `cbDoneAll` | All habits marked done |
| `callbacks.go` | `cbConfirmDelete` | Habit deleted |
| `callbacks.go` | `cbCancelDelete` | Delete cancelled |
| `callbacks.go` | `cbPauseResume` | Habit paused/resumed |
| `callbacks.go` | `cbTimezone` | Timezone saved |
| `callbacks.go` | `cbTimezoneOnboard` | Timezone saved (onboarding) |
| `callbacks.go` | `cbEditInterval` | Interval updated |
| `callbacks.go` | `cbEditEnd` | Hours updated |
| `callbacks.go` | `cbSetGoal` | Goal set/removed |
| `callbacks.go` | `cbSnooze` | Snooze set |

`editMsg` is kept only where the keyboard intentionally stays mid-flow (e.g. `cbInterval` mid-wizard confirmation — but since the add-habit wizard was simplified, this is now a very small set).

---

## 2. Stats Redesign

### Output Format

The new `/stats` message has two parts:

**Part 1 — Summary header:**
```
📊 Your Stats

Today:      3/5 done ✅
This week:  72% (38/53)
This month: 61% (91/150)

⭐ Level 3 · 320 XP  🛡 2 shields
```

- **Today**: count of non-paused habits done today / total non-paused habits
- **This week**: sum of CompletedDays for the last 7 days / sum of TotalDays (last 7 days), derived from the existing 30-day stats data
- **This month**: same but 30-day window — same data as before, just reframed

**Part 2 — Per-habit inline keyboard:**

One button per non-paused habit:
```
[ 💧 Morning Run  🔥12  61% ]
[ 📚 Read         🔥3   40% ]
[ 🧘 Meditation   ⏸ paused  ]
```

Button label format: `{name}  🔥{streak}  {pct}%` for active habits, `{name}  ⏸ paused` for paused habits.

Tapping a button fires `history:{habitID}` — the existing `cbHistory` handler, no new callbacks needed.

### Data Sources

`handleStats` makes three calls:

```go
habits, _    := h.habitUC.ListHabits(ctx, user.ID)
weekStats, _ := h.habitUC.GetStats(ctx, user.ID, 7)
monthStats, _ := h.habitUC.GetStats(ctx, user.ID, 30)
```

- **Today done/total**: iterate `habits`, count non-paused where `usecase.IsDoneToday(habit, now) == true`
- **Week %**: sum `s.CompletedDays` / sum `s.TotalDays` across all entries in `weekStats` (where `TotalDays > 0`)
- **Month %**: same aggregation on `monthStats`
- **Per-habit streak + %**: `habit.Streak` from `ListHabits`; `s.CompletionPct` from the matching entry in `monthStats`

`GetStats` is an existing usecase method that accepts a `days int` parameter — calling it with 7 and 30 needs no new code.

No new repository or usecase methods needed.

### i18n Keys Required

Add to `ru.go`, `en.go`, `kz.go`:

| Key | EN value |
|-----|----------|
| `stats.today_line` | `Today:      %d/%d done` |
| `stats.week_line` | `This week:  %d%% (%d/%d)` |
| `stats.month_line` | `This month: %d%% (%d/%d)` |
| `stats.habit_btn` | `%s  🔥%d  %d%%` |
| `stats.habit_btn_paused` | `%s  ⏸ paused` |

---

## Files Changed

| File | Change |
|------|--------|
| `internal/delivery/telegram/handler.go` | Add `editMsgAndClearMarkup` helper |
| `internal/delivery/telegram/callbacks.go` | Replace `editMsg` → `editMsgAndClearMarkup` at all definitive-action sites |
| `internal/delivery/telegram/commands.go` | Rewrite `handleStats` |
| `internal/i18n/en.go`, `ru.go`, `kz.go` | Add stats i18n keys |
