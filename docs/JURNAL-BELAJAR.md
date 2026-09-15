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
