# HANDOVER

## Terakhir dikerjakan (2026-09-16, lanjutan) — Fase 1.1: tiga item susulan dari HANDOVER "Berikutnya"
Diminta "lanjutkan tasklist yang belum sesuai plan" — dikerjakan tiga item Fase 1.1 yang tercatat di bawah (bukan item Fase 2, itu sengaja di luar cakupan per docs/01 §1.3):
1. **Sentinel `ErrIntegrityViolation`** dipisah dari `ErrInvalidReversalLink`. `translate()` (postgres/errors.go) sekarang memetakan `chk_normal_balance`/`chk_owner` → `ErrIntegrityViolation`; `chk_reversal_link` tetap → `ErrInvalidReversalLink`.
2. **Rate limit** `/auth/refresh` (per IP, `RefreshLimiter`/`REFRESH_RATE_LIMIT`) dan `topup`+`withdraw` (per actor, satu `MoneyLimiter`/`MONEY_RATE_LIMIT` bersama — keduanya operasi tulis satu-akun yang setara, sengaja tidak berbagi budget dengan `transfer`). Test: `TestHTTP_RefreshRateLimit`, `TestHTTP_MoneyRateLimit`.
3. **Refresh-token reuse detection.** `RefreshTokenStore` mendapat method baru `RevokeAllForUser`. `Auth.Refresh` sekarang memeriksa `rt.RevokedAt != nil` SEBELUM cek `Usable` — kalau token yang diajukan sudah pernah dicabut (dirotasi sebelumnya) tapi dipakai lagi, itu sinyal token dicuri, dan **semua sesi user tersebut dicabut** (tanpa migrasi skema — cukup `UPDATE ... WHERE user_id=$1 AND revoked_at IS NULL`, bukan true "token family" berbasis kolom baru). Test: `TestRefresh_ReuseTerdeteksi_CabutSemuaSesi` (membuktikan sesi LAIN yang tidak terlibat pun ikut tercabut).

**Item performa (sharding akun fee) SENGAJA dilewati** — perlu pengukuran ulang k6 di Linux native, tidak bisa diverifikasi bermakna di Docker Desktop Windows. Tetap tercatat di README §Yang akan diperbaiki.

Semua tiga fix diverifikasi `-race` penuh (unit + integration, T-04 ×3), lint/vet bersih.

## Terakhir dikerjakan (2026-09-16, lanjutan) — Audit keamanan penuh setelah tag v1.0.0
Repo sudah di-tag `v1.0.0` (lihat entri di bawah), lalu diminta menjalankan **audit keamanan penuh** (bukan diff, seluruh repo). Model Opus (read-only) untuk audit kode; sesi utama untuk pemeriksaan mekanis (grep secret, deep-scan git history, govulncheck verbose) — bersih total, tidak ada secret yang pernah bocor.

**1 temuan High, ditutup segera** (repo sudah publik): `GetTransaction` memfilter header transaksi dengan benar tapi query ENTRIES di bawahnya tidak difilter sama sekali, dan bug yang sama ada di respons *langsung* topup/transfer/withdraw — siapa pun yang bertransaksi melihat saldo pihak lain, termasuk saldo kumulatif akun sistem (`SYSTEM_CASH`, `SYSTEM_FEE_REVENUE`). Ditutup dengan `domain.PostResult.ViewerAccountID`: service (`Topup`/`Withdraw`/`Transfer`/`GetTransaction`) menandai akun mana yang boleh menampilkan `balance_after`; dto.go merender `null` untuk yang lain. `amount_sen`/`fee_sen` tetap benar untuk semua pihak. Test baru: `TestHTTP_B1_SaldoPihakLainTidakBocor`.

3 Medium lain: 2 ditutup (JWT secret ≥32 byte tanpa syarat `APP_ENV`; port dev Postgres/API diikat `127.0.0.1` bukan `0.0.0.0`), 1 diterima (`/metrics` publik — pembatasan yang benar ada di jaringan, bukan kode). Beberapa Low ditutup sekalian (validasi `reverseRequest`, header keamanan HTTP di root router). Sisanya (9 Low, 4 Info) didokumentasikan sebagai keterbatasan Fase 1 di README, bukan disembunyikan.

