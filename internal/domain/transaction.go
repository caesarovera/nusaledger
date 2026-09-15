package domain

import (
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
)

// TxnType adalah jenis peristiwa keuangan.
type TxnType string

const (
	TxnTopup    TxnType = "TOPUP"
	TxnTransfer TxnType = "TRANSFER"
	TxnWithdraw TxnType = "WITHDRAW"
	TxnReversal TxnType = "REVERSAL"
)

// TxnStatus hanya berubah satu arah: POSTED → REVERSED.
type TxnStatus string

const (
	TxnPosted   TxnStatus = "POSTED"
	TxnReversed TxnStatus = "REVERSED"
)

const maxDescriptionLen = 255

// Transaction adalah satu peristiwa keuangan utuh yang terdiri dari beberapa entry.
type Transaction struct {
	ID          uuid.UUID
	Type        TxnType
	Status      TxnStatus
	Description string
	InitiatedBy *int64     // user id pemicu; nil untuk proses sistem
	ReversesID  *uuid.UUID // hanya untuk REVERSAL
	Entries     []Entry
	CreatedAt   time.Time
}

// Validate menegakkan BR-02, BR-03, dan K-02 SEBELUM menyentuh database.
// Trigger database tetap menjadi jaminan terakhir; ini memberi pesan error
// yang berguna dan bisa diuji dalam mikrodetik tanpa database.
func (t *Transaction) Validate() error {
	if len(t.Entries) < 2 {
		return ErrTooFewEntries
	}
	if len(t.Description) > maxDescriptionLen {
		return ErrDescriptionTooLong
	}

	var debit, credit Money
	seen := make(map[int64]struct{}, len(t.Entries))
	for _, e := range t.Entries {
		if e.Amount <= 0 {
			return ErrAmountNotPositive
		}
		if _, dup := seen[e.AccountID]; dup {
			return fmt.Errorf("%w: akun %d", ErrDuplicateAccount, e.AccountID)
		}
		seen[e.AccountID] = struct{}{}

		var err error
		switch e.Direction {
		case DirectionDebit:
			debit, err = debit.Add(e.Amount)
		case DirectionCredit:
			credit, err = credit.Add(e.Amount)
		default:
			return fmt.Errorf("%w: %q", ErrUnknownDirection, e.Direction)
		}
		if err != nil {
			return err
		}
	}
	if debit != credit {
		return fmt.Errorf("%w: debit=%s kredit=%s", ErrUnbalanced, debit, credit)
	}
	return nil
}

// AccountIDs mengembalikan id akun yang terlibat, TERURUT naik.
// Urutan ini yang dipakai untuk mengunci baris — kunci anti-deadlock.
func (t *Transaction) AccountIDs() []int64 {
	ids := make([]int64, 0, len(t.Entries))
	for _, e := range t.Entries {
		ids = append(ids, e.AccountID)
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}

// EntryFor mengembalikan entry milik akun tertentu, bila ada.
func (t *Transaction) EntryFor(accountID int64) (Entry, bool) {
	for _, e := range t.Entries {
		if e.AccountID == accountID {
			return e, true
		}
	}
	return Entry{}, false
}

// ---- Builder empat operasi Fase 1 (docs/01 §2.4) ----
// Builder hidup di domain, bukan service, supaya jalur lain (worker Fase 2)
// tidak bisa menyusun transaksi dengan arah yang salah.

// NewTopup: kas perusahaan bertambah (debit aset), utang ke pengguna bertambah (kredit kewajiban).
func NewTopup(cashAccountID, walletID int64, amount Money, initiatedBy int64, desc string) *Transaction {
	return &Transaction{
		Type:        TxnTopup,
		Status:      TxnPosted,
		Description: desc,
		InitiatedBy: &initiatedBy,
		Entries: []Entry{
			{AccountID: cashAccountID, Direction: DirectionDebit, Amount: amount},
			{AccountID: walletID, Direction: DirectionCredit, Amount: amount},
		},
	}
}

// NewTransfer: pengirim didebit amount+fee, penerima dikredit amount, pendapatan fee dikredit fee.
// Bila fee = 0, entry fee tidak dibuat (amount harus > 0 menurut BR-02).
func NewTransfer(fromWalletID, toWalletID, feeAccountID int64, amount, fee Money, initiatedBy int64, desc string) (*Transaction, error) {
	if fromWalletID == toWalletID {
		return nil, ErrSelfTransfer
	}
	if fee < 0 {
		return nil, ErrAmountNotPositive
	}
	total, err := amount.Add(fee)
	if err != nil {
		return nil, err
	}
	entries := []Entry{
		{AccountID: fromWalletID, Direction: DirectionDebit, Amount: total},
		{AccountID: toWalletID, Direction: DirectionCredit, Amount: amount},
	}
	if fee > 0 {
		entries = append(entries, Entry{AccountID: feeAccountID, Direction: DirectionCredit, Amount: fee})
	}
	return &Transaction{
		Type:        TxnTransfer,
		Status:      TxnPosted,
		Description: desc,
		InitiatedBy: &initiatedBy,
		Entries:     entries,
	}, nil
}

// NewWithdraw: dompet didebit (kewajiban berkurang), kas dikredit (aset berkurang).
func NewWithdraw(walletID, cashAccountID int64, amount Money, initiatedBy int64, desc string) *Transaction {
	return &Transaction{
		Type:        TxnWithdraw,
		Status:      TxnPosted,
		Description: desc,
		InitiatedBy: &initiatedBy,
		Entries: []Entry{
			{AccountID: walletID, Direction: DirectionDebit, Amount: amount},
			{AccountID: cashAccountID, Direction: DirectionCredit, Amount: amount},
		},
	}
}

// NewReversal membuat transaksi BARU dengan semua entry dibalik arahnya (BR-11).
// Transaksi asli tidak disentuh selain statusnya. Reversal atas reversal ditolak.
func NewReversal(original *Transaction, initiatedBy int64, desc string) (*Transaction, error) {
	if original.Status != TxnPosted {
		return nil, ErrAlreadyReversed
	}
	if original.Type == TxnReversal {
		return nil, ErrNotReversible
	}
	entries := make([]Entry, len(original.Entries))
	for i, e := range original.Entries {
		entries[i] = Entry{AccountID: e.AccountID, Direction: e.Direction.Opposite(), Amount: e.Amount}
	}
	origID := original.ID
	return &Transaction{
		Type:        TxnReversal,
		Status:      TxnPosted,
		Description: desc,
		InitiatedBy: &initiatedBy,
		ReversesID:  &origID,
		Entries:     entries,
	}, nil
}
