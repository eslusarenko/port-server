package config

import (
	"flag"
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

func TestLoadFromArgs_FlagOverridesDefault(t *testing.T) {
	cfg, versionRequested, err := LoadFromArgs([]string{"-addr=:1234"}, flag.ContinueOnError)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if versionRequested {
		t.Fatal("versionRequested should be false")
	}
	if cfg.Addr != ":1234" {
		t.Errorf("Addr = %q, want :1234", cfg.Addr)
	}
	// Everything else should stay at defaults.
	if cfg.BaseDomain != "tunnel.localhost" {
		t.Errorf("BaseDomain = %q, want tunnel.localhost", cfg.BaseDomain)
	}
}

func TestLoadFromArgs_FlagOverridesEnv(t *testing.T) {
	t.Setenv("PORT_ADDR", ":9999")
	cfg, _, err := LoadFromArgs([]string{"-addr=:7777"}, flag.ContinueOnError)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Addr != ":7777" {
		t.Errorf("Addr = %q, want :7777 (flag should win over env)", cfg.Addr)
	}
}

func TestLoadFromArgs_EnvWithoutFlag(t *testing.T) {
	t.Setenv("PORT_ADDR", ":5555")
	cfg, _, err := LoadFromArgs([]string{}, flag.ContinueOnError)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Addr != ":5555" {
		t.Errorf("Addr = %q, want :5555 (env should win over default)", cfg.Addr)
	}
}

func TestLoadFromArgs_UnknownFlagErrors(t *testing.T) {
	_, _, err := LoadFromArgs([]string{"-unknown-flag=x"}, flag.ContinueOnError)
	if err == nil {
		t.Error("expected error for unknown flag, got nil")
	}
}

func TestLoadFromArgs_AllFlags(t *testing.T) {
	args := []string{
		"-addr=:2222",
		"-base-domain=example.com",
		"-tunnel-ttl=2h",
		"-log-level=debug",
		"-max-body-size=1048576",
		"-ping-interval=15s",
		"-ping-timeout=45s",
	}
	cfg, _, err := LoadFromArgs(args, flag.ContinueOnError)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Addr != ":2222" {
		t.Errorf("Addr = %q, want :2222", cfg.Addr)
	}
	if cfg.BaseDomain != "example.com" {
		t.Errorf("BaseDomain = %q, want example.com", cfg.BaseDomain)
	}
	if cfg.TunnelTTL != 2*time.Hour {
		t.Errorf("TunnelTTL = %v, want 2h", cfg.TunnelTTL)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", cfg.LogLevel)
	}
	if cfg.MaxBodySize != 1048576 {
		t.Errorf("MaxBodySize = %d, want 1048576", cfg.MaxBodySize)
	}
	if cfg.Ping.Interval != 15*time.Second {
		t.Errorf("Ping.Interval = %v, want 15s", cfg.Ping.Interval)
	}
	if cfg.Ping.Timeout != 45*time.Second {
		t.Errorf("Ping.Timeout = %v, want 45s", cfg.Ping.Timeout)
	}
}

func TestLoadFromArgs_VersionFlag(t *testing.T) {
	cfg, versionRequested, err := LoadFromArgs([]string{"--version"}, flag.ContinueOnError)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !versionRequested {
		t.Error("versionRequested should be true when --version is passed")
	}
	if cfg != nil {
		t.Error("cfg should be nil when version is requested")
	}
}
