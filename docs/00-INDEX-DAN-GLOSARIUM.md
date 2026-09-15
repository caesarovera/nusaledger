# NusaLedger — Indeks Dokumen & Glosarium

**NusaLedger** adalah platform dompet digital berbasis *double-entry ledger*, dibangun sebagai portofolio teknis untuk melamar posisi Backend Engineer (Go) di sektor bank/fintech.

> ⚠️ **Ini simulasi teknis, bukan produk keuangan.** Tidak ada uang sungguhan, tidak ada integrasi bank nyata, tidak ada lisensi regulator. Kalimat ini wajib ada di paling atas README repo.

---

## Peta dokumen

| # | Dokumen | Isi | Dibaca oleh |
|---|---|---|---|
| 00 | **INDEX-DAN-GLOSARIUM** (ini) | Peta + kamus istilah | Semua |
| 01 | **PRD-FASE-1** | Apa yang dibangun & kenapa, aturan bisnis, kriteria selesai | Semua |
| 02 | **SKEMA-DATABASE** | DDL lengkap, index, constraint, migration | DBA, Developer |
| 03 | **SPESIFIKASI-TEKNIS-FASE-1** | Arsitektur, kontrak API, aturan kode, SLO | Developer, QA |
| 04 | **SETUP-CLAUDE-AI** | CLAUDE.md, subagent, skill, routing model & effort, hemat token | Developer |
| 05 | **LANGKAH-MANUAL-JUNIOR** | 14 langkah dari repo kosong sampai jalan, dengan alasan tiap langkah | Developer |

**Urutan baca:** 00 → 01 → 02 → 03 → 04 → 05.
**Urutan kerja:** 04 (setup dulu) → 05 (eksekusi) sambil membuka 02 & 03 sebagai acuan.

---

## Glosarium

Istilah dikelompokkan. Kalau menemukan istilah asing di dokumen lain, cari di sini dulu.

### A. Istilah akuntansi

| Istilah | Penjelasan sederhana |
|---|---|
| **Ledger** | Buku besar. Catatan resmi semua pergerakan uang. Di sistem ini, tabel `entries` adalah ledger-nya |
| **Double-entry** | Pencatatan berpasangan. Setiap pergerakan uang dicatat minimal dua baris: dari mana dan ke mana. Total keduanya **selalu** sama |
| **Debit / Kredit** | Dua sisi pencatatan. Jangan artikan "debit = keluar, kredit = masuk" — artinya tergantung jenis akun (lihat `normal_balance`) |
| **Normal balance** | Sisi yang menambah saldo akun. Akun aset bertambah di sisi debit; akun kewajiban bertambah di sisi kredit |
| **Aset (Asset)** | Harta yang dimiliki perusahaan. Contoh: uang tunai di rekening bank perusahaan |
| **Kewajiban (Liability)** | Utang perusahaan ke pihak lain. **Saldo dompet pengguna adalah kewajiban** — itu uang pengguna yang kita titipi |
| **Pendapatan (Revenue)** | Penghasilan perusahaan. Contoh: biaya admin transfer |
| **Posting** | Proses mencatat transaksi ke ledger |
| **Journal / Transaction** | Satu peristiwa keuangan utuh yang terdiri dari beberapa baris entry |
| **Entry** | Satu baris pencatatan: akun mana, sisi apa, berapa |
| **Reversal** | Pembatalan transaksi dengan cara **membuat transaksi kebalikan**, bukan menghapus yang lama |
| **Suspense account** | Akun penampung sementara untuk dana yang belum jelas tujuannya |
| **Trial balance** | Uji keseimbangan: total semua debit harus sama dengan total semua kredit di seluruh sistem |
| **Settlement** | Proses penyelesaian akhir pemindahan dana antar pihak |

### B. Istilah keandalan sistem

