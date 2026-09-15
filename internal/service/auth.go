package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/caesarovera/nusaledger/internal/domain"
)

const (
	minPasswordLen   = 8
	maxPasswordLen   = 128
	refreshTokenSize = 32 // byte acak
)

// Auth adalah use case registrasi, login, refresh, dan logout.
type Auth struct {
	users      UserStore
	tokens     RefreshTokenStore
	hasher     PasswordHasher
	issuer     AccessTokenIssuer
	refreshTTL time.Duration
	now        func() time.Time

	// dummyHash dipakai saat email tidak ditemukan supaya durasi login sama (anti enumerasi).
	dummyHash string
}

// NewAuth menyiapkan service auth. dummyHash dihitung sekali saat startup.
func NewAuth(users UserStore, tokens RefreshTokenStore, hasher PasswordHasher, issuer AccessTokenIssuer, refreshTTL time.Duration) (*Auth, error) {
	dummy, err := hasher.Hash("dummy-password-untuk-menyamakan-waktu")
	if err != nil {
		return nil, fmt.Errorf("menyiapkan dummy hash: %w", err)
	}
	return &Auth{users: users, tokens: tokens, hasher: hasher, issuer: issuer, refreshTTL: refreshTTL, now: time.Now, dummyHash: dummy}, nil
}

// RegisterInput adalah data pendaftaran.
type RegisterInput struct {
	Email    string
	Password string
	FullName string
}

// TokenPair adalah hasil login/refresh.
type TokenPair struct {
	AccessToken      string
	AccessExpiresAt  time.Time
	RefreshToken     string
	RefreshExpiresAt time.Time
}

// Register membuat pengguna + dompet (satu transaksi di repository).
func (a *Auth) Register(ctx context.Context, in RegisterInput) (*domain.User, *domain.Account, error) {
	fields := map[string]string{}
	email := strings.TrimSpace(strings.ToLower(in.Email))
	if _, err := mail.ParseAddress(email); err != nil || email == "" || strings.ContainsAny(email, " <>") {
		fields["email"] = "format email tidak valid"
	}
	if n := utf8.RuneCountInString(in.Password); n < minPasswordLen || n > maxPasswordLen {
		fields["password"] = fmt.Sprintf("panjang %d–%d karakter", minPasswordLen, maxPasswordLen)
	}
	name := strings.TrimSpace(in.FullName)
	if n := utf8.RuneCountInString(name); n < 1 || n > 100 {
		fields["full_name"] = "wajib, maksimal 100 karakter"
	}
	if err := domain.NewValidationError(fields); err != nil {
		return nil, nil, err
	}

	hash, err := a.hasher.Hash(in.Password)
	if err != nil {
		return nil, nil, fmt.Errorf("hash kata sandi: %w", err)
	}
	user, wallet, err := a.users.CreateWithWallet(ctx, &domain.User{
		Email: email, PasswordHash: hash, FullName: name, Role: domain.RoleUser,
	})
	if err != nil {
		return nil, nil, err
	}
	return user, wallet, nil
}

// Login memverifikasi kredensial. Pesan gagal SELALU sama, dan hashing tetap
// dijalankan meski email tidak ada, supaya waktu respons tidak membocorkan keberadaan akun.
func (a *Auth) Login(ctx context.Context, email, password string) (*TokenPair, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	user, err := a.users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) || errors.Is(err, domain.ErrNotFound) {
			_, _ = a.hasher.Verify(a.dummyHash, password) // samakan durasi
			return nil, domain.ErrInvalidCredentials
		}
		return nil, fmt.Errorf("mencari pengguna: %w", err)
	}
	ok, err := a.hasher.Verify(user.PasswordHash, password)
	if err != nil {
		return nil, fmt.Errorf("verifikasi kata sandi: %w", err)
	}
	if !ok {
		return nil, domain.ErrInvalidCredentials
	}
	return a.issuePair(ctx, user)
}

// Refresh menukar refresh token dengan pasangan baru (rotasi: token lama dicabut).
func (a *Auth) Refresh(ctx context.Context, refreshToken string) (*TokenPair, error) {
	hash := hashToken(refreshToken)
	rt, err := a.tokens.Find(ctx, hash)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrInvalidToken
		}
		return nil, fmt.Errorf("mencari refresh token: %w", err)
	}
	if !rt.Usable(a.now()) {
		return nil, domain.ErrInvalidToken
	}
	user, err := a.users.GetByID(ctx, rt.UserID)
	if err != nil {
		return nil, fmt.Errorf("memuat pengguna: %w", err)
	}
	if err := a.tokens.Revoke(ctx, hash); err != nil {
		return nil, fmt.Errorf("mencabut token lama: %w", err)
	}
	return a.issuePair(ctx, user)
}

// Logout mencabut refresh token. Token yang tidak dikenal dianggap sudah logout (idempoten).
func (a *Auth) Logout(ctx context.Context, refreshToken string) error {
	if err := a.tokens.Revoke(ctx, hashToken(refreshToken)); err != nil && !errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("mencabut token: %w", err)
	}
	return nil
}

func (a *Auth) issuePair(ctx context.Context, user *domain.User) (*TokenPair, error) {
	access, accessExp, err := a.issuer.Issue(user.ID, user.PublicID, user.Role)
	if err != nil {
		return nil, fmt.Errorf("menerbitkan access token: %w", err)
	}
	raw := make([]byte, refreshTokenSize)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("membuat refresh token: %w", err)
	}
	refresh := base64.RawURLEncoding.EncodeToString(raw)
	refreshExp := a.now().Add(a.refreshTTL)
	if err := a.tokens.Store(ctx, user.ID, hashToken(refresh), refreshExp); err != nil {
		return nil, fmt.Errorf("menyimpan refresh token: %w", err)
	}
	return &TokenPair{AccessToken: access, AccessExpiresAt: accessExp, RefreshToken: refresh, RefreshExpiresAt: refreshExp}, nil
}

// hashToken: yang disimpan adalah SHA-256, bukan token asli — kebocoran database tidak membocorkan sesi.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
