# Plan Eksekusi NusaLedger Fase 1 — Tim AI, Model, Skill, Agent

**Disusun:** 2026-09-16 · **Sudut pandang:** 🧑‍💻 Senior Dev + 🏛️ Senior Architect + 🗄️ Senior DBA + 🧪 Senior QA + ⚙️ Senior DevOps
**Dasar:** analisis penuh dokumen 00–05, GO-BACKEND-HANDBOOK, MESSAGE-QUEUE-GO, dan kondisi mesin saat ini.

> Dokumen ini **tidak mengganti** dokumen 01–05. Ia menjawab tiga hal yang belum dijawab dokumen itu: (a) apa yang harus diputuskan/diperbaiki sebelum mulai, (b) sesi kerja mana memakai model apa, dan (c) urutan eksekusi konkret dari mesin kosong sampai Definition of Done tercentang.

---

## 0. Ringkasan Eksekutif

| Hal | Kesimpulan |
|---|---|
| Kualitas dokumen | Sangat matang. PRD, skema, dan spesifikasi konsisten satu sama lain kecuali 6 titik kecil (§2). Tidak perlu redesign. |
| Kesiapan mesin | **Belum siap.** Go, make, migrate, golangci-lint, govulncheck, k6, dlv belum terpasang. Docker daemon jalan (v29.7), psql 16.2 client ada, winget ada. Repo belum `git init`. |
| Strategi AI | Pola dokumen 04 dipertahankan: **memutuskan → mengerjakan → mencari** dipetakan ke **Opus 5 → Sonnet 5 → Haiku 4.5**, plus **Fable 5.1** hanya untuk 3 gerbang review tertinggi. Tambah 1 agent (`senior-devops`) dan 3 skill baru. |
| Jumlah sesi | 30 sesi kerja dalam 4 minggu, tiap sesi satu peran, satu tujuan, satu `/clear`. |
| Risiko utama | Bukan teknis, tapi scope creep dan kehabisan tenaga. Plan ini memaksa repo "bisa dipamerkan" di akhir setiap minggu. |

---

## 1. Kondisi Lingkungan & Langkah 0 (revisi untuk Windows)

Hasil pemeriksaan mesin ini (Windows 11, PowerShell):

| Alat | Status | Tindakan |
|---|---|---|
| Docker daemon | ✅ v29.7.2 jalan | — |
| psql | ✅ 16.2 (client saja; server dipakai lewat Docker `postgres:17`) | Cukup untuk verifikasi manual |
| winget | ✅ | Dipakai untuk semua instalasi di bawah |
| Go | ❌ | `winget install GoLang.Go` (target Go 1.27) |
| GNU make | ❌ | `winget install ezwinports.make` |
| golang-migrate | ❌ | `go install github.com/golang-migrate/migrate/v4/cmd/migrate@latest` (setelah Go ada) |
| golangci-lint | ❌ | `winget install GolangCI.golangci-lint` |
| govulncheck | ❌ | `go install golang.org/x/vuln/cmd/govulncheck@latest` |
| k6 | ❌ | `winget install k6.k6` |
| delve | ❌ | `go install github.com/go-delve/delve/cmd/dlv@latest` |
| `ANTHROPIC_API_KEY` | ✅ kosong | Sudah benar — Claude Code memakai kuota langganan, bukan kredit API |

**Catatan Windows yang memengaruhi dokumen 05:**
- `$(DATABASE_URL)` di Makefile membaca env var. Di PowerShell set dengan `$env:DATABASE_URL = "..."`, bukan `export`.
- Skrip `head -c 50000000 /dev/zero | curl ...` (uji body raksasa) jalankan di Git Bash, bukan PowerShell.
- `kill -TERM $(pgrep ...)` untuk uji graceful shutdown tidak ada di Windows → T-13 diuji **di dalam Go** lewat `httptest` + pengiriman sinyal ke `signal.NotifyContext` (lihat §2 keputusan K-06). Ini lebih baik karena bisa masuk CI.
- Testcontainers butuh Docker Desktop dengan API terekspos; sudah terpenuhi.

---

## 2. Temuan Analisis & Keputusan Tim

Enam ketidaksesuaian/celah ditemukan saat membaca silang dokumen. Masing-masing diberi keputusan agar tidak menjadi perdebatan di tengah implementasi.

### 2.1 Ketidaksesuaian antar dokumen (perbaiki di dokumen sumber)

