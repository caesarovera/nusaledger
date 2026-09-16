// Command worker menjalankan outbox relay + consumer Fase 2. Binary TERPISAH dari
// cmd/api (docs/06, handbook §2.3 "Kenapa cmd/api dan cmd/worker terpisah"): profil
// skala berbeda — API butuh banyak instance kecil yang responsif, worker butuh
// sedikit instance yang boleh berjalan lama memegang koneksi AMQP.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/caarlos0/env/v11"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/caesarovera/nusaledger/internal/consumer"
	"github.com/caesarovera/nusaledger/internal/platform/broker"
	"github.com/caesarovera/nusaledger/internal/platform/dbmigrate"
	"github.com/caesarovera/nusaledger/internal/platform/logger"
	"github.com/caesarovera/nusaledger/internal/platform/outbox"
	"github.com/caesarovera/nusaledger/internal/repository/postgres"
)

// workerConfig sengaja terpisah dari internal/config.Config (config.go milik cmd/api):
// worker tidak butuh JWT_SECRET, rate limit, atau setelan HTTP apa pun.
type workerConfig struct {
	// DatabaseURL harus peran terbatas nusaledger_app (migration 000011); MigrationDatabaseURL
	// (opsional, kosong = pakai DatabaseURL) harus peran owner — sama pola dengan cmd/api.
	DatabaseURL          string        `env:"DATABASE_URL,required,notEmpty"`
	MigrationDatabaseURL string        `env:"MIGRATION_DATABASE_URL"`
	RabbitMQURL          string        `env:"RABBITMQ_URL,required,notEmpty"`
	LogLevel             string        `env:"LOG_LEVEL" envDefault:"info"`
	RunMigrations        bool          `env:"RUN_MIGRATIONS" envDefault:"false"`
	RelayInterval        time.Duration `env:"RELAY_INTERVAL" envDefault:"1s"`
}

func (c workerConfig) migrationURL() string {
	if c.MigrationDatabaseURL != "" {
		return c.MigrationDatabaseURL
	}
	return c.DatabaseURL
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	var cfg workerConfig
	if err := env.Parse(&cfg); err != nil {
		return fmt.Errorf("memuat konfigurasi: %w", err)
	}
	log := logger.New(os.Stdout, cfg.LogLevel).With("app", "nusaledger-worker")
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if cfg.RunMigrations {
		if err := dbmigrate.Up(cfg.migrationURL()); err != nil {
			return err
		}
	}

	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	conn, err := broker.Dial(ctx, cfg.RabbitMQURL)
	if err != nil {
		return err
	}
	defer conn.Close() //nolint:errcheck // shutdown, kegagalan close tidak actionable

	relayCh, err := newConfirmChannel(conn)
	if err != nil {
		return fmt.Errorf("channel relay: %w", err)
	}
	defer relayCh.Close() //nolint:errcheck

	consumeCh, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("channel consumer: %w", err)
	}
	defer consumeCh.Close() //nolint:errcheck

	if err := broker.DeclareTopology(consumeCh); err != nil {
		return err
	}

	relay := outbox.NewRelay(pool, relayCh, log)
	auditLog := consumer.NewAuditLog(pool, log)

	errCh := make(chan error, 1)
	go relay.Run(ctx, cfg.RelayInterval)
	go func() {
		if err := auditLog.Run(ctx, consumeCh); err != nil {
			errCh <- fmt.Errorf("consumer audit-log: %w", err)
		}
	}()

	log.Info("worker berjalan", "relay_interval", cfg.RelayInterval.String())
	select {
	case <-ctx.Done():
		log.Info("sinyal berhenti diterima, menyelesaikan pekerjaan yang sedang berjalan")
	case err := <-errCh:
		return err
	}
	return nil
}

// newConfirmChannel membuka channel AMQP dalam mode publisher-confirm (relay.publish
// memerlukan ini). Terpisah dari channel consumer: satu channel = satu peran, supaya
// error di satu sisi (mis. nack konsumen) tidak menutup channel sisi lain.
func newConfirmChannel(conn *amqp.Connection) (*amqp.Channel, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, err
	}
	if err := ch.Confirm(false); err != nil {
		return nil, fmt.Errorf("aktifkan publisher confirm: %w", err)
	}
	return ch, nil
}
