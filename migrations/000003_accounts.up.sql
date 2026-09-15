CREATE TABLE accounts (
    id             BIGINT          GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id      UUID            NOT NULL DEFAULT gen_random_uuid(),
    user_id        BIGINT          REFERENCES users(id),   -- NULL untuk akun sistem
    account_type   account_type    NOT NULL,
    normal_balance entry_direction NOT NULL,
    status         account_status  NOT NULL DEFAULT 'ACTIVE',
    currency       CHAR(3)         NOT NULL DEFAULT 'IDR' CHECK (currency = 'IDR'),

    balance        BIGINT          NOT NULL DEFAULT 0,     -- sen; saldo termaterialisasi
    version        BIGINT          NOT NULL DEFAULT 1,     -- optimistic lock

    created_at     TIMESTAMPTZ     NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ     NOT NULL DEFAULT now(),

    -- BR-05: dompet pengguna tidak boleh negatif; akun sistem boleh (BR-06)
    CONSTRAINT chk_wallet_non_negative
        CHECK (account_type <> 'USER_WALLET' OR balance >= 0),

    -- sisi normal per jenis akun tidak boleh salah
    CONSTRAINT chk_normal_balance CHECK (
        (account_type = 'SYSTEM_CASH' AND normal_balance = 'DEBIT') OR
        (account_type IN ('USER_WALLET', 'SYSTEM_FEE_REVENUE', 'SYSTEM_SUSPENSE')
            AND normal_balance = 'CREDIT')
    ),

    -- akun sistem tidak punya pemilik; dompet wajib punya pemilik
    CONSTRAINT chk_owner CHECK (
        (account_type = 'USER_WALLET' AND user_id IS NOT NULL) OR
        (account_type <> 'USER_WALLET' AND user_id IS NULL)
    ),

    -- satu pengguna tepat satu dompet
    CONSTRAINT uq_user_wallet UNIQUE (user_id, account_type)
);

CREATE UNIQUE INDEX idx_accounts_public_id   ON accounts (public_id);
CREATE INDEX        idx_accounts_user        ON accounts (user_id) WHERE user_id IS NOT NULL;
-- hanya SATU akun per jenis sistem
CREATE UNIQUE INDEX idx_accounts_system_type ON accounts (account_type) WHERE user_id IS NULL;
