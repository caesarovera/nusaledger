# Bukti uji trigger & constraint — 2026-09-16

Dijalankan pada PostgreSQL 17.11 (container), skema versi 8, SEBELUM ada kode Go. Perintah:

```
docker compose exec -T postgres psql -U nusa -d nusaledger -a -f - < docs/evidence/trigger-test.sql
```

Setiap blok "HARUS GAGAL" menghasilkan ERROR; setiap "HARUS SUKSES" berhasil. Transkrip lengkap:

```
-- Uji trigger & constraint SEBELUM ada kode Go (docs/05 Langkah 3).
-- Jalankan: docker compose exec -T postgres psql -U nusa -d nusaledger -a -f - < docs/evidence/trigger-test.sql
-- Baris berlabel "HARUS GAGAL" wajib menghasilkan ERROR. Kalau lolos, trigger salah pasang.
\set ON_ERROR_STOP 0
\echo '=== 0. akun sistem hasil seed (harus 3 baris: id 1 CASH, 2 FEE, 3 SUSPENSE) ==='
=== 0. akun sistem hasil seed (harus 3 baris: id 1 CASH, 2 FEE, 3 SUSPENSE) ===
SELECT id, account_type, normal_balance, balance FROM accounts ORDER BY id;
 id |    account_type    | normal_balance | balance 
----+--------------------+----------------+---------
  1 | SYSTEM_CASH        | DEBIT          |       0
  2 | SYSTEM_FEE_REVENUE | CREDIT         |       0
  3 | SYSTEM_SUSPENSE    | CREDIT         |       0
(3 rows)

\echo '=== 1. HARUS GAGAL saat COMMIT: transaksi tidak seimbang (BR-03, deferred trigger) ==='
=== 1. HARUS GAGAL saat COMMIT: transaksi tidak seimbang (BR-03, deferred trigger) ===
BEGIN;
BEGIN
INSERT INTO transactions (id, txn_type) VALUES ('00000000-0000-0000-0000-000000000001', 'TOPUP');
INSERT 0 1
INSERT INTO entries (transaction_id, account_id, direction, amount, balance_after)
VALUES ('00000000-0000-0000-0000-000000000001', 1, 'DEBIT', 1000, 1000);
INSERT 0 1
COMMIT;
ERROR:  transaksi 00000000-0000-0000-0000-000000000001 tidak seimbang: debit=1000 kredit=0
CONTEXT:  PL/pgSQL function assert_transaction_balanced() line 13 at RAISE
\echo '=== 2. HARUS SUKSES: transaksi seimbang (debit CASH 1000 = kredit SUSPENSE 1000) ==='
=== 2. HARUS SUKSES: transaksi seimbang (debit CASH 1000 = kredit SUSPENSE 1000) ===
BEGIN;
BEGIN
INSERT INTO transactions (id, txn_type) VALUES ('00000000-0000-0000-0000-000000000002', 'TOPUP');
INSERT 0 1
INSERT INTO entries (transaction_id, account_id, direction, amount, balance_after) VALUES
  ('00000000-0000-0000-0000-000000000002', 1, 'DEBIT',  1000, 1000),
  ('00000000-0000-0000-0000-000000000002', 3, 'CREDIT', 1000, 1000);
INSERT 0 2
COMMIT;
COMMIT
SELECT count(*) AS jumlah_entry FROM entries;
 jumlah_entry 
--------------
            2
(1 row)

\echo '=== 3. HARUS GAGAL: UPDATE entries (BR-04 append-only) ==='
=== 3. HARUS GAGAL: UPDATE entries (BR-04 append-only) ===
-- Catatan: id 1 sudah "terbakar" oleh transaksi yang di-rollback di uji 1 (sequence tidak di-rollback),
-- jadi jangan pakai WHERE id = 1 — sasar baris yang benar-benar ada.
UPDATE entries SET amount = 5 WHERE id = (SELECT min(id) FROM entries);
\echo '=== 4. HARUS GAGAL: DELETE entries (BR-04) ==='
=== 4. HARUS GAGAL: DELETE entries (BR-04) ===
DELETE FROM entries WHERE id = (SELECT min(id) FROM entries);
ERROR:  tabel entries bersifat append-only: UPDATE ditolak
CONTEXT:  PL/pgSQL function forbid_mutation() line 3 at RAISE
ERROR:  tabel entries bersifat append-only: DELETE ditolak
CONTEXT:  PL/pgSQL function forbid_mutation() line 3 at RAISE
\echo '=== 5. HARUS GAGAL: UPDATE transactions kolom selain status (K-05) ==='
=== 5. HARUS GAGAL: UPDATE transactions kolom selain status (K-05) ===
UPDATE transactions SET txn_type = 'TRANSFER' WHERE id = '00000000-0000-0000-0000-000000000002';
ERROR:  tabel transactions hanya boleh mengubah kolom status
CONTEXT:  PL/pgSQL function transactions_status_only() line 9 at RAISE
\echo '=== 6. HARUS SUKSES: UPDATE transactions.status POSTED -> REVERSED (K-05) ==='
=== 6. HARUS SUKSES: UPDATE transactions.status POSTED -> REVERSED (K-05) ===
UPDATE transactions SET status = 'REVERSED' WHERE id = '00000000-0000-0000-0000-000000000002';
UPDATE 1
\echo '=== 7. HARUS GAGAL: status REVERSED -> POSTED (K-05, tidak bisa kembali) ==='
=== 7. HARUS GAGAL: status REVERSED -> POSTED (K-05, tidak bisa kembali) ===
UPDATE transactions SET status = 'POSTED' WHERE id = '00000000-0000-0000-0000-000000000002';
ERROR:  transaksi 00000000-0000-0000-0000-000000000002 sudah REVERSED dan tidak bisa dikembalikan
CONTEXT:  PL/pgSQL function transactions_status_only() line 14 at RAISE
\echo '=== 8. HARUS GAGAL: DELETE transactions ==='
=== 8. HARUS GAGAL: DELETE transactions ===
DELETE FROM transactions WHERE id = '00000000-0000-0000-0000-000000000002';
ERROR:  tabel transactions bersifat append-only: DELETE ditolak
CONTEXT:  PL/pgSQL function forbid_mutation() line 3 at RAISE
\echo '=== 9. HARUS GAGAL: amount = 0 (BR-02) ==='
=== 9. HARUS GAGAL: amount = 0 (BR-02) ===
BEGIN;
BEGIN
INSERT INTO transactions (id, txn_type) VALUES ('00000000-0000-0000-0000-000000000003', 'TOPUP');
INSERT 0 1
INSERT INTO entries (transaction_id, account_id, direction, amount, balance_after)
VALUES ('00000000-0000-0000-0000-000000000003', 1, 'DEBIT', 0, 0);
ERROR:  new row for relation "entries" violates check constraint "entries_amount_check"
DETAIL:  Failing row contains (4, 00000000-0000-0000-0000-000000000003, 1, DEBIT, 0, 0, 2026-09-15 21:52:01.431664+00).
ROLLBACK;
ROLLBACK
\echo '=== 10. HARUS GAGAL: akun yang sama dua kali dalam satu transaksi (K-02) ==='
=== 10. HARUS GAGAL: akun yang sama dua kali dalam satu transaksi (K-02) ===
BEGIN;
BEGIN
INSERT INTO transactions (id, txn_type) VALUES ('00000000-0000-0000-0000-000000000004', 'TOPUP');
INSERT 0 1
INSERT INTO entries (transaction_id, account_id, direction, amount, balance_after) VALUES
  ('00000000-0000-0000-0000-000000000004', 1, 'DEBIT',  500, 0),
  ('00000000-0000-0000-0000-000000000004', 1, 'CREDIT', 500, 0);
ROLLBACK;
ERROR:  duplicate key value violates unique constraint "uq_entry_account_per_txn"
DETAIL:  Key (transaction_id, account_id)=(00000000-0000-0000-0000-000000000004, 1) already exists.
ROLLBACK
\echo '=== 11. HARUS GAGAL: akun SYSTEM_CASH kedua (partial unique index) ==='
=== 11. HARUS GAGAL: akun SYSTEM_CASH kedua (partial unique index) ===
INSERT INTO accounts (account_type, normal_balance, user_id) VALUES ('SYSTEM_CASH', 'DEBIT', NULL);
ERROR:  duplicate key value violates unique constraint "idx_accounts_system_type"
DETAIL:  Key (account_type)=(SYSTEM_CASH) already exists.
\echo '=== 12. HARUS GAGAL: dompet pengguna bersaldo negatif (BR-05) ==='
=== 12. HARUS GAGAL: dompet pengguna bersaldo negatif (BR-05) ===
INSERT INTO users (email, password_hash, full_name) VALUES ('uji@test.local', 'x', 'Uji');
INSERT 0 1
INSERT INTO accounts (account_type, normal_balance, user_id)
VALUES ('USER_WALLET', 'CREDIT', (SELECT id FROM users WHERE email = 'uji@test.local'));
INSERT 0 1
UPDATE accounts SET balance = -1 WHERE account_type = 'USER_WALLET';
\echo '=== 13. HARUS SUKSES: akun sistem boleh negatif (BR-06) ==='
=== 13. HARUS SUKSES: akun sistem boleh negatif (BR-06) ===
UPDATE accounts SET balance = -1 WHERE id = 1 RETURNING id, balance;
ERROR:  new row for relation "accounts" violates check constraint "chk_wallet_non_negative"
DETAIL:  Failing row contains (5, ce6a620a-62ca-46a6-8315-a047a30fe099, 1, USER_WALLET, CREDIT, ACTIVE, IDR, -1, 1, 2026-09-15 21:52:01.437279+00, 2026-09-15 21:52:01.437279+00).
 id | balance 
----+---------
  1 |      -1
(1 row)

UPDATE 1
\echo '=== 14. HARUS GAGAL: dompet dengan normal_balance DEBIT (chk_normal_balance) ==='
=== 14. HARUS GAGAL: dompet dengan normal_balance DEBIT (chk_normal_balance) ===
INSERT INTO accounts (account_type, normal_balance, user_id)
VALUES ('USER_WALLET', 'DEBIT', (SELECT id FROM users WHERE email = 'uji@test.local'));
ERROR:  new row for relation "accounts" violates check constraint "chk_normal_balance"
DETAIL:  Failing row contains (6, eabb4e70-8acf-4590-975c-56347e5a0c8c, 1, USER_WALLET, DEBIT, ACTIVE, IDR, 0, 1, 2026-09-15 21:52:01.44281+00, 2026-09-15 21:52:01.44281+00).
\echo '=== 15. HARUS GAGAL: email beda huruf besar-kecil dianggap sama ==='
=== 15. HARUS GAGAL: email beda huruf besar-kecil dianggap sama ===
INSERT INTO users (email, password_hash, full_name) VALUES ('UJI@test.local', 'x', 'Uji 2');
ERROR:  duplicate key value violates unique constraint "idx_users_email"
DETAIL:  Key (lower(email))=(uji@test.local) already exists.
\echo '=== 16. HARUS GAGAL: reversal kedua atas transaksi yang sama (BR-11 uq_reversal) ==='
=== 16. HARUS GAGAL: reversal kedua atas transaksi yang sama (BR-11 uq_reversal) ===
INSERT INTO transactions (txn_type, reverses_transaction_id) VALUES ('REVERSAL', '00000000-0000-0000-0000-000000000002');
INSERT 0 1
INSERT INTO transactions (txn_type, reverses_transaction_id) VALUES ('REVERSAL', '00000000-0000-0000-0000-000000000002');
ERROR:  duplicate key value violates unique constraint "uq_reversal"
DETAIL:  Key (reverses_transaction_id)=(00000000-0000-0000-0000-000000000002) already exists.
\echo '=== 17. HARUS GAGAL: idempotency key sama untuk user sama; HARUS SUKSES untuk user beda (K-01) ==='
=== 17. HARUS GAGAL: idempotency key sama untuk user sama; HARUS SUKSES untuk user beda (K-01) ===
INSERT INTO users (email, password_hash, full_name) VALUES ('uji2@test.local', 'x', 'Uji Dua');
INSERT 0 1
INSERT INTO idempotency_keys (user_id, key, endpoint, request_hash, status_code, response_body)
VALUES ((SELECT id FROM users WHERE email='uji@test.local'), 'k1', 'transfer', 'h', 0, '{}');
INSERT 0 1
INSERT INTO idempotency_keys (user_id, key, endpoint, request_hash, status_code, response_body)
VALUES ((SELECT id FROM users WHERE email='uji@test.local'), 'k1', 'transfer', 'h', 0, '{}');
ERROR:  duplicate key value violates unique constraint "idempotency_keys_pkey"
DETAIL:  Key (user_id, key)=(1, k1) already exists.
INSERT INTO idempotency_keys (user_id, key, endpoint, request_hash, status_code, response_body)
VALUES ((SELECT id FROM users WHERE email='uji2@test.local'), 'k1', 'transfer', 'h', 0, '{}');
INSERT 0 1
SELECT user_id, key FROM idempotency_keys ORDER BY user_id;
 user_id | key 
---------+-----
       1 | k1
       3 | k1
(2 rows)

\echo '=== 18. Trial balance (harus difference = 0) ==='
=== 18. Trial balance (harus difference = 0) ===
SELECT COALESCE(SUM(amount) FILTER (WHERE direction = 'DEBIT'),  0) AS total_debit,
       COALESCE(SUM(amount) FILTER (WHERE direction = 'CREDIT'), 0) AS total_credit,
       COALESCE(SUM(amount) FILTER (WHERE direction = 'DEBIT'),  0)
     - COALESCE(SUM(amount) FILTER (WHERE direction = 'CREDIT'), 0) AS difference
FROM entries;
 total_debit | total_credit | difference 
-------------+--------------+------------
        1000 |         1000 |          0
(1 row)

```
