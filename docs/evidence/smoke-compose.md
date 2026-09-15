# Bukti smoke test `docker compose up` dari nol (Sesi 28, 2026-09-16)

Perintah: `docker compose down -v && docker compose up -d` (image dibangun dari Dockerfile; migrasi dijalankan API saat start).

- Image `nusaledger:dev`: **18.6 MB** (distroless static, nonroot, target < 20 MB)
- `/readyz` 200 setelah ~4 detik dari nol (termasuk Postgres healthy + migrasi)

```
GET  /healthz → ok [200]
GET  /readyz  → ready [200]
POST /auth/register (andi) → {"data":{"user":{"public_id":"4c8cf7f6-126d-44bb-8a92-e8a4f7612460","email":"andi@demo.local","full_name":"Andi","role":"USER","created_at":"2026-09-1...
POST /auth/register (budi) → wallet fa0355de-0701-4eb1-8468-24155e466693
POST /auth/login (andi) → access_token eyJhbGciOiJIUzI1NiIs…
POST /transactions/topup TANPA Idempotency-Key → {"error":{"code":"IDEMPOTENCY_KEY_REQUIRED","message":"header Idempotency-Key wajib untuk operasi uang","fields":null,"request_id":"3a9ce7d36e23547f9b74e02daf8ac52a"}}
POST /transactions/topup Rp 100.000 → {"data":{"transaction_id":"f778a0dc-954e-4e43-b256-8f22680176df","type":"TOPUP","status":"POSTED","description":"isi","amount_sen":10000000,"fee_sen":0,"created…
POST /transactions/topup (retry key sama) → respons BYTE-IDENTIK, saldo tidak naik dua kali
POST /transactions/topup (key sama, body beda) → {"error":{"code":"IDEMPOTENCY_CONFLICT","message":"idempotency key sudah dipakai dengan isi permintaan berbeda","fields":null,"request_id":"fab06a61a7ad545acb8a6289d337f504"}}
POST /transactions/transfer Rp 50.000 → Budi → {"data":{"transaction_id":"44bd050c-bde1-49f4-86a1-cb7429294a10","type":"TRANSFER","status":"POSTED","description":"bayar kos","amount_sen":5000000,"fee_sen":100000,"created_at":"2026-09-16T05:44:01.655657+07:00","entrie…
POST /transactions/transfer Rp 5.000 (di bawah minimum) → {"error":{"code":"AMOUNT_OUT_OF_RANGE","message":"nominal di luar batas yang diizinkan","fields":null,"request_id":"b93c7b87de3694a90b3e877e7e814631"}}
GET  /accounts/me (andi) → {"data":{"public_id":"61709c51-3460-4c29-872f-ae38d1047ed8","type":"USER_WALLET","status":"ACTIVE","currency":"IDR","balance_sen":4900000,"updated_at":"2026-09-16T05:44:01.655657+07:00"}}
GET  /accounts/me/entries?limit=1 → {"data":[{"id":3,"transaction_id":"44bd050c-bde1-49f4-86a1-cb7429294a10","account_public_id":"61709c51-3460-4c29-872f-ae38d1047ed8","account_type":"USER_WALLET","directio…
GET  /accounts/me tanpa token → {"error":{"code":"UNAUTHENTICATED","message":"autentikasi gagal","fields":null,"request_id":"d39756f5c8c8deeffe19275afb60c7a0"}}
GET  /transactions/{id-acak} → {"error":{"code":"TRANSACTION_NOT_FOUND","message":"transaksi tidak ditemukan","fields":null,"request_id":"791a4f93fe40827ea258b9add2f09d34"}}
GET  /metrics (cuplikan) →
    http_request_duration_seconds_count{method="POST",route="/api/v1/transactions/transfer",status="201"} 1
    http_request_duration_seconds_count{method="POST",route="/api/v1/transactions/transfer",status="422"} 1
    ledger_balance_drift_total 0
    ledger_transactions_total{status="failed",type="TOPUP"} 1
    ledger_transactions_total{status="failed",type="TRANSFER"} 1
    ledger_transactions_total{status="ok",type="TOPUP"} 2
    ledger_transactions_total{status="ok",type="TRANSFER"} 1
    ledger_trial_balance_difference 0
```

Satu baris log JSON terstruktur dari container (request_id, user_id, route, status, duration_ms):

```json
{"time":"2026-09-16T05:44:02.074856486+07:00","level":"INFO","msg":"request","app":"nusaledger","version":"dev","env":"development","request_id":"b93c7b87de3694a90b3e877e7e814631","method":"POST","route":"/api/v1/transactions/transfer","path":"/api/v1/transactions/transfer","status":422,"duration_ms":0,"bytes":152,"ip":"172.21.0.1:47082"}
```
