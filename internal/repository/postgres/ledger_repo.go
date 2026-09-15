package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/caesarovera/nusaledger/internal/domain"
)

// LedgerRepo memposting dan membaca transaksi ledger. Ini fungsi terpenting di seluruh sistem.
type LedgerRepo struct {
	db *pgxpool.Pool
}

func NewLedgerRepo(db *pgxpool.Pool) *LedgerRepo { return &LedgerRepo{db: db} }

// lockedAccount adalah baris accounts yang sudah dikunci FOR UPDATE.
type lockedAccount struct {
	ID            int64     `db:"id"`
	PublicID      uuid.UUID `db:"public_id"`
	Balance       int64     `db:"balance"`
	Version       int64     `db:"version"`
	Status        string    `db:"status"`
	NormalBalance string    `db:"normal_balance"`
	AccountType   string    `db:"account_type"`
}

// Post memposting transaksi secara ATOMIK: klaim idempotency, kunci akun terurut,
// tulis header + entries, perbarui saldo, simpan hasil untuk retry, tulis outbox.
// Semua atau tidak sama sekali. Trigger database memverifikasi keseimbangan saat COMMIT.
func (r *LedgerRepo) Post(ctx context.Context, txn *domain.Transaction, claim domain.IdempotencyClaim) (*domain.PostResult, error) {
	// Pertahanan lapis kedua: service sudah memanggil Validate, tetapi jalur lain
	// (worker Fase 2, skrip) tidak boleh bisa melewatinya.
	if err := txn.Validate(); err != nil {
		return nil, err
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op setelah Commit; jaring pengaman untuk setiap return

	// 1. Klaim idempotency key DI DALAM transaksi yang sama.
	//    Kalau transaksi ini rollback, klaimnya ikut hilang dan klien boleh retry.
	ct, err := tx.Exec(ctx, `
		INSERT INTO idempotency_keys (user_id, key, endpoint, request_hash, status_code, response_body)
		VALUES ($1, $2, $3, $4, 0, '{}'::jsonb)
		ON CONFLICT (user_id, key) DO NOTHING`,
		claim.UserID, claim.Key, claim.Endpoint, claim.RequestHash)
	if err != nil {
		return nil, fmt.Errorf("klaim idempotency: %w", translate(err))
	}
	if ct.RowsAffected() == 0 {
		return nil, domain.ErrIdempotencyInFlight
	}

	// 2. Kunci semua akun terlibat, TERURUT berdasarkan id → transfer silang antre, bukan deadlock.
	ids := txn.AccountIDs()
	rows, err := tx.Query(ctx, `
		SELECT id, public_id, balance, version, status, normal_balance, account_type
		FROM accounts WHERE id = ANY($1) ORDER BY id FOR UPDATE`, ids)
	if err != nil {
		return nil, fmt.Errorf("kunci akun: %w", err)
	}
	accounts, err := pgx.CollectRows(rows, pgx.RowToStructByName[lockedAccount])
	if err != nil {
		return nil, fmt.Errorf("scan akun: %w", err)
	}
	if len(accounts) != len(ids) {
		return nil, domain.ErrAccountNotFound
	}
	byID := make(map[int64]lockedAccount, len(accounts))
	for _, a := range accounts {
		if a.Status != string(domain.AccountActive) {
			return nil, fmt.Errorf("%w: akun %d berstatus %s", domain.ErrAccountNotActive, a.ID, a.Status)
		}
		byID[a.ID] = a
	}

	// 3. Reversal: tandai transaksi asal. 0 baris = sudah dibalik / tidak ada (BR-11).
	if txn.ReversesID != nil {
		ct, err := tx.Exec(ctx, `
			UPDATE transactions SET status = 'REVERSED'
			WHERE id = $1 AND status = 'POSTED'`, *txn.ReversesID)
		if err != nil {
			return nil, fmt.Errorf("menandai transaksi asal: %w", translate(err))
		}
		if ct.RowsAffected() == 0 {
			return nil, domain.ErrAlreadyReversed
		}
	}

	// 4. Header transaksi. uq_reversal adalah jaring pengaman kedua untuk double reversal.
	var txnID uuid.UUID
	var createdAt time.Time
	err = tx.QueryRow(ctx, `
		INSERT INTO transactions (txn_type, description, initiated_by_user_id, reverses_transaction_id)
		VALUES ($1, $2, $3, $4) RETURNING id, created_at`,
		string(txn.Type), txn.Description, txn.InitiatedBy, txn.ReversesID).Scan(&txnID, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("insert transaksi: %w", translate(err))
	}

	// 5. Tiap entry: hitung saldo baru, update akun (atomik + optimistic lock), sisipkan entry.
	posted := make([]domain.PostedEntry, 0, len(txn.Entries))
	for _, e := range txn.Entries {
		acc := byID[e.AccountID]
		delta := domain.BalanceDelta(e, domain.Direction(acc.NormalBalance))

		// Pre-check di Go memberi error yang rapi; CHECK constraint tetap jaring terakhir.
		if acc.AccountType == string(domain.AccountUserWallet) && acc.Balance+int64(delta) < 0 {
			return nil, fmt.Errorf("%w: akun %d", domain.ErrInsufficientBalance, e.AccountID)
		}

		var newBalance int64
		err = tx.QueryRow(ctx, `
			UPDATE accounts
			SET balance = balance + $1, version = version + 1, updated_at = now()
			WHERE id = $2 AND version = $3 AND status = 'ACTIVE'
			RETURNING balance`,
			int64(delta), e.AccountID, acc.Version).Scan(&newBalance)
		if err != nil {
			if err2 := translate(err); err2 == domain.ErrNotFound { //nolint:errorlint // sentinel hasil translate
				return nil, fmt.Errorf("%w: akun %d", domain.ErrConcurrentModification, e.AccountID)
			}
			return nil, fmt.Errorf("update saldo akun %d: %w", e.AccountID, translate(err))
		}

		var entryID int64
		var entryAt time.Time
		err = tx.QueryRow(ctx, `
			INSERT INTO entries (transaction_id, account_id, direction, amount, balance_after)
			VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at`,
			txnID, e.AccountID, string(e.Direction), int64(e.Amount), newBalance).Scan(&entryID, &entryAt)
		if err != nil {
			return nil, fmt.Errorf("insert entry: %w", translate(err))
		}
		posted = append(posted, domain.PostedEntry{
			ID: entryID, TransactionID: txnID, AccountID: e.AccountID,
			AccountPublicID: acc.PublicID, AccountType: domain.AccountType(acc.AccountType),
			Direction: e.Direction, Amount: e.Amount, BalanceAfter: domain.Money(newBalance), CreatedAt: entryAt,
			TxnType: txn.Type, Description: txn.Description,
		})
	}

	txn.ID = txnID
	txn.CreatedAt = createdAt
	txn.Status = domain.TxnPosted
	result := &domain.PostResult{Transaction: txn, Entries: posted}

	// 6. Simpan hasil final agar retry dengan key sama menerima respons IDENTIK.
	body, err := json.Marshal(toStored(result))
	if err != nil {
		return nil, fmt.Errorf("marshal hasil: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE idempotency_keys SET transaction_id = $1, status_code = 201, response_body = $2
		WHERE user_id = $3 AND key = $4`, txnID, body, claim.UserID, claim.Key); err != nil {
		return nil, fmt.Errorf("simpan hasil idempotency: %w", err)
	}

	// 7. Outbox (Fase 1 hanya menulis; relay & broker baru di Fase 2). Satu INSERT, satu transaksi.
	if _, err := tx.Exec(ctx, `
		INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload)
		VALUES ('transaction', $1, 'ledger.transaction.posted.v1', $2)`, txnID.String(), body); err != nil {
		return nil, fmt.Errorf("tulis outbox: %w", err)
	}

	// COMMIT — di sinilah trigger DEFERRED memverifikasi debit = kredit.
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", translate(err))
	}
	return result, nil
}

// GetTransaction mengambil transaksi beserta semua entry-nya.
// visibleTo nil = tanpa filter (ADMIN). Selain itu hanya transaksi yang menyentuh akun
// tersebut yang terlihat — difilter di WHERE, bukan di aplikasi (BR-12): data milik
// orang lain tidak pernah keluar dari database.
func (r *LedgerRepo) GetTransaction(ctx context.Context, id uuid.UUID, visibleTo *int64) (*domain.PostResult, error) {
	txn := &domain.Transaction{}
	var txnType, status string
	err := r.db.QueryRow(ctx, `
		SELECT t.id, t.txn_type, t.status, t.description, t.initiated_by_user_id, t.reverses_transaction_id, t.created_at
		FROM transactions t
		WHERE t.id = $1
		  AND ($2::BIGINT IS NULL OR EXISTS (
		        SELECT 1 FROM entries e WHERE e.transaction_id = t.id AND e.account_id = $2))`, id, visibleTo).
		Scan(&txn.ID, &txnType, &status, &txn.Description, &txn.InitiatedBy, &txn.ReversesID, &txn.CreatedAt)
	if err != nil {
		if translate(err) == domain.ErrNotFound { //nolint:errorlint
			return nil, domain.ErrTransactionNotFound
		}
		return nil, fmt.Errorf("ambil transaksi %s: %w", id, err)
	}
	txn.Type, txn.Status = domain.TxnType(txnType), domain.TxnStatus(status)

	rows, err := r.db.Query(ctx, `
		SELECT e.id, e.account_id, a.public_id, a.account_type, e.direction, e.amount, e.balance_after, e.created_at
		FROM entries e JOIN accounts a ON a.id = e.account_id
		WHERE e.transaction_id = $1 ORDER BY e.id`, id)
	if err != nil {
		return nil, fmt.Errorf("ambil entries %s: %w", id, err)
	}
	defer rows.Close()

	var posted []domain.PostedEntry
	for rows.Next() {
		var e domain.PostedEntry
		var dir, accType string
		var amount, after int64
		if err := rows.Scan(&e.ID, &e.AccountID, &e.AccountPublicID, &accType, &dir, &amount, &after, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan entry: %w", err)
		}
		e.TransactionID, e.AccountType = id, domain.AccountType(accType)
		e.Direction, e.Amount, e.BalanceAfter = domain.Direction(dir), domain.Money(amount), domain.Money(after)
		e.TxnType, e.Description = txn.Type, txn.Description
		txn.Entries = append(txn.Entries, domain.Entry{AccountID: e.AccountID, Direction: e.Direction, Amount: e.Amount})
		posted = append(posted, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows entries: %w", err)
	}
	return &domain.PostResult{Transaction: txn, Entries: posted}, nil
}

// TrialBalance menghitung Σ debit, Σ kredit, dan selisihnya di seluruh ledger (docs/02 §3.4).
func (r *LedgerRepo) TrialBalance(ctx context.Context) (*domain.TrialBalance, error) {
	var tb domain.TrialBalance
	var debit, credit, diff int64
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount) FILTER (WHERE direction = 'DEBIT'),  0),
		       COALESCE(SUM(amount) FILTER (WHERE direction = 'CREDIT'), 0),
		       COALESCE(SUM(amount) FILTER (WHERE direction = 'DEBIT'),  0)
		     - COALESCE(SUM(amount) FILTER (WHERE direction = 'CREDIT'), 0),
		       count(*)
		FROM entries`).Scan(&debit, &credit, &diff, &tb.EntryCount)
	if err != nil {
		return nil, fmt.Errorf("trial balance: %w", err)
	}
	tb.TotalDebit, tb.TotalCredit, tb.Difference = domain.Money(debit), domain.Money(credit), domain.Money(diff)
	return &tb, nil
}

// BalanceDrift mengembalikan jumlah akun yang saldo termaterialisasinya tidak cocok
// dengan Σ entries (docs/02 §3.5). Harus selalu 0; dipakai job verifikasi & metrik.
func (r *LedgerRepo) BalanceDrift(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.QueryRow(ctx, `
		WITH computed AS (
		    SELECT e.account_id,
		           SUM(CASE WHEN e.direction = a.normal_balance THEN e.amount ELSE -e.amount END) AS calc
		    FROM entries e JOIN accounts a ON a.id = e.account_id
		    GROUP BY e.account_id
		)
		SELECT count(*)
		FROM accounts a LEFT JOIN computed c ON c.account_id = a.id
		WHERE a.balance <> COALESCE(c.calc, 0)`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("cek drift: %w", err)
	}
	return n, nil
}

// ---- format simpanan idempotency (privat untuk paket ini) ----

type storedResult struct {
	TransactionID uuid.UUID     `json:"transaction_id"`
	Type          string        `json:"type"`
	Status        string        `json:"status"`
	Description   string        `json:"description"`
	InitiatedBy   *int64        `json:"initiated_by,omitempty"`
	ReversesID    *uuid.UUID    `json:"reverses_transaction_id,omitempty"`
	CreatedAt     time.Time     `json:"created_at"`
	Entries       []storedEntry `json:"entries"`
}

type storedEntry struct {
	ID              int64     `json:"id"`
	AccountID       int64     `json:"account_id"`
	AccountPublicID uuid.UUID `json:"account_public_id"`
	AccountType     string    `json:"account_type"`
	Direction       string    `json:"direction"`
	AmountSen       int64     `json:"amount_sen"`
	BalanceAfter    int64     `json:"balance_after_sen"`
	CreatedAt       time.Time `json:"created_at"`
}

func toStored(p *domain.PostResult) storedResult {
	s := storedResult{
		TransactionID: p.Transaction.ID, Type: string(p.Transaction.Type), Status: string(p.Transaction.Status),
		Description: p.Transaction.Description, InitiatedBy: p.Transaction.InitiatedBy,
		ReversesID: p.Transaction.ReversesID, CreatedAt: p.Transaction.CreatedAt,
		Entries: make([]storedEntry, 0, len(p.Entries)),
	}
	for _, e := range p.Entries {
		s.Entries = append(s.Entries, storedEntry{
			ID: e.ID, AccountID: e.AccountID, AccountPublicID: e.AccountPublicID, AccountType: string(e.AccountType),
			Direction: string(e.Direction), AmountSen: int64(e.Amount), BalanceAfter: int64(e.BalanceAfter), CreatedAt: e.CreatedAt,
		})
	}
	return s
}

func fromStored(s storedResult) *domain.PostResult {
	txn := &domain.Transaction{
		ID: s.TransactionID, Type: domain.TxnType(s.Type), Status: domain.TxnStatus(s.Status),
		Description: s.Description, InitiatedBy: s.InitiatedBy, ReversesID: s.ReversesID, CreatedAt: s.CreatedAt,
	}
	posted := make([]domain.PostedEntry, 0, len(s.Entries))
	for _, e := range s.Entries {
		txn.Entries = append(txn.Entries, domain.Entry{AccountID: e.AccountID, Direction: domain.Direction(e.Direction), Amount: domain.Money(e.AmountSen)})
		posted = append(posted, domain.PostedEntry{
			ID: e.ID, TransactionID: s.TransactionID, AccountID: e.AccountID,
			AccountPublicID: e.AccountPublicID, AccountType: domain.AccountType(e.AccountType),
			Direction: domain.Direction(e.Direction), Amount: domain.Money(e.AmountSen), BalanceAfter: domain.Money(e.BalanceAfter),
			CreatedAt: e.CreatedAt, TxnType: txn.Type, Description: txn.Description,
		})
	}
	return &domain.PostResult{Transaction: txn, Entries: posted}
}
