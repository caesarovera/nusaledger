# NusaLedger

> ⚠️ **Ini simulasi teknis, bukan produk keuangan.** Tidak ada uang sungguhan, tidak ada integrasi bank nyata, tidak ada lisensi regulator.

Ledger *double-entry* untuk dompet digital, ditulis dengan **Go 1.27** dan **PostgreSQL 17**. Dibangun sebagai portofolio untuk menunjukkan **correctness uang**, **konkurensi database**, dan **idempotency** — bukan sekadar CRUD.

[![ci](https://github.com/caesarovera/nusaledger/actions/workflows/ci.yml/badge.svg)](https://github.com/caesarovera/nusaledger/actions/workflows/ci.yml)

## Status (2026-09-16)

Fase 1 **hampir selesai**: seluruh kode, test T-01…T-14, Docker, CI, dan bukti operasional sudah ada. Yang tersisa: review keamanan akhir dan tag `v1.0.0`. SLO latensi **belum tercapai** di laptop pengembang (lihat *Hasil Pengujian*); penyebabnya sudah diidentifikasi dan dicatat jujur.

## Menjalankan

```bash
docker compose up -d --build        # PostgreSQL 17 + API (migrasi otomatis) → http://localhost:8081
curl localhost:8081/readyz          # "ready"
```

Pengembangan tanpa image:

```bash
docker compose up -d postgres       # PostgreSQL 17 di host port 5433
make migrate-up                     # skema + trigger + seed akun sistem
cp .env.example .env && make run    # API di :8081
make help                           # semua perintah
```

Contoh alur (lengkap di `docs/evidence/smoke-compose.md`):

```bash
curl -X POST localhost:8081/api/v1/auth/register -H 'Content-Type: application/json' \
  -d '{"email":"andi@x.com","password":"rahasia123","full_name":"Andi"}'
curl -X POST localhost:8081/api/v1/transactions/transfer \
  -H "Authorization: Bearer $TOKEN" -H "Idempotency-Key: $(uuidgen)" -H 'Content-Type: application/json' \
  -d '{"to_account_public_id":"<uuid dompet tujuan>","amount_sen":5000000,"description":"bayar kos"}'
```

## Arsitektur

```
 HTTP (chi)                 service                     domain (tanpa dependensi)
 ┌───────────────┐   ┌──────────────────────┐   ┌────────────────────────────┐
 │ handler tipis │──▶│ Ledger: Topup/Transfer│──▶│ Money (int64 sen, overflow) │
 │ mapError (1x) │   │  Withdraw/Reverse     │   │ Transaction.Validate()      │
 │ middleware:   │   │  postIdempotent       │   │ NewTopup/Transfer/Withdraw/ │
 │  auth, idem,  │   │ Auth: argon2id, JWT   │   │  Reversal (builder)         │
 │  rate limit   │   └──────────┬───────────┘   └────────────────────────────┘
 └───────────────┘              │ ports.go (interface di sisi konsumen)
                     ┌──────────▼───────────┐
                     │ repository/postgres  │  Post(): 1 transaksi DB =
                     │  LedgerRepo.Post     │  klaim idempotency → FOR UPDATE ORDER BY id
                     │  translate() SQLSTATE│  → header → entries + saldo (version) →
                     └──────────┬───────────┘  simpan hasil → outbox → COMMIT (trigger deferred)
                                ▼
                     PostgreSQL 17: entries append-only (trigger), Σdebit=Σkredit (constraint trigger
                     DEFERRED), CHECK saldo ≥ 0, PK idempotency (user_id, key), UNIQUE reversal
```

Arah dependensi: `transport → service → domain`, `repository → domain`. Domain tidak mengimpor apa pun dari lapisan lain.

## Keputusan Teknis

| Keputusan | Alternatif yang ditolak | Alasan |
|---|---|---|
| Uang = `type Money int64` dalam sen | `float64`, `int64` telanjang | Float tidak eksak; tipe sendiri membuat compiler menolak argumen tertukar; overflow dideteksi di `Add` |
| Saldo dimaterialisasi + `version`, `entries` sumber kebenaran | Selalu `SUM(entries)` | Baca instan & cek saldo atomik; job drift + metrik `ledger_balance_drift_total` membuktikan keduanya cocok |
| `SELECT … ORDER BY id FOR UPDATE` sebelum update saldo | Kunci sesuai urutan permintaan | Transfer silang A↔B antre, bukan deadlock — dibuktikan T-07 (0 deadlock) dan eksperimen tanpa klausa ini (98–99 deadlock) |
| Idempotency key diklaim **di dalam** transaksi yang sama; PK `(user_id, key)` | Tabel terpisah / key global | Crash setelah klaim tidak meninggalkan key yatim; key user lain tidak saling mengganggu |
| Trigger `DEFERRABLE INITIALLY DEFERRED` untuk Σdebit=Σkredit | Cek di aplikasi saja | Berlaku juga untuk skrip migrasi & perbaikan manual; entry disisipkan satu per satu jadi cek harus di COMMIT |
| Reversal = transaksi baru berlawanan arah | `UPDATE`/`DELETE` | Jejak audit utuh; `UNIQUE(reverses_transaction_id)` menutup double reversal di DB |
| Filter kepemilikan di `WHERE` (`EXISTS entries …`) | Cek di Go setelah query | Data milik orang lain tidak pernah keluar dari database (IDOR) |
| Builder transaksi di **domain** | Di service/handler | Worker Fase 2 memakai jalur yang sama; tidak ada cara menyusun arah entry yang salah |
| `mapError` satu tempat di transport | Status code di tiap handler | Kesalahan pemetaan (500 padahal 422) tidak bisa tersebar |
| Rate limit fail-closed, backend in-memory ATAU Redis di balik interface sama (`REDIS_URL` opsional) | Selalu satu atau selalu yang lain | Fase 1 satu instance cukup in-memory; Fase 2 `REDIS_URL` diisi → hitungan konsisten lintas banyak instance `cmd/api`, tanpa mengubah `router.go` sama sekali (Sesi 36) |
| Rate limit `/auth/register` per IP (ditambah pasca G-3) | Tanpa limiter | argon2id (64 MiB, t=3) mahal; registrasi anonim berulang bisa menghabiskan CPU/memori tanpa pembatas |
| Tanpa `middleware.RealIP` | Percaya `X-Forwarded-For` | Header itu bisa dipalsukan siapa pun tanpa proxy tepercaya (GHSA-3fxj-6jh8-hvhx) |
| `ViewerAccountID` pada `PostResult`: respons transaksi hanya menampilkan `balance_after` milik pemanggil sendiri | Tampilkan semua entry apa adanya | Tanpa ini, transfer/topup/withdraw membocorkan saldo pihak lain — termasuk saldo kumulatif akun sistem — ke pengguna biasa (audit keamanan penuh, temuan B-1, High) |
| JWT secret minimal 32 byte **tanpa syarat** `APP_ENV` | Hanya wajib saat `APP_ENV=production` | Lupa menyetel `APP_ENV` di produksi tidak lagi diam-diam meloloskan secret lemah (temuan A-1) |
| Port dev (`5433`, `8081`) terikat `127.0.0.1` | Terikat `0.0.0.0` (default Docker) | Postgres berpassword sama untuk semua orang yang clone repo ini tidak boleh terjangkau LAN/Wi-Fi (temuan F-2) |
| Header keamanan HTTP (`nosniff`, CSP, dll.) di root router | Tidak dipasang | Murah, tidak ada ruginya untuk API JSON-only; melindungi juga `/healthz`/`/readyz`/`/metrics` |

## Hasil Pengujian

| Kategori | Hasil | Bukti |
|---|---|---|
| Unit (`-race`) | domain **99,1 %**, service 84,2 %, config 90 %, ratelimit 100 % | `make test` |
| Integration (testcontainers, Postgres 17) | **24 test hijau**: T-02, T-03, T-10, T-10b, **T-10c**, T-11, K-05, repo user/token, register rate limit | `make test-int` |
| **T-04** 100 goroutine dari Rp 1.000.000 @ Rp 50.000 + fee | **tepat 19 sukses**, sisa Rp 31.000, ×5, `conflict==0` diassert | `test/integration/concurrency_test.go` |
| **T-05** 10 goroutine key sama | tepat 1 transaksi | idem |
| **T-07** 50× A→B ∥ 50× B→A | 100 sukses, **0 deadlock** | idem |
| **T-08/T-09** 1.000 transaksi acak | trial balance 0, drift 0 | idem |
| **T-10c** 10 reversal konkuren atas transaksi yang sama | tepat 1 sukses, 9× `ALREADY_REVERSED`, saldo pulih sekali | `test/integration/ledger_repo_test.go` |
| **T-13** graceful shutdown | request 1,5 s selesai saat context dibatalkan di 0,3 s (simulasi SIGTERM lewat `signal.NotifyContext`; uji `kill -TERM` manual belum direkam sebagai bukti) | `internal/app/server_test.go` |
| E2E HTTP | idempotency replay **byte-identik**, 400/401/403/404/409/413/422/429 | `test/integration/http_test.go` |
| Trigger & constraint (psql, tanpa Go) | 14 skenario HARUS GAGAL semua gagal | `docs/evidence/trigger-test.md` |
| Cursor pagination @200.000 entries | `Index Scan` 0,49 ms | `docs/evidence/explain-cursor.md` |
| Lint / vuln | `golangci-lint` 0 issue, `govulncheck` bersih | `make lint` |
| Image | **18,6 MB** distroless nonroot; `docker compose up` → ready 4 s | `docs/evidence/smoke-compose.md` |

### Load test (k6, 100 VU, 3 menit) — SLO tercapai setelah sharding akun fee

| Metrik | Target | Sebelum (1 akun fee) | **Setelah (8 shard fee)** |
|---|---|---|---|
| Ketepatan saldo | 100 % | ✅ trial balance 0, drift 0 | ✅ trial balance 0, drift 0, fee tersebar rata ke 8 shard (selisih <2%) |
| Error rate | < 1 % | ✅ 0,00 % | ✅ 0,00 % |
| Throughput | ≥ 200/s | ❌ 177/s | ✅ **546/s** (×3,1) |
| p95 | < 200 ms | ❌ 515 ms | 🟡 **210 ms** (hampir tercapai) |
| p99 | < 500 ms | ❌ 568 ms | ✅ **288 ms** |
| Transfer selesai (3 menit) | — | 33.340 | **104.333** |

**Penyebab yang teridentifikasi (Sesi 29) dan diperbaiki (Sesi 33, migration `000009_fee_shards`):** setiap transfer mengunci akun `SYSTEM_FEE_REVENUE` yang **sama**, membuat seluruh sistem terserialisasi pada satu baris panas. Perbaikan: fee dipecah ke 8 akun ("shard"), dipilih **acak** per transfer (`pickFeeShard()` di `internal/service/ledger.go`); trial balance & job drift tidak berubah karena keduanya menjumlahkan seluruh entries/accounts, agnostik jumlah shard. Dibuktikan `TestHTTP_FeeSharding` (fee benar-benar tersebar, bukan diam-diam tetap satu akun) dan diverifikasi ulang T-04/T-07/T-08/T-09 (`-race`, ×3) tanpa regresi.

⚠️ Perbandingan di atas dijalankan pada **lingkungan yang sama** (Docker Desktop Windows, satu laptop untuk klien dan server) — bukan Linux native seperti semula direncanakan, karena pengembangan tetap di Windows. Perbandingan relatif (sebelum vs setelah, lingkungan identik) tetap valid untuk mengisolasi efek perubahan ini; angka absolut belum tentu sama persis di Linux produksi. p95 210 ms sedikit di atas target 200 ms — kemungkinan sisa kontensi ada di commit WAL Postgres pada Docker Desktop, bukan lagi akun fee. Detail lengkap dan analisis: `docs/evidence/k6-sharded.md` (baseline lama: `docs/evidence/k6.md`).

## Bug yang saya temukan sendiri lewat test

1. **`required` tidak menolak env kosong.** Test `t.Setenv("DATABASE_URL", "")` lolos validasi: tag `required` di `caarlos0/env` hanya memeriksa variabel *ada*, bukan *berisi*. Di produksi, `DATABASE_URL=` (salah ketik di manifest) akan lolos lalu gagal dengan pesan membingungkan. Perbaikan: `required,notEmpty`.
2. **Dokumen spesifikasi salah hitung T-04.** Tabel menulis "20 sukses, saldo akhir 0"; kodenya benar 19 (1.000.000 ÷ 51.000 = 19,6). Fee mengubah aritmetika. Dokumen diperbaiki sebelum test ditulis — kalau tidak, test yang salah akan lulus dengan percaya diri.
3. **Melepas `version` tidak memunculkan lost update** — berbeda dari dugaan dokumen. Eksperimen (`docs/evidence/eksperimen-kunci.md`) menunjukkan `UPDATE … SET balance = balance + $1` (relatif) + `CHECK (balance >= 0)` sudah menjaga uang; yang benar-benar rusak tanpa `FOR UPDATE ORDER BY id` adalah **deadlock** (99 dari 100 transfer silang). Pertahanan berlapis bekerja, dan tiap lapis ternyata menjaga hal yang berbeda.
4. **Rate limit membatalkan load test pertama.** 99,98 % request k6 ditolak 401/429 karena batas login 5/15 menit per IP dan transfer 20/menit per user. Pembatas bekerja; load test memakai `docker-compose.load.yml` yang menaikkannya.
5. **Bug idempotency ditemukan lewat review G-2 (model Fable, read-only), bukan test yang sudah ada.** `postIdempotent` yang kalah balapan idempotency membuang hasil `replay()` dan selalu mengembalikan `IDEMPOTENCY_IN_FLIGHT`, walau body request ternyata berbeda (seharusnya `IDEMPOTENCY_CONFLICT`). Klien dengan key sama + body beda yang kalah balapan akan retry selamanya menerima "coba lagi" yang tidak pernah menjadi benar. Ditemukan lewat review kode, ditutup dengan mengembalikan `err2` dari `replay()` apa adanya. Ini contoh nyata kenapa gerbang review terpisah (bukan sekadar test yang lulus) tetap perlu sebelum rilis.
6. **Kebocoran saldo lintas pengguna (audit keamanan penuh, model Opus, temuan High).** `GetTransaction` memfilter kepemilikan HEADER transaksi dengan benar (`EXISTS entries WHERE account_id = visibleTo`), tapi query ENTRIES di bawahnya tidak difilter sama sekali — dan bug yang sama ada di respons *langsung* setiap topup/transfer/withdraw. Akibatnya: penerima transfer melihat saldo pengirim setelah transaksi; siapa pun yang topup melihat saldo kumulatif kas platform (`SYSTEM_CASH`); transfer apa pun membocorkan saldo `SYSTEM_FEE_REVENUE`. Ini secara efektif membatalkan pembatasan admin-only pada `/internal/ledger/trial-balance`. Ditutup dengan `PostResult.ViewerAccountID`: setiap respons transaksi hanya menampilkan `balance_after` milik akun pemanggil sendiri (`null` untuk yang lain), sementara `amount_sen`/`fee_sen` tetap benar untuk semua pihak. Dibuktikan test `TestHTTP_B1_SaldoPihakLainTidakBocor`.

## Audit Keamanan Penuh (2026-09-16, model Opus, read-only)

Selain tiga gerbang review G-1/G-2/G-3, dijalankan audit keamanan menyeluruh (bukan diff, seluruh repo): 0 Critical, **1 High** (bug #6 di atas, sudah ditutup), 3 Medium, 9 Low, 4 Info. Pemeriksaan mekanis (sesi utama, sebelum audit kode): tidak ada secret *hardcode*, `.env` tidak pernah masuk git di seluruh riwayat, deep-scan seluruh objek git untuk pola token API/private key bersih total, `govulncheck` nol kerentanan yang dipanggil kode kita.

Medium yang ditutup selain bug #6: JWT secret kini minimal 32 byte **tanpa syarat** `APP_ENV` (sebelumnya validasi ketat hanya jalan bila `APP_ENV=production` disetel eksplisit); port dev Postgres & API diikat ke `127.0.0.1` (sebelumnya `0.0.0.0`, terjangkau LAN/Wi-Fi). Low yang ditutup: `reverseRequest` sekarang divalidasi (deskripsi > 255 karakter dulu jadi 500, sekarang 400); header keamanan HTTP dipasang di root router.

**Ditutup pada iterasi Fase 1.1 (2026-09-16, setelah audit):**
- Rate limit ditambah untuk `/auth/refresh` (per IP — tanpa auth, lookup DB tak terbatas) dan `topup`/`withdraw` (per actor, satu limiter `MoneyLimiter` bersama karena keduanya operasi tulis satu-akun yang setara).
- Rotasi refresh token kini mendeteksi pemakaian ulang: token yang sudah dirotasi tapi dipakai lagi mencabut **seluruh sesi** user tersebut (bukan hanya menolak permintaan itu) — dibuktikan `TestRefresh_ReuseTerdeteksi_CabutSemuaSesi`.
- `translate()` kini memetakan `chk_normal_balance`/`chk_owner` (tabel `accounts`) ke sentinel baru `ErrIntegrityViolation`, dipisah dari `ErrInvalidReversalLink` yang khusus untuk `chk_reversal_link` — namanya tidak lagi menyebut "reversal" untuk pelanggaran yang bukan soal reversal.

**Diterima sebagai keterbatasan Fase 1, tidak ditutup** (Low/Info, tidak eksploitatif untuk repo publik ini):
- `/metrics` tanpa autentikasi di port publik — membatasinya lewat kode akan merusak model *scraping* Prometheus standar; pembatasan yang benar ada di jaringan/reverse proxy saat deploy sungguhan.
- Rate limit login mengunci akun korban 15 menit jika penyerang tahu emailnya — trade-off standar rate-limit-per-akun, sudah dispesifikasikan sejak docs/03 §6.
- `/auth/register` membocorkan keberadaan email lewat `409 EMAIL_TAKEN` (orakel enumerasi), membatalkan sebagian usaha anti-enumerasi di `Login`.
- `/transactions/{id}/reverse` belum punya rate limit khusus (admin-only, risiko rendah).

## Fase 2 — Outbox relay, RabbitMQ, consumer idempoten (mulai Sesi 34)

Fase 1 hanya MENULIS ke `outbox_events` (docs/02 §2.9); Fase 2 menambahkan yang membaca dan mengirimkannya:

- **`cmd/worker`** — binary terpisah dari `cmd/api` (profil skala berbeda), menjalankan dua hal: **relay** dan **consumer**, dalam satu proses (cukup untuk volume Fase 2; pisahkan jadi dua binary bila salah satu perlu skala berbeda).
- **Relay** (`internal/platform/outbox`) memoles `outbox_events WHERE published_at IS NULL` setiap `RELAY_INTERVAL` (default 1s) dengan `FOR UPDATE SKIP LOCKED` — aman dijalankan lebih dari satu instance bersamaan, tidak ada yang mengirim ganda. Publish memakai **publisher confirm** (menunggu ack broker, bukan sekadar "berhasil ditulis ke socket").
- **Consumer** (`internal/consumer.AuditLog`) mencatat setiap event ke tabel `processed_events` (UNIQUE `event_id`) — **idempoten**: RabbitMQ hanya menjamin *at-least-once*, jadi event yang sama BISA datang dua kali; redelivery tidak memproses ulang.
- **Topologi** (`internal/platform/broker`): exchange topic `ledger.events`, queue `ledger.audit-log` dengan DLQ `ledger.audit-log.dlq` (pesan cacat permanen di-nack tanpa requeue, mendarat di DLQ untuk diperiksa, bukan hilang atau diulang selamanya).
- `event_id` (kolom `outbox_events`, sudah ada sejak Fase 1) dibawa lewat properti standar AMQP `MessageId` — satu-satunya kunci deduplikasi yang consumer punya.

Dibuktikan `TestOutbox_RelayDanConsumer_EndToEnd` (RabbitMQ sungguhan via testcontainers, `-race`) DAN lewat `docker compose up` sungguhan (`docs/evidence/fase2-outbox.md`): topup nyata → relay mengirim dalam ~1 detik → consumer mencatat → *graceful shutdown* `SIGTERM` terbukti tidak mematikan di tengah pemrosesan.

```bash
docker compose up -d --build   # api + worker + postgres + rabbitmq, satu perintah
# RabbitMQ management UI: http://localhost:15673 (user/pass: nusa / nusa_dev_only)
```

**Role database terbatas untuk `entries`** (migration `000011`, lapis kedua BR-04 docs/02 §2.6): `cmd/api` dan `cmd/worker` connect sebagai `nusaledger_app`, peran yang HAK AKSESNYA SENDIRI (bukan hanya trigger) tidak mengizinkan UPDATE/DELETE/TRUNCATE pada `entries` atau DELETE pada `transactions` — migrasi tetap jalan sebagai superuser lewat `MIGRATION_DATABASE_URL` terpisah. Dua ancaman berbeda ditutup dua lapis berbeda: trigger `forbid_mutation` menahan bug kode, REVOKE di level role menahan siapa pun yang connect dengan kredensial aplikasi dan mengetik SQL manual. Dibuktikan `TestAppRole_TidakBisaMengubahLedger` — menyambung sungguhan sebagai `nusaledger_app` (bukan superuser test), mendapat SQLSTATE `42501` (insufficient_privilege), berbeda dari trigger (`23514`).

**Rate limit Redis opsional** (`internal/platform/ratelimit/redis.go`, Sesi 36): `RedisLimiter` mengimplementasikan kontrak `Allow(key string) bool` yang sama dengan limiter in-memory Fase 1, jadi router tidak berubah — hanya `REDIS_URL` (kosong = in-memory, diisi = Redis) yang menentukan backend di `cmd/api/main.go`. Hitungan `INCR`+`PEXPIRE` dijalankan sebagai satu skrip Lua atomik (dua panggilan terpisah berisiko key tanpa TTL kalau proses mati di antaranya). Fail-closed saat Redis tidak terjangkau (docs/03 §6), timeout 500ms sendiri (bukan mewarisi context request, supaya Redis lambat tidak ikut memperlambat semua request). Dibuktikan `TestRedisLimiter_DibagiLintasInstance` — dua `*RedisLimiter` Go yang berbeda, terhubung ke Redis yang sama, berbagi satu kuota (skenario nyata 2 instance `cmd/api`) — dan lewat `docker compose up` sungguhan: percobaan login ke-6 (limit 5) mendapat 429, `redis-cli KEYS "ratelimit:*"` menunjukkan key tersimpan sungguhan di Redis.

## Yang akan diperbaiki berikutnya

- ~~Baris panas fee~~ **selesai** (Sesi 33) — lihat §Load test di atas. p95 masih 10 ms di atas target; kandidat penyebab sisa: fsync WAL Postgres di Docker Desktop, bukan lagi akun fee.
- ~~Role DB terbatas untuk `entries`~~ **selesai** (Sesi 35) — lihat §Fase 2 di atas.
- ~~Rate limit Redis~~ **selesai** (Sesi 36) — lihat §Fase 2 di atas.
- **Fase 2, satu-satunya sisa item**: batasi `/metrics` di reverse proxy — dokumentasi/infra (contoh konfigurasi nginx/Caddy), bukan kode; pembatasan jaringan bukan tanggung jawab aplikasi.
- Migrasi produksi sebagai langkah deploy terpisah (`RUN_MIGRATIONS=false`), secret dari secret manager.
- Pengukuran ulang di Linux native (bukan Docker Desktop Windows) untuk menutup selisih p95 10 ms yang tersisa.

## Dokumen

| Dokumen | Isi |
|---|---|
| `docs/00`–`03` | Glosarium, PRD, skema DB, spesifikasi teknis (sudah direvisi) |
| `docs/06-PLAN-EKSEKUSI-AI.md` | Plan 30 sesi, keputusan desain yang dikunci (F-01…F-06, K-01…K-09) |
| `docs/JURNAL-BELAJAR.md` | **Catatan tiap langkah: apa, kenapa, contoh, bukti, jebakan** — untuk diulang manual |
| `docs/evidence/` | Transkrip trigger, EXPLAIN, eksperimen kunci, smoke compose, k6 |
| `docs/openapi.yaml` | Kontrak API lengkap |
| `HANDOVER.md` | Posisi terakhir & langkah berikutnya |

## Struktur

```
cmd/api            entrypoint, DI manual, graceful shutdown, job drift, pprof loopback
cmd/worker         Fase 2: entrypoint relay + consumer, binary terpisah dari api
internal/domain    Money, Account, Entry, Transaction (+builder, Validate), error sentinel
internal/service   Ledger, Auth, ports.go (interface konsumen), cursor
internal/repository/postgres  LedgerRepo.Post (atomik), Account/User/Token/Idempotency repo, translate()
internal/transport/http       router (chi), middleware, handler, DTO, mapError
internal/platform  config, logger (redaksi), metrics, ratelimit, token (JWT), password (argon2id), dbmigrate,
                   outbox (relay Fase 2), broker (topologi RabbitMQ Fase 2)
internal/consumer  Fase 2: AuditLog — consumer idempoten (tabel processed_events)
migrations/        10 pasang up/down, ter-embed
test/integration   testcontainers: schema, repo, konkurensi, E2E HTTP, outbox+RabbitMQ
test/load          k6
```
