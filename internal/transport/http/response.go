// Package http adalah lapisan transport: parse request, panggil service, terjemahkan error.
// Tidak ada logika bisnis di sini.
package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/caesarovera/nusaledger/internal/domain"
)

type dataEnvelope struct {
	Data any `json:"data"`
}

type pageEnvelope struct {
	Data       any     `json:"data"`
	NextCursor *string `json:"next_cursor"`
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code      string            `json:"code"`
	Message   string            `json:"message"`
	Fields    map[string]string `json:"fields"`
	RequestID string            `json:"request_id"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v) // klien sudah putus bila gagal; tidak ada yang bisa dilakukan
}

func writeData(w http.ResponseWriter, status int, data any) {
	writeJSON(w, status, dataEnvelope{Data: data})
}

func writePage(w http.ResponseWriter, data any, nextCursor string) {
	var nc *string
	if nextCursor != "" {
		nc = &nextCursor
	}
	writeJSON(w, http.StatusOK, pageEnvelope{Data: data, NextCursor: nc})
}

// apiError adalah hasil pemetaan error domain → kontrak API (docs/03 §4.3 + F-03).
type apiError struct {
	status  int
	code    string
	message string
	fields  map[string]string
}

// SATU-SATUNYA tempat error domain diterjemahkan ke status HTTP.
func mapError(err error) apiError {
	var ve *domain.ValidationError
	var maxBytes *http.MaxBytesError
	switch {
	case errors.As(err, &ve):
		return apiError{http.StatusBadRequest, "VALIDATION_ERROR", "permintaan tidak valid", ve.Fields}
	case errors.As(err, &maxBytes):
		return apiError{http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "ukuran body melebihi batas", nil}
	case errors.Is(err, domain.ErrValidation), errors.Is(err, domain.ErrInvalidCursor):
		return apiError{http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil}
	case errors.Is(err, errIdempotencyKeyRequired):
		return apiError{http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "header Idempotency-Key wajib untuk operasi uang", nil}
	case errors.Is(err, domain.ErrInvalidCredentials), errors.Is(err, domain.ErrInvalidToken), errors.Is(err, errUnauthenticated):
		return apiError{http.StatusUnauthorized, "UNAUTHENTICATED", "autentikasi gagal", nil}
	case errors.Is(err, domain.ErrForbidden):
		return apiError{http.StatusForbidden, "FORBIDDEN", "tidak berhak melakukan operasi ini", nil}
	case errors.Is(err, domain.ErrAccountNotFound):
		return apiError{http.StatusNotFound, "ACCOUNT_NOT_FOUND", "akun tidak ditemukan", nil}
	case errors.Is(err, domain.ErrTransactionNotFound):
		return apiError{http.StatusNotFound, "TRANSACTION_NOT_FOUND", "transaksi tidak ditemukan", nil}
	case errors.Is(err, domain.ErrUserNotFound), errors.Is(err, domain.ErrNotFound):
		return apiError{http.StatusNotFound, "NOT_FOUND", "data tidak ditemukan", nil}
	case errors.Is(err, domain.ErrEmailTaken):
		return apiError{http.StatusConflict, "EMAIL_TAKEN", "email sudah terdaftar", nil}
	case errors.Is(err, domain.ErrIdempotencyConflict):
		return apiError{http.StatusConflict, "IDEMPOTENCY_CONFLICT", "idempotency key sudah dipakai dengan isi permintaan berbeda", nil}
	case errors.Is(err, domain.ErrIdempotencyInFlight):
		return apiError{http.StatusConflict, "IDEMPOTENCY_IN_FLIGHT", "permintaan dengan key sama sedang diproses, coba lagi", nil}
	case errors.Is(err, domain.ErrAlreadyReversed), errors.Is(err, domain.ErrNotReversible):
		return apiError{http.StatusConflict, "ALREADY_REVERSED", "transaksi sudah dibalik atau tidak bisa dibalik", nil}
	case errors.Is(err, domain.ErrConcurrentModification):
		return apiError{http.StatusConflict, "CONCURRENT_MODIFICATION", "data berubah oleh proses lain, coba lagi", nil}
	case errors.Is(err, domain.ErrInsufficientBalance):
		return apiError{http.StatusUnprocessableEntity, "INSUFFICIENT_BALANCE", "saldo tidak mencukupi", nil}
	case errors.Is(err, domain.ErrAccountNotActive):
		return apiError{http.StatusUnprocessableEntity, "ACCOUNT_NOT_ACTIVE", "akun tidak aktif", nil}
	case errors.Is(err, domain.ErrSelfTransfer):
		return apiError{http.StatusUnprocessableEntity, "SELF_TRANSFER", "tidak bisa transfer ke akun sendiri", nil}
	case errors.Is(err, domain.ErrAmountOutOfRange), errors.Is(err, domain.ErrAmountNotPositive), errors.Is(err, domain.ErrAmountOverflow):
		return apiError{http.StatusUnprocessableEntity, "AMOUNT_OUT_OF_RANGE", "nominal di luar batas yang diizinkan", nil}
	case errors.Is(err, errRateLimited):
		return apiError{http.StatusTooManyRequests, "RATE_LIMITED", "terlalu banyak permintaan, coba lagi nanti", nil}
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return apiError{http.StatusServiceUnavailable, "TIMEOUT", "permintaan melebihi batas waktu", nil}
	}
	return apiError{http.StatusInternalServerError, "INTERNAL_ERROR", "terjadi kesalahan internal", nil}
}

// writeError merender error sesuai kontrak. Detail 500 hanya masuk log, tidak ke klien.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	ae := mapError(err)
	reqID := requestIDFrom(r.Context())
	if ae.status >= 500 {
		slog.ErrorContext(r.Context(), "kesalahan internal", "request_id", reqID, "err", err)
	}
	if ae.code == "IDEMPOTENCY_IN_FLIGHT" {
		w.Header().Set("Retry-After", "1")
	}
	writeJSON(w, ae.status, errorEnvelope{Error: errorBody{Code: ae.code, Message: ae.message, Fields: ae.fields, RequestID: reqID}})
}

// error khusus transport
var (
	errIdempotencyKeyRequired = errors.New("idempotency key wajib")
	errUnauthenticated        = errors.New("tidak terautentikasi")
	errRateLimited            = errors.New("melebihi batas laju")
)
