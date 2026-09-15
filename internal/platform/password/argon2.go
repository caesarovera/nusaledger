// Package password meng-hash kata sandi dengan argon2id (tahan serangan GPU).
package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Parameter OWASP (2023): m=64 MiB, t=3, p=1 — atau setara lebih hemat memori.
// Dibuat konstanta di sini supaya bisa diturunkan di test tanpa menyentuh produksi.
type Params struct {
	Memory      uint32 // KiB
	Iterations  uint32
	Parallelism uint8
	SaltLen     uint32
	KeyLen      uint32
}

var DefaultParams = Params{Memory: 64 * 1024, Iterations: 3, Parallelism: 1, SaltLen: 16, KeyLen: 32}

// FastParams hanya untuk test: memori kecil supaya ratusan test tidak lambat.
var FastParams = Params{Memory: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLen: 16, KeyLen: 32}

type Argon2 struct{ p Params }

func NewArgon2(p Params) *Argon2 { return &Argon2{p: p} }

// Hash menghasilkan string format PHC: $argon2id$v=19$m=...,t=...,p=...$<salt>$<hash>
func (a *Argon2) Hash(password string) (string, error) {
	salt := make([]byte, a.p.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("membuat salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, a.p.Iterations, a.p.Memory, a.p.Parallelism, a.p.KeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, a.p.Memory, a.p.Iterations, a.p.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// Verify membandingkan dalam waktu konstan. Parameter dibaca dari hash tersimpan,
// sehingga hash lama tetap bisa diverifikasi setelah parameter default dinaikkan.
func (a *Argon2) Verify(encoded, password string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errors.New("format hash tidak dikenal")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, errors.New("versi argon2 tidak didukung")
	}
	var mem, iter uint32
	var par uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &iter, &par); err != nil {
		return false, fmt.Errorf("parameter hash rusak: %w", err)
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("salt rusak: %w", err)
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("hash rusak: %w", err)
	}
	got := argon2.IDKey([]byte(password), salt, iter, mem, par, uint32(len(want))) //nolint:gosec // panjang key kecil
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
