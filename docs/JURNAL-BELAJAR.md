# Jurnal Belajar NusaLedger

Catatan setiap langkah yang dilakukan, **untuk dipelajari dan diulang sendiri secara manual.** Format tiap entri: **Apa** (yang dikerjakan) · **Kenapa** (alasannya — bagian terpenting) · **Contoh** (potongan konkret) · **Bukti** (cara memastikan benar). Tanggal mengikuti urutan pengerjaan.

> Cara memakai jurnal ini untuk naik level: jangan hanya membaca. Setelah satu entri, tutup jurnalnya, lalu coba ulangi langkahnya dari ingatan di folder lain. Kalau macet, buka lagi bagian **Kenapa**, bukan bagian **Contoh**.

---

## Sesi 1 — Memasang alat (2026-09-16)

### Apa
Memasang seluruh toolchain di Windows 11 lewat `winget` dan `go install`:

```powershell
winget install --id GoLang.Go -e                 # Go 1.27.0
winget install --id ezwinports.make -e           # GNU Make 4.4.1
winget install --id GolangCI.golangci-lint -e    # golangci-lint 2.13.2
winget install --id GrafanaLabs.k6 -e            # k6 2.2.0  (bukan "k6.k6" — id itu tidak ada)

# tiga alat berikut dipasang lewat Go sendiri, hasilnya masuk ke %USERPROFILE%\go\bin
go install github.com/golang-migrate/migrate/v4/cmd/migrate@latest
go install golang.org/x/vuln/cmd/govulncheck@latest
go install github.com/go-delve/delve/cmd/dlv@latest
```

### Kenapa
- **Semua alat dipasang di awal**, sebelum satu baris kode pun. Berhenti di tengah debugging untuk memasang debugger adalah cara tercepat kehilangan alur pikiran (docs/05 Langkah 0).
- **`winget` untuk aplikasi, `go install` untuk alat berbasis Go.** `go install` mengompilasi dari sumber ke `$GOPATH/bin`; ini juga cara Anda nanti memasang alat internal tim.
- **Kenapa `migrate` dan bukan ORM auto-migrate:** migration adalah file SQL bernomor yang di-review seperti kode. Skema uang tidak boleh berubah "otomatis" karena struct Go berubah.
- **`govulncheck`** memeriksa apakah dependensi punya kerentanan yang *benar-benar dipanggil* kode kita, bukan sekadar ada di `go.sum`. Jauh lebih sedikit false positive daripada pemindai lain.

### Jebakan yang ditemui
1. Shell yang sudah terbuka **tidak melihat PATH baru**. Solusi tanpa restart:
   ```powershell
   $env:Path = [Environment]::GetEnvironmentVariable('Path','Machine') + ';' + [Environment]::GetEnvironmentVariable('Path','User')
   ```
   Kenapa: PATH dibaca sekali saat proses shell dibuat. winget mengubah registri, bukan proses yang sedang jalan.
2. Id paket winget harus persis. Cari dulu: `winget search k6`.

### Bukti
```
go version go1.27.0 windows/amd64
GNU Make 4.4.1
golangci-lint has version 2.13.2
govulncheck@v1.8.0
k6.exe v2.2.0
Delve Version: 1.27.2
Docker version 29.7.2
```

---

## Sesi 2 — Bootstrap repo & setup Claude (2026-09-16)

### Apa
1. Membuat struktur direktori lengkap (docs/03 §1), `go mod init`, `git init -b main`.
2. Menulis `Makefile`, `.gitignore`, `.env.example`, `.golangci.yml`, `docker-compose.yml`, `README.md`.
3. Menyalin dokumen 00–06 ke `docs/` dan **memperbaiki empat hal** langsung di salinannya:
   - F-01: T-04 = **19** sukses, sisa Rp 31.000 (bukan 20 / 0).
   - F-02: Go **1.27** (yang terpasang), bukan 1.24/1.25.
   - F-03: kode error baru `IDEMPOTENCY_IN_FLIGHT` (409).
   - K-01: `idempotency_keys` PK = `(user_id, key)`.
4. Menulis `CLAUDE.md`, `HANDOVER.md`, `.claude/settings.json`, 6 agent, 6 skill.

### Kenapa
**Struktur dibuat lengkap di awal** supaya tidak ada godaan menaruh file "sementara di root dulu". Nama direktori `internal/` bukan gaya: compiler Go **melarang** paket di dalamnya diimpor dari modul lain. Ini satu-satunya enkapsulasi level-modul di Go, dan gratis.

**`go mod init github.com/caesarovera/nusaledger`** — module path adalah "nama lengkap" paket Anda. Semua import internal ditulis relatif terhadapnya: `import "github.com/caesarovera/nusaledger/internal/domain"`. Kalau username GitHub berbeda, ganti sekarang selagi belum ada import: `go mod edit -module github.com/<user>/nusaledger`.

**`git init -b main`** sebelum ada kode, supaya commit pertama adalah kerangka kosong. Diff berikutnya jadi kecil dan mudah di-review — termasuk oleh Anda sendiri minggu depan.

**Makefile sebelum ada kode**, karena `go test -tags=integration -race -count=1 ./test/...` tidak akan Anda ketik ratusan kali; `make test-int` akan. Dua detail penting:
- `SHELL := bash` — resep Makefile memakai sintaks bash (`CGO_ENABLED=0 go build ...`). Tanpa baris ini, make di Windows memanggil `cmd.exe` dan resep itu gagal.
- `export GOFLAGS ?= -mod=readonly` — `go build` **menolak** mengubah `go.mod` diam-diam. Perubahan dependensi harus lewat `make tidy` yang sadar.

**`.gitignore` menolak `.env`** dan `settings.json` menolak `Read(./.env)`. Dua lapis untuk ancaman berbeda: yang pertama mencegah kredensial masuk git; yang kedua mencegah kredensial masuk konteks AI. Keduanya adalah kebocoran yang sulit ditarik kembali.

**Perbaikan dokumen dilakukan di salinan `docs/`**, bukan di file asli di luar repo, karena agent AI membaca `docs/`. Kalau dokumen yang dibaca salah (T-04 = 20), agent akan menulis test yang salah dengan sangat percaya diri.

**Kenapa PK `(user_id, key)` (K-01):** dengan `key` sebagai PK global, user B yang mengirim key yang sama dengan user A akan ditolak 409 — padahal mereka tidak berhubungan. Klien mobile sering memakai UUID, jadi tabrakan jarang; tapi "jarang" bukan "tidak pernah", dan perbaikannya cuma satu baris selagi tabel kosong.

**`docker-compose.yml`** hanya berisi Postgres dulu. Tiga flag `-c` yang dipasang:
- `log_min_duration_statement=200ms` → query lambat tercatat sejak hari pertama.
- `idle_in_transaction_session_timeout=60000` → transaksi yang lupa di-commit dibunuh setelah 60 detik. Di ledger, transaksi menggantung **mengunci baris akun** dan menghentikan semua transfer ke akun itu.
- `statement_timeout=30000` → satu query nyangkut tidak menahan koneksi selamanya.
- `healthcheck` dengan `pg_isready` → layanan lain (`api`, nanti) bisa `depends_on: condition: service_healthy`, bukan menebak dengan `sleep`.

**`.golangci.yml`** memilih linter yang menutup kelas bug nyata, bukan gaya:
`errorlint` (memaksa `errors.Is`), `rowserrcheck`/`sqlclosecheck` (rows bocor), `noctx`/`contextcheck` (ctx hilang di tengah jalan), `gosec` (SQL concat, crypto lemah), `bodyclose`.

**Setup Claude (`.claude/`)** memisahkan peran: satu sesi yang menulis kode sekaligus mereview kodenya sendiri akan bias. Agent punya context window sendiri, jadi review-nya jujur. Model: Opus untuk memutuskan (architect, dba), Sonnet untuk mengerjakan (go-dev, qa, devops), Haiku untuk mencari (explorer). Skill dimuat hanya saat relevan, supaya `CLAUDE.md` tetap ramping (setiap barisnya dibayar setiap sesi).

### Contoh — cara membaca Makefile
```makefile
test-int: ## integration test dengan testcontainers, butuh Docker
	go test -tags=integration -race -count=1 ./test/...
```
- `-tags=integration` → hanya file dengan `//go:build integration` yang ikut dikompilasi. Tanpa tag, `make test` tetap cepat karena test Docker tidak ikut.
- `-race` → race detector. Test konkurensi tanpa `-race` nyaris tidak berguna.
- `-count=1` → matikan cache hasil test. Test yang menyentuh database eksternal tidak boleh di-cache.

### Bukti
```
go build ./...          # sukses (modul valid meski belum ada kode)
make help               # daftar perintah muncul
git log --oneline       # commit pertama: chore: bootstrap repo
```

---

## Sesi 3 — PostgreSQL 17 lewat Docker Compose (2026-09-16)

### Apa
```bash
docker compose up -d postgres
docker compose ps                      # tunggu sampai "healthy"
psql -h 127.0.0.1 -p 5433 -U nusa -d nusaledger -c "SELECT version();"
```

### Kenapa
**Database lewat Docker, bukan instalasi lokal**, supaya versinya persis sama dengan yang dipakai test (testcontainers) dan produksi nanti. "Jalan di laptop saya" berhenti menjadi alasan.

**`healthcheck` + menunggu `healthy` sebelum lanjut.** PostgreSQL butuh beberapa detik sebelum menerima koneksi. Aplikasi yang start bersamaan akan gagal terhubung, dan Anda akan menghabiskan waktu mencari bug yang sebenarnya cuma soal waktu.

### Jebakan yang ditemui — dan ini pelajaran penting
Koneksi dari host **gagal autentikasi** padahal `docker compose exec postgres psql` berhasil. Diagnosis:
```bash
netstat -ano | grep ":5432" | grep LISTEN     # DUA proses: postgres.exe (laragon) dan com.docker.backend.exe
```
Ada PostgreSQL lokal (laragon) yang sudah menempati port 5432. Windows mengarahkan koneksi ke layanan yang lebih dulu ada, jadi kita login ke Postgres yang salah dengan password yang salah.

**Cara berpikirnya:** kalau gejalanya "password salah" tapi Anda yakin password benar, pertanyaannya bukan "password apa yang benar?" melainkan **"saya sedang bicara dengan server yang mana?"** Cek siapa yang mendengarkan di port itu sebelum mengutak-atik kredensial.

**Solusi yang dipilih:** petakan host `5433` → container `5432` di compose. Kenapa bukan mematikan laragon: mengubah layanan sistem milik orang lain untuk kebutuhan satu proyek itu rapuh; port mapping itu lokal ke proyek dan reversibel. Konsekuensinya `DATABASE_URL` memakai `127.0.0.1:5433` (bukan `localhost`, supaya tidak ambigu IPv4/IPv6).

### Bukti
```
nusa | PostgreSQL 17.11 on x86_64-pc-linux-musl
```

---

## Sesi 4 — Migration & skema (2026-09-16)

### Apa
Delapan pasang file di `migrations/` (`000001` … `000008`, masing-masing `.up.sql` dan `.down.sql`), isinya dari `docs/02 §2` ditambah tiga keputusan baru:
- **K-01** `idempotency_keys`: `PRIMARY KEY (user_id, key)`.
- **K-02** `entries`: `UNIQUE (transaction_id, account_id)` — satu akun satu entry per transaksi.
- **K-05** trigger `trg_transactions_status_only`: `transactions` hanya boleh mengubah `status`, dan hanya `POSTED → REVERSED`.

Tambahan kecil yang tidak ada di dokumen tapi menutup lubang: `chk_owner` (dompet wajib punya `user_id`, akun sistem wajib `NULL`), `chk_reversal_link` (hanya `REVERSAL` yang boleh punya `reverses_transaction_id`), dan `CONSTRAINT = 'trg_entries_balanced'` pada `RAISE` supaya kode Go bisa membedakan pelanggaran ini dari `CHECK` lain lewat `pgErr.ConstraintName`.

```bash
make migrate-up
migrate -path migrations -database "$DATABASE_URL" down -all   # uji rollback selagi kosong
migrate -path migrations -database "$DATABASE_URL" up
```

### Kenapa
**Skema sebelum kode Go**, karena skema adalah kontrak yang paling mahal diubah. Kode direfaktor satu jam; mengubah tabel berisi jutaan baris uang butuh perencanaan migrasi tersendiri.

