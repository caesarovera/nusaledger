---
name: ledger-invariants
description: Aturan wajib untuk semua kode yang menyentuh uang, saldo, entry, atau transaksi ledger. Gunakan saat menulis atau meninjau kode di internal/domain, internal/service, atau internal/repository.
---

# Invariant Ledger

## 1. Representasi uang
- Selalu `domain.Money` (int64, satuan SEN). Rp 50.000 ditulis `50_000_00`, bukan `50000`.
- Penjumlahan memakai `.Add()` yang memeriksa overflow, bukan operator `+` langsung.
- Semua nominal dari luar lewat `NewMoney()`.

## 2. Aturan double-entry
Setiap transaksi wajib memenuhi: Σ(DEBIT) = Σ(CREDIT). Minimal dua entry. Satu akun maksimal satu entry (K-02).
Panggil `txn.Validate()` SEBELUM menyentuh database.

| Akun | Kategori | Normal balance | Bertambah saat |
|---|---|---|---|
| USER_WALLET | Kewajiban | CREDIT | dikredit |
| SYSTEM_CASH | Aset | DEBIT | didebit |
| SYSTEM_FEE_REVENUE | Pendapatan | CREDIT | dikredit |
| SYSTEM_SUSPENSE | Kewajiban | CREDIT | dikredit |

Rumus: `delta = (direction == normal_balance) ? +amount : -amount`

Operasi Fase 1:
- TOPUP: DEBIT SYSTEM_CASH, CREDIT USER_WALLET
- TRANSFER: DEBIT pengirim (amount+fee), CREDIT penerima (amount), CREDIT SYSTEM_FEE_REVENUE (fee)
- WITHDRAW: DEBIT USER_WALLET, CREDIT SYSTEM_CASH
- REVERSAL: semua entry transaksi asli dibalik arahnya; `reverses_transaction_id` diisi; transaksi asli hanya berubah `status`

## 3. Konkurensi
- Kunci akun: `WHERE id = ANY($1) ORDER BY id FOR UPDATE` — urutan WAJIB.
- Update saldo selalu menyertakan `AND version = $n AND status = 'ACTIVE'`.
- RowsAffected = 0 berarti konflik (ErrConcurrentModification), bukan sukses.
- CHECK `chk_wallet_non_negative` → ErrInsufficientBalance.

## 4. Idempotency
- Klaim key (`INSERT ... ON CONFLICT (user_id, key) DO NOTHING`) DI DALAM transaksi database yang sama dengan pekerjaannya.
- RowsAffected = 0 saat klaim → ErrIdempotencyInFlight (409, Retry-After: 1).
- Simpan response_body utuh agar retry menerima respons identik.
- Key sama + hash body beda → ErrIdempotencyConflict (409), jangan kembalikan hasil lama.

## 5. Append-only
`entries` tidak boleh di-UPDATE atau DELETE. Koreksi = transaksi reversal baru.
`transactions` hanya boleh UPDATE kolom `status`.

## 6. Checklist sebelum commit kode uang
- [ ] Tidak ada float
- [ ] Semua nominal lewat NewMoney()
- [ ] Validate() dipanggil sebelum posting
- [ ] Penguncian terurut
- [ ] Optimistic lock terpasang
- [ ] Ada test konkurensi
- [ ] Assert invariant di akhir test
