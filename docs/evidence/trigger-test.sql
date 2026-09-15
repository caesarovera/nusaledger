-- Uji trigger & constraint SEBELUM ada kode Go (docs/05 Langkah 3).
-- Jalankan: docker compose exec -T postgres psql -U nusa -d nusaledger -a -f - < docs/evidence/trigger-test.sql
-- Baris berlabel "HARUS GAGAL" wajib menghasilkan ERROR. Kalau lolos, trigger salah pasang.
\set ON_ERROR_STOP 0

\echo '=== 0. akun sistem hasil seed (harus 3 baris: id 1 CASH, 2 FEE, 3 SUSPENSE) ==='
SELECT id, account_type, normal_balance, balance FROM accounts ORDER BY id;

\echo '=== 1. HARUS GAGAL saat COMMIT: transaksi tidak seimbang (BR-03, deferred trigger) ==='
BEGIN;
INSERT INTO transactions (id, txn_type) VALUES ('00000000-0000-0000-0000-000000000001', 'TOPUP');
INSERT INTO entries (transaction_id, account_id, direction, amount, balance_after)
VALUES ('00000000-0000-0000-0000-000000000001', 1, 'DEBIT', 1000, 1000);
COMMIT;

\echo '=== 2. HARUS SUKSES: transaksi seimbang (debit CASH 1000 = kredit SUSPENSE 1000) ==='
BEGIN;
INSERT INTO transactions (id, txn_type) VALUES ('00000000-0000-0000-0000-000000000002', 'TOPUP');
INSERT INTO entries (transaction_id, account_id, direction, amount, balance_after) VALUES
  ('00000000-0000-0000-0000-000000000002', 1, 'DEBIT',  1000, 1000),
  ('00000000-0000-0000-0000-000000000002', 3, 'CREDIT', 1000, 1000);
COMMIT;
SELECT count(*) AS jumlah_entry FROM entries;

\echo '=== 3. HARUS GAGAL: UPDATE entries (BR-04 append-only) ==='
-- Catatan: id 1 sudah "terbakar" oleh transaksi yang di-rollback di uji 1 (sequence tidak di-rollback),
-- jadi jangan pakai WHERE id = 1 — sasar baris yang benar-benar ada.
UPDATE entries SET amount = 5 WHERE id = (SELECT min(id) FROM entries);

\echo '=== 4. HARUS GAGAL: DELETE entries (BR-04) ==='
DELETE FROM entries WHERE id = (SELECT min(id) FROM entries);

\echo '=== 5. HARUS GAGAL: UPDATE transactions kolom selain status (K-05) ==='
UPDATE transactions SET txn_type = 'TRANSFER' WHERE id = '00000000-0000-0000-0000-000000000002';

\echo '=== 6. HARUS SUKSES: UPDATE transactions.status POSTED -> REVERSED (K-05) ==='
UPDATE transactions SET status = 'REVERSED' WHERE id = '00000000-0000-0000-0000-000000000002';

\echo '=== 7. HARUS GAGAL: status REVERSED -> POSTED (K-05, tidak bisa kembali) ==='
UPDATE transactions SET status = 'POSTED' WHERE id = '00000000-0000-0000-0000-000000000002';

\echo '=== 8. HARUS GAGAL: DELETE transactions ==='
DELETE FROM transactions WHERE id = '00000000-0000-0000-0000-000000000002';

\echo '=== 9. HARUS GAGAL: amount = 0 (BR-02) ==='
BEGIN;
INSERT INTO transactions (id, txn_type) VALUES ('00000000-0000-0000-0000-000000000003', 'TOPUP');
INSERT INTO entries (transaction_id, account_id, direction, amount, balance_after)
VALUES ('00000000-0000-0000-0000-000000000003', 1, 'DEBIT', 0, 0);
ROLLBACK;