**Satu migration satu tujuan.** Kalau `000005_ledger_triggers` gagal di tengah, Anda tahu persis apa yang belum terpasang. File "semua-dalam-satu" yang gagal meninggalkan skema setengah jadi yang sulit didiagnosis.

**Setiap `up` punya `down` yang diuji sekarang**, selagi database kosong. Nanti saat sudah ada data, Anda tidak akan berani mengujinya.

**Kenapa `DEFERRABLE INITIALLY DEFERRED` pada trigger balanced:** entry disisipkan satu per satu. Setelah baris pertama (debit 1.000), transaksi belum seimbang. Kalau trigger diperiksa saat itu juga, semua transaksi selalu ditolak. Dengan *deferred*, pemeriksaan ditunda sampai `COMMIT`. Konsekuensi untuk kode Go nanti: **error muncul dari `tx.Commit()`, bukan dari `tx.Exec()`** — ini sering mengejutkan.

**Kenapa `RAISE ... USING ERRCODE = 'check_violation', CONSTRAINT = 'trg_entries_balanced'`:** driver pgx mengembalikan `PgError{Code: "23514", ConstraintName: "..."}`. Tanpa nama constraint, kode Go tidak bisa membedakan "tidak seimbang" dari "saldo negatif" (keduanya 23514) kecuali membandingkan string pesan — dan itu rapuh.

### Jebakan yang ditemui
1. **`go install .../cmd/migrate@latest` menghasilkan binary TANPA driver database.** Errornya: `unknown driver postgres (forgotten import?)`. Perbaikan: `go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest`. Pelajaran umum Go: build tag mengubah isi binary; baca README alat sebelum `go install`.
2. **`migrate drop` tidak membuang tipe ENUM dan function**, hanya tabel. `up` berikutnya gagal: `type "account_type" already exists`, dan database masuk keadaan *dirty*. Untuk reset dev dengan data: `DROP SCHEMA public CASCADE; CREATE SCHEMA public;` lalu `migrate up` (sudah jadi `make db-reset`).
3. **`down -all` gagal begitu ada data** di `entries`: migration 8 (`DELETE FROM accounts`) ditolak foreign key. Ini **disengaja** — migration tidak boleh menghapus jejak uang. Artinya `down` hanya untuk skema kosong atau rollback struktur, bukan alat reset data.

### Bukti
```
8/u seed_system_accounts   → versi 8, 8 tabel (7 + schema_migrations)
trg_entries_balanced | entries | deferrable=t | initdeferred=t
down -all → 0 tabel → up → versi 8
```

---

## Sesi 5 — Uji trigger manual, sebelum satu baris Go pun (2026-09-16)

### Apa
Skrip `docs/evidence/trigger-test.sql` berisi 18 skenario, dijalankan lewat psql; transkripnya disimpan di `docs/evidence/trigger-test.md`. Setiap skenario diberi label **HARUS GAGAL** atau **HARUS SUKSES** sebelum dijalankan.

### Kenapa
**Seluruh jaminan kebenaran sistem bergantung pada trigger dan constraint ini.** Kalau `COMMIT` transaksi tidak seimbang *berhasil*, semua kode Go di atasnya membangun di atas pasir. Mengujinya lewat psql, tanpa Go, memisahkan dua pertanyaan: "apakah databasenya benar?" dan "apakah kode Go-nya benar?". Kalau nanti test Go gagal, Anda sudah tahu jawaban pertanyaan pertama.

**Menulis ekspektasi (HARUS GAGAL / HARUS SUKSES) sebelum menjalankan** adalah inti dari pengujian. Tanpa itu, Anda cenderung menerima apa pun hasilnya sebagai "benar".

### Contoh — skenario terpenting
```sql
BEGIN;
INSERT INTO transactions (id, txn_type) VALUES ('0000...0001', 'TOPUP');
INSERT INTO entries (transaction_id, account_id, direction, amount, balance_after)
VALUES ('0000...0001', 1, 'DEBIT', 1000, 1000);          -- INSERT 0 1  ← lolos! belum diperiksa
COMMIT;                                                   -- ERROR: transaksi ... tidak seimbang: debit=1000 kredit=0
```
Perhatikan: `INSERT` **berhasil**, `COMMIT` yang **gagal**. Itulah *deferred*.

### Jebakan yang ditemui
Uji `UPDATE entries ... WHERE id = 1` menghasilkan `UPDATE 0`, bukan ERROR. Bukan karena trigger tidak jalan, tapi karena **id 1 sudah "terbakar"** oleh transaksi yang di-rollback di skenario sebelumnya: *sequence* PostgreSQL tidak ikut di-rollback. Baris nyata ber-id 2 dan 3. `UPDATE 0` berarti "tidak ada baris yang cocok", dan trigger `BEFORE UPDATE ... FOR EACH ROW` tidak pernah dipanggil untuk nol baris. Pelajaran: **hasil "tidak ada error" belum tentu "lolos uji"** — periksa jumlah baris yang tersentuh.

### Bukti
14 ERROR persis pada 14 skenario HARUS GAGAL; 4 skenario HARUS SUKSES berhasil; trial balance `1000 | 1000 | 0`. Transkrip lengkap: `docs/evidence/trigger-test.md`.

---

## Sesi 6 — Config, logger, connection pool (2026-09-16)

### Apa
- `internal/config/config.go`: struct dengan tag `env:"..."`, dibaca `caarlos0/env`, lalu **divalidasi** di `Load()`.
- `internal/platform/logger/logger.go`: `slog` JSON dengan `ReplaceAttr` yang mengganti nilai field sensitif menjadi `[REDACTED]`.
- `internal/repository/postgres/pool.go`: `pgxpool` dengan MaxConns 25, MinConns 5, lifetime 30 m, idle 5 m, dan `Ping` saat startup.

### Kenapa
**Konfigurasi divalidasi saat startup, bukan saat dipakai.** `Load()` menolak `APP_ENV` asing, `JWT_SECRET` pendek di produksi, `MIN > MAX`, refresh token lebih pendek dari access token. Aplikasi yang menolak start dengan pesan jelas jauh lebih murah daripada aplikasi yang hidup lalu gagal saat ada yang login.

**Nominal (fee, min, max) di config, bukan hardcode** (BR-13, BR-14). Keputusan bisnis tidak boleh butuh deploy ulang, dan test bisa memakai nilai berbeda tanpa mengakali kode.

**Redaksi log terpusat**, bukan "ingat jangan log password" di tiap handler. Manusia lupa; `ReplaceAttr` tidak. Daftar kuncinya: `password`, `token`, `access_token`, `refresh_token`, `authorization`, `secret`.

**`Ping` di `NewPool`.** `pgxpool.New` **tidak** membuka koneksi — ia malas (*lazy*). Tanpa `Ping`, aplikasi "berhasil start" dengan database mati, dan semua request 500. Dengan `Ping`, kegagalan muncul sebagai "aplikasi menolak start".

### Contoh — pola validasi
```go
func Load() (Config, error) {
    var c Config
    if err := env.Parse(&c); err != nil {
        return c, fmt.Errorf("memuat konfigurasi: %w", err)   // bungkus dengan konteks, %w agar errors.Is tetap jalan
    }
    if err := c.validate(); err != nil {
        return c, fmt.Errorf("konfigurasi tidak valid: %w", err)
    }
    return c, nil
}
```

### 🐛 Bug #1 yang ditemukan lewat test (bahan README)
Test "tanpa DATABASE_URL" memakai `t.Setenv("DATABASE_URL", "")`, dan `Load()` **tidak** mengembalikan error. Ternyata tag `required` di `caarlos0/env` hanya memeriksa "variabel ada", bukan "variabel berisi". Di produksi, `DATABASE_URL=` (kosong karena salah ketik di manifest) akan lolos validasi lalu gagal di `Ping` dengan pesan yang membingungkan. Perbaikan: `env:"DATABASE_URL,required,notEmpty"`.
Pelajaran: **baca dokumentasi tag pustaka pihak ketiga**, dan tulis test untuk kasus "ada tapi kosong", bukan hanya "tidak ada".

### Bukti
```
ok  internal/config          coverage: 90.0%
ok  internal/platform/logger coverage: 80.0%   (nilai "rahasia123" tidak muncul di output)
```

---

## Sesi 7–8 — Domain: Money, Account, Entry, Transaction, User (2026-09-16)

### Apa
`internal/domain/` berisi lima file dan **tidak mengimpor paket internal apa pun** (hanya stdlib + `uuid`):
- `errors.go` — sentinel error, dikelompokkan per kategori.
- `money.go` — `type Money int64`, `NewMoney`, `Add`/`Sub` dengan deteksi overflow, `String`.
- `account.go` — `Direction`, `AccountType` + `NormalBalance()`, `AccountStatus`, `Account`.
- `entry.go` — `Entry`, `PostedEntry`, `BalanceDelta`.
- `transaction.go` — `Transaction`, `Validate()`, `AccountIDs()`, dan **empat builder** `NewTopup`, `NewTransfer`, `NewWithdraw`, `NewReversal`.

### Kenapa
**`type Money int64`, bukan `int64` telanjang.** Dengan `int64`, `transfer(amount, userID)` yang argumennya tertukar lolos kompilasi. Dengan `Money`, compiler menolaknya. Biayanya nol saat runtime.

**Overflow diperiksa manual di `Add`.** Go tidak melempar error saat `int64` meluap; nilainya berputar menjadi negatif tanpa peringatan. Rumus deteksinya: kalau menambah bilangan positif tapi hasilnya lebih kecil, atau menambah negatif tapi hasilnya lebih besar, berarti meluap.

**`NormalBalance()` adalah method di `AccountType`, bukan tabel `if`.** Satu tempat kebenaran: hanya `SYSTEM_CASH` (aset) yang bernormal DEBIT. `BalanceDelta` lalu jadi satu rumus untuk semua akun: searah normal → `+amount`, berlawanan → `-amount`.

**Builder empat operasi ada di domain, bukan service.** Di Fase 2, worker message queue juga akan memposting transaksi. Kalau susunan entry ada di service HTTP, worker bisa menyusun arah yang salah tanpa ada yang menyadari. Dengan builder di domain, hanya ada satu cara membuat TRANSFER.

**`Validate()` menegakkan tiga aturan sekaligus sebelum database disentuh:** minimal dua entry, semua `amount > 0`, Σdebit = Σkredit, dan (K-02) satu akun sekali saja. Ini duplikasi *sengaja* dengan trigger database: domain memberi pesan error berguna dalam mikrodetik; trigger adalah jaminan terakhir untuk jalur yang lupa memanggil `Validate()`.

**`AccountIDs()` mengembalikan id TERURUT dan tanpa duplikat.** Ini bukan kerapian: urutan inilah yang dipakai `FOR UPDATE` nanti, dan urutan tetap adalah satu-satunya hal yang mencegah deadlock transfer silang.

**`NewReversal` menolak reversal atas reversal** (`ErrNotReversible`). BR-11 tidak melarangnya secara eksplisit, tapi membalik pembalikan hanya membuat jejak audit membingungkan; kalau reversal salah, buat transaksi baru yang benar.

### Contoh — cara membaca `BalanceDelta`
```go
// Andi (USER_WALLET, normal CREDIT) didebit Rp 51.000 saat transfer:
delta := domain.BalanceDelta(domain.Entry{Direction: DEBIT, Amount: 51_000_00}, CREDIT)
// DEBIT ≠ CREDIT → -51.000 → saldo Andi berkurang. Benar: utang perusahaan ke Andi berkurang.

// Kas perusahaan (SYSTEM_CASH, normal DEBIT) didebit Rp 100.000 saat topup:
delta = domain.BalanceDelta(domain.Entry{Direction: DEBIT, Amount: 100_000_00}, DEBIT)
// DEBIT == DEBIT → +100.000 → kas bertambah. Benar.
```

---

## Sesi 9 — Unit test domain (2026-09-16)

### Apa
`money_test.go`, `transaction_test.go` (paket `domain_test`, bukan `domain`), `config_test.go`, `logger_test.go`. Semua table-driven dengan `t.Run` dan `t.Parallel()`.
```powershell
go test -race -count=1 -cover ./internal/...
```

### Kenapa
**Paket `domain_test` (external test package)** memaksa test memakai domain lewat API publiknya saja, persis seperti service nanti. Kalau sesuatu tidak bisa diuji dari luar, itu sinyal API-nya kurang.

