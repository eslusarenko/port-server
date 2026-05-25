package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/eslusarenko/port-server/internal/admin"
	"github.com/eslusarenko/port-server/internal/app"
	"github.com/eslusarenko/port-server/internal/config"
	"github.com/eslusarenko/port-server/internal/version"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "admin" {
		// Admin subcommand: load DB DSN from env or --db-dsn flag in remaining args.
		dsn := os.Getenv("PORT_DB_DSN")
		remainingArgs := os.Args[2:]
		for i, a := range remainingArgs {
			if a == "--db-dsn" && i+1 < len(remainingArgs) {
				dsn = remainingArgs[i+1]
				break
			}
			if strings.HasPrefix(a, "--db-dsn=") {
				dsn = strings.TrimPrefix(a, "--db-dsn=")
				break
			}
		}
		if len(remainingArgs) > 0 && (remainingArgs[0] == "--help" || remainingArgs[0] == "-h") {
			admin.PrintHelp()
			os.Exit(0)
		}
		if dsn == "" {
			_, _ = fmt.Fprintln(os.Stderr, "port-server admin: PORT_DB_DSN is required")
			os.Exit(1)
		}
		if err := admin.Run(dsn, remainingArgs); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "port-server admin: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

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
		defer func() {
			if cerr := logFile.Close(); cerr != nil {
				fmt.Fprintf(os.Stderr, "port-server: failed to close log file: %v\n", cerr)
			}
		}()
	}

	out := os.Stderr
	if logFile != nil {
		out = logFile
	}

	replaceMsg := func(_ []string, a slog.Attr) slog.Attr {
		if a.Key == slog.MessageKey {
			a.Key = "event"
		}
		return a
	}

	var handler slog.Handler
	switch cfg.LogType {
	case "json":
		handler = slog.NewJSONHandler(out, &slog.HandlerOptions{Level: level, ReplaceAttr: replaceMsg})
	case "silent":
		handler = slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: level})
	default:
		handler = slog.NewTextHandler(out, &slog.HandlerOptions{Level: level, ReplaceAttr: replaceMsg})
	}
	logger := slog.New(handler)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := app.New(cfg, logger).Run(ctx); err != nil {
		logger.Error("server_failed", "error", err)
		os.Exit(1)
	}
}
