// Package broker berisi koneksi RabbitMQ dan topologi (exchange, queue, DLQ) Fase 2.
// Hanya lapisan ini yang boleh tahu detail AMQP — relay dan consumer memakai nama
// topik/queue dari sini, tidak membangun string topologi sendiri di tempat lain.
package broker

import (
	"context"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Nama topologi. Satu tempat kebenaran — dipakai relay (publish) dan worker (consume).
const (
	ExchangeLedgerEvents = "ledger.events" // topic exchange, durable

	QueueAuditLog    = "ledger.audit-log"
	QueueAuditLogDLQ = "ledger.audit-log.dlq"

	// RoutingKeyTransactionPosted HARUS sama dengan event_type yang ditulis
	// LedgerRepo.Post ke outbox_events (docs/02 §2.9).
	RoutingKeyTransactionPosted = "ledger.transaction.posted.v1"
)

// Dial menyambung dengan retry+backoff (handbook §3.2): RabbitMQ dan aplikasi sering
// start bersamaan (docker compose) — mencoba sekali lalu menyerah membuat startup rapuh.
func Dial(ctx context.Context, url string) (*amqp.Connection, error) {
	const (
		maxAttempts = 10
		baseDelay   = 500 * time.Millisecond
	)
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		conn, err := amqp.DialConfig(url, amqp.Config{Heartbeat: 10 * time.Second})
		if err == nil {
			return conn, nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("dial rabbitmq dibatalkan: %w", ctx.Err())
		case <-time.After(baseDelay * time.Duration(attempt)):
		}
	}
	return nil, fmt.Errorf("dial rabbitmq setelah %d percobaan: %w", maxAttempts, lastErr)
}

// DeclareTopology mendeklarasikan exchange, DLQ, dan queue kerja. Idempoten — aman
// dipanggil berkali-kali (relay dan worker masing-masing memanggilnya saat start).
func DeclareTopology(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(ExchangeLedgerEvents, amqp.ExchangeTopic, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare exchange %s: %w", ExchangeLedgerEvents, err)
	}

	// DLQ dulu: pesan yang di-nack (requeue=false) atau ditolak berkali-kali oleh
	// consumer mendarat di sini, bukan hilang. Queue biasa TANPA exchange (didorong
	// via x-dead-letter-exchange bawaan, bukan routing key custom).
	if _, err := ch.QueueDeclare(QueueAuditLogDLQ, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare DLQ %s: %w", QueueAuditLogDLQ, err)
	}

	// x-dead-letter-exchange kosong ("") berarti default exchange — pesan dikirim
	// langsung ke queue bernama sama dengan routing key aslinya. Kita paksa routing
	// key DLQ = nama DLQ itu sendiri via x-dead-letter-routing-key.
	q, err := ch.QueueDeclare(QueueAuditLog, true, false, false, false, amqp.Table{
		"x-dead-letter-exchange":    "",
		"x-dead-letter-routing-key": QueueAuditLogDLQ,
	})
	if err != nil {
		return fmt.Errorf("declare queue %s: %w", QueueAuditLog, err)
	}
	if err := ch.QueueBind(q.Name, RoutingKeyTransactionPosted, ExchangeLedgerEvents, false, nil); err != nil {
		return fmt.Errorf("bind queue %s: %w", q.Name, err)
	}
	return nil
}
