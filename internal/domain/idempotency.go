package domain

import (
	"time"

	"github.com/google/uuid"
)

// IdempotencyClaim adalah klaim "permintaan ini milik saya" yang dicatat
// DI DALAM transaksi database yang sama dengan pekerjaannya (BR-07).
type IdempotencyClaim struct {
	UserID      int64
	Key         string
	Endpoint    string
	RequestHash string // SHA-256 hex dari body; membedakan retry sah dari salah pakai key (BR-08)
}

// IdempotencyRecord adalah hasil yang tersimpan untuk sebuah klaim.
// StatusCode 0 berarti permintaan masih diproses (in-flight).
type IdempotencyRecord struct {
	Claim         IdempotencyClaim
	StatusCode    int
	TransactionID *uuid.UUID
	Result        *PostResult
	CreatedAt     time.Time
}

// InFlight benar bila klaim sudah ada tetapi hasilnya belum tersimpan.
func (r IdempotencyRecord) InFlight() bool { return r.StatusCode == 0 }
