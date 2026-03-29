package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DBHost            string
	DBPort            int
	DBUser            string
	DBPassword        string
	DBName            string
	ProxyURL          string
	DiscordWebhookURL string
	HarvestInterval   time.Duration
	LogLevel          slog.Level
}

func Load() (Config, error) {
	cfg := Config{}

	cfg.DBHost = strings.TrimSpace(os.Getenv("DB_HOST"))
	cfg.DBUser = strings.TrimSpace(os.Getenv("DB_USER"))
	cfg.DBPassword = os.Getenv("DB_PASSWORD")
	cfg.DBName = strings.TrimSpace(os.Getenv("DB_NAME"))
	cfg.ProxyURL = strings.TrimSpace(os.Getenv("PROXY_URL"))
	cfg.DiscordWebhookURL = strings.TrimSpace(os.Getenv("DISCORD_WEBHOOK_URL"))

	dbPortRaw := strings.TrimSpace(os.Getenv("DB_PORT"))
	if dbPortRaw == "" {
		cfg.DBPort = 3306
	} else {
		port, err := strconv.Atoi(dbPortRaw)
		if err != nil {
			return Config{}, fmt.Errorf("invalid DB_PORT %q: %w", dbPortRaw, err)
		}
		cfg.DBPort = port
	}

	harvestIntervalRaw := strings.TrimSpace(os.Getenv("HARVEST_INTERVAL"))
	if harvestIntervalRaw == "" {
		harvestIntervalRaw = "1h"
	}
	interval, err := time.ParseDuration(harvestIntervalRaw)
	if err != nil {
		return Config{}, fmt.Errorf("invalid HARVEST_INTERVAL %q: %w", harvestIntervalRaw, err)
	}
	cfg.HarvestInterval = interval

	logLevelRaw := strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL")))
	if logLevelRaw == "" {
		logLevelRaw = "info"
	}
	level, err := parseLogLevel(logLevelRaw)
	if err != nil {
		return Config{}, err
	}
	cfg.LogLevel = level

	if cfg.DBHost == "" || cfg.DBUser == "" || cfg.DBName == "" {
		return Config{}, fmt.Errorf("DB_HOST, DB_USER, and DB_NAME are required")
	}

	return cfg, nil
}

func parseLogLevel(raw string) (slog.Level, error) {
	switch raw {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("invalid LOG_LEVEL %q", raw)
	}
}
