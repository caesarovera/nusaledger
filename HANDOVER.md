# HANDOVER

## Terakhir dikerjakan (2026-09-16, lanjutan) — Menutup dua lubang dokumentasi + tag `v1.2.0`
Ditanya "apakah dari tahap setup awal sampai sekarang sudah dijelaskan step by step". Jawabannya sebagian besar ya, tapi ditemukan dua lubang nyata lewat pemeriksaan ulang `docs/JURNAL-BELAJAR.md`:

1. **Setup GitHub CLI, remote, push pertama, dan CI** hanya tercatat sebagai log fakta singkat di HANDOVER ("Update — push pertama & CI"), bukan format belajar Apa/Kenapa/Contoh/Bukti.
2. **Identitas git** (`git config user.name "Overa Caesar"`, LOKAL bukan `--global`) tidak tercatat sama sekali, padahal ini aturan tetap yang diminta eksplisit.

Ditutup dengan **Sesi 38 dan 39** (jurnal, ditulis mundur — kejadiannya SUDAH lama, dokumentasinya yang baru sekarang lengkap). Sesi 38 juga mencatat temuan jujur: `CLAUDE.md` sudah berisi "Jangan git push" SEJAK COMMIT PERTAMA, sebelum push pertama itu terjadi — dijelaskan kenapa itu bukan kontradiksi (permintaan eksplisit di momen itu mengesahkan satu tindakan spesifik, bukan mengubah aturan baku).

**Juga dibuat tag `v1.2.0`** (lokal, BELUM di-push) menandai Fase 2 benar-benar selesai total — tag `v1.1.0` sebelumnya hanya menunjuk ke slice outbox pertama (commit `3aef909`), sebelum tiga pekerjaan terakhir (role DB, Redis, `/metrics`). `v1.1.0` TIDAK diubah/dipindah, `v1.2.0` ditambahkan di HEAD saat ini (`4a56ddf`).

**Status push:** empat commit terbaru (role DB, Redis, `/metrics` docs, dua entri jurnal) + tag `v1.2.0` masih LOKAL SAJA, belum di-push — sesuai `CLAUDE.md` ("Jangan git push. Push adalah keputusan manusia"), menunggu keputusan Anda.

## Terakhir dikerjakan (2026-09-16, lanjutan) — Batasi `/metrics` di jaringan (Fase 2, item terakhir — FASE 2 SELESAI TOTAL)
Item ketiga dan terakhir dari "lanjutkan berdasarkan prioritas terpenting dahulu". Sengaja BUKAN perubahan kode: `/metrics` tanpa autentikasi di level aplikasi TETAP demikian (mengubahnya akan merusak model *scraping* Prometheus standar), pembatasan yang benar ada di jaringan. Ditulis `docs/deploy-metrics-network.md` — contoh konfigurasi nginx (`allow`/`deny` per CIDR), Caddy (`remote_ip` matcher), dan alternatif `NetworkPolicy` Kubernetes (lebih kuat: menempel di Pod, bukan per instance proxy) — plus penjelasan kenapa dev repo ini SUDAH aman (semua port diikat `127.0.0.1` sejak audit F-2).

**Fase 2 sekarang selesai total**: outbox relay (Sesi 34), role DB terbatas (Sesi 35), rate limit Redis (Sesi 36), pembatasan jaringan `/metrics` (Sesi 37, dokumentasi). Tidak ada item Fase 2 yang tersisa.

## Terakhir dikerjakan (2026-09-16, lanjutan) — Rate limit Redis opsional (Fase 2, prioritas kedua dari 3 sisa item)
Lanjutan langsung dari item sebelumnya (role DB). Dikerjakan `RedisLimiter` (`internal/platform/ratelimit/redis.go`) yang mengimplementasikan kontrak `allower` (`Allow(key string) bool`) yang SAMA dengan `Limiter` in-memory — jadi `router.go`/`middleware.go` tidak disentuh sama sekali.

**Dikerjakan:**
- `RedisLimiter`: fixed-window counter via skrip Lua (`INCR` + `PEXPIRE` atomik di sisi server — dua panggilan terpisah berisiko key tanpa TTL kalau proses mati di antaranya). Fail-closed saat Redis error/timeout/tidak terjangkau (docs/03 §6), timeout 500ms sendiri (tidak mewarisi context request HTTP, supaya Redis yang lambat tidak ikut memperlambat semua request).
- `config.RedisURL` (opsional, `env:"REDIS_URL"`): kosong → tetap limiter in-memory (default, cukup 1 instance); diisi → limiter Redis. `cmd/api/main.go` mendapat `newLimiters()` sebagai satu titik keputusan backend.
- `docker-compose.yml`: service `redis` baru (redis:7-alpine, `127.0.0.1:6380` — port 6379 host dipakai laragon), `REDIS_URL` diisi untuk `api`.
- 3 test baru (`test/integration/ratelimit_redis_test.go`, Redis sungguhan via testcontainers): jendela tetap dengan TTL asli, hitungan terbukti dibagi lintas DUA instance `*RedisLimiter` Go yang berbeda (skenario nyata: 2 instance `cmd/api`), fail-closed saat Redis tidak terjangkau.
- Diverifikasi lewat `docker compose up` sungguhan: 6 percobaan login → percobaan ke-6 dapat 429, dan `redis-cli KEYS "ratelimit:*"` menunjukkan key benar-benar tersimpan di Redis.
- Build, vet, lint (0 issue), unit+integration `-race` penuh (33 test, 25 detik), `govulncheck` (0 vulnerabilitas nyata) — semua hijau.