**Table-driven + `errors.Is`.** Satu daftar kasus, satu loop; menambah kasus baru = menambah satu baris. Jenis error dibandingkan dengan `errors.Is`, bukan string pesan, supaya pesan boleh diubah tanpa merusak test.

**`t.Parallel()` di semua subtest domain** karena domain murni tanpa state bersama. Ini juga cara murah menangkap *data race* tersembunyi begitu `-race` aktif.

**Empat kasus `BalanceDelta` diuji satu per satu** (arah × normal balance). Kesalahan tanda di sini membuat seluruh ledger salah dengan cara yang sangat sulit dilacak di kemudian hari.

**Test builder memverifikasi *efek saldo*, bukan hanya struktur.** `TestNewTransfer` menghitung delta Andi (−51.000), Budi (+50.000), fee (+1.000) dengan `BalanceDelta` — ini menguji skenario docs/01 §2.4 apa adanya.

### Jebakan yang ditemui
1. **`go test -race` di Windows butuh cgo → butuh gcc.** Errornya: `-race requires cgo`. Solusi: `winget install BrechtSanders.WinLibs.POSIX.UCRT`, lalu muat ulang PATH. Di Linux/CI ini tidak terjadi.
2. **Linter `misspell` menandai "implementasi" sebagai salah eja "implements".** Karena seluruh komentar berbahasa Indonesia, linter ini dimatikan di `.golangci.yml` dengan alasan tertulis.
3. **gofmt menyelaraskan kolom struct.** Setelah menambah field/komentar, jalankan `gofmt -s -w .` — lint akan menolak file yang belum diformat.

### Bukti
```
ok  internal/config           1.9s  coverage: 90.0%
ok  internal/domain           1.9s  coverage: 93.5%   ← target DoD ≥ 90 %
ok  internal/platform/logger  1.9s  coverage: 80.0%
golangci-lint run ./...   → 0 issues
```

---

## Sesi 11 — Harness integration test dengan testcontainers (2026-09-16)

### Apa
- `migrations/embed.go`: `//go:embed *.sql` — file migration ikut masuk binary.
- `internal/platform/dbmigrate`: menjalankan migration ter-embed lewat `golang-migrate` (source `iofs`, driver `pgx5`).
- `test/integration/main_test.go`: `TestMain` menyalakan **satu** container `postgres:17-alpine`, migrasi, buat pool; semua test memakainya.
- `helpers_test.go`: `resetDB` (TRUNCATE + seed akun sistem), `seedUser`, `seedWallet` (saldo awal lewat TOPUP sungguhan), dan **tiga assert invariant**.
- `schema_test.go`: T-02, T-03, K-05 dibuktikan dari Go, termasuk `pgErr.Code` dan `ConstraintName`.

```powershell
go test -tags=integration -race -count=1 ./test/...
```

### Kenapa
**Test unit tidak pernah bisa membuktikan SQL benar.** Mock hanya membuktikan Go memanggil fungsi yang benar; ia tidak tahu apakah `ORDER BY` ada atau trigger menyala. Hanya PostgreSQL sungguhan yang bisa.

**Testcontainers, bukan database lokal**, karena database lokal menyimpan sisa data dari run sebelumnya, dan test yang lulus karena data sisa adalah test yang berbohong. Container baru per run = kondisi awal yang bersih dan **versi Postgres persis sama** dengan compose.

**Satu container per paket (`TestMain`), bukan per test.** Menyalakan Postgres ±5 detik; dengan 30 test itu 2,5 menit terbuang. Isolasi antar test cukup lewat `TRUNCATE ... RESTART IDENTITY CASCADE` yang butuh milidetik.

**Migration di-embed dan dijalankan dari Go**, bukan `migrate` CLI dari shell test. Dua alasan: (1) test tidak bergantung pada alat eksternal dan PATH; (2) migration yang diuji **persis** yang akan dijalankan aplikasi saat startup nanti — tidak ada dua sumber kebenaran.

**`seedWallet` mengisi saldo lewat TOPUP sungguhan** (transaksi + 2 entry + update balance), bukan `UPDATE accounts SET balance = ...`. Kalau seed memakai UPDATE langsung, `assertMaterializedBalanceMatchesEntries` langsung gagal sejak awal — invariant harus benar dari baris pertama data.

**`TRUNCATE`, bukan `DELETE`, untuk reset.** `entries` punya trigger yang menolak DELETE; `TRUNCATE` adalah operasi berbeda yang tidak memicu trigger baris. Ini juga alasan trigger append-only bukan pertahanan satu-satunya — di produksi, hak `TRUNCATE` dicabut dari role aplikasi.

### Contoh — cara membaca error Postgres dari Go
```go
err = tx.Commit(ctx)                       // trigger DEFERRED baru bicara di sini
var pgErr *pgconn.PgError
if errors.As(err, &pgErr) && pgErr.Code == "23514" && pgErr.ConstraintName == "trg_entries_balanced" {
    // tidak seimbang
}
```
`errors.As` menggali rantai `%w` sampai menemukan `*pgconn.PgError`. Membandingkan `pgErr.Message` (teks) akan rapuh; kode SQLSTATE dan nama constraint stabil.

### Bukti
`ok test/integration 10.6s` — 5 test, container menyala sekali.

---

## Sesi 12–13 — `LedgerRepo.Post`: fungsi terpenting di seluruh sistem (2026-09-16)

### Apa
`internal/repository/postgres/ledger_repo.go`. Urutan langkah di dalam SATU transaksi database:
1. `Validate()` (lapis kedua).
2. Klaim idempotency: `INSERT ... ON CONFLICT (user_id, key) DO NOTHING`; 0 baris → `ErrIdempotencyInFlight`.
3. Kunci akun: `WHERE id = ANY($1) ORDER BY id FOR UPDATE`; jumlah baris ≠ jumlah id → `ErrAccountNotFound`; status ≠ ACTIVE → `ErrAccountNotActive`.
4. Kalau reversal: `UPDATE transactions SET status='REVERSED' WHERE id=$1 AND status='POSTED'`; 0 baris → `ErrAlreadyReversed`.
5. Insert header `transactions`.
6. Per entry: hitung delta, **pre-check saldo dompet di Go**, `UPDATE accounts ... WHERE id AND version AND status='ACTIVE' RETURNING balance`, insert `entries` dengan `balance_after`.
7. Simpan hasil ke `idempotency_keys` (status 201 + JSON hasil).
8. Tulis satu baris `outbox_events`.
9. `Commit()` — trigger memverifikasi debit = kredit.

Ditambah `errors.go` (`translate`: SQLSTATE → error domain), `account_repo.go`, `idempotency_repo.go`.

### Kenapa — enam keputusan yang membuat kode ini benar
| Keputusan | Kalau tidak dilakukan |
|---|---|
| Klaim idempotency **di dalam** transaksi yang sama | Key tercatat, transaksi rollback → retry ditolak padahal uang belum pindah |
| `ORDER BY id` sebelum `FOR UPDATE` | Andi→Budi dan Budi→Andi bersamaan saling tunggu = deadlock |
| `version` di `WHERE` update saldo | Lost update saat dua proses membaca saldo lama |
| `status = 'ACTIVE'` di SQL, bukan hanya di Go | Ada jendela waktu antara pengecekan dan update |
| `translate()` mengubah pelanggaran CHECK jadi error domain | Klien menerima 500 padahal seharusnya 422 |
| `defer tx.Rollback(ctx)` di baris kedua | Satu `return` yang terlewat meninggalkan transaksi menggantung → kunci akun tertahan |

**Kenapa pre-check saldo di Go padahal ada CHECK constraint:** keduanya dipertahankan. Pre-check memberi error rapi tanpa membuat Postgres melempar exception (yang membatalkan transaksi dan lebih mahal). CHECK adalah jaring terakhir untuk jalur yang lupa pre-check.

**Kenapa hasil disimpan sebagai JSON `storedResult` privat, bukan DTO HTTP:** repository tidak boleh tahu HTTP. Yang disimpan adalah *hasil domain* (transaksi + entry + saldo sesudah). Handler nanti merender DTO dari hasil ini, sehingga replay menghasilkan respons **identik** tanpa repository mengenal bentuk JSON API.

**Kenapa reversal menandai transaksi asal DAN mengandalkan `uq_reversal`:** UPDATE bersyarat `status='POSTED'` menangkap kasus normal dengan pesan jelas; unique index menangkap kasus dua reversal bersamaan yang lolos pengecekan pada saat yang sama. Dua lapis untuk dua ancaman.

**Kenapa outbox ditulis sekarang padahal belum ada konsumen:** menambah satu INSERT di jalur yang sudah teruji itu murah sekarang; mengubah `Post` setelah T-04–T-09 hijau itu mahal (semua test uang harus diulang). Fase 2 tinggal menambah relay.

### Contoh — pola `RowToStructByName`
```go
type lockedAccount struct {
    ID      int64  `db:"id"`
    Balance int64  `db:"balance"`
    Version int64  `db:"version"`
    // ...
}
rows, _ := tx.Query(ctx, `SELECT id, balance, version, ... ORDER BY id FOR UPDATE`, ids)
accounts, err := pgx.CollectRows(rows, pgx.RowToStructByName[lockedAccount])
```
`CollectRows` menutup `rows` sendiri dan memetakan kolom ke field lewat tag `db`. Lebih aman daripada `Scan` manual yang urutannya mudah tertukar.

### Bukti
```
--- PASS: TestLedgerRepo_Post_Transfer                       (3 entry, saldo Andi 49.000, Budi 50.000, fee 1.000)
--- PASS: TestLedgerRepo_Post_SaldoKurang                    (rollback bersih, klaim idempotency ikut hilang)
--- PASS: TestLedgerRepo_Post_IdempotencyTersimpanDanInFlight (Find → 201 + hasil identik; key ulang → in-flight)
--- PASS: TestLedgerRepo_Post_AkunTidakAktifDanTidakAda
--- PASS: TestLedgerRepo_Post_Reversal                       (T-10 & T-10b)
--- PASS: TestLedgerRepo_TrialBalanceDanDrift
ok  test/integration  7.8s  (-race)
```

---

## Sesi 14 — Membuktikan cursor pagination dengan EXPLAIN (2026-09-16)

### Apa
Mengisi DB dev dengan 100 dompet, 100.000 transaksi, 200.000 entries lewat `generate_series`, lalu `EXPLAIN (ANALYZE, BUFFERS)` pada query mutasi rekening (docs/02 §3.3). Hasil di `docs/evidence/explain-cursor.md`. DB di-reset setelahnya.

### Kenapa
**Index yang "seharusnya dipakai" belum tentu dipakai.** Planner PostgreSQL memutuskan berdasarkan statistik; pada tabel kosong ia memilih Seq Scan karena memang lebih murah. Pembuktian harus dengan data berukuran realistis **dan** setelah `ANALYZE`.

**Kenapa OFFSET ditolak:** `OFFSET 90000` memaksa Postgres membaca 90.020 baris index lalu membuang 90.000. Cursor `id < $2` langsung melompat ke posisi lewat index, jadi halaman ke-1 dan ke-5000 sama cepatnya.

### Bukti
```
Index Scan using idx_entries_account_id_desc on entries e  (actual rows=20)
Index Scan using transactions_pkey on transactions t        (loops=20)
Execution Time: 0.489 ms   ← 200.000 baris, dengan JOIN
```

### Jebakan
`INSERT ... SELECT 'DEBIT'` gagal: kolom bertipe enum `entry_direction`, literal teks di `UNION ALL` harus di-cast eksplisit (`'DEBIT'::entry_direction`). Di `INSERT ... VALUES` biasa Postgres meng-cast otomatis; di `SELECT`/`UNION` tidak.

---

## Sesi 19 — Test konkurensi: T-04, T-05, T-07, T-08/T-09 (2026-09-16)

### Apa
`test/integration/concurrency_test.go`, dijalankan dengan `-race`, T-04 dengan `-count=5`.

| Test | Skenario | Hasil |
|---|---|---|
| T-04 | 100 goroutine transfer Rp 50.000 + fee dari saldo Rp 1.000.000, key berbeda | **19** sukses, 81 saldo kurang, sisa Rp 31.000 |
| T-05 | 10 goroutine, key SAMA | **1** transaksi, 9 in-flight, saldo naik sekali |
| T-07 | 50× A→B dan 50× B→A bersamaan | 100 sukses, **0** deadlock |
| T-08/09 | 1.000 transaksi acak, 8 worker, 5 dompet | 947 sukses, 53 ditolak saldo, trial balance 0, drift 0 |