\echo '=== 10. HARUS GAGAL: akun yang sama dua kali dalam satu transaksi (K-02) ==='
BEGIN;
INSERT INTO transactions (id, txn_type) VALUES ('00000000-0000-0000-0000-000000000004', 'TOPUP');
INSERT INTO entries (transaction_id, account_id, direction, amount, balance_after) VALUES
  ('00000000-0000-0000-0000-000000000004', 1, 'DEBIT',  500, 0),
  ('00000000-0000-0000-0000-000000000004', 1, 'CREDIT', 500, 0);
ROLLBACK;

\echo '=== 11. HARUS GAGAL: akun SYSTEM_CASH kedua (partial unique index) ==='
INSERT INTO accounts (account_type, normal_balance, user_id) VALUES ('SYSTEM_CASH', 'DEBIT', NULL);

\echo '=== 12. HARUS GAGAL: dompet pengguna bersaldo negatif (BR-05) ==='
INSERT INTO users (email, password_hash, full_name) VALUES ('uji@test.local', 'x', 'Uji');
INSERT INTO accounts (account_type, normal_balance, user_id)
VALUES ('USER_WALLET', 'CREDIT', (SELECT id FROM users WHERE email = 'uji@test.local'));
UPDATE accounts SET balance = -1 WHERE account_type = 'USER_WALLET';

\echo '=== 13. HARUS SUKSES: akun sistem boleh negatif (BR-06) ==='
UPDATE accounts SET balance = -1 WHERE id = 1 RETURNING id, balance;

\echo '=== 14. HARUS GAGAL: dompet dengan normal_balance DEBIT (chk_normal_balance) ==='
INSERT INTO accounts (account_type, normal_balance, user_id)
VALUES ('USER_WALLET', 'DEBIT', (SELECT id FROM users WHERE email = 'uji@test.local'));

\echo '=== 15. HARUS GAGAL: email beda huruf besar-kecil dianggap sama ==='
INSERT INTO users (email, password_hash, full_name) VALUES ('UJI@test.local', 'x', 'Uji 2');

\echo '=== 16. HARUS GAGAL: reversal kedua atas transaksi yang sama (BR-11 uq_reversal) ==='
INSERT INTO transactions (txn_type, reverses_transaction_id) VALUES ('REVERSAL', '00000000-0000-0000-0000-000000000002');
INSERT INTO transactions (txn_type, reverses_transaction_id) VALUES ('REVERSAL', '00000000-0000-0000-0000-000000000002');

\echo '=== 17. HARUS GAGAL: idempotency key sama untuk user sama; HARUS SUKSES untuk user beda (K-01) ==='
INSERT INTO users (email, password_hash, full_name) VALUES ('uji2@test.local', 'x', 'Uji Dua');
INSERT INTO idempotency_keys (user_id, key, endpoint, request_hash, status_code, response_body)
VALUES ((SELECT id FROM users WHERE email='uji@test.local'), 'k1', 'transfer', 'h', 0, '{}');
INSERT INTO idempotency_keys (user_id, key, endpoint, request_hash, status_code, response_body)
VALUES ((SELECT id FROM users WHERE email='uji@test.local'), 'k1', 'transfer', 'h', 0, '{}');
INSERT INTO idempotency_keys (user_id, key, endpoint, request_hash, status_code, response_body)
VALUES ((SELECT id FROM users WHERE email='uji2@test.local'), 'k1', 'transfer', 'h', 0, '{}');
SELECT user_id, key FROM idempotency_keys ORDER BY user_id;

\echo '=== 18. Trial balance (harus difference = 0) ==='
SELECT COALESCE(SUM(amount) FILTER (WHERE direction = 'DEBIT'),  0) AS total_debit,
       COALESCE(SUM(amount) FILTER (WHERE direction = 'CREDIT'), 0) AS total_credit,
       COALESCE(SUM(amount) FILTER (WHERE direction = 'DEBIT'),  0)
     - COALESCE(SUM(amount) FILTER (WHERE direction = 'CREDIT'), 0) AS difference
FROM entries;
