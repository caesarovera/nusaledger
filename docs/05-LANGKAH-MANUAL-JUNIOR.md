# Panduan Manual Langkah demi Langkah
## Untuk dikerjakan sendiri tanpa AI — 14 langkah dari repo kosong sampai jalan

Setiap langkah punya tiga bagian:
- **Apa** — yang dikerjakan
- **Kenapa** — alasan di baliknya (bagian terpenting; ini yang ditanyakan saat wawancara)
- **Bukti** — cara memastikan langkahnya benar sebelum lanjut

> **Aturan utama: jangan lanjut sebelum bagian "Bukti" tercapai.** Bug di sistem uang yang ditemukan di langkah 12 hampir selalu berasal dari langkah 3 yang dilewati buru-buru.

---

## Langkah 0 — Persiapan

### Apa
```bash
# Wajib
go version        # butuh 1.27 (terpasang: go1.27.0)
docker --version  # butuh Docker berjalan
psql --version    # klien PostgreSQL

# Pasang alat bantu
go install github.com/golang-migrate/migrate/v4/cmd/migrate@latest
go install github.com/go-delve/delve/cmd/dlv@latest
go install golang.org/x/vuln/cmd/govulncheck@latest
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin
```

### Kenapa
Memasang semua alat di awal mencegah gangguan di tengah kerja. Saat sedang mengejar satu bug, berhenti untuk memasang debugger adalah cara tercepat kehilangan alur pikiran.

### Bukti
Semua perintah di atas mengeluarkan versi tanpa error.

---

## Langkah 1 — Struktur repo & Makefile

### Apa
```bash
mkdir nusaledger && cd nusaledger
go mod init github.com/<username>/nusaledger
git init

mkdir -p cmd/api internal/{domain,service,config} \
         internal/repository/postgres internal/transport/http \
         internal/platform/{logger,metrics,token} \
         migrations test/{integration,load} docs .claude/{agents,skills}
```

`Makefile`:
```makefile
.PHONY: test test-int race lint migrate-up migrate-down run up down

test:        ## unit test cepat, tanpa Docker
	go test ./internal/...

test-int:    ## integration test, butuh Docker
	go test -tags=integration -race ./test/...

race:
	go test -race ./...

lint:
	go vet ./...
	golangci-lint run
	govulncheck ./...

migrate-up:
	migrate -path migrations -database "$(DATABASE_URL)" up

migrate-down:
	migrate -path migrations -database "$(DATABASE_URL)" down 1

up:
	docker compose up -d

run:
	go run ./cmd/api
```

### Kenapa
**Struktur dibuat lengkap di awal** supaya tidak ada godaan menaruh file "sementara di root dulu" — file sementara itu selalu menetap.

**Makefile dibuat sebelum ada kode** karena perintah yang panjang cenderung tidak dijalankan. `make test` akan kamu ketik ratusan kali; `go test -tags=integration -race ./test/...` tidak akan.

`internal/` bukan pilihan gaya: compiler Go melarang paket di dalamnya diimpor dari luar modul. Ini satu-satunya enkapsulasi level-modul yang tersedia di Go, dan gratis.

### Bukti
`go build ./...` berhasil (belum ada apa-apa, tapi modul valid).

---

## Langkah 2 — Docker Compose untuk database

### Apa
`docker-compose.yml`:
```yaml
services:
  postgres:
    image: postgres:17-alpine
    environment:
      POSTGRES_USER: nusa
      POSTGRES_PASSWORD: nusa_dev_only
      POSTGRES_DB: nusaledger
    ports: ["5432:5432"]
    volumes: ["pgdata:/var/lib/postgresql/data"]
    command:
      - "postgres"
      - "-c"
      - "log_min_duration_statement=200ms"
      - "-c"
      - "idle_in_transaction_session_timeout=60000"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U nusa -d nusaledger"]
      interval: 5s
      timeout: 3s
      retries: 10
volumes:
  pgdata:
```

### Kenapa
**`healthcheck` bukan hiasan.** PostgreSQL butuh beberapa detik sebelum menerima koneksi. Tanpa healthcheck, aplikasi yang start bersamaan akan gagal terhubung — dan kamu akan menghabiskan waktu mencari bug yang sebenarnya hanya masalah waktu.

