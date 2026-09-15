---
name: api-contract
description: Kontrak HTTP proyek ini — envelope respons, katalog kode error ke status HTTP, aturan DTO, header Idempotency-Key, urutan middleware, timeout server. Gunakan saat menulis atau meninjau handler, DTO, middleware, atau openapi.yaml.
---

# Kontrak API NusaLedger  (base path `/api/v1`)

## Envelope
Sukses: `{"data": ...}` · Daftar: `{"data": [...], "next_cursor": "..."|null}`
Error: `{"error": {"code": "...", "message": "...", "fields": {...}|null, "request_id": "..."}}`

## Kode error → HTTP (docs/03 §4.3 + F-03)
| Kode | HTTP |
|---|---|
| VALIDATION_ERROR, IDEMPOTENCY_KEY_REQUIRED | 400 |
| UNAUTHENTICATED | 401 |
| FORBIDDEN | 403 |
| ACCOUNT_NOT_FOUND, TRANSACTION_NOT_FOUND | 404 |
| IDEMPOTENCY_CONFLICT, IDEMPOTENCY_IN_FLIGHT (+`Retry-After: 1`), ALREADY_REVERSED, CONCURRENT_MODIFICATION | 409 |
| PAYLOAD_TOO_LARGE | 413 |
| INSUFFICIENT_BALANCE, ACCOUNT_NOT_ACTIVE, SELF_TRANSFER, AMOUNT_OUT_OF_RANGE | 422 |
| RATE_LIMITED | 429 |
| INTERNAL_ERROR | 500 |

Pemetaan `errors.Is(err, domain.ErrX)` → kode ada di SATU tempat: `transport/http/response.go`.
Pesan 500 tidak pernah memuat detail internal; detail masuk log dengan request_id.

## Endpoint
| Method | Path | Auth | Idempotency-Key |
|---|---|---|---|
| POST | /auth/register, /auth/login, /auth/refresh | — | — |
| POST | /auth/logout | ✅ | — |
| GET | /accounts/me, /accounts/me/entries?cursor=&limit= | ✅ | — |
| POST | /transactions/topup, /transfer, /withdraw | ✅ | wajib |
| GET | /transactions/{id} | ✅ | — |
| POST | /transactions/{id}/reverse | ✅ ADMIN | wajib |
| GET | /internal/ledger/trial-balance | ✅ ADMIN | — |
| GET | /healthz, /readyz, /metrics | — | — |

## DTO
- Nominal selalu bernama `*_sen` (int64). ID publik selalu UUID (`public_id`), tidak pernah id internal.
- `json.NewDecoder(r.Body)` + `DisallowUnknownFields()`; body dibatasi `http.MaxBytesReader(w, r.Body, 1<<20)`.
- Validasi manual di method `Validate() map[string]string` pada DTO; hasil masuk `fields`.
- DTO di `dto.go` tidak pernah dipakai di service; konversi eksplisit di handler.

## Idempotency-Key
Wajib di POST topup/transfer/withdraw/reverse. 1–128 karakter. Tanpa header → 400 IDEMPOTENCY_KEY_REQUIRED.
`request_hash = hex(SHA-256(method + path + body mentah))`; user_id sudah jadi bagian PK.

## Urutan middleware
Recover → RequestID → RealIP → Logger → Timeout(30s) → MaxBytes → [Auth] → [RateLimit] → handler
Recover di paling luar supaya panic di middleware lain pun tertangkap.

## Timeout server
ReadHeaderTimeout 5s · ReadTimeout 15s · WriteTimeout 30s · IdleTimeout 60s

## Auth
- Access JWT HS256 15 menit; validasi `alg` eksplisit (`jwt.WithValidMethods`).
- Refresh 32 byte acak, disimpan SHA-256, 30 hari, bisa dicabut.
- Login: pesan gagal selalu sama, dummy-hash bila email tidak ada (anti enumerasi).
- Rate limit login 5/15 menit per email + per IP; transfer 20/menit per user; fail-closed.