| # | Temuan | Lokasi | Keputusan |
|---|---|---|---|
| F-01 | T-04: tabel §7.2 menulis "tepat 20 sukses, saldo akhir 0", kode di bawahnya `expected = 19`. Hitungan: 1.000.000 / 51.000 = 19,6 → **19** sukses, sisa saldo **Rp 31.000**, bukan 0. | 03 §7.2 | Pakai **19** dan sisa 31.000. Perbaiki tabel. Ini contoh bagus bahwa fee mengubah aritmetika — layak masuk README "bug yang saya temukan". |
| F-02 | Versi Go: dok 04 dan handbook menulis 1.25, dok 05 menulis "1.24+". | 04 §2, 05 L0 | **Go 1.27** di `go.mod`, Dockerfile, CI. |
| F-03 | `ErrIdempotencyInFlight` dikembalikan repository (03 §5) tapi tidak ada di katalog error API (03 §4.3). | 03 §4.3 | Tambah kode **`IDEMPOTENCY_IN_FLIGHT` → 409** dengan header `Retry-After: 1`. Klien boleh retry dengan key sama. |
| F-04 | Rate limit "fail-closed" (03 §6) butuh penyimpanan; Redis **tidak** ada di scope Fase 1. | 03 §6, 01 §1.3 | Rate limiter **in-memory** di balik interface `service.RateLimiter`. Cukup untuk 1 instance. Catat di README sebagai keterbatasan yang sadar; Redis masuk Fase 2. |
| F-05 | "Job verifikasi saldo" + metrik `ledger_balance_drift_total` (02 §3.5, 03 §8) tidak punya rumah — Fase 1 hanya punya satu binary `cmd/api`. | 02 §3.5 | Goroutine ticker di `cmd/api` (interval `DRIFT_CHECK_INTERVAL`, default 60s), dimiliki oleh `main` dan dihentikan lewat context saat shutdown. Dipindah ke `cmd/worker` di Fase 2. |
| F-06 | Endpoint `/api/v1/slow` untuk uji graceful shutdown (05 L13) tidak ada di kontrak API. | 05 L13 | **Tidak dibuat sebagai endpoint publik.** T-13 diuji in-process: handler lambat didaftarkan hanya di test. |

### 2.2 Celah desain yang perlu keputusan (belum dibahas dokumen)

| # | Celah | Analisis | Keputusan |
|---|---|---|---|
| K-01 | `idempotency_keys.key` adalah PK global. Dua user berbeda yang kebetulan (atau sengaja) memakai key sama akan saling mengganggu: user B dapat 409/in-flight karena key milik A. | 🗄️ Ini celah kecil tapi nyata; perbaikannya murah selagi tabel kosong. | **PK komposit `(user_id, key)`** dan `ON CONFLICT (user_id, key)`. Deviasi dari dok 02 §2.7 — dicatat di HANDOVER dan README. |
| K-02 | Kode acuan memakai **FOR UPDATE + optimistic lock `version`** sekaligus. Kalau satu akun muncul dua kali dalam satu transaksi, UPDATE kedua gagal karena `version` sudah naik → `ErrConcurrentModification` palsu. | 🏛️ Operasi Fase 1 tidak pernah mengulang akun, tapi domain harus menjaminnya, bukan berharap. | Tambah aturan domain **"satu akun maksimal satu entry per transaksi"** (`ErrDuplicateAccount`) di `Validate()` + test. Kedua kunci tetap dipertahankan (belt-and-braces; ceritanya bagus di wawancara). |
| K-03 | Reversal transfer bisa membuat dompet penerima negatif kalau ia sudah membelanjakan uangnya → `chk_wallet_non_negative` menolak. | 🗄️ Ini perilaku **benar** (ledger tidak boleh menciptakan uang), tapi harus dipetakan ke error yang jelas. | Reversal mengembalikan `INSUFFICIENT_BALANCE` (422) dengan pesan "saldo penerima tidak mencukupi untuk pembatalan". Uji sebagai T-10b. |
| K-04 | Format cursor pagination belum diputuskan (HANDOVER contoh dok 04). | 🧑‍💻 | **base64url dari `id` int64** (opaque bagi klien). Response: `{"data":[...],"next_cursor":"..."}`; `next_cursor` null bila habis. |
| K-05 | Reversal mengubah `transactions.status` transaksi asli → tabel `transactions` hanya dilindungi dari DELETE. Tapi UPDATE ke kolom lain (mis. `txn_type`) tetap lolos trigger. | 🗄️ | Trigger `forbid_mutation` diperluas: pada `transactions`, UPDATE **hanya boleh** mengubah `status` (`IF OLD.* IS DISTINCT FROM NEW.* kecuali status → RAISE`). Ditulis di migration 000005. |
| K-06 | Cara membuktikan T-13 (graceful shutdown) yang bisa masuk CI dan jalan di Windows. | ⚙️ | Test start server di goroutine, kirim request ke handler yang tidur 3s (didaftarkan hanya di test), lalu panggil `srv.Shutdown(ctx)` — assert request selesai 200 dan Shutdown kembali tanpa error. Dokumen 05 versi `kill -TERM` tetap dijalankan manual sekali untuk README. |
| K-07 | Pilihan pustaka tidak dikunci; agent `senior-go-dev` dilarang menambah dependensi tanpa izin → akan bertanya berulang. | 🧑‍💻 | Daftar dependensi **disetujui di muka** (§2.3). Di luar daftar itu, agent wajib bertanya. |
| K-08 | Dokumen 00–05 harus bisa dibaca Claude dari dalam repo (`docs/01-PRD...` dirujuk CLAUDE.md), tetapi dua handbook (150 KB) adalah materi belajar pribadi. | 🏛️ hemat token | Salin **00–05** ke `nusaledger/docs/`. Handbook **tidak** masuk repo; kalau perlu dirujuk, tempelkan potongan §-nya saja. |
| K-09 | Router: handbook merekomendasikan chi; spesifikasi tidak menyebut. | 🧑‍💻 | **chi v5** — ringan, idiomatik `net/http`, middleware ekosistem matang, dan sering muncul di codebase perusahaan Indonesia. |

### 2.3 Dependensi yang disetujui di muka

