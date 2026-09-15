//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/caesarovera/nusaledger/internal/domain"
)

// id akun sistem hasil seed setelah RESTART IDENTITY: 1 CASH, 2 FEE, 3 SUSPENSE.
const (
	sysCashID     = int64(1)
	sysFeeID      = int64(2)
	sysSuspenseID = int64(3)
)

func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// resetDB mengosongkan semua tabel data dan menanam ulang akun sistem.
// TRUNCATE bukan DELETE, jadi trigger append-only tidak menghalangi.
func resetDB(t *testing.T) {
	t.Helper()
	ctx := testCtx(t)
	_, err := testPool.Exec(ctx, `
		TRUNCATE entries, transactions, idempotency_keys, refresh_tokens,
		         outbox_events, accounts, users RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("reset db: %v", err)
	}
	_, err = testPool.Exec(ctx, `
		INSERT INTO accounts (account_type, normal_balance, user_id) VALUES
		    ('SYSTEM_CASH',        'DEBIT',  NULL),
		    ('SYSTEM_FEE_REVENUE', 'CREDIT', NULL),
		    ('SYSTEM_SUSPENSE',    'CREDIT', NULL)`)
	if err != nil {
		t.Fatalf("seed akun sistem: %v", err)
	}
}

// seedUser membuat pengguna dan mengembalikan id internalnya.
func seedUser(t *testing.T, email string, role domain.Role) int64 {
	t.Helper()
	var id int64
	err := testPool.QueryRow(testCtx(t), `
		INSERT INTO users (email, password_hash, full_name, role)
		VALUES ($1, 'x', 'Uji', $2) RETURNING id`, email, string(role)).Scan(&id)
	if err != nil {
		t.Fatalf("seed user %s: %v", email, err)
	}
	return id
}

// seedWallet membuat dompet untuk pengguna. Saldo awal diisi lewat TOPUP sungguhan
// (entries + balance), bukan UPDATE langsung, supaya invariant tetap benar sejak awal.
func seedWallet(t *testing.T, userID int64, initial domain.Money) int64 {
	t.Helper()
	ctx := testCtx(t)
	var walletID int64
	err := testPool.QueryRow(ctx, `
		INSERT INTO accounts (account_type, normal_balance, user_id)
		VALUES ('USER_WALLET', 'CREDIT', $1) RETURNING id`, userID).Scan(&walletID)
	if err != nil {
		t.Fatalf("seed wallet user %d: %v", userID, err)
	}
	if initial <= 0 {
		return walletID
	}

	tx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin seed topup: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op setelah commit

	txnID := uuid.New()
	var cashAfter, walletAfter int64
	if _, err := tx.Exec(ctx, `INSERT INTO transactions (id, txn_type, description) VALUES ($1, 'TOPUP', 'seed')`, txnID); err != nil {
		t.Fatalf("seed txn: %v", err)
	}
	if err := tx.QueryRow(ctx, `UPDATE accounts SET balance = balance + $1, version = version + 1 WHERE id = $2 RETURNING balance`, int64(initial), sysCashID).Scan(&cashAfter); err != nil {
		t.Fatalf("seed update cash: %v", err)
	}
	if err := tx.QueryRow(ctx, `UPDATE accounts SET balance = balance + $1, version = version + 1 WHERE id = $2 RETURNING balance`, int64(initial), walletID).Scan(&walletAfter); err != nil {
		t.Fatalf("seed update wallet: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO entries (transaction_id, account_id, direction, amount, balance_after) VALUES
		    ($1, $2, 'DEBIT',  $4, $5),
		    ($1, $3, 'CREDIT', $4, $6)`,
		txnID, sysCashID, walletID, int64(initial), cashAfter, walletAfter); err != nil {
		t.Fatalf("seed entries: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit seed topup: %v", err)
	}
	return walletID
}

