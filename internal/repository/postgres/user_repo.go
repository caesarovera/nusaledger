package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/caesarovera/nusaledger/internal/domain"
)

// UserRepo menyimpan pengguna.
type UserRepo struct {
	db *pgxpool.Pool
}

func NewUserRepo(db *pgxpool.Pool) *UserRepo { return &UserRepo{db: db} }

const userColumns = `id, public_id, email, password_hash, full_name, role, created_at`

func scanUser(row pgx.Row) (*domain.User, error) {
	var u domain.User
	var role string
	if err := row.Scan(&u.ID, &u.PublicID, &u.Email, &u.PasswordHash, &u.FullName, &role, &u.CreatedAt); err != nil {
		if translate(err) == domain.ErrNotFound { //nolint:errorlint
			return nil, domain.ErrUserNotFound
		}
		return nil, translate(err)
	}
	u.Role = domain.Role(role)
	return &u, nil
}

// CreateWithWallet membuat pengguna DAN dompetnya dalam satu transaksi database:
// tidak boleh ada pengguna tanpa dompet, atau dompet tanpa pengguna.
func (r *UserRepo) CreateWithWallet(ctx context.Context, u *domain.User) (*domain.User, *domain.Account, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	created, err := scanUser(tx.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, full_name, role)
		VALUES ($1, $2, $3, $4) RETURNING `+userColumns,
		u.Email, u.PasswordHash, u.FullName, string(u.Role)))
	if err != nil {
		return nil, nil, fmt.Errorf("membuat pengguna: %w", err)
	}
	wallet, err := scanAccount(tx.QueryRow(ctx, `
		INSERT INTO accounts (account_type, normal_balance, user_id)
		VALUES ('USER_WALLET', 'CREDIT', $1) RETURNING `+accountColumns, created.ID))
	if err != nil {
		return nil, nil, fmt.Errorf("membuat dompet: %w", translate(err))
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, nil, fmt.Errorf("commit: %w", translate(err))
	}
	return created, wallet, nil
}

// GetByEmail mencari pengguna tanpa memedulikan huruf besar-kecil (selaras idx_users_email).
func (r *UserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	u, err := scanUser(r.db.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE LOWER(email) = LOWER($1)`, email))
	if err != nil {
		return nil, fmt.Errorf("cari pengguna: %w", err)
	}
	return u, nil
}

func (r *UserRepo) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	u, err := scanUser(r.db.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id))
	if err != nil {
		return nil, fmt.Errorf("ambil pengguna %d: %w", id, err)
	}
	return u, nil
}