```
github.com/jackc/pgx/v5                       # driver + pool
github.com/golang-migrate/migrate/v4          # migration (+ driver postgres, source file)
github.com/go-chi/chi/v5                      # router
github.com/golang-jwt/jwt/v5                  # JWT HS256, validasi alg eksplisit
golang.org/x/crypto                           # argon2id
github.com/caarlos0/env/v11                   # config dari env
github.com/google/uuid
github.com/prometheus/client_golang           # /metrics
github.com/testcontainers/testcontainers-go   # + modules/postgres, modules/rabbitmq, modules/redis (test saja)
github.com/rabbitmq/amqp091-go                # Fase 2: outbox relay & consumer (cmd/worker)
github.com/redis/go-redis/v9                  # Fase 2: rate limiter multi-instance (cmd/api, opsional via REDIS_URL)
log/slog                                      # stdlib, bukan zap/zerolog
```

Tidak disetujui: framework DI, ORM, validator berbasis reflection (validasi ditulis manual di DTO — lebih mudah dijelaskan di wawancara), zap/zerolog.

---

## 3. Tim AI: Model, Agent, Skill

### 3.1 Peta model (Claude 5 family, per 2026-09-16)

| Model | Alias frontmatter | Harga API (in/out per 1M) | Peran di proyek ini |
|---|---|---|---|
| **Claude Fable 5.1** | `fable` (Agent tool) / sesi utama | $10 / $50 | **3 gerbang review** saja (G-1, G-2, G-3 di §4). Terlalu mahal untuk pekerjaan rutin. |
| **Claude Opus 5** | `opus` | $5 / $25 | **Memutuskan**: arsitektur, skema, review konkurensi, debugging buntu. |
| **Claude Sonnet 5** | `sonnet` | $2 / $10 | **Mengerjakan**: implementasi, test, DevOps, dokumentasi. |
| **Claude Haiku 4.5** | `haiku` | $1 / $5 | **Mencari**: explorer, perbaikan lint/format mekanis. |

Catatan: pada plan Pro, biaya di atas tercermin sebagai konsumsi kuota, bukan tagihan. Rasio harganya tetap jadi panduan: satu review Fable ≈ 5 sesi Sonnet. Verifikasi routing lewat dashboard usage setelah minggu pertama (dok 04 §3.6).

**Aturan effort:**
- `high`/`xhigh` hanya untuk: desain skema, `PostTransaction`, T-04/T-05/T-07, debugging buntu, tiga gerbang review.
- `medium` untuk implementasi service/repository/handler dan penulisan test biasa.
- `low` untuk explorer, lint, dokumentasi, boilerplate DTO.
- Effort sesi utama diatur lewat `/model`; subagent mengikuti `model` di frontmatter-nya.

### 3.2 Agent — lima dari dokumen 04, satu tambahan

Format dan isi empat agent dokumen 04 §3 (`senior-architect`, `senior-dba`, `senior-go-dev`, `senior-qa`, `explorer`) **dipakai apa adanya** dengan tiga penyesuaian:

1. `senior-go-dev` diberi daftar dependensi §2.3 di system prompt-nya, supaya tidak bertanya berulang.
2. `senior-qa` diberi tabel T-01…T-14 + T-10b ringkas, supaya tahu target tanpa membaca dok 03 utuh.
3. `senior-dba` diberi hak `Bash` yang dipakai untuk `docker compose exec postgres psql -c "EXPLAIN (ANALYZE, BUFFERS) ..."`.

**Agent baru — `.claude/agents/senior-devops.md`** (peran yang Anda minta tapi belum ada di dok 04):

```markdown
---
name: senior-devops
description: Menangani Dockerfile, docker-compose, Makefile, CI GitHub Actions, skrip k6, wiring metrik/health/pprof, dan graceful shutdown. Gunakan untuk semua pekerjaan infrastruktur & operasional, bukan logika bisnis.
tools: Read, Write, Edit, Bash, Grep, Glob
model: sonnet
---

Anda Senior DevOps/SRE untuk layanan Go yang menangani uang.

Prioritas Anda, berurutan:
1. Reproducible — `docker compose up` dari nol harus jalan tanpa langkah manual.
2. Aman — image distroless nonroot, tidak ada secret di image/repo, pprof hanya di 127.0.0.1.
3. Terukur — /healthz tanpa DB, /readyz dengan DB, /metrics Prometheus, log JSON dengan request_id.
4. Cepat — build cache dimanfaatkan, `make test` < 10 detik, image < 20 MB.

Yang selalu Anda periksa:
- Dockerfile multi-stage, CGO_ENABLED=0, -trimpath, -ldflags "-s -w", distroless/static nonroot.
- compose: healthcheck postgres, depends_on condition service_healthy, setelan
  idle_in_transaction_session_timeout & log_min_duration_statement.
- CI: lint → unit → race → integration (Docker) → build image, gagal cepat.
- Graceful shutdown: signal.NotifyContext → srv.Shutdown(ctx timeout) → pool.Close().
- k6: threshold p95<200ms p99<500ms, dan SETELAH load test jalankan verifikasi trial balance.

Jangan mengubah kode di internal/domain, internal/service, internal/repository.
Kalau butuh perubahan di sana, laporkan ke sesi utama.
Setelah mengubah Dockerfile/compose, jalankan `docker compose config` dan `docker build`.
Laporkan ringkas: file yang diubah + satu kalimat alasan per file.
```

**Kapan memakai agent bawaan Claude Code, bukan agent kustom:**

