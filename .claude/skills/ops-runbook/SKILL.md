---
name: ops-runbook
description: Cara membangun, menjalankan, mengukur, dan memverifikasi NusaLedger — perintah make, compose, k6, metrik wajib, health check, log, image. Gunakan untuk pekerjaan Docker, CI, load test, observability.
---

# Runbook Operasional

## Perintah
`make help` menampilkan semuanya: test · test-int · race · cover · lint · fmt · migrate-up/down/drop · up · down · run · build

## Metrik wajib (docs/03 §8)
```
http_request_duration_seconds{method,route,status}   histogram
http_requests_total{method,route,status}
db_pool_conns_in_use · db_pool_conns_max              gauge
ledger_transactions_total{type,status}
ledger_balance_drift_total                             HARUS 0 — alert keras
ledger_trial_balance_difference                        HARUS 0
auth_login_failures_total{reason}
```
Route label memakai pola chi (`/transactions/{id}`), bukan path mentah — cardinality terkendali.

## Health
- `/healthz`: proses hidup, TANPA DB. Dipakai liveness.
- `/readyz`: `pool.Ping(ctx 2s)`. Dipakai readiness. Saat shutdown, readyz mulai mengembalikan 503 sebelum server berhenti.

## Graceful shutdown
`signal.NotifyContext(SIGINT, SIGTERM)` → tandai not-ready → `srv.Shutdown(ctx SHUTDOWN_WAIT)` → hentikan ticker drift → `pool.Close()`.

## pprof
Server terpisah di `127.0.0.1:6060`, mux sendiri, hanya bila `APP_ENV != production`.

## Load test
```
k6 run test/load/transfer.js
```
Threshold: p95 < 200 ms, p99 < 500 ms, error < 1 % @100 VU.
SETELAH load test WAJIB: `GET /internal/ledger/trial-balance` → `difference == 0`. Simpan hasil di docs/evidence/k6.md.

## Image
`docker build -t nusaledger .` → `docker images nusaledger` → < 20 MB. Distroless static nonroot, tanpa shell.

## Log
`slog` JSON. Field wajib: `request_id`, `user_id`, `transaction_id`, `duration_ms`, `route`, `status`.
Redaksi terpusat: `password`, `token`, `authorization`, `refresh_token` tidak pernah masuk log.

## CI (GitHub Actions)
lint → unit → race → integration (Docker tersedia di ubuntu-latest) → build image. Gagal cepat, cache modul Go.
