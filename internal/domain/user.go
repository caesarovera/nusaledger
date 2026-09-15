package domain

import (
	"time"

	"github.com/google/uuid"
)

// Role menentukan hak akses (docs/01 §4).
type Role string

const (
	RoleUser  Role = "USER"
	RoleAdmin Role = "ADMIN"
)

// User adalah pengguna terdaftar. PasswordHash tidak pernah keluar dari lapisan service.
type User struct {
	ID           int64
	PublicID     uuid.UUID
	Email        string
	PasswordHash string
	FullName     string
	Role         Role
	CreatedAt    time.Time
}

// IsAdmin benar bila pengguna berperan ADMIN.
func (u User) IsAdmin() bool { return u.Role == RoleAdmin }
