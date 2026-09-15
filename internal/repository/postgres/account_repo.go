package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/caesarovera/nusaledger/internal/domain"
)

// AccountRepo membaca dan membuat akun. Saldo TIDAK pernah diubah dari sini —
// hanya LedgerRepo.Post yang boleh, karena harus disertai entries.
type AccountRepo struct {
	db *pgxpool.Pool
}

func NewAccountRepo(db *pgxpool.Pool) *AccountRepo { return &AccountRepo{db: db} }

const accountColumns = `id, public_id, user_id, account_type, normal_balance, status, balance, version, created_at, updated_at`

func scanAccount(row pgx.Row) (*domain.Account, error) {
	var a domain.Account
	var typ, normal, status string
	var balance int64
	err := row.Scan(&a.ID, &a.PublicID, &a.UserID, &typ, &normal, &status, &balance, &a.Version, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		if translate(err) == domain.ErrNotFound { //nolint:errorlint
			return nil, domain.ErrAccountNotFound
		}
		return nil, err
	}
	a.Type, a.NormalBalance, a.Status, a.Balance = domain.AccountType(typ), domain.Direction(normal), domain.AccountStatus(status), domain.Money(balance)
	return &a, nil
}

// CreateWallet membuat dompet untuk pengguna (dipanggil saat registrasi).
func (r *AccountRepo) CreateWallet(ctx context.Context, userID int64) (*domain.Account, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO accounts (account_type, normal_balance, user_id)
		VALUES ('USER_WALLET', 'CREDIT', $1)
		RETURNING `+accountColumns, userID)
	a, err := scanAccount(row)
	if err != nil {
		return nil, fmt.Errorf("membuat dompet user %d: %w", userID, translate(err))
	}
	return a, nil
}

func (r *AccountRepo) GetByID(ctx context.Context, id int64) (*domain.Account, error) {
	a, err := scanAccount(r.db.QueryRow(ctx, `SELECT `+accountColumns+` FROM accounts WHERE id = $1`, id))
	if err != nil {
		return nil, fmt.Errorf("ambil akun %d: %w", id, err)
	}
	return a, nil
}

func (r *AccountRepo) GetByPublicID(ctx context.Context, publicID uuid.UUID) (*domain.Account, error) {
	a, err := scanAccount(r.db.QueryRow(ctx, `SELECT `+accountColumns+` FROM accounts WHERE public_id = $1`, publicID))
	if err != nil {
		return nil, fmt.Errorf("ambil akun %s: %w", publicID, err)
	}
	return a, nil
}

// GetWalletByUserID mengambil dompet milik pengguna. Filter kepemilikan ada di WHERE (BR-12).
func (r *AccountRepo) GetWalletByUserID(ctx context.Context, userID int64) (*domain.Account, error) {
	a, err := scanAccount(r.db.QueryRow(ctx, `
		SELECT `+accountColumns+` FROM accounts
		WHERE user_id = $1 AND account_type = 'USER_WALLET'`, userID))
	if err != nil {
		return nil, fmt.Errorf("ambil dompet user %d: %w", userID, err)
	}
	return a, nil
}

// SystemAccount mengambil akun sistem berdasarkan jenisnya (tepat satu per jenis).
func (r *AccountRepo) SystemAccount(ctx context.Context, typ domain.AccountType) (*domain.Account, error) {
	a, err := scanAccount(r.db.QueryRow(ctx, `
		SELECT `+accountColumns+` FROM accounts
		WHERE account_type = $1 AND user_id IS NULL`, string(typ)))
	if err != nil {
		return nil, fmt.Errorf("ambil akun sistem %s: %w", typ, err)
	}
	return a, nil
}

// ListEntries mengembalikan mutasi akun, terbaru dulu, cursor pada id (docs/02 §3.3).
// Mengambil limit+1 baris agar pemanggil tahu ada halaman berikutnya.
func (r *AccountRepo) ListEntries(ctx context.Context, accountID int64, beforeID *int64, limit int) ([]domain.PostedEntry, error) {
	rows, err := r.db.Query(ctx, `
		SELECT e.id, e.transaction_id, e.direction, e.amount, e.balance_after, e.created_at,
		       t.txn_type, t.description
		FROM entries e
		JOIN transactions t ON t.id = e.transaction_id
		WHERE e.account_id = $1
		  AND ($2::BIGINT IS NULL OR e.id < $2)
		ORDER BY e.id DESC
		LIMIT $3`, accountID, beforeID, limit+1)
	if err != nil {
		return nil, fmt.Errorf("mutasi akun %d: %w", accountID, err)
	}
	defer rows.Close()

	out := make([]domain.PostedEntry, 0, limit+1)
	for rows.Next() {
		var e domain.PostedEntry
		var dir, typ string
		var amount, after int64
		if err := rows.Scan(&e.ID, &e.TransactionID, &dir, &amount, &after, &e.CreatedAt, &typ, &e.Description); err != nil {
			return nil, fmt.Errorf("scan mutasi: %w", err)
		}
		e.AccountID = accountID
		e.Direction, e.Amount, e.BalanceAfter, e.TxnType = domain.Direction(dir), domain.Money(amount), domain.Money(after), domain.TxnType(typ)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows mutasi: %w", err)
	}
	return out, nil
}
