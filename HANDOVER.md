# HANDOVER

## Terakhir dikerjakan (2026-09-16)
Selesai: Sesi 1–9 (seluruh Minggu 1). Config + logger + pool; domain lengkap dengan builder 4 operasi; unit test hijau dengan `-race`: domain 93,5 %, config 90 %. Lint bersih. Bug #1 ditemukan lewat test (env `required` vs `notEmpty`).

## Berikutnya
Sesi 11: harness integration test (testcontainers + migrate dari `../../migrations`, `TestMain` satu container per paket, 3 helper invariant). Lalu Sesi 12–13: rencana & implementasi `LedgerRepo.PostTransaction`.
G-1 (review skema, Fable) belum dijalankan — jadwalkan sebelum Sesi 13.

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
