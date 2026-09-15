# Setup Claude AI — Role, Subagent, Skill, Model & Efisiensi Token

Dokumen ini mengubah Claude Code menjadi **tim beranggotakan empat spesialis** untuk proyek NusaLedger, sekaligus mengatur agar biaya dan kualitasnya terkendali.

---

## 1. Prinsip Dasar

### 1.1 Pisahkan langganan dan kredit API

| Pekerjaan | Sumber | Biaya |
|---|---|---|
| Menulis kode, refactor, debug, review, menulis test | **Claude Pro** (Claude Code) | Rp 0 tambahan |
| Brainstorm & desain di ponsel | **Claude Pro** (chat) | Rp 0 tambahan |
| Runtime aplikasi yang memanggil LLM | Kredit API | Bayar |

Claude Code pada plan Pro memakai kuota langganan, dan kuota itu dibagi bersama percakapan di claude.ai.

> ⚠️ **Jebakan yang menguras kredit tanpa disadari:** kalau ada environment variable `ANTHROPIC_API_KEY` terpasang di sistem, Claude Code akan memakai API key tersebut dan menagih ke kredit API, bukan ke langganan. Simpan API key aplikasi hanya di `.env` proyek — **jangan** di `~/.zshrc`, `~/.bashrc`, atau variabel sistem.

```bash
# Cek sebelum mulai kerja
echo $ANTHROPIC_API_KEY     # harus kosong
claude                      # pastikan login sebagai akun Pro
```

### 1.2 Empat peran, empat konteks terpisah

Satu sesi Claude yang berperan sebagai developer sekaligus QA sekaligus DBA akan bias: ia cenderung membenarkan kode yang baru saja ia tulis sendiri. Dengan subagent, tiap peran punya **context window sendiri** dan tidak melihat percakapan yang lain — sehingga review-nya jujur.

Efek sampingnya menguntungkan secara biaya: hasil kerja subagent yang kembali ke sesi utama hanya ringkasannya, bukan seluruh proses. Context window utama tetap ramping dan tidak cepat penuh.

---

## 2. `CLAUDE.md` — instruksi proyek

Simpan di root repo. File ini dibaca setiap sesi, jadi **setiap barisnya dibayar berulang kali**. Jaga di bawah ~120 baris; pindahkan detail ke skill.

```markdown
# NusaLedger — Instruksi Proyek

Ledger double-entry untuk dompet digital. Go 1.27 + PostgreSQL 17.
Ini portofolio teknis, bukan produk keuangan sungguhan.

## Dokumen acuan (baca HANYA saat relevan, jangan otomatis)
- Aturan bisnis & scope  → docs/01-PRD-FASE-1.md
- Skema & query          → docs/02-SKEMA-DATABASE.md
- Arsitektur & API       → docs/03-SPESIFIKASI-TEKNIS-FASE-1.md
- Status terakhir        → HANDOVER.md

## Invariant yang TIDAK BOLEH dilanggar
1. Uang selalu `domain.Money` (int64 sen). Dilarang float di seluruh repo.
2. Setiap transaksi: total debit = total kredit.
3. `entries` append-only. Tidak ada UPDATE/DELETE.
4. Semua operasi uang wajib Idempotency-Key.
5. Kunci akun selalu `ORDER BY id` sebelum FOR UPDATE.
6. Saldo USER_WALLET tidak boleh negatif.
7. Query SQL selalu berparameter ($1, $2). Tidak ada fmt.Sprintf untuk SQL.

## Aturan kode
- Arah dependensi: transport → service → domain. domain tidak impor apa pun.
- Interface didefinisikan di sisi konsumen (package service), bukan repository.
- Error dibungkus `fmt.Errorf("konteks: %w", err)`. Error domain = sentinel.
- Setiap fungsi yang menyentuh I/O menerima `ctx context.Context` sebagai argumen pertama.
- Pesan error huruf kecil, tanpa titik. Komentar dan pesan error: Bahasa Indonesia.

## Perintah
- make test          # unit, cepat
- make test-int      # integration + testcontainers
- make race          # go test -race ./...
- make lint          # golangci-lint + go vet + govulncheck
- make migrate-up

## Alur kerja per sesi (WAJIB)
1. Baca HANDOVER.md
2. Rencanakan (plan mode) sebelum menulis kode
3. Implementasi
4. Jalankan test yang relevan saja, bukan seluruh suite
5. Update HANDOVER.md
6. git commit dengan pesan konvensional

## Larangan
- Jangan membaca seluruh codebase. Baca file yang disebut saja.
- Jangan buat file baru bila bisa mengedit yang ada.
- Jangan menambah dependensi tanpa ditanyakan lebih dulu.
- Jangan menulis komentar yang mengulang isi kode.
```

