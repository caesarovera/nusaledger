package token_test

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/caesarovera/nusaledger/internal/domain"
	"github.com/caesarovera/nusaledger/internal/platform/token"
)

const secret = "rahasia-test-yang-cukup-panjang-32b"

func TestJWT_IssueParse(t *testing.T) {
	j, err := token.NewJWT(secret, 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	pid := uuid.New()
	tok, exp, err := j.Issue(42, pid, domain.RoleAdmin)
	if err != nil || tok == "" || time.Until(exp) < 14*time.Minute {
		t.Fatalf("Issue: %v exp=%v", err, exp)
	}
	c, err := j.Parse(tok)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if c.UserID != 42 || c.Role != domain.RoleAdmin || c.Subject != pid.String() {
		t.Fatalf("claims salah: %+v", c)
	}
}

func TestJWT_Ditolak(t *testing.T) {
	j, _ := token.NewJWT(secret, 15*time.Minute)
	lain, _ := token.NewJWT("secret-lain-yang-juga-panjang-32b", 15*time.Minute)
	valid, _, _ := j.Issue(1, uuid.New(), domain.RoleUser)
	parts := strings.Split(valid, ".")

	// token dengan header alg=none dan tanpa tanda tangan
	noneHeader := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	algNone := noneHeader + "." + parts[1] + "."

	// payload diubah (tanda tangan tidak cocok)
	tampered := parts[0] + "." + base64.RawURLEncoding.EncodeToString([]byte(`{"uid":999,"role":"ADMIN","iss":"nusaledger","exp":9999999999}`)) + "." + parts[2]

	// secret berbeda
	wrongSecret, _, _ := lain.Issue(1, uuid.New(), domain.RoleUser)

	tests := map[string]string{
		"alg none":       algNone,
		"payload diubah": tampered,
		"secret berbeda": wrongSecret,
		"bukan jwt":      "abc.def",
		"kosong":         "",
	}
	for name, tok := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := j.Parse(tok); !errors.Is(err, domain.ErrInvalidToken) {
				t.Fatalf("mau ErrInvalidToken, dapat %v", err)
			}
		})
	}
}

func TestJWT_Kedaluwarsa(t *testing.T) {
	j, _ := token.NewJWT(secret, time.Millisecond)
	tok, _, _ := j.Issue(1, uuid.New(), domain.RoleUser)
	time.Sleep(5 * time.Millisecond)
	if _, err := j.Parse(tok); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("kedaluwarsa: mau ErrInvalidToken, dapat %v", err)
	}
}

func TestJWT_SecretPendekDitolak(t *testing.T) {
	if _, err := token.NewJWT("pendek", time.Minute); err == nil {
		t.Fatal("secret pendek harus ditolak saat startup")
	}
}
