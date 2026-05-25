package admin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/eslusarenko/port-server/internal/db"
)

// Run parses args like: create-user --email X --name Y
// and dispatches to the appropriate admin operation.
// Returns an error on failure.
func Run(dsn string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: port-server admin <command> [flags]\n\nCommands:\n  create-user  --email <email> --name <name>\n  create-key   --user-id <id> --label <label>\n  revoke-key   --id <key-id>\n  list-users\n  list-keys    [--user-id <id>]")
	}

	database, err := db.Open(dsn)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer func() { _ = database.Close() }()

	cmd := args[0]
	flags := parseFlags(args[1:])

	switch cmd {
	case "create-user":
		return createUser(database, flags)
	case "create-key":
		return createKey(database, flags)
	case "revoke-key":
		return revokeKey(database, flags)
	case "list-users":
		return listUsers(database)
	case "list-keys":
		return listKeys(database, flags)
	default:
		return fmt.Errorf("unknown admin command: %q", cmd)
	}
}

func createUser(db *sql.DB, flags map[string]string) error {
	email := flags["email"]
	name := flags["name"]
	if email == "" || name == "" {
		return fmt.Errorf("create-user requires --email and --name")
	}
	res, err := db.ExecContext(context.Background(),
		`INSERT INTO users (email, name) VALUES (?, ?)`, email, name)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	id, _ := res.LastInsertId()
	fmt.Printf("user_id=%d\n", id)
	return nil
}

func createKey(db *sql.DB, flags map[string]string) error {
	userIDStr := flags["user-id"]
	label := flags["label"]
	if userIDStr == "" {
		return fmt.Errorf("create-key requires --user-id")
	}
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid --user-id: %w", err)
	}

	rawBytes := make([]byte, 32)
	if _, randErr := rand.Read(rawBytes); randErr != nil {
		return fmt.Errorf("generate key: %w", randErr)
	}
	rawKey := hex.EncodeToString(rawBytes)
	sum := sha256.Sum256([]byte(rawKey))
	hash := hex.EncodeToString(sum[:])

	res, err := db.ExecContext(context.Background(),
		`INSERT INTO api_keys (user_id, key_hash, label) VALUES (?, ?, ?)`,
		userID, hash, label)
	if err != nil {
		return fmt.Errorf("create key: %w", err)
	}
	keyID, _ := res.LastInsertId()
	fmt.Printf("key_id=%d\nkey=%s\n", keyID, rawKey)
	return nil
}

func revokeKey(db *sql.DB, flags map[string]string) error {
	idStr := flags["id"]
	if idStr == "" {
		return fmt.Errorf("revoke-key requires --id")
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid --id: %w", err)
	}
	res, err := db.ExecContext(context.Background(),
		`UPDATE api_keys SET revoked_at = NOW() WHERE id = ? AND revoked_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("revoke key: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("key %d not found or already revoked", id)
	}
	fmt.Printf("revoked key_id=%d\n", id)
	return nil
}

func listUsers(db *sql.DB) error {
	rows, err := db.QueryContext(context.Background(),
		`SELECT id, email, name, created_at, status FROM users ORDER BY id`)
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}
	defer func() { _ = rows.Close() }()
	fmt.Printf("%-6s  %-30s  %-20s  %-20s  %s\n", "ID", "EMAIL", "NAME", "CREATED_AT", "STATUS")
	for rows.Next() {
		var id int64
		var email, name, createdAt, status string
		if err := rows.Scan(&id, &email, &name, &createdAt, &status); err != nil {
			return err
		}
		fmt.Printf("%-6d  %-30s  %-20s  %-20s  %s\n", id, email, name, createdAt, status)
	}
	return rows.Err()
}

func listKeys(db *sql.DB, flags map[string]string) error {
	var rows *sql.Rows
	var err error
	if uid := flags["user-id"]; uid != "" {
		userID, parseErr := strconv.ParseInt(uid, 10, 64)
		if parseErr != nil {
			return fmt.Errorf("invalid --user-id: %w", parseErr)
		}
		rows, err = db.QueryContext(context.Background(),
			`SELECT id, user_id, label, created_at, last_used_at, revoked_at FROM api_keys WHERE user_id = ? ORDER BY id`,
			userID)
	} else {
		rows, err = db.QueryContext(context.Background(),
			`SELECT id, user_id, label, created_at, last_used_at, revoked_at FROM api_keys ORDER BY id`)
	}
	if err != nil {
		return fmt.Errorf("list keys: %w", err)
	}
	defer func() { _ = rows.Close() }()
	fmt.Printf("%-6s  %-8s  %-20s  %-20s  %-20s  %s\n", "ID", "USER_ID", "LABEL", "CREATED_AT", "LAST_USED_AT", "REVOKED_AT")
	for rows.Next() {
		var id, userID int64
		var label, createdAt string
		var lastUsedAt, revokedAt sql.NullString
		if err := rows.Scan(&id, &userID, &label, &createdAt, &lastUsedAt, &revokedAt); err != nil {
			return err
		}
		lu := nullStr(lastUsedAt)
		rv := nullStr(revokedAt)
		fmt.Printf("%-6d  %-8d  %-20s  %-20s  %-20s  %s\n", id, userID, label, createdAt, lu, rv)
	}
	return rows.Err()
}

func nullStr(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return "-"
}

// parseFlags parses --key value and --key=value style flags into a map.
func parseFlags(args []string) map[string]string {
	m := make(map[string]string)
	for i := 0; i < len(args); i++ {
		a := args[i]
		a = strings.TrimPrefix(a, "--")
		a = strings.TrimPrefix(a, "-")
		if idx := strings.IndexByte(a, '='); idx >= 0 {
			m[a[:idx]] = a[idx+1:]
		} else if i+1 < len(args) {
			m[a] = args[i+1]
			i++
		}
	}
	return m
}

// PrintHelp prints admin subcommand help to stdout.
func PrintHelp() {
	_, _ = fmt.Fprintln(os.Stdout, `Usage: port-server admin <command> [flags]

Commands:
  create-user  --email <email> --name <name>
               Create a new user and print the user_id.

  create-key   --user-id <id> --label <label>
               Generate a random API key for a user, print key_id and the key
               (shown once - store it securely).

  revoke-key   --id <key-id>
               Revoke an API key by ID.

  list-users
               Print all users.

  list-keys    [--user-id <id>]
               Print all keys, or only keys for a specific user.`)
}
