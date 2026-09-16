package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/caesarovera/nusaledger/internal/domain"
	"github.com/caesarovera/nusaledger/internal/service"
)

type memUsers struct {
	byEmail map[string]*domain.User
	nextID  int64
}

func (m *memUsers) CreateWithWallet(_ context.Context, u *domain.User) (*domain.User, *domain.Account, error) {
	if _, dup := m.byEmail[u.Email]; dup {
		return nil, nil, domain.ErrEmailTaken
	}
	m.nextID++
	c := *u
	c.ID, c.PublicID = m.nextID, uuid.New()
	m.byEmail[c.Email] = &c
	return &c, &domain.Account{ID: 100 + c.ID, UserID: &c.ID, Type: domain.AccountUserWallet}, nil
}
func (m *memUsers) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	if u, ok := m.byEmail[email]; ok {
		return u, nil
	}
	return nil, domain.ErrUserNotFound
}
func (m *memUsers) GetByID(_ context.Context, id int64) (*domain.User, error) {
	for _, u := range m.byEmail {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, domain.ErrUserNotFound
}

type memTokens struct {
	byHash map[string]*domain.RefreshToken
}

func (m *memTokens) Store(_ context.Context, uid int64, hash string, exp time.Time) error {
	m.byHash[hash] = &domain.RefreshToken{UserID: uid, TokenHash: hash, ExpiresAt: exp}
	return nil
}
func (m *memTokens) Find(_ context.Context, hash string) (*domain.RefreshToken, error) {
	if t, ok := m.byHash[hash]; ok {
		return t, nil
	}
	return nil, domain.ErrNotFound
}
func (m *memTokens) Revoke(_ context.Context, hash string) error {
	t, ok := m.byHash[hash]
	if !ok || t.RevokedAt != nil {
		return domain.ErrNotFound
	}
	now := time.Now()
	t.RevokedAt = &now
	return nil
}
func (m *memTokens) RevokeAllForUser(_ context.Context, userID int64) error {
	now := time.Now()
	for _, t := range m.byHash {
		if t.UserID == userID && t.RevokedAt == nil {
			t.RevokedAt = &now
		}
	}
	return nil
}

// hasher palsu yang bisa dihitung: hash = "h:" + password. Cukup untuk menguji alur, bukan kriptografi.
type fakeHasher struct{ verifyCalls int }

func (f *fakeHasher) Hash(pw string) (string, error) { return "h:" + pw, nil }
func (f *fakeHasher) Verify(hash, pw string) (bool, error) {
	f.verifyCalls++
	return hash == "h:"+pw, nil
}

type fakeIssuer struct{}

func (fakeIssuer) Issue(uid int64, pid uuid.UUID, role domain.Role) (string, time.Time, error) {
	return "jwt-" + pid.String(), time.Now().Add(15 * time.Minute), nil
}

func newAuth(t *testing.T) (*service.Auth, *memUsers, *memTokens, *fakeHasher) {
	t.Helper()
	users := &memUsers{byEmail: map[string]*domain.User{}}
	tokens := &memTokens{byHash: map[string]*domain.RefreshToken{}}
	hasher := &fakeHasher{}
	a, err := service.NewAuth(users, tokens, hasher, fakeIssuer{}, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return a, users, tokens, hasher
}

func TestRegister_Validasi(t *testing.T) {
	a, _, _, _ := newAuth(t)
	tests := []struct {
		name  string
		in    service.RegisterInput
		field string
	}{
		{"email tidak valid", service.RegisterInput{Email: "bukan-email", Password: "rahasia123", FullName: "Andi"}, "email"},
		{"password pendek", service.RegisterInput{Email: "a@x.com", Password: "pendek", FullName: "Andi"}, "password"},
		{"nama kosong", service.RegisterInput{Email: "a@x.com", Password: "rahasia123", FullName: "  "}, "full_name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := a.Register(context.Background(), tt.in)
			var ve *domain.ValidationError
			if !errors.As(err, &ve) || ve.Fields[tt.field] == "" || !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("mau ValidationError pada %q, dapat %v", tt.field, err)
			}
		})
	}
}

func TestRegister_SuksesDanEmailDuplikat(t *testing.T) {
	a, users, _, _ := newAuth(t)
	u, w, err := a.Register(context.Background(), service.RegisterInput{Email: "  Andi@X.com ", Password: "rahasia123", FullName: " Andi "})
	if err != nil {
		t.Fatal(err)
	}
	if u.Email != "andi@x.com" || u.FullName != "Andi" || u.Role != domain.RoleUser || w == nil {
		t.Fatalf("email harus dinormalisasi, nama di-trim, peran USER, dompet ada: %+v %+v", u, w)
	}
	if users.byEmail["andi@x.com"].PasswordHash == "rahasia123" {
		t.Fatal("kata sandi tidak boleh disimpan mentah")
	}
	if _, _, err := a.Register(context.Background(), service.RegisterInput{Email: "andi@x.com", Password: "rahasia123", FullName: "Andi 2"}); !errors.Is(err, domain.ErrEmailTaken) {
		t.Fatalf("duplikat: mau ErrEmailTaken, dapat %v", err)
	}
}

