package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/caesarovera/nusaledger/internal/domain"
	"github.com/caesarovera/nusaledger/internal/service"
)

// ---- stub kecil: hanya method yang ada di ports.go ----

type stubLedger struct {
	post  func(ctx context.Context, txn *domain.Transaction, claim domain.IdempotencyClaim) (*domain.PostResult, error)
	get   func(ctx context.Context, id uuid.UUID, visibleTo *int64) (*domain.PostResult, error)
	calls int
}

func (s *stubLedger) Post(ctx context.Context, txn *domain.Transaction, c domain.IdempotencyClaim) (*domain.PostResult, error) {
	s.calls++
	return s.post(ctx, txn, c)
}
func (s *stubLedger) GetTransaction(ctx context.Context, id uuid.UUID, v *int64) (*domain.PostResult, error) {
	return s.get(ctx, id, v)
}
func (s *stubLedger) TrialBalance(context.Context) (*domain.TrialBalance, error) {
	return &domain.TrialBalance{}, nil
}
func (s *stubLedger) BalanceDrift(context.Context) (int64, error) { return 0, nil }

type stubAccounts struct {
	byPublic map[uuid.UUID]*domain.Account
	wallets  map[int64]*domain.Account
	entries  []domain.PostedEntry
}

func (s *stubAccounts) GetByPublicID(_ context.Context, id uuid.UUID) (*domain.Account, error) {
	if a, ok := s.byPublic[id]; ok {
		return a, nil
	}
	return nil, domain.ErrAccountNotFound
}
func (s *stubAccounts) GetWalletByUserID(_ context.Context, uid int64) (*domain.Account, error) {
	if a, ok := s.wallets[uid]; ok {
		return a, nil
	}
	return nil, domain.ErrAccountNotFound
}
func (s *stubAccounts) SystemAccount(_ context.Context, t domain.AccountType) (*domain.Account, error) {
	switch t {
	case domain.AccountSystemCash:
		return &domain.Account{ID: 1, Type: t, NormalBalance: domain.DirectionDebit}, nil
	case domain.AccountSystemFeeRevenue:
		return &domain.Account{ID: 2, Type: t, NormalBalance: domain.DirectionCredit}, nil
	}
	return nil, domain.ErrAccountNotFound
}

// SystemAccounts (jamak): stub sengaja HANYA punya satu akun fee (id 2), supaya
// pickFeeShard() deterministik untuk unit test yang menegaskan akun fee id tertentu
// (mis. TestTransfer_JalurBahagia). Sharding sungguhan diuji lewat integration test.
func (s *stubAccounts) SystemAccounts(ctx context.Context, t domain.AccountType) ([]*domain.Account, error) {
	a, err := s.SystemAccount(ctx, t)
	if err != nil {
		return nil, err
	}
	return []*domain.Account{a}, nil
}
func (s *stubAccounts) ListEntries(_ context.Context, _ int64, before *int64, limit int) ([]domain.PostedEntry, error) {
	out := make([]domain.PostedEntry, 0, limit+1)
	for _, e := range s.entries {
		if before != nil && e.ID >= *before {
			continue
		}
		out = append(out, e)
		if len(out) == limit+1 {
			break
		}
	}
	return out, nil
}

type stubIdem struct{ rec *domain.IdempotencyRecord }

func (s *stubIdem) Find(context.Context, int64, string) (*domain.IdempotencyRecord, error) {
	if s.rec == nil {
		return nil, domain.ErrNotFound
	}
	return s.rec, nil
}

var cfg = service.LedgerConfig{TransferFee: 1_000 * domain.Rupiah, MinTransfer: 10_000 * domain.Rupiah, MaxTransfer: 50_000_000 * domain.Rupiah}