## Terakhir dikerjakan (2026-09-16, lanjutan) — Role DB terbatas untuk `entries` (Fase 2, prioritas tertinggi dari 3 sisa item)
Diminta "lanjutkan berdasarkan prioritas terpenting dahulu" atas 3 sisa item Fase 2 (rate limit Redis, role DB, batas jaringan `/metrics`). Dipilih role DB LEBIH DULU: kegagalan tanpa lapis ini berarti data ledger yang salah (tidak bisa di-rollback lewat restart), sedangkan dua item lain "hanya" konsistensi rate limit dan kebocoran informasi — alasan lengkap di jurnal Sesi 35.

**Dikerjakan:**
- Migration `000011_app_role`: peran `nusaledger_app` (LOGIN, bukan superuser), GRANT baseline SELECT/INSERT/UPDATE/DELETE semua tabel, lalu REVOKE UPDATE+DELETE khusus `entries` dan REVOKE DELETE khusus `transactions`. Nama database TIDAK di-hardcode — pakai `current_database()` lewat `EXECUTE format(...)` supaya migration yang sama benar di dev (`nusaledger`) maupun testcontainers (`testdb`).
- `internal/config.Config` + `cmd/worker` mendapat `MigrationDatabaseURL`/`MIGRATION_DATABASE_URL` (opsional, fallback ke `DatabaseURL`): migrasi tetap jalan sebagai superuser `nusa`, runtime `cmd/api`/`cmd/worker` connect sebagai `nusaledger_app` yang terbatas.
- `docker-compose.yml`: `api` dan `worker` sekarang `DATABASE_URL` memakai `nusaledger_app`; `api` mendapat `MIGRATION_DATABASE_URL` terpisah (superuser) khusus untuk `RUN_MIGRATIONS`.
- Test baru `TestAppRole_TidakBisaMengubahLedger` (6 subtest) — connect SUNGGUHAN sebagai `nusaledger_app` (bukan superuser `test` yang dipakai test lain), membuktikan UPDATE/DELETE/TRUNCATE `entries` dan DELETE `transactions` ditolak Postgres sendiri dengan SQLSTATE `42501`, BERBEDA dari trigger `forbid_mutation` (T-03, SQLSTATE `23514`) — dua lapis pertahanan independen, dibuktikan dengan dua test independen.
- Diverifikasi: build OK, `go vet` bersih, `golangci-lint` 0 issue, unit `-race` semua paket OK, integration `-race` penuh 30 test (22,8 detik) OK termasuk test baru, `govulncheck` 0 vulnerabilitas nyata.

## Terakhir dikerjakan (2026-09-16, lanjutan) — Fase 2 dimulai: outbox relay + RabbitMQ + consumer
Diminta "lanjutkan" setelah dikonfirmasi maksudnya memulai Fase 2 (docs/01 §1.3, sebelumnya di luar cakupan Fase 1 secara sengaja). Fase 2 TIDAK punya PRD sendiri seperti Fase 1 — dikerjakan dengan asumsi eksplisit mengikuti arah yang SUDAH dinyatakan di docs/02 §2.9 ("saat Fase 2 tiba, yang perlu ditambahkan hanya relay") dan docs/06: RabbitMQ, bukan Kafka.

**Dikerjakan (slice pertama Fase 2):**
- `migrations/000010_processed_events`: tabel dedup consumer (UNIQUE `event_id`).
- `internal/platform/broker`: topologi RabbitMQ (exchange topic `ledger.events`, queue `ledger.audit-log` + DLQ `ledger.audit-log.dlq`), `Dial` dengan retry+backoff.
- `internal/platform/outbox`: `Relay.PollOnce` — `FOR UPDATE SKIP LOCKED` (aman multi-instance), publisher confirm, `event_id` dibawa lewat AMQP `MessageId`.
- `internal/consumer.AuditLog`: consumer IDEMPOTEN (insert `processed_events` sebelum kerja; redelivery dilewati, bukan diproses ulang). Payload cacat → DLQ (nack tanpa requeue); kegagalan DB → requeue (retry, bukan DLQ) — dua kelas error dibedakan sengaja.
- `cmd/worker`: binary baru, DI manual seperti `cmd/api`, `signal.NotifyContext` untuk graceful shutdown.
- `docker-compose.yml`: service `rabbitmq` (management UI di 127.0.0.1:15673) dan `worker`; `Dockerfile.worker` (image 12,9 MB).
- Test: `TestOutbox_RelayDanConsumer_EndToEnd` (RabbitMQ sungguhan via testcontainers, `-race`) — jalur bahagia, publisher confirm, DAN redelivery idempoten (event yang sama dua kali → `processed_events` tetap 1 baris).
- Diverifikasi jalan sungguhan lewat `docker compose up` dari nol: topup nyata → relay mengirim ~1 detik → consumer mencatat → `SIGTERM` graceful shutdown terbukti (`docs/evidence/fase2-outbox.md`).

