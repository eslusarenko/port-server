package tunnel

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const maxSubdomainRetries = 10

// Manager tracks all active tunnels, keyed by subdomain.
type Manager struct {
	mu         sync.RWMutex
	tunnels    map[string]*Tunnel
	baseDomain string
	ttl        time.Duration
	logger     *slog.Logger
}

// NewManager creates a tunnel manager.
func NewManager(baseDomain string, ttl time.Duration, logger *slog.Logger) *Manager {
	return &Manager{
		tunnels:    make(map[string]*Tunnel),
		baseDomain: baseDomain,
		ttl:        ttl,
		logger:     logger,
	}
}

// Register creates a new tunnel for the given WebSocket connection.
// It generates a unique subdomain and returns the Tunnel.
func (m *Manager) Register(conn *websocket.Conn) (*Tunnel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := 0; i < maxSubdomainRetries; i++ {
		sub, err := GenerateSubdomain()
		if err != nil {
			return nil, fmt.Errorf("generate subdomain: %w", err)
		}
		if _, exists := m.tunnels[sub]; exists {
			continue
		}
		t := NewTunnel(sub, conn)
		m.tunnels[sub] = t
		m.logger.Info("tunnel registered", "subdomain", sub)
		return t, nil
	}
	return nil, fmt.Errorf("failed to generate unique subdomain after %d retries", maxSubdomainRetries)
}

// Lookup returns the tunnel for the given subdomain, if it exists.
func (m *Manager) Lookup(subdomain string) (*Tunnel, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tunnels[subdomain]
	return t, ok
}

// Remove removes and closes the tunnel for the given subdomain.
func (m *Manager) Remove(subdomain string) {
	m.mu.Lock()
	t, ok := m.tunnels[subdomain]
	if ok {
		delete(m.tunnels, subdomain)
	}
	m.mu.Unlock()

	if ok {
		t.Close()
		m.logger.Info("tunnel removed", "subdomain", subdomain)
	}
}

// StartCleanup runs a background goroutine that evicts tunnels that have
// exceeded the configured TTL. It stops when ctx is cancelled.
func (m *Manager) StartCleanup(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.evictExpired()
		}
	}
}

func (m *Manager) evictExpired() {
	now := time.Now()
	m.mu.Lock()
	var expired []string
	for sub, t := range m.tunnels {
		if now.Sub(t.CreatedAt) > m.ttl {
			expired = append(expired, sub)
		}
	}
	for _, sub := range expired {
		delete(m.tunnels, sub)
	}
	m.mu.Unlock()

	for _, sub := range expired {
		m.logger.Info("tunnel expired", "subdomain", sub)
	}
}

// CloseAll closes all active tunnels. Used during graceful shutdown.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	tunnels := m.tunnels
	m.tunnels = make(map[string]*Tunnel)
	m.mu.Unlock()

	for _, t := range tunnels {
		t.Close()
	}
}

// BaseDomain returns the configured base domain.
func (m *Manager) BaseDomain() string {
	return m.baseDomain
}
