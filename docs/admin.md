# Admin subcommands

`port-server admin` provides user and API-key management. It requires a MySQL DSN — supplied via `PORT_DB_DSN` or `--db-dsn`.

```bash
PORT_DB_DSN='user:pass@tcp(127.0.0.1:3306)/port?parseTime=true' \
  port-server admin <command> [flags]
```

- [Commands](#commands)
  - [create-user](#create-user)
  - [create-key](#create-key)
  - [revoke-key](#revoke-key)
  - [list-users](#list-users)
  - [list-keys](#list-keys)
- [Provisioning a user end-to-end](#provisioning-a-user-end-to-end)

## Commands

### create-user

```
port-server admin create-user --email <email> --name <name>
```

Creates a new user. Prints `user_id=<n>` on success. `email` must be unique.

```bash
port-server admin create-user --email alice@example.com --name Alice
# user_id=1
```

### create-key

```
port-server admin create-key --user-id <id> --label <label>
```

Generates a random 32-byte API key (hex-encoded, 64 characters), stores its SHA-256 hash in `api_keys`, and prints the plaintext key **once**. The plaintext is never stored; if lost, revoke and create a new key.

```bash
port-server admin create-key --user-id 1 --label laptop
# key_id=3
# key=4f3a8b...  (64 hex chars)
```

### revoke-key

```
port-server admin revoke-key --id <key-id>
```

Sets `revoked_at = NOW()` for the given key. The key is immediately rejected by the server on the next tunnel attempt. Revocation is permanent — there is no un-revoke command.

```bash
port-server admin revoke-key --id 3
# revoked key_id=3
```

Returns an error if the key does not exist or is already revoked.

### list-users

```
port-server admin list-users
```

Prints all users in tabular form: `ID`, `EMAIL`, `NAME`, `CREATED_AT`, `STATUS`.

```
ID      EMAIL                          NAME                 CREATED_AT            STATUS
1       alice@example.com              Alice                2025-01-10 09:00:00   active
```

### list-keys

```
port-server admin list-keys [--user-id <id>]
```

Prints all API keys (or only keys for a specific user): `ID`, `USER_ID`, `LABEL`, `CREATED_AT`, `LAST_USED_AT`, `REVOKED_AT`. Null timestamps are displayed as `-`.

```bash
port-server admin list-keys --user-id 1
```

## Provisioning a user end-to-end

```bash
# 1. Start the server (or use an already-running instance)
PORT_DB_DSN='port:secret@tcp(db:3306)/port?parseTime=true' port-server &

# 2. Create the user
PORT_DB_DSN='port:secret@tcp(db:3306)/port?parseTime=true' \
  port-server admin create-user --email ops@example.com --name ops
# user_id=1

# 3. Create a key for that user
PORT_DB_DSN='port:secret@tcp(db:3306)/port?parseTime=true' \
  port-server admin create-key --user-id 1 --label workstation
# key_id=1
# key=<64 hex chars>   ← copy this now

# 4. Distribute the key to the client
export PORT_API_KEY=<key>
port expose 8080
# server log: tunnel_open authed=true user_id=1 subdomain=xyz
```

If the operator needs to rotate the key:

```bash
# Revoke old key
PORT_DB_DSN='...' port-server admin revoke-key --id 1

# Issue a new one
PORT_DB_DSN='...' port-server admin create-key --user-id 1 --label workstation-v2
```

---

See [authentication.md](authentication.md) for the full access-control model.
