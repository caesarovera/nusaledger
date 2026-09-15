-- Disiapkan untuk Fase 2 (outbox relay). Fase 1 hanya menulis, tidak ada konsumen.
CREATE TABLE outbox_events (
    id             BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    event_id       UUID        NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    aggregate_type TEXT        NOT NULL,
    aggregate_id   TEXT        NOT NULL,
    event_type     TEXT        NOT NULL,
    payload        JSONB       NOT NULL,
    trace_id       TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at   TIMESTAMPTZ,
    attempts       INT         NOT NULL DEFAULT 0,
    last_error     TEXT
);

-- Partial index: hanya baris yang belum terkirim, tetap kecil meski tabel besar
CREATE INDEX idx_outbox_unpublished ON outbox_events (created_at) WHERE published_at IS NULL;
