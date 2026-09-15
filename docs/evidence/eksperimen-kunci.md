# Eksperimen Sesi 20 — melepas kunci, melihat apa yang terjadi (2026-09-16)

Kode asli: `SELECT ... ORDER BY id FOR UPDATE` lalu `UPDATE ... WHERE id = $2 AND version = $3`. Setiap varian dijalankan pada T-04 (100 goroutine, saldo Rp 1.000.000, transfer Rp 50.000 + fee Rp 1.000; jawaban benar = 19 sukses, sisa Rp 31.000) dan T-07 (50× transfer silang A↔B). Kode dipulihkan dengan `git checkout` setelahnya; analisis di docs/JURNAL-BELAJAR.md Sesi 20.

## Varian A — tanpa `FOR UPDATE`, hanya optimistic lock `version`

```
concurrency_test.go:77: sukses=8 saldo_kurang=0 konflik=92 saldo_pengirim=Rp 592000,00
concurrency_test.go:84: transfer sukses: mau 19, dapat 8
--- FAIL: TestConcurrency_T04_TransferParalelDariSatuAkun (0.30s)
concurrency_test.go:186: error tak terduga: data berubah oleh proses lain, coba lagi: akun 5
concurrency_test.go:190: terjadi 98 deadlock — urutan penguncian rusak
--- FAIL: TestConcurrency_T07_TransferSilangTanpaDeadlock (9.12s)
```

## Varian B — tanpa `FOR UPDATE` DAN tanpa `version`

```
concurrency_test.go:74: error tak terduga: update saldo akun 4: expected 2 arguments, got 3
concurrency_test.go:74: error tak terduga: update saldo akun 4: expected 2 arguments, got 3
concurrency_test.go:74: error tak terduga: update saldo akun 4: expected 2 arguments, got 3
concurrency_test.go:74: error tak terduga: update saldo akun 4: expected 2 arguments, got 3
concurrency_test.go:74: error tak terduga: update saldo akun 4: expected 2 arguments, got 3
concurrency_test.go:74: error tak terduga: update saldo akun 4: expected 2 arguments, got 3
concurrency_test.go:74: error tak terduga: update saldo akun 4: expected 2 arguments, got 3
concurrency_test.go:74: error tak terduga: update saldo akun 4: expected 2 arguments, got 3
concurrency_test.go:74: error tak terduga: update saldo akun 4: expected 2 arguments, got 3
concurrency_test.go:74: error tak terduga: update saldo akun 4: expected 2 arguments, got 3
concurrency_test.go:74: error tak terduga: update saldo akun 4: expected 2 arguments, got 3
concurrency_test.go:74: error tak terduga: update saldo akun 4: expected 2 arguments, got 3
concurrency_test.go:74: error tak terduga: update saldo akun 4: expected 2 arguments, got 3
concurrency_test.go:74: error tak terduga: update saldo akun 4: expected 2 arguments, got 3
```
