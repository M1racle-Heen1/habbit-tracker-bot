# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Telegram habit tracker bot. Go + PostgreSQL + Redis + Uber FX, Clean Architecture.

Module: `github.com/saidakmal/habbit-tracker-bot`

## Commands

```bash
make run          # go run ./cmd/main.go
make build        # compile to ./app
make test         # go test ./...
make lint         # golangci-lint run ./...
make tidy         # go mod tidy
make migrate      # apply pending migrations (requires postgres on localhost:5432)
make rollback     # rollback last migration
make docker-up    # docker-compose up --build -d (runs migrations automatically)
make docker-logs  # docker-compose logs -f app
```

Single test: `go test ./internal/usecase/... -run TestName`

## Architecture

```
cmd/main.go                    → calls app.New().Run()
cmd/migrate/main.go            → standalone migration binary (golang-migrate)
internal/app/app.go            → fx.New wiring (all providers + lifecycle hooks)
internal/domain/               → entities (User, Habit, Activity, HabitWithTelegramID, UserAchievement) + error sentinels
internal/usecase/
  interfaces.go                → UserRepository, HabitRepository, ActivityRepository, Cache
  user.go, habit.go            → business logic + scheduler helpers (IsDoneToday, ShouldSendInterval, etc.)
internal/delivery/telegram/
  bot.go                       → polling loop + sets bot command menu on start
  handler.go                   → Handler struct, routing (HandleUpdate/handleCommand/handleCallback/handleText),
                                  state helpers, getUserFromCallback helper, editMsg/editMsgAndClearMarkup helpers
  commands.go                  → all /xxx command handlers
  callbacks.go                 → all callback handlers + undoKey/timerKey helpers
  keyboards.go                 → all keyboard builders, hourButtons helper, pure formatting functions
internal/repository/postgres/  → pgxpool implementations; wraps pgx.ErrNoRows → domain.ErrNotFound
internal/repository/redis/     → Cache implementation
internal/scheduler/            → ticker every minute; reminders, streak resets, evening recap, streak-risk alert, timer expiry
internal/i18n/                 → T(lang, key, args...) lookup; ru.go / en.go / kz.go translation maps
internal/gamification/         → XP calculation, level thresholds, achievement definitions + runner
internal/infrastructure/
  config/                      → reads + validates env vars
  logger/                      → zap.NewProduction()
migrations/                    → *.up.sql / *.down.sql files for golang-migrate
```

### Dependency flow

```
delivery → usecase → domain
repository → usecase (via interfaces, never imported directly)
scheduler → usecase (reads habits, calls MarkDone/UpdateNotified/ResetStreaks)
gamification → usecase (via GamificationNotifier callback, wired in app.go)
```

Repositories are bound to interfaces via `fx.Annotate(..., fx.As(new(usecase.XRepository)))` in `app.go`. Use cases take concrete types (not interfaces).

### Key invariants

- **userID in business logic = `users.id` (internal DB ID)**, not `msg.From.ID` (Telegram ID). Handlers always call `GetOrCreateUser` first.
- `GetOrCreateUser` only creates a user when error is `domain.ErrNotFound`; other DB errors propagate.
- Every handler method runs with `context.WithTimeout(ctx, 10s)`.
- Postgres repositories wrap `pgx.ErrNoRows` as `domain.ErrNotFound`.
- Scheduler converts `time.Now()` to `user.Timezone` location before all hour comparisons. The `TIMEZONE` env is only used for the default timezone and midnight streak-reset check.
- All business logic (streak calculation, reminder eligibility) lives in `usecase/`; handlers only route.
- Conversation state is **Redis-backed** (`state:{telegramID}` key, JSON-encoded `convState`), not in-memory.

### Delivery layer patterns

**Loading the user in callbacks:** use `getUserFromCallback(ctx, cq)` — it calls `GetOrCreateUser`, sends an error message on failure, and returns `(user, lang, err)`. Never repeat the inline pattern.

**Editing messages:** use `editMsgAndClearMarkup` for any **definitive** action (habit created, marked done, deleted, paused, goal set, etc.) so old inline buttons become inert. Use `editMsg` only for mid-wizard confirmations where the next keyboard immediately follows.

**convState fields:** `Step`, `HabitName`, `IntervalMinutes`, `StartHour`, `EndHour`, `EditHabitID`, `Lang`. `EndHour` stores the end hour during the add-habit wizard; `EditHabitID` stores the habit being edited during the edit-habit wizard. Do not reuse fields across flows.

### i18n

All user-facing strings go through `i18n.T(lang, key, args...)`. `lang` is always `user.Language` (stored in DB). Missing keys fall back to the English key string — never panic. Add new keys to all three files (`ru.go`, `en.go`, `kz.go`).

### Gamification

`HabitUsecase.MarkDone` fires a `GamificationNotifier` callback (set via `SetGamificationNotifier`) in a goroutine after a successful completion. The callback calls `gamification.Run` which checks achievements, awards XP, handles level-ups, and sends Telegram notifications. Achievement check failures are swallowed — they must not block `MarkDone`.

XP per completion: `+10 base + min(streak, 20) bonus`. Level thresholds: 1→0, 2→100, 3→250, 4→500, 5→1000, 6+→prev+500.