func newFixture(t *testing.T) (*service.Ledger, *stubLedger, *stubAccounts, *stubIdem) {
	t.Helper()
	uid1, uid2 := int64(10), int64(11)
	w1 := &domain.Account{ID: 100, PublicID: uuid.New(), UserID: &uid1, Type: domain.AccountUserWallet, NormalBalance: domain.DirectionCredit, Status: domain.AccountActive}
	w2 := &domain.Account{ID: 101, PublicID: uuid.New(), UserID: &uid2, Type: domain.AccountUserWallet, NormalBalance: domain.DirectionCredit, Status: domain.AccountActive}
	cash := &domain.Account{ID: 1, PublicID: uuid.New(), Type: domain.AccountSystemCash, NormalBalance: domain.DirectionDebit, Status: domain.AccountActive}
	accounts := &stubAccounts{
		byPublic: map[uuid.UUID]*domain.Account{w1.PublicID: w1, w2.PublicID: w2, cash.PublicID: cash},
		wallets:  map[int64]*domain.Account{uid1: w1, uid2: w2},
	}
	ledger := &stubLedger{
		post: func(_ context.Context, txn *domain.Transaction, _ domain.IdempotencyClaim) (*domain.PostResult, error) {
			txn.ID = uuid.New()
			return &domain.PostResult{Transaction: txn}, nil
		},
		get: func(context.Context, uuid.UUID, *int64) (*domain.PostResult, error) {
			return nil, domain.ErrTransactionNotFound
		},
	}
	idem := &stubIdem{}
	svc, err := service.NewLedger(context.Background(), ledger, accounts, idem, cfg)
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}
	return svc, ledger, accounts, idem
}

func userActor(id int64) domain.Actor  { return domain.Actor{UserID: id, Role: domain.RoleUser} }
func adminActor(id int64) domain.Actor { return domain.Actor{UserID: id, Role: domain.RoleAdmin} }

func transferReq(accounts *stubAccounts, amountSen int64) service.TransferRequest {
	return service.TransferRequest{
		MoneyRequest:      service.MoneyRequest{Actor: userActor(10), IdempotencyKey: "k1", RequestHash: "h1", AmountSen: amountSen},
		ToAccountPublicID: accounts.wallets[11].PublicID,
	}
}

func TestTransfer_ValidasiNominal(t *testing.T) {
	svc, ledger, accounts, _ := newFixture(t)
	tests := []struct {
		name    string
		amount  int64
		wantErr error
	}{
		{"nol", 0, domain.ErrAmountNotPositive},
		{"negatif", -5, domain.ErrAmountNotPositive},
		{"di bawah minimum BR-14", 9_999_99, domain.ErrAmountOutOfRange},
		{"di atas maksimum BR-14", 50_000_001_00, domain.ErrAmountOutOfRange},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Transfer(context.Background(), transferReq(accounts, tt.amount))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("mau %v, dapat %v", tt.wantErr, err)
			}
		})
	}
	if ledger.calls != 0 {
		t.Fatal("validasi gagal tidak boleh menyentuh repository")
	}
}

func TestTransfer_JalurBahagia(t *testing.T) {
	svc, ledger, accounts, _ := newFixture(t)
	res, err := svc.Transfer(context.Background(), transferReq(accounts, 50_000_00))
	if err != nil {
		t.Fatalf("Transfer: %v", err)
	}
	txn := res.Transaction
	if txn.Type != domain.TxnTransfer || len(txn.Entries) != 3 || ledger.calls != 1 {
		t.Fatalf("mau TRANSFER 3 entry 1 panggilan, dapat %s %d %d", txn.Type, len(txn.Entries), ledger.calls)
	}
	if e, _ := txn.EntryFor(100); e.Amount != 51_000_00 || e.Direction != domain.DirectionDebit {
		t.Fatalf("pengirim harus didebit amount+fee: %+v", e)
	}
	if e, _ := txn.EntryFor(2); e.Amount != 1_000_00 {
		t.Fatalf("fee harus masuk akun 2: %+v", e)
	}
}

func TestTransfer_KeDiriSendiriDanKeAkunSistem(t *testing.T) {
	svc, _, accounts, _ := newFixture(t)
	req := transferReq(accounts, 50_000_00)
	req.ToAccountPublicID = accounts.wallets[10].PublicID // dompet sendiri
	if _, err := svc.Transfer(context.Background(), req); !errors.Is(err, domain.ErrSelfTransfer) {
		t.Fatalf("mau ErrSelfTransfer, dapat %v", err)
	}
	for id, a := range accounts.byPublic {
		if a.Type == domain.AccountSystemCash {
			req.ToAccountPublicID = id
		}
	}
	if _, err := svc.Transfer(context.Background(), req); !errors.Is(err, domain.ErrAccountNotFound) {
		t.Fatalf("ke akun sistem: mau ErrAccountNotFound, dapat %v", err)
	}
}

