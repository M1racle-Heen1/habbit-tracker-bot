# Habit Tracker Bot — Improvements Design

**Date:** 2026-03-26
**Status:** Approved
**Scope:** Multi-language support, gamification (streak shield, achievements, XP/levels), richer UX (/today, onboarding), smarter notifications (end-of-day recap, adaptive reminders, progress visualization)

---

## Context

The bot is a Telegram habit tracker built with Go + PostgreSQL + Redis + Uber FX (clean architecture). It currently supports: habit creation wizard, list/done/delete/edit/pause/resume, snooze reminders, streaks, goal days, stats, history, timezone, morning digest, weekly digest, streak-break notification, and 5 habit templates. All text is hardcoded in Russian.

The goal is to open this to a public/international audience. The improvements are delivered in three phases.

---

## Architecture Overview

No structural rewrites. Two new packages are introduced; everything else extends existing patterns.

**New packages:**
- `internal/i18n/` — translation lookup. Single function `T(lang, key string, args ...any) string` backed by `map[Lang]map[string]string`. Keys use dot-notation (e.g. `"habit.done"`, `"streak.shield_used"`). All handler and scheduler message strings replace hardcoded Russian text with `T(user.Language, ...)`.
- `internal/gamification/` — achievement checking, XP calculation, level-up logic. Called from `HabitUsecase.MarkDone` after a successful completion.

**Database (migration `004`):**
- `users`: add `language VARCHAR(5) DEFAULT 'ru'`, `xp INT DEFAULT 0`, `level INT DEFAULT 1`, `streak_shields INT DEFAULT 3`, `evening_recap_hour INT DEFAULT 21`
- new table `user_achievements(id BIGSERIAL PRIMARY KEY, user_id BIGINT REFERENCES users(id), achievement_code VARCHAR(64), unlocked_at TIMESTAMPTZ DEFAULT NOW(), UNIQUE(user_id, achievement_code))`

**No changes to:** domain interfaces, delivery routing pattern, scheduler architecture, Redis state machine.

---

## Phase 1 — Multi-language + Streak Shield + Onboarding

### Multi-language

`internal/i18n/` package with `Lang` type (`"ru"`, `"en"`, `"kz"`) and a `T(lang, key, args...)` function. Translation maps cover all bot-facing strings: commands, reminders, digests, errors, onboarding.

`User` domain struct gains `Language string` field. `UserRepository` gains `UpdateLanguage(ctx, userID, lang)`.

New `/language` command shows an inline keyboard: 🇷🇺 Русский / 🇬🇧 English / 🇰🇿 Қазақша. Persists to DB. Also the first step in the new onboarding flow.

### Streak Shield

`User.StreakShields int` — starts at 3 for new users.

Scheduler midnight reset: before breaking a streak, check if the user has shields > 0. If yes: decrement shields, skip the streak reset, send localized message: *"🛡 Shield used! Your [habit] streak is protected. Shields remaining: 2"*. If no shields: existing reset + break notification.

Shield balance shown in `/stats` output.

### Improved Onboarding (`/start` for new users)

Three-step guided flow for first-time users:
1. Language selection (inline keyboard — reused for `/language`)
2. Timezone selection (existing inline keyboard, reused)
3. "Add your first habit?" — ✅ Yes / ⏭ Later buttons. "Yes" triggers existing `startAddHabit` flow.

Returning users skip to the normal welcome message. Detection: `User.CreatedAt` within last 60 seconds = new user.

---

## Phase 2 — Achievements, XP & Levels, /today

### Achievements

`gamification.CheckAchievements(ctx, userID)` is called at the end of `HabitUsecase.MarkDone`. It queries `user_achievements` to skip already-unlocked achievements, then evaluates the predefined set:

| Code | Trigger | Reward |
|------|---------|--------|
| `first_done` | First habit completed ever | +1 shield |
| `streak_7` | 7-day streak on any habit | +1 shield |
| `streak_30` | 30-day streak on any habit | +1 shield + 100 XP bonus |
| `streak_100` | 100-day streak on any habit | +2 shields + 500 XP bonus |
| `perfect_week` | All habits done every day for 7 days | +1 shield |
| `early_bird` | Habit marked done before 09:00, 5 times | badge only |
| `completionist` | 100% completion rate over any 30-day window | badge only |

On unlock, a message is sent to the user: *"🏆 Achievement unlocked: 7-day Warrior! +1 streak shield"* (localized). New `/achievements` command lists all earned badges with unlock dates.

### XP & Levels

`User.XP` and `User.Level` tracked in DB.

XP per `MarkDone`: **+10 base** + **+1 per streak day** (capped at +20 bonus). Total max per completion: 30 XP.

Level thresholds:

| Level | XP required |
|-------|-------------|
| 1 | 0 |
| 2 | 100 |
| 3 | 250 |
| 4 | 500 |
| 5 | 1000 |
| 6+ | previous + 500 |

On level-up: *"⬆️ Level up! You're now Level 3 🎉"* (localized). XP and level displayed in `/stats`.

### `/today` Command

Shows only today's incomplete habits with inline ✅ done buttons. No streak/goal clutter — fast daily check-in. If all done: *"✅ All habits done today! Great job."* If none: shows all habits.

---

## Phase 3 — Smarter Notifications & Progress Visualization

### End-of-day Recap

Sent at `User.EveningRecapHour` (default 21) in the user's timezone. Scheduler checks this like the morning digest (Redis dedup key per user per day).

Format:
```
🌙 Today's recap:
✅ Drink water — done
✅ Exercise — done
○ Reading — missed

2/3 habits completed (67%) 💪
Streak shields: 2 🛡
```

Response varies by completion rate:
- 100%: celebratory message
- 50–99%: encouraging message
- 0–49%: gentle nudge

All strings localized via i18n.

### Progress Visualization

**`/history` ASCII heatmap** — last 28 days displayed as a weekly grid per habit:
```
📅 Drink water — last 28 days:
Mo ■ ■ □ ■
Tu ■ □ ■ ■
We ■ ■ ■ □
...
■ done  □ missed
```

**`/stats` progress bar toward goal** — using Unicode block characters:
```
🏃 Exercise  ██████░░░░  18/30 days (60%)
```

### Adaptive Reminders

After 7+ activity records for a habit, compute the user's average completion hour from the `activities` table. The scheduler shifts the first daily reminder to 30 minutes before that average. Falls back to the habit's configured `start_hour` if fewer than 7 data points exist. No new DB column required — computed at runtime from existing data.

---

## Data Flow Summary

```
MarkDone (usecase)
  → update habit streak + lastDoneAt
  → activityRepo.Save
  → gamification.CheckAchievements
      → unlock achievements → notify user
      → award XP → check level-up → notify user
      → award shields if applicable

Scheduler tick (every minute)
  → per-user: morning digest, evening recap, weekly digest
  → per-habit: interval reminders (adaptive timing in Phase 3)
  → midnight: streak reset with shield check (Phase 1)

/start (new user)
  → language select → timezone select → first habit wizard
```

---

## Error Handling

- Achievement check failures are logged and swallowed — they must not block `MarkDone`.
- Missing translations fall back to the English key string (never panic).
- Shield decrement uses a DB transaction with the streak reset to prevent races.
- Adaptive reminder timing is best-effort; any query error falls back to configured hours.

---

## Out of Scope

- Social features (leaderboards, accountability partners)
- Monetization / premium tier
- Web dashboard
- Push notifications outside Telegram
