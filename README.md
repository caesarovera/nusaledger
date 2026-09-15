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
| Rate limit in-memory, fail-closed | Redis | Fase 1 satu instance; interface memungkinkan Redis di Fase 2 |
| Tanpa `middleware.RealIP` | Percaya `X-Forwarded-For` | Header itu bisa dipalsukan siapa pun tanpa proxy tepercaya (GHSA-3fxj-6jh8-hvhx) |

## Hasil Pengujian

| Kategori | Hasil | Bukti |
|---|---|---|
| Unit (`-race`) | domain **99,1 %**, service 84,2 %, config 90 %, ratelimit 100 % | `make test` |
| Integration (testcontainers, Postgres 17) | 20 test hijau: T-02, T-03, T-10, T-10b, T-11, K-05, repo user/token | `make test-int` |
| **T-04** 100 goroutine dari Rp 1.000.000 @ Rp 50.000 + fee | **tepat 19 sukses**, sisa Rp 31.000, ×5 | `test/integration/concurrency_test.go` |
| **T-05** 10 goroutine key sama | tepat 1 transaksi | idem |
| **T-07** 50× A→B ∥ 50× B→A | 100 sukses, **0 deadlock** | idem |
| **T-08/T-09** 1.000 transaksi acak | trial balance 0, drift 0 | idem |
| **T-13** graceful shutdown | request 1,5 s selesai saat SIGTERM di 0,3 s | `internal/app/server_test.go` |
| E2E HTTP | idempotency replay **byte-identik**, 400/401/403/404/409/413/422/429 | `test/integration/http_test.go` |
| Trigger & constraint (psql, tanpa Go) | 14 skenario HARUS GAGAL semua gagal | `docs/evidence/trigger-test.md` |
| Cursor pagination @200.000 entries | `Index Scan` 0,49 ms | `docs/evidence/explain-cursor.md` |
| Lint / vuln | `golangci-lint` 0 issue, `govulncheck` bersih | `make lint` |
| Image | **18,6 MB** distroless nonroot; `docker compose up` → ready 4 s | `docs/evidence/smoke-compose.md` |

### Load test (k6, 100 VU, 3 menit) — jujur: SLO latensi belum tercapai

| Metrik | Target | Hasil |
|---|---|---|
| Ketepatan saldo setelah 33.340 transfer | 100 % | ✅ trial balance 0, drift 0, fee = 33.340 × Rp 1.000 |
| Error rate | < 1 % | ✅ 0,00 % |
| Throughput | ≥ 200/s | ❌ **177/s** |
| p95 / p99 | < 200 / < 500 ms | ❌ **515 / 568 ms** |

Penyebab utama yang teridentifikasi: **setiap transfer mengunci akun `SYSTEM_FEE_REVENUE` yang sama**, sehingga seluruh sistem terserialisasi pada satu baris panas; ditambah Postgres di Docker Desktop (Windows) dengan klien, API, dan DB berbagi satu laptop. Detail dan rencana perbaikan: `docs/evidence/k6.md`. Ini dicatat apa adanya karena *sistem yang cepat tetapi salah lebih buruk daripada yang lambat tetapi benar* — dan yang benar sudah terbukti.

## Bug yang saya temukan sendiri lewat test

1. **`required` tidak menolak env kosong.** Test `t.Setenv("DATABASE_URL", "")` lolos validasi: tag `required` di `caarlos0/env` hanya memeriksa variabel *ada*, bukan *berisi*. Di produksi, `DATABASE_URL=` (salah ketik di manifest) akan lolos lalu gagal dengan pesan membingungkan. Perbaikan: `required,notEmpty`.
2. **Dokumen spesifikasi salah hitung T-04.** Tabel menulis "20 sukses, saldo akhir 0"; kodenya benar 19 (1.000.000 ÷ 51.000 = 19,6). Fee mengubah aritmetika. Dokumen diperbaiki sebelum test ditulis — kalau tidak, test yang salah akan lulus dengan percaya diri.
3. **Melepas `version` tidak memunculkan lost update** — berbeda dari dugaan dokumen. Eksperimen (`docs/evidence/eksperimen-kunci.md`) menunjukkan `UPDATE … SET balance = balance + $1` (relatif) + `CHECK (balance >= 0)` sudah menjaga uang; yang benar-benar rusak tanpa `FOR UPDATE ORDER BY id` adalah **deadlock** (99 dari 100 transfer silang). Pertahanan berlapis bekerja, dan tiap lapis ternyata menjaga hal yang berbeda.
4. **Rate limit membatalkan load test pertama.** 99,98 % request k6 ditolak 401/429 karena batas login 5/15 menit per IP dan transfer 20/menit per user. Pembatas bekerja; load test memakai `docker-compose.load.yml` yang menaikkannya.

## Yang akan diperbaiki berikutnya

- **Baris panas fee** → sharding sub-akun fee atau akumulasi per periode, lalu ukur ulang SLO di Linux native.
- **Fase 2**: outbox relay → RabbitMQ (tabel `outbox_events` sudah ditulis sejak Fase 1), worker `cmd/worker`, rate limit Redis, role DB aplikasi tanpa hak `UPDATE/DELETE/TRUNCATE` pada `entries`.
- Migrasi produksi sebagai langkah deploy terpisah (`RUN_MIGRATIONS=false`), secret dari secret manager.

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
internal/domain    Money, Account, Entry, Transaction (+builder, Validate), error sentinel
internal/service   Ledger, Auth, ports.go (interface konsumen), cursor
internal/repository/postgres  LedgerRepo.Post (atomik), Account/User/Token/Idempotency repo, translate()
internal/transport/http       router (chi), middleware, handler, DTO, mapError
internal/platform  config, logger (redaksi), metrics, ratelimit, token (JWT), password (argon2id), dbmigrate
migrations/        8 pasang up/down, ter-embed
test/integration   testcontainers: schema, repo, konkurensi, E2E HTTP
test/load          k6
```
