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
