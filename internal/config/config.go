package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DBPath             string
	Listen             string
	AdminPassword      string
	TelegramToken      string
	TelegramChatID     string
	HTTPTimeout        time.Duration
	CheckTimeout       time.Duration
	MaxResponseBytes   int64
	DisplayTimezone    string
	CheckSchedule      string
	AllowPrivateTarget bool
}

func Load() (Config, error) {
	c := Config{
		DBPath:           env("HANDY_DB_PATH", "data/handy.db"),
		Listen:           env("HANDY_LISTEN", "127.0.0.1:8080"),
		TelegramChatID:   strings.TrimSpace(os.Getenv("HANDY_TELEGRAM_CHAT_ID")),
		DisplayTimezone:  env("HANDY_DISPLAY_TIMEZONE", "Europe/Moscow"),
		CheckSchedule:    env("HANDY_CHECK_SCHEDULE", "00:00,06:00,12:00,18:00"),
		MaxResponseBytes: 2 * 1024 * 1024,
	}
	var err error
	c.HTTPTimeout, err = durationEnv("HANDY_HTTP_TIMEOUT", 20*time.Second)
	if err != nil {
		return Config{}, err
	}
	c.CheckTimeout, err = durationEnv("HANDY_CHECK_TIMEOUT", 4*time.Minute)
	if err != nil {
		return Config{}, err
	}
	if raw := strings.TrimSpace(os.Getenv("HANDY_MAX_RESPONSE_BYTES")); raw != "" {
		c.MaxResponseBytes, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || c.MaxResponseBytes < 1024 {
			return Config{}, fmt.Errorf("invalid HANDY_MAX_RESPONSE_BYTES")
		}
	}
	c.AllowPrivateTarget = strings.EqualFold(os.Getenv("HANDY_ALLOW_PRIVATE_TARGETS"), "true")
	c.AdminPassword, err = secret("HANDY_ADMIN_PASSWORD", "HANDY_ADMIN_PASSWORD_FILE")
	if err != nil {
		return Config{}, err
	}
	c.TelegramToken, err = secret("HANDY_TELEGRAM_TOKEN", "HANDY_TELEGRAM_TOKEN_FILE")
	if err != nil {
		return Config{}, err
	}
	return c, nil
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func durationEnv(key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	v, err := time.ParseDuration(raw)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("invalid %s", key)
	}
	return v, nil
}

func secret(valueKey, fileKey string) (string, error) {
	if p := strings.TrimSpace(os.Getenv(fileKey)); p != "" {
		b, err := os.ReadFile(p)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", fileKey, err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	return strings.TrimSpace(os.Getenv(valueKey)), nil
}
