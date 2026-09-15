---
name: go-conventions
description: Konvensi kode Go untuk proyek ini — struktur paket, penanganan error, context, resource, penamaan, dan gaya penulisan. Gunakan saat menulis atau meninjau kode Go.
---

# Konvensi Go NusaLedger

## Struktur
transport → service → domain (satu arah, tidak boleh terbalik)
repository → domain
Interface didefinisikan di package konsumen (`service/ports.go`), kecil (2–4 method).
DI manual di `cmd/api/main.go`: rakit dari bawah (pool → repo → service → handler → router).

## Error
- Domain: sentinel (`var ErrX = errors.New("...")`) di `domain/errors.go`
- Bungkus dengan konteks: `fmt.Errorf("mengambil akun %d: %w", id, err)`
- Periksa dengan `errors.Is` / `errors.As`, jangan bandingkan string
- Hanya `transport/http/response.go` yang menerjemahkan error domain → status code
- Error pgx diterjemahkan ke error domain di repository (`errors.As(err, &pgErr)` + `pgErr.Code`)

## Context
- Parameter pertama, bernama `ctx`
- Jangan simpan di struct
- `defer cancel()` selalu
- Key untuk `context.Value` bertipe privat (`type ctxKey int`)

## Resource
`defer` ditulis segera setelah pengecekan error:
```go
rows, err := db.Query(ctx, q, args...)
if err != nil { return fmt.Errorf("query: %w", err) }
defer rows.Close()
```
```go
tx, err := pool.Begin(ctx)
if err != nil { return fmt.Errorf("begin: %w", err) }
defer tx.Rollback(ctx) // no-op setelah Commit
```

## Penamaan
- Paket: satu kata, huruf kecil (`domain`, `service`, `postgres`)
- Hindari stutter: `service.Ledger`, bukan `service.LedgerService`
- Pesan error: huruf kecil, tanpa titik akhir, Bahasa Indonesia
- Konstruktor `NewX(...)` mengembalikan `*X`; dependensi lewat argumen, bukan global

## Larangan
- `panic` di jalur normal
- Mengabaikan error dengan `_` tanpa komentar
- Goroutine tanpa pemilik yang menunggunya
- `time.Sleep` untuk sinkronisasi
- `init()` dengan efek samping
- Variabel global mutable