| Istilah | Penjelasan sederhana |
|---|---|
| **Idempoten** | Dijalankan sekali atau sepuluh kali, hasilnya sama. Wajib untuk operasi uang |
| **Idempotency key** | Kode unik dari klien untuk menandai "ini permintaan yang sama, bukan yang baru" |
| **Race condition** | Dua proses berebut data yang sama dan hasilnya salah karena urutannya tidak terkendali |
| **Lost update** | Bentuk race condition: dua proses membaca nilai lama, lalu keduanya menulis, satu perubahan hilang |
| **Optimistic lock** | Kunci "longgar": baca nilai + nomor versi, saat menyimpan cek versinya masih sama. Kalau berubah, tolak |
| **Pessimistic lock** | Kunci "ketat": `SELECT ... FOR UPDATE`, baris dikunci sampai transaksi selesai |
| **Deadlock** | Dua transaksi saling menunggu kunci yang dipegang pihak lain. Keduanya macet |
| **ACID** | Sifat transaksi database: Atomic (utuh), Consistent (konsisten), Isolated (terisolasi), Durable (permanen) |
| **Isolation level** | Seberapa ketat transaksi terisolasi dari transaksi lain. Default PostgreSQL: READ COMMITTED |
| **Constraint** | Aturan yang ditegakkan database. Kalau dilanggar, data ditolak — bukan sekadar peringatan |
| **Deferred constraint** | Constraint yang diperiksa saat COMMIT, bukan saat setiap baris disisipkan |
| **Trigger** | Fungsi yang otomatis dijalankan database saat ada INSERT/UPDATE/DELETE |
| **Append-only** | Tabel yang hanya boleh ditambah, tidak boleh diubah atau dihapus |
| **Graceful shutdown** | Berhenti dengan rapi: tolak permintaan baru, selesaikan yang sedang jalan, baru mati |
| **Outbox pattern** | Menulis event ke tabel database dalam transaksi yang sama, lalu proses lain mengirimnya ke message broker |

### C. Istilah Go & backend

| Istilah | Penjelasan sederhana |
|---|---|
| **Goroutine** | Unit kerja ringan yang jalan bersamaan di Go |
| **Channel** | Pipa untuk mengirim data antar goroutine |
| **Context** | Objek yang membawa batas waktu & sinyal pembatalan menembus seluruh pemanggilan fungsi |
| **Worker pool** | Sekumpulan pekerja dengan jumlah terbatas, supaya beban tidak meledak |
| **Connection pool** | Kumpulan koneksi database yang dipakai ulang, dengan batas jumlah |
| **DTO** | *Data Transfer Object*. Struct khusus untuk request/response API, terpisah dari entity database |
| **Middleware** | Lapisan yang membungkus setiap request HTTP: logging, auth, rate limit |
| **Repository** | Lapisan kode yang khusus berurusan dengan database |
| **Service / Use case** | Lapisan berisi aturan bisnis. Tidak tahu HTTP, tidak tahu SQL |
| **Domain** | Lapisan paling dalam: entity dan aturan murni. Tidak bergantung apa pun |
| **Sentinel error** | Error yang dideklarasikan sebagai variabel agar bisa dibandingkan (`errors.Is`) |
| **Migration** | File SQL bernomor yang mengubah struktur database secara terurut dan bisa dibalik |
| **Cursor pagination** | Paginasi berbasis penanda posisi, bukan `OFFSET`. Jauh lebih cepat di data besar |

### D. Istilah pengujian & operasional

| Istilah | Penjelasan sederhana |
|---|---|
| **Unit test** | Menguji satu fungsi/kelas terisolasi, tanpa database |
| **Integration test** | Menguji beberapa komponen bersama dengan database sungguhan |
| **Table-driven test** | Gaya test Go: satu daftar kasus, satu loop |
| **Testcontainers** | Library yang menyalakan PostgreSQL asli di Docker khusus untuk test |
| **Race detector** | `go test -race`. Mendeteksi akses data bersamaan yang tidak aman |
| **Coverage** | Persentase baris kode yang dieksekusi test. Bukan jaminan kualitas |
| **SLO** | *Service Level Objective*. Target terukur, misal "p99 < 300 ms" |
| **p95 / p99** | Persentil. p99 = 300 ms berarti 99% permintaan selesai di bawah 300 ms |
| **Golden signals** | Empat sinyal wajib dipantau: latensi, trafik, error, saturasi |
| **pprof** | Alat profiling bawaan Go untuk mencari pemborosan CPU/memori |

### E. Istilah Claude Code

| Istilah | Penjelasan sederhana |
|---|---|
| **Token** | Potongan teks (±3–4 karakter). Semua biaya AI dihitung per token |
| **Context window** | Jumlah token maksimum yang bisa dipegang model dalam satu percakapan |
| **CLAUDE.md** | File instruksi proyek yang otomatis dibaca Claude Code setiap sesi |
| **Subagent** | Claude terpisah dengan context window, tool, dan model sendiri. Dipanggil untuk tugas khusus |
| **Skill** | Kumpulan instruksi & praktik terbaik untuk jenis pekerjaan tertentu |
| **Plan mode** | Mode Claude Code yang menyusun rencana dulu tanpa mengubah file |
| **Prompt caching** | Menyimpan bagian prompt yang berulang agar tidak dibayar penuh setiap kali |
| **Batch API** | Mengirim banyak permintaan sekaligus untuk diproses asinkron, tarif 50% lebih murah |
| **Effort** | Seberapa dalam model berpikir sebelum menjawab. Makin tinggi, makin akurat, makin mahal |
