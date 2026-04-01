# Main Navigation & UX Redesign

**Date:** 2026-04-01  
**Scope:** Persistent reply keyboard nav, simplified add-habit wizard, consolidated habit management, settings screen, onboarding timezone fix.

---

## 1. Persistent Reply Keyboard (Main Nav)

A `ReplyKeyboardMarkup` is sent to the user in two situations:
- After completing onboarding (first habit created or skipped)
- On `/start` for returning users

Layout (2+2+1, `ResizeKeyboard: true`, `OneTimeKeyboard: false`):
```
[ 📋 Today ]     [ 🗂 My Habits ]
[ ➕ Add Habit ] [ 📊 Stats ]
[ ⚙️ Settings ]
```

Button labels are i18n keys (`nav.today`, `nav.my_habits`, `nav.add_habit`, `nav.stats`, `nav.settings`) — translated in all three languages (ru, en, kz).

`handleText` checks if the incoming message matches a nav button label and routes to the corresponding handler. Slash commands continue to work unchanged for power users and the Telegram bot menu.

A helper `sendMainNav(chatID int64, lang i18n.Lang)` sends the keyboard. It is called from:
- `cbTemplate` after habit creation (both template and custom paths)
- `handleStart` for returning users

---

## 2. Simplified Add Habit Wizard

### Template path (unchanged)
One tap → habit created instantly → confirmation message → nav keyboard sent.

### Custom path (simplified from 5 steps to 2)
1. User taps "✏️ Custom" (or types `/add_habit`) → bot removes template keyboard, asks for name
2. User types name → habit created with smart defaults:
   - Interval: 120 minutes
   - Start hour: 8
   - End hour: 22
   - Goal days: 0 (none)
3. Bot replies with confirmation (i18n key `habit.created_with_defaults`):
   > ✅ "Morning Run" created!  
   > Reminders every 2h, 8:00–22:00.  
   > Want to adjust? Open **🗂 My Habits** → tap the habit → Edit.

Steps `stepAwaitInterval`, `stepAwaitStartHour`, `stepAwaitEndHour`, `stepAwaitGoal` are **removed from the add-habit wizard**. These steps and their `convState` fields are kept because the edit flow still uses them — but `cbTemplate` (custom branch) and `handleText` (stepAwaitName) no longer transition into them.

All new i18n strings added to `ru.go`, `en.go`, and `kz.go`.

---

## 3. My Habits — Inline Action Menu

`handleListHabits` (triggered by `/list_habits` or **🗂 My Habits** nav button) renders each habit as a row:

```
○ Morning Run 🔥5    [⚙️]
✅ Read 📚           [⚙️]
⏸ Meditation        [⚙️]
```

Each `⚙️` button carries callback data `habit_menu:{id}`.

`cbHabitMenu` sends a new message with the habit's inline action menu:
```
Morning Run
[ ✅ Mark Done ]  [ ✏️ Edit ]
[ ⏸ Pause ]      [ 📅 History ]
[ 🗑 Delete ]
```

- **Mark Done** → routes to existing `cbDone`
- **Edit** → routes to existing `cbEditMenu`
- **Pause/Resume** → routes to existing `cbPauseResume` (label toggles based on `habit.IsPaused`)
- **History** → routes to existing `cbHistory`
- **Delete** → routes to existing `cbPreDelete`

The separate `/edit_habit`, `/pause_habit`, `/resume_habit`, `/delete_habit` commands remain functional but are removed from the bot command menu (`SetMyCommands`). My Habits is now the primary management surface.

---

## 4. Settings Screen

**⚙️ Settings** nav button (and `/language`, `/timezone` commands) sends:
```
⚙️ Settings
[ 🌐 Language ]  [ 🕐 Timezone ]
```

Callback `settings:language` → shows existing language inline keyboard.  
Callback `settings:timezone` → shows existing timezone inline keyboard.

No changes to the underlying language/timezone logic.

---

## 5. Onboarding Flow

**Before:** language → (Almaty auto-set silently) → template picker  
**After:** language → timezone picker → welcome screen → template picker

Steps:
1. `/start` for new user → language keyboard (unchanged)
2. `cbLanguage` (onboarding branch) → saves language, sends timezone keyboard (same keyboard as `/timezone`) with `tz_ob:` prefix callbacks
3. `cbTimezoneOnboard` → saves timezone, sends welcome screen + template keyboard
4. Template or custom habit created → `sendMainNav` called → persistent nav appears

`tz_ob` callbacks follow the same validation as `tz` but route to the onboarding continuation instead of just confirming.

The nav keyboard is sent as soon as the first habit is created (or user skips via a "Later" button on the template picker).

---

## i18n Keys Required

All keys added to `ru.go`, `en.go`, `kz.go`:

| Key | Purpose |
|-----|---------|
| `nav.today` | Reply button label |
| `nav.my_habits` | Reply button label |
| `nav.add_habit` | Reply button label |
| `nav.stats` | Reply button label |
| `nav.settings` | Reply button label |
| `habit.created_with_defaults` | Custom habit creation confirmation with edit hint |
| `settings.header` | Settings screen title |
| `habit.menu_header` | Habit action menu title (habit name) |

---

## Files Changed

| File | Change |
|------|--------|
| `internal/delivery/telegram/handler.go` | Add nav button routing in `handleText`; add `sendMainNav` helper; add `habit_menu`, `tz_ob`, `settings` cases to `handleCallback` switch |
| `internal/delivery/telegram/commands.go` | Update `handleStart`, `startAddHabit`, `handleListHabits`; add `handleSettings` |
| `internal/delivery/telegram/callbacks.go` | Update `cbTemplate` (custom path), `cbLanguage` (onboarding); add `cbHabitMenu`, `cbTimezoneOnboard`, `cbSettings` |
| `internal/delivery/telegram/keyboards.go` | Add `mainNavKeyboard(lang)` |
| `internal/delivery/telegram/bot.go` | Update `SetMyCommands` to remove management commands from menu |
| `internal/i18n/ru.go`, `en.go`, `kz.go` | Add all new keys |