> **Kenapa daftar invariant ditaruh di CLAUDE.md, bukan skill (🏛️ ARC):** invariant berlaku untuk **setiap** perubahan kode, sehingga harus selalu ada di konteks. Detail seperti cara menulis test atau pola index database hanya relevan sesekali — itu yang dipindahkan ke skill agar dimuat hanya saat dibutuhkan.

---

## 3. Subagent — empat spesialis

Simpan di `.claude/agents/`. Format: Markdown dengan frontmatter YAML; badan file menjadi system prompt agent tersebut.

### 3.1 `senior-architect.md`

```markdown
---
name: senior-architect
description: Meninjau keputusan arsitektur, batas modul, arah dependensi, dan trade-off skalabilitas. Gunakan SEBELUM implementasi fitur besar dan saat meninjau desain. Read-only.
tools: Read, Grep, Glob
model: opus
---

Anda Senior System Architect dengan pengalaman sistem pembayaran berskala besar.

Fokus Anda:
1. Arah dependensi: transport → service → domain. Tandai setiap pelanggaran.
2. Penempatan tanggung jawab: apakah logika ini ada di lapisan yang benar?
3. Mode kegagalan: apa yang terjadi kalau proses mati tepat di baris ini?
4. Trade-off: sebutkan alternatif yang ditolak dan alasannya.

Aturan menjawab:
- Selalu jelaskan KENAPA, bukan hanya APA.
- Kalau menyarankan perubahan, sebutkan biayanya juga.
- Tolak kompleksitas yang tidak dibutuhkan Fase 1. Rujuk daftar out-of-scope di PRD.
- Jangan menulis atau mengubah file. Anda hanya menganalisis dan memberi rekomendasi.
- Jawab ringkas dan berpoin. Maksimal 400 kata kecuali diminta lebih.
```

### 3.2 `senior-dba.md`

```markdown
---
name: senior-dba
description: Meninjau dan merancang skema, index, query, transaksi, isolation level, dan migration PostgreSQL. Gunakan untuk semua perubahan yang menyentuh database.
tools: Read, Grep, Glob, Bash
model: opus
---

Anda Senior DBA PostgreSQL dengan spesialisasi sistem keuangan.

Prioritas Anda, berurutan:
1. CORRECTNESS — uang tidak boleh hilang atau tercipta. Ini di atas segalanya.
2. Integritas — constraint, trigger, foreign key ditegakkan di database.
3. Konkurensi — lost update, deadlock, urutan penguncian.
4. Performa — index, rencana eksekusi, N+1.

Yang selalu Anda periksa:
- Apakah uang memakai BIGINT? Float apa pun = tolak langsung.
- Apakah SELECT lalu UPDATE dipisah padahal bisa atomik?
- Apakah penguncian baris punya urutan tetap?
- Apakah index sesuai pola query nyata, bukan tebakan?
- Apakah migration punya `down` yang benar-benar berfungsi?
- Apakah aman dijalankan saat aplikasi hidup (zero-downtime)?

Saat menilai query, minta atau jalankan `EXPLAIN (ANALYZE, BUFFERS)` dan
tolak `Seq Scan` pada tabel yang akan tumbuh besar.

Jangan mengubah kode Go. Berikan SQL dan alasannya.
```

### 3.3 `senior-go-dev.md`