func balanceOf(t *testing.T, accountID int64) domain.Money {
	t.Helper()
	var b int64
	if err := testPool.QueryRow(testCtx(t), `SELECT balance FROM accounts WHERE id = $1`, accountID).Scan(&b); err != nil {
		t.Fatalf("saldo akun %d: %v", accountID, err)
	}
	return domain.Money(b)
}

func countRows(t *testing.T, table string) int {
	t.Helper()
	var n int
	// nama tabel berasal dari konstanta test, bukan input pengguna
	if err := testPool.QueryRow(testCtx(t), fmt.Sprintf(`SELECT count(*) FROM %s`, table)).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// ---- Tiga helper invariant: panggil di akhir SETIAP test uang (docs/02 §3.4, §3.5) ----

// assertTrialBalanceZero: Σ debit = Σ kredit di seluruh ledger (BR-15).
func assertTrialBalanceZero(t *testing.T) {
	t.Helper()
	var debit, credit, diff int64
	err := testPool.QueryRow(testCtx(t), `
		SELECT COALESCE(SUM(amount) FILTER (WHERE direction = 'DEBIT'),  0),
		       COALESCE(SUM(amount) FILTER (WHERE direction = 'CREDIT'), 0),
		       COALESCE(SUM(amount) FILTER (WHERE direction = 'DEBIT'),  0)
		     - COALESCE(SUM(amount) FILTER (WHERE direction = 'CREDIT'), 0)
		FROM entries`).Scan(&debit, &credit, &diff)
	if err != nil {
		t.Fatalf("trial balance: %v", err)
	}
	if diff != 0 {
		t.Fatalf("trial balance TIDAK seimbang: debit=%d kredit=%d selisih=%d", debit, credit, diff)
	}
}

// assertNoNegativeWallet: tidak ada USER_WALLET bersaldo negatif (BR-05).
func assertNoNegativeWallet(t *testing.T) {
	t.Helper()
	var n int
	err := testPool.QueryRow(testCtx(t),
		`SELECT count(*) FROM accounts WHERE account_type = 'USER_WALLET' AND balance < 0`).Scan(&n)
	if err != nil {
		t.Fatalf("cek saldo negatif: %v", err)
	}
	if n != 0 {
		t.Fatalf("ada %d dompet bersaldo negatif", n)
	}
}

// assertMaterializedBalanceMatchesEntries: accounts.balance = Σ delta entries untuk setiap akun.
func assertMaterializedBalanceMatchesEntries(t *testing.T) {
	t.Helper()
	rows, err := testPool.Query(testCtx(t), `
		WITH computed AS (
		    SELECT e.account_id,
		           SUM(CASE WHEN e.direction = a.normal_balance THEN e.amount ELSE -e.amount END) AS calc
		    FROM entries e JOIN accounts a ON a.id = e.account_id
		    GROUP BY e.account_id
		)
		SELECT a.id, a.balance, COALESCE(c.calc, 0)
		FROM accounts a LEFT JOIN computed c ON c.account_id = a.id
		WHERE a.balance <> COALESCE(c.calc, 0)`)
	if err != nil {
		t.Fatalf("cek drift: %v", err)
	}
	defer rows.Close()
	var drift []string
	for rows.Next() {
		var id, stored, calc int64
		if err := rows.Scan(&id, &stored, &calc); err != nil {
			t.Fatalf("scan drift: %v", err)
		}
		drift = append(drift, fmt.Sprintf("akun %d: tersimpan=%d hitung=%d", id, stored, calc))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows drift: %v", err)
	}
	if len(drift) > 0 {
		t.Fatalf("saldo termaterialisasi TIDAK cocok dengan entries:\n%v", drift)
	}
}

// assertAllInvariants adalah tiga assert di atas sekaligus.
func assertAllInvariants(t *testing.T) {
	t.Helper()
	assertTrialBalanceZero(t)
	assertNoNegativeWallet(t)
	assertMaterializedBalanceMatchesEntries(t)
}
