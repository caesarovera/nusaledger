# HANDOVER

## Terakhir dikerjakan (2026-09-16) — DIHENTIKAN atas permintaan pemilik setelah Sesi 29
Selesai: Sesi 1–29. Semua kode Fase 1, T-01…T-14 hijau (T-04 ×5), E2E HTTP hijau, lint 0 issue, govulncheck bersih, image 18,6 MB, `docker compose up` → ready 4 s, CI workflow, OpenAPI, README final, k6 dijalankan.
Coverage: domain 99,1 %, service 84,2 %. Bukti lengkap di `docs/evidence/`. Jurnal sampai Sesi 28 (+ analisis k6 di `docs/evidence/k6.md`).
**SLO latensi TIDAK tercapai** (177 tps, p95 515 ms, p99 568 ms) — correctness 100 %. Penyebab: baris panas `SYSTEM_FEE_REVENUE` (semua transfer mengunci akun fee yang sama) + Docker Desktop Windows. Belum diperbaiki, sengaja dicatat jujur di README dan k6.md.
G-1 dilakukan inline saat menulis migration. G-2 dan G-3 (review Fable read-only) **belum** dijalankan.

## Berikutnya
1. Sesi 30: `/security-review` pada seluruh diff, G-3 (review rilis), lalu tag `v1.0.0`.
2. CI belum pernah jalan di GitHub (repo baru di-push) — cek hasil run pertama; kemungkinan perlu penyesuaian coverage gate di `ci.yml`.
3. Perbaikan performa (opsional Fase 1.1): sharding akun fee → ukur ulang k6 di Linux native. Jangan mengubah `Post()` tanpa mengulang T-04…T-09.
4. Jurnal Sesi 29–30 belum ditulis (isinya ada di `docs/evidence/k6.md` §Analisis).

## Keputusan yang sudah diambil
- Semua keputusan docs/06 §2 (F-01…F-06, K-01…K-09) DISETUJUI pemilik proyek pada 2026-09-16.
- Go 1.27.0 (bukan 1.25 — itu yang terpasang lewat winget). Module: `github.com/caesarovera/nusaledger` (ganti kalau username GitHub berbeda: `go mod edit -module ...` + sed impor).
- Router chi v5, JWT golang-jwt/v5, hash argon2id, config caarlos0/env, driver pgx/v5.

## Catatan / jebakan
- **Port 5432 host dipakai PostgreSQL laragon.** Compose memetakan 5433→5432. `DATABASE_URL=postgres://nusa:nusa_dev_only@127.0.0.1:5433/nusaledger?sslmode=disable`.
- **Port 8080 host juga dipakai laragon.** Compose memetakan 8081→8080; `.env.example` `ADDR=:8081`. API dev: `http://localhost:8081`.
- Load test WAJIB memakai `docker-compose.load.yml` (rate limit dinaikkan); tanpa itu k6 kena 401/429 dan angkanya tidak bermakna.
- `migrate` harus dipasang dengan `-tags postgres`, kalau tidak: `unknown driver postgres`.
- Reset DB dev: `make db-reset` (DROP SCHEMA + up). `migrate drop` tidak membuang enum; `down -all` gagal bila `entries` sudah berisi data (disengaja).
- Trigger balanced bersifat DEFERRED: error muncul di `Commit()`, bukan `Exec()`. `PgError.Code=23514`, `ConstraintName='trg_entries_balanced'`.
- Di PowerShell, PATH sesi lama tidak melihat alat baru; muat ulang: `$env:Path = [Environment]::GetEnvironmentVariable('Path','Machine') + ';' + [Environment]::GetEnvironmentVariable('Path','User')`.
- winget id k6 adalah `GrafanaLabs.k6`, bukan `k6.k6`.
- Makefile memakai `SHELL := bash` (Git Bash ada di PATH lewat laragon). `make help` menampilkan daftar perintah.
- `ANTHROPIC_API_KEY` kosong — Claude Code memakai kuota Pro, bukan kredit API. Jangan diset di env sistem.

## Belum diputuskan
- Username GitHub untuk module path (asumsi: caesarovera).
