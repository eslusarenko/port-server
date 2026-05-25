package config

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr              string
	BaseDomain        string
	DBDSN             string
	AllowUnauthed     bool
	TunnelTTL         time.Duration
	LogLevel          string
	LogType           string
	LogFile           string
	MaxBodySize       int64
	Ping              PingConfig
	TrustProxyHeaders bool
}

type PingConfig struct {
	Interval time.Duration
	Timeout  time.Duration
}

// defaults returns a Config with hardcoded defaults (no env).
func defaults() *Config {
	return &Config{
		Addr:        ":8080",
		BaseDomain:  "tunnel.localhost",
		TunnelTTL:   24 * time.Hour,
		LogLevel:    "info",
		LogType:     "plain",
		LogFile:     "",
		MaxBodySize: 10 << 20,
		Ping: PingConfig{
			Interval: 30 * time.Second,
			Timeout:  90 * time.Second,
		},
		TrustProxyHeaders: false,
	}
}

// applyEnv overwrites cfg fields with environment variable values where set.
func applyEnv(cfg *Config) {
	if v := os.Getenv("PORT_ADDR"); v != "" {
		cfg.Addr = v
	}
	if v := os.Getenv("PORT_BASE_DOMAIN"); v != "" {
		cfg.BaseDomain = v
	}
	if v := os.Getenv("PORT_TUNNEL_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.TunnelTTL = d
		}
	}
	if v := os.Getenv("PORT_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("PORT_LOG_TYPE"); v != "" {
		cfg.LogType = v
	}
	if v := os.Getenv("PORT_LOG_FILENAME"); v != "" {
		cfg.LogFile = v
	}
	if v := os.Getenv("PORT_MAX_BODY_SIZE"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.MaxBodySize = n
		}
	}
	if v := os.Getenv("PORT_PING_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Ping.Interval = d
		}
	}
	if v := os.Getenv("PORT_PING_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Ping.Timeout = d
		}
	}
	if v := os.Getenv("PORT_DB_DSN"); v != "" {
		cfg.DBDSN = v
	}
	switch os.Getenv("PORT_ALLOW_UNAUTHED") {
	case "1", "true", "yes":
		cfg.AllowUnauthed = true
	case "0", "false", "no":
		cfg.AllowUnauthed = false
	}
	switch os.Getenv("PORT_TRUST_PROXY_HEADERS") {
	case "1", "true", "yes":
		cfg.TrustProxyHeaders = true
	case "0", "false", "no":
		cfg.TrustProxyHeaders = false
	}
}

// Load returns a Config populated from environment variables and defaults.
func Load() *Config {
	cfg := defaults()
	applyEnv(cfg)
	return cfg
}