### Kenapa
**Pola test konkurensi:** N goroutine → `sync.WaitGroup` → hitung hasil dengan `atomic.Int64` → error tak terduga dikirim lewat channel berkapasitas N (bukan `t.Errorf` dari goroutine, yang tidak aman setelah test selesai) → assert.

**Invariant diperiksa PERTAMA, sebelum jumlah sukses.** Berapa yang sukses adalah properti implementasi; uang tidak tercipta/hilang adalah properti kebenaran. Kalau invariant gagal, itulah judul beritanya.

**Key berbeda per goroutine di T-04, key sama di T-05.** Yang pertama menguji konkurensi saldo; yang kedua menguji idempotency. Mencampurnya membuat kegagalan sulit didiagnosis.

**`-count=5` untuk T-04.** Bug konkurensi sering lolos sekali dan gagal di percobaan keempat. Sekali lulus belum berarti benar.

**Kenapa T-08 memakai `rand.NewPCG(seed)` per worker:** hasil bisa direproduksi kalau gagal — sebutkan seed-nya, jalankan lagi, dapat urutan yang sama.

### Contoh — kerangka yang bisa dipakai ulang
```go
var wg sync.WaitGroup
var okCount atomic.Int64
errCh := make(chan error, workers)
for i := 0; i < workers; i++ {
    wg.Add(1)
    go func() {
        defer wg.Done()
        _, err := repo.Post(ctx, txn, newClaim(userID))
        switch {
        case err == nil:                                   okCount.Add(1)
        case errors.Is(err, domain.ErrInsufficientBalance): // kegagalan yang sah
        default:                                            errCh <- err
        }
    }()
}
wg.Wait(); close(errCh)
for err := range errCh { t.Errorf("error tak terduga: %v", err) }
assertAllInvariants(t)
```

---

## Sesi 20 — Eksperimen: lepas kuncinya, lihat apa yang rusak (2026-09-16)

### Apa
Dua varian `ledger_repo.go` diuji terhadap T-04 dan T-07, lalu kode dipulihkan dengan `git checkout`. Transkrip: `docs/evidence/eksperimen-kunci.md`.

| Varian | T-04 (jawaban benar: 19) | T-07 (jawaban benar: 0 deadlock) | Uang utuh? |
|---|---|---|---|
| **Asli** (FOR UPDATE + version) | 19 sukses, 81 saldo kurang | 100 sukses, 0 deadlock | ✅ |
| **A** tanpa FOR UPDATE, version tetap | **8** sukses, **92 konflik**, sisa Rp 592.000 | **98 deadlock** | ✅ (592.000 = 1.000.000 − 8×51.000) |
| **B** tanpa FOR UPDATE dan tanpa version | 19 sukses, 81 saldo kurang | **99 deadlock** | ✅ |

### Kenapa hasilnya begini — ini bagian yang layak diceritakan di wawancara
**Dugaan awal (dari dokumen 05):** melepas `version` akan memunculkan *lost update* dan saldo yang tidak masuk akal. **Kenyataannya tidak**, dan alasannya penting:

1. **`UPDATE accounts SET balance = balance + $1`** adalah update **relatif**. PostgreSQL mengunci baris saat UPDATE dan menghitung `balance + delta` dari nilai **terbaru** yang sudah di-commit, bukan dari nilai yang dibaca Go. Lost update klasik hanya terjadi pada pola *baca → hitung di aplikasi → tulis nilai absolut* (`SET balance = 949000`). Kode kita tidak pernah menulis nilai absolut.
2. **`CHECK (balance >= 0)`** menolak transfer ke-20 dan seterusnya di level database, meskipun pre-check di Go memakai saldo basi. Ini jaring pengaman terakhir yang bekerja persis seperti dirancang.
3. Jadi apa gunanya `version`? Pada varian A ia **menolak 92 permintaan** yang sebenarnya bisa sukses — karena tanpa `FOR UPDATE`, semua goroutine membaca `version` yang sama lalu 91 di antaranya kalah. Optimistic lock menjamin **kebenaran**, tetapi buruk untuk **throughput** pada kontensi tinggi. Itulah mengapa kode asli memakai `FOR UPDATE` (pesimistik) sebagai kunci utama, dan `version` hanya sebagai sabuk pengaman kedua.
4. Yang **benar-benar rusak** tanpa `FOR UPDATE ... ORDER BY id` adalah **deadlock** (98–99 dari 100 transfer silang). Tanpa `SELECT ... FOR UPDATE` terurut, kunci baris diambil oleh statement `UPDATE` sesuai urutan entry: A→B mengunci A lalu B, B→A mengunci B lalu A → saling tunggu → PostgreSQL membunuh salah satunya. Satu klausa `ORDER BY id` menghilangkan seluruh kelas bug ini.

**Pelajaran yang lebih besar:** pertahanan berlapis bekerja. Melepas dua lapis sekaligus pun uang tetap utuh, karena lapis ketiga (update relatif + CHECK) masih berdiri. Tetapi sistemnya menjadi **tidak dapat dipakai** (deadlock) — benar tetapi tidak berguna adalah kegagalan juga.

### Contoh — cara mereproduksi lost update sungguhan (jangan di-commit)
```go
// pola SALAH: baca, hitung di Go, tulis absolut
var bal int64
tx.QueryRow(ctx, `SELECT balance FROM accounts WHERE id=$1`, id).Scan(&bal)
tx.Exec(ctx, `UPDATE accounts SET balance = $1 WHERE id = $2`, bal+delta, id)   // ← 100 goroutine: banyak yang hilang
```
Tanpa `FOR UPDATE` dan tanpa `version`, pola ini akan menghasilkan saldo penerima yang lebih kecil dari 19×50.000. Kode NusaLedger sengaja tidak pernah ditulis begini.

### Bukti
`docs/evidence/eksperimen-kunci.md`; setelah `git checkout`, T-04 kembali 19 sukses dan `git diff` kosong.

---

## Sesi 18 & 22 — Service: use case ledger dan auth (2026-09-16)

### Apa
- `service/ports.go`: enam interface kecil yang **dibutuhkan** service (`LedgerStore`, `AccountStore`, `IdempotencyStore`, `UserStore`, `RefreshTokenStore`, `PasswordHasher`, `AccessTokenIssuer`).
- `service/ledger.go`: `Topup`, `Withdraw`, `Transfer`, `Reverse`, `GetTransaction`, `MyWallet`, `MyEntries`, `TrialBalance`; semua operasi uang lewat satu jalur `postIdempotent`.
- `service/auth.go`: `Register`, `Login`, `Refresh` (rotasi), `Logout`.
- `service/cursor.go`: cursor = base64url(id) (K-04).
- `platform/token/jwt.go` (HS256, `WithValidMethods`), `platform/password/argon2.go` (argon2id, format PHC).
- `repository/postgres/user_repo.go` (`CreateWithWallet` satu transaksi), `token_repo.go`.
- Unit test service dengan **stub** (tanpa database): 84 % coverage.

### Kenapa
**Interface didefinisikan di package `service`, bukan `repository`.** Interface adalah kebutuhan pemakainya. Di sisi repository ia cenderung membengkak jadi 20 method "siapa tahu perlu". Di sisi service, tiap interface hanya 2–4 method → stub untuk test jadi 15 baris, dan test berjalan dalam mikrodetik.

**Service tidak tahu HTTP.** Ia mengembalikan `domain.ErrInsufficientBalance`, bukan `422`. Di Fase 2, worker queue memanggil service yang sama tanpa konsep status code.

**Alur idempotency ada di satu fungsi (`postIdempotent`)**, dipakai semua operasi uang:
1. `Find` → ada + hash sama → kembalikan hasil lama; hash beda → `ErrIdempotencyConflict`; masih diproses → `ErrIdempotencyInFlight`.
2. Belum ada → susun transaksi (builder domain) → `Validate()` → `Post`.
3. `Post` mengembalikan in-flight (kalah balapan dengan permintaan kembar) → cek lagi; kalau pemenang sudah selesai, kembalikan hasilnya. Klien tidak perlu tahu ada balapan.

**Id akun sistem dimuat sekali di `NewLedger`.** Partial unique index menjamin tepat satu akun per jenis, jadi aman di-cache selama proses hidup; satu query per transfer terhemat.

**Login anti-enumerasi:** email tidak ada → tetap jalankan `Verify` dengan *dummy hash* supaya durasinya sama dengan password salah, dan pesan errornya identik. Tanpa ini, penyerang bisa memetakan daftar email dari perbedaan waktu respons.

**Refresh token dirotasi**: setiap `Refresh` mencabut token lama dan menerbitkan yang baru. Token yang dicuri hanya berguna sampai pemilik aslinya me-refresh — dan saat itu pencurian terdeteksi (token lama ditolak).

**`CreateWithWallet` satu transaksi database.** Tidak boleh ada pengguna tanpa dompet. Dua statement terpisah berarti crash di antaranya menciptakan pengguna yang tidak bisa bertransaksi.

**JWT: `WithValidMethods([HS256])`** menutup `alg: none` dan *algorithm confusion*; `WithIssuer`, `WithExpirationRequired` memastikan token tanpa `exp` ditolak. `Subject` = `public_id`, bukan id internal.

**argon2id dengan parameter tersimpan di hash** (format PHC `$argon2id$v=19$m=…,t=…,p=…$salt$hash`): parameter bisa dinaikkan nanti tanpa mematahkan hash lama; `subtle.ConstantTimeCompare` mencegah timing attack.

### Contoh — stub kecil untuk unit test service
```go
type stubIdem struct{ rec *domain.IdempotencyRecord }
func (s *stubIdem) Find(context.Context, int64, string) (*domain.IdempotencyRecord, error) {
    if s.rec == nil { return nil, domain.ErrNotFound }
    return s.rec, nil
}
// test: idem.rec = &domain.IdempotencyRecord{Claim: ...RequestHash: "LAIN"}, ... → mau ErrIdempotencyConflict, Post tidak dipanggil
```

### Bukti
```
ok  internal/service            coverage: 84.2%   (≥ 80 % DoD)
ok  internal/platform/token     coverage: 88.0%   (alg none, payload diubah, secret beda, kedaluwarsa → ditolak)
ok  internal/platform/password  coverage: 83.3%
```

---

## Sesi 23–24 & 27 — HTTP, main, observability, graceful shutdown (2026-09-16)

### Apa
- `transport/http/response.go`: envelope `{"data"}` / `{"error"}` dan **satu** fungsi `mapError` (error domain → kode + status).
- `dto.go`: request/response terpisah dari domain; `decodeJSON` dengan `MaxBytesReader` 1 MB + `DisallowUnknownFields`.
- `middleware.go`: `requestID`, `recoverer`, `logAndMeasure` (log JSON + metrik per route berpola), `authenticate` (Bearer JWT → `Actor` di context), `requireAdmin`, `rateLimitByActor`, `idempotency` (header wajib + SHA-256 body).
- `handler_auth.go`, `handler_ledger.go`, `router.go` (chi).
- `platform/metrics`: 8 metrik docs/03 §8; `platform/ratelimit`: jendela tetap in-memory (F-04).
- `app/server.go`: `Serve` (graceful shutdown) + `RunPeriodic` (job drift); `app/server_test.go` = **T-13**.
- `cmd/api/main.go`: DI manual dari bawah ke atas; pprof di `127.0.0.1:6060` hanya non-produksi; job verifikasi ledger tiap 60 s.

### Kenapa
**Handler tipis, tiga tugas saja:** parse, panggil service, terjemahkan error. Semua yang berisiko ada di service/domain yang bisa diuji tanpa server HTTP.

**`mapError` adalah satu-satunya tempat status HTTP ditentukan.** Kalau tersebar di tiap handler, satu error baru berarti mengubah 10 file dan pasti ada yang terlewat → klien menerima 500 untuk kasus yang seharusnya 422. Pesan 500 tidak pernah memuat detail internal; detailnya ke log dengan `request_id`.

**Kenapa `Idempotency-Key` dibaca di middleware dan body di-hash di sana:** body hanya bisa dibaca sekali. Middleware membacanya, menghitung SHA-256, lalu **mengembalikannya** ke `r.Body` supaya handler tetap bisa men-decode. Hash mencakup method + path + body: key yang sama dipakai untuk endpoint berbeda otomatis konflik (409).

