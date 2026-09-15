# NusaLedger — Instruksi Proyek

Ledger double-entry untuk dompet digital. Go 1.27 + PostgreSQL 17.
Ini portofolio teknis, bukan produk keuangan sungguhan.

## Dokumen acuan (baca HANYA saat relevan, jangan otomatis)
- Aturan bisnis & scope  → docs/01-PRD-FASE-1.md
- Skema & query          → docs/02-SKEMA-DATABASE.md
- Arsitektur & API       → docs/03-SPESIFIKASI-TEKNIS-FASE-1.md
- Keputusan yang dikunci → docs/06-PLAN-EKSEKUSI-AI.md §2
- Status terakhir        → HANDOVER.md
- Jurnal belajar         → docs/JURNAL-BELAJAR.md (WAJIB ditambah setiap langkah selesai)

## Invariant yang TIDAK BOLEH dilanggar
1. Uang selalu `domain.Money` (int64 sen). Dilarang float di seluruh repo.
2. Setiap transaksi: total debit = total kredit; satu akun maks satu entry.
3. `entries` append-only. Tidak ada UPDATE/DELETE.
4. Semua operasi uang wajib Idempotency-Key. PK idempotency = (user_id, key).
5. Kunci akun selalu `ORDER BY id` sebelum FOR UPDATE.
6. Saldo USER_WALLET tidak boleh negatif.
7. Query SQL selalu berparameter ($1, $2). Tidak ada fmt.Sprintf untuk SQL.

## Aturan kode
- Arah dependensi: transport → service → domain. domain tidak impor apa pun.
- Interface didefinisikan di sisi konsumen (package service), bukan repository.
- Error dibungkus `fmt.Errorf("konteks: %w", err)`. Error domain = sentinel.
- Setiap fungsi yang menyentuh I/O menerima `ctx context.Context` sebagai argumen pertama.
- Pesan error huruf kecil, tanpa titik. Komentar dan pesan error: Bahasa Indonesia.
- Cursor pagination = base64url(id). ErrIdempotencyInFlight → 409 IDEMPOTENCY_IN_FLIGHT.
- Dependensi yang boleh: docs/06 §2.3. Di luar itu, tanya dulu.

## Perintah
- make test          # unit, cepat
- make test-int      # integration + testcontainers
- make race          # go test -race ./...
- make lint          # go vet + golangci-lint + govulncheck
- make migrate-up

## Alur kerja per sesi (WAJIB)
1. Baca HANDOVER.md
2. Rencanakan (plan mode) sebelum menulis kode uang
3. Implementasi
4. Jalankan test yang relevan saja, bukan seluruh suite
5. Tambah entri docs/JURNAL-BELAJAR.md (Apa / Kenapa / Contoh / Bukti)
6. Update HANDOVER.md
7. git commit dengan pesan konvensional (feat:, fix:, test:, docs:, chore:)

## Larangan
- Jangan membaca seluruh codebase. Baca file yang disebut saja.
- Jangan buat file baru bila bisa mengedit yang ada.
- Jangan menambah dependensi di luar daftar tanpa ditanyakan lebih dulu.
- Jangan menulis komentar yang mengulang isi kode.
- Jangan git push. Push adalah keputusan manusia.
