package config

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
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
	if cfg.TrustProxyHeaders != false {
		t.Errorf("TrustProxyHeaders = %v, want false", cfg.TrustProxyHeaders)
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
	t.Setenv("PORT_TRUST_PROXY_HEADERS", "true")

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
	if cfg.TrustProxyHeaders != true {
		t.Errorf("TrustProxyHeaders = %v, want true", cfg.TrustProxyHeaders)
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
	if cfg.BaseDomain != "tunnel.localhost" {
		t.Errorf("BaseDomain = %q, want tunnel.localhost", cfg.BaseDomain)
	}
}

func TestLoadFromArgs_EnvOverridesFlag(t *testing.T) {
	t.Setenv("PORT_ADDR", ":9999")
	cfg, _, err := LoadFromArgs([]string{"-addr=:7777"}, flag.ContinueOnError)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Addr != ":9999" {
		t.Errorf("Addr = %q, want :9999 (env should win over flag)", cfg.Addr)
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

func TestLoadFromArgs_TrustProxyHeadersFlag(t *testing.T) {
	cfg, _, err := LoadFromArgs([]string{"--trust-proxy-headers"}, flag.ContinueOnError)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.TrustProxyHeaders {
		t.Error("TrustProxyHeaders should be true when --trust-proxy-headers flag is passed")
	}
}

func TestLoadDefaults_LogFields(t *testing.T) {
	cfg := Load()
	if cfg.LogType != "plain" {
		t.Errorf("LogType = %q, want plain", cfg.LogType)
	}
	if cfg.LogFile != "" {
		t.Errorf("LogFile = %q, want empty", cfg.LogFile)
	}
}

func TestLoadFromEnv_LogFields(t *testing.T) {
	t.Setenv("PORT_LOG_TYPE", "json")
	t.Setenv("PORT_LOG_FILENAME", "/tmp/test.log")
	cfg := Load()
	if cfg.LogType != "json" {
		t.Errorf("LogType = %q, want json", cfg.LogType)
	}
	if cfg.LogFile != "/tmp/test.log" {
		t.Errorf("LogFile = %q, want /tmp/test.log", cfg.LogFile)
	}
}

func TestLoadFromArgs_EnvOverridesLogTypeFlag(t *testing.T) {
	t.Setenv("PORT_LOG_TYPE", "json")
	cfg, _, err := LoadFromArgs([]string{"--log-type=plain"}, flag.ContinueOnError)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LogType != "json" {
		t.Errorf("LogType = %q, want json (env should win over flag)", cfg.LogType)
	}
}

func TestLoadFromArgs_JsonLogsShortcut(t *testing.T) {
	cfg, _, err := LoadFromArgs([]string{"--json-logs"}, flag.ContinueOnError)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LogType != "json" {
		t.Errorf("LogType = %q, want json", cfg.LogType)
	}
}

func TestLoadFromArgs_ExplicitLogTypeBeatJsonLogs(t *testing.T) {
	cfg, _, err := LoadFromArgs([]string{"--json-logs", "--log-type=plain"}, flag.ContinueOnError)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LogType != "plain" {
		t.Errorf("LogType = %q, want plain (explicit --log-type should win over --json-logs)", cfg.LogType)
	}
}

func TestLoadFromArgs_InvalidLogTypeErrors(t *testing.T) {
	_, _, err := LoadFromArgs([]string{"--log-type=garbage"}, flag.ContinueOnError)
	if err == nil {
		t.Error("expected error for invalid --log-type, got nil")
	}
}

func TestLoadFromArgs_ConfigFileAppliesValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "port-server.conf")
	content := "PORT_ADDR=:3333\n"
	if err := osWriteFile(path, content); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	cfg, _, err := LoadFromArgs([]string{"--config", path}, flag.ContinueOnError)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Addr != ":3333" {
		t.Errorf("Addr = %q, want :3333", cfg.Addr)
	}
}

func TestLoadFromArgs_FlagOverridesConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "port-server.conf")
	content := "PORT_ADDR=:3333\n"
	if err := osWriteFile(path, content); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	cfg, _, err := LoadFromArgs([]string{"--config", path, "--addr=:4444"}, flag.ContinueOnError)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Addr != ":4444" {
		t.Errorf("Addr = %q, want :4444", cfg.Addr)
	}
}

func TestLoadFromArgs_EnvOverridesFlagAndConfig(t *testing.T) {
	t.Setenv("PORT_ADDR", ":9999")

	path := filepath.Join(t.TempDir(), "port-server.conf")
	content := "PORT_ADDR=:3333\n"
	if err := osWriteFile(path, content); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	cfg, _, err := LoadFromArgs([]string{"--config", path, "--addr=:4444"}, flag.ContinueOnError)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Addr != ":9999" {
		t.Errorf("Addr = %q, want :9999", cfg.Addr)
	}
}

func TestLoadFromArgs_UnknownKeyWarnsButDoesntFail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "port-server.conf")
	content := "UNKNOWN_KEY=foo\n"
	if err := osWriteFile(path, content); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	_, _, err := LoadFromArgs([]string{"--config", path}, flag.ContinueOnError)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadFromArgs_MissingConfigFileIsOK(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.conf")

	cfg, _, err := LoadFromArgs([]string{"--config", path}, flag.ContinueOnError)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("cfg should not be nil")
	}
}

func TestLoadFromArgs_MalformedLineErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "port-server.conf")
	content := "NOTAVALIDLINE\n"
	if err := osWriteFile(path, content); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	_, _, err := LoadFromArgs([]string{"--config", path}, flag.ContinueOnError)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "malformed line") {
		t.Fatalf("error = %q, want malformed line", err)
	}
}

func TestLoadFromArgs_BadValueErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "port-server.conf")
	content := "PORT_TUNNEL_TTL=not-a-duration\n"
	if err := osWriteFile(path, content); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	_, _, err := LoadFromArgs([]string{"--config", path}, flag.ContinueOnError)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid value for PORT_TUNNEL_TTL") {
		t.Fatalf("error = %q, want invalid value for PORT_TUNNEL_TTL", err)
	}
}

func TestLoadFromEnv_DBAuthFields(t *testing.T) {
	t.Setenv("PORT_DB_DSN", "user:pass@tcp(localhost:3306)/port")
	t.Setenv("PORT_ALLOW_UNAUTHED", "true")

	cfg := Load()
	if cfg.DBDSN != "user:pass@tcp(localhost:3306)/port" {
		t.Fatalf("DBDSN = %q, want expected DSN", cfg.DBDSN)
	}
	if !cfg.AllowUnauthed {
		t.Fatalf("AllowUnauthed = false, want true")
	}
}

func TestLoadFromArgs_ConfigFileInvalidAllowUnauthed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "port-server.conf")
	content := "PORT_ALLOW_UNAUTHED=maybe\n"
	if err := osWriteFile(path, content); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	_, _, err := LoadFromArgs([]string{"--config", path}, flag.ContinueOnError)
	if err == nil {
		t.Fatal("expected error for invalid PORT_ALLOW_UNAUTHED")
	}
	if !strings.Contains(err.Error(), "invalid value for PORT_ALLOW_UNAUTHED") {
		t.Fatalf("error = %q, want invalid value for PORT_ALLOW_UNAUTHED", err)
	}
}

func osWriteFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