**`X-Request-ID` selalu dibuat server, bukan diambil dari klien.** Klien yang bisa memilih request id bisa mengacaukan penelusuran log (dua request dengan id sama).

**`middleware.RealIP` sengaja TIDAK dipakai** (linter menandainya deprecated dengan CVE spoofing): ia mempercayai `X-Forwarded-For` dari siapa pun, sehingga rate limit per IP bisa dilewati hanya dengan mengirim header palsu. Tanpa proxy tepercaya, `RemoteAddr` adalah satu-satunya sumber yang jujur.

**Recover middleware di posisi paling luar**, supaya panic di middleware lain pun tertangkap. Batasannya: `recover()` hanya menangkap panic di goroutine yang sama; goroutine yang dibuat handler butuh recovery sendiri.

**Liveness dan readiness dipisah.** `/healthz` tidak menyentuh DB — kalau ikut mengecek DB, database yang lambat 3 detik membuat orchestrator me-restart semua pod dan memperparah beban. `/readyz` mengecek DB **dan** saklar `Readiness`: saat SIGTERM datang, saklar dimatikan dulu supaya load balancer berhenti mengirim trafik baru, baru `Shutdown` menunggu request yang sedang jalan.

**Job drift ada di `cmd/api` (F-05).** Tiap 60 detik membandingkan `accounts.balance` dengan Σ entries dan trial balance, lalu menulis metrik `ledger_balance_drift_total`. Nilai selain 0 = alert keras, bukan "lihat besok pagi".

**pprof di mux terpisah, alamat loopback, hanya non-produksi.** Mengimpor `net/http/pprof` biasa mendaftarkan handler ke `DefaultServeMux`; kalau server utama memakainya, `/debug/pprof` terbuka ke internet.

**Rate limit fail-closed:** key kosong selalu ditolak. Lebih baik menolak satu request sah daripada membuka pintu saat ada bug.

### Contoh — urutan middleware dan artinya
```
recoverer → requestID → logAndMeasure → Timeout(30s) → NoCache
  └─ /auth/*            (tanpa auth)
  └─ authenticate → /accounts/me, /transactions/*
        └─ idempotency → topup / withdraw
        └─ idempotency + rateLimitByActor → transfer
        └─ requireAdmin + idempotency → {id}/reverse
        └─ requireAdmin → /internal/ledger/trial-balance
```

### Bukti
- T-13 (`app/server_test.go`): request 1,5 detik tetap selesai 200 saat `cancel()` dipanggil di detik 0,3; `Serve` kembali nil; koneksi baru ditolak.
- E2E HTTP (`test/integration/http_test.go`): register/login/refresh/logout, 400 tanpa key, replay **byte-identik**, 409 conflict, 422 saldo/self/range, 404 IDOR (T-11), 403 non-admin, reversal 201 lalu 409, 413 body 2 MB, 429 rate limit, trial balance seimbang, tidak ada `account_id` internal yang bocor.

### Jebakan
1. Linter `noctx`/`contextcheck`: `net.Listen` → `(&net.ListenConfig{}).Listen(ctx, ...)`, `http.Get` → `NewRequestWithContext` + `Client.Do`. Bukan kosmetik: request tanpa context tidak bisa dibatalkan.
2. `bodyclose`: response yang error pun bisa punya body; tutup sebelum `t.Fatal`.
3. Di test E2E, `?cursor=%%%` tidak pernah sampai ke handler: parser URL Go membuang pasangan query yang percent-encoding-nya rusak. Uji cursor rusak harus memakai nilai yang valid sebagai URL tapi bukan base64url.
4. Counter Prometheus berlabel (`http_requests_total{...}`) baru muncul di `/metrics` setelah kombinasi labelnya pernah terjadi; gauge muncul sejak awal. Test yang memeriksa `/metrics` harus memicu satu request dulu.

---

## Sesi 28 — Dockerfile, compose lengkap, CI (2026-09-16)

### Apa
- `Dockerfile` multi-stage: `golang:1.27-alpine` → `gcr.io/distroless/static-debian12:nonroot`. Hasil: **18,6 MB**.
- `docker-compose.yml`: service `api` (build dari Dockerfile, `RUN_MIGRATIONS=true`, `depends_on: postgres: condition: service_healthy`) + `postgres`.
- `.dockerignore`, `.github/workflows/ci.yml` (lint+govulncheck → unit+coverage gate → integration+T-04×3 → build image < 20 MB).
- Smoke test dari nol: `docker compose down -v && docker compose up -d` → `/readyz` 200 dalam ~4 detik → register/login/topup/transfer lewat curl. Transkrip: `docs/evidence/smoke-compose.md`.

### Kenapa
**Setiap baris Dockerfile ada alasannya:** `COPY go.mod go.sum` terpisah supaya layer dependency tidak batal saat kode berubah; `--mount=type=cache` menyimpan modul & build cache antar build; `CGO_ENABLED=0` menghasilkan binary statis yang jalan tanpa libc; `-trimpath` membuang path absolut mesin build; `-ldflags "-s -w"` membuang symbol table; `distroless/static` tidak punya shell maupun package manager — kalaupun penyerang bisa mengeksekusi kode, tidak ada `sh` untuk dipanggil; `USER nonroot`.

**Migrasi dijalankan API saat start hanya di compose (`RUN_MIGRATIONS=true`).** Di produksi, migrasi adalah langkah deploy terpisah yang di-review, bukan efek samping start aplikasi (dua instance yang start bersamaan bisa berebut).

**`depends_on: condition: service_healthy`**, bukan `sleep 5`. Healthcheck `pg_isready` adalah kebenaran; `sleep` adalah tebakan yang suatu hari salah.

**CI mengulang T-04 tiga kali** dan menolak image di atas 20 MB serta coverage domain < 90 % — Definition of Done ditegakkan mesin, bukan diingat manusia.

### Jebakan (dan pola yang berulang)
Port **8080** di host sudah dipakai layanan lain (laragon), persis seperti 5432 sebelumnya: request ke `localhost:8080` dijawab "Not found." oleh server yang salah. Gejalanya bukan "connection refused" tetapi jawaban yang tidak masuk akal. Solusi: compose memetakan **8081 → 8080**. Pelajaran umum: kalau jawabannya aneh, tanya dulu *"saya bicara dengan proses yang mana?"* (`netstat -ano | grep :8080`).

### Bukti
`docs/evidence/smoke-compose.md`: image 18,6 MB; readyz 4 detik; 400 tanpa key; retry byte-identik; 409 conflict; 422 di bawah minimum; 401 tanpa token; 404 id acak; metrik `ledger_balance_drift_total 0`; satu baris log JSON dengan `request_id`, `user_id`, `route`, `status`, `duration_ms`.

---

## Sesi 29 — Load test k6: benar dulu, baru cepat (2026-09-16)

### Apa
`test/load/transfer.js` (100 VU, 3 menit, transfer Rp 10.000 antar 50 dompet, key baru tiap iterasi) terhadap `docker compose` dengan override `docker-compose.load.yml`. Setelah selesai: trial balance, drift, fee dihitung ulang. Hasil di `docs/evidence/k6.md`.

### Kenapa
**Load test selalu diakhiri verifikasi ledger.** Angka p95 yang bagus pada sistem yang menciptakan uang tidak berarti apa-apa. Yang diverifikasi: Σdebit = Σkredit, `accounts.balance` = Σ entries, tidak ada dompet negatif, fee terkumpul = jumlah transfer × Rp 1.000 **persis**.

**Override rate limit hanya untuk load test.** Run pertama tanpa override: 99,98 % request ditolak 401/429 — bukan bug, tapi pembatas login (5/15 menit per IP) dan transfer (20/menit per user) yang bekerja sesuai desain. Load test mengukur ledger, jadi pembatas dinaikkan lewat file override yang terpisah dan terdokumentasi, bukan dengan mengubah default.

### Hasil — dan pelajaran terpenting sesi ini
| | Target | Hasil |
|---|---|---|
| Correctness (33.340 transfer) | 100 % | ✅ trial balance 0, drift 0, fee tepat |
| Error rate | < 1 % | ✅ 0 % |
| Throughput | ≥ 200/s | ❌ 177/s |
| p95 / p99 | < 200 / 500 ms | ❌ 515 / 568 ms |

**Kenapa lambat — dan kenapa ini ditulis apa adanya:** setiap transfer mengunci tiga akun: pengirim, penerima, dan `SYSTEM_FEE_REVENUE`. Akun fee itu **sama untuk semua transfer**, jadi `FOR UPDATE` membuat seluruh sistem antre pada satu baris. Throughput maksimum = 1 ÷ (durasi satu transaksi DB termasuk fsync). Di Docker Desktop Windows dengan k6, API, dan Postgres di satu laptop, itu ≈ 180/detik.

Ini bukan bug: ini **konsekuensi desain yang benar untuk correctness** dan baru terlihat saat diukur. Jawaban wawancaranya: *"Saya memilih satu akun fee karena sederhana dan auditable; load test menunjukkan ia jadi baris panas pada 177 tps. Kalau trafik 10×, saya akan memecah fee ke N sub-akun (sharding) atau mengakumulasi fee per periode, karena kontensi tidak bisa diselesaikan dengan index — hanya dengan mengurangi hal yang diperebutkan."*

### Jebakan
1. k6 `http_req_failed` menghitung semua status ≥ 400 sebagai gagal; 422 (saldo kurang) itu sah. Pakai `http.setResponseCallback(http.expectedStatuses(201, 422))`.
2. `setup()` k6 harus **gagal cepat** (`fail()`) kalau login tidak 200; kalau tidak, VU berjalan dengan token kosong dan hasilnya menyesatkan.
3. Token admin untuk verifikasi harus dibuat **setelah** load test dengan login baru — token 15 menit bisa kedaluwarsa.

### Bukti
`docs/evidence/k6.md` + `docs/evidence/k6-summary.json`.

---

## Sesi 26 — G-2: review PostTransaction & konkurensi, model Fable (2026-09-16)

### Apa
Dipanggil lewat Agent tool, `model: "fable"`, tools dibatasi Read/Grep/Glob (read-only), meninjau `ledger_repo.go`, `errors.go`, `service/ledger.go`, `transaction.go`, trigger migration, dan seluruh test konkurensi. Tidak ada file diubah oleh reviewer — semua perbaikan dikerjakan setelahnya oleh subagent terpisah (Sonnet, peran "mengerjakan").

### Kenapa gerbang review terpisah, padahal semua test sudah hijau
**Test membuktikan apa yang Anda pikirkan untuk diuji; review menemukan apa yang tidak Anda pikirkan.** Semua T-01…T-10b sudah hijau sebelum G-2, tetapi review menemukan bug nyata yang tidak tersentuh test manapun: lihat temuan #3 di bawah.

### Hasil
**Kesimpulan pertama (paling penting): tidak ada temuan yang bisa menciptakan/menghilangkan uang atau memicu deadlock.** Semua jalur mutasi saldo berada di bawah `FOR UPDATE ORDER BY id`.

Tujuh temuan, tiga langsung ditindaklanjuti:

1. **[Sedang] Badai key-sama menahan koneksi pool.** `INSERT ... ON CONFLICT DO NOTHING` membuat request kembar menunggu baris uncommitted pemenang. N request kembar = N-1 koneksi tertahan. Diterima sebagai keterbatasan Fase 1 (dicatat, tidak diperbaiki — perbaikannya butuh mekanisme coalescing yang di luar scope).
2. **[Sedang] Tidak ada test reversal konkuren** → ditutup dengan **T-10c**.
3. **[Rendah-sedang, BUG NYATA] Kalah balapan idempotency + hash beda → error salah.** `postIdempotent` membuang hasil `replay()` saat kalah balapan dan selalu mengembalikan `ErrIdempotencyInFlight`, padahal replay bisa mengembalikan `ErrIdempotencyConflict` (body beda). Klien akan retry selamanya menerima "coba lagi" yang tidak pernah menjadi benar. **Diperbaiki**: `return res, err2` apa adanya dari `replay()`.
4. **[Rendah]** `Validate()` tidak menegakkan `Type==REVERSAL ⇔ ReversesID≠nil` → **diperbaiki** (sentinel `ErrInvalidReversalLink` baru).
5. **[Rendah]** `translate()` tidak memetakan `chk_normal_balance`, `chk_owner`, deadlock (40P01), serialization (40001) → **diperbaiki** (dipetakan; lihat catatan penamaan di Sesi 30/G-3 di bawah).
6. **[Rendah]** T-04 tidak mengassert `conflict==0` secara eksplisit → **ditambahkan**.
7. **[Rendah]** Pre-check saldo `acc.Balance+int64(delta)` rawan overflow int64 → **diperbaiki** memakai `domain.Money.Add()`.