### Bot commands & conversation flow

| Command | Handler | Notes |
|---------|---------|-------|
| `/start` | `handleStart` | Language picker for new users; welcome for returning |
| `/add_habit` | `startAddHabit` | Multi-step: template picker or custom → name → interval → start hour → end hour → goal (optional) |
| `/list_habits` | `handleListHabits` | ✅/○ status + streak + inline done buttons |
| `/done` | `handleDone` | Inline keyboard → `done:{id}` callback → `MarkDone` → undo button (5 min TTL) |
| `/today` | `handleToday` | Incomplete habits only; `[✅ Name][⏱]` per row + bulk "all done" button |
| `/stats` | `handleStats` | Today/week/month summary header + per-habit tappable buttons (opens history) |
| `/history` | `handleHistory` | Habit picker → `history:{id}` → ASCII 28-day heatmap |
| `/achievements` | `handleAchievements` | Lists earned achievements with unlock dates |
| `/language` | `handleLanguage` | Inline keyboard: RU / EN / KZ |
| `/timezone` | `handleTimezone` | Inline keyboard of common timezones |
| `/edit_habit` | `handleEditHabit` | Name / interval / hours submenu |
| `/pause_habit` | `handlePauseHabit` | Pause reminders without deleting |
| `/resume_habit` | `handleResumeHabit` | Resume paused habit |
| `/delete_habit` | `handleDeleteHabit` | Two-step confirm |

**Conversation state steps** (`step` enum in `handler.go`):
- `stepIdle` — no active wizard
- `stepAwaitName / Interval / StartHour / EndHour / Goal` — add-habit wizard
- `stepEditAwaitName / EditAwaitEndHour` — edit-habit wizard

**Callback data format:** `action:arg` or `action:arg1:arg2`. All callbacks are routed in `handleCallback` via a switch on the action prefix.

**Key callback prefixes:** `done`, `done_all`, `undo`, `timer_start`, `timer_set`, `lang`, `tz`, `snooze`, `template`, `interval`, `start_hour`, `end_hour`, `add_goal`, `pre_delete`, `confirm_delete`, `cancel_delete`, `pause`, `resume`, `edit`, `edit_name`, `edit_interval`, `edit_start`, `edit_end`, `set_goal`, `goal_menu`, `history`.

### Redis key patterns

| Key | Value | TTL | Purpose |
|-----|-------|-----|---------|
| `state:{telegramID}` | JSON `convState` | 30 min | Conversation wizard state |
| `undo:{habitID}` | `{prevStreak}\|{prevLastDoneAtUnix}` | 5 min | Undo mark-done |
| `timer:{habitID}:{userID}` | unix expiry timestamp | duration+2 min | Habit session timer |
| `morning:{telegramID}:{date}` | `"1"` | 25h | Morning digest dedup |
| `evening:{telegramID}:{date}` | `"1"` | 25h | Evening recap dedup |
| `streak_risk:{telegramID}:{date}` | `"1"` | 25h | Streak-risk alert dedup |
| `weekly:{telegramID}:{date}` | `"1"` | 25h | Weekly digest dedup |

### Scheduler behaviors (every minute tick)

- **00:00 default timezone** — reset streaks (with shield check), send break notifications
- **08:00 user timezone** — morning digest (deduped per day)
- **20:00 user timezone** — streak-at-risk alert for habits with streak > 0 not yet done today
- **Sunday 20:00 user timezone** — weekly digest
- **`user.EveningRecapHour`:00 user timezone** (default 21) — evening recap
- **Per-habit every minute** — check expired timers → auto `MarkDone` + notify; send interval/final reminders (adaptive timing if 7+ activity records)

### Streak shield logic

At midnight reset: if `user.StreakShields > 0`, decrement shields and skip streak reset instead of breaking it. Uses a DB transaction to prevent races.

### Environment variables

| Var | Required | Default |
|-----|----------|---------|
| `TELEGRAM_TOKEN` | yes | — |
| `DB_DSN` | yes | — (use `@postgres:5432` in Docker, `@localhost:5432` locally) |
| `MIGRATE_DSN` | no | `postgres://user:pass@localhost:5432/habbit?sslmode=disable` |
| `REDIS_ADDR` | no | `localhost:6379` |
| `TIMEZONE` | no | `UTC` |

### Migrations

Files live in `migrations/` as `{N}_{name}.up.sql` / `{N}_{name}.down.sql`. Current highest migration: `004_gamification` (adds `language`, `xp`, `level`, `streak_shields`, `evening_recap_hour` to `users`; creates `user_achievements` table).

### Adding a new feature

1. Entity → `internal/domain/`
2. Interface method → `internal/usecase/interfaces.go`
3. Implementation → `internal/repository/postgres/`
4. Use case method → `internal/usecase/`
5. Command handler → `delivery/telegram/commands.go`; callback handlers → `delivery/telegram/callbacks.go`; keyboards → `delivery/telegram/keyboards.go`; i18n keys (all 3 lang files)
6. Wire in `internal/app/app.go` if new provider needed
7. Migration → `migrations/{N}_{name}.up.sql` + `.down.sql`
