package main

import (
	"context"
	"flag"
	"fmt"
	"io"
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
		// flag.ExitOnError handles parse errors, but post-parse validation errors reach here.
		fmt.Fprintf(os.Stderr, "port-server: %v\n", err)
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
	var logFile *os.File
	if cfg.LogFile != "" {
		logFile, err = os.OpenFile(cfg.LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to open log file %q: %v\n", cfg.LogFile, err)
			os.Exit(1)
		}
		defer logFile.Close()
	}

	out := os.Stderr
	if logFile != nil {
		out = logFile
	}

	var handler slog.Handler
	switch cfg.LogType {
	case "json":
		handler = slog.NewJSONHandler(out, &slog.HandlerOptions{Level: level})
	case "silent":
		handler = slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: level})
	default:
		handler = slog.NewTextHandler(out, &slog.HandlerOptions{Level: level})
	}
	logger := slog.New(handler)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := app.New(cfg, logger).Run(ctx); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}
