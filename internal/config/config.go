package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Addr        string
	BaseDomain  string
	TunnelTTL   time.Duration
	LogLevel    string
	MaxBodySize int64
	Ping        PingConfig
}

type PingConfig struct {
	Interval time.Duration
	Timeout  time.Duration
}

func Load() *Config {
	return &Config{
		Addr:        envOr("PORT_ADDR", ":8080"),
		BaseDomain:  envOr("PORT_BASE_DOMAIN", "tunnel.localhost"),
		TunnelTTL:   envDuration("PORT_TUNNEL_TTL", 24*time.Hour),
		LogLevel:    envOr("PORT_LOG_LEVEL", "info"),
		MaxBodySize: envInt64("PORT_MAX_BODY_SIZE", 10<<20), // 10 MB
		Ping: PingConfig{
			Interval: envDuration("PORT_PING_INTERVAL", 30*time.Second),
			Timeout:  envDuration("PORT_PING_TIMEOUT", 90*time.Second),
		},
	}
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