func TestLogin(t *testing.T) {
	a, _, tokens, hasher := newAuth(t)
	ctx := context.Background()
	if _, _, err := a.Register(ctx, service.RegisterInput{Email: "andi@x.com", Password: "rahasia123", FullName: "Andi"}); err != nil {
		t.Fatal(err)
	}

	t.Run("email tidak ada → pesan sama, hashing tetap dijalankan (anti enumerasi)", func(t *testing.T) {
		before := hasher.verifyCalls
		_, err := a.Login(ctx, "tidakada@x.com", "apa saja")
		if !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("mau ErrInvalidCredentials, dapat %v", err)
		}
		if hasher.verifyCalls != before+1 {
			t.Fatal("Verify harus tetap dipanggil dengan dummy hash")
		}
	})
	t.Run("password salah → pesan sama", func(t *testing.T) {
		if _, err := a.Login(ctx, "andi@x.com", "salah"); !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("mau ErrInvalidCredentials, dapat %v", err)
		}
	})
	t.Run("sukses → pasangan token, refresh disimpan sebagai hash", func(t *testing.T) {
		pair, err := a.Login(ctx, "ANDI@x.com", "rahasia123")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(pair.AccessToken, "jwt-") || pair.RefreshToken == "" {
			t.Fatalf("token kosong: %+v", pair)
		}
		if _, raw := tokens.byHash[pair.RefreshToken]; raw {
			t.Fatal("refresh token tersimpan mentah, harus hash")
		}
		if len(tokens.byHash) != 1 {
			t.Fatalf("mau 1 refresh token tersimpan, dapat %d", len(tokens.byHash))
		}
	})
}

func TestRefreshDanLogout(t *testing.T) {
	a, _, tokens, _ := newAuth(t)
	ctx := context.Background()
	_, _, _ = a.Register(ctx, service.RegisterInput{Email: "andi@x.com", Password: "rahasia123", FullName: "Andi"})
	pair, _ := a.Login(ctx, "andi@x.com", "rahasia123")

	pair2, err := a.Refresh(ctx, pair.RefreshToken)
	if err != nil || pair2.RefreshToken == pair.RefreshToken {
		t.Fatalf("refresh harus menghasilkan token baru: %v", err)
	}
	// rotasi: token lama tidak bisa dipakai lagi
	if _, err := a.Refresh(ctx, pair.RefreshToken); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("token lama: mau ErrInvalidToken, dapat %v", err)
	}
	if _, err := a.Refresh(ctx, "token-ngawur"); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("token asing: mau ErrInvalidToken, dapat %v", err)
	}
	// kedaluwarsa
	for _, rt := range tokens.byHash {
		if rt.RevokedAt == nil {
			rt.ExpiresAt = time.Now().Add(-time.Second)
		}
	}
	if _, err := a.Refresh(ctx, pair2.RefreshToken); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("kedaluwarsa: mau ErrInvalidToken, dapat %v", err)
	}
	// logout idempoten
	if err := a.Logout(ctx, "tidak-dikenal"); err != nil {
		t.Fatalf("logout token asing harus nil, dapat %v", err)
	}
	pair3, _ := a.Login(ctx, "andi@x.com", "rahasia123")
	if err := a.Logout(ctx, pair3.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Refresh(ctx, pair3.RefreshToken); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatal("setelah logout, refresh harus ditolak")
	}
}

// Temuan audit A-4: token yang sudah dirotasi tapi dipakai LAGI adalah sinyal token
// dicuri (rotasi normal tidak pernah memakai token yang sama dua kali). Respons
// defensif: cabut SEMUA sesi user, bukan hanya menolak permintaan reuse ini.
func TestRefresh_ReuseTerdeteksi_CabutSemuaSesi(t *testing.T) {
	a, _, tokens, _ := newAuth(t)
	ctx := context.Background()
	_, _, _ = a.Register(ctx, service.RegisterInput{Email: "andi@x.com", Password: "rahasia123", FullName: "Andi"})

	pairA, _ := a.Login(ctx, "andi@x.com", "rahasia123") // sesi A, mis. laptop
	pairB, _ := a.Login(ctx, "andi@x.com", "rahasia123") // sesi B, mis. HP — dua sesi aktif bersamaan

	pairA2, err := a.Refresh(ctx, pairA.RefreshToken) // rotasi normal sesi A
	if err != nil {
		t.Fatalf("rotasi normal: %v", err)
	}

	// Token LAMA sesi A dipakai lagi — skenario: token itu dicuri SEBELUM dirotasi.
	if _, err := a.Refresh(ctx, pairA.RefreshToken); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("reuse: mau ErrInvalidToken, dapat %v", err)
	}

	// Efeknya BUKAN hanya token lama sesi A yang mati: token BARU sesi A (pairA2) dan
	// sesi B yang sama sekali tidak terlibat pun ikut tercabut — nuke seluruh sesi.
	if _, err := a.Refresh(ctx, pairA2.RefreshToken); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("sesi A (baru, tidak dipakai penyerang) harus ikut tercabut, dapat %v", err)
	}
	if _, err := a.Refresh(ctx, pairB.RefreshToken); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("sesi B (tidak terlibat sama sekali) harus ikut tercabut, dapat %v", err)
	}
	for hash, rt := range tokens.byHash {
		if rt.RevokedAt == nil {
			t.Fatalf("semua token user harus tercabut setelah reuse terdeteksi, %q masih aktif", hash)
		}
	}
}