### Kenapa ini bukti nilai model termahal di titik yang tepat
Model murah (Sonnet) menghabiskan waktunya MENGERJAKAN — menulis kode sesuai spesifikasi yang sudah jelas. Model termahal (Fable) dipakai HANYA untuk membaca dan bertanya "apa yang belum diuji?" pada kode yang paling berisiko. Satu panggilan ini menemukan bug yang lolos dari 20+ test yang sudah ditulis sebelumnya — persis alasan §4 plan menaruh gerbang ini sebelum rilis, bukan menggantikan test.

### Bukti
Perbaikan diverifikasi `-race` penuh (unit + integration, T-04 ×3): semua hijau, tidak ada regresi. Commit `fix(g2): ...`.

---

## Sesi 30 — G-3 (release gate) dan penutupan Fase 1 (2026-09-16)

### Apa
G-3 dipanggil sama seperti G-2 (model Fable, read-only), tapi cakupannya lebih luas: Definition of Done PRD §6 dicocokkan satu-satu dengan artefak nyata di repo, kode hasil perbaikan G-2 ditinjau ulang, dan seluruh dokumen (README, HANDOVER, JURNAL, plan) dicek konsistensinya dengan kode yang sebenarnya.

### Hasil
**Kesimpulan: YA-DENGAN-CATATAN.** Tidak ada temuan kode kritis. Yang ditindaklanjuti sebelum tag:

1. **DoD tanpa bukti penuh** (empat kotak, semua sudah jujur tercatat sebagai keterbatasan, bukan disembunyikan): SLO k6 (sudah ditulis gagal apa adanya), T-13 memakai simulasi context bukan sinyal OS sungguhan, replay idempotency hanya diuji 1× bukan 10× serial, `request_id` hanya mengalir di transport bukan "seluruh lapisan". README diperbaiki agar klaimnya presisi.
2. **[Medium, ditindaklanjuti] `/auth/register` tanpa rate limit.** argon2id (64 MiB, t=3) per panggilan; tanpa pembatas, registrasi anonim berulang menghabiskan CPU/memori server. **Diperbaiki**: `rateLimitByIP` baru (`middleware.go`), diterapkan di route register, dikonfigurasi lewat `REGISTER_RATE_LIMIT`/`REGISTER_RATE_WINDOW` (default 10/15 menit), diuji `TestHTTP_RegisterRateLimit`.
3. **[Rendah, diterima]** Limiter login per email bisa dipakai mengunci akun korban 15 menit — trade-off yang sudah disadari sejak spesifikasi (docs/03 §6), dicatat eksplisit di README.
4. **[Rendah, diterima untuk Fase 1]** `/metrics` tanpa auth di port publik — dibatasi di jaringan saat deploy sungguhan, bukan di kode Fase 1.
5. **[Rendah, dijadwalkan Fase 1.1]** `translate()` memetakan constraint `chk_normal_balance`/`chk_owner` (tabel `accounts`) ke sentinel `ErrInvalidReversalLink` yang namanya untuk reversal. Benar secara perilaku HTTP (400), salah secara penamaan. Dicatat di README, tidak menghalangi rilis.

### Kenapa rate limit register layak menghentikan rilis padahal "hanya" temuan Medium
**Biaya perbaikannya jauh lebih kecil daripada biaya insidennya.** Menambah satu middleware + satu field config + satu test adalah kerja setengah jam. Kalau dibiarkan dan seseorang menemukan endpoint publik yang memanggil fungsi hash mahal tanpa batas, itu bisa jadi laporan "denial of service" yang memalukan di portofolio yang justru mengaku paham keamanan (docs/03 §6 sudah menulis daftar rate limit lengkap — register yang terlewat adalah inkonsistensi, bukan keputusan sadar).

### Bukti
`TestHTTP_RegisterRateLimit` PASS; seluruh suite unit + integration `-race` tetap hijau setelah perubahan. Commit `fix(g3): ...`.

---

## Sesi 31 — Audit keamanan penuh (setelah tag v1.0.0), model Opus (2026-09-16)

### Apa
Diminta secara eksplisit: audit keamanan **penuh** atas seluruh repo — berbeda dari G-3 (yang fokusnya kesiapan rilis vs Definition of Done) dan dari skill bawaan `/security-review` (yang hanya mereview *diff*; karena semua sudah ter-commit, diff-nya kosong dan skill itu tidak menemukan apa-apa — bukan karena repo bersih, tapi karena tidak ada yang dibandingkan).

Dua lapis:
1. **Mekanis, sesi utama:** grep pola secret/API key di seluruh working tree; `git log --all` untuk cek `.env` pernah ter-commit; **deep-scan 164 objek git di SEMUA commit** (bukan hanya HEAD) untuk pola token dikenal (`sk-ant-`, `ghp_`, `AKIA`, PEM private key); `govulncheck -show verbose`; cek permission workflow CI; cek Dockerfile/compose.
2. **Kode, subagent Opus, read-only:** autentikasi, otorisasi/IDOR, injection, kebocoran informasi, konfigurasi, Docker runtime, rate limiting — tujuh kategori (A–G), masing-masing WAJIB melaporkan "tidak ada temuan" secara eksplisit bila bersih, supaya tidak ada kategori yang diam-diam terlewat.

### Kenapa dua lapis, dan kenapa deep-scan SELURUH riwayat git
**Grep di working tree saja tidak cukup** — kalau sebuah secret pernah di-commit lalu "dihapus" di commit berikutnya, ia tetap ada selamanya di riwayat git (`git log` menyimpan setiap versi, `rm` tidak menghapus blob lama). Repo publik yang pernah bocor secret harus **rotate kredensial itu**, tidak bisa cukup menghapus filenya. Karena itu deep-scan memakai `git rev-list --objects --all` + `cat-file` per blob, bukan `grep -r` yang hanya melihat working tree saat ini.

**Kenapa model Opus, bukan Fable, untuk audit ini:** ini BUKAN salah satu dari tiga gerbang G-1/G-2/G-3 yang dikunci di docs/06 §4 — itu jenis review baru. Per aturan §3.1 ("Opus untuk memutuskan, Fable hanya di tiga gerbang"), audit umum yang membutuhkan penilaian tapi bukan gerbang rilis final memakai model kedua-termahal, bukan yang paling mahal. Menghemat ~separuh biaya per token tanpa mengorbankan kualitas audit — kategori read-only + checklist detail sudah cukup menuntun model manapun yang cukup mampu.

### Hasil
0 Critical, **1 High**, 3 Medium, 9 Low, 4 Info. Yang ditutup:

1. **[High] Kebocoran saldo lintas pengguna.** Query header transaksi difilter kepemilikan dengan benar (`EXISTS entries WHERE account_id = visibleTo`), tapi query ENTRIES di baris berikutnya **tidak difilter sama sekali** — dan bug yang sama ada di respons *langsung* setiap topup/transfer/withdraw (bukan hanya saat melihat detail belakangan). Skenario: penerima transfer melihat saldo pengirim; siapa pun yang topup melihat saldo kumulatif `SYSTEM_CASH`; transfer apa pun membocorkan saldo `SYSTEM_FEE_REVENUE` — membatalkan pembatasan admin-only pada trial balance.

   **Perbaikan:** `domain.PostResult` mendapat field baru `ViewerAccountID *int64` — anotasi presentasi (bukan kebenaran domain), diisi oleh service (Topup/Withdraw/Transfer memindahkan resolusi wallet ke LUAR closure supaya id-nya tersedia setelah `postIdempotent` kembali; GetTransaction memakai wallet id yang sudah dihitung). dto.go merender `balance_after_sen` sebagai **pointer** (`*int64`, `omitempty`) — `null` untuk entry bukan milik pemanggil, bukan `0`, supaya "tersembunyi" tidak pernah tertukar dengan "saldo nol". Aggregat `amount_sen`/`fee_sen` tetap dihitung dari SEMUA entry (tidak berubah) — hanya `balance_after` yang diredaksi, sehingga tidak ada regresi UX pada informasi yang memang boleh diketahui.

2. **[Medium]** JWT secret ≥32 byte kini ditegakkan **tanpa syarat** `APP_ENV` (sebelumnya, lupa menyetel `APP_ENV=production` diam-diam meloloskan secret 16 byte). Cukup naikkan satu angka di `NewJWT`, tidak breaking karena semua secret dev/test sudah ≥33 byte.
3. **[Medium]** Port dev Postgres (5433) dan API (8081) diikat `127.0.0.1`, bukan `0.0.0.0` default Docker — sebelumnya terjangkau siapa pun di LAN/Wi-Fi yang sama dengan password dev yang sama untuk semua orang yang *clone* repo ini.
4. **[Low]** `reverseRequest` tidak punya `validate()` — deskripsi >255 karakter lolos sampai `Transaction.Validate()` lalu jatuh ke 500 generik karena `ErrDescriptionTooLong` tidak dipetakan di `mapError`. Ditambahkan validasi di DTO, konsisten dengan request lain.
5. **[Low, murah, "tidak ada ruginya"]** Header keamanan HTTP (`X-Content-Type-Options`, CSP, `X-Frame-Options`, dll.) dipasang di **root router** (bukan hanya `/api/v1`) supaya `/healthz`/`/readyz`/`/metrics` ikut terlindungi. `Strict-Transport-Security` hanya dikirim di produksi.

**Diterima, tidak ditutup** (9 Low + 4 Info, semua didokumentasikan di README): `/metrics` publik (pembatasannya benar di jaringan, bukan kode — menggerbanginya di kode akan merusak model *scraping* Prometheus standar), trade-off rate-limit-per-akun (sudah dispesifikasikan), beberapa endpoint tanpa rate limit khusus, refresh-token reuse detection, orakel enumerasi via `409 EMAIL_TAKEN`, penamaan sentinel `ErrInvalidReversalLink` yang dipakai ulang untuk constraint `accounts`.

### Kenapa memilih menutup High SEKARANG, bukan sekadar melaporkannya
**Repo ini sudah publik.** Sebuah temuan High yang genuinely bisa dieksploitasi (bukan teoretis) pada repo yang sudah live berbeda dari temuan pada kode yang belum dirilis — menunggu izin eksplisit untuk menutup kebocoran data yang sudah bisa diakses siapa pun hari ini bukan sikap yang bertanggung jawab. Prinsip yang sama dipakai saat menutup temuan G-2/G-3 sebelumnya: kalau perbaikannya murah, spesifikasinya jelas dari hasil review, dan risikonya nyata — kerjakan, verifikasi dengan test, lalu laporkan apa yang dilakukan dan kenapa.

### Contoh — kenapa `*int64` bukan `int64` untuk field yang bisa "disembunyikan"
```go
type entryResponse struct {
    // ...
    BalanceAfterSen *int64 `json:"balance_after_sen,omitempty"`
}
```
Kalau memakai `int64` biasa dan menulis `0` untuk "disembunyikan", klien tidak bisa membedakan "saldo akun ini memang nol" dari "saya tidak boleh melihat saldo akun ini" — dua makna yang SANGAT berbeda untuk sistem uang. Pointer + `omitempty` membuat field itu hilang total dari JSON (bukan `0`), dan Go `nil` tidak bisa disalahartikan sebagai nilai.

### Bukti
`TestHTTP_B1_SaldoPihakLainTidakBocor` — membuktikan: pengirim topup/transfer hanya melihat saldo sendiri (1 dari N akun terlibat); penerima yang melihat transaksi lewat GET juga hanya melihat saldo sendiri; ADMIN tetap melihat semua; `amount_sen`/`fee_sen` tetap benar di semua sudut pandang. Seluruh suite `-race` (unit + integration, T-04 ×3) tetap hijau setelah perbaikan. `docker compose config` valid setelah port diikat ulang.

---

## Sesi 32 — Fase 1.1: menuntaskan tiga item susulan dari HANDOVER (2026-09-16)

