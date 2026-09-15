// Package config memuat dan MEMVALIDASI konfigurasi saat startup.
// Aplikasi yang start dengan konfigurasi salah harus menolak start,
// bukan gagal tiga jam kemudian saat ada yang login.
package config

import (
	"errors"
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

const (
	EnvDevelopment = "development"
	EnvProduction  = "production"
	EnvTest        = "test"
)

type Config struct {
	Addr      string `env:"ADDR" envDefault:":8080"`
	PprofAddr string `env:"PPROF_ADDR" envDefault:"127.0.0.1:6060"`
	Env       string `env:"APP_ENV" envDefault:"development"`
	LogLevel  string `env:"LOG_LEVEL" envDefault:"info"`
	// `required` saja tidak cukup: variabel yang ADA tapi KOSONG dianggap terisi. `notEmpty` menutupnya.
	DatabaseURL string `env:"DATABASE_URL,required,notEmpty"`

	JWTSecret       string        `env:"JWT_SECRET,required,notEmpty"`
	AccessTokenTTL  time.Duration `env:"ACCESS_TOKEN_TTL" envDefault:"15m"`
	RefreshTokenTTL time.Duration `env:"REFRESH_TOKEN_TTL" envDefault:"720h"` // 30 hari

	// Semua nominal dalam SEN. Biaya & batas ada di config, bukan hardcode (BR-13, BR-14).
	TransferFeeSen int64 `env:"TRANSFER_FEE_SEN" envDefault:"100000"`     // Rp 1.000
	MinTransferSen int64 `env:"MIN_TRANSFER_SEN" envDefault:"1000000"`    // Rp 10.000
	MaxTransferSen int64 `env:"MAX_TRANSFER_SEN" envDefault:"5000000000"` // Rp 50.000.000

	ShutdownWait       time.Duration `env:"SHUTDOWN_WAIT" envDefault:"30s"`
	DriftCheckInterval time.Duration `env:"DRIFT_CHECK_INTERVAL" envDefault:"60s"`

	// Rate limit (in-memory di Fase 1, lihat docs/06 F-04)
	LoginRateLimit     int           `env:"LOGIN_RATE_LIMIT" envDefault:"5"`
	LoginRateWindow    time.Duration `env:"LOGIN_RATE_WINDOW" envDefault:"15m"`
	TransferRateLimit  int           `env:"TRANSFER_RATE_LIMIT" envDefault:"20"`
	TransferRateWindow time.Duration `env:"TRANSFER_RATE_WINDOW" envDefault:"1m"`
}

// Load membaca env dan memvalidasi. Error berarti aplikasi tidak boleh start.
func Load() (Config, error) {
	var c Config
	if err := env.Parse(&c); err != nil {
		return c, fmt.Errorf("memuat konfigurasi: %w", err)
	}
	if err := c.validate(); err != nil {
		return c, fmt.Errorf("konfigurasi tidak valid: %w", err)
	}
	return c, nil
}

func (c Config) validate() error {
	switch c.Env {
	case EnvDevelopment, EnvProduction, EnvTest:
	default:
		return fmt.Errorf("APP_ENV %q tidak dikenal", c.Env)
	}
	if c.Env == EnvProduction && len(c.JWTSecret) < 32 {
		return errors.New("JWT_SECRET minimal 32 byte di produksi")
	}
	if c.TransferFeeSen < 0 {
		return errors.New("TRANSFER_FEE_SEN tidak boleh negatif")
	}
	if c.MinTransferSen <= 0 || c.MaxTransferSen <= c.MinTransferSen {
		return errors.New("batas transfer tidak masuk akal: MIN harus > 0 dan MAX > MIN")
	}
	if c.AccessTokenTTL <= 0 || c.RefreshTokenTTL <= c.AccessTokenTTL {
		return errors.New("umur token tidak masuk akal: REFRESH harus lebih lama dari ACCESS")
	}
	if c.ShutdownWait <= 0 || c.DriftCheckInterval <= 0 {
		return errors.New("SHUTDOWN_WAIT dan DRIFT_CHECK_INTERVAL harus > 0")
	}
	return nil
}

// IsProduction benar di lingkungan produksi; dipakai untuk mematikan pprof dsb.
func (c Config) IsProduction() bool { return c.Env == EnvProduction }
