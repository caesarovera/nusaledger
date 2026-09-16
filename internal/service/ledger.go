package service

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"

	"github.com/google/uuid"

	"github.com/caesarovera/nusaledger/internal/domain"
)

// LedgerConfig adalah aturan bisnis yang bisa dikonfigurasi (BR-13, BR-14).
type LedgerConfig struct {
	TransferFee domain.Money
	MinTransfer domain.Money
	MaxTransfer domain.Money
}

// Ledger adalah use case topup, transfer, withdraw, reversal, dan pembacaan.
type Ledger struct {
	store    LedgerStore
	accounts AccountStore
	idem     IdempotencyStore
	cfg      LedgerConfig

	// id akun sistem dimuat sekali saat startup.
	cashID int64 // partial unique index menjamin tepat satu

	// feeIDs adalah SHARD pendapatan fee (perbaikan performa, migration 000009): setiap
	// transfer mengunci SATU shard yang dipilih ACAK, bukan selalu baris yang sama —
	// tanpa ini, k6 mengukur seluruh sistem terserialisasi pada satu baris (177 tps,
	// p95 515 ms; docs/evidence/k6.md). Trial balance & job drift tetap benar karena
	// keduanya menjumlahkan SELURUH entries/accounts, agnostik jumlah shard.
	feeIDs []int64
}

// NewLedger memuat akun sistem. Gagal di sini = aplikasi menolak start.
func NewLedger(ctx context.Context, store LedgerStore, accounts AccountStore, idem IdempotencyStore, cfg LedgerConfig) (*Ledger, error) {
	if cfg.TransferFee < 0 || cfg.MinTransfer <= 0 || cfg.MaxTransfer < cfg.MinTransfer {
		return nil, errors.New("konfigurasi ledger tidak masuk akal")
	}
	cash, err := accounts.SystemAccount(ctx, domain.AccountSystemCash)
	if err != nil {
		return nil, fmt.Errorf("memuat akun SYSTEM_CASH: %w", err)
	}
	fees, err := accounts.SystemAccounts(ctx, domain.AccountSystemFeeRevenue)
	if err != nil {
		return nil, fmt.Errorf("memuat akun SYSTEM_FEE_REVENUE: %w", err)
	}
	if len(fees) == 0 {
		return nil, errors.New("tidak ada akun SYSTEM_FEE_REVENUE — migration belum lengkap")
	}
	feeIDs := make([]int64, len(fees))
	for i, f := range fees {
		feeIDs[i] = f.ID
	}
	return &Ledger{store: store, accounts: accounts, idem: idem, cfg: cfg, cashID: cash.ID, feeIDs: feeIDs}, nil
}

// pickFeeShard memilih satu akun fee acak untuk satu transfer, supaya kontensi
// FOR UPDATE tersebar ke banyak baris, bukan menumpuk di satu baris yang sama.
func (s *Ledger) pickFeeShard() int64 {
	if len(s.feeIDs) == 1 {
		return s.feeIDs[0]
	}
	return s.feeIDs[rand.IntN(len(s.feeIDs))] //nolint:gosec // pemilihan shard, bukan kriptografi
}

// MoneyRequest adalah input bersama semua operasi uang (BR-07: idempotency wajib).
type MoneyRequest struct {
	Actor          domain.Actor
	IdempotencyKey string
	RequestHash    string
	AmountSen      int64
	Description    string
}

// TransferRequest menambahkan tujuan transfer.
type TransferRequest struct {
	MoneyRequest
	ToAccountPublicID uuid.UUID
}

// ReverseRequest membatalkan transaksi (ADMIN saja).
type ReverseRequest struct {
	Actor          domain.Actor
	IdempotencyKey string
	RequestHash    string
	TransactionID  uuid.UUID
	Description    string
}

func (r MoneyRequest) claim(endpoint string) domain.IdempotencyClaim {
	return domain.IdempotencyClaim{UserID: r.Actor.UserID, Key: r.IdempotencyKey, Endpoint: endpoint, RequestHash: r.RequestHash}
}

// Topup mengisi saldo dompet pemanggil (simulasi; tidak ada gateway nyata).
func (s *Ledger) Topup(ctx context.Context, req MoneyRequest) (*domain.PostResult, error) {
	amount, err := s.amountWithin(req.AmountSen, 1, s.cfg.MaxTransfer)
	if err != nil {
		return nil, err
	}
	wallet, err := s.accounts.GetWalletByUserID(ctx, req.Actor.UserID)
	if err != nil {
		return nil, err
	}
	res, err := s.postIdempotent(ctx, req.claim("topup"), func() (*domain.Transaction, error) {
		return domain.NewTopup(s.cashID, wallet.ID, amount, req.Actor.UserID, req.Description), nil
	})
	return markViewer(res, err, wallet.ID)
}

