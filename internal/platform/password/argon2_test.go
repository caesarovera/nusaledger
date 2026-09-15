package password_test

import (
	"strings"
	"testing"

	"github.com/caesarovera/nusaledger/internal/platform/password"
)

func TestArgon2_HashVerify(t *testing.T) {
	h := password.NewArgon2(password.FastParams)
	enc, err := h.Hash("rahasia123")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(enc, "$argon2id$v=19$") || strings.Contains(enc, "rahasia123") {
		t.Fatalf("format hash salah atau bocor: %s", enc)
	}
	if ok, err := h.Verify(enc, "rahasia123"); err != nil || !ok {
		t.Fatalf("password benar harus lolos: ok=%v err=%v", ok, err)
	}
	if ok, _ := h.Verify(enc, "rahasia124"); ok {
		t.Fatal("password salah harus ditolak")
	}
	enc2, _ := h.Hash("rahasia123")
	if enc == enc2 {
		t.Fatal("dua hash password sama harus berbeda (salt acak)")
	}
}

func TestArgon2_FormatRusak(t *testing.T) {
	h := password.NewArgon2(password.FastParams)
	for _, bad := range []string{"", "bukan-hash", "$bcrypt$x$y$z$w", "$argon2id$v=18$m=8,t=1,p=1$c2FsdA$aGFzaA"} {
		if ok, err := h.Verify(bad, "x"); err == nil || ok {
			t.Fatalf("%q: mau error, dapat ok=%v err=%v", bad, ok, err)
		}
	}
}