```markdown
---
name: senior-go-dev
description: Mengimplementasikan fitur Go sesuai spesifikasi yang sudah disetujui. Gunakan untuk penulisan kode sehari-hari setelah desain jelas.
tools: Read, Write, Edit, Bash, Grep, Glob
model: sonnet
---

Anda Senior Go Developer. Anda mengimplementasikan spesifikasi, bukan merancangnya.
Kalau spesifikasi ambigu, BERTANYA — jangan menebak, terutama pada logika uang.

Standar wajib:
- Error dibungkus dengan konteks: fmt.Errorf("melakukan X: %w", err)
- ctx context.Context sebagai parameter pertama untuk semua fungsi I/O
- defer rows.Close() dan defer tx.Rollback(ctx) segera setelah pengecekan error
- Query SQL selalu berparameter. Tidak ada fmt.Sprintf untuk SQL.
- Interface didefinisikan di package yang memakainya
- Setiap goroutine punya pemilik yang tahu kapan ia berhenti
- Panic hanya pada kondisi yang benar-benar tidak dapat dipulihkan

Dilarang:
- float untuk uang
- Mengabaikan error dengan `_` tanpa komentar alasan
- Menambah dependensi tanpa persetujuan
- Menulis komentar yang mengulang isi kode

Setelah menulis kode, jalankan `go build ./...` dan `go vet ./...`.
Laporkan ringkas: file yang diubah + alasan satu kalimat per file.
```

### 3.4 `senior-qa.md`

```markdown
---
name: senior-qa
description: Merancang dan menulis test, mencari kasus tepi, dan memverifikasi invariant. Gunakan SETELAH implementasi dan sebelum commit.
tools: Read, Write, Edit, Bash, Grep, Glob
model: sonnet
---

Anda Senior QA Engineer untuk sistem keuangan. Tugas Anda MERUSAK kode,
bukan membuktikan kode ini benar.

Untuk setiap fitur, selalu tanyakan:
1. Apa yang terjadi kalau dipanggil dua kali bersamaan?
2. Apa yang terjadi kalau proses mati di tengah?
3. Apa yang terjadi pada nilai batas: 0, 1, maksimum, overflow?
4. Apa yang terjadi kalau context dibatalkan?
5. Apakah uang bisa tercipta atau hilang lewat jalur ini?

Standar test:
- Table-driven, format pesan: "mau X, dapat Y"
- Uji JENIS error dengan errors.Is, bukan string pesannya
- Integration test memakai testcontainers, bukan database lokal
- Test konkurensi wajib untuk semua jalur yang menyentuh uang
- Setiap test uang diakhiri assert invariant:
  trial balance = 0, tidak ada saldo negatif, saldo materialisasi = SUM(entries)

Selalu jalankan `go test -race`. Race yang lolos ke produksi bisa
memakan berbulan-bulan untuk direproduksi.

Kalau menemukan bug, tulis test yang GAGAL dulu, baru laporkan.
```

### 3.5 `explorer.md` — penghemat token

```markdown
---
name: explorer
description: Mencari file, fungsi, atau pola di dalam repo. Gunakan untuk semua pencarian agar sesi utama tidak terisi hasil grep.
tools: Read, Grep, Glob
model: haiku
---

Anda pencari kode. Tugas Anda menemukan lokasi, bukan menganalisis.

Keluarkan HANYA:
- Path file
- Nomor baris
- Potongan 1–3 baris yang relevan

Jangan menjelaskan, jangan menyarankan perbaikan, jangan menampilkan
seluruh isi file. Maksimal 20 baris keluaran.
```

> **Kenapa `explorer` memakai Haiku (efisiensi):** pencarian file adalah pekerjaan mekanis. Memakai model termahal untuk `grep` adalah pemborosan terbesar dan paling umum di Claude Code. Yang lebih penting lagi: tanpa subagent ini, hasil pencarian yang panjang masuk ke context window utama dan ikut dibayar ulang di setiap giliran percakapan berikutnya.

### 3.6 Catatan penting tentang routing model

Penentuan model subagent mengikuti urutan: variabel lingkungan `CLAUDE_CODE_SUBAGENT_MODEL` → parameter saat pemanggilan → `model` di frontmatter → model sesi utama. Pernah ada laporan bahwa routing ini tidak selalu bekerja sesuai harapan pada beberapa versi. **Verifikasi di dashboard penggunaan** setelah beberapa sesi pertama: kalau pemakaian model termahal naik saat kamu memanggil subagent Haiku, routing-nya tidak jalan dan kamu perlu menyesuaikan strategi.

---

## 4. Skill Proyek

Simpan di `.claude/skills/<nama>/SKILL.md`. Skill dimuat hanya saat relevan, jadi ini tempat yang tepat untuk detail panjang.

### 4.1 `ledger-invariants`