// Withdraw menarik dana dari dompet pemanggil ke kas (simulasi).
func (s *Ledger) Withdraw(ctx context.Context, req MoneyRequest) (*domain.PostResult, error) {
	amount, err := s.amountWithin(req.AmountSen, 1, s.cfg.MaxTransfer)
	if err != nil {
		return nil, err
	}
	wallet, err := s.accounts.GetWalletByUserID(ctx, req.Actor.UserID)
	if err != nil {
		return nil, err
	}
	res, err := s.postIdempotent(ctx, req.claim("withdraw"), func() (*domain.Transaction, error) {
		return domain.NewWithdraw(wallet.ID, s.cashID, amount, req.Actor.UserID, req.Description), nil
	})
	return markViewer(res, err, wallet.ID)
}

// Transfer memindahkan dana antar dompet dengan biaya admin (docs/01 §5.1).
func (s *Ledger) Transfer(ctx context.Context, req TransferRequest) (*domain.PostResult, error) {
	amount, err := s.amountWithin(req.AmountSen, s.cfg.MinTransfer, s.cfg.MaxTransfer)
	if err != nil {
		return nil, err
	}
	from, err := s.accounts.GetWalletByUserID(ctx, req.Actor.UserID)
	if err != nil {
		return nil, err
	}
	res, err := s.postIdempotent(ctx, req.claim("transfer"), func() (*domain.Transaction, error) {
		to, err := s.accounts.GetByPublicID(ctx, req.ToAccountPublicID)
		if err != nil {
			return nil, err
		}
		if to.Type != domain.AccountUserWallet {
			return nil, domain.ErrAccountNotFound // akun sistem bukan tujuan transfer yang sah
		}
		return domain.NewTransfer(from.ID, to.ID, s.pickFeeShard(), amount, s.cfg.TransferFee, req.Actor.UserID, req.Description)
	})
	// Kenapa pemanggil (pengirim) yang jadi ViewerAccountID, bukan penerima: pengirim
	// yang menerima respons ini. Saldo Budi (penerima) dan akun fee tidak boleh terlihat
	// di respons transfer milik Andi (temuan audit B-1) — hanya saldo Andi sendiri.
	return markViewer(res, err, from.ID)
}

// markViewer menandai akun mana yang boleh melihat balance_after di respons (BR-12).
// Dipanggil di SETIAP operasi uang non-admin — jangan sampai ada jalur yang lupa.
func markViewer(res *domain.PostResult, err error, ownAccountID int64) (*domain.PostResult, error) {
	if err != nil {
		return nil, err
	}
	res.ViewerAccountID = &ownAccountID
	return res, nil
}

// Reverse membuat transaksi kebalikan (BR-11). Hanya ADMIN.
func (s *Ledger) Reverse(ctx context.Context, req ReverseRequest) (*domain.PostResult, error) {
	if !req.Actor.IsAdmin() {
		return nil, domain.ErrForbidden
	}
	claim := domain.IdempotencyClaim{UserID: req.Actor.UserID, Key: req.IdempotencyKey, Endpoint: "reverse", RequestHash: req.RequestHash}
	return s.postIdempotent(ctx, claim, func() (*domain.Transaction, error) {
		orig, err := s.store.GetTransaction(ctx, req.TransactionID, nil)
		if err != nil {
			return nil, err
		}
		return domain.NewReversal(orig.Transaction, req.Actor.UserID, req.Description)
	})
}

// GetTransaction: USER hanya melihat transaksi yang menyentuh dompetnya; ADMIN melihat semua.
func (s *Ledger) GetTransaction(ctx context.Context, actor domain.Actor, id uuid.UUID) (*domain.PostResult, error) {
	if actor.IsAdmin() {
		return s.store.GetTransaction(ctx, id, nil)
	}
	wallet, err := s.accounts.GetWalletByUserID(ctx, actor.UserID)
	if err != nil {
		return nil, err
	}
	res, err := s.store.GetTransaction(ctx, id, &wallet.ID)
	return markViewer(res, err, wallet.ID)
}

// MyWallet mengembalikan dompet pemanggil.
func (s *Ledger) MyWallet(ctx context.Context, actor domain.Actor) (*domain.Account, error) {
	return s.accounts.GetWalletByUserID(ctx, actor.UserID)
}

