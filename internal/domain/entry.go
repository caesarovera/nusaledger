package domain

import (
	"time"

	"github.com/google/uuid"
)

// Entry adalah satu baris pencatatan yang AKAN diposting: akun mana, sisi apa, berapa.
type Entry struct {
	AccountID int64
	Direction Direction
	Amount    Money
}

// PostedEntry adalah entry yang sudah tercatat di ledger, dibaca untuk mutasi rekening.
// AccountPublicID dan AccountType disertakan supaya API tidak pernah membocorkan id internal.
type PostedEntry struct {
	ID              int64
	TransactionID   uuid.UUID
	AccountID       int64
	AccountPublicID uuid.UUID
	AccountType     AccountType
	Direction       Direction
	Amount          Money
	BalanceAfter    Money
	CreatedAt       time.Time
	TxnType         TxnType
	Description     string
}

// BalanceDelta menghitung perubahan saldo satu entry terhadap akunnya.
// Satu rumus untuk semua jenis akun: searah normal balance → bertambah, sebaliknya → berkurang.
func BalanceDelta(e Entry, normalBalance Direction) Money {
	if e.Direction == normalBalance {
		return e.Amount
	}
	return -e.Amount
}
