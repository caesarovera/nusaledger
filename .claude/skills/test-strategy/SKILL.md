---
name: test-strategy
description: Cara menulis test untuk proyek ini — table-driven, konkurensi, testcontainers, helper invariant, build tag. Gunakan saat menulis atau meninjau test.
---

# Strategi Test

## Piramida
Unit 70 % (domain, service dengan stub) · Integration 25 % (repository + Postgres testcontainers) · E2E 5 % (httptest + DB)

## Gaya
Table-driven, subtest dengan `t.Run`, `t.Parallel()` bila aman.
Pesan kegagalan: `t.Fatalf("saldo: mau %d, dapat %d", want, got)`
Uji jenis error: `if !errors.Is(err, domain.ErrX)`

## Helper invariant — panggil di akhir SETIAP test uang
```go
assertTrialBalanceZero(t, db)
assertNoNegativeWallet(t, db)
assertMaterializedBalanceMatchesEntries(t, db)
```
Query-nya: docs/02 §3.4 dan §3.5.

## Test konkurensi
Pola: N goroutine → `sync.WaitGroup` → hitung sukses/gagal dengan `atomic.Int64` →
assert jumlah sukses → assert invariant. Key idempotency BERBEDA per goroutine (kecuali T-05).
Jalankan: `go test -race -tags=integration -run Concurrent -count=5 ./test/...`

## Integration test
- Build tag `//go:build integration` di baris pertama file.
- `setupDB(t)` menyalakan `postgres:17-alpine` via testcontainers, menjalankan migration dari `../../migrations`, `t.Cleanup` terminate.
- Satu container per paket test (`TestMain`), bukan per test — hemat 5–10 detik per test.
- Isolasi antar test: `TRUNCATE ... RESTART IDENTITY CASCADE` pada tabel data, seed ulang akun sistem.

## Stub untuk unit test service
Struct kecil yang mengimplementasikan interface di `service/ports.go`, dengan field fungsi:
```go
type stubRepo struct{ post func(ctx context.Context, ...) (..., error) }
```

## Yang TIDAK perlu di-test
- Getter/setter sederhana
- Kode yang hanya memanggil pustaka pihak ketiga tanpa logika
- Handler HTTP yang hanya memetakan error (cukup satu test tabel di response_test.go)
