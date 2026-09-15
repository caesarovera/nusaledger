CREATE TABLE users (
    id            BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id     UUID        NOT NULL DEFAULT gen_random_uuid(),
    email         TEXT        NOT NULL CHECK (length(email) BETWEEN 3 AND 254),
    password_hash TEXT        NOT NULL,
    full_name     TEXT        NOT NULL CHECK (length(full_name) BETWEEN 1 AND 100),
    role          TEXT        NOT NULL DEFAULT 'USER' CHECK (role IN ('USER', 'ADMIN')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Andi@x.com dan andi@x.com adalah orang yang sama.
CREATE UNIQUE INDEX idx_users_email     ON users (LOWER(email));
CREATE UNIQUE INDEX idx_users_public_id ON users (public_id);

CREATE TABLE refresh_tokens (
    id          BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id     BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT        NOT NULL,              -- SHA-256, bukan token asli
    expires_at  TIMESTAMPTZ NOT NULL,
    revoked_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_refresh_hash   ON refresh_tokens (token_hash);
CREATE INDEX        idx_refresh_active ON refresh_tokens (user_id) WHERE revoked_at IS NULL;
