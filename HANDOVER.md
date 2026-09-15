# HANDOVER

## Terakhir dikerjakan (2026-09-16)
Selesai: Sesi 1–13. `LedgerRepo.Post` atomik (idempotency, FOR UPDATE terurut, optimistic lock, reversal, outbox), AccountRepo, IdempotencyRepo, harness testcontainers; 11 integration test hijau dengan `-race` (T-02, T-03, T-10, T-10b, K-05).
G-1 (review skema) dilakukan inline oleh sesi utama saat menulis migration: menambah `chk_owner`, `chk_reversal_link`, `uq_entry_account_per_txn`, dan `CONSTRAINT` name pada RAISE.

## Berikutnya
Sesi 19 (dimajukan, di level repository): test konkurensi T-04, T-05, T-07, T-08, T-09 → `test/integration/concurrency_test.go`. Lalu Sesi 14: EXPLAIN cursor query → `docs/evidence/`. Lalu Sesi 18: service + ports.

## Keputusan yang sudah diambil
- Semua keputusan docs/06 §2 (F-01…F-06, K-01…K-09) DISETUJUI pemilik proyek pada 2026-09-16.
- Go 1.27.0 (bukan 1.25 — itu yang terpasang lewat winget). Module: `github.com/caesarovera/nusaledger` (ganti kalau username GitHub berbeda: `go mod edit -module ...` + sed impor).
- Router chi v5, JWT golang-jwt/v5, hash argon2id, config caarlos0/env, driver pgx/v5.

## Catatan / jebakan
- **Port 5432 host dipakai PostgreSQL laragon.** Compose memetakan 5433→5432. `DATABASE_URL=postgres://nusa:nusa_dev_only@127.0.0.1:5433/nusaledger?sslmode=disable`.
- `migrate` harus dipasang dengan `-tags postgres`, kalau tidak: `unknown driver postgres`.
- Reset DB dev: `make db-reset` (DROP SCHEMA + up). `migrate drop` tidak membuang enum; `down -all` gagal bila `entries` sudah berisi data (disengaja).
- Trigger balanced bersifat DEFERRED: error muncul di `Commit()`, bukan `Exec()`. `PgError.Code=23514`, `ConstraintName='trg_entries_balanced'`.
- Di PowerShell, PATH sesi lama tidak melihat alat baru; muat ulang: `$env:Path = [Environment]::GetEnvironmentVariable('Path','Machine') + ';' + [Environment]::GetEnvironmentVariable('Path','User')`.
- winget id k6 adalah `GrafanaLabs.k6`, bukan `k6.k6`.
- Makefile memakai `SHELL := bash` (Git Bash ada di PATH lewat laragon). `make help` menampilkan daftar perintah.
- `ANTHROPIC_API_KEY` kosong — Claude Code memakai kuota Pro, bukan kredit API. Jangan diset di env sistem.

## Belum diputuskan
- Username GitHub untuk module path (asumsi: caesarovera).
