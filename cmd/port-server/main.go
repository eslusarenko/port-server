package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/eslusarenko/port-server/internal/app"
	"github.com/eslusarenko/port-server/internal/config"
)

func main() {
	cfg := config.Load()

	var level slog.Level
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := app.New(cfg, logger).Run(ctx); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}
