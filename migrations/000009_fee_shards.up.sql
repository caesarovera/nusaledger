-- Perbaikan performa (k6, Sesi 29): SETIAP transfer mengunci SYSTEM_FEE_REVENUE yang
-- sama (FOR UPDATE), sehingga seluruh sistem terserialisasi pada satu baris — diukur
-- 177 tps, p95 515 ms (target 200 tps, p95 200 ms). Solusi: pecah pendapatan fee ke N
-- akun ("shard"), dipilih ACAK per transfer di service layer. Trial balance & job drift
-- TIDAK berubah — keduanya menjumlahkan SELURUH entries/accounts, agnostik jumlah shard.
--
-- SYSTEM_CASH dan SYSTEM_SUSPENSE TIDAK di-shard: keduanya bukan baris panas per-transfer
-- (CASH hanya tersentuh saat topup/withdraw, jauh lebih jarang dan bukan lawan dari
-- transfer-to-transfer contention; SUSPENSE hampir tidak pernah tersentuh Fase 1).

-- Index lama mewajibkan TEPAT SATU baris untuk SETIAP jenis akun sistem. Diganti index
-- yang hanya menegakkan itu untuk CASH & SUSPENSE; SYSTEM_FEE_REVENUE boleh banyak baris.
DROP INDEX idx_accounts_system_type;

CREATE UNIQUE INDEX idx_accounts_system_singleton ON accounts (account_type)
    WHERE user_id IS NULL AND account_type IN ('SYSTEM_CASH', 'SYSTEM_SUSPENSE');

-- 7 shard tambahan (total 8 dengan yang sudah ada dari migration 000008) — angka bulat
-- yang jelas mengurangi kontensi pada beban k6 Sesi 29 (100 VU), bukan hasil perhitungan
-- rumus; didokumentasikan sebagai keputusan pragmatis, bukan angka yang "terbukti optimal".
INSERT INTO accounts (account_type, normal_balance, user_id)
SELECT 'SYSTEM_FEE_REVENUE', 'CREDIT', NULL FROM generate_series(1, 7);
