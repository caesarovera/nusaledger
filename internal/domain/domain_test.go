package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/caesarovera/nusaledger/internal/domain"
)

func TestValidationError(t *testing.T) {
	t.Parallel()
	if err := domain.NewValidationError(nil); err != nil {
		t.Fatal("tanpa field harus nil")
	}
	err := domain.NewValidationError(map[string]string{"email": "wajib", "amount_sen": "harus > 0"})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatal("harus cocok dengan ErrValidation lewat errors.Is")
	}
	var ve *domain.ValidationError
	if !errors.As(err, &ve) || len(ve.Fields) != 2 {
		t.Fatalf("errors.As gagal: %v", err)
	}
	if got := err.Error(); got != "permintaan tidak valid: amount_sen: harus > 0; email: wajib" {
		t.Fatalf("pesan harus terurut & stabil, dapat %q", got)
	}
}

func TestRefreshToken_Usable(t *testing.T) {
	t.Parallel()
	now := time.Now()
	tok := domain.RefreshToken{ExpiresAt: now.Add(time.Hour)}
	if !tok.Usable(now) {
		t.Fatal("token aktif harus usable")
	}
	if tok.Usable(now.Add(2 * time.Hour)) {
		t.Fatal("token kedaluwarsa tidak boleh usable")
	}
	tok.RevokedAt = &now
	if tok.Usable(now) {
		t.Fatal("token dicabut tidak boleh usable")
	}
}

func TestActorDanUser_IsAdmin(t *testing.T) {
	t.Parallel()
	if !(domain.Actor{Role: domain.RoleAdmin}).IsAdmin() || (domain.Actor{Role: domain.RoleUser}).IsAdmin() {
		t.Fatal("Actor.IsAdmin salah")
	}
	if !(domain.User{Role: domain.RoleAdmin}).IsAdmin() || (domain.User{Role: domain.RoleUser}).IsAdmin() {
		t.Fatal("User.IsAdmin salah")
	}
}

func TestPostResultDanTrialBalance(t *testing.T) {
	t.Parallel()
	p := &domain.PostResult{Entries: []domain.PostedEntry{{AccountID: 7, BalanceAfter: 500}}}
	if got, ok := p.BalanceAfterFor(7); !ok || got != 500 {
		t.Fatalf("BalanceAfterFor: mau 500, dapat %d ok=%v", got, ok)
	}
	if _, ok := p.BalanceAfterFor(8); ok {
		t.Fatal("akun tak ada harus ok=false")
	}
	if !(domain.TrialBalance{Difference: 0}).Balanced() || (domain.TrialBalance{Difference: 1}).Balanced() {
		t.Fatal("Balanced salah")
	}
	if !(domain.IdempotencyRecord{StatusCode: 0}).InFlight() || (domain.IdempotencyRecord{StatusCode: 201}).InFlight() {
		t.Fatal("InFlight salah")
	}
}

func TestAccount_HelperKecil(t *testing.T) {
	t.Parallel()
	if !domain.DirectionDebit.Valid() || domain.Direction("X").Valid() {
		t.Fatal("Direction.Valid salah")
	}
	if domain.AccountUserWallet.IsSystem() || !domain.AccountSystemCash.IsSystem() {
		t.Fatal("IsSystem salah")
	}
	a := domain.Account{Status: domain.AccountFrozen}
	if a.IsActive() {
		t.Fatal("FROZEN tidak aktif")
	}
	if _, ok := (&domain.Transaction{}).EntryFor(1); ok {
		t.Fatal("EntryFor pada transaksi kosong harus false")
	}
	_ = uuid.Nil
}