func TestIdempotency_ReplayKonflikInFlight(t *testing.T) {
	stored := &domain.PostResult{Transaction: &domain.Transaction{ID: uuid.New(), Type: domain.TxnTransfer}}

	t.Run("key sama + hash sama → hasil lama tanpa Post", func(t *testing.T) {
		svc, ledger, accounts, idem := newFixture(t)
		idem.rec = &domain.IdempotencyRecord{Claim: domain.IdempotencyClaim{RequestHash: "h1"}, StatusCode: 201, Result: stored}
		res, err := svc.Transfer(context.Background(), transferReq(accounts, 50_000_00))
		if err != nil || res.Transaction.ID != stored.Transaction.ID || ledger.calls != 0 {
			t.Fatalf("mau replay tanpa Post, dapat err=%v calls=%d", err, ledger.calls)
		}
	})
	t.Run("key sama + hash beda → 409 conflict", func(t *testing.T) {
		svc, ledger, accounts, idem := newFixture(t)
		idem.rec = &domain.IdempotencyRecord{Claim: domain.IdempotencyClaim{RequestHash: "LAIN"}, StatusCode: 201, Result: stored}
		_, err := svc.Transfer(context.Background(), transferReq(accounts, 50_000_00))
		if !errors.Is(err, domain.ErrIdempotencyConflict) || ledger.calls != 0 {
			t.Fatalf("mau ErrIdempotencyConflict tanpa Post, dapat %v calls=%d", err, ledger.calls)
		}
	})
	t.Run("key sedang diproses → in-flight", func(t *testing.T) {
		svc, _, accounts, idem := newFixture(t)
		idem.rec = &domain.IdempotencyRecord{Claim: domain.IdempotencyClaim{RequestHash: "h1"}, StatusCode: 0}
		_, err := svc.Transfer(context.Background(), transferReq(accounts, 50_000_00))
		if !errors.Is(err, domain.ErrIdempotencyInFlight) {
			t.Fatalf("mau ErrIdempotencyInFlight, dapat %v", err)
		}
	})
	t.Run("kalah balapan lalu pemenang selesai → hasil pemenang", func(t *testing.T) {
		svc, ledger, accounts, idem := newFixture(t)
		ledger.post = func(context.Context, *domain.Transaction, domain.IdempotencyClaim) (*domain.PostResult, error) {
			idem.rec = &domain.IdempotencyRecord{Claim: domain.IdempotencyClaim{RequestHash: "h1"}, StatusCode: 201, Result: stored}
			return nil, domain.ErrIdempotencyInFlight
		}
		res, err := svc.Transfer(context.Background(), transferReq(accounts, 50_000_00))
		if err != nil || res.Transaction.ID != stored.Transaction.ID {
			t.Fatalf("mau hasil pemenang, dapat %v / %v", res, err)
		}
	})
	t.Run("kalah balapan lalu hash beda → ErrIdempotencyConflict, bukan ErrIdempotencyInFlight", func(t *testing.T) {
		svc, ledger, accounts, idem := newFixture(t)
		ledger.post = func(context.Context, *domain.Transaction, domain.IdempotencyClaim) (*domain.PostResult, error) {
			idem.rec = &domain.IdempotencyRecord{Claim: domain.IdempotencyClaim{RequestHash: "LAIN"}, StatusCode: 201, Result: stored}
			return nil, domain.ErrIdempotencyInFlight
		}
		_, err := svc.Transfer(context.Background(), transferReq(accounts, 50_000_00))
		if !errors.Is(err, domain.ErrIdempotencyConflict) {
			t.Fatalf("mau ErrIdempotencyConflict, dapat %v", err)
		}
	})
	t.Run("tanpa key → validation error", func(t *testing.T) {
		svc, _, accounts, _ := newFixture(t)
		req := transferReq(accounts, 50_000_00)
		req.IdempotencyKey = ""
		if _, err := svc.Transfer(context.Background(), req); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("mau ErrValidation, dapat %v", err)
		}
	})
}

