//go:build integration

// Package integration menguji repository dan alur uang di atas PostgreSQL 17 sungguhan.
// Satu container per paket (TestMain), bukan per test — hemat 5–10 detik per test.
package integration

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/caesarovera/nusaledger/internal/platform/dbmigrate"
	"github.com/caesarovera/nusaledger/internal/repository/postgres"
)

// testPool dipakai bersama oleh semua test di paket ini; tiap test memanggil resetDB.
var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()

	ctr, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(90*time.Second)),
	)
	if err != nil {
		log.Fatalf("menjalankan container postgres: %v", err)
	}

	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		log.Fatalf("mengambil DSN: %v", err)
	}
	if err := dbmigrate.Up(dsn); err != nil {
		log.Fatalf("migration: %v", err)
	}
	testPool, err = postgres.NewPool(ctx, dsn)
	if err != nil {
		log.Fatalf("membuat pool: %v", err)
	}

	code := m.Run()

	testPool.Close()
	if err := ctr.Terminate(ctx); err != nil {
		log.Printf("menghentikan container: %v", err)
	}
	os.Exit(code)
}
