//go:build integration

package integration

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	tcrabbitmq "github.com/testcontainers/testcontainers-go/modules/rabbitmq"

	"github.com/caesarovera/nusaledger/internal/consumer"
	"github.com/caesarovera/nusaledger/internal/domain"
	"github.com/caesarovera/nusaledger/internal/platform/broker"
	"github.com/caesarovera/nusaledger/internal/platform/outbox"
	"github.com/caesarovera/nusaledger/internal/repository/postgres"
)

// setupRabbit menyalakan RabbitMQ sungguhan untuk SATU test — beda dari Postgres
// (TestMain, dipakai SEMUA test): hanya test di file ini yang butuh broker, jadi
// tidak sepadan membebani seluruh paket dengan biaya start container tambahan.
func setupRabbit(t *testing.T) *amqp.Connection {
	t.Helper()
	ctx := testCtx(t)
	ctr, err := tcrabbitmq.Run(ctx, "rabbitmq:4-management-alpine",
		tcrabbitmq.WithAdminUsername("nusa"), tcrabbitmq.WithAdminPassword("nusa_dev_only"))
	if err != nil {
		t.Fatalf("menjalankan rabbitmq: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })

	url, err := ctr.AmqpURL(ctx)
	if err != nil {
		t.Fatalf("amqp url: %v", err)
	}
	conn, err := broker.Dial(ctx, url)
	if err != nil {
		t.Fatalf("dial rabbitmq: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// Pipeline penuh Fase 2, end-to-end: Post() menulis outbox → relay membaca &
// mempublish dengan publisher confirm → consumer menerima & mencatat idempoten.
// Ini T-15 versi Fase 2 dari docs/06 (belum ada nomor resmi — item baru).
func TestOutbox_RelayDanConsumer_EndToEnd(t *testing.T) {
	resetDB(t)
	ctx := testCtx(t)
	conn := setupRabbit(t)

	relayCh, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	if err := relayCh.Confirm(false); err != nil {
		t.Fatal(err)
	}
	consumeCh, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	if err := broker.DeclareTopology(consumeCh); err != nil {
		t.Fatal(err)
	}

	// Jalur bahagia: satu transfer sungguhan lewat repo -> Post() menulis outbox_events
	// dalam transaksi yang SAMA dengan entries (docs/02 §2.9), belum ada relay yang jalan.
	repo := postgres.NewLedgerRepo(testPool)
	andi := seedUser(t, "andi@test.local", domain.RoleUser)
	budi := seedUser(t, "budi@test.local", domain.RoleUser)
	wA := seedWallet(t, andi, 100_000*domain.Rupiah)
	wB := seedWallet(t, budi, 0)
	txn, _ := domain.NewTransfer(wA, wB, sysFeeID, 50_000*domain.Rupiah, 1_000*domain.Rupiah, andi, "")
	if _, err := repo.Post(ctx, txn, newClaim(andi)); err != nil {
		t.Fatalf("Post: %v", err)
	}
	if n := countRows(t, "outbox_events"); n != 1 {
		t.Fatalf("outbox: mau 1 baris belum terkirim, dapat %d", n)
	}
	assertOutboxPublished(t, false) // belum ada relay yang jalan

	relay := outbox.NewRelay(testPool, relayCh, discardLogger())
	sent, err := relay.PollOnce(ctx)
	if err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	if sent != 1 {
		t.Fatalf("mau 1 event terkirim, dapat %d", sent)
	}
	assertOutboxPublished(t, true)

	// Konsumsi: jalankan consumer sebentar di goroutine, tunggu processed_events terisi.
	auditLog := consumer.NewAuditLog(testPool, discardLogger())
	consumeCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- auditLog.Run(consumeCtx, consumeCh) }()

	waitFor(t, 5*time.Second, func() bool { return countRows(t, "processed_events") == 1 })
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("consumer Run: %v", err)
	}

	// Idempotensi: publish ULANG event yang SAMA (simulasi redelivery at-least-once —
	// RabbitMQ TIDAK menjamin exactly-once) -> harus tetap 1 baris processed_events,
	// bukan error, bukan duplikat.
	sent2, err := relay.PollOnce(ctx) // tidak ada baris baru, harus 0
	if err != nil || sent2 != 0 {
		t.Fatalf("poll kedua tanpa event baru: mau 0 tanpa error, dapat %d, %v", sent2, err)
	}
	republish(t, relayCh)
	consumeCtx2, cancel2 := context.WithCancel(ctx)
	done2 := make(chan error, 1)
	go func() { done2 <- auditLog.Run(consumeCtx2, consumeCh) }()
	time.Sleep(300 * time.Millisecond) // beri waktu redelivery diproses
	cancel2()
	<-done2 //nolint:errcheck // ctx dibatalkan sengaja, error di sini bukan bug

	if n := countRows(t, "processed_events"); n != 1 {
		t.Fatalf("redelivery HARUS tetap 1 baris processed_events (idempoten), dapat %d", n)
	}
	assertAllInvariants(t)
}

func assertOutboxPublished(t *testing.T, want bool) {
	t.Helper()
	var published bool
	if err := testPool.QueryRow(testCtx(t), `SELECT published_at IS NOT NULL FROM outbox_events LIMIT 1`).Scan(&published); err != nil {
		t.Fatalf("cek outbox published_at: %v", err)
	}
	if published != want {
		t.Fatalf("outbox published_at IS NOT NULL: mau %v, dapat %v", want, published)
	}
}

// republish mengirim ULANG event yang sudah ada di outbox_events, memakai event_id
// dan payload yang SAMA — mensimulasikan redelivery, bukan event baru.
func republish(t *testing.T, ch *amqp.Channel) {
	t.Helper()
	ctx := testCtx(t)
	var eventID, eventType string
	var payload []byte
	if err := testPool.QueryRow(ctx, `SELECT event_id, event_type, payload FROM outbox_events LIMIT 1`).Scan(&eventID, &eventType, &payload); err != nil {
		t.Fatalf("ambil event untuk republish: %v", err)
	}
	confirm, err := ch.PublishWithDeferredConfirmWithContext(ctx, broker.ExchangeLedgerEvents, eventType, true, false,
		amqp.Publishing{MessageId: eventID, Type: eventType, ContentType: "application/json", DeliveryMode: amqp.Persistent, Body: payload})
	if err != nil {
		t.Fatalf("republish: %v", err)
	}
	if ok, err := confirm.WaitContext(ctx); err != nil || !ok {
		t.Fatalf("republish tidak di-confirm: ok=%v err=%v", ok, err)
	}
}

// waitFor mengulang cek sampai true atau timeout — konsumsi AMQP asinkron, bukan
// operasi database sinkron, jadi polling pendek lebih tepat daripada sleep tetap.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("kondisi tidak terpenuhi dalam %s", timeout)
}