**`log_min_duration_statement=200ms`** membuat semua query lambat tercatat sejak hari pertama. Jauh lebih mudah memperbaiki query lambat saat baru ditulis daripada mencarinya enam minggu kemudian.

**`idle_in_transaction_session_timeout`** membunuh transaksi yang dibuka tapi lupa di-commit. Di sistem ledger, transaksi menggantung akan mengunci baris akun dan menghentikan semua transfer ke akun itu.

### Bukti
```bash
docker compose up -d
docker compose ps          # status harus "healthy"
psql "postgres://nusa:nusa_dev_only@localhost:5432/nusaledger" -c "SELECT 1"
```

---

## Langkah 3 — Migration & skema

### Apa
```bash
export DATABASE_URL="postgres://nusa:nusa_dev_only@localhost:5432/nusaledger?sslmode=disable"

migrate create -ext sql -dir migrations -seq extensions_and_types
migrate create -ext sql -dir migrations -seq users_and_auth
migrate create -ext sql -dir migrations -seq accounts
migrate create -ext sql -dir migrations -seq transactions_and_entries
migrate create -ext sql -dir migrations -seq ledger_triggers
migrate create -ext sql -dir migrations -seq idempotency
migrate create -ext sql -dir migrations -seq outbox
migrate create -ext sql -dir migrations -seq seed_system_accounts
```

Isi setiap file dari **dokumen 02-SKEMA-DATABASE.md §2**. Jangan menyalin sebagian — constraint dan trigger adalah inti dari jaminan kebenarannya.

### Kenapa
**Skema dibuat sebelum kode Go** karena skema adalah kontrak yang paling mahal diubah. Kode bisa direfaktor dalam satu jam; mengubah skema pada tabel berisi jutaan baris uang butuh perencanaan migrasi tersendiri.

**Setiap `up` wajib punya `down` yang berfungsi.** Migration tanpa `down` berarti deploy tanpa jalan pulang. Uji sekarang selagi database kosong, karena kalau diuji nanti saat sudah ada data, kamu tidak akan berani.

### Bukti
```bash
make migrate-up
psql $DATABASE_URL -c "\dt"       # 6 tabel muncul
psql $DATABASE_URL -c "\d entries" # trigger terlihat

# Uji rollback benar-benar berfungsi
migrate -path migrations -database "$DATABASE_URL" down -all
migrate -path migrations -database "$DATABASE_URL" up
```

**Uji trigger sekarang juga, sebelum ada kode Go sama sekali:**
```sql
BEGIN;
INSERT INTO transactions (txn_type) VALUES ('TRANSFER') RETURNING id;
-- pakai id di atas, sengaja tidak seimbang:
INSERT INTO entries (transaction_id,account_id,direction,amount,balance_after)
VALUES ('<id>', 1, 'DEBIT', 1000, 0);
COMMIT;
-- HARUS gagal: "transaksi ... tidak seimbang: debit=1000 kredit=0"
```

> Kalau `COMMIT` di atas **berhasil**, trigger-nya salah pasang. Berhenti dan perbaiki. Seluruh jaminan kebenaran sistem ini bergantung pada satu trigger itu.

---

## Langkah 4 — Konfigurasi & logger

### Apa
```go
// internal/config/config.go
type Config struct {
    Addr         string        `env:"ADDR" envDefault:":8080"`
    DatabaseURL  string        `env:"DATABASE_URL,required"`
    JWTSecret    string        `env:"JWT_SECRET,required"`
    Env          string        `env:"APP_ENV" envDefault:"development"`
    TransferFee  int64         `env:"TRANSFER_FEE_SEN" envDefault:"100000"`
    MinTransfer  int64         `env:"MIN_TRANSFER_SEN" envDefault:"1000000"`
    MaxTransfer  int64         `env:"MAX_TRANSFER_SEN" envDefault:"5000000000"`
    ShutdownWait time.Duration `env:"SHUTDOWN_WAIT" envDefault:"30s"`
}

func Load() (Config, error) {
    var c Config
    if err := env.Parse(&c); err != nil {
        return c, fmt.Errorf("memuat konfigurasi: %w", err)
    }
    if c.Env == "production" && len(c.JWTSecret) < 32 {
        return c, errors.New("JWT_SECRET minimal 32 byte di produksi")
    }
    if c.MinTransfer <= 0 || c.MaxTransfer <= c.MinTransfer {
        return c, errors.New("batas transfer tidak masuk akal")
    }
    return c, nil
}
```