| Kebutuhan | Pakai | Bukan |
|---|---|---|
| Menyusun rencana implementasi satu langkah besar | Plan mode sesi utama (`EnterPlanMode`) | agent `Plan` — kontek proyek sudah ada di CLAUDE.md |
| Mencari lokasi kode | `explorer` (Haiku) | agent `Explore` bawaan (model lebih mahal, output panjang) |
| Review diff sebelum commit | skill `/code-review` (medium) | agent kustom |
| Audit keamanan sebelum tag `v1.0` | skill `/security-review` | menulis agent security sendiri |
| Merapikan kode setelah fitur hijau | skill `/simplify` | — |
| Menjalankan app untuk smoke test | skill `/run` | — |

### 3.3 Skill — tiga dari dokumen 04, tiga tambahan

Tiga skill dokumen 04 §4 dipakai apa adanya: `ledger-invariants`, `go-conventions`, `test-strategy`. Tiga skill baru menutup area yang agent DBA, handler, dan DevOps butuhkan berulang:

**`.claude/skills/postgres-patterns/SKILL.md`** (dipakai `senior-dba`, `senior-go-dev` saat menyentuh repository)

```markdown
---
name: postgres-patterns
description: Pola PostgreSQL proyek ini — penguncian terurut, atomic update, cursor pagination, trigger deferred, aturan migration, dan checklist EXPLAIN. Gunakan saat menulis SQL, migration, atau kode repository.
---
# Pola PostgreSQL NusaLedger
## Kunci akun (anti-deadlock)
SELECT ... FROM accounts WHERE id = ANY($1) ORDER BY id FOR UPDATE
## Update saldo (atomik + optimistic)
UPDATE accounts SET balance = balance + $1, version = version + 1, updated_at = now()
WHERE id = $2 AND version = $3 AND status = 'ACTIVE' RETURNING balance, version
RowsAffected = 0 → ErrConcurrentModification. CHECK chk_wallet_non_negative → ErrInsufficientBalance.
## Cursor pagination
WHERE account_id = $1 AND ($2::BIGINT IS NULL OR id < $2) ORDER BY id DESC LIMIT $3
Cursor = base64url(id). Wajib Index Scan Backward pada idx_entries_account_id_desc.
## Trigger
trg_entries_balanced: CONSTRAINT TRIGGER DEFERRABLE INITIALLY DEFERRED → error muncul di Commit(), bukan Exec().
forbid_mutation: entries tolak UPDATE/DELETE; transactions tolak DELETE dan UPDATE selain kolom status.
## Migration
Satu file satu tujuan. Setiap up punya down yang diuji `migrate down -all && up`.
Tidak ada migration yang mengubah data uang.
## Terjemahan error pgx
pgconn.PgError.Code 23514 (check_violation) + ConstraintName → error domain.
Code 23505 (unique_violation) pada uq_reversal → ErrAlreadyReversed.
## Checklist EXPLAIN sebelum commit query baru
- [ ] EXPLAIN (ANALYZE, BUFFERS) dijalankan
- [ ] Tidak ada Seq Scan pada entries/transactions/idempotency_keys
- [ ] Index yang dipakai sesuai yang direncanakan di docs/02 §2
```

**`.claude/skills/api-contract/SKILL.md`** (dipakai `senior-go-dev` saat menulis transport/http)

```markdown
---
name: api-contract
description: Kontrak HTTP proyek ini — envelope respons, katalog kode error → status, aturan DTO, header Idempotency-Key, dan middleware wajib. Gunakan saat menulis atau meninjau handler, DTO, middleware, atau openapi.yaml.
---
# Kontrak API NusaLedger  (base path /api/v1)
## Envelope
Sukses: {"data": ...}   Error: {"error": {"code","message","fields","request_id"}}
## Kode error → HTTP  (sumber: docs/03 §4.3 + F-03)
VALIDATION_ERROR 400 · IDEMPOTENCY_KEY_REQUIRED 400 · UNAUTHENTICATED 401 · FORBIDDEN 403
ACCOUNT_NOT_FOUND 404 · TRANSACTION_NOT_FOUND 404
IDEMPOTENCY_CONFLICT 409 · IDEMPOTENCY_IN_FLIGHT 409 (+Retry-After: 1) · ALREADY_REVERSED 409 · CONCURRENT_MODIFICATION 409
INSUFFICIENT_BALANCE 422 · ACCOUNT_NOT_ACTIVE 422 · SELF_TRANSFER 422 · AMOUNT_OUT_OF_RANGE 422
RATE_LIMITED 429 · INTERNAL_ERROR 500
Pemetaan ada di SATU tempat: transport/http/response.go (errors.Is → kode).
## DTO
- Nominal selalu bernama *_sen (int64). ID publik selalu UUID (public_id), tidak pernah id internal.
- json.Decoder dengan DisallowUnknownFields; body dibatasi http.MaxBytesReader 1 MB.
- DTO tidak mengimpor domain sebagai field publik; konversi eksplisit.
## Idempotency-Key
Wajib di POST topup/transfer/withdraw/reverse. Format bebas, maks 128 char.
request_hash = SHA-256(user_id + endpoint + body kanonik).
## Urutan middleware
Recover → RequestID → Logger → MaxBytes → (Auth) → (RateLimit) → handler
## Timeout server
ReadHeader 5s · Read 15s · Write 30s · Idle 60s
```

**`.claude/skills/ops-runbook/SKILL.md`** (dipakai `senior-devops`)

