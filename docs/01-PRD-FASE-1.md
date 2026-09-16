# PRD — NusaLedger Fase 1: Ledger Core

**Versi:** 1.0 · **Status:** Siap dieksekusi · **Durasi target:** 3–4 minggu (paruh waktu)

---

## 1. Latar Belakang & Tujuan

### 1.1 Masalah yang diselesaikan

Sebagian besar aplikasi yang menangani uang menyimpan saldo sebagai **satu kolom angka** yang di-`UPDATE` setiap ada transaksi. Pendekatan ini terlihat sederhana dan bekerja di demo, tetapi gagal pada tiga hal yang tidak bisa ditawar di sistem keuangan:

| Masalah | Akibat nyata |
|---|---|
| Tidak ada jejak asal-usul saldo | Saat saldo pengguna salah, tidak ada cara membuktikan kenapa |
| `UPDATE balance = balance - x` tanpa perlindungan | *Lost update* — dua transaksi bersamaan membuat uang tercipta atau hilang |
| Koreksi dilakukan dengan `UPDATE` atau `DELETE` | Jejak audit rusak; tidak bisa dipertanggungjawabkan |

NusaLedger Fase 1 membangun **ledger double-entry** yang menjadikan setiap rupiah dapat ditelusuri, tidak dapat diubah setelah dicatat, dan tahan terhadap akses bersamaan.

### 1.2 Tujuan portofolio

Dokumen ini juga berfungsi sebagai bukti kemampuan. Yang harus terlihat oleh pewawancara setelah Fase 1 selesai:

1. Paham **correctness uang** — bukan sekadar CRUD
2. Paham **konkurensi database** — dan bisa membuktikannya dengan test
3. Paham **idempotency** — dan tahu kenapa itu wajib
4. Menulis kode Go yang **berlapis, teruji, dan bisa di-review**

### 1.3 Bukan tujuan Fase 1 (out of scope)

Ditulis eksplisit supaya tidak melebar:

- ❌ Message queue (RabbitMQ/Kafka) → Fase 2 & 3
- ❌ Fraud detection → Fase 3
- ❌ Rekonsiliasi eksternal → Fase 4
- ❌ Lapisan AI → Fase 4
- ❌ Multi-currency → tidak pernah; satu mata uang (IDR) saja
- ❌ Frontend → cukup Swagger/OpenAPI + koleksi HTTP request
- ❌ Integrasi payment gateway sungguhan → simulasi internal

> **Kenapa scope dibatasi ketat (🏛️ ARC):** penyebab utama portofolio mangkrak bukan kesulitan teknis, tapi scope yang membengkak diam-diam. Fase 1 harus bisa berdiri sendiri sebagai portofolio utuh. Kalau berhenti di sini pun, hasilnya sudah layak dipamerkan.

---

## 2. Model Akuntansi

Bagian ini adalah **fondasi seluruh sistem**. Salah di sini, semua yang di atasnya salah.

### 2.1 Aturan dasar double-entry

Setiap transaksi terdiri dari beberapa *entry*. Untuk setiap transaksi:

```
Σ(amount sisi DEBIT)  =  Σ(amount sisi CREDIT)
```

`amount` selalu **positif**. Arah (debit/kredit) yang menentukan maknanya.

### 2.2 Jenis akun dan sisi normalnya

| Jenis akun | Kategori | Normal balance | Bertambah saat | Contoh |
|---|---|---|---|---|
| `USER_WALLET` | Kewajiban | **CREDIT** | Dikredit | Dompet pengguna |
| `SYSTEM_CASH` | Aset | **DEBIT** | Didebit | Kas perusahaan di bank |
| `SYSTEM_FEE_REVENUE` | Pendapatan | **CREDIT** | Dikredit | Biaya admin terkumpul |
| `SYSTEM_SUSPENSE` | Kewajiban | **CREDIT** | Dikredit | Dana belum teridentifikasi |

> **Kenapa dompet pengguna adalah kewajiban, bukan aset (🗄️ DBA):** uang di dompet pengguna bukan milik perusahaan — perusahaan **berutang** sejumlah itu kepada pengguna. Ini bukan detail akademis: kalau salah mengklasifikasi, laporan keuangan sistem akan salah dan trial balance tidak akan pernah seimbang. Aturan turunannya yang sering bikin bingung developer: **menambah saldo pengguna berarti meng-KREDIT akun dompetnya.**

### 2.3 Rumus perubahan saldo

```
delta = (direction == account.normal_balance) ? +amount : -amount
```

Dengan satu rumus ini, semua jenis akun ditangani seragam. Tidak perlu percabangan `if tipe akun == ...` di mana-mana.

### 2.4 Empat operasi Fase 1

**A. TOPUP** — pengguna mengisi saldo Rp 100.000

| Akun | Sisi | Jumlah | Efek saldo |
|---|---|---|---|
| SYSTEM_CASH (aset) | DEBIT | 100.000 | +100.000 |
| USER_WALLET Andi (kewajiban) | CREDIT | 100.000 | +100.000 |

