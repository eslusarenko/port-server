package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/eslusarenko/port-server/internal/app"
	"github.com/eslusarenko/port-server/internal/config"
	"github.com/eslusarenko/port-server/internal/version"
)

func main() {
	cfg, versionRequested, err := config.LoadFromArgs(os.Args[1:], flag.ExitOnError)
	if err != nil {
		// flag.ExitOnError already calls os.Exit; this path is unreachable
		// in practice but satisfies the compiler.
		os.Exit(2)
	}
	if versionRequested {
		fmt.Println(version.Version)
		os.Exit(0)
	}

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
