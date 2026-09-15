CREATE TABLE idempotency_keys (
    user_id        BIGINT      NOT NULL REFERENCES users(id),
    key            TEXT        NOT NULL CHECK (length(key) BETWEEN 1 AND 128),
    endpoint       TEXT        NOT NULL,
    request_hash   TEXT        NOT NULL,              -- SHA-256 hex dari body
    transaction_id UUID        REFERENCES transactions(id),
    status_code    INT         NOT NULL,              -- 0 = sedang diproses
    response_body  JSONB       NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- K-01: key berlaku PER PENGGUNA, bukan global
    PRIMARY KEY (user_id, key)
);

-- pembersihan retensi 30 hari
CREATE INDEX idx_idem_cleanup ON idempotency_keys (created_at);
