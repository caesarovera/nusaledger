// Package postgres berisi implementasi repository di atas pgx.
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Setelan pool (docs/02 §5): max_connections Postgres default 100, sisakan ~20
// untuk admin & tooling, dibagi 3 instance aplikasi → 25 per instance.
const (
	maxConns        = 25
	minConns        = 5
	maxConnLifetime = 30 * time.Minute
	maxConnIdleTime = 5 * time.Minute
	pingTimeout     = 5 * time.Second
)

// NewPool membuka connection pool dan memastikan database bisa dihubungi.
// Gagal di sini berarti aplikasi menolak start — jauh lebih mudah didiagnosis
// daripada aplikasi hidup tapi semua request 500.
func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("membaca DATABASE_URL: %w", err)
	}
	cfg.MaxConns = maxConns
	cfg.MinConns = minConns
	cfg.MaxConnLifetime = maxConnLifetime
	cfg.MaxConnIdleTime = maxConnIdleTime

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("membuat pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("menghubungi database: %w", err)
	}
	return pool, nil
}
