# NusaLedger

> ⚠️ **Ini simulasi teknis, bukan produk keuangan.** Tidak ada uang sungguhan, tidak ada integrasi bank nyata, tidak ada lisensi regulator.

Ledger *double-entry* untuk dompet digital, ditulis dengan Go 1.27 dan PostgreSQL 17. Dibangun sebagai portofolio untuk menunjukkan **correctness uang**, **konkurensi database**, dan **idempotency**, bukan sekadar CRUD.

## Status

🚧 Fase 1 sedang dibangun. Lihat `HANDOVER.md` untuk posisi terakhir dan `docs/JURNAL-BELAJAR.md` untuk catatan langkah demi langkah.

## Menjalankan

```bash
docker compose up -d postgres   # PostgreSQL 17
make migrate-up                 # skema + trigger + seed akun sistem
make run                        # API di :8080
```

## Arsitektur

```
transport/http  →  service  →  domain
repository      →  domain
```

Diagram lengkap dan tabel keputusan teknis menyusul di akhir Fase 1.

## Keputusan Teknis

| Keputusan | Alternatif yang ditolak | Alasan |
|---|---|---|
| Saldo dimaterialisasi + `version` | Selalu `SUM(entries)` | Baca instan & cek saldo atomik; `entries` tetap sumber kebenaran, diverifikasi job drift |
| Idempotency key wajib, PK `(user_id, key)` | Andalkan transaksi DB saja / key global | Transaksi DB tidak menutup kasus ACK hilang setelah commit; key global bisa saling mengganggu antar user |
| Kunci akun `ORDER BY id` | Kunci sesuai urutan permintaan | Menghilangkan deadlock transfer silang |
| Trigger `DEFERRED` untuk balanced | Cek di aplikasi saja | Berlaku juga untuk migrasi & perbaikan data manual |

## Dokumen

`docs/00` sampai `docs/06`. Urutan baca: 00 → 01 → 02 → 03 → 06.
