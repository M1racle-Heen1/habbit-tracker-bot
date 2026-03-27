# Habit Tracker Bot

Telegram-бот для формирования привычек. Напоминает в активные часы, считает стрики, поддерживает русский/английский/казахский язык.

**Стек:** Go · PostgreSQL · Redis · Uber FX · Clean Architecture

---

## Быстрый старт (Docker)

```bash
cp .env.example .env
# Укажи TELEGRAM_TOKEN и TIMEZONE в .env

make docker-up
```

`docker-compose` автоматически: postgres → миграции → бот.

## Локальная разработка

```bash
docker-compose up postgres redis -d
make migrate
export TELEGRAM_TOKEN=...
export DB_DSN=postgres://user:pass@localhost:5432/habbit?sslmode=disable
make run
```

---

## Команды make

| Команда | Описание |
|---------|----------|
| `make run` | Запустить локально |
| `make build` | Собрать бинарник `./app` |
| `make test` | Тесты |
| `make lint` | golangci-lint |
| `make migrate` | Применить pending миграции (localhost:5432) |
| `make rollback` | Откатить последнюю миграцию |
| `make docker-up` | Собрать и запустить в Docker |
| `make docker-down` | Остановить контейнеры |
| `make docker-logs` | Логи контейнера app |

---

## Переменные окружения

| Переменная | Обязательная | По умолчанию |
|------------|:---:|---|
| `TELEGRAM_TOKEN` | ✅ | — |
| `DB_DSN` | ✅ | — |
| `MIGRATE_DSN` | ❌ | `postgres://user:pass@localhost:5432/habbit?sslmode=disable` |
| `REDIS_ADDR` | ❌ | `localhost:6379` |
| `TIMEZONE` | ❌ | `UTC` |

---

## Команды бота

| Команда | Описание |
|---------|----------|
| `/start` | Онбординг нового пользователя (язык → часовой пояс → первая привычка) |
| `/add_habit` | Добавить привычку (шаблоны или custom: название → интервал → часы активности) |
| `/list_habits` | Список привычек со статусом, стриком, меню паузы/редактирования |
| `/done` | Отметить привычку выполненной (с кнопкой отмены в течение 5 мин) |
| `/today` | Только невыполненные привычки на сегодня + кнопка "Все выполнены" |
| `/stats` | Статистика за 30 дней: прогресс-бар, стрик, рекорд, XP/уровень/щиты |
| `/history` | ASCII-тепловая карта выполнений за 28 дней |
| `/achievements` | Список заработанных достижений |
| `/language` | Сменить язык (🇷🇺 / 🇬🇧 / 🇰🇿) |
| `/timezone` | Сменить часовой пояс |
| `/edit_habit` | Изменить название, интервал или часы активности |
| `/pause_habit` | Приостановить напоминания |
| `/resume_habit` | Возобновить напоминания |
| `/delete_habit` | Удалить привычку |

### Создание привычки

`/add_habit` запускает пошаговый флоу:
1. Выбор шаблона (вода, зарядка, чтение, сон, медитация) или кастомный
2. Название (для кастомного)
3. Интервал напоминаний (30 мин / 1ч / 2ч / 3ч / 4ч / 8ч)
4. Начало активного времени
5. Конец активного времени
6. Цель (30 / 66 / 100 дней — опционально)

### Таймер

В `/today` и напоминаниях рядом с кнопкой выполнения появляется `⏱`. Тап запускает обратный отсчёт (15/30/45/60 мин) — по окончании привычка отмечается автоматически.

---

## Геймификация

- **XP и уровни** — каждое выполнение приносит XP (+10 базовых + бонус за стрик до +20)
- **Достижения** — `first_done`, `streak_7`, `streak_30`, `streak_100`, `perfect_week`, `early_bird`, `completionist`
- **Щиты стрика** — защищают стрик от сброса (начальный запас: 3). Пополняются через достижения
- **Рекорд стрика** — в `/stats` отображается лучший стрик за всё время

---

## Уведомления

| Время | Событие |
|-------|---------|
| 08:00 | Утренний дайджест — список привычек на день |
| 20:00 | Предупреждение о стриках под угрозой |
| 21:00 (настраивается) | Вечерний итог дня с процентом выполнения |
| Воскресенье 20:00 | Итоги недели |
| Полночь | Сброс стриков (с проверкой щитов) |

---

## Архитектура

```
delivery → usecase → domain
repository → usecase (через интерфейсы)
scheduler → usecase
gamification → usecase (callback после MarkDone)
```

```
internal/domain/             сущности (User, Habit, Activity, UserAchievement) + ошибки
internal/usecase/            бизнес-логика, интерфейсы репозиториев
internal/delivery/telegram/  polling, роутинг, conversation state (Redis)
internal/repository/         postgres + redis реализации
internal/scheduler/          тикер каждую минуту, все уведомления
internal/i18n/               переводы RU/EN/KZ
internal/gamification/       XP, уровни, достижения
migrations/                  *.up.sql / *.down.sql (текущая: 004_gamification)
```

---

## CI / CD

Пайплайн — GitHub Actions (`.github/workflows/ci.yml`).

| Джоб | Триггер | Что делает |
|------|---------|-----------|
| **lint** | любой push/PR | `golangci-lint` |
| **test** | любой push/PR | `go vet` + `go test -race` |
| **build** | после lint+test | компилирует бинарник |
| **deploy** | push в `main` | SSH → git pull → пересобирает docker-compose |

### Секреты (Settings → Secrets → Actions)

`SERVER_HOST`, `SERVER_USER`, `SERVER_SSH_KEY`, `DEPLOY_PATH`, `TELEGRAM_TOKEN`, `DB_DSN`, `REDIS_ADDR`, `TIMEZONE`
