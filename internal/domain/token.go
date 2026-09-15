package domain

import "time"

// RefreshToken disimpan sebagai hash; token aslinya hanya pernah ada di tangan klien.
type RefreshToken struct {
	ID        int64
	UserID    int64
	TokenHash string
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

// Usable benar bila token belum dicabut dan belum kedaluwarsa.
func (t RefreshToken) Usable(now time.Time) bool {
	return t.RevokedAt == nil && now.Before(t.ExpiresAt)
}

// Actor adalah identitas pemanggil yang sudah terautentikasi.
type Actor struct {
	UserID int64
	Role   Role
}

// IsAdmin benar bila pemanggil berperan ADMIN.
func (a Actor) IsAdmin() bool { return a.Role == RoleAdmin }
