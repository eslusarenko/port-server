package config

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Addr        string
	BaseDomain  string
	TunnelTTL   time.Duration
	LogLevel    string
	LogType     string
	LogFile     string
	MaxBodySize int64
	Ping        PingConfig
	TrustProxyHeaders bool
}

type PingConfig struct {
	Interval time.Duration
	Timeout  time.Duration
}

// Load returns a Config populated from environment variables and defaults.
func Load() *Config {
	return &Config{
		Addr:        envOr("PORT_ADDR", ":8080"),
		BaseDomain:  envOr("PORT_BASE_DOMAIN", "tunnel.localhost"),
		TunnelTTL:   envDuration("PORT_TUNNEL_TTL", 24*time.Hour),
		LogLevel:    envOr("PORT_LOG_LEVEL", "info"),
		LogType:     envOr("PORT_LOG_TYPE", "plain"),
		LogFile:     envOr("PORT_LOG_FILENAME", ""),
		MaxBodySize: envInt64("PORT_MAX_BODY_SIZE", 10<<20), // 10 MB
		Ping: PingConfig{
			Interval: envDuration("PORT_PING_INTERVAL", 30*time.Second),
			Timeout:  envDuration("PORT_PING_TIMEOUT", 90*time.Second),
		},
		TrustProxyHeaders: envBool("PORT_TRUST_PROXY_HEADERS", false),
	}
}

// LoadFromArgs parses args (typically os.Args[1:]) on top of env/defaults.
// Precedence: flag > env > default.
// Pass flag.ExitOnError for normal CLI use, flag.ContinueOnError for tests.
// The bool return is true when --version was requested; in that case cfg is nil.
func LoadFromArgs(args []string, errorHandling flag.ErrorHandling) (*Config, bool, error) {
	cfg := Load()

	fs := flag.NewFlagSet("port-server", errorHandling)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(fs.Output(), "port-server — public tunnel server\n\nFlags:\n")
		fs.VisitAll(func(f *flag.Flag) {
			typeName, usage := flag.UnquoteUsage(f)
			if typeName != "" {
				_, _ = fmt.Fprintf(fs.Output(), "  --%s %s\n", f.Name, typeName)
			} else {
				_, _ = fmt.Fprintf(fs.Output(), "  --%s\n", f.Name)
			}

			_, _ = fmt.Fprintf(fs.Output(), "\t%s", usage)
			if f.DefValue != "" {
				defVal := f.DefValue
				if typeName == "string" {
					defVal = strconv.Quote(defVal)
				}
				_, _ = fmt.Fprintf(fs.Output(), " (default %s)", defVal)
			}
			_, _ = fmt.Fprintln(fs.Output())
		})
	}

	var versionRequested bool
	var jsonLogs bool
	fs.BoolVar(&versionRequested, "version", false, "print version and exit")
	fs.StringVar(&cfg.Addr, "addr", cfg.Addr, "listen address (env PORT_ADDR)")
	fs.StringVar(&cfg.BaseDomain, "base-domain", cfg.BaseDomain, "base domain for tunnel hostnames (env PORT_BASE_DOMAIN)")
	fs.DurationVar(&cfg.TunnelTTL, "tunnel-ttl", cfg.TunnelTTL, "how long an idle tunnel lives (env PORT_TUNNEL_TTL)")
	fs.StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "log verbosity: debug|info|warn|error (env PORT_LOG_LEVEL)")
	fs.Int64Var(&cfg.MaxBodySize, "max-body-size", cfg.MaxBodySize, "max request body size in bytes (env PORT_MAX_BODY_SIZE)")
	fs.DurationVar(&cfg.Ping.Interval, "ping-interval", cfg.Ping.Interval, "WebSocket ping interval (env PORT_PING_INTERVAL)")
	fs.DurationVar(&cfg.Ping.Timeout, "ping-timeout", cfg.Ping.Timeout, "WebSocket ping timeout (env PORT_PING_TIMEOUT)")
	fs.BoolVar(&cfg.TrustProxyHeaders, "trust-proxy-headers", cfg.TrustProxyHeaders, "trust X-Forwarded-For / X-Real-IP headers from reverse proxy (env PORT_TRUST_PROXY_HEADERS)")
	fs.StringVar(&cfg.LogType, "log-type", cfg.LogType, "log format: plain|json|silent (env PORT_LOG_TYPE)")
	fs.BoolVar(&jsonLogs, "json-logs", false, "shortcut for --log-type=json (overridden by explicit --log-type)")
	fs.StringVar(&cfg.LogFile, "log-file", cfg.LogFile, "write logs to this file instead of stderr (env PORT_LOG_FILENAME)")

	if err := fs.Parse(args); err != nil {
		return nil, false, err
	}
	// If --json-logs was set AND --log-type was NOT explicitly set, treat it as --log-type=json.
	if jsonLogs {
		logTypeExplicit := false
		fs.Visit(func(f *flag.Flag) {
			if f.Name == "log-type" {
				logTypeExplicit = true
			}
		})
		if !logTypeExplicit {
			cfg.LogType = "json"
		}
	}
	// Validate log-type.
	switch cfg.LogType {
	case "plain", "json", "silent":
		// valid
	default:
		return nil, false, fmt.Errorf("invalid --log-type %q: must be plain, json, or silent", cfg.LogType)
	}
	if versionRequested {
		return nil, true, nil
	}
	return cfg, false, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

func envInt64(key string, fallback int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	switch os.Getenv(key) {
	case "1", "true", "yes":
		return true
	case "0", "false", "no":
		return false
	}
	return fallback
}