func TestTopupWithdraw_MemakaiDompetPemanggil(t *testing.T) {
	svc, _, _, _ := newFixture(t)
	req := service.MoneyRequest{Actor: userActor(10), IdempotencyKey: "k", RequestHash: "h", AmountSen: 5_000_00}
	res, err := svc.Topup(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if e, _ := res.Transaction.EntryFor(100); e.Direction != domain.DirectionCredit {
		t.Fatal("topup harus mengkredit dompet pemanggil")
	}
	res, err = svc.Withdraw(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if e, _ := res.Transaction.EntryFor(100); e.Direction != domain.DirectionDebit {
		t.Fatal("withdraw harus mendebit dompet pemanggil")
	}
	if _, err := svc.Topup(context.Background(), service.MoneyRequest{Actor: userActor(99), IdempotencyKey: "k", RequestHash: "h", AmountSen: 100}); !errors.Is(err, domain.ErrAccountNotFound) {
		t.Fatalf("user tanpa dompet: mau ErrAccountNotFound, dapat %v", err)
	}
}

func TestReverseDanTrialBalance_HanyaAdmin(t *testing.T) {
	svc, ledger, _, _ := newFixture(t)
	rreq := service.ReverseRequest{Actor: userActor(10), IdempotencyKey: "k", RequestHash: "h", TransactionID: uuid.New()}
	if _, err := svc.Reverse(context.Background(), rreq); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("USER reverse: mau ErrForbidden, dapat %v", err)
	}
	if _, err := svc.TrialBalance(context.Background(), userActor(10)); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("USER trial balance: mau ErrForbidden, dapat %v", err)
	}

	orig, _ := domain.NewTransfer(100, 101, 2, 50_000_00, 1_000_00, 10, "")
	orig.ID = uuid.New()
	ledger.get = func(context.Context, uuid.UUID, *int64) (*domain.PostResult, error) {
		return &domain.PostResult{Transaction: orig}, nil
	}
	rreq.Actor = adminActor(1)
	res, err := svc.Reverse(context.Background(), rreq)
	if err != nil || res.Transaction.Type != domain.TxnReversal || *res.Transaction.ReversesID != orig.ID {
		t.Fatalf("ADMIN reverse: mau REVERSAL menunjuk asal, dapat %v / %v", res, err)
	}
	if _, err := svc.TrialBalance(context.Background(), adminActor(1)); err != nil {
		t.Fatalf("ADMIN trial balance: %v", err)
	}
}

func TestGetTransaction_FilterKepemilikan(t *testing.T) {
	svc, ledger, _, _ := newFixture(t)
	var seen *int64
	ledger.get = func(_ context.Context, _ uuid.UUID, v *int64) (*domain.PostResult, error) {
		seen = v
		return &domain.PostResult{Transaction: &domain.Transaction{}}, nil
	}
	if _, err := svc.GetTransaction(context.Background(), userActor(10), uuid.New()); err != nil || seen == nil || *seen != 100 {
		t.Fatalf("USER harus difilter ke dompetnya (100), dapat %v / %v", seen, err)
	}
	if _, err := svc.GetTransaction(context.Background(), adminActor(1), uuid.New()); err != nil || seen != nil {
		t.Fatalf("ADMIN tanpa filter, dapat %v / %v", seen, err)
	}
}

func TestMyEntries_CursorPagination(t *testing.T) {
	svc, _, accounts, _ := newFixture(t)
	for id := int64(30); id >= 1; id-- {
		accounts.entries = append(accounts.entries, domain.PostedEntry{ID: id, AccountID: 100})
	}
	ctx := context.Background()

	p1, err := svc.MyEntries(ctx, userActor(10), "", 10)
	if err != nil || len(p1.Entries) != 10 || p1.NextCursor == "" || p1.Entries[0].ID != 30 {
		t.Fatalf("halaman 1: %+v, %v", p1, err)
	}
	p2, err := svc.MyEntries(ctx, userActor(10), p1.NextCursor, 10)
	if err != nil || len(p2.Entries) != 10 || p2.Entries[0].ID != 20 {
		t.Fatalf("halaman 2 harus mulai dari id 20: %+v, %v", p2, err)
	}
	p3, err := svc.MyEntries(ctx, userActor(10), p2.NextCursor, 10)
	if err != nil || len(p3.Entries) != 10 || p3.NextCursor != "" {
		t.Fatalf("halaman 3 terakhir tanpa next_cursor: %+v, %v", p3, err)
	}
	if _, err := svc.MyEntries(ctx, userActor(10), "bukan-base64!", 10); !errors.Is(err, domain.ErrInvalidCursor) {
		t.Fatalf("cursor rusak: mau ErrInvalidCursor, dapat %v", err)
	}
	big, _ := svc.MyEntries(ctx, userActor(10), "", 1000)
	if len(big.Entries) != 30 { // dibatasi MaxPageLimit=100, data hanya 30
		t.Fatalf("limit besar harus dibatasi, dapat %d", len(big.Entries))
	}
	def, _ := svc.MyEntries(ctx, userActor(10), "", 0)
	if len(def.Entries) != service.DefaultPageLimit {
		t.Fatalf("limit 0 → default %d, dapat %d", service.DefaultPageLimit, len(def.Entries))
	}
}