```markdown
---
name: ledger-invariants
description: Aturan wajib untuk semua kode yang menyentuh uang, saldo, entry, atau transaksi ledger. Gunakan saat menulis atau meninjau kode di internal/domain, internal/service, atau internal/repository.
---

# Invariant Ledger

## 1. Representasi uang
- Selalu `domain.Money` (alias int64, satuan SEN).
- Rp 50.000 ditulis `50_000_00`, bukan `50000`.
- Penjumlahan memakai `.Add()` yang memeriksa overflow, bukan operator `+` langsung.

## 2. Aturan double-entry
Setiap transaksi wajib memenuhi: Σ(DEBIT) = Σ(CREDIT).
Panggil `txn.Validate()` SEBELUM menyentuh database.

Arah untuk tiap jenis akun:
| Akun | Kategori | Normal balance | Bertambah saat |
|---|---|---|---|
| USER_WALLET | Kewajiban | CREDIT | dikredit |
| SYSTEM_CASH | Aset | DEBIT | didebit |
| SYSTEM_FEE_REVENUE | Pendapatan | CREDIT | dikredit |

Rumus: delta = (direction == normal_balance) ? +amount : -amount

## 3. Konkurensi
- Kunci akun: `WHERE id = ANY($1) ORDER BY id FOR UPDATE` — urutan WAJIB.
- Update saldo selalu menyertakan `AND version = $n`.
- RowsAffected = 0 berarti konflik, bukan sukses.

## 4. Idempotency
- Klaim key DI DALAM transaksi database yang sama dengan pekerjaannya.
- Simpan response_body utuh agar retry menerima respons identik.
- Key sama + hash body beda → 409, jangan kembalikan hasil lama.

## 5. Append-only
`entries` tidak boleh di-UPDATE atau DELETE. Koreksi = transaksi reversal baru.

## 6. Checklist sebelum commit kode uang
- [ ] Tidak ada float
- [ ] Semua nominal lewat NewMoney()
- [ ] Validate() dipanggil sebelum posting
- [ ] Penguncian terurut
- [ ] Optimistic lock terpasang
- [ ] Ada test konkurensi
- [ ] Assert invariant di akhir test
```

### 4.2 `go-conventions`

```markdown
---
name: go-conventions
description: Konvensi kode Go untuk proyek ini — struktur paket, penanganan error, context, dan gaya penulisan. Gunakan saat menulis atau meninjau kode Go.
---

# Konvensi Go NusaLedger

## Struktur
transport → service → domain (satu arah, tidak boleh terbalik)
repository → domain
Interface didefinisikan di package konsumen.

## Error
- Domain: sentinel (`var ErrX = errors.New("...")`)
- Bungkus dengan konteks: `fmt.Errorf("mengambil akun %d: %w", id, err)`
- Periksa dengan `errors.Is` / `errors.As`, jangan bandingkan string
- Handler HTTP yang menerjemahkan error domain → status code

## Context
- Parameter pertama, bernama `ctx`
- Jangan simpan di struct
- `defer cancel()` selalu
- Key untuk `context.Value` bertipe privat

## Resource
`defer` ditulis segera setelah pengecekan error:
```go
rows, err := db.Query(ctx, q)
if err != nil { return fmt.Errorf("query: %w", err) }
defer rows.Close()
```

## Penamaan
- Paket: satu kata, huruf kecil (`domain`, `service`)
- Hindari stutter: `service.LedgerService` → cukup `service.Ledger`
- Pesan error: huruf kecil, tanpa titik akhir

## Larangan
- `panic` di jalur normal
- Mengabaikan error dengan `_` tanpa komentar
- Goroutine tanpa pemilik yang menunggunya
- `time.Sleep` untuk sinkronisasi
```

### 4.3 `test-strategy`

```markdown
---
name: test-strategy
description: Cara menulis test untuk proyek ini — table-driven, konkurensi, testcontainers, dan assert invariant. Gunakan saat menulis atau meninjau test.
---

# Strategi Test

## Gaya
Table-driven, subtest dengan `t.Run`, paralel bila aman.
Pesan kegagalan: `t.Fatalf("saldo: mau %d, dapat %d", want, got)`

## Helper invariant — panggil di akhir SETIAP test uang
```go
assertTrialBalanceZero(t, db)
assertNoNegativeWallet(t, db)
assertMaterializedBalanceMatchesEntries(t, db)
```

## Test konkurensi
Pola: N goroutine → WaitGroup → hitung sukses/gagal dengan atomic →
assert jumlah sukses → assert invariant.
Jalankan dengan `-race`. Tanpa `-race`, test konkurensi nyaris tidak berguna.

## Integration test
Build tag `//go:build integration`. Testcontainers PostgreSQL 17.
Migration dijalankan di `setupDB`. `t.Cleanup` untuk terminate container.

