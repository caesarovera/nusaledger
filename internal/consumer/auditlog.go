// Package consumer berisi consumer Fase 2. Satu-satunya yang ada saat ini adalah
// AuditLog — mencatat setiap event ledger.transaction.posted.v1 secara idempoten,
// membuktikan pipeline outbox→relay→broker→consumer bekerja end-to-end.
package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/caesarovera/nusaledger/internal/platform/broker"
)

const name = "audit-log"

// AuditLog adalah consumer IDEMPOTEN: event yang sama (event_id sama) yang datang
// dua kali — wajar untuk RabbitMQ yang at-least-once — hanya diproses SEKALI.
// MESSAGE-QUEUE-GO.md §4.2 "Strategi 1: Deduplikasi dengan tabel".
type AuditLog struct {
	db  *pgxpool.Pool
	log *slog.Logger
}

func NewAuditLog(db *pgxpool.Pool, log *slog.Logger) *AuditLog {
	return &AuditLog{db: db, log: log}
}

// minimal payload yang dibaca — bagian dari storedResult (repository/postgres/ledger_repo.go),
// TIDAK mengimpor tipe itu (paket private); consumer sengaja hanya membaca yang perlu
// dicatat, mencontohkan "skema event ber-versi, consumer lama tidak rusak saat payload
// berubah" (MESSAGE-QUEUE-GO §8 checklist).
type transactionPostedPayload struct {
	TransactionID string `json:"transaction_id"`
	Type          string `json:"type"`
	Status        string `json:"status"`
}

// Run mengonsumsi sampai ctx selesai. Qos(prefetch) membatasi beban per worker;
// autoAck=false WAJIB — ack hanya setelah baris processed_events benar-benar tersimpan.
func (a *AuditLog) Run(ctx context.Context, ch *amqp.Channel) error {
	if err := ch.Qos(10, 0, false); err != nil {
		return fmt.Errorf("set qos: %w", err)
	}
	deliveries, err := ch.ConsumeWithContext(ctx, broker.QueueAuditLog, name, false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume %s: %w", broker.QueueAuditLog, err)
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case d, ok := <-deliveries:
			if !ok {
				return nil // channel/koneksi ditutup — worker main yang menangani reconnect
			}
			a.handle(ctx, d)
		}
	}
}

// handle memutuskan ack/nack. Panic pada goroutine consumer TIDAK boleh mematikan
// seluruh worker (beda proses dari cmd/api, tapi prinsipnya sama: setiap goroutine
// yang menangani pesan luar butuh recovery sendiri).
func (a *AuditLog) handle(ctx context.Context, d amqp.Delivery) {
	defer func() {
		if r := recover(); r != nil {
			a.log.ErrorContext(ctx, "panic saat memproses event, dikirim ke DLQ", "panic", r, "message_id", d.MessageId)
			_ = d.Nack(false, false)
		}
	}()

	eventID, err := uuid.Parse(d.MessageId)
	if err != nil {
		// message_id rusak = pesan cacat permanen, retry tidak akan memperbaikinya.
		a.log.ErrorContext(ctx, "message_id bukan UUID, dikirim ke DLQ", "message_id", d.MessageId, "err", err)
		_ = d.Nack(false, false)
		return
	}

	ct, err := a.db.Exec(ctx, `
		INSERT INTO processed_events (event_id, consumer) VALUES ($1, $2)
		ON CONFLICT (event_id) DO NOTHING`, eventID, name)
	if err != nil {
		// Kegagalan DB biasanya sementara (koneksi, timeout) — requeue, JANGAN DLQ.
		a.log.ErrorContext(ctx, "gagal mencatat processed_events, requeue", "event_id", eventID, "err", err)
		_ = d.Nack(false, true)
		return
	}
	if ct.RowsAffected() == 0 {
		// Sudah pernah diproses (redelivery at-least-once) — ack tanpa mengulang kerja.
		a.log.InfoContext(ctx, "event sudah pernah diproses, dilewati (idempoten)", "event_id", eventID)
		_ = d.Ack(false)
		return
	}

	var p transactionPostedPayload
	if err := json.Unmarshal(d.Body, &p); err != nil {
		// Baris processed_events SUDAH tersimpan di atas — sengaja TIDAK di-rollback.
		// Payload cacat tidak akan pernah valid pada percobaan berikutnya, dan mencatatnya
		// sebagai "sudah diproses" mencegah pesan poison diulang selamanya. Dikirim ke DLQ
		// untuk diperiksa manusia, bukan diam-diam dibuang.
		a.log.ErrorContext(ctx, "payload tidak valid, dikirim ke DLQ", "event_id", eventID, "err", err)
		_ = d.Nack(false, false)
		return
	}

	a.log.InfoContext(ctx, "audit: transaksi diposting",
		"event_id", eventID, "transaction_id", p.TransactionID, "type", p.Type, "status", p.Status)
	_ = d.Ack(false)
}
