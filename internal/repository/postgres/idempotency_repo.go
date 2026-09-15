package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/caesarovera/nusaledger/internal/domain"
)

// IdempotencyRepo membaca klaim idempotency. Penulisannya terjadi di LedgerRepo.Post
// (harus satu transaksi dengan pekerjaannya), jadi repo ini hanya membaca.
type IdempotencyRepo struct {
	db *pgxpool.Pool
}

func NewIdempotencyRepo(db *pgxpool.Pool) *IdempotencyRepo { return &IdempotencyRepo{db: db} }

// Find mengembalikan rekaman untuk (user, key). ErrNotFound bila belum pernah ada.
func (r *IdempotencyRepo) Find(ctx context.Context, userID int64, key string) (*domain.IdempotencyRecord, error) {
	rec := &domain.IdempotencyRecord{Claim: domain.IdempotencyClaim{UserID: userID, Key: key}}
	var body []byte
	err := r.db.QueryRow(ctx, `
		SELECT endpoint, request_hash, status_code, transaction_id, response_body, created_at
		FROM idempotency_keys WHERE user_id = $1 AND key = $2`, userID, key).
		Scan(&rec.Claim.Endpoint, &rec.Claim.RequestHash, &rec.StatusCode, &rec.TransactionID, &body, &rec.CreatedAt)
	if err != nil {
		if translate(err) == domain.ErrNotFound { //nolint:errorlint
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("cari idempotency: %w", err)
	}
	if rec.InFlight() {
		return rec, nil
	}
	var stored storedResult
	if err := json.Unmarshal(body, &stored); err != nil {
		return nil, fmt.Errorf("membaca hasil tersimpan: %w", err)
	}
	rec.Result = fromStored(stored)
	return rec, nil
}

// DeleteOlderThan membersihkan klaim lama (retensi 30 hari, docs/02 §2.7).
func (r *IdempotencyRepo) DeleteOlderThan(ctx context.Context, days int) (int64, error) {
	ct, err := r.db.Exec(ctx, `DELETE FROM idempotency_keys WHERE created_at < now() - make_interval(days => $1)`, days)
	if err != nil {
		return 0, fmt.Errorf("bersihkan idempotency: %w", err)
	}
	return ct.RowsAffected(), nil
}