### Kenapa
**Konfigurasi divalidasi saat startup, bukan saat dipakai.** Aplikasi yang start dengan `JWT_SECRET` kosong lalu gagal tiga jam kemudian saat ada yang login jauh lebih sulit didiagnosis daripada aplikasi yang menolak start.

**Biaya admin ada di config, bukan hardcode.** Angka yang bisa berubah karena keputusan bisnis tidak boleh memerlukan deploy ulang. Ini juga membuat test bisa memakai nilai berbeda tanpa mengakali kode.

### Bukti
Jalankan tanpa `DATABASE_URL` → aplikasi menolak start dengan pesan jelas.

---

## Langkah 5 — Tipe Money

### Apa
Implementasikan `internal/domain/money.go` dari **dokumen 03 §2**.

### Kenapa
Ini fondasi yang mencegah seluruh kelas bug. Dua hal yang dicegahnya:

**Pertama, pencampuran tipe.** Dengan `int64` biasa, kode ini lolos kompilasi:
```go
transfer(amount, userID)   // argumen tertukar, keduanya int64
```
Dengan `Money`, compiler menolaknya.

**Kedua, overflow diam-diam.** Di Go, `int64` yang meluap tidak melempar error — nilainya berputar menjadi negatif tanpa peringatan apa pun. Pada sistem uang, itu artinya saldo bisa tiba-tiba menjadi minus triliunan.

### Bukti
```go
func TestMoney_Overflow(t *testing.T) {
    m := Money(math.MaxInt64 - 10)
    if _, err := m.Add(Money(100)); !errors.Is(err, ErrAmountOverflow) {
        t.Fatalf("mau ErrAmountOverflow, dapat %v", err)
    }
}

func TestMoney_String(t *testing.T) {
    if got := Money(5_000_050).String(); got != "Rp 50000,50" {
        t.Errorf("mau Rp 50000,50, dapat %s", got)
    }
}
```
`make test` hijau dalam bawah 1 detik.

---

## Langkah 6 — Domain: Transaction & aturan double-entry

### Apa
Implementasikan `internal/domain/transaction.go` dari **dokumen 03 §3**: tipe `Entry`, `Transaction`, fungsi `Validate()` dan `BalanceDelta()`.

### Kenapa
**Aturan bisnis diletakkan di domain, bukan di service atau handler.** Alasannya praktis: di Fase 2, worker message queue juga akan memposting transaksi. Kalau `Validate()` ada di handler HTTP, worker akan melewatinya tanpa ada yang menyadari — dan ledger bisa rusak lewat jalur yang tidak diperiksa.

**Domain tidak mengimpor apa pun.** Itu sebabnya test-nya berjalan dalam mikrodetik dan bisa dijalankan ribuan kali sehari tanpa menyalakan Docker.

### Bukti
Test tabel untuk semua kasus dari dokumen 01 §2.4:

```go
func TestTransaction_Validate(t *testing.T) {
    tests := []struct {
        name    string
        entries []Entry
        wantErr error
    }{
        {"transfer seimbang dengan fee", []Entry{
            {1, DirectionDebit,  5_100_000},
            {2, DirectionCredit, 5_000_000},
            {3, DirectionCredit,   100_000},
        }, nil},
        {"tidak seimbang", []Entry{
            {1, DirectionDebit,  5_000_000},
            {2, DirectionCredit, 4_000_000},
        }, ErrUnbalanced},
        {"amount nol ditolak", []Entry{
            {1, DirectionDebit,  0},
            {2, DirectionCredit, 0},
        }, ErrAmountNotPositive},
        {"satu entry ditolak", []Entry{
            {1, DirectionDebit, 1000},
        }, ErrTooFewEntries},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            txn := &Transaction{Entries: tt.entries}
            if err := txn.Validate(); !errors.Is(err, tt.wantErr) {
                t.Fatalf("mau %v, dapat %v", tt.wantErr, err)
            }
        })
    }
}
```

