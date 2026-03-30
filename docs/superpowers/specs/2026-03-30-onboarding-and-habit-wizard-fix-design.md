# Onboarding Simplification + Custom Habit Wizard Bug Fix

**Date:** 2026-03-30
**Status:** Approved

---

## Problem

Two issues:

1. **Onboarding drop-off** — the current 4-step new-user flow (language → timezone → "add habit?" → template) has too many steps. Users drop off before completing it.

2. **Custom habit wizard stops after name entry** — when a user clicks "✏️ Custom habit" and types a name, the interval keyboard never appears. The wizard silently stalls. Root causes:
   - `sendIntervalKeyboard` (and sibling functions) log errors but send no feedback to the user.
   - `handleText` has no cases for intermediate wizard steps (`stepAwaitInterval`, `stepAwaitStartHour`, `stepAwaitEndHour`, `stepAwaitGoal`), so a user stuck in one of these steps has no recovery path.
   - In the onboarding path, a double-click on "yes" sent two `onboard_habit` callbacks; the second could clear state after `cbTemplate` had already set it, causing `handleText` to see nil state.

---

## Solution

### 1. Simplified onboarding

**New flow for new users:**

```
/start → language picker → (language chosen) → set timezone default + send welcome screen → send template picker
```

- **`isNew` detection**: replace `time.Since(user.CreatedAt) < 60s` with `user.Language == ""`. More reliable — time window was fragile for pre-created accounts.
- **Default timezone**: set `Asia/Almaty` automatically when the user picks a language during onboarding. No timezone picker step.
- **Welcome screen**: one localized message between language selection and the template picker. Explains the bot's purpose and sets the expectation that the next step is picking a first habit.
- **Remove**: `stepOnboardTimezone`, `stepOnboardHabit`, `cbOnboardTimezone`, `cbOnboardHabit`, and the timezone-picker call inside `cbLanguage`. No state is held during onboarding — `cbLanguage` fires, sets timezone + sends welcome + sends template keyboard, done.
- **Onboarding detection in `cbLanguage`**: check `user.Language == ""` before calling `SetLanguage`. If empty, it's the onboarding path → also call `SetTimezone(Asia/Almaty)`, send welcome screen, send template keyboard. If already set, it's a regular `/language` change → just update language, no extra steps.

**Onboarding state machine before:**
```
stepOnboardTimezone → (tz chosen) → stepOnboardHabit → (yes) → nil → cbTemplate
```

**After:**
```
nil → (language chosen) → nil [template keyboard shown directly]
```

### 2. Custom habit wizard bug fix

#### `resendCurrentStep` helper

New function on `Handler`:

```go
func (h *Handler) resendCurrentStep(chatID int64, lang i18n.Lang, state *convState) error
```

Maps each intermediate step to re-sending its keyboard:

| `state.Step`        | Action                                      |
|---------------------|---------------------------------------------|
| `stepAwaitInterval` | `sendIntervalKeyboard(chatID, lang)`         |
| `stepAwaitStartHour`| `sendStartHourKeyboard(chatID, lang)`        |
| `stepAwaitEndHour`  | `sendEndHourKeyboard(chatID, lang, state.StartHour+1)` |
| `stepAwaitGoal`     | `sendGoalKeyboard(chatID, lang)`             |

Returns an error if the underlying send fails.

#### `handleText` default case

```go
default:
    if err := h.resendCurrentStep(msg.Chat.ID, h.lang(user), state); err != nil {
        h.clearState(msg.From.ID)
        h.send(msg.Chat.ID, i18n.T(h.lang(user), "error.generic"))
    }
```

#### Keyboard sender signatures

All keyboard sender functions gain a `lang i18n.Lang` parameter and use existing i18n keys instead of hardcoded Russian strings:

| Function               | i18n key used              |
|------------------------|----------------------------|
| `sendIntervalKeyboard` | `habit.choose_interval`    |
| `sendStartHourKeyboard`| `habit.choose_start`       |
| `sendEndHourKeyboard`  | `habit.choose_end`         |
| `sendGoalKeyboard`     | `habit.choose_goal`        |
| `cbTemplate` (custom)  | `habit.enter_name`         |
| `handleText` (empty)   | `habit.name_empty`         |

All return `error` so callers can detect failures.

### 3. i18n additions

Add one new key to `ru.go`, `en.go`, `kz.go`:

| Key | RU | EN | KZ |
|-----|----|----|-----|
| `onboarding.welcome_screen` | "Я помогу тебе строить полезные привычки — напомню, отслежу прогресс и отмечу стрики.\n\nВыбери первую привычку:" | "I'll help you build good habits — send reminders, track progress, and celebrate streaks.\n\nPick your first habit:" | "Мен сізге пайдалы әдеттер қалыптастыруға көмектесемін.\n\nБірінші әдетті таңда:" |

---

## Architecture

No new files. All changes in:
- `internal/delivery/telegram/handler.go` — main logic changes
- `internal/i18n/ru.go`, `en.go`, `kz.go` — one new key each

No migration needed — no schema changes.

---

## What is NOT changing

- `/timezone` command — still works for users who want to change timezone later
- Template habits — unchanged
- All post-onboarding flows — unchanged
- Gamification, scheduler, repository layer — untouched