Semua perbaikan diverifikasi `-race` penuh (unit + integration, T-04 ×3) dan lint/vet bersih sebelum commit.

## Terakhir dikerjakan (2026-09-16) — G-2, G-3, dan tag v1.0.0
Selesai: Sesi 1–29 + G-2 + G-3 + perbaikan hasil keduanya. Semua kode Fase 1, T-01…T-14 hijau (T-04 ×5), T-10c baru, E2E HTTP hijau (24 test integration), lint 0 issue, govulncheck bersih, image 18,6 MB, `docker compose up` → ready 4 s, CI workflow (lulus di GitHub, run pertama), OpenAPI, README final, k6 dijalankan.
Coverage: domain 99,1 %, service 84,2 %. Bukti lengkap di `docs/evidence/`. Jurnal sampai Sesi 26 (G-2) — lihat entri terbaru.
**SLO latensi TIDAK tercapai** (177 tps, p95 515 ms, p99 568 ms) — correctness 100 %. Penyebab: baris panas `SYSTEM_FEE_REVENUE`. Belum diperbaiki, dicatat jujur di README dan k6.md; ini keputusan sadar, bukan celah.

**G-2** (Fable, read-only, atas `LedgerRepo.Post` & konkurensi): tidak ada temuan penciptaan/kehilangan uang atau deadlock. 4 perbaikan diterapkan & diverifikasi `-race`: konsistensi `Validate()` Type/ReversesID (`ErrInvalidReversalLink`), `translate()` melengkapi SQLSTATE (deadlock/serialization → retry, bukan 500), pre-check saldo overflow-safe, **bug nyata** di `postIdempotent` (kalah balapan idempotency salah mengembalikan IN_FLIGHT padahal seharusnya CONFLICT). Ditambah T-10c (10 reversal konkuren) dan assert `conflict==0` di T-04.

**G-3** (Fable, read-only, release gate): kesimpulan **YA-DENGAN-CATATAN**. Satu temuan ditindaklanjuti sebelum tag: `/auth/register` tanpa rate limit (argon2id mahal, vektor DoS ringan) → ditambah `rateLimitByIP` + `REGISTER_RATE_LIMIT` (default 10/15 menit). Temuan minor lain (limiter login per email, `/metrics` publik, penamaan `ErrInvalidReversalLink` untuk constraint accounts) diterima sebagai trade-off Fase 1, dicatat di README.

## Berikutnya
Fase 1 (termasuk Fase 1.1) **selesai** — semua item yang tercatat "belum" sudah ditutup atau didokumentasikan sebagai keterbatasan sadar. Sisa pekerjaan:
1. **Pertimbangkan tag `v1.0.2`** untuk memuat tiga perbaikan Fase 1.1 di atas — `v1.0.1` tidak memuatnya.
2. Perbaikan performa (butuh Linux native, bukan blocker): sharding akun fee → ukur ulang k6. Jangan mengubah `Post()` tanpa mengulang T-04…T-09.
3. Fase 2 (di luar cakupan Fase 1 secara sengaja, docs/01 §1.3): outbox relay, worker, rate limit Redis, batasi `/metrics` di jaringan.

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

## Update — push pertama & CI (2026-09-16)

`gh` CLI sudah terpasang sebelumnya (v2.101.0, `C:\Program Files\GitHub CLI\gh.exe`) dan sudah login sebagai `caesarovera` (scope repo, workflow). Remote `origin` ditambahkan ke `https://github.com/caesarovera/nusaledger.git` (repo publik, sudah dibuat kosong oleh pemilik), lalu `git push -u origin main`.

CI (`.github/workflows/ci.yml`) otomatis terpicu dan **lulus semua 4 job di percobaan pertama**: lint (50s) → unit (1m3s) → integration termasuk T-04×3 (1m17s) → image <20MB (53s). Link: https://github.com/caesarovera/nusaledger/actions/runs/35033810565

Catatan: annotation "Node.js 20 deprecated" pada tiga job — bukan kegagalan, GitHub Actions runner otomatis fallback ke Node 24. Tidak perlu ditindaklanjuti kecuali actions/checkout atau setup-go merilis versi yang mensyaratkan update.
