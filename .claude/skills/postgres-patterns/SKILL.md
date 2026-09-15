---
name: postgres-patterns
description: Pola PostgreSQL proyek ini — penguncian terurut, atomic update, cursor pagination, trigger deferred, aturan migration, terjemahan error pgx, dan checklist EXPLAIN. Gunakan saat menulis SQL, migration, atau kode repository.
---

# Pola PostgreSQL NusaLedger

## Kunci akun (anti-deadlock)
```sql
SELECT id, balance, version, status, normal_balance, account_type
FROM accounts WHERE id = ANY($1) ORDER BY id FOR UPDATE
```
`ORDER BY id` wajib: dua transaksi silang akan antre, bukan deadlock.

## Update saldo (atomik + optimistic)
```sql
UPDATE accounts SET balance = balance + $1, version = version + 1, updated_at = now()
WHERE id = $2 AND version = $3 AND status = 'ACTIVE'
RETURNING balance, version
```
`pgx.ErrNoRows` → ErrConcurrentModification. CHECK `chk_wallet_non_negative` → ErrInsufficientBalance.

## Cursor pagination
```sql
SELECT e.id, e.transaction_id, e.direction, e.amount, e.balance_after, e.created_at, t.txn_type, t.description
FROM entries e JOIN transactions t ON t.id = e.transaction_id
WHERE e.account_id = $1 AND ($2::BIGINT IS NULL OR e.id < $2)
ORDER BY e.id DESC LIMIT $3
```
Cursor = base64url(id). Ambil `limit+1` baris untuk tahu ada halaman berikutnya. Wajib `Index Scan Backward using idx_entries_account_id_desc`.

## Trigger
- `trg_entries_balanced`: CONSTRAINT TRIGGER DEFERRABLE INITIALLY DEFERRED → error muncul di `Commit()`, bukan `Exec()`.
- `forbid_mutation`: entries tolak UPDATE/DELETE; transactions tolak DELETE dan UPDATE selain kolom `status` (K-05).

## Migration
- Satu file satu tujuan. Setiap `up` punya `down` yang diuji: `migrate ... down -all && migrate ... up`.
- Tidak ada migration yang mengubah data uang.
- Migration sequence: 000001 extensions_and_types → 000002 users_and_auth → 000003 accounts → 000004 transactions_and_entries → 000005 ledger_triggers → 000006 idempotency → 000007 outbox → 000008 seed_system_accounts.

## Terjemahan error pgx → domain (di repository)
```go
var pgErr *pgconn.PgError
if errors.As(err, &pgErr) {
    switch {
    case pgErr.Code == "23514" && pgErr.ConstraintName == "chk_wallet_non_negative": return domain.ErrInsufficientBalance
    case pgErr.Code == "23505" && pgErr.ConstraintName == "uq_reversal":            return domain.ErrAlreadyReversed
    case pgErr.Code == "23505" && pgErr.ConstraintName == "idx_users_email":        return domain.ErrEmailTaken
    case pgErr.Code == "23514": /* trigger balanced memakai check_violation */        return domain.ErrUnbalanced
    }
}
```

## Pool (docs/02 §5)
MaxConns 25 · MinConns 5 · MaxConnLifetime 30m · MaxConnIdleTime 5m · Ping saat startup.

## Checklist EXPLAIN sebelum commit query baru
- [ ] `EXPLAIN (ANALYZE, BUFFERS)` dijalankan lewat `docker compose exec -T postgres psql -U nusa -d nusaledger`
- [ ] Tidak ada Seq Scan pada entries / transactions / idempotency_keys
- [ ] Index yang dipakai sesuai rencana docs/02 §2
- [ ] Hasilnya disimpan di docs/evidence/