### Apa
Permintaan: "lanjutkan tasklist yang belum sesuai plan". `HANDOVER.md` §Berikutnya mencatat empat item: tag `v1.0.1` (sudah dikerjakan sesi sebelumnya), tiga item Fase 1.1 (belum), sharding fee (butuh Linux native), dan Fase 2 (sengaja di luar cakupan Fase 1, docs/01 §1.3 — bukan "belum", tapi "tidak sekarang"). Dikerjakan tiga item Fase 1.1:

1. **`ErrIntegrityViolation`** — sentinel baru di domain/errors.go, dipisah dari `ErrInvalidReversalLink`. `translate()` (repository/postgres/errors.go) sekarang: `chk_reversal_link` → `ErrInvalidReversalLink` (memang soal reversal); `chk_normal_balance`, `chk_owner` → `ErrIntegrityViolation` (soal `accounts`, bukan reversal).
2. **Rate limit refresh & topup/withdraw** — `rateLimitByIP` dipasang di `/auth/refresh` (limiter baru `RefreshLimiter`); `rateLimitByActor` dipasang di `topup` dan `withdraw` dengan **satu** limiter (`MoneyLimiter`) yang dipakai BERSAMA oleh keduanya.
3. **Refresh-token reuse detection** — `RefreshTokenStore.RevokeAllForUser(ctx, userID)` baru. `Auth.Refresh` memeriksa `rt.RevokedAt != nil` sebelum `Usable()`: kalau token yang diajukan sudah pernah dicabut tapi dipakai lagi, cabut SEMUA sesi user itu.

### Kenapa topup dan withdraw berbagi SATU limiter, bukan dua terpisah
Awalnya sempat dipertimbangkan menggabungkannya dengan `TransferLimiter` yang sudah ada (`transfer` sudah punya limiter sendiri) — ditolak, karena itu akan diam-diam mengubah budget efektif `transfer` (tiga operasi memperebutkan satu kuota yang tadinya dirancang untuk satu operasi). Membuat limiter TERPISAH untuk topup dan withdraw masing-masing juga berlebihan: keduanya operasi tulis satu-akun yang setara risikonya (kunci satu baris + satu baris sistem), jadi wajar berbagi satu kuota. Satu limiter baru (`MoneyLimiter`) adalah titik tengah yang tepat: menambah proteksi tanpa mengubah perilaku `transfer` yang sudah ada dan sudah diuji.

### Kenapa reuse detection TIDAK butuh migrasi skema
Desain "token family" yang lengkap (kolom `family_id`, melacak silsilah rotasi) adalah cara yang lebih presisi, tapi butuh migrasi baru dan mengubah bentuk `Store()`. Insight yang membuat versi tanpa migrasi tetap benar: `Revoke()` **tidak menghapus baris**, hanya mengisi `revoked_at`. Itu berarti baris token yang sudah dirotasi TETAP ADA di tabel, dan `Find()` masih menemukannya. Jadi "token ini pernah ada, dan `revoked_at` terisi" SAMA PERSIS dengan "token ini sudah dirotasi sebelumnya" — sinyal reuse tanpa perlu kolom baru apa pun. Responsnya sengaja "nuke semua sesi" (bukan hanya menolak permintaan), karena begitu satu token dalam rantai rotasi terbukti bocor, tidak ada cara mengetahui token MANA dalam rantai itu yang masih dipegang penyerang — mencabut semuanya adalah satu-satunya respons yang aman.

### Contoh — kenapa urutan pengecekan penting
```go
if rt.RevokedAt != nil {           // reuse — HARUS diperiksa DULU
    a.tokens.RevokeAllForUser(ctx, rt.UserID)
    return nil, domain.ErrInvalidToken
}
if !rt.Usable(a.now()) {           // kedaluwarsa biasa — baru diperiksa SETELAHNYA
    return nil, domain.ErrInvalidToken
}
```
`Usable()` sudah mengembalikan `false` untuk token yang dicabut ATAU kedaluwarsa — kalau urutan dibalik, reuse yang sudah lama (jadi juga sudah lewat masa berlakunya) akan salah terklasifikasi sebagai "kedaluwarsa biasa" dan tidak pernah memicu pencabutan seluruh sesi. Klasifikasi HARUS lebih spesifik dulu (revoked = reuse) sebelum klasifikasi umum (tidak usable).

### Bukti
```
TestHTTP_RefreshRateLimit, TestHTTP_MoneyRateLimit         PASS
TestRefresh_ReuseTerdeteksi_CabutSemuaSesi                 PASS (sesi lain yang tidak terlibat pun ikut tercabut)
go test -race (-count=1) seluruh internal/...              hijau
go test -tags=integration -race (-count=1) seluruh test/...  hijau
T-04 -race -count=3                                        hijau (tidak ada regresi pada jalur uang)
golangci-lint run                                          0 issues
```

---

## Sesi 33 — Sharding akun fee: item performa terakhir (2026-09-16)

### Apa
Diminta lagi "lanjutkan task yang belum selesai sesuai plan" — satu-satunya item Fase 1 yang masih tersisa. Sebelumnya dilewati dengan alasan "butuh Linux native"; dikerjakan ulang setelah menyadari perbandingan RELATIF (sebelum vs setelah, lingkungan IDENTIK) tetap sah untuk mengisolasi efek satu perubahan, walau bukan pengukuran produksi Linux yang sesungguhnya.

Urutan kerja:
1. **Cek dulu blast radius sebelum menulis kode** — grep semua pemakaian `sysFeeID` di test. Temuan penting: SEMUA test correctness (T-04, T-07, T-08/09, reversal) memanggil `domain.NewTransfer` LANGSUNG di level repository, melewati `service.Ledger.Transfer` sama sekali. Karena sharding diimplementasikan HANYA di service layer (`pickFeeShard()`), test-test itu tetap deterministik dan tidak perlu diubah sama sekali. Ini mengubah keputusan dari "terlalu berisiko" jadi "aman dikerjakan" — analisis dampak SEBELUM menulis kode mengubah kalkulasi risiko sepenuhnya.
2. Migration `000009_fee_shards`: ganti index unik (yang mewajibkan tepat satu baris per JENIS akun sistem) dengan index yang hanya berlaku untuk `SYSTEM_CASH`/`SYSTEM_SUSPENSE`; tambah 7 baris `SYSTEM_FEE_REVENUE` (total 8).
3. `AccountStore.SystemAccounts` (method baru, jamak) → `service.Ledger.feeIDs []int64` → `pickFeeShard()` memilih satu ID secara acak per transfer.
4. `resetDB` test helper diperbarui menanam 8 shard, bukan 1 — kalau tidak, SEMUA test HTTP (yang lewat service layer) diam-diam tetap hanya punya 1 akun fee dan sharding tidak pernah benar-benar teruji.
5. k6 diulang di lingkungan yang sama.

### Kenapa risikonya kecil — dan kenapa itu HARUS diverifikasi, bukan diasumsikan
Poin 1 di atas adalah bagian terpenting sesi ini. Godaan alaminya: "sharding itu perubahan besar, berisiko tinggi, tunda saja." Tapi "besar" dan "berisiko" adalah dua hal berbeda — grep 30 detik membuktikan blast radius sesungguhnya kecil, KARENA desain proyek ini SEJAK AWAL memisahkan "test correctness" (langsung ke domain/repository, deterministik) dari "test alur pengguna" (lewat HTTP/service). Pemisahan lapisan yang dijaga ketat sejak Sesi 1 (transport → service → domain) itulah yang membuat perubahan di SATU lapisan (service) bisa dilakukan dengan percaya diri tanpa merusak lapisan lain. Ini bukan kebetulan — ini bayaran dari disiplin arsitektur yang konsisten sejak awal.

**Kenapa index diganti, bukan sekadar di-drop:** kalau index unik dihapus tanpa pengganti, tidak ada apa pun yang mencegah SYSTEM_CASH atau SYSTEM_SUSPENSE tiba-tiba punya dua baris suatu hari (bug aplikasi, migrasi ceroboh). Index barunya tetap menegakkan "tepat satu" untuk KEDUA akun itu — hanya SYSTEM_FEE_REVENUE yang sengaja dibebaskan. Constraint yang tepat sasaran, bukan constraint yang dihapus.

**Kenapa acak, bukan round-robin atau hash:** transfer-transfer di production adalah request independen tanpa urutan yang berarti — round-robin butuh state bersama (kontensi baru!), hash butuh sesuatu yang stabil untuk di-hash (tidak ada yang alami di sini). Acak murni (`math/rand/v2`) sudah cukup untuk MENGURANGI kontensi; tidak perlu distribusi sempurna, hanya perlu "tidak semua di satu baris".

### Hasil (lingkungan sama, sebelum vs setelah)
| | Sebelum | Setelah |
|---|---|---|
| Throughput | 177 tps | **546 tps** (×3,1) |
| p95 | 515 ms | **210 ms** |
| p99 | 568 ms | **288 ms** (lulus target 500 ms) |
| Transfer/3 menit | 33.340 | **104.333** |

Trial balance tetap 0, drift tetap 0, fee tersebar ke semua 8 shard dengan selisih <2% antar shard — bukan diam-diam tetap satu akun. p95 210 ms masih 10 ms di atas target 200 ms; dianalisis sebagai kemungkinan sisa kontensi di fsync WAL Postgres pada Docker Desktop, bukan lagi akun fee (yang sudah terbukti BUKAN lagi penyebab dominan).

### Contoh — kenapa peningkatannya ~3× bukan ~8× (delapan shard)
```
Setiap transfer MENGUNCI TIGA baris: dompet pengirim, dompet penerima, satu shard fee.
Sharding menghilangkan kontensi pada baris KETIGA saja.
Baris PERTAMA dan KEDUA (dompet asli) tidak bisa di-shard — itu identitas akun sungguhan,
bukan agregat yang boleh dipecah sembarangan.
```
Ini bagian yang bagus dijelaskan saat wawancara: peningkatan tidak linear terhadap jumlah shard karena fee hanya SATU dari tiga sumber kontensi per transfer — memahami MANA baris yang boleh dipecah (agregat, bukan identitas) adalah inti dari keputusan desain ini.

### Bukti
`TestHTTP_FeeSharding` (fee tersebar ke >1 shard, dibuktikan bukan diasumsikan) PASS ×3 dengan `-race`; seluruh 28 test integration PASS; T-04/T-07/T-08-09 PASS ×3 tanpa regresi; migration diuji rollback+reapply di database dev sungguhan (bukan hanya testcontainers). `docs/evidence/k6-sharded.md`.

---

## Sesi 34 — Fase 2 dimulai: outbox relay, RabbitMQ, consumer idempoten (2026-09-16)

### Apa
Fase 1 selesai total (Sesi 33). Diminta "lanjutkan"; setelah klarifikasi singkat, disepakati mulai Fase 2. TIDAK ada PRD Fase 2 di repo ini — docs 00–06 semuanya tentang Fase 1. Dikerjakan dengan **asumsi eksplisit** mengikuti arah yang sudah tertulis di docs/02 §2.9 (komentar pada tabel `outbox_events`, ditulis sejak Fase 1: *"menambah tabel baru itu murah; mengubah kode transfer yang sudah teruji itu mahal... saat Fase 2 tiba, yang perlu ditambahkan hanya relay"*) dan docs/06 (menyebut RabbitMQ, bukan Kafka).

Dibangun: `internal/platform/broker` (topologi), `internal/platform/outbox` (relay), `internal/consumer` (consumer), `cmd/worker` (binary), migration `000010_processed_events`, service `rabbitmq` + `worker` di compose, `Dockerfile.worker`, test `TestOutbox_RelayDanConsumer_EndToEnd`.

### Kenapa binary terpisah, kenapa relay DAN consumer di proses yang sama
**Binary terpisah dari `cmd/api`** (bukan endpoint HTTP baru): profil skalanya beda total. API butuh banyak instance kecil yang responsif terhadap trafik; worker butuh sedikit instance yang memegang koneksi AMQP lama dan boleh berjalan tanpa terburu-buru. Menggabungkannya dalam satu binary berarti scaling API (mis. karena trafik HTTP naik) ikut menggandakan koneksi AMQP tanpa alasan.

