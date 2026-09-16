package http

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/caesarovera/nusaledger/internal/domain"
	"github.com/caesarovera/nusaledger/internal/platform/metrics"
	"github.com/caesarovera/nusaledger/internal/platform/token"
)

// key context bertipe privat supaya paket lain tidak bisa menabrak.
type ctxKey int

const (
	ctxRequestID ctxKey = iota
	ctxActor
	ctxIdempotencyKey
	ctxRequestHash
)

func requestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(ctxRequestID).(string)
	return id
}

func actorFrom(ctx context.Context) (domain.Actor, bool) {
	a, ok := ctx.Value(ctxActor).(domain.Actor)
	return a, ok
}

// requestID membuat id acak 16 byte per request; dibawa di context, header, dan log.
// Id dari klien TIDAK dipercaya (bisa dipalsukan untuk mengacaukan penelusuran log).
func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		id := hex.EncodeToString(b)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxRequestID, id)))
	})
}

// recoverer menangkap panic di goroutine handler dan mengembalikan 500, bukan mematikan server.
// Catatan: panic di goroutine lain yang dibuat handler TIDAK tertangkap di sini.
func recoverer(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() { //nolint:contextcheck // closure memakai r.Context() secara eksplisit
				if rec := recover(); rec != nil {
					log.ErrorContext(r.Context(), "panic tertangkap", "request_id", requestIDFrom(r.Context()), "panic", rec, "stack", string(debug.Stack()))
					writeJSON(w, http.StatusInternalServerError, errorEnvelope{Error: errorBody{
						Code: "INTERNAL_ERROR", Message: "terjadi kesalahan internal", RequestID: requestIDFrom(r.Context()),
					}})
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// logAndMeasure menulis satu baris log JSON per request dan mencatat metrik dengan label route berpola.
func logAndMeasure(log *slog.Logger, m *metrics.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			d := time.Since(start)
			route := chi.RouteContext(r.Context()).RoutePattern()
			if route == "" {
				route = "unmatched"
			}
			if m != nil {
				m.ObserveHTTP(r.Method, route, ww.Status(), d)
			}
			attrs := []any{
				"request_id", requestIDFrom(r.Context()), "method", r.Method, "route", route, "path", r.URL.Path,
				"status", ww.Status(), "duration_ms", d.Milliseconds(), "bytes", ww.BytesWritten(), "ip", r.RemoteAddr,
			}
			if a, ok := actorFrom(r.Context()); ok {
				attrs = append(attrs, "user_id", a.UserID)
			}
			if ww.Status() >= 500 {
				log.Error("request", attrs...)
			} else {
				log.Info("request", attrs...)
			}
		})
	}
}

// authenticate memverifikasi Bearer JWT dan menaruh Actor di context.
func authenticate(jwt *token.JWT) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if !strings.HasPrefix(h, "Bearer ") {
				writeError(w, r, errUnauthenticated)
				return
			}
			claims, err := jwt.Parse(strings.TrimPrefix(h, "Bearer "))
			if err != nil {
				writeError(w, r, err)
				return
			}
			actor := domain.Actor{UserID: claims.UserID, Role: claims.Role}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxActor, actor)))
		})
	}
}

// requireAdmin menolak selain ADMIN dengan 403.
func requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a, ok := actorFrom(r.Context()); !ok || !a.IsAdmin() {
			writeError(w, r, domain.ErrForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// allower adalah kontrak pembatas laju (in-memory di Fase 1).
type allower interface {
	Allow(key string) bool
}

// rateLimitByActor membatasi per user_id (mis. transfer 20/menit).
func rateLimitByActor(l allower) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			a, ok := actorFrom(r.Context())
			if !ok || !l.Allow("user:"+itoa(a.UserID)) {
				writeError(w, r, errRateLimited)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// rateLimitByIP membatasi per alamat IP, untuk endpoint SEBELUM ada Actor (register).
// Ditemukan G-3: /auth/register memanggil argon2id (64 MiB, t=3) tanpa pembatas apa pun —
// registrasi anonim berulang bisa menghabiskan CPU/memori server (BR tidak bernomor, ditutup pra-rilis).
func rateLimitByIP(l allower) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !l.Allow("ip:" + clientIP(r)) {
				writeError(w, r, errRateLimited)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// idempotency membaca header Idempotency-Key (wajib) dan menghitung hash body (BR-07, BR-08).
// Body dibaca di sini lalu dikembalikan ke request agar handler tetap bisa men-decode-nya.
func idempotency(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if key == "" || len(key) > 128 {
			writeError(w, r, errIdempotencyKeyRequired)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
		if err != nil {
			writeError(w, r, err)
			return
		}
		r.Body = io.NopCloser(strings.NewReader(string(body)))

		sum := sha256.Sum256([]byte(r.Method + " " + r.URL.Path + "\n" + string(body)))
		ctx := context.WithValue(r.Context(), ctxIdempotencyKey, key)
		ctx = context.WithValue(ctx, ctxRequestHash, hex.EncodeToString(sum[:]))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func idempotencyFrom(ctx context.Context) (key, hash string) {
	key, _ = ctx.Value(ctxIdempotencyKey).(string)
	hash, _ = ctx.Value(ctxRequestHash).(string)
	return key, hash
}

func itoa(n int64) string {
	const digits = "0123456789"
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = digits[n%10]
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// securityHeaders memasang header pertahanan-berlapis yang murah untuk API JSON-only
// (temuan audit keamanan): API ini tidak merender HTML dan tidak memakai cookie, jadi
// dampak nyata tanpa header ini rendah — tapi memasangnya tidak ada ruginya, dan
// beberapa klien/proxy tetap memeriksanya. HSTS hanya dikirim di produksi (harus di
// belakang TLS; mengirimnya di dev lewat HTTP biasa tidak berguna dan bisa membingungkan).
func securityHeaders(isProd bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
			h.Set("Referrer-Policy", "no-referrer")
			h.Set("Cross-Origin-Resource-Policy", "same-origin")
			if isProd {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}