Seimbang: debit 100.000 = kredit 100.000. Kas perusahaan bertambah, dan utang perusahaan ke Andi juga bertambah. Keduanya benar.

**B. TRANSFER** — Andi kirim Rp 50.000 ke Budi, biaya admin Rp 1.000

| Akun | Sisi | Jumlah | Efek saldo |
|---|---|---|---|
| USER_WALLET Andi | DEBIT | 51.000 | −51.000 |
| USER_WALLET Budi | CREDIT | 50.000 | +50.000 |
| SYSTEM_FEE_REVENUE | CREDIT | 1.000 | +1.000 |

Seimbang: debit 51.000 = kredit 51.000.

**C. WITHDRAW** — Andi tarik Rp 200.000 ke rekening bank

| Akun | Sisi | Jumlah | Efek saldo |
|---|---|---|---|
| USER_WALLET Andi | DEBIT | 200.000 | −200.000 |
| SYSTEM_CASH | CREDIT | 200.000 | −200.000 |

**D. REVERSAL** — membatalkan transaksi X

Membuat transaksi **baru** dengan semua entry dibalik arahnya, dan mencatat `reverses_transaction_id = X`. Transaksi asli **tidak disentuh sama sekali**.

> **Kenapa tidak menghapus saja (🗄️ DBA + 🏛️ ARC):** ledger yang barisnya bisa dihapus tidak bisa diaudit. Kalau auditor bertanya "kenapa saldo tanggal 3 berbeda dengan tanggal 2", jawabannya harus ada di data — bukan di ingatan developer. Ini juga alasan tabel `entries` dibuat *append-only* dan ditegakkan lewat trigger database, bukan sekadar konvensi tim.

---

## 3. Aturan Bisnis

Diberi kode `BR-xx` supaya bisa dirujuk dari test dan review.

| Kode | Aturan | Ditegakkan di mana |
|---|---|---|
| **BR-01** | Semua nominal disimpan sebagai `BIGINT` dalam satuan **sen** (Rp 1 = 100). Dilarang `float` di seluruh sistem | Skema DB + review kode |
| **BR-02** | `amount` setiap entry harus > 0 | `CHECK` constraint |
| **BR-03** | Total debit = total kredit untuk setiap transaksi | Deferred constraint trigger |
| **BR-04** | Tabel `entries` dan `transactions` append-only | Trigger penolak UPDATE/DELETE |
| **BR-05** | Saldo `USER_WALLET` tidak boleh negatif | `CHECK` + atomic update |
| **BR-06** | Akun sistem **boleh** bernilai negatif (mis. SYSTEM_CASH saat simulasi) | Tidak ada CHECK |
| **BR-07** | Setiap operasi yang memindahkan uang **wajib** `Idempotency-Key` | Validasi di handler |
| **BR-08** | Idempotency key sama + body berbeda → tolak `409 Conflict` | Perbandingan hash body |
| **BR-09** | Transfer ke diri sendiri ditolak | Validasi di service |
| **BR-10** | Akun berstatus `FROZEN`/`CLOSED` tidak bisa didebit maupun dikredit | Validasi di service + cek saat posting |
| **BR-11** | Reversal hanya untuk transaksi berstatus `POSTED` dan belum pernah dibalik | Unique index pada `reverses_transaction_id` |
| **BR-12** | Pengguna hanya boleh melihat akun & mutasi miliknya sendiri | Filter di klausa `WHERE`, bukan di aplikasi |
| **BR-13** | Biaya admin transfer: flat Rp 1.000 (konfigurasi, bukan hardcode) | Config |
| **BR-14** | Nominal transfer minimum Rp 10.000, maksimum Rp 50.000.000 per transaksi | Validasi request |
| **BR-15** | Trial balance global harus selalu seimbang | Endpoint verifikasi + test |

---

## 4. Pengguna & Peran

| Peran | Bisa melakukan |
|---|---|
| `USER` | Registrasi, login, lihat saldo & mutasi sendiri, topup, transfer, withdraw |
| `ADMIN` | Semua di atas + reversal transaksi + lihat trial balance + lihat akun mana pun |

Fase 1 tidak membangun panel admin. Peran `ADMIN` cukup dibuat lewat seeder.

---

## 5. Alur Utama

### 5.1 Transfer (alur terpenting)

```
1. Klien kirim POST /transactions/transfer + header Idempotency-Key
2. Middleware: validasi JWT → dapat user_id
3. Handler: validasi format request (nominal, akun tujuan)
4. Service: cek idempotency key
   ├─ sudah ada + hash sama  → kembalikan hasil lama (201; kode status sama seperti permintaan pertama)
   ├─ sudah ada + hash beda  → tolak (409)
   └─ belum ada              → lanjut
5. MULAI TRANSAKSI DATABASE
   a. Sisipkan idempotency_keys (ON CONFLICT DO NOTHING)
   b. Kunci kedua akun dengan FOR UPDATE, urut berdasarkan id (anti-deadlock)
   c. Validasi status akun & kecukupan saldo
   d. Sisipkan transactions
   e. Sisipkan 3 entries (debit pengirim, kredit penerima, kredit fee)
   f. Perbarui saldo kedua akun + naikkan version
   g. Simpan response ke idempotency_keys
   COMMIT  ← trigger memverifikasi debit = kredit di sini
6. Kembalikan 201 Created
```

