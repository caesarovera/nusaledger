-- Fase 2: consumer wajib idempoten (MESSAGE-QUEUE-GO.md §4.2 "Strategi 1 — Deduplikasi
-- dengan tabel"). RabbitMQ hanya menjamin AT-LEAST-ONCE — pesan yang sama BISA sampai
-- dua kali (consumer crash setelah proses tapi sebelum ack, requeue, dsb). UNIQUE pada
-- event_id membuat pemrosesan ganda ditolak database, bukan diharapkan dari disiplin kode.
CREATE TABLE processed_events (
    event_id     UUID        PRIMARY KEY,
    consumer     TEXT        NOT NULL,   -- nama consumer; satu event bisa diproses BEBERAPA consumer berbeda
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