Uji juga `BalanceDelta` untuk semua kombinasi arah × normal balance — empat kasus, semuanya harus benar. Kesalahan tanda di sini akan membuat seluruh ledger salah dengan cara yang sangat sulit dilacak.

---

## Langkah 7 — Connection pool

### Apa
Implementasikan `internal/repository/postgres/pool.go` dari **dokumen 02 §5**.

### Kenapa
**`MaxConns` bukan angka sembarangan.** Kalau setiap instance aplikasi membuka 100 koneksi dan ada 5 instance, database yang hanya menerima 100 koneksi akan menolak — lalu health check gagal, orchestrator me-restart pod, pod baru minta koneksi lagi, dan terjadilah crash loop. Rumusnya ada di dokumen 02.

**`Ping` saat startup** membuat kegagalan koneksi muncul sebagai "aplikasi menolak start", bukan "aplikasi hidup tapi semua request 500".

### Bukti
```go
pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
// matikan Docker → NewPool harus mengembalikan error, bukan pool yang rusak
```

---

## Langkah 8 — Repository: memposting transaksi

### Apa
Implementasikan `PostTransaction` dari **dokumen 03 §5**. Ini fungsi terpenting di seluruh sistem.

### Kenapa
Enam keputusan di dalamnya masing-masing menutup satu kelas bug. Yang paling penting untuk dipahami:

**Kenapa akun dikunci dengan `ORDER BY id`.** Bayangkan Andi (id 5) transfer ke Budi (id 9), dan bersamaan Budi transfer ke Andi:

```
Tanpa ORDER BY:
  Transaksi 1: kunci 5 ✓ → minta kunci 9 ... menunggu
  Transaksi 2: kunci 9 ✓ → minta kunci 5 ... menunggu
  → saling menunggu selamanya. PostgreSQL membunuh salah satunya.

Dengan ORDER BY id:
  Transaksi 1: kunci 5 ✓ → kunci 9 ✓ → selesai
  Transaksi 2: minta kunci 5 ... menunggu → dapat → kunci 9 ✓ → selesai
  → yang kedua hanya MENUNGGU, tidak deadlock.
```

Satu klausa `ORDER BY` menghilangkan seluruh kelas bug ini.

**Kenapa idempotency key diklaim di dalam transaksi yang sama.** Kalau key disimpan lewat transaksi terpisah:
```
INSERT key → COMMIT → [proses mati] → transfer tidak pernah terjadi
```
Key tercatat "sudah diproses" padahal uang belum berpindah. Retry akan ditolak, dan uang pelanggan hilang dari sudut pandangnya. Dengan satu transaksi, crash di mana pun akan me-rollback keduanya.

**Kenapa `version` ada di klausa `WHERE`.** Tanpanya:
```
T1 baca saldo 100.000 → T2 baca saldo 100.000
T1 tulis 50.000       → T2 tulis 50.000   ← satu transfer hilang jejaknya
```

### Bukti
Baru bisa dibuktikan penuh di langkah 10. Untuk sekarang, cukup verifikasi jalur bahagia satu transfer menghasilkan 3 entry dan saldo yang benar.

---

## Langkah 9 — Integration test dengan testcontainers

### Apa
```go
//go:build integration

func setupDB(t *testing.T) *pgxpool.Pool {
    t.Helper()
    ctx := context.Background()

    ctr, err := tcpostgres.Run(ctx, "postgres:17-alpine",
        tcpostgres.WithDatabase("testdb"),
        tcpostgres.WithUsername("test"),
        tcpostgres.WithPassword("test"),
        testcontainers.WithWaitStrategy(
            wait.ForLog("database system is ready to accept connections").
                WithOccurrence(2).WithStartupTimeout(30*time.Second)))
    if err != nil {
        t.Fatalf("menjalankan container: %v", err)
    }
    t.Cleanup(func() { _ = ctr.Terminate(ctx) })

    dsn, _ := ctr.ConnectionString(ctx, "sslmode=disable")
    runMigrations(t, dsn)

    pool, err := pgxpool.New(ctx, dsn)
    if err != nil {
        t.Fatalf("membuat pool: %v", err)
    }
    return pool
}
```

