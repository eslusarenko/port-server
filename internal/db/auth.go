package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
)

// ErrKeyNotFound is returned when no matching active key is found.
var ErrKeyNotFound = errors.New("api key not found or revoked")

// KeyLookupResult holds data from a successful key lookup.
type KeyLookupResult struct {
	KeyID  int64
	UserID int64
}

// LookupAPIKey SHA-256-hashes rawKey, queries api_keys where revoked_at IS NULL,
// and returns the key ID and user ID on a hit.
func LookupAPIKey(ctx context.Context, db *sql.DB, rawKey string) (KeyLookupResult, error) {
	hash := hashKey(rawKey)
	var res KeyLookupResult
	err := db.QueryRowContext(ctx,
		`SELECT id, user_id FROM api_keys WHERE key_hash = ? AND revoked_at IS NULL LIMIT 1`,
		hash,
	).Scan(&res.KeyID, &res.UserID)
	if errors.Is(err, sql.ErrNoRows) {
		return KeyLookupResult{}, ErrKeyNotFound
	}
	if err != nil {
		return KeyLookupResult{}, fmt.Errorf("lookup api key: %w", err)
	}
	return res, nil
}

// UpdateLastUsed sets last_used_at = NOW() for the given key ID.
// Best-effort: errors are ignored by the caller.
func UpdateLastUsed(db *sql.DB, keyID int64) {
	_, _ = db.Exec(`UPDATE api_keys SET last_used_at = NOW() WHERE id = ?`, keyID)
}

func hashKey(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
