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
