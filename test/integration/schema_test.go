//go:build integration

package integration

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/caesarovera/nusaledger/internal/domain"
)

func TestSchema_SeedAkunSistem(t *testing.T) {
	resetDB(t)
	if got := countRows(t, "accounts"); got != 3 {
		t.Fatalf("akun sistem: mau 3, dapat %d", got)
	}
	assertAllInvariants(t)
}

func TestSchema_SeedWalletMenjagaInvariant(t *testing.T) {
	resetDB(t)
	uid := seedUser(t, "andi@test.local", domain.RoleUser)
	w := seedWallet(t, uid, 1_000_000*domain.Rupiah)
	if got := balanceOf(t, w); got != 1_000_000*domain.Rupiah {
		t.Fatalf("saldo seed: mau %s, dapat %s", 1_000_000*domain.Rupiah, got)
	}
	assertAllInvariants(t)
}

// T-02: trigger DEFERRED menolak transaksi tidak seimbang SAAT COMMIT, bukan saat INSERT.
func TestSchema_T02_TriggerMenolakSaatCommit(t *testing.T) {
	resetDB(t)
	ctx := testCtx(t)

	tx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	txnID := uuid.New()
	if _, err := tx.Exec(ctx, `INSERT INTO transactions (id, txn_type) VALUES ($1, 'TOPUP')`, txnID); err != nil {
		t.Fatalf("insert txn: %v", err)
	}
	// INSERT harus LOLOS — pemeriksaan ditunda
	if _, err := tx.Exec(ctx, `
		INSERT INTO entries (transaction_id, account_id, direction, amount, balance_after)
		VALUES ($1, $2, 'DEBIT', 1000, 1000)`, txnID, sysCashID); err != nil {
		t.Fatalf("insert entry tidak seimbang harus lolos saat INSERT, dapat: %v", err)
	}

	err = tx.Commit(ctx)
	if err == nil {
		t.Fatal("COMMIT transaksi tidak seimbang HARUS gagal")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("mau *pgconn.PgError, dapat %T: %v", err, err)
	}
	if pgErr.Code != "23514" || pgErr.ConstraintName != "trg_entries_balanced" {
		t.Fatalf("mau code 23514 constraint trg_entries_balanced, dapat %s / %q", pgErr.Code, pgErr.ConstraintName)
	}
	if got := countRows(t, "entries"); got != 0 {
		t.Fatalf("rollback harus membersihkan entries, tersisa %d", got)
	}
	assertAllInvariants(t)
}

// T-03: entries append-only — UPDATE dan DELETE ditolak database.
func TestSchema_T03_EntriesAppendOnly(t *testing.T) {
	resetDB(t)
	ctx := testCtx(t)
	uid := seedUser(t, "andi@test.local", domain.RoleUser)
	seedWallet(t, uid, 10_000*domain.Rupiah) // menghasilkan 2 entries

	for _, q := range []string{
		`UPDATE entries SET amount = amount + 1 WHERE id = (SELECT min(id) FROM entries)`,
		`DELETE FROM entries WHERE id = (SELECT min(id) FROM entries)`,
	} {
		ct, err := testPool.Exec(ctx, q)
		if err == nil {
			t.Fatalf("%q harus ditolak, tapi lolos (%d baris)", q, ct.RowsAffected())
		}
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42501" { // insufficient_privilege
			t.Fatalf("%q: mau code 42501, dapat %v", q, err)
		}
	}
	if got := countRows(t, "entries"); got != 2 {
		t.Fatalf("entries harus tetap 2, dapat %d", got)
	}
	assertAllInvariants(t)
}

// K-05: transactions hanya boleh mengubah status, satu arah.
func TestSchema_K05_TransactionsStatusOnly(t *testing.T) {
	resetDB(t)
	ctx := testCtx(t)
	uid := seedUser(t, "andi@test.local", domain.RoleUser)
	seedWallet(t, uid, 10_000*domain.Rupiah)

	if _, err := testPool.Exec(ctx, `UPDATE transactions SET txn_type = 'WITHDRAW'`); err == nil {
		t.Fatal("mengubah txn_type harus ditolak")
	}
	if _, err := testPool.Exec(ctx, `UPDATE transactions SET status = 'REVERSED'`); err != nil {
		t.Fatalf("mengubah status POSTED→REVERSED harus boleh: %v", err)
	}
	if _, err := testPool.Exec(ctx, `UPDATE transactions SET status = 'POSTED'`); err == nil {
		t.Fatal("REVERSED→POSTED harus ditolak")
	}
	if _, err := testPool.Exec(ctx, `DELETE FROM transactions`); err == nil {
		t.Fatal("DELETE transactions harus ditolak")
	}
}
