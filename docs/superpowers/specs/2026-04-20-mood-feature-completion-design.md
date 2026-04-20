# Mood Feature Completion Design

**Date:** 2026-04-20  
**Status:** Approved  

## Overview

The mood check-in system (DB schema, repo, usecase, DI, `cbMood` handler, weekly digest integration) is already built. This spec covers the four remaining gaps to make the feature complete:

1. `/mood` command — manual log entry with "already logged" update flow
2. Evening mood prompt via scheduler — fires after evening recap if not yet logged
3. Mood summary in `/stats` — 7-day summary appended to stats message
4. Dedup coordination — prevents double-prompting from two sources

---

## Section 1 — Data Layer

**No new repository interface methods.**

The `/mood` command needs today's logged mood. Rather than adding `GetByDate`, call the existing `GetByUserAndDateRange(ctx, userID, today, tomorrow)` and take `result[0]` if non-empty. Keeps `MoodRepository` interface minimal and avoids new `app.go` wiring.

---

## Section 2 — `/mood` Command

**File:** `internal/delivery/telegram/commands.go`

New handler `handleMood(ctx, msg, user)`:

1. Call `moodUC.GetWeekMoods(ctx, user.ID, today, tomorrow)` to check today's entry.
2. **If logged:** send a single message with text `i18n.T(lang, "mood.already_logged", moodEmoji)` and the mood inline keyboard as `ReplyMarkup` (same buttons as `sendMoodPrompt`). `moodEmoji` is resolved via a local `map[int]string{1:"😞", 2:"😐", 3:"😊"}` on `moods[0].Mood`. The existing `cbMood` handler does upsert, so re-tapping overwrites silently.
3. **If not logged:** call `h.sendMoodPrompt(chatID, lang)` directly.

**New i18n key** (all 3 language files):
- `mood.already_logged` — e.g. `"You logged %s today — change it?"`

**Bot command menu** (`internal/delivery/telegram/bot.go`): add `{Command: "mood", Description: "Log mood / Настроение"}`.

**Command routing** (`internal/delivery/telegram/handler.go`): add `case "mood"` in `handleCommand`.

---

## Section 3 — Evening Mood Prompt via Scheduler

**File:** `internal/scheduler/scheduler.go`

At the end of `maybeSendEveningRecap`, after the recap message is sent successfully:

1. Check the dedup key `mood_prompt:{telegramID}:{date}` in Redis. If present, skip.
2. Call `s.moodUC.HasLoggedToday(ctx, userID, now)`. If already logged, skip.
3. Send mood prompt via `s.api.Send` using the same inline keyboard structure as `sendMoodPrompt` in the delivery layer (😊 Great / 😐 Okay / 😞 Tough → `mood:3`, `mood:2`, `mood:1`).
4. Set dedup key `mood_prompt:{telegramID}:{date}` = `"1"` with 25h TTL.

**Dedup coordination with delivery layer:**

`maybeSendMoodPrompt` in `internal/delivery/telegram/callbacks.go` (fires when all habits done) must also:
1. Check `mood_prompt:{telegramID}:{date}` before sending — skip if set.
2. Set the key after sending.

This ensures that whichever path fires first wins; the second is a no-op.

---

## Section 4 — Mood in `/stats`

**File:** `internal/delivery/telegram/commands.go`

At the end of `handleStats`, after building the today/week/month text:

1. Call `moodUC.GetWeekMoods(ctx, user.ID, weekFrom, weekTo)` where `weekFrom = now - 6 days`.
2. If moods non-empty, append `format.BuildMoodSummary(moods, lang)` to the stats string.
3. If no moods logged yet, append nothing (silent skip).

No new keyboard buttons. Mood appears as plain text at the bottom of the existing stats message.

---

## Redis Key

| Key | Value | TTL | Purpose |
|-----|-------|-----|---------|
| `mood_prompt:{telegramID}:{date}` | `"1"` | 25h | Dedup mood prompt across delivery + scheduler |

---

## i18n Changes

Add to `en.go`, `ru.go`, `kz.go`:

| Key | EN value |
|-----|----------|
| `mood.already_logged` | `"You logged %s today — change it?"` |

---

## Files Changed

| File | Change |
|------|--------|
| `internal/delivery/telegram/commands.go` | Add `handleMood`, append mood to `handleStats` |
| `internal/delivery/telegram/handler.go` | Route `case "mood"` in `handleCommand` |
| `internal/delivery/telegram/bot.go` | Add `/mood` to command menu |
| `internal/delivery/telegram/callbacks.go` | Add dedup key check/set in `maybeSendMoodPrompt` |
| `internal/scheduler/scheduler.go` | Append mood prompt at end of `maybeSendEveningRecap` |
| `internal/i18n/en.go`, `ru.go`, `kz.go` | Add `mood.already_logged` key |

No new migrations, no new domain types, no new DI wiring needed.