```markdown
---
name: ops-runbook
description: Cara membangun, menjalankan, mengukur, dan memverifikasi NusaLedger — perintah make, compose, k6, metrik wajib, dan prosedur setelah load test. Gunakan untuk pekerjaan Docker, CI, load test, observability.
---
# Runbook Operasional
## Perintah
make test / test-int / race / lint / migrate-up / up / run
## Metrik wajib (docs/03 §8)
http_request_duration_seconds{method,route,status} · http_requests_total
db_pool_conns_in_use · db_pool_conns_max
ledger_transactions_total{type,status} · ledger_balance_drift_total (HARUS 0) · ledger_trial_balance_difference (HARUS 0)
auth_login_failures_total{reason}
## Health
/healthz: proses hidup, TANPA DB. /readyz: pool.Ping dengan timeout 2s.
## Load test
k6 run test/load/transfer.js  → lalu WAJIB: curl /internal/ledger/trial-balance → difference == 0
## Image
docker build -t nusaledger . && docker images nusaledger  → < 20 MB
## Log
slog JSON; field wajib: request_id, user_id, transaction_id, duration_ms. Redaksi: password, token, authorization.
```

### 3.4 `CLAUDE.md` dan `settings.json`

`CLAUDE.md` dokumen 04 §2 dipakai utuh, ditambah **tiga baris**:

```markdown
## Keputusan yang mengunci (lihat docs/06 §2)
- idempotency_keys PK = (user_id, key). Satu akun maks satu entry per transaksi.
- Cursor = base64url(id). ErrIdempotencyInFlight → 409 IDEMPOTENCY_IN_FLIGHT.
- Dependensi yang boleh: lihat docs/06 §2.3. Di luar itu, tanya dulu.
```

`settings.json` dokumen 04 §5 dipakai utuh, ditambah izin yang dibutuhkan di Windows dan DevOps:

```json
"Bash(docker build:*)", "Bash(docker images:*)", "Bash(k6 run:*)",
"Bash(golangci-lint:*)", "Bash(govulncheck:*)", "Bash(migrate:*)",
"Bash(gh pr:*)"
```

`git push` tetap **ditolak** — push adalah keputusan sadar manusia.

---

## 4. Tiga Gerbang Review (Fable 5.1)

Model termahal dipakai hanya di tiga titik yang kesalahannya paling mahal untuk dibongkar belakangan. Setiap gerbang bersifat **read-only** dan menghasilkan daftar temuan berprioritas yang dicatat di `HANDOVER.md`.

| Gerbang | Kapan | Input | Pertanyaan yang harus dijawab |
|---|---|---|---|
| **G-1 Skema & penguncian** | Akhir minggu 1, sebelum kode repository ditulis | `migrations/*`, docs/02 §2–3, keputusan K-01/K-02/K-05 | Adakah jalur yang memungkinkan uang tercipta/hilang lewat DB saja? Adakah constraint yang bisa dilewati? Apakah urutan kunci menutup semua deadlock? |
| **G-2 PostTransaction & konkurensi** | Minggu 3, setelah T-04/T-05/T-07 hijau | `ledger_repo.go`, `ledger_service.go`, `idempotency.go`, ketiga test | Apakah ada jendela waktu antara cek dan tulis? Apa yang terjadi kalau proses mati di setiap baris? Apakah test benar-benar membuktikan, atau hanya kebetulan lulus? |
| **G-3 Rilis** | Akhir minggu 4, sebelum tag `v1.0.0` | Seluruh diff sejak G-2, README, hasil k6, `/security-review` | Apakah semua DoD PRD §6 terbukti dengan artefak, bukan klaim? Apa yang akan ditanyakan pewawancara dan apakah jawabannya ada di README? |

Cara memanggil: dari sesi utama, `Pakai model fable, effort tinggi, read-only: tinjau ... Jangan ubah file. Keluarkan temuan berurutan dari yang paling berbahaya.` Lalu `/clear`.

---

## 5. Rencana Eksekusi 4 Minggu — 30 Sesi

Kolom **Peran** = agent/skill yang dipanggil dari sesi utama. Kolom **Bukti** = kondisi yang harus tercapai sebelum `/clear`. Setiap sesi diakhiri: update `HANDOVER.md` → `git commit`.

### Minggu 1 — Fondasi: alat, skema, domain (target: T-01, T-14 hijau; `make test` < 5 s)

