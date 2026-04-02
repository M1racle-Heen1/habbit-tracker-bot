# Repository & Delivery Layer Refactor

**Date:** 2026-04-01  
**Status:** Implemented

## Problem

Two layers had accumulated technical debt that made the codebase hard to navigate and maintain:

- `handler.go` was a 1,734-line monolith mixing routing, state management, command handlers, callback handlers, and keyboard builders
- `convState.EditHabitID` was misused to store `endHour` (an int, not a habit ID) during the add-habit wizard
- ~25 identical `GetOrCreateUser` + `h.lang(user)` blocks scattered across callback handlers
- ~10 hardcoded Russian strings bypassing the i18n system
- `AddXP` used two separate queries (race condition: level could be written with wrong value if concurrent)
- `ListAllWithTelegramID` and `ListStreaksToBeReset` duplicated an identical 6-line JOIN query

## Approach

Structural refactor without any user-visible behavior change.

## Design

### Delivery layer — file split

`handler.go` split into 4 focused files in the same package:

| File | Responsibility |
|------|----------------|
| `handler.go` | `Handler` struct, routing (`HandleUpdate`, `handleCommand`, `handleCallback`, `handleText`), state helpers, message helpers, `getUserFromCallback` |
| `commands.go` | All `/xxx` command handlers |
| `callbacks.go` | All callback handlers + `undoKey`/`timerKey` helpers |
| `keyboards.go` | All keyboard builders + shared `hourButtons(prefix, hours)` helper + pure formatting functions (`doneMessage`, `formatInterval`, `progressBar`, `buildHeatmap`) |

### convState fix

Added `EndHour int` field to `convState`. Removed the `EditHabitID: int64(endHour)` write in `cbEndHour` and `int(state.EditHabitID)` read in `cbAddGoal`.

### getUserFromCallback helper

```go
func (h *Handler) getUserFromCallback(ctx, cq) (*domain.User, i18n.Lang, error)
```

Loads user, handles error internally, returns `(user, lang, err)`. Replaces ~25 inline repetitions.

### i18n fixes

All hardcoded Russian strings replaced with `i18n.T(lang, key)`. 15 new keys added to all three translation files (`ru.go`, `en.go`, `kz.go`):

- `habit.goal_progress`, `habit.none_for_history`
- `goal.choose`, `goal.days_btn`, `goal.no_goal`, `goal.removed`, `goal.set`
- `snooze.remind_in`, `timezone.invalid`
- `edit.name_btn`, `edit.interval_btn`, `edit.hours_btn`, `edit.goal_btn`
- `timer.min_btn`

### Keyboard deduplication

`hourButtons(prefix string, hours []int)` replaces 4 near-identical hour keyboard functions and is also used by the edit variants. `sendEditIntervalKeyboard` / `sendEditStartHourKeyboard` / `sendEditEndHourKeyboard` now accept `lang i18n.Lang` so they produce localized labels.

### Repository — AddXP atomicity

Single atomic `UPDATE … RETURNING xp, level` with level computed in SQL via `CASE WHEN`. Eliminates the two-query race condition.

### Repository — list query deduplication

Private `listHabitsWithUser(ctx, where, args...)` helper owns the JOIN query. `ListAllWithTelegramID` and `ListStreaksToBeReset` each call it with their respective WHERE clause.

### Repository — scan helper unification

`scanner` interface (`Scan(...any) error`) extracted. `scanHabit(scanner, *Habit)` implements the scan once. `scanHabitRow` and `scanHabitRows` delegate to it.