Lalu tiga helper invariant yang akan dipakai di semua test uang:
```go
func assertTrialBalanceZero(t *testing.T, db *pgxpool.Pool) { ... }
func assertNoNegativeWallet(t *testing.T, db *pgxpool.Pool) { ... }
func assertMaterializedBalanceMatchesEntries(t *testing.T, db *pgxpool.Pool) { ... }
```
Query-nya ada di **dokumen 02 §3.4 dan §3.5**.

### Kenapa
**Test unit tidak pernah bisa membuktikan SQL-mu benar.** Mock repository hanya membuktikan kode Go memanggil fungsi yang benar — ia tidak tahu apakah `ORDER BY` ada, apakah trigger menyala, atau apakah constraint bekerja. Hanya PostgreSQL sungguhan yang bisa membuktikan itu.

**Kenapa testcontainers, bukan database lokal:** database lokal menyimpan sisa data dari test sebelumnya, dan test yang lulus karena data sisa adalah test yang berbohong. Container baru setiap run menjamin kondisi awal yang bersih.

**Kenapa build tag `integration`:** `make test` harus selesai di bawah 10 detik supaya kamu benar-benar menjalankannya. Test yang lambat adalah test yang dilewati.

### Bukti
```bash
make test-int   # container menyala, migration jalan, test lulus
```

---

## Langkah 10 — Test konkurensi (langkah paling penting)

### Apa
Implementasikan T-04, T-05, dan T-07 dari **dokumen 03 §7.2**.

### Kenapa
Ini titik di mana kamu **membuktikan** — bukan berharap — bahwa sistemnya benar.

Yang diuji T-04 bukan "apakah transfer berhasil", melainkan **"apakah uang tetap utuh saat 100 permintaan datang bersamaan"**. Perbedaannya besar: transfer tunggal hampir selalu berhasil; masalah baru muncul saat ada kontensi.

Tiga assert invariant di akhir setiap test adalah inti pengujiannya. Berapa transfer yang berhasil boleh bervariasi tergantung penjadwalan goroutine — yang tidak boleh bervariasi adalah: tidak ada saldo negatif, trial balance tetap nol, dan saldo tersimpan cocok dengan penjumlahan entries.

**Eksperimen yang wajib kamu lakukan sekali:** hapus `AND version = $3` dari query update saldo, jalankan T-04 lagi, dan lihat test gagal dengan saldo yang tidak masuk akal. Setelah melihatnya dengan mata sendiri, kamu tidak akan pernah lupa kenapa optimistic lock ada — dan kamu punya cerita nyata untuk diceritakan saat wawancara.

### Bukti
```bash
go test -race -tags=integration -run TestTransfer_Concurrent ./test/... -count=5
```

`-count=5` menjalankan test lima kali. Bug konkurensi sering lolos sekali dan gagal di percobaan keempat — sekali lulus belum berarti benar.

---

## Langkah 11 — Service & idempotency

### Apa
Implementasikan `internal/service/ledger_service.go` dari **dokumen 03 §5**, beserta interface yang dibutuhkannya di `ports.go`.

### Kenapa
**Interface didefinisikan di package `service`, bukan `repository`.** Alasannya: interface adalah kebutuhan pemakainya. Kalau ditaruh di sisi repository, ia cenderung membengkak menjadi 20 method karena "mungkin nanti ada yang butuh". Di sisi service, tiap service hanya mendeklarasikan 2–3 method yang benar-benar dipakainya — sehingga stub untuk test jadi kecil dan test jadi cepat.

**Service tidak tahu HTTP.** Ia mengembalikan `domain.ErrInsufficientBalance`, bukan `http.StatusUnprocessableEntity`. Ini bukan kerapian semata: di Fase 2, worker queue akan memanggil service yang sama, dan worker tidak punya konsep status code.