| # | Sesi | Peran (model, effort) | Masukan | Keluaran | Bukti |
|---|---|---|---|---|---|
| 1 | Pasang alat (manual + Claude bantu) | sesi utama (Sonnet, low) | §1 dokumen ini | Semua alat §1 terpasang | `go version`, `make --version`, `migrate -version`, `golangci-lint version`, `k6 version` keluar tanpa error |
| 2 | Bootstrap repo & setup Claude | `senior-devops` (Sonnet, low) | docs 04 §2, §5; 05 L1; §3 dokumen ini | `go.mod`, struktur direktori, Makefile, `.claude/{agents,skills,settings.json}`, `CLAUDE.md`, `HANDOVER.md`, `docs/00–06`, `.gitignore`, `.env.example`, `git init` + commit pertama | `go build ./...` sukses; `claude` membaca CLAUDE.md; 6 agent & 6 skill terdaftar |
| 3 | Docker compose Postgres 17 | `senior-devops` (Sonnet, low) | 05 L2 | `docker-compose.yml` | `docker compose ps` healthy; `psql ... -c "SELECT 1"` |
| 4 | **Rancang migration** | `senior-dba` (Opus, high) | docs/02 §2, §4; K-01, K-02, K-05 | 8 pasang file `migrations/*.up/.down.sql` | `make migrate-up`; `\dt` 6 tabel; `migrate down -all && up` bersih |
| 5 | Uji trigger manual | `senior-dba` (Opus, medium) | 05 L3 blok "Uji trigger" | Transkrip psql disimpan di `docs/evidence/trigger-test.md` | COMMIT tidak seimbang **gagal**; UPDATE entries **ditolak**; UPDATE transactions.txn_type **ditolak**, UPDATE status **lolos** |
| 6 | Config + logger + pool | `senior-go-dev` (Sonnet, medium) | 05 L4, L7; 02 §5 | `internal/config`, `platform/logger`, `repository/postgres/pool.go` | Start tanpa `DATABASE_URL` → tolak dengan pesan jelas; Docker mati → `NewPool` error |
| 7 | Domain: Money | `senior-go-dev` (Sonnet, medium) | 03 §2; skill `ledger-invariants` | `domain/money.go`, `errors.go` | — |
| 8 | Domain: Transaction, Validate, BalanceDelta | `senior-go-dev` (Sonnet, medium) | 03 §3; K-02 | `domain/transaction.go`, `account.go`, `entry.go`, `user.go` | `go vet` bersih |
| 9 | Unit test domain | `senior-qa` (Sonnet, medium) | 05 L5–L6; T-01, T-14, K-02 | `domain/*_test.go` table-driven | `make test` hijau < 5 s; coverage domain ≥ 90 % |
| 10 | **G-1 Review skema & penguncian** | **Fable 5.1 (high, read-only)** | migrations, docs/02, HANDOVER | Daftar temuan di HANDOVER | Temuan kritis = 0 sebelum minggu 2 |

Akhir minggu 1: repo bisa `make test` hijau dan `make migrate-up` jalan. Sudah bisa dipamerkan sebagai "skema ledger yang constraint-nya terbukti".

### Minggu 2 — Repository & pembuktian correctness (target: T-02, T-03, T-08, T-09 hijau)

| # | Sesi | Peran (model, effort) | Masukan | Keluaran | Bukti |
|---|---|---|---|---|---|
| 11 | Harness integration test | `senior-qa` (Sonnet, medium) | 05 L9; skill `test-strategy` | `test/integration/setup_test.go` (testcontainers + migrate), 3 helper invariant | `make test-int` menyalakan container, migration jalan, test kosong lulus |
| 12 | Rencana PostTransaction | Plan mode sesi utama (Opus, high) | 03 §5; skill `postgres-patterns`; K-01, K-02 | Rencana tertulis di HANDOVER: urutan statement, pemetaan error pgx → domain | Disetujui manusia sebelum kode |
| 13 | **Implementasi `LedgerRepo.PostTransaction`** | `senior-go-dev` (Sonnet, **high**) | Rencana sesi 12 | `repository/postgres/ledger_repo.go`, `errors.go` (isCheckViolation dsb.) | `go build`, `go vet`; jalur bahagia 1 transfer = 3 entry + saldo benar |
| 14 | Account, User, Idempotency repo | `senior-go-dev` (Sonnet, medium) | 02 §3.3; 03 §1 | `account_repo.go`, `user_repo.go`, `idempotency_repo.go` | Cursor query: `EXPLAIN` → Index Scan Backward (disimpan di `docs/evidence/`) |
| 15 | Test T-02, T-03 (trigger via Go) | `senior-qa` (Sonnet, medium) | 03 §7.2 | `test/integration/ledger_db_test.go` | Error muncul di `Commit()`, dicek `errors.Is` |
| 16 | Test T-08, T-09 (1.000 transaksi acak) | `senior-qa` (Sonnet, high) | 03 §7.2 | `test/integration/invariant_test.go` | Trial balance 0; drift 0; `-count=3` |
| 17 | Review DBA atas SQL repository | `senior-dba` (Opus, high, read-only) | semua `*_repo.go` | Temuan di HANDOVER | Tidak ada SELECT-then-UPDATE non-atomik, tidak ada Seq Scan |

### Minggu 3 — Service, idempotency, HTTP, auth (target: T-04–T-07, T-10, T-10b, T-11 hijau)

| # | Sesi | Peran (model, effort) | Masukan | Keluaran | Bukti |
|---|---|---|---|---|---|
| 18 | Service ledger + ports + idempotency | `senior-go-dev` (Sonnet, high) | 03 §5 (service), 05 L11; F-03 | `service/ledger_service.go`, `ports.go`, `idempotency.go` (hash body) | Unit test dengan stub: self-transfer, range, hash beda → ErrIdempotencyConflict |
| 19 | **Test konkurensi T-04, T-05, T-07** | `senior-qa` (Sonnet, **high**) | 03 §7.2 kode T-04; F-01 (expected 19) | `test/integration/concurrency_test.go` | `go test -race -tags=integration -run Concurrent -count=5` hijau |
| 20 | **Eksperimen "lepas version"** | manusia + `senior-qa` (Sonnet, low) | 05 L10 | Catatan kegagalan T-04 tanpa optimistic lock di `docs/evidence/lost-update.md` | Test **gagal** tanpa `AND version`; kembali hijau setelah dipulihkan. Ini bahan README "bug yang saya temukan". |
| 21 | Reversal + T-10, T-10b | `senior-go-dev` → `senior-qa` (Sonnet, medium) | 01 §2.4 D; K-03; BR-11 | `Reverse()` di service & repo; test | Reversal kedua → ErrAlreadyReversed (dari 23505); penerima bangkrut → ErrInsufficientBalance |
| 22 | Auth service + JWT + argon2id | `senior-go-dev` (Sonnet, medium) | 03 §6; handbook 6.1–6.2 | `service/auth_service.go`, `platform/token/jwt.go`, `refresh_tokens` repo | Unit test: alg none ditolak, token kedaluwarsa ditolak, refresh dicabut ditolak |
| 23 | HTTP: response, dto, middleware | `senior-go-dev` (Sonnet, medium) | 05 L12; skill `api-contract` | `transport/http/{response,dto,middleware}.go`, rate limiter in-memory (F-04) | Test tabel pemetaan error → status untuk semua kode katalog |
| 24 | HTTP: handler + router + main | `senior-go-dev` (Sonnet, medium) | 03 §4; handbook 2.4 (DI manual) | `handler_*.go`, `router.go`, `cmd/api/main.go` | `make run` → curl register/login/topup/transfer sesuai 05 L12 |
| 25 | E2E httptest + T-11 (IDOR) + T-12 | `senior-qa` (Sonnet, medium) | 03 §7.2 | `test/integration/http_test.go` | User A baca akun B → 404/403; ctx cancel → `context.Canceled` |
| 26 | **G-2 Review PostTransaction & konkurensi** | **Fable 5.1 (xhigh, read-only)** | ledger_repo, ledger_service, idempotency, test 19–21 | Temuan di HANDOVER | Temuan kritis = 0; `/code-review high` sebagai pelengkap |

