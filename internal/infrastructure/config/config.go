package config

import (
	"errors"
	"fmt"
	"os"
	"time"
)

type Config struct {
	TelegramToken string
	DBDSN         string
	RedisAddr     string
	Timezone      string
}

func New() (*Config, error) {
	cfg := &Config{
		TelegramToken: os.Getenv("TELEGRAM_TOKEN"),
		DBDSN:         os.Getenv("DB_DSN"),
		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6379"),
		Timezone:      getEnv("TIMEZONE", "UTC"),
	}
	if cfg.TelegramToken == "" {
		return nil, errors.New("TELEGRAM_TOKEN is required")
	}
	if cfg.DBDSN == "" {
		return nil, errors.New("DB_DSN is required")
	}
	if _, err := time.LoadLocation(cfg.Timezone); err != nil {
		return nil, fmt.Errorf("invalid TIMEZONE %q: %w", cfg.Timezone, err)
	}
	return cfg, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
