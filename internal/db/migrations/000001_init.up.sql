CREATE TABLE IF NOT EXISTS users (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    email       VARCHAR(255)    NOT NULL UNIQUE,
    name        VARCHAR(255)    NOT NULL,
    created_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status      VARCHAR(32)     NOT NULL DEFAULT 'active'
);

CREATE TABLE IF NOT EXISTS api_keys (
    id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id      BIGINT UNSIGNED NOT NULL,
    key_hash     CHAR(64)        NOT NULL,
    label        VARCHAR(255)    NOT NULL DEFAULT '',
    created_at   DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at DATETIME        NULL,
    revoked_at   DATETIME        NULL,
    UNIQUE KEY uq_api_keys_key_hash (key_hash),
    CONSTRAINT fk_api_keys_user FOREIGN KEY (user_id) REFERENCES users (id)
);

CREATE TABLE IF NOT EXISTS subdomain_reservations (
    subdomain  VARCHAR(63)     NOT NULL PRIMARY KEY,
    user_id    BIGINT UNSIGNED NOT NULL,
    created_at DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_subdomain_reservations_user FOREIGN KEY (user_id) REFERENCES users (id)
);