### 5.2 Titik kegagalan & penanganannya

| Gagal di langkah | Yang terjadi | Apakah aman? |
|---|---|---|
| 1–4 | Belum ada perubahan data | ✅ Aman |
| 5a–5g | Seluruh transaksi di-rollback | ✅ Aman, klien boleh retry dengan key sama |
| Setelah COMMIT, sebelum respons terkirim | Data sudah benar, klien tidak tahu | ✅ Retry dengan key sama mengembalikan hasil tersimpan |
| Saldo tidak cukup | Rollback, kembalikan 422 | ✅ Aman |

> **Kenapa baris ketiga adalah inti dari idempotency (🏛️ ARC):** inilah satu-satunya skenario yang tidak bisa diselesaikan dengan transaksi database saja. Uang sudah berpindah, tetapi klien mengira gagal dan mencoba lagi. Tanpa idempotency key, percobaan kedua akan memindahkan uang untuk kedua kalinya. Ini bukan kasus langka — di jaringan seluler Indonesia, ini terjadi setiap hari.

---

## 6. Kriteria Penerimaan (Definition of Done)

Fase 1 dinyatakan selesai **hanya jika semua** tercentang:

### Fungsional
- [ ] Registrasi & login menghasilkan access token + refresh token
- [ ] Akun `USER_WALLET` otomatis dibuat saat registrasi
- [ ] Topup, transfer, withdraw berfungsi dan menghasilkan entry yang seimbang
- [ ] Reversal membuat transaksi kebalikan, transaksi asli utuh
- [ ] Mutasi rekening bisa dipaginasi dengan cursor
- [ ] Endpoint trial balance mengembalikan selisih 0

### Correctness
- [ ] Test konkurensi: 100 goroutine transfer dari 1 akun bersaldo terbatas → jumlah sukses persis sesuai saldo, tidak ada saldo negatif
- [ ] Test idempotency: request identik 10× → 1 transaksi, 10 respons sama
- [ ] Test idempotency paralel: 10 goroutine dengan key sama bersamaan → tetap 1 transaksi
- [ ] Test deadlock: transfer A→B dan B→A bersamaan 50× → tidak ada deadlock
- [ ] Test invariant: setelah 1.000 transaksi acak, trial balance tetap 0
- [ ] Percobaan `UPDATE`/`DELETE` pada `entries` ditolak database

### Kualitas
- [ ] `go test -race ./...` bersih
- [ ] Coverage: domain ≥ 90%, service ≥ 80%
- [ ] `golangci-lint run` bersih
- [ ] `govulncheck ./...` bersih
- [ ] Integration test memakai testcontainers, bukan database lokal

### Operasional
- [ ] `docker compose up` → sistem jalan dari nol, satu perintah
- [ ] `/healthz`, `/readyz`, `/metrics` tersedia dan terpisah fungsinya
- [ ] Graceful shutdown terbukti (request 10 detik tetap selesai saat SIGTERM)
- [ ] Log JSON dengan `request_id` mengalir di seluruh lapisan
- [ ] Load test k6: p95 < 200 ms, p99 < 500 ms pada 100 VU

### Dokumentasi
- [ ] README dengan diagram, cara menjalankan, tabel keputusan teknis
- [ ] OpenAPI/Swagger untuk semua endpoint
- [ ] Catatan "bug yang saya temukan sendiri lewat test" — minimal satu

---

## 7. Target Non-Fungsional

| Aspek | Target | Cara verifikasi |
|---|---|---|
| Latensi transfer | p95 < 200 ms, p99 < 500 ms | k6, 100 VU, 3 menit |
| Throughput | ≥ 200 transfer/detik | k6 |
| Ketepatan saldo | 100%, tanpa toleransi | Test invariant |
| Waktu startup | < 5 detik | Manual |
| Ukuran image Docker | < 20 MB | `docker images` |
| Waktu unit test | < 10 detik | `time go test ./...` |

---

## 8. Risiko

| Risiko | Dampak | Mitigasi |
|---|---|---|
| Salah memahami arah debit/kredit | Seluruh ledger salah | Test invariant dijalankan sejak langkah pertama; tabel §2.4 jadi acuan tunggal |
| Deadlock pada transfer silang | Transaksi gagal acak, sulit direproduksi | Kunci akun selalu urut `ORDER BY id` |
| Scope melebar ke Fase 2 | Tidak pernah selesai | Daftar out-of-scope §1.3 dipatuhi ketat |
| Kehabisan tenaga di 70% | Tidak ada yang bisa dipamerkan | Commit harian; README ditulis sejak minggu pertama, bukan terakhir |
