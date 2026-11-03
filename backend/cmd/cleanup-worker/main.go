package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	cleanupworker "hr-assistant/backend/internal/app/cleanup/worker"
	"hr-assistant/backend/internal/core/config"
	"hr-assistant/backend/internal/core/database"
	corelogger "hr-assistant/backend/internal/core/logger"
)

func main() {
	logger := corelogger.New(os.Stdout)
	if err := run(logger); err != nil {
		logger.Error("cleanup worker stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	settings, err := config.LoadCleanup()
	if err != nil {
		return err
	}
	openContext, cancelOpen := context.WithTimeout(context.Background(), 10*time.Second)
	pool, err := database.Open(openContext, settings.DatabaseURL)
	cancelOpen()
	if err != nil {
		return err
	}
	defer pool.Close()

	cleaner, err := cleanupworker.New(pool, cleanupworker.Config{
		Interval:                  settings.Interval,
		BatchSize:                 settings.BatchSize,
		InboxRetention:            settings.InboxRetention,
		OutboxRetention:           settings.OutboxRetention,
		TelegramDeliveryRetention: settings.TelegramDeliveryRetention,
	}, logger)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return cleaner.Run(ctx)
}