**Fase 2, sisa item (belum dikerjakan):** rate limit Redis (in-memory sekarang cukup untuk 1 instance API, tidak konsisten kalau diskalakan >1), role DB aplikasi tanpa hak `UPDATE/DELETE/TRUNCATE` pada `entries` (sudah direkomendasikan docs/02 §2.6, belum diimplementasikan), batasi `/metrics` di reverse proxy.

## Terakhir dikerjakan (2026-09-16, lanjutan) — Sharding akun fee (item performa terakhir)
Diminta lagi "lanjutkan task yang belum selesai sesuai plan" — satu-satunya item Fase 1 yang masih tersisa (sharding akun fee, sebelumnya sengaja dilewati) dikerjakan:

- **Migration `000009_fee_shards`**: index unik lama (`idx_accounts_system_type`, tepat satu baris per jenis akun sistem) diganti index yang HANYA berlaku untuk `SYSTEM_CASH`/`SYSTEM_SUSPENSE`; `SYSTEM_FEE_REVENUE` boleh banyak baris. +7 shard fee (total 8).
- `AccountStore.SystemAccounts` (jamak) baru; `service.Ledger` menyimpan `feeIDs []int64`, memilih satu secara **acak** per transfer lewat `pickFeeShard()`. Repo-level test (T-04, T-07, T-08/09, reversal) memanggil `domain.NewTransfer` langsung dan TIDAK terpengaruh (mereka menentukan akun fee sendiri) — blast radius perubahan ini kecil dan terverifikasi sebelum implementasi.
- `resetDB` test helper diperbarui menanam 8 shard (bukan 1) supaya test benar-benar melewati jalur `pickFeeShard()`. Dua assertion jumlah baris `accounts` yang hardcode (`TestSchema_SeedAkunSistem`, `TestUserRepo_CreateWithWalletDanRefreshToken`) disesuaikan. Test baru `TestHTTP_FeeSharding` membuktikan fee benar-benar tersebar ke >1 shard.
- **k6 diulang di lingkungan yang SAMA** (Docker Desktop Windows — bukan Linux native seperti semula direncanakan, karena mesin dev tetap Windows): throughput 177→**546 tps** (×3,1), p99 568→**288 ms** (lulus), p95 515→**210 ms** (hampir lulus, 10 ms di atas target). Trial balance & drift tetap 0; fee tersebar rata ke 8 shard (selisih <2%). Detail: `docs/evidence/k6-sharded.md`.
- Diverifikasi `-race` penuh (unit + integration, T-04/T-07/T-08-09/FeeSharding ×3), lint 0 issue. Migration diuji rollback+reapply di DB dev sungguhan, bukan hanya testcontainers.

**Fase 1 (termasuk Fase 1.1) SEKARANG BENAR-BENAR SELESAI TOTAL** — tidak ada item dalam cakupan Fase 1 yang tersisa, termasuk item performa. Sisa p95 10 ms dan seluruh Fase 2 dicatat eksplisit sebagai di luar cakupan/belum, bukan terlupa.

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
**Fase 1 SELESAI TOTAL** (v1.0.3, sudah di-tag). **Fase 2 SEKARANG SELESAI TOTAL JUGA** — outbox relay, role DB terbatas, rate limit Redis, pembatasan jaringan `/metrics`, semua selesai dan terverifikasi. Tidak ada item Fase 2 yang tersisa.
1. **Pertimbangkan tag `v1.1.0`** untuk memuat seluruh Fase 2 (`cmd/worker`, migration `000010` & `000011`, `RedisLimiter`, `docs/deploy-metrics-network.md`) — versi minor, bukan `v1.0.4`.
2. Satu-satunya item lama yang masih terbuka di seluruh proyek: pengukuran ulang k6 di Linux native untuk menutup selisih p95 10 ms dari Sesi 33 (bukan blocker, dicatat sadar sebagai keterbatasan Docker Desktop Windows).
3. Tidak ada rencana Fase 3 yang dikunci — kalau diminta melanjutkan lagi, tanyakan dulu arah yang diinginkan (Fase 2 tidak punya PRD sejak awal; sudah dikerjakan atas asumsi eksplisit yang dicatat di jurnal Sesi 34).

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
