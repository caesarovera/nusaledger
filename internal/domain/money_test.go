package domain_test

import (
	"errors"
	"math"
	"testing"

	"github.com/caesarovera/nusaledger/internal/domain"
)

func TestNewMoney(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		sen     int64
		want    domain.Money
		wantErr error
	}{
		{"nominal wajar", 50_000_00, 50_000_00, nil},
		{"satu sen", 1, 1, nil},
		{"nol ditolak", 0, 0, domain.ErrAmountNotPositive},
		{"negatif ditolak", -1, 0, domain.ErrAmountNotPositive},
		{"tepat Rp 1 triliun diterima", 1_000_000_000_000_00, 1_000_000_000_000_00, nil},
		{"lebih dari Rp 1 triliun ditolak", 1_000_000_000_000_01, 0, domain.ErrAmountOverflow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := domain.NewMoney(tt.sen)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error: mau %v, dapat %v", tt.wantErr, err)
			}
			if got != tt.want {
				t.Fatalf("nilai: mau %d, dapat %d", tt.want, got)
			}
		})
	}
}

// T-14: overflow harus terdeteksi, bukan berputar diam-diam menjadi negatif.
func TestMoney_Add_Overflow(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		a, b    domain.Money
		want    domain.Money
		wantErr error
	}{
		{"penjumlahan biasa", 100, 50, 150, nil},
		{"tambah negatif", 100, -30, 70, nil},
		{"overflow positif", domain.Money(math.MaxInt64 - 10), 100, 0, domain.ErrAmountOverflow},
		{"overflow negatif", domain.Money(math.MinInt64 + 10), -100, 0, domain.ErrAmountOverflow},
		{"tepat di batas atas", domain.Money(math.MaxInt64 - 1), 1, domain.Money(math.MaxInt64), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := tt.a.Add(tt.b)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error: mau %v, dapat %v", tt.wantErr, err)
			}
			if got != tt.want {
				t.Fatalf("nilai: mau %d, dapat %d", tt.want, got)
			}
		})
	}
}

func TestMoney_Sub(t *testing.T) {
	t.Parallel()
	got, err := domain.Money(100).Sub(30)
	if err != nil || got != 70 {
		t.Fatalf("100-30: mau 70 tanpa error, dapat %d, %v", got, err)
	}
	if _, err := domain.Money(0).Sub(domain.Money(math.MinInt64)); !errors.Is(err, domain.ErrAmountOverflow) {
		t.Fatalf("mengurangi MinInt64: mau ErrAmountOverflow, dapat %v", err)
	}
}

func TestMoney_String(t *testing.T) {
	t.Parallel()
	tests := []struct {
		m    domain.Money
		want string
	}{
		{5_000_050, "Rp 50000,50"},
		{100, "Rp 1,00"},
		{0, "Rp 0,00"},
		{7, "Rp 0,07"},
		{-150, "-Rp 1,50"},
	}
	for _, tt := range tests {
		if got := tt.m.String(); got != tt.want {
			t.Errorf("Money(%d).String(): mau %q, dapat %q", int64(tt.m), tt.want, got)
		}
	}
}
