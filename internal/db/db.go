package db

import (
	"database/sql"
	"embed"
	"fmt"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	_ "github.com/go-sql-driver/mysql"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Open connects to MySQL and runs all pending up-migrations before returning.
// dsn is a standard go-sql-driver DSN (without multiStatements; it's added
// internally for the migration phase).
func Open(dsn string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("db open: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("db ping: %w", err)
	}

	if err := runMigrations(dsn); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func runMigrations(dsn string) error {
	migDB, err := sql.Open("mysql", addParam(dsn, "multiStatements=true"))
	if err != nil {
		return fmt.Errorf("migration db open: %w", err)
	}
	defer func() { _ = migDB.Close() }()

	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("migration source: %w", err)
	}

	driver, err := mysql.WithInstance(migDB, &mysql.Config{})
	if err != nil {
		return fmt.Errorf("migration driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "mysql", driver)
	if err != nil {
		return fmt.Errorf("migrate init: %w", err)
	}
	defer func() {
		_, _ = m.Close()
	}()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

// addParam appends a query parameter to a MySQL DSN.
func addParam(dsn, param string) string {
	if strings.Contains(dsn, "?") {
		return dsn + "&" + param
	}
	return dsn + "?" + param
}
