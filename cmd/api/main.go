// Command api adalah satu-satunya entrypoint Fase 1.
// main.go merakit dependensi secara manual dari bawah ke atas: pool → repo → service → handler → router.
// Kalau main jadi berantakan, itu sinyal desain yang berguna — jangan sembunyikan dengan framework DI.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/caesarovera/nusaledger/internal/app"
	"github.com/caesarovera/nusaledger/internal/config"
	"github.com/caesarovera/nusaledger/internal/domain"
	"github.com/caesarovera/nusaledger/internal/platform/dbmigrate"
	"github.com/caesarovera/nusaledger/internal/platform/logger"
	"github.com/caesarovera/nusaledger/internal/platform/metrics"
	"github.com/caesarovera/nusaledger/internal/platform/password"
	"github.com/caesarovera/nusaledger/internal/platform/ratelimit"
	"github.com/caesarovera/nusaledger/internal/platform/token"
	"github.com/caesarovera/nusaledger/internal/repository/postgres"
	"github.com/caesarovera/nusaledger/internal/service"
	httptransport "github.com/caesarovera/nusaledger/internal/transport/http"
)

var version = "dev" // diisi -ldflags "-X main.version=..."

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := logger.New(os.Stdout, cfg.LogLevel).With("app", "nusaledger", "version", version, "env", cfg.Env)
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if cfg.RunMigrations {
		if err := dbmigrate.Up(cfg.MigrationURL()); err != nil {
			return err
		}
		log.Info("migration selesai")
	}

	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	limiters, closeLimiters, err := newLimiters(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeLimiters()

	// --- repository ---
	ledgerRepo := postgres.NewLedgerRepo(pool)
	accountRepo := postgres.NewAccountRepo(pool)
	idemRepo := postgres.NewIdempotencyRepo(pool)
	userRepo := postgres.NewUserRepo(pool)
	tokenRepo := postgres.NewRefreshTokenRepo(pool)

	// --- platform ---
	jwt, err := token.NewJWT(cfg.JWTSecret, cfg.AccessTokenTTL)
	if err != nil {
		return err
	}
	hasher := password.NewArgon2(password.DefaultParams)
	m := metrics.New()
	m.DBConnsMax.Set(float64(pool.Config().MaxConns))

	// --- service ---
	ledgerSvc, err := service.NewLedger(ctx, ledgerRepo, accountRepo, idemRepo, service.LedgerConfig{
		TransferFee: domain.Money(cfg.TransferFeeSen), MinTransfer: domain.Money(cfg.MinTransferSen), MaxTransfer: domain.Money(cfg.MaxTransferSen),
	})
	if err != nil {
		return err
	}
	authSvc, err := service.NewAuth(userRepo, tokenRepo, hasher, jwt, cfg.RefreshTokenTTL)
	if err != nil {
		return err
	}

	// --- transport ---
	readiness := httptransport.NewReadiness()
	router := httptransport.NewRouter(httptransport.Deps{
		Logger: log, Metrics: m, JWT: jwt, Auth: authSvc, Ledger: ledgerSvc,
		LoginLimiter:    limiters.login,
		TransferLimiter: limiters.transfer,
		RegisterLimiter: limiters.register,
		RefreshLimiter:  limiters.refresh,
		MoneyLimiter:    limiters.money,
		Ready:           pool.Ping,
		Readiness:       readiness,
		Timeout:         30 * time.Second,
		Prod:            cfg.IsProduction(),
	})
	srv := &http.Server{
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second, // Slowloris
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", cfg.Addr, err)
	}

	// --- job latar: verifikasi drift & trial balance → metrik (docs/06 F-05) ---
	go app.RunPeriodic(ctx, cfg.DriftCheckInterval, log, "verifikasi-ledger", func(ctx context.Context) error {
		drift, err := ledgerRepo.BalanceDrift(ctx)
		if err != nil {
			return err
		}
		tb, err := ledgerRepo.TrialBalance(ctx)
		if err != nil {
			return err
		}
		m.BalanceDrift.Set(float64(drift))
		m.TrialBalance.Set(float64(tb.Difference))
		m.DBConnsInUse.Set(float64(pool.Stat().AcquiredConns()))
		if drift != 0 || tb.Difference != 0 {
			log.Error("LEDGER TIDAK KONSISTEN — hentikan penerimaan transaksi", "drift_akun", drift, "trial_balance_selisih", int64(tb.Difference))
		}
		return nil
	})

	// --- pprof hanya di alamat internal, mux sendiri, bukan di produksi ---
	if !cfg.IsProduction() {
		go servePprof(ctx, cfg.PprofAddr, log)
	}

	return app.Serve(ctx, srv, ln, cfg.ShutdownWait, log, func() { readiness.Set(false) })
}

// apiLimiters mengelompokkan kelima pembatas laju supaya newLimiters punya satu titik
// keputusan backend (Redis vs in-memory), bukan diulang lima kali di run().
type apiLimiters struct {
	login, transfer, register, refresh, money interface{ Allow(string) bool }
}

// newLimiters memilih backend rate limiter dari cfg.RedisURL: kosong → in-memory
// (docs/06 F-04, cukup untuk satu instance); diisi → Redis (Fase 2, konsisten lintas
// banyak instance cmd/api). Redis diperiksa dengan Ping SEKALI di sini, sama seperti
// pool Postgres — gagal connect harus menghentikan startup, bukan baru terlihat saat
// request pertama masuk.
func newLimiters(ctx context.Context, cfg config.Config) (apiLimiters, func(), error) {
	noop := func() {}
	if cfg.RedisURL == "" {
		return apiLimiters{
			login:    ratelimit.New(cfg.LoginRateLimit, cfg.LoginRateWindow),
			transfer: ratelimit.New(cfg.TransferRateLimit, cfg.TransferRateWindow),
			register: ratelimit.New(cfg.RegisterRateLimit, cfg.RegisterRateWindow),
			refresh:  ratelimit.New(cfg.RefreshRateLimit, cfg.RefreshRateWindow),
			money:    ratelimit.New(cfg.MoneyRateLimit, cfg.MoneyRateWindow),
		}, noop, nil
	}

	opts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return apiLimiters{}, noop, fmt.Errorf("REDIS_URL tidak valid: %w", err)
	}
	client := redis.NewClient(opts)
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return apiLimiters{}, noop, fmt.Errorf("connect redis: %w", err)
	}
	return apiLimiters{
		login:    ratelimit.NewRedis(client, cfg.LoginRateLimit, cfg.LoginRateWindow),
		transfer: ratelimit.NewRedis(client, cfg.TransferRateLimit, cfg.TransferRateWindow),
		register: ratelimit.NewRedis(client, cfg.RegisterRateLimit, cfg.RegisterRateWindow),
		refresh:  ratelimit.NewRedis(client, cfg.RefreshRateLimit, cfg.RefreshRateWindow),
		money:    ratelimit.NewRedis(client, cfg.MoneyRateLimit, cfg.MoneyRateWindow),
	}, func() { _ = client.Close() }, nil
}

func servePprof(ctx context.Context, addr string, log *slog.Logger) {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	log.Info("pprof aktif", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Warn("pprof berhenti", "err", err)
	}
}
