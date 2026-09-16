// Package service berisi use case. Ia tidak tahu HTTP dan tidak tahu SQL.
// Interface di file ini didefinisikan di sisi KONSUMEN: hanya method yang
// benar-benar dibutuhkan service, sehingga stub untuk test tetap kecil.
package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/caesarovera/nusaledger/internal/domain"
)

// LedgerStore memposting dan membaca transaksi ledger.
type LedgerStore interface {
	Post(ctx context.Context, txn *domain.Transaction, claim domain.IdempotencyClaim) (*domain.PostResult, error)
	// GetTransaction: visibleTo nil = tanpa filter (admin); selain itu hanya transaksi
	// yang menyentuh akun tersebut (BR-12, difilter di WHERE).
	GetTransaction(ctx context.Context, id uuid.UUID, visibleTo *int64) (*domain.PostResult, error)
	TrialBalance(ctx context.Context) (*domain.TrialBalance, error)
	BalanceDrift(ctx context.Context) (int64, error)
}

// AccountStore membaca akun dan mutasinya.
type AccountStore interface {
	GetByPublicID(ctx context.Context, publicID uuid.UUID) (*domain.Account, error)
	GetWalletByUserID(ctx context.Context, userID int64) (*domain.Account, error)
	SystemAccount(ctx context.Context, typ domain.AccountType) (*domain.Account, error)
	ListEntries(ctx context.Context, accountID int64, beforeID *int64, limit int) ([]domain.PostedEntry, error)
}

// IdempotencyStore membaca klaim yang sudah ada. Penulisan terjadi di LedgerStore.Post.
type IdempotencyStore interface {
	Find(ctx context.Context, userID int64, key string) (*domain.IdempotencyRecord, error)
}

// UserStore menyimpan pengguna. CreateWithWallet membuat user DAN dompetnya dalam satu transaksi.
type UserStore interface {
	CreateWithWallet(ctx context.Context, u *domain.User) (*domain.User, *domain.Account, error)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	GetByID(ctx context.Context, id int64) (*domain.User, error)
}

// RefreshTokenStore menyimpan hash refresh token.
type RefreshTokenStore interface {
	Store(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) error
	Find(ctx context.Context, tokenHash string) (*domain.RefreshToken, error)
	Revoke(ctx context.Context, tokenHash string) error
	// RevokeAllForUser mencabut SEMUA refresh token aktif milik user (deteksi reuse,
	// temuan audit A-4): token yang sudah dirotasi tapi dipakai lagi adalah sinyal
	// token itu dicuri — respons defensifnya adalah mencabut seluruh sesi, bukan
	// hanya menolak permintaan ini.
	RevokeAllForUser(ctx context.Context, userID int64) error
}

// PasswordHasher menyembunyikan algoritma hash (argon2id) dari service.
type PasswordHasher interface {
	Hash(password string) (string, error)
	Verify(encodedHash, password string) (bool, error)
}

// AccessTokenIssuer menerbitkan access token (JWT) untuk seorang pengguna.
type AccessTokenIssuer interface {
	Issue(userID int64, publicID uuid.UUID, role domain.Role) (token string, expiresAt time.Time, err error)
}
