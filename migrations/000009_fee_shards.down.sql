-- Turunkan HANYA bila shard tambahan belum pernah menerima entries — sama seperti
-- 000008: down tidak boleh menghapus jejak uang. Sisakan satu (yang id-nya terkecil)
-- supaya SYSTEM_FEE_REVENUE tetap ada tepat satu baris seperti sebelum migration ini.
DELETE FROM accounts
WHERE account_type = 'SYSTEM_FEE_REVENUE'
  AND user_id IS NULL
  AND id <> (SELECT min(id) FROM accounts WHERE account_type = 'SYSTEM_FEE_REVENUE' AND user_id IS NULL);

DROP INDEX idx_accounts_system_singleton;

CREATE UNIQUE INDEX idx_accounts_system_type ON accounts (account_type) WHERE user_id IS NULL;