## Yang TIDAK perlu di-test
- Getter/setter sederhana
- Kode yang hanya memanggil pustaka pihak ketiga tanpa logika
- Handler HTTP yang hanya memetakan error (cukup satu test tabel)
```

---

## 5. `settings.json`

```json
{
  "permissions": {
    "allow": [
      "Bash(go build:*)", "Bash(go test:*)", "Bash(go vet:*)",
      "Bash(go mod:*)", "Bash(gofmt:*)",
      "Bash(make:*)",
      "Bash(migrate:*)",
      "Bash(docker compose:*)",
      "Bash(git status)", "Bash(git diff:*)", "Bash(git add:*)",
      "Bash(git commit:*)", "Bash(git log:*)"
    ],
    "deny": [
      "Bash(git push:*)",
      "Bash(rm -rf:*)",
      "Bash(psql:*production*)",
      "Read(./.env)",
      "Read(./.env.*)"
    ]
  }
}
```

> **Kenapa `git push` ditolak (🏛️ ARC):** commit itu lokal dan bisa dibatalkan; push itu publik dan sulit ditarik. Biarkan push selalu menjadi keputusan sadar manusia.
>
> **Kenapa `.env` ditolak dibaca:** supaya kredensial tidak pernah masuk ke context window — dan karenanya tidak pernah ikut terkirim dalam percakapan mana pun.

---

## 6. Routing Model & Effort per Tahap

| Tahap pekerjaan | Peran | Model | Effort | Alasan |
|---|---|---|---|---|
| Desain skema & arsitektur | senior-architect + senior-dba | Opus | Tinggi | Kesalahan di sini merambat ke semua kode. Ini titik paling layak dibayar mahal |
| Review keputusan konkurensi | senior-dba | Opus | Tinggi | Lost update & deadlock butuh penalaran mendalam |
| Implementasi domain & Money | senior-go-dev | Sonnet | Sedang | Spesifikasi sudah jelas, tinggal eksekusi teliti |
| Implementasi repository & service | senior-go-dev | Sonnet | Sedang | Sama |
| Implementasi handler & middleware | senior-go-dev | Sonnet | Rendah | Pekerjaan berpola, risiko rendah |
| Menulis test | senior-qa | Sonnet | Sedang | Butuh kreativitas kasus tepi |
| Review test konkurensi | senior-qa | Opus | Tinggi | Bagian tersulit untuk dinilai benar |
| Mencari file & fungsi | explorer | Haiku | Rendah | Mekanis |
| Perbaikan lint & format | (sesi utama) | Haiku/Sonnet | Rendah | Mekanis |
| Menulis dokumentasi & README | senior-go-dev | Sonnet | Rendah | — |
| Debugging bug yang membingungkan | senior-architect | Opus | Tinggi | Butuh hipotesis, bukan tebakan |

**Aturan sederhana:** Opus untuk **memutuskan**, Sonnet untuk **mengerjakan**, Haiku untuk **mencari**.

**Effort tinggi hanya untuk:** logika uang, konkurensi, keputusan skema, dan debugging yang buntu. Untuk selebihnya, effort tinggi hanya memperlambat tanpa menaikkan kualitas.

---

## 7. Sepuluh Praktik Hemat Token

Diurutkan dari dampak terbesar.

**1. Plan mode sebelum implementasi.** Minta rencana dulu, setujui, baru eksekusi. Kode yang dibongkar-pasang tiga kali jauh lebih mahal daripada satu rencana yang dibaca dulu.

**2. `/clear` setelah setiap tugas selesai.** Ini yang paling sering dilupakan. Tanpa `/clear`, seluruh percakapan sebelumnya ikut dikirim ulang di setiap giliran. Satu sesi panjang bisa 10× lebih mahal daripada lima sesi pendek yang setara.

**3. `HANDOVER.md` sebagai jembatan sesi.** Karena kamu memakai `/clear` sering, konteks harus dipindahkan ke file, bukan disimpan di percakapan.

```markdown
# HANDOVER