### Bukti
Unit test service dengan stub repository, tanpa database:
```go
func TestTransfer_SelfTransferDitolak(t *testing.T) {
    svc := NewLedgerService(&stubRepo{}, &stubIdem{}, testCfg)
    _, err := svc.Transfer(ctx, TransferInput{FromAccountID: 1, ToAccountID: 1, AmountSen: 5_000_000})
    if !errors.Is(err, domain.ErrSelfTransfer) {
        t.Fatalf("mau ErrSelfTransfer, dapat %v", err)
    }
}
```
Berjalan dalam mikrodetik, tanpa Docker.

---

## Langkah 12 — HTTP: router, middleware, handler

### Apa
Urutan pengerjaan yang disarankan:
1. `response.go` — `writeJSON`, `writeError` (dipakai semua handler)
2. `dto.go` — struct request & response
3. `middleware.go` — RequestID, Logging, Recover, Auth, RateLimit
4. `handler_*.go` — handler tipis
5. `router.go` — perakitan

### Kenapa
**Handler harus tipis.** Tugasnya hanya tiga: parse request, panggil service, terjemahkan error ke status code. Semua logika yang berisiko ada di service dan domain, yang bisa diuji tanpa menyalakan HTTP server.

**Recovery middleware wajib**, tapi perhatikan batasannya: `recover()` hanya menangkap panic di goroutine yang sama. Kalau handler memanggil `go someFunc()` dan fungsi itu panic, server tetap mati. Setiap goroutine butuh recovery sendiri.

**Batasi ukuran body sejak awal:**
```go
r.Body = http.MaxBytesReader(w, r.Body, 1<<20)   // 1 MB
```
Tanpa ini, satu request dengan body 5 GB menghabiskan seluruh RAM server. Ini serangan paling murah yang ada.

**`DisallowUnknownFields`** mencegah mass assignment — klien mengirim field yang tidak kamu sadari dan ikut ter-bind ke struct.

### Bukti
```bash
# Registrasi
curl -X POST localhost:8080/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"andi@test.com","password":"rahasiapanjang123","full_name":"Andi"}'

# Transfer tanpa Idempotency-Key → harus 400
curl -X POST localhost:8080/api/v1/transactions/transfer \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"to_account_public_id":"...","amount_sen":5000000}'

# Body raksasa → harus 413, bukan server mati
head -c 50000000 /dev/zero | curl -X POST localhost:8080/api/v1/transactions/transfer \
  -H "Authorization: Bearer $TOKEN" --data-binary @-
```

---

## Langkah 13 — Observability & graceful shutdown

### Apa
1. Logger `slog` JSON dengan redaksi field sensitif
2. Metrik Prometheus dari **dokumen 03 §8**
3. `/healthz` (tanpa cek DB) dan `/readyz` (dengan cek DB)
4. Graceful shutdown
5. `pprof` di port terpisah `127.0.0.1:6060`

### Kenapa
**Liveness dan readiness dipisah.** Kalau liveness ikut mengecek database, maka saat database lambat 3 detik, orchestrator akan **me-restart semua pod** — padahal aplikasinya sehat. Restart massal justru memperparah beban database. Liveness menjawab "perlukah proses ini dibunuh?"; readiness menjawab "layakkah proses ini menerima traffic sekarang?".

**Graceful shutdown menentukan apakah kamu berani deploy siang hari.** Tanpa itu, setiap deploy memutus transfer yang sedang berjalan — dan di sistem uang, request yang terputus di tengah adalah insiden, bukan gangguan kecil.

**`pprof` harus di port internal.** Mengimpor `net/http/pprof` mendaftarkan handler ke `http.DefaultServeMux`. Kalau server utamamu memakai mux itu, `/debug/pprof` terbuka ke internet — membocorkan struktur kode dan menjadi vektor DoS.

**`ledger_balance_drift_total` adalah metrik terpenting.** Nilai selain nol berarti ada uang yang tidak dapat dipertanggungjawabkan.

### Bukti
```bash
# Graceful shutdown
curl localhost:8080/api/v1/slow &     # endpoint uji 10 detik
sleep 1
kill -TERM $(pgrep nusaledger)
# request tetap selesai; log menampilkan "shutdown selesai"
```

