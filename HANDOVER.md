# HANDOVER

## Terakhir dikerjakan (2026-09-16)
Selesai: Sesi 1 (semua alat terpasang & terverifikasi) dan Sesi 2 (bootstrap repo, `.claude/`, docs dengan perbaikan F-01/F-02/F-03/K-01).

## Berikutnya
Sesi 3: `make up` → verifikasi Postgres healthy. Lalu Sesi 4: tulis 8 pasang migration sesuai docs/02 §2 + K-01 + K-05.

## Keputusan yang sudah diambil
- Semua keputusan docs/06 §2 (F-01…F-06, K-01…K-09) DISETUJUI pemilik proyek pada 2026-09-16.
- Go 1.27.0 (bukan 1.25 — itu yang terpasang lewat winget). Module: `github.com/caesarovera/nusaledger` (ganti kalau username GitHub berbeda: `go mod edit -module ...` + sed impor).
- Router chi v5, JWT golang-jwt/v5, hash argon2id, config caarlos0/env, driver pgx/v5.

## Catatan / jebakan
- Di PowerShell, PATH sesi lama tidak melihat alat baru; muat ulang: `$env:Path = [Environment]::GetEnvironmentVariable('Path','Machine') + ';' + [Environment]::GetEnvironmentVariable('Path','User')`.
- winget id k6 adalah `GrafanaLabs.k6`, bukan `k6.k6`.
- Makefile memakai `SHELL := bash` (Git Bash ada di PATH lewat laragon). `make help` menampilkan daftar perintah.
- `ANTHROPIC_API_KEY` kosong — Claude Code memakai kuota Pro, bukan kredit API. Jangan diset di env sistem.

## Belum diputuskan
- Username GitHub untuk module path (asumsi: caesarovera).
