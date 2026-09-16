// Package outbox menjalankan relay: baca outbox_events yang belum terkirim, publish
// ke RabbitMQ, tandai terkirim — semuanya di LUAR transaksi ledger (Fase 1 hanya
// menulis outbox; relay dan broker baru ada di Fase 2, sesuai docs/02 §2.9).
package outbox

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/caesarovera/nusaledger/internal/platform/broker"
)

// row adalah satu baris outbox_events yang belum terkirim.
type row struct {
	ID        int64
	EventID   uuid.UUID
	EventType string
	Payload   []byte
	CreatedAt time.Time
}

// Relay memoles outbox_events → RabbitMQ. Aman dijalankan LEBIH DARI SATU instance
// bersamaan (FOR UPDATE SKIP LOCKED): masing-masing mengambil baris berbeda, tidak
// ada yang mengirim ganda dan tidak ada yang saling menunggu.
type Relay struct {
	db    *pgxpool.Pool
	ch    *amqp.Channel
	log   *slog.Logger
	batch int
}

func NewRelay(db *pgxpool.Pool, ch *amqp.Channel, log *slog.Logger) *Relay {
	return &Relay{db: db, ch: ch, log: log, batch: 50}
}

// Run memoles setiap interval sampai ctx selesai — dimiliki pemanggil, pola sama
// dengan app.RunPeriodic yang sudah dipakai job drift di cmd/api.
func (r *Relay) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			n, err := r.PollOnce(ctx)
			if err != nil {
				r.log.ErrorContext(ctx, "relay gagal", "err", err)
				continue
			}
			if n > 0 {
				r.log.InfoContext(ctx, "relay mengirim event", "jumlah", n)
			}
		}
	}
}

// PollOnce mengambil SATU batch, mengirim satu per satu, dan menandai hasilnya.
// Mengembalikan jumlah event yang BERHASIL terkirim pada batch ini.
func (r *Relay) PollOnce(ctx context.Context) (int, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op setelah commit

	rows, err := tx.Query(ctx, `
		SELECT id, event_id, event_type, payload, created_at
		FROM outbox_events
		WHERE published_at IS NULL
		ORDER BY id
		LIMIT $1
		FOR UPDATE SKIP LOCKED`, r.batch)
	if err != nil {
		return 0, fmt.Errorf("query outbox: %w", err)
	}
	var batch []row
	for rows.Next() {
		var e row
		if err := rows.Scan(&e.ID, &e.EventID, &e.EventType, &e.Payload, &e.CreatedAt); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan outbox: %w", err)
		}
		batch = append(batch, e)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("rows outbox: %w", err)
	}
	rows.Close()

	sent := 0
	for _, e := range batch {
		if err := r.publish(ctx, e); err != nil {
			// Gagal kirim: catat percobaan, JANGAN tandai terkirim — batch berikutnya
			// akan mencobanya lagi. published_at tetap NULL, tetap dalam SKIP LOCKED scope.
			if _, uerr := tx.Exec(ctx, `
				UPDATE outbox_events SET attempts = attempts + 1, last_error = $2 WHERE id = $1`,
				e.ID, err.Error()); uerr != nil {
				return sent, fmt.Errorf("catat kegagalan outbox %d: %w", e.ID, uerr)
			}
			r.log.WarnContext(ctx, "publish outbox gagal, akan dicoba lagi", "outbox_id", e.ID, "err", err)
			continue
		}
		if _, uerr := tx.Exec(ctx, `UPDATE outbox_events SET published_at = now() WHERE id = $1`, e.ID); uerr != nil {
			return sent, fmt.Errorf("tandai terkirim outbox %d: %w", e.ID, uerr)
		}
		sent++
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return sent, nil
}

// publish mengirim SATU event dengan publisher confirm (menunggu ack broker sebelum
// dianggap sukses) — tanpa ini, "terkirim" hanya berarti "berhasil ditulis ke socket",
// bukan "diterima broker" (docs/… MESSAGE-QUEUE-GO §3.3).
// MessageId = event_id: satu-satunya kunci deduplikasi yang consumer punya (BR event
// unik, MESSAGE-QUEUE-GO §8 checklist "Event punya event_id unik untuk deduplikasi").
func (r *Relay) publish(ctx context.Context, e row) error {
	confirm, err := r.ch.PublishWithDeferredConfirmWithContext(ctx,
		broker.ExchangeLedgerEvents, e.EventType, true, false,
		amqp.Publishing{
			MessageId:    e.EventID.String(),
			Type:         e.EventType,
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Timestamp:    e.CreatedAt,
			Body:         e.Payload,
		})
	if err != nil {
		return fmt.Errorf("publish: %w", err)
	}
	ok, err := confirm.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("menunggu confirm: %w", err)
	}
	if !ok {
		return fmt.Errorf("broker menolak (nack) event %s", e.EventID)
	}
	return nil
}