### Minggu 4 — Observability, load test, Docker, CI, README (target: T-13 hijau; SLO tercapai; `docker compose up` jalan)

| # | Sesi | Peran (model, effort) | Masukan | Keluaran | Bukti |
|---|---|---|---|---|---|
| 27 | Metrik, health, pprof, drift job, graceful shutdown | `senior-devops` (Sonnet, medium) | 03 §8; 05 L13; F-05; K-06 | `platform/metrics`, `/healthz`, `/readyz`, `/metrics`, ticker drift, `Shutdown` | T-13 in-process hijau; `/metrics` memuat 8 metrik wajib |
| 28 | Dockerfile + compose lengkap + CI | `senior-devops` (Sonnet, medium) | handbook 9.1–9.3 (adaptasi ke GitHub Actions) | `Dockerfile`, `docker-compose.yml` (api + postgres + migrate), `.github/workflows/ci.yml` | image < 20 MB; `docker compose up` dari nol jalan; CI hijau di PR pertama |
| 29 | Load test k6 + verifikasi | `senior-devops` (Sonnet, medium) | 03 §7.3; skill `ops-runbook` | `test/load/transfer.js`, hasil di `docs/evidence/k6.md` | p95 < 200 ms, p99 < 500 ms @100 VU; **trial balance 0 setelah load** |
| 30 | OpenAPI + README + `/security-review` + **G-3** | `senior-go-dev` (Sonnet, low) → skill `/security-review` → **Fable 5.1 (high, read-only)** | 05 L14 struktur README; semua `docs/evidence/*` | `docs/openapi.yaml`, `README.md`, tag `v1.0.0` | Semua kotak DoD PRD §6 tercentang dengan tautan bukti |

Cadangan (bukan sesi terjadwal): `/simplify` setelah sesi 24 dan 27; `explorer` (Haiku) kapan pun butuh mencari.

---

## 6. Peta Definition of Done → Bukti

| DoD (PRD §6) | Test / artefak | Sesi |
|---|---|---|
| Registrasi/login/refresh, dompet otomatis | E2E httptest | 22, 24, 25 |
| Topup/transfer/withdraw seimbang | T-08 | 16 |
| Reversal, transaksi asli utuh | T-10, T-10b + T-03 | 21, 15 |
| Cursor pagination | test repo + EXPLAIN evidence | 14 |
| Trial balance = 0 | T-08, endpoint `/internal/ledger/trial-balance` | 16, 24 |
| 100 goroutine → 19 sukses, tak ada negatif | T-04 | 19 |
| Idempotency 10× serial & paralel | T-05, T-06 | 18, 19 |
| Deadlock A↔B 50× | T-07 | 19 |
| 1.000 transaksi acak | T-08, T-09 | 16 |
| UPDATE/DELETE entries ditolak | T-03 + `docs/evidence/trigger-test.md` | 5, 15 |
| `-race` bersih, coverage, lint, govulncheck | CI | 28 |
| testcontainers | harness | 11 |
| `docker compose up` satu perintah, image < 20 MB | Dockerfile/compose | 28 |
| healthz/readyz/metrics terpisah | sesi 27 | 27 |
| Graceful shutdown terbukti | T-13 in-process + catatan manual | 27 |
| Log JSON + request_id | middleware + logger | 6, 23 |
| k6 p95/p99 | `docs/evidence/k6.md` | 29 |
| README, OpenAPI, "bug yang saya temukan" | sesi 20 (bahan) + 30 | 20, 30 |

---

## 7. Praktik Hemat Kuota (ringkasan yang wajib, dari dok 04 §7)

1. **Satu sesi, satu peran, satu tujuan, `/clear`.** Tabel §5 sudah dipecah dengan aturan ini.
2. **`HANDOVER.md` adalah memori proyek**, bukan percakapan. Format dok 04 §7 butir 3.
3. **Sebut file dengan `@path`**, jangan "lihat service transfer".
4. **Pencarian lewat `explorer`** (Haiku), bukan grep di sesi utama.
5. **Fable hanya di G-1/G-2/G-3.** Kalau tergoda memakainya untuk debugging, coba `senior-architect` (Opus) dulu.
6. **Plan mode sebelum sesi 12, 13, 18, 19.** Kode uang yang dibongkar tiga kali lebih mahal daripada satu rencana yang dibaca.
7. Cek dashboard usage di akhir minggu 1: kalau Opus/Fable terpakai untuk pekerjaan Sonnet/Haiku, routing frontmatter tidak jalan — pindah ke pemanggilan eksplisit `model:` di Agent tool.

