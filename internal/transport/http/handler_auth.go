package http

import (
	"net"
	"net/http"
	"strings"

	"github.com/caesarovera/nusaledger/internal/domain"
	"github.com/caesarovera/nusaledger/internal/service"
)

type authHandler struct {
	auth         *service.Auth
	loginLimiter allower
	onLoginFail  func(reason string)
}

// register: POST /auth/register → 201 user + dompet.
func (h *authHandler) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	user, wallet, err := h.auth.Register(r.Context(), service.RegisterInput{Email: req.Email, Password: req.Password, FullName: req.FullName})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeData(w, http.StatusCreated, registerResponse{User: toUserResponse(user), Wallet: toAccountResponse(wallet)})
}

// login: POST /auth/login → 200 pasangan token. Dibatasi per email DAN per IP, fail-closed.
func (h *authHandler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, r, err)
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if !h.loginLimiter.Allow("email:"+email) || !h.loginLimiter.Allow("ip:"+clientIP(r)) {
		h.onLoginFail("rate_limited")
		writeError(w, r, errRateLimited)
		return
	}
	pair, err := h.auth.Login(r.Context(), email, req.Password)
	if err != nil {
		if err == domain.ErrInvalidCredentials { //nolint:errorlint // sentinel langsung
			h.onLoginFail("invalid_credentials")
		}
		writeError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, toTokenResponse(pair))
}

// refresh: POST /auth/refresh → 200 pasangan baru (rotasi).
func (h *authHandler) refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, r, err)
		return
	}
	pair, err := h.auth.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, toTokenResponse(pair))
}

// logout: POST /auth/logout (auth) → 204.
func (h *authHandler) logout(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, r, err)
		return
	}
	if err := h.auth.Logout(r.Context(), req.RefreshToken); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// clientIP memakai RemoteAddr apa adanya. X-Forwarded-For hanya boleh dipercaya
// di belakang proxy yang dikenal — di Fase 1 tidak ada, jadi tidak dibaca.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
