//go:build integration

package integration

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/caesarovera/nusaledger/internal/domain"
)

const codeInsufficientPrivilege = "42501"

// appRolePool menyambung sebagai nusaledger_app (peran RUNTIME, migration 000011) —
// BUKAN peran superuser "test" yang dipakai testPool. Membuktikan hak akses ditegakkan
// POSTGRES SENDIRI, bukan hanya trigger forbid_mutation (lapis pertama, sudah dibuktikan
// T-03). Kalau trigger suatu hari terhapus tidak sengaja, lapis ini tetap menahan.
func appRolePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := strings.Replace(testDSN, "test:test@", "nusaledger_app:app_dev_only_ganti_di_produksi@", 1)
	if dsn == testDSN {
		t.Fatal("gagal membangun DSN nusaledger_app — pola testDSN berubah?")
	}
	pool, err := pgxpool.New(testCtx(t), dsn)
	if err != nil {
		t.Fatalf("connect sebagai nusaledger_app: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func assertInsufficientPrivilege(t *testing.T, err error) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != codeInsufficientPrivilege {
		t.Fatalf("mau SQLSTATE %s (insufficient_privilege), dapat %v", codeInsufficientPrivilege, err)
	}
}

// Temuan Fase 2 (docs/02 §2.6, "lapis kedua"): peran runtime TIDAK BOLEH bisa
// UPDATE/DELETE/TRUNCATE entries, atau DELETE transactions — SEKALIPUN trigger
// forbid_mutation dimatikan. Ini menguji lapisan yang BEDA dari T-03.
func TestAppRole_TidakBisaMengubahLedger(t *testing.T) {
	resetDB(t)
	andi := seedUser(t, "andi@test.local", domain.RoleUser)
	seedWallet(t, andi, 10_000*domain.Rupiah) // menghasilkan 2 entries + 1 transaction

	app := appRolePool(t)
	ctx := testCtx(t)

	t.Run("UPDATE entries ditolak hak akses", func(t *testing.T) {
		_, err := app.Exec(ctx, `UPDATE entries SET amount = amount + 1 WHERE id = (SELECT min(id) FROM entries)`)
		assertInsufficientPrivilege(t, err)
	})
	t.Run("DELETE entries ditolak hak akses", func(t *testing.T) {
		_, err := app.Exec(ctx, `DELETE FROM entries WHERE id = (SELECT min(id) FROM entries)`)
		assertInsufficientPrivilege(t, err)
	})
	t.Run("TRUNCATE entries ditolak hak akses", func(t *testing.T) {
		_, err := app.Exec(ctx, `TRUNCATE entries`)
		assertInsufficientPrivilege(t, err)
	})
	t.Run("DELETE transactions ditolak hak akses", func(t *testing.T) {
		_, err := app.Exec(ctx, `DELETE FROM transactions`)
		assertInsufficientPrivilege(t, err)
	})
	t.Run("SELECT dan INSERT entries tetap boleh — hanya UPDATE/DELETE/TRUNCATE yang dicabut", func(t *testing.T) {
		var n int64
		if err := app.QueryRow(ctx, `SELECT count(*) FROM entries`).Scan(&n); err != nil || n != 2 {
			t.Fatalf("SELECT entries: mau 2 baris tanpa error, dapat %d, %v", n, err)
		}
	})
	t.Run("UPDATE transactions.status tetap boleh (reversal butuh ini)", func(t *testing.T) {
		var id string
		if err := app.QueryRow(ctx, `SELECT id FROM transactions LIMIT 1`).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := app.Exec(ctx, `UPDATE transactions SET status = 'REVERSED' WHERE id = $1`, id); err != nil {
			t.Fatalf("UPDATE status harus tetap diizinkan (trigger yang membatasi kolom, bukan hak akses): %v", err)
		}
	})
	assertAllInvariants(t)
}
