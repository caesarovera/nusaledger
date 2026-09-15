package config_test

import (
	"testing"

	"github.com/caesarovera/nusaledger/internal/config"
)

func setBase(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://x:y@127.0.0.1:5433/db")
	t.Setenv("JWT_SECRET", "rahasia-dev")
	t.Setenv("APP_ENV", "development")
}

func TestLoad_Default(t *testing.T) {
	setBase(t)
	c, err := config.Load()
	if err != nil {
		t.Fatalf("mau sukses, dapat %v", err)
	}
	if c.TransferFeeSen != 100000 || c.MinTransferSen != 1000000 || c.MaxTransferSen != 5000000000 {
		t.Errorf("default nominal salah: %+v", c)
	}
	if c.Addr != ":8080" || c.PprofAddr != "127.0.0.1:6060" {
		t.Errorf("default alamat salah: %+v", c)
	}
}

func TestLoad_TolakStart(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{"tanpa DATABASE_URL", map[string]string{"DATABASE_URL": ""}},
		{"tanpa JWT_SECRET", map[string]string{"JWT_SECRET": ""}},
		{"produksi dengan secret pendek", map[string]string{"APP_ENV": "production", "JWT_SECRET": "pendek"}},
		{"APP_ENV tidak dikenal", map[string]string{"APP_ENV": "staging"}},
		{"MIN > MAX", map[string]string{"MIN_TRANSFER_SEN": "100", "MAX_TRANSFER_SEN": "50"}},
		{"MIN nol", map[string]string{"MIN_TRANSFER_SEN": "0"}},
		{"fee negatif", map[string]string{"TRANSFER_FEE_SEN": "-1"}},
		{"refresh lebih pendek dari access", map[string]string{"ACCESS_TOKEN_TTL": "1h", "REFRESH_TOKEN_TTL": "30m"}},
		{"durasi tidak valid", map[string]string{"SHUTDOWN_WAIT": "cepat"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setBase(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			if _, err := config.Load(); err == nil {
				t.Fatal("mau error, dapat nil")
			}
		})
	}
}
