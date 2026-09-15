package postgres

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/caesarovera/nusaledger/internal/domain"
)

// Kode SQLSTATE PostgreSQL yang dipetakan ke error domain.
const (
	codeUniqueViolation = "23505"
	codeCheckViolation  = "23514"
)

// translate menerjemahkan error pgx menjadi error domain, HANYA di lapisan ini.
// Service dan handler tidak boleh tahu SQLSTATE.
func translate(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch pgErr.Code {
	case codeCheckViolation:
		switch pgErr.ConstraintName {
		case "chk_wallet_non_negative":
			return domain.ErrInsufficientBalance
		case "trg_entries_balanced":
			return domain.ErrUnbalanced
		case "entries_amount_check":
			return domain.ErrAmountNotPositive
		}
	case codeUniqueViolation:
		switch pgErr.ConstraintName {
		case "uq_reversal":
			return domain.ErrAlreadyReversed
		case "idx_users_email":
			return domain.ErrEmailTaken
		case "uq_entry_account_per_txn":
			return domain.ErrDuplicateAccount
		case "idempotency_keys_pkey":
			return domain.ErrIdempotencyInFlight
		}
	}
	return err
}
