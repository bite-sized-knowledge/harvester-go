package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"harvester-go/internal/config"
	"harvester-go/internal/database"
	"harvester-go/internal/fetcher"
	worker "harvester-go/internal/harvester"
	"harvester-go/internal/notify"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := database.New(ctx, cfg, logger)
	if err != nil {
		logger.Error("failed to connect database", "error", err)
		return
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Error("failed to close database", "error", err)
		}
	}()

	client, err := fetcher.NewClient(cfg.ProxyURL, logger)
	if err != nil {
		logger.Error("failed to create http client", "error", err)
		return
	}

	runner := worker.NewRunner(db, client, notify.NewDiscordNotifier(cfg.DiscordWebhookURL), logger)

	runCycle := func() {
		cycleCtx, cancel := context.WithTimeout(ctx, 55*time.Minute)
		defer cancel()
		if err := runner.Run(cycleCtx); err != nil && err != context.Canceled {
			logger.Error("harvest cycle failed", "error", err)
		}
	}

	runCycle()

	ticker := time.NewTicker(cfg.HarvestInterval)
	defer ticker.Stop()

	logger.Info("harvester ticker started", "interval", cfg.HarvestInterval.String())

	for {
		select {
		case <-ctx.Done():
			logger.Info("shutting down harvester")
			return
		case <-ticker.C:
			runCycle()
		}
	}
}