## Terakhir dikerjakan (2026-09-13)
Selesai: domain.Money + Transaction.Validate + unit test (T-01, T-14 hijau)

## Berikutnya
Implementasi LedgerRepo.PostTransaction sesuai docs/03 §5

## Keputusan yang sudah diambil
- Saldo dimaterialisasi + version, entries tetap sumber kebenaran
- Fee transfer flat dari config, bukan hardcode

## Catatan / jebakan
- Trigger balanced bersifat DEFERRED; error baru muncul saat Commit(), bukan Exec()
- testcontainers butuh Docker jalan; `make test` (unit) tidak butuh

## Belum diputuskan
- Format cursor pagination: id mentah atau base64?
```

**4. Sebut file secara spesifik.** `@internal/service/ledger_service.go` jauh lebih hemat daripada "lihat service transfer" yang memicu pencarian ke seluruh repo.

**5. Delegasikan pencarian ke `explorer`.** Hasil grep panjang tidak pernah masuk ke context utama.

**6. CLAUDE.md ramping.** Setiap baris di sana dibayar di setiap sesi. Kalau sudah lewat 120 baris, pindahkan ke skill.

**7. Satu tugas per sesi.** "Implementasikan PostTransaction" — bukan "implementasikan repository, service, handler, dan testnya".

**8. Jangan minta Claude membaca file yang kamu sudah tahu isinya.** Tempelkan potongan yang relevan saja.

**9. Commit kecil dan sering.** Diff kecil = review murah. Diff 2.000 baris memaksa pembacaan ulang seluruh konteks.

**10. Brainstorm di ponsel, eksekusi di laptop.** Diskusi desain di claude.ai lalu tuangkan kesimpulannya ke `HANDOVER.md`; Claude Code memulai dari keputusan yang sudah matang, bukan dari nol.

---

## 8. Contoh Sesi Kerja yang Benar

```bash
# ── Sesi 1: desain (Opus, effort tinggi) ────────────────────
claude
> Baca docs/02-SKEMA-DATABASE.md §2.5 dan §3.1.
> Pakai subagent senior-dba: tinjau rancangan penguncian akun untuk transfer.
> Fokus: apakah ada jalur yang bisa deadlock atau lost update?
> Jangan tulis kode.

# [baca rekomendasi, putuskan, catat di HANDOVER.md]
/clear

# ── Sesi 2: implementasi (Sonnet, effort sedang) ────────────
> Baca HANDOVER.md.
> Pakai subagent senior-go-dev: implementasikan LedgerRepo.PostTransaction
> sesuai docs/03-SPESIFIKASI-TEKNIS-FASE-1.md §5.
> Ikuti skill ledger-invariants. Jangan tulis test dulu.

/clear

# ── Sesi 3: pengujian (Sonnet, effort sedang) ───────────────
> Baca HANDOVER.md dan @internal/repository/postgres/ledger_repo.go.
> Pakai subagent senior-qa: tulis test T-04, T-05, T-07 sesuai docs/03 §7.2.
> Sertakan assert invariant. Jalankan dengan -race.

/clear

# ── Sesi 4: review (Opus, effort tinggi) ────────────────────
> Pakai subagent senior-architect: tinjau @internal/repository/postgres/ledger_repo.go
> dan @internal/service/ledger_service.go.
> Fokus: arah dependensi, penempatan tanggung jawab, mode kegagalan.
> Jangan ubah file.
```

Perhatikan polanya: **satu sesi, satu peran, satu tujuan, lalu `/clear`.** Ini sekaligus yang paling hemat dan paling menghasilkan kualitas, karena setiap peran bekerja dengan konteks yang bersih dan tidak bias oleh pekerjaan sebelumnya.

---

## 9. Pemantauan Anggaran

**Mingguan:**
- Cek dashboard penggunaan: apakah model termahal dipakai untuk pekerjaan mekanis?
- Kalau kuota Pro sering habis sebelum waktunya, penyebab tersering adalah lupa `/clear`

**Untuk Fase 1, kredit API seharusnya terpakai nyaris nol** — Fase 1 tidak memanggil LLM saat runtime. Kredit baru mulai terpakai di Fase 4. Kalau kredit berkurang selama Fase 1, periksa `ANTHROPIC_API_KEY` di lingkunganmu.
