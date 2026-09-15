// Package dbmigrate menjalankan migration ter-embed lewat golang-migrate di atas pgx.
package dbmigrate

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib" // driver database/sql "pgx"

	"github.com/caesarovera/nusaledger/migrations"
)

// Up menerapkan semua migration yang belum dijalankan. Tidak ada perubahan = bukan error.
func Up(dsn string) error {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("membaca migration ter-embed: %w", err)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("membuka koneksi migration: %w", err)
	}
	defer db.Close()

	drv, err := pgxmigrate.WithInstance(db, &pgxmigrate.Config{})
	if err != nil {
		return fmt.Errorf("menyiapkan driver migration: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "pgx5", drv)
	if err != nil {
		return fmt.Errorf("menyiapkan migrator: %w", err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("menjalankan migration: %w", err)
	}
	return nil
}
