# Habbit Tracker Bot

Telegram-бот для формирования привычек. Напоминает в активные часы, считает стрики, не беспокоит ночью.

**Стек:** Go · PostgreSQL · Redis · Uber FX · Clean Architecture

---

## Быстрый старт (Docker)

```bash
cp .env.example .env
# Укажи TELEGRAM_TOKEN и TIMEZONE в .env

make docker-up
```

`docker-compose` автоматически запускает postgres → применяет миграции → стартует бот.

## Локальная разработка

```bash
# Поднять инфраструктуру
docker-compose up postgres redis -d

# Применить миграции
make migrate

# Запустить бота
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

| Переменная | Обязательная | По умолчанию | Описание |
|------------|:---:|---|---|
| `TELEGRAM_TOKEN` | ✅ | — | Токен от @BotFather |
| `DB_DSN` | ✅ | — | DSN postgres (в Docker: `@postgres:5432`) |
| `MIGRATE_DSN` | ❌ | `postgres://user:pass@localhost:5432/habbit?sslmode=disable` | DSN для `make migrate` |
| `REDIS_ADDR` | ❌ | `localhost:6379` | Адрес Redis |
| `TIMEZONE` | ❌ | `UTC` | Часовой пояс (напр. `Asia/Tashkent`, `Europe/Moscow`) |

---

## Команды бота

| Команда | Описание |
|---------|----------|
| `/start` | Приветствие и описание бота |
| `/add_habit` | Добавить привычку (название → интервал → время активности) |
| `/list_habits` | Список привычек с прогрессом и стриком |
| `/done` | Отметить привычку выполненной |
| `/delete_habit` | Удалить привычку |
| `/health` | Проверка работоспособности |

### Создание привычки

`/add_habit` запускает пошаговый флоу:
1. Бот просит ввести название
2. Выбор интервала напоминаний (30 мин / 1ч / 2ч / 3ч)
3. Выбор начала активного времени (7:00–10:00)
4. Выбор конца активного времени (20:00–23:00)

---

## Архитектура

```
delivery → usecase → domain
repository → usecase (через интерфейсы)
scheduler → usecase
```

```
cmd/main.go                  точка входа
cmd/migrate/main.go          CLI для миграций
internal/app/app.go          FX wiring
internal/domain/             сущности + ошибки
internal/usecase/            бизнес-логика, интерфейсы репозиториев
internal/delivery/telegram/  polling, роутинг, conversation state
internal/repository/         postgres + redis реализации
internal/scheduler/          тикер каждую минуту, напоминания
internal/infrastructure/     config, logger
migrations/                  *.up.sql / *.down.sql
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

| Секрет | Описание |
|--------|----------|
| `SERVER_HOST` | IP или hostname VPS |
| `SERVER_USER` | SSH-пользователь |
| `SERVER_SSH_KEY` | Приватный SSH-ключ |
| `DEPLOY_PATH` | Путь на сервере (напр. `/opt/habbit-tracker-bot`) |
| `TELEGRAM_TOKEN` | Токен бота |
| `DB_DSN` | DSN postgres |
| `REDIS_ADDR` | Адрес Redis (опц.) |
| `TIMEZONE` | Часовой пояс (напр. `Asia/Almaty`) |
