CREATE TABLE transactions (
    id                      UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    txn_type                txn_type    NOT NULL,
    status                  txn_status  NOT NULL DEFAULT 'POSTED',
    description             TEXT        NOT NULL DEFAULT '' CHECK (length(description) <= 255),
    initiated_by_user_id    BIGINT      REFERENCES users(id),
    reverses_transaction_id UUID        REFERENCES transactions(id),
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- BR-11: satu transaksi hanya boleh dibalik sekali (NULL boleh berulang)
    CONSTRAINT uq_reversal UNIQUE (reverses_transaction_id),

    -- reversal wajib menunjuk transaksi asal; transaksi lain tidak boleh
    CONSTRAINT chk_reversal_link CHECK (
        (txn_type = 'REVERSAL' AND reverses_transaction_id IS NOT NULL) OR
        (txn_type <> 'REVERSAL' AND reverses_transaction_id IS NULL)
    )
);

CREATE INDEX idx_txn_created      ON transactions (created_at DESC);
CREATE INDEX idx_txn_type_created ON transactions (txn_type, created_at DESC);

CREATE TABLE entries (
    id             BIGINT          GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    transaction_id UUID            NOT NULL REFERENCES transactions(id),
    account_id     BIGINT          NOT NULL REFERENCES accounts(id),
    direction      entry_direction NOT NULL,
    amount         BIGINT          NOT NULL CHECK (amount > 0),     -- BR-02
    balance_after  BIGINT          NOT NULL,                        -- snapshot saldo
    created_at     TIMESTAMPTZ     NOT NULL DEFAULT now(),

    -- K-02: satu akun maksimal satu entry per transaksi
    CONSTRAINT uq_entry_account_per_txn UNIQUE (transaction_id, account_id)
);

-- Mutasi rekening: akun tertentu, terbaru dulu, cursor pada id
CREATE INDEX idx_entries_account_id_desc ON entries (account_id, id DESC);
-- Tabel besar & terurut waktu: BRIN jauh lebih kecil daripada B-tree
CREATE INDEX idx_entries_created_brin    ON entries USING BRIN (created_at);
