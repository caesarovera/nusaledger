package http

import (
	"context"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/caesarovera/nusaledger/internal/platform/metrics"
	"github.com/caesarovera/nusaledger/internal/platform/token"
	"github.com/caesarovera/nusaledger/internal/service"
)

// Deps adalah semua yang dibutuhkan router; dirakit di main (DI manual).
type Deps struct {
	Logger          *slog.Logger
	Metrics         *metrics.Metrics
	JWT             *token.JWT
	Auth            *service.Auth
	Ledger          *service.Ledger
	LoginLimiter    allower
	TransferLimiter allower
	RegisterLimiter allower // per IP; argon2id di register/login mahal, tanpa limiter bisa jadi vektor DoS (G-3)
	// Ready dipanggil /readyz: cek DB. Saat shutdown, Readiness.Set(false) membuat /readyz 503 duluan.
	Ready     func(ctx context.Context) error
	Readiness *Readiness
	Timeout   time.Duration
}

// Readiness adalah saklar siap-menerima-trafik. Dimatikan sebelum server berhenti
// supaya load balancer berhenti mengirim request baru sementara yang lama diselesaikan.
type Readiness struct{ ready atomic.Bool }

func NewReadiness() *Readiness      { r := &Readiness{}; r.ready.Store(true); return r }
func (r *Readiness) Set(ready bool) { r.ready.Store(ready) }
func (r *Readiness) IsReady() bool  { return r.ready.Load() }

// NewRouter merakit seluruh endpoint (docs/03 §4.1). Urutan middleware: docs skill api-contract.
func NewRouter(d Deps) http.Handler {
	if d.Timeout <= 0 {
		d.Timeout = 30 * time.Second
	}
	r := chi.NewRouter()

	// operasional — tanpa auth, tanpa log per request agar tidak membanjiri
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	r.Get("/readyz", func(w http.ResponseWriter, req *http.Request) {
		if d.Readiness != nil && !d.Readiness.IsReady() {
			http.Error(w, "shutting down", http.StatusServiceUnavailable)
			return
		}
		ctx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
		defer cancel()
		if d.Ready != nil {
			if err := d.Ready(ctx); err != nil {
				http.Error(w, "database tidak siap", http.StatusServiceUnavailable)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})
	if d.Metrics != nil {
		r.Method(http.MethodGet, "/metrics", d.Metrics.Handler())
	}

	auth := &authHandler{auth: d.Auth, loginLimiter: d.LoginLimiter, onLoginFail: func(reason string) {
		if d.Metrics != nil {
			d.Metrics.LoginFailures.WithLabelValues(reason).Inc()
		}
	}}
	ledger := &ledgerHandler{ledger: d.Ledger, onTxn: func(t string, ok bool) {
		if d.Metrics != nil {
			status := "ok"
			if !ok {
				status = "failed"
			}
			d.Metrics.Transactions.WithLabelValues(t, status).Inc()
		}
	}}

	r.Route("/api/v1", func(api chi.Router) {
		api.Use(recoverer(d.Logger)) // paling luar: panic di middleware lain pun tertangkap
		api.Use(requestID)           // id sebelum log
		// Sengaja TANPA middleware.RealIP: ia mempercayai X-Forwarded-For dari siapa pun (spoofing IP,
		// GHSA-3fxj-6jh8-hvhx). Rate limit per IP memakai RemoteAddr; proxy tepercaya diatur di Fase 2.
		api.Use(logAndMeasure(d.Logger, d.Metrics))
		api.Use(middleware.Timeout(d.Timeout))
		api.Use(middleware.NoCache)

		api.Route("/auth", func(a chi.Router) {
			a.With(rateLimitByIP(d.RegisterLimiter)).Post("/register", auth.register)
			a.Post("/login", auth.login)
			a.Post("/refresh", auth.refresh)
			a.With(authenticate(d.JWT)).Post("/logout", auth.logout)
		})

		api.Group(func(p chi.Router) {
			p.Use(authenticate(d.JWT))

			p.Get("/accounts/me", ledger.myWallet)
			p.Get("/accounts/me/entries", ledger.myEntries)

			p.Route("/transactions", func(t chi.Router) {
				money := t.With(idempotency)
				money.Post("/topup", ledger.topup)
				money.Post("/withdraw", ledger.withdraw)
				money.With(rateLimitByActor(d.TransferLimiter)).Post("/transfer", ledger.transfer)
				t.Get("/{id}", ledger.getTransaction)
				t.With(requireAdmin, idempotency).Post("/{id}/reverse", ledger.reverse)
			})

			p.With(requireAdmin).Get("/internal/ledger/trial-balance", ledger.trialBalance)
		})
	})

	return r
}