**Relay dan consumer dalam SATU proses `cmd/worker`** (bukan dua binary terpisah): pada volume Fase 2 ini, keduanya ringan dan tidak butuh skala independen. Kalau nanti consumer perlu 5× lebih banyak instance daripada relay (misalnya pemrosesan yang berat), itu alasan cukup untuk memisahkannya — bukan sekarang, saat belum ada bukti kebutuhan itu. Prinsip yang sama dengan kenapa fee TIDAK di-shard 64× di Sesi 33: jangan menambah kompleksitas sebelum diukur perlu.

### Kenapa dua channel AMQP berbeda untuk relay dan consumer, bukan satu
`newConfirmChannel` (untuk relay) mengaktifkan `Confirm(false)` — mode ini mengubah semantik SETIAP publish di channel itu. Consumer tidak pernah publish, jadi channel-nya tidak perlu (dan sebaiknya tidak) berbagi mode itu. Lebih penting: kalau consumer nack pesan atau channel-nya error, itu TIDAK BOLEH menutup kemampuan relay mengirim — dua channel terpisah mengisolasi kegagalan satu peran dari peran lain, prinsip yang sama dengan "setiap goroutine punya pemilik yang tahu kapan ia berhenti" yang sudah dipegang sejak `go-conventions` skill Fase 1.

### Kenapa dua kelas error dibedakan di consumer (requeue vs DLQ)
```go
if err != nil {                    // gagal INSERT processed_events — biasanya SEMENTARA
    d.Nack(false, true)            // requeue: coba lagi, kemungkinan besar akan berhasil
}
...
if err := json.Unmarshal(...); err != nil {   // payload cacat — PERMANEN
    d.Nack(false, false)                       // ke DLQ: mengulang tidak akan memperbaikinya
}
```
Menyamakan keduanya (semua nack requeue=false, atau semua requeue=true) adalah kesalahan umum. Kegagalan sementara yang dikirim ke DLQ berarti kehilangan pesan yang sebenarnya valid hanya karena database sempat lambat sedetik. Kegagalan permanen yang di-requeue berarti pesan itu berputar selamanya di antara consumer dan broker tanpa pernah selesai — "poison message" yang menghabiskan resource tanpa progres.

### Kenapa `processed_events` diisi SEBELUM "kerja" dilakukan, bukan setelah
```go
ct, err := a.db.Exec(ctx, `INSERT INTO processed_events (event_id, consumer) VALUES ($1, $2) ON CONFLICT DO NOTHING`, ...)
if ct.RowsAffected() == 0 { /* sudah pernah, lewati */ }
// baru SETELAH INI parsing payload & "kerja" consumer
```
Untuk consumer INI (audit log — kerjanya cuma logging), urutan ini tidak terlalu penting. Tapi ditulis dengan urutan yang BENAR untuk consumer yang kerjanya punya efek samping nyata (kirim email, kurangi stok): klaim dedup HARUS terjadi sebelum efek samping, supaya efek samping itu sendiri idempoten secara alami — persis pola yang sama dengan klaim idempotency key di `LedgerRepo.Post` Fase 1 (klaim dulu, baru kerja). Konsistensi pola ini bukan kebetulan; dua masalah yang tampak berbeda (idempotency HTTP vs idempotency consumer) punya bentuk solusi yang identik.

### Contoh — cara membaca hasil uji redelivery
```go
sent2, _ := relay.PollOnce(ctx)     // 0 — tidak ada outbox_events baru untuk dikirim
republish(t, relayCh)               // publish MANUAL event yang SAMA, simulasi redelivery broker
// ... consumer jalan lagi ...
if n := countRows(t, "processed_events"); n != 1 { t.Fatal(...) }   // TETAP 1, bukan 2
```
Test ini secara sengaja TIDAK menyuruh relay mengirim ulang (relay tidak akan pernah melakukan itu untuk event yang sudah `published_at` terisi) — sebaliknya, ia mensimulasikan apa yang RabbitMQ SENDIRI bisa lakukan tanpa sepengetahuan relay (redelivery karena consumer restart, network blip, dll). Idempotensi consumer harus tahan terhadap skenario ini walau relay-nya sendiri berkelakuan baik.

### Bukti
`TestOutbox_RelayDanConsumer_EndToEnd` PASS dengan `-race` (RabbitMQ sungguhan via testcontainers, 8 detik). Diverifikasi ULANG lewat `docker compose up` sungguhan (bukan hanya test): topup nyata via curl → `outbox_events.published_at` terisi dalam ~1 detik → `processed_events` bertambah 1 baris → log worker menunjukkan `event_id` yang SAMA mengalir dari database sampai consumer. `docker compose stop worker` (SIGTERM) → log "menyelesaikan pekerjaan yang sedang berjalan" sebelum proses berhenti. Image worker 12,9 MB. Detail: `docs/evidence/fase2-outbox.md`.

---

## Sesi 35 — Role database terbatas untuk `entries` (lapis kedua, docs/02 §2.6) (2026-09-16)

### Apa
Diminta "lanjutkan berdasarkan prioritas terpenting dahulu" atas 3 sisa item Fase 2. Dipilih role DB terbatas SEBAGAI PRIORITAS TERTINGGI (bukan rate limit Redis atau `/metrics`) — alasan urutan ditulis di bawah. Dibangun migration `000011_app_role`: peran `nusaledger_app` (LOGIN, bukan superuser) dibuat, diberi GRANT baseline SELECT/INSERT/UPDATE/DELETE ke semua tabel, lalu `entries` di-REVOKE UPDATE+DELETE (TRUNCATE otomatis tidak pernah ada karena baseline tidak menyebutnya) dan `transactions` di-REVOKE DELETE. `cmd/api` dan `cmd/worker` sekarang connect sebagai `nusaledger_app`; migrasi tetap jalan sebagai superuser `nusa` lewat `MIGRATION_DATABASE_URL` (variabel baru, terpisah dari `DATABASE_URL`, fallback ke `DATABASE_URL` kalau kosong — jadi `make migrate-up` di lokal tidak perlu berubah).

### Kenapa item ini yang dikerjakan LEBIH DULU dari rate limit Redis dan `/metrics`
Ketiganya sudah lama tercatat sebagai sisa Fase 2, tapi punya konsekuensi kegagalan yang berbeda jauh. Rate limit in-memory yang tidak konsisten lintas instance API "hanya" berarti batas laju sedikit lebih longgar dari niat — bukan uang salah. Pembatasan jaringan `/metrics` adalah kebocoran INFORMASI (nama endpoint, tingkat trafik), bukan kebocoran KENDALI atas data. Role DB yang tidak dibatasi berarti SATU bug kode saja (mis. lupa memakai fungsi yang benar, atau operator yang membuka `psql` langsung dengan niat baik tapi salah tabel) bisa menimpa baris `entries` yang seharusnya append-only — dan `entries` adalah SATU-SATUNYA sumber kebenaran saldo di seluruh sistem (docs/02 §2.1). Kerusakan di sana tidak bisa diperbaiki dengan restart atau rollback kode; harus rekonstruksi manual dari log. Urutan prioritas mengikuti besar kerusakan-kalau-gagal, bukan urutan disebut di HANDOVER.

### Kenapa dua lapis (trigger DAN hak akses), bukan salah satu saja
`forbid_mutation` (trigger, sudah ada sejak Fase 1) menahan BUG KODE — kode Go yang salah menulis `UPDATE entries` masih akan ditolak Postgres di level trigger. Tapi trigger hanya berjalan untuk peran yang PUNYA hak UPDATE/DELETE di kolom itu; trigger tidak mencegah siapa pun yang connect dengan kredensial aplikasi dan mengetik SQL manual, karena secara hak akses murni, peran itu memang BOLEH melakukannya (triggernya baru menolak setelah percobaan dimulai, dan — lebih penting — trigger bisa (secara teori) di-`DISABLE` oleh siapa pun yang punya hak `ALTER TABLE`, sesuatu yang peran aplikasi normal-nya TIDAK butuh). Mencabut hak akses di level role menutup jalur itu sebelum trigger sempat relevan: dua ancaman berbeda (bug kode vs operator/kredensial bocor), dua lapis berbeda, bukan duplikasi.

### Kenapa nama database tidak boleh di-hardcode dalam migration
Percobaan pertama menulis `GRANT CONNECT ON DATABASE nusaledger TO nusaledger_app` langsung. Ini GAGAL saat integration test jalan (testcontainers memberi nama database `testdb`, bukan `nusaledger`) dengan error `database "nusaledger" does not exist (SQLSTATE 3D000)` — migration yang sama harus benar di DUA lingkungan berbeda (dev pakai `nusaledger`, test pakai `testdb`), padahal `GRANT ... ON DATABASE` di PostgreSQL mensyaratkan identifier LITERAL, tidak menerima ekspresi atau parameter seperti query biasa. Solusinya SQL dinamis:
```sql
DO $$
BEGIN
    EXECUTE format('GRANT CONNECT ON DATABASE %I TO nusaledger_app', current_database());
END$$;
```
`current_database()` selalu mengembalikan nama yang BENAR di lingkungan mana pun ia dijalankan; `format('%I', ...)` meng-quote identifier itu dengan aman (mencegah SQL injection kalau nama database pernah mengandung karakter aneh); `EXECUTE` menjalankan string SQL yang dihasilkan sebagai statement sungguhan. Pola ini berguna kapan pun sebuah migration harus menyebut nama database/objek yang BERBEDA antar lingkungan tapi tidak tersedia sebagai parameter di DDL PostgreSQL.

### Kenapa test barunya BUKAN cuma mengulang T-03 (trigger)
`TestSchema_T03_EntriesAppendOnly` (Fase 1) connect sebagai `test` (superuser di testcontainers) — peran itu SECARA HAK AKSES boleh UPDATE/DELETE `entries`, dan yang menahannya HANYA trigger. Test baru (`TestAppRole_TidakBisaMengubahLedger`) connect sebagai `nusaledger_app` sungguhan (bukan superuser) dan membuktikan hal yang BERBEDA: permintaan yang sama ditolak Postgres SEBELUM trigger sempat relevan, dengan SQLSTATE `42501` (`insufficient_privilege`) bukan `23514` (`check_violation`, kode yang dipakai trigger). Kalau trigger dihapus tidak sengaja di migration masa depan, T-03 akan mulai gagal (bagus, itu perannya) — tapi test role ini TETAP hijau, karena lapisannya independen. Ditambahkan juga assert bahwa `UPDATE transactions.status` (dibutuhkan reversal) TETAP diizinkan di level hak akses — membuktikan REVOKE-nya tepat sasaran, tidak sengaja terlalu ketat.

### Contoh — membangun DSN peran lain dari DSN testcontainers
```go
dsn := strings.Replace(testDSN, "test:test@", "nusaledger_app:app_dev_only_ganti_di_produksi@", 1)
pool, _ := pgxpool.New(ctx, dsn)   // koneksi BARU, peran BERBEDA, database SAMA
```
`testDSN` (variabel level-package baru di `main_test.go`, diisi dari `ctr.ConnectionString`) menyimpan DSN superuser `test` yang dipakai HAMPIR semua test lain. Test ini satu-satunya yang perlu menyambung sebagai peran LAIN untuk menguji hak akses peran itu sendiri — mengganti kredensial di DSN string yang sama (host/port/database tetap, hanya user:password berubah) lebih murah daripada membangun DSN dari nol.

### Bukti
`TestAppRole_TidakBisaMengubahLedger` PASS dengan `-race` (6 subtest: UPDATE/DELETE/TRUNCATE `entries` ditolak `42501`, DELETE `transactions` ditolak `42501`, SELECT/INSERT `entries` tetap boleh, UPDATE `transactions.status` tetap boleh). Suite integration penuh (30 test, termasuk T-03 lama) tetap hijau setelah migration baru ditambahkan — 22,8 detik. Lint (`golangci-lint`) 0 issue, `go vet` bersih, `govulncheck` 0 vulnerabilitas nyata (1 modul transitif tidak terpakai, sudah diverifikasi sejak Sesi 31).

---

## Status akhir sesi (2026-09-16)

Sesi 1–35 selesai: Fase 1 SELESAI TOTAL (v1.0.3), Fase 2 slice pertama (outbox) + role DB terbatas untuk `entries` (lapis kedua) selesai dan teruji. Sisa Fase 2: rate limit Redis, pembatasan jaringan untuk `/metrics`. Ringkasan bukti ada di README §Fase 2, `docs/evidence/fase2-outbox.md`. Semua keputusan, jebakan, dan alasan tercatat di jurnal ini agar bisa diulang manual dari repo kosong.
