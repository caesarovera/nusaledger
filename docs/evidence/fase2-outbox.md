# Bukti Fase 2 — Outbox relay, RabbitMQ, consumer idempoten (Sesi 34, 2026-09-16)

Perintah: `docker compose down -v && docker compose up -d --build` (api + worker + postgres + rabbitmq, satu perintah dari nol).

## Ukuran image
```
nusaledger-api:    18,6 MB (tidak berubah)
nusaledger-worker: 12,9 MB
```

## Alur nyata lewat API sungguhan (bukan test)

```
POST /transactions/topup -> 201 {transaction_id: c73cfeff-...}

outbox_events:    1 baris, published_at TERISI (relay mengirim dalam ~1 detik, sesuai RELAY_INTERVAL)
processed_events: 1 baris, consumer=audit-log, event_id b53f0ee8-... (SAMA dengan yang di-generate DB, dibawa lewat AMQP MessageId)

Log worker:
{"msg":"relay mengirim event","jumlah":1}
{"msg":"audit: transaksi diposting","event_id":"b53f0ee8-...","transaction_id":"c73cfeff-...","type":"TOPUP","status":"POSTED"}
```

## Test otomatis

```
TestOutbox_RelayDanConsumer_EndToEnd (RabbitMQ sungguhan via testcontainers, -race): PASS
  - outbox ditulis oleh Post() -> belum terkirim (published_at NULL)
  - relay.PollOnce() -> 1 event terkirim dengan publisher confirm, published_at terisi
  - consumer memproses -> processed_events 1 baris
  - REDELIVERY event yang SAMA (simulasi at-least-once) -> processed_events TETAP 1 baris (idempoten, bukan duplikat/error)
```

## Graceful shutdown worker

```
$ docker compose stop -t 10 worker
{"time":"2026-09-16T09:49:12.714884844+07:00","level":"INFO","msg":"sinyal berhenti diterima, menyelesaikan pekerjaan yang sedang berjalan","app":"nusaledger-worker"}
```
