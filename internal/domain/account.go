package domain

import (
	"time"

	"github.com/google/uuid"
)

// Direction adalah sisi pencatatan. Maknanya bergantung pada normal balance akun.
type Direction string

const (
	DirectionDebit  Direction = "DEBIT"
	DirectionCredit Direction = "CREDIT"
)

// Opposite mengembalikan sisi kebalikannya; dipakai reversal.
func (d Direction) Opposite() Direction {
	if d == DirectionDebit {
		return DirectionCredit
	}
	return DirectionDebit
}

// Valid benar bila nilainya salah satu dari dua arah yang dikenal.
func (d Direction) Valid() bool {
	return d == DirectionDebit || d == DirectionCredit
}

// AccountType adalah jenis akun dalam bagan akun Fase 1.
type AccountType string

const (
	AccountUserWallet       AccountType = "USER_WALLET"        // kewajiban
	AccountSystemCash       AccountType = "SYSTEM_CASH"        // aset
	AccountSystemFeeRevenue AccountType = "SYSTEM_FEE_REVENUE" // pendapatan
	AccountSystemSuspense   AccountType = "SYSTEM_SUSPENSE"    // kewajiban
)

// NormalBalance adalah sisi yang MENAMBAH saldo akun jenis ini.
// Aset bertambah di debit; kewajiban dan pendapatan bertambah di kredit.
func (t AccountType) NormalBalance() Direction {
	if t == AccountSystemCash {
		return DirectionDebit
	}
	return DirectionCredit
}

// IsSystem benar untuk akun milik sistem (tanpa pemilik pengguna).
func (t AccountType) IsSystem() bool { return t != AccountUserWallet }

// AccountStatus menentukan apakah akun boleh disentuh transaksi.
type AccountStatus string

const (
	AccountActive AccountStatus = "ACTIVE"
	AccountFrozen AccountStatus = "FROZEN"
	AccountClosed AccountStatus = "CLOSED"
)

// Account adalah akun ledger. Balance adalah saldo termaterialisasi;
// entries tetap sumber kebenaran.
type Account struct {
	ID            int64
	PublicID      uuid.UUID
	UserID        *int64 // nil untuk akun sistem
	Type          AccountType
	NormalBalance Direction
	Status        AccountStatus
	Balance       Money
	Version       int64 // optimistic lock
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// IsActive benar bila akun boleh didebit maupun dikredit (BR-10).
func (a Account) IsActive() bool { return a.Status == AccountActive }

// OwnedBy benar bila akun milik pengguna tersebut (BR-12).
func (a Account) OwnedBy(userID int64) bool {
	return a.UserID != nil && *a.UserID == userID
}