// parseConfigFile reads a KEY=VALUE config file and applies values to cfg.
// Lines starting with # (after trim) are comments. Blank lines are skipped.
// Unknown keys warn to stderr. Bad values return an error.
func parseConfigFile(path string, cfg *Config) (retErr error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && retErr == nil {
			retErr = cerr
		}
	}()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			return fmt.Errorf("config: %s: line %d: malformed line (expected KEY=VALUE)", path, lineNum)
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])

		switch key {
		case "PORT_ADDR":
			cfg.Addr = val
		case "PORT_BASE_DOMAIN":
			cfg.BaseDomain = val
		case "PORT_TUNNEL_TTL":
			d, err := time.ParseDuration(val)
			if err != nil {
				return fmt.Errorf("config: %s: line %d: invalid value for %s: %w", path, lineNum, key, err)
			}
			cfg.TunnelTTL = d
		case "PORT_LOG_LEVEL":
			cfg.LogLevel = val
		case "PORT_LOG_TYPE":
			cfg.LogType = val
		case "PORT_LOG_FILENAME":
			cfg.LogFile = val
		case "PORT_MAX_BODY_SIZE":
			n, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				return fmt.Errorf("config: %s: line %d: invalid value for %s: %w", path, lineNum, key, err)
			}
			cfg.MaxBodySize = n
		case "PORT_PING_INTERVAL":
			d, err := time.ParseDuration(val)
			if err != nil {
				return fmt.Errorf("config: %s: line %d: invalid value for %s: %w", path, lineNum, key, err)
			}
			cfg.Ping.Interval = d
		case "PORT_PING_TIMEOUT":
			d, err := time.ParseDuration(val)
			if err != nil {
				return fmt.Errorf("config: %s: line %d: invalid value for %s: %w", path, lineNum, key, err)
			}
			cfg.Ping.Timeout = d
		case "PORT_DB_DSN":
			cfg.DBDSN = val
		case "PORT_ALLOW_UNAUTHED":
			switch val {
			case "1", "true", "yes":
				cfg.AllowUnauthed = true
			case "0", "false", "no":
				cfg.AllowUnauthed = false
			default:
				return fmt.Errorf("config: %s: line %d: invalid value for %s: %q", path, lineNum, key, val)
			}
		case "PORT_TRUST_PROXY_HEADERS":
			switch val {
			case "1", "true", "yes":
				cfg.TrustProxyHeaders = true
			case "0", "false", "no":
				cfg.TrustProxyHeaders = false
			default:
				return fmt.Errorf("config: %s: line %d: invalid value for %s: %q (want true/false/1/0/yes/no)", path, lineNum, key, val)
			}
		default:
			_, _ = fmt.Fprintf(os.Stderr, "port-server: config: unknown key %q, ignoring\n", key)
		}
	}
	return scanner.Err()
}

// autoConfigPath returns the default config file path next to the binary, or "".
func autoConfigPath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(exe), "port-server.conf")
}

// LoadFromArgs parses args (typically os.Args[1:]) applying priority:
// env > flag > config file > default.
// Pass flag.ExitOnError for normal CLI use, flag.ContinueOnError for tests.
// The bool return is true when --version was requested; in that case cfg is nil.
func LoadFromArgs(args []string, errorHandling flag.ErrorHandling) (*Config, bool, error) {
	configPath := ""
	for i, a := range args {
		switch {
		case a == "--config" || a == "-config":
			if i+1 < len(args) {
				configPath = args[i+1]
			}
		case strings.HasPrefix(a, "--config="):
			configPath = strings.TrimPrefix(a, "--config=")
		case strings.HasPrefix(a, "-config="):
			configPath = strings.TrimPrefix(a, "-config=")
		}
	}
	if configPath == "" {
		configPath = autoConfigPath()
	}

	cfg := defaults()

	if err := parseConfigFile(configPath, cfg); err != nil {
		return nil, false, err
	}

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
	fs.StringVar(&cfg.DBDSN, "db-dsn", cfg.DBDSN, "MySQL connection string (env PORT_DB_DSN)")
	fs.BoolVar(&cfg.AllowUnauthed, "allow-unauthed", cfg.AllowUnauthed, "allow unauthenticated tunnels when no DB is configured (env PORT_ALLOW_UNAUTHED)")
	fs.BoolVar(&cfg.TrustProxyHeaders, "trust-proxy-headers", cfg.TrustProxyHeaders, "trust X-Forwarded-For / X-Real-IP headers from reverse proxy (env PORT_TRUST_PROXY_HEADERS)")
	fs.StringVar(&cfg.LogType, "log-type", cfg.LogType, "log format: plain|json|silent (env PORT_LOG_TYPE)")
	fs.BoolVar(&jsonLogs, "json-logs", false, "shortcut for --log-type=json (overridden by explicit --log-type)")
	fs.StringVar(&cfg.LogFile, "log-file", cfg.LogFile, "write logs to this file instead of stderr (env PORT_LOG_FILENAME)")
	fs.String("config", "", "path to config file (default: port-server.conf next to binary)")

	if err := fs.Parse(args); err != nil {
		return nil, false, err
	}

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

	applyEnv(cfg)

	switch cfg.LogType {
	case "plain", "json", "silent":
	default:
		return nil, false, fmt.Errorf("invalid --log-type %q: must be plain, json, or silent", cfg.LogType)
	}
	if versionRequested {
		return nil, true, nil
	}
	return cfg, false, nil
}
