// Package token menerbitkan dan memverifikasi access token JWT HS256.
package token

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/caesarovera/nusaledger/internal/domain"
)

const issuer = "nusaledger"

// Claims adalah isi access token. Subject = public_id (bukan id internal).
type Claims struct {
	UserID int64       `json:"uid"`
	Role   domain.Role `json:"role"`
	jwt.RegisteredClaims
}

// JWT menandatangani dengan HS256 dan HANYA menerima HS256 saat verifikasi.
type JWT struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

func NewJWT(secret string, ttl time.Duration) (*JWT, error) {
	if len(secret) < 16 {
		return nil, errors.New("JWT secret terlalu pendek")
	}
	if ttl <= 0 {
		return nil, errors.New("umur access token harus > 0")
	}
	return &JWT{secret: []byte(secret), ttl: ttl, now: time.Now}, nil
}

// Issue menerbitkan token untuk pengguna.
func (j *JWT) Issue(userID int64, publicID uuid.UUID, role domain.Role) (string, time.Time, error) {
	now := j.now()
	exp := now.Add(j.ttl)
	claims := Claims{
		UserID: userID, Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: issuer, Subject: publicID.String(), ID: uuid.NewString(),
			IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(j.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("menandatangani token: %w", err)
	}
	return signed, exp, nil
}

// Parse memverifikasi tanda tangan, algoritma, issuer, dan waktu.
// WithValidMethods menutup serangan `alg: none` dan algorithm confusion.
func (j *JWT) Parse(tokenString string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(tokenString, claims,
		func(t *jwt.Token) (any, error) { return j.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(issuer),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(j.now),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrInvalidToken, err)
	}
	if claims.UserID <= 0 || (claims.Role != domain.RoleUser && claims.Role != domain.RoleAdmin) {
		return nil, domain.ErrInvalidToken
	}
	return claims, nil
}
