package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"github.com/eslusarenko/port-server/internal/config"
	dbpkg "github.com/eslusarenko/port-server/internal/db"
	"github.com/eslusarenko/port-server/internal/proxy"
	"github.com/eslusarenko/port-server/internal/transport"
	"github.com/eslusarenko/port-server/internal/tunnel"
)

// managerAdapter adapts *tunnel.Manager to the proxy.TunnelLookup interface.
type managerAdapter struct {
	mgr *tunnel.Manager
}

func (a *managerAdapter) Lookup(subdomain string) (proxy.TunnelForwarder, bool) {
	t, ok := a.mgr.Lookup(subdomain)
	if !ok {
		return nil, false
	}
	return t, true
}

// App wires all server components together.
type App struct {
	cfg    *config.Config
	mgr    *tunnel.Manager
	logger *slog.Logger
	server *http.Server
}

// New creates a new App from the given config and logger.
func New(cfg *config.Config, logger *slog.Logger) *App {
	return &App{
		cfg:    cfg,
		logger: logger,
	}
}

// Run starts the server and blocks until ctx is cancelled.
func (a *App) Run(ctx context.Context) error {
	a.mgr = tunnel.NewManager(a.cfg.BaseDomain, a.logger)

	var database *sql.DB
	if a.cfg.DBDSN != "" {
		var err error
		database, err = dbpkg.Open(a.cfg.DBDSN)
		if err != nil {
			return fmt.Errorf("database: %w", err)
		}
		defer func() { _ = database.Close() }()
	} else if !a.cfg.AllowUnauthed {
		return fmt.Errorf("PORT_DB_DSN is not set. Authentication requires a database. To run in legacy unauthed-only mode, pass --allow-unauthed")
	}

	// Reserved subdomain list is purely operator-supplied (PORT_RESERVED_SUBDOMAINS).
	// An empty list means no enforcement.
	reserved := a.cfg.ReservedSubdomains
	wsHandler := transport.NewHandler(
		a.mgr,
		a.logger,
		database,
		a.cfg.AllowUnauthed,
		a.cfg.NoUnauthedRestrictions,
		a.cfg.TunnelTTL,
		a.cfg.UnauthedTTL,
		reserved,
		a.cfg.MaxBodySize,
		a.cfg.TrustProxyHeaders,
	)
	proxyHandler := proxy.NewProxyWithOptions(&managerAdapter{a.mgr}, a.cfg.BaseDomain, a.logger, a.cfg.MaxBodySize, a.cfg.TrustProxyHeaders)

	mux := http.NewServeMux()
	mux.Handle("GET /tunnel/connect", wsHandler)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("/", proxyHandler)

	a.server = &http.Server{
		Addr:    a.cfg.Addr,
		Handler: mux,
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
	}

	// Start TTL cleanup.
	go a.mgr.StartCleanup(ctx)

	// Graceful shutdown on context cancellation.
	go func() {
		<-ctx.Done()
		a.logger.Info("server_stopping")
		a.mgr.CloseAll()
		_ = a.server.Shutdown(context.Background())
	}()

	a.logger.Info("server_starting", "addr", a.cfg.Addr, "domain", a.cfg.BaseDomain)
	err := a.server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