// EntriesPage adalah satu halaman mutasi rekening.
type EntriesPage struct {
	Entries    []domain.PostedEntry
	NextCursor string // kosong bila tidak ada halaman berikutnya
}

// MyEntries mengembalikan mutasi dompet pemanggil dengan cursor pagination.
func (s *Ledger) MyEntries(ctx context.Context, actor domain.Actor, cursor string, limit int) (*EntriesPage, error) {
	before, err := decodeCursor(cursor)
	if err != nil {
		return nil, err
	}
	limit = normalizeLimit(limit)
	wallet, err := s.accounts.GetWalletByUserID(ctx, actor.UserID)
	if err != nil {
		return nil, err
	}
	entries, err := s.accounts.ListEntries(ctx, wallet.ID, before, limit)
	if err != nil {
		return nil, err
	}
	page := &EntriesPage{Entries: entries}
	if len(entries) > limit { // repo mengambil limit+1 sebagai penanda halaman berikutnya
		page.Entries = entries[:limit]
		page.NextCursor = encodeCursor(entries[limit-1].ID)
	}
	return page, nil
}

// TrialBalance hanya untuk ADMIN (docs/03 §4.1).
func (s *Ledger) TrialBalance(ctx context.Context, actor domain.Actor) (*domain.TrialBalance, error) {
	if !actor.IsAdmin() {
		return nil, domain.ErrForbidden
	}
	return s.store.TrialBalance(ctx)
}

// BalanceDrift dipakai job verifikasi & metrik; bukan endpoint publik.
func (s *Ledger) BalanceDrift(ctx context.Context) (int64, error) {
	return s.store.BalanceDrift(ctx)
}

// amountWithin memvalidasi nominal: positif, tidak overflow, dalam rentang.
func (s *Ledger) amountWithin(sen int64, minimum, maximum domain.Money) (domain.Money, error) {
	amount, err := domain.NewMoney(sen)
	if err != nil {
		return 0, err
	}
	if amount < minimum || amount > maximum {
		return 0, fmt.Errorf("%w: %s (batas %s–%s)", domain.ErrAmountOutOfRange, amount, minimum, maximum)
	}
	return amount, nil
}

// postIdempotent adalah jalur idempotency bersama (docs/01 §5.1 langkah 4):
//   - key sudah ada + hash sama  → kembalikan hasil lama
//   - key sudah ada + hash beda  → ErrIdempotencyConflict
//   - key sedang diproses        → ErrIdempotencyInFlight
//   - belum ada                  → susun transaksi, Validate, Post
func (s *Ledger) postIdempotent(ctx context.Context, claim domain.IdempotencyClaim, build func() (*domain.Transaction, error)) (*domain.PostResult, error) {
	if claim.Key == "" || len(claim.Key) > 128 {
		return nil, domain.NewValidationError(map[string]string{"Idempotency-Key": "wajib, 1–128 karakter"})
	}
	if res, done, err := s.replay(ctx, claim); done {
		return res, err
	}

	txn, err := build()
	if err != nil {
		return nil, err
	}
	if err := txn.Validate(); err != nil {
		return nil, fmt.Errorf("transaksi tidak valid: %w", err)
	}

	res, err := s.store.Post(ctx, txn, claim)
	if errors.Is(err, domain.ErrIdempotencyInFlight) {
		// Kalah balapan dengan permintaan kembar. Tunggu keputusan pemenang: bisa sukses,
		// masih in-flight, atau ternyata body-nya beda (conflict) — apa pun itu, teruskan apa adanya.
		if res, done, err2 := s.replay(ctx, claim); done {
			return res, err2
		}
		return nil, domain.ErrIdempotencyInFlight
	}
	return res, err
}

// replay memeriksa klaim yang sudah ada. done=true berarti jawaban sudah ditentukan.
func (s *Ledger) replay(ctx context.Context, claim domain.IdempotencyClaim) (*domain.PostResult, bool, error) {
	rec, err := s.idem.Find(ctx, claim.UserID, claim.Key)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, false, nil
		}
		return nil, true, fmt.Errorf("cek idempotency: %w", err)
	}
	if rec.Claim.RequestHash != claim.RequestHash {
		return nil, true, domain.ErrIdempotencyConflict
	}
	if rec.InFlight() {
		return nil, true, domain.ErrIdempotencyInFlight
	}
	return rec.Result, true, nil
}
