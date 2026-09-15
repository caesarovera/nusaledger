package service

import (
	"encoding/base64"
	"strconv"

	"github.com/caesarovera/nusaledger/internal/domain"
)

// Cursor pagination (K-04): cursor adalah base64url dari id entry terakhir.
// Opaque bagi klien — klien tidak perlu tahu (dan tidak boleh mengandalkan) isinya.

const (
	DefaultPageLimit = 20
	MaxPageLimit     = 100
)

func encodeCursor(id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(id, 10)))
}

// decodeCursor mengembalikan nil untuk cursor kosong (halaman pertama).
func decodeCursor(s string) (*int64, error) {
	if s == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, domain.ErrInvalidCursor
	}
	id, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil || id <= 0 {
		return nil, domain.ErrInvalidCursor
	}
	return &id, nil
}

// normalizeLimit menerapkan default dan batas atas.
func normalizeLimit(limit int) int {
	switch {
	case limit <= 0:
		return DefaultPageLimit
	case limit > MaxPageLimit:
		return MaxPageLimit
	default:
		return limit
	}
}
