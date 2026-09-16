package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/caesarovera/nusaledger/internal/domain"
)

// RefreshTokenRepo menyimpan HASH refresh token, tidak pernah token aslinya.
type RefreshTokenRepo struct {
	db *pgxpool.Pool
}

func NewRefreshTokenRepo(db *pgxpool.Pool) *RefreshTokenRepo { return &RefreshTokenRepo{db: db} }

func (r *RefreshTokenRepo) Store(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) error {
	if _, err := r.db.Exec(ctx, `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
		userID, tokenHash, expiresAt); err != nil {
		return fmt.Errorf("simpan refresh token: %w", translate(err))
	}
	return nil
}

func (r *RefreshTokenRepo) Find(ctx context.Context, tokenHash string) (*domain.RefreshToken, error) {
	var t domain.RefreshToken
	err := r.db.QueryRow(ctx, `
		SELECT id, user_id, token_hash, expires_at, revoked_at, created_at
		FROM refresh_tokens WHERE token_hash = $1`, tokenHash).
		Scan(&t.ID, &t.UserID, &t.TokenHash, &t.ExpiresAt, &t.RevokedAt, &t.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("cari refresh token: %w", translate(err))
	}
	return &t, nil
}

// Revoke mencabut token. ErrNotFound bila tidak ada atau sudah dicabut.
func (r *RefreshTokenRepo) Revoke(ctx context.Context, tokenHash string) error {
	ct, err := r.db.Exec(ctx, `
		UPDATE refresh_tokens SET revoked_at = now()
		WHERE token_hash = $1 AND revoked_at IS NULL`, tokenHash)
	if err != nil {
		return fmt.Errorf("cabut refresh token: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// RevokeAllForUser mencabut semua token aktif milik user. 0 baris (tidak ada yang
// aktif) BUKAN error — dipanggil setelah reuse terdeteksi, keadaan itu sah terjadi.
func (r *RefreshTokenRepo) RevokeAllForUser(ctx context.Context, userID int64) error {
	if _, err := r.db.Exec(ctx, `
		UPDATE refresh_tokens SET revoked_at = now()
		WHERE user_id = $1 AND revoked_at IS NULL`, userID); err != nil {
		return fmt.Errorf("cabut semua refresh token user %d: %w", userID, err)
	}
	return nil
}