---

## Langkah 14 — Docker, CI, dokumentasi

### Apa
1. `Dockerfile` multi-stage dengan `distroless` (pola ada di handbook Go Bagian 9.1)
2. Pipeline CI: lint → test → race → integration → build
3. `docs/openapi.yaml`
4. `README.md`

### Kenapa
**Image kecil bukan soal estetika.** Image 15 MB versus 300 MB berarti deploy lebih cepat, biaya transfer lebih murah, dan — yang terpenting — permukaan serangan jauh lebih kecil. Image distroless tidak punya shell; kalaupun penyerang berhasil mengeksekusi kode, tidak ada `sh` untuk dipanggil.

**README adalah bagian yang paling banyak dibaca dan paling sering ditulis terburu-buru.** Recruiter melihat repomu 5 menit. Struktur yang membuat kedalaman terbaca cepat:

```markdown
# NusaLedger
> Simulasi teknis ledger double-entry. Bukan produk keuangan; tanpa uang sungguhan.

## Menjalankan
docker compose up    # satu perintah, sistem siap

## Arsitektur
[diagram]

## Keputusan Teknis
| Keputusan | Alternatif yang ditolak | Alasan |
|---|---|---|
| Saldo dimaterialisasi + version | Selalu SUM(entries) | Baca instan & cek saldo atomik; entries tetap sumber kebenaran, diverifikasi job |
| Idempotency key wajib | Andalkan transaksi DB saja | Transaksi DB tidak menutup kasus ACK hilang setelah commit |
| Kunci akun ORDER BY id | Kunci sesuai urutan permintaan | Menghilangkan deadlock transfer silang |
| Trigger DEFERRED untuk balanced | Cek di aplikasi saja | Berlaku juga untuk migrasi & perbaikan data manual |

## Hasil Pengujian
- go test -race: bersih
- Coverage: domain 94%, service 86%
- k6 100 VU: p95 148ms, p99 392ms
- Test konkurensi: 100 transfer paralel, invariant utuh

## Bug yang saya temukan lewat test
Awalnya saya membaca saldo lalu meng-update-nya dalam dua statement terpisah.
Test T-04 dengan 100 goroutine menemukan lost update: saldo akhir tidak sesuai
jumlah transfer yang sukses. Diperbaiki menjadi atomic update dengan optimistic
lock, dan CHECK constraint ditambahkan sebagai jaring pengaman terakhir.
```

> **Bagian terakhir itu yang paling berkesan bagi pewawancara.** Menceritakan bug yang kamu temukan sendiri lewat test membuktikan prosesmu, bukan sekadar hasil akhirnya — dan itu jauh lebih sulit dipalsukan daripada kode yang rapi.

### Bukti
```bash
docker build -t nusaledger .
docker images nusaledger    # < 20 MB
docker compose up           # dari nol sampai jalan, satu perintah
```

---

## Ringkasan Urutan & Alasan

| Langkah | Kenapa urutannya begini |
|---|---|
| 1–2 Struktur & Docker | Fondasi kerja; setelah ini tidak ada gangguan setup |
| 3 Skema | Kontrak paling mahal diubah, jadi dikunci lebih dulu |
| 4–7 Config, Money, Domain, Pool | Lapisan terdalam ke luar; semuanya bisa diuji tanpa Docker |
| 8–10 Repository & test konkurensi | Titik pembuktian correctness; tidak boleh dilewati |
| 11 Service | Baru masuk akal setelah repository terbukti benar |
| 12 HTTP | Lapisan terluar dan paling tipis, jadi terakhir |
| 13 Observability | Butuh sistem yang sudah jalan untuk dipantau |
| 14 Kemasan | Yang dilihat orang lain, dikerjakan setelah isinya benar |

**Kesalahan urutan yang paling sering terjadi:** membangun HTTP handler lebih dulu karena cepat terlihat hasilnya, lalu menambal database di belakangnya. Hasilnya adalah sistem yang demo-nya bagus dan salah menghitung uang — dan kesalahannya baru ketahuan saat sudah terlalu mahal untuk diperbaiki.
