BINARY      = app
MAIN        = ./cmd/main.go
MIGRATE_DSN ?= postgres://user:pass@localhost:5432/habbit?sslmode=disable

.PHONY: run build test lint tidy migrate rollback docker-up docker-down docker-logs

run:
	go run $(MAIN)

build:
	go build -ldflags="-s -w" -o $(BINARY) $(MAIN)

test:
	go test ./...

lint:
	golangci-lint run ./...

tidy:
	go mod tidy

migrate:
	go run ./cmd/migrate -dsn "$(MIGRATE_DSN)"

rollback:
	go run ./cmd/migrate -dsn "$(MIGRATE_DSN)" -down

docker-up:
	docker-compose up --build -d

docker-down:
	docker-compose down

docker-logs:
	docker-compose logs -f app

.DEFAULT_GOAL := run
