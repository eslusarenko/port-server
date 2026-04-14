package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	cfg := Load()

	if cfg.Addr != ":8080" {
		t.Errorf("Addr = %q, want :8080", cfg.Addr)
	}
	if cfg.BaseDomain != "tunnel.localhost" {
		t.Errorf("BaseDomain = %q, want tunnel.localhost", cfg.BaseDomain)
	}
	if cfg.TunnelTTL != 24*time.Hour {
		t.Errorf("TunnelTTL = %v, want 24h", cfg.TunnelTTL)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", cfg.LogLevel)
	}
	if cfg.MaxBodySize != 10<<20 {
		t.Errorf("MaxBodySize = %d, want %d", cfg.MaxBodySize, 10<<20)
	}
	if cfg.Ping.Interval != 30*time.Second {
		t.Errorf("Ping.Interval = %v, want 30s", cfg.Ping.Interval)
	}
	if cfg.Ping.Timeout != 90*time.Second {
		t.Errorf("Ping.Timeout = %v, want 90s", cfg.Ping.Timeout)
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("PORT_ADDR", ":9090")
	t.Setenv("PORT_BASE_DOMAIN", "port.example.com")
	t.Setenv("PORT_TUNNEL_TTL", "1h")
	t.Setenv("PORT_LOG_LEVEL", "debug")
	t.Setenv("PORT_MAX_BODY_SIZE", "5242880")
	t.Setenv("PORT_PING_INTERVAL", "10s")
	t.Setenv("PORT_PING_TIMEOUT", "30s")

	cfg := Load()

	if cfg.Addr != ":9090" {
		t.Errorf("Addr = %q, want :9090", cfg.Addr)
	}
	if cfg.BaseDomain != "port.example.com" {
		t.Errorf("BaseDomain = %q, want port.example.com", cfg.BaseDomain)
	}
	if cfg.TunnelTTL != time.Hour {
		t.Errorf("TunnelTTL = %v, want 1h", cfg.TunnelTTL)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", cfg.LogLevel)
	}
	if cfg.MaxBodySize != 5242880 {
		t.Errorf("MaxBodySize = %d, want 5242880", cfg.MaxBodySize)
	}
	if cfg.Ping.Interval != 10*time.Second {
		t.Errorf("Ping.Interval = %v, want 10s", cfg.Ping.Interval)
	}
	if cfg.Ping.Timeout != 30*time.Second {
		t.Errorf("Ping.Timeout = %v, want 30s", cfg.Ping.Timeout)
	}
}