---

## 8. Risiko & Mitigasi (tambahan atas PRD §8)

| Risiko | Kemungkinan | Mitigasi |
|---|---|---|
| Alat belum terpasang memakan hari pertama | Tinggi | Sesi 1 dikerjakan sekali, terpisah; jangan mulai sesi 2 sebelum semua versi keluar |
| Testcontainers gagal di Windows (socket Docker) | Sedang | Docker Desktop sudah jalan; kalau gagal, set `DOCKER_HOST=npipe:////./pipe/docker_engine`, atau `TESTCONTAINERS_RYUK_DISABLED=true` sebagai langkah diagnosis (bukan permanen) |
| Agent Sonnet menebak logika uang saat spesifikasi ambigu | Sedang | System prompt `senior-go-dev` mewajibkan bertanya; dokumen ini menutup ambiguitas yang sudah diketahui (§2) |
| T-04 flaky karena `ErrConcurrentModification` diterima sebagai gagal sah | Rendah | Assert bukan pada jumlah gagal, tapi pada 19 sukses + 3 invariant. Jalankan `-count=5` |
| Deviasi K-01 (PK komposit) lupa ditulis di dok 02 | Sedang | Sesi 2 memperbarui `docs/02` §2.7 sekaligus, supaya dokumen dan skema tidak berbeda |
| Scope melebar ke outbox relay karena tabelnya sudah ada | Sedang | Fase 1 hanya **menulis** ke `outbox_events` di `PostTransaction` (opsional, satu INSERT). Tidak ada relay, tidak ada broker. |

---

## 9. Langkah Berikutnya (urutan mutlak)

1. Sesi 1: pasang alat (§1). Verifikasi semua versi.
2. Sesi 2: bootstrap repo `nusaledger/` + salin docs 00–06 + pasang `.claude/` (§3). Commit pertama.
3. Perbaiki F-01, F-02, F-03 dan K-01 di `docs/02` dan `docs/03` sebelum sesi 4, supaya agent membaca sumber yang sudah benar.
4. Lanjut tabel §5 secara berurutan. Jangan lompat ke HTTP sebelum T-04 hijau — itu kesalahan urutan yang dokumen 05 peringatkan di baris terakhirnya.

---

## 10. Status Eksekusi (diperbarui 2026-09-16)

| Sesi | Status | Catatan |
|---|---|---|
| 1–9 (Minggu 1) | ✅ | Alat, repo, skema + 18 uji trigger, domain 99 % coverage. Go 1.27 (bukan 1.25). Port DB 5433. |
| 10 (G-1) | ✅ inline | Dilakukan sesi utama saat menulis migration: `chk_owner`, `chk_reversal_link`, `uq_entry_account_per_txn`, CONSTRAINT name pada RAISE. |
| 11–17 (Minggu 2) | ✅ | `LedgerRepo.Post`, harness testcontainers, EXPLAIN 0,49 ms @200k, T-02/03/08/09. Review DBA (17) dilakukan inline. |
| 18–25 (Minggu 3) | ✅ | Service, auth, HTTP, T-04 (19 sukses ×5), T-05, T-06, T-07 (0 deadlock), T-10/10b, T-11, T-12 (ctx via Timeout middleware), eksperimen kunci (temuan berbeda dari dugaan dokumen). |
| 26 (G-2) | ✅ | Review Fable (Agent tool, `model: "fable"`, read-only) atas `Post` + konkurensi. Kesimpulan: nol temuan penciptaan/kehilangan uang atau deadlock. 4 perbaikan diterapkan (bug idempotency race, konsistensi Validate reversal, translate() SQLSTATE, overflow pre-check) + T-10c + assert conflict==0. Diverifikasi `-race` penuh. |
| 27–29 (Minggu 4) | ✅ | T-13, Docker 18,6 MB, compose ready 4 s, CI (lulus di GitHub run pertama), OpenAPI, k6. **SLO latensi tidak tercapai** (177 tps, p95 515 ms) — penyebab: baris panas akun fee. Correctness 100 %. |
| 30 (G-3, security review, tag) | ✅ | G-3 (Fable, read-only): YA-DENGAN-CATATAN. Temuan Medium (register tanpa rate limit, argon2id) ditindaklanjuti sebelum tag. `/security-review` skill: diff kosong (semua sudah di-push), diganti audit manual dalam G-3. Tag `v1.0.0` dibuat setelah dokumen ini. |

Penyimpangan dari plan yang perlu diketahui: port API host 8081 (8080 dipakai laragon); `misspell` dimatikan; `middleware.RealIP` sengaja tidak dipakai; `docker-compose.load.yml` untuk k6; subagent kustom di `.claude/agents/*.md` **tidak dikenali** sebagai `subagent_type` oleh Agent tool di harness ini — G-1 informal, G-2/G-3 dijalankan via `subagent_type: "general-purpose"` dengan `model` di-override manual (`"fable"`/`"sonnet"`) dan persona ditulis lengkap di prompt, bukan lewat file agent proyek.
