# Habbit Tracker Bot

Telegram-бот для отслеживания привычек. Go + PostgreSQL + Redis + Uber FX, Clean Architecture.

## Быстрый старт (Docker)

```bash
cp .env.example .env
# Укажи TELEGRAM_TOKEN в .env

make docker-up
```

Postgres применит миграции автоматически через `docker-entrypoint-initdb.d`.

## Локальная разработка

```bash
go mod tidy
# Запусти postgres и redis (или docker-compose up postgres redis -d)
export TELEGRAM_TOKEN=...
export DB_DSN=postgres://user:pass@localhost:5432/habbit?sslmode=disable
make run
```

## Команды

| Команда        | Описание                          |
|----------------|-----------------------------------|
| `make run`     | Запустить бота локально           |
| `make build`   | Собрать бинарник `./app`          |
| `make test`    | Запустить тесты                   |
| `make lint`    | Запустить golangci-lint           |
| `make tidy`    | Обновить зависимости              |
| `make docker-up` | Собрать и запустить в Docker    |
| `make docker-down` | Остановить контейнеры         |
| `make docker-logs` | Логи контейнера app           |

## Переменные окружения

| Переменная       | Обязательная | По умолчанию        | Описание                   |
|------------------|:---:|---------------------|----------------------------|
| `TELEGRAM_TOKEN` | ✅  | —                   | Токен от @BotFather        |
| `DB_DSN`         | ✅  | —                   | DSN подключения к Postgres |
| `REDIS_ADDR`     | ❌  | `localhost:6379`    | Адрес Redis                |

## CI / CD

Пайплайн запускается через GitHub Actions (`.github/workflows/ci.yml`).

| Джоб | Триггер | Что делает |
|---|---|---|
| **lint** | любой push/PR | `golangci-lint` |
| **test** | любой push/PR | `go vet` + `go test -race` |
| **build** | после lint+test | компилирует бинарник |
| **deploy** | push в `main` | SSH → git pull → пересобирает docker-compose |

### Секреты (Settings → Secrets → Actions)

| Секрет | Описание |
|---|---|
| `SERVER_HOST` | IP или hostname VPS |
| `SERVER_USER` | SSH-пользователь (например `ubuntu`) |
| `SERVER_SSH_KEY` | Приватный SSH-ключ (публичный должен быть в `~/.ssh/authorized_keys` на сервере) |
| `DEPLOY_PATH` | Путь к проекту на сервере (например `/opt/habbit-tracker-bot`) |
| `TELEGRAM_TOKEN` | Токен бота |
| `DB_DSN` | DSN подключения к Postgres |
| `REDIS_ADDR` | Адрес Redis (опционально, по умолчанию `redis:6379`) |

### Подготовка сервера (первый раз)

```bash
ssh user@your-server
git clone https://github.com/your/habbit-tracker-bot /opt/habbit-tracker-bot
cd /opt/habbit-tracker-bot
docker-compose up -d postgres redis   # поднять инфру
```

После этого каждый push в `main` деплоится автоматически.

## Архитектура

```
cmd/main.go                        → точка входа
internal/
  app/app.go                       → FX wiring (providers + lifecycle)
  domain/                          → сущности (User, Habit, Activity) + ErrNotFound
  usecase/                         → бизнес-логика + интерфейсы репозиториев
  delivery/telegram/               → polling, роутинг команд
  repository/postgres/             → реализация интерфейсов через pgxpool
  repository/redis/                → кэш
  infrastructure/config/           → конфиг из env
  infrastructure/logger/           → zap
migrations/001_init.sql            → DDL (users, habits, activities)
```

Зависимости идут **внутрь**: `delivery → usecase → domain`. Usecase знает только об интерфейсах, не об postgres/redis.

## Добавление новой команды

1. Добавь метод в `usecase/`
2. Добавь `case "cmd":` в `handler.HandleUpdate`
3. Реализуй `handle*` метод — принимает `(ctx, msg, user)`
