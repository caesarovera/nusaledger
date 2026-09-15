package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/caesarovera/nusaledger/internal/domain"
	"github.com/caesarovera/nusaledger/internal/service"
)

// DTO TERPISAH dari domain: bentuk JSON boleh berubah tanpa menyentuh aturan bisnis.
// Nominal selalu bernama *_sen; id publik selalu UUID.

const maxBodyBytes = 1 << 20 // 1 MB

// decodeJSON membaca body dengan aturan ketat: batas ukuran, tolak field tak dikenal, satu objek saja.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields() // mencegah mass assignment
	if err := dec.Decode(dst); err != nil {
		var maxBytes *http.MaxBytesError
		var syntax *json.SyntaxError
		var typeErr *json.UnmarshalTypeError
		switch {
		case errors.As(err, &maxBytes):
			return err
		case errors.Is(err, io.EOF):
			return domain.NewValidationError(map[string]string{"body": "wajib"})
		case errors.As(err, &syntax):
			return domain.NewValidationError(map[string]string{"body": "JSON tidak valid"})
		case errors.As(err, &typeErr):
			return domain.NewValidationError(map[string]string{typeErr.Field: "tipe data salah"})
		case strings.HasPrefix(err.Error(), "json: unknown field"):
			return domain.NewValidationError(map[string]string{"body": "field tidak dikenal: " + strings.Trim(strings.TrimPrefix(err.Error(), "json: unknown field "), `"`)})
		}
		return domain.NewValidationError(map[string]string{"body": "tidak bisa dibaca"})
	}
	if dec.More() {
		return domain.NewValidationError(map[string]string{"body": "hanya satu objek JSON"})
	}
	return nil
}

// ---- request ----

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"full_name"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (r loginRequest) validate() error {
	f := map[string]string{}
	if strings.TrimSpace(r.Email) == "" {
		f["email"] = "wajib"
	}
	if r.Password == "" {
		f["password"] = "wajib"
	}
	return domain.NewValidationError(f)
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (r refreshRequest) validate() error {
	if r.RefreshToken == "" {
		return domain.NewValidationError(map[string]string{"refresh_token": "wajib"})
	}
	return nil
}

type moneyRequest struct {
	AmountSen   int64  `json:"amount_sen"`
	Description string `json:"description"`
}

func (r moneyRequest) validate() error {
	f := map[string]string{}
	if r.AmountSen <= 0 {
		f["amount_sen"] = "harus lebih dari nol"
	}
	if len(r.Description) > 255 {
		f["description"] = "maksimal 255 karakter"
	}
	return domain.NewValidationError(f)
}

type transferRequest struct {
	moneyRequest
	ToAccountPublicID string `json:"to_account_public_id"`
}

func (r transferRequest) validate() (uuid.UUID, error) {
	f := map[string]string{}
	if r.AmountSen <= 0 {
		f["amount_sen"] = "harus lebih dari nol"
	}
	if len(r.Description) > 255 {
		f["description"] = "maksimal 255 karakter"
	}
	to, err := uuid.Parse(r.ToAccountPublicID)
	if err != nil {
		f["to_account_public_id"] = "harus UUID"
	}
	return to, domain.NewValidationError(f)
}

type reverseRequest struct {
	Description string `json:"description"`
}

// ---- response ----

type userResponse struct {
	PublicID  uuid.UUID `json:"public_id"`
	Email     string    `json:"email"`
	FullName  string    `json:"full_name"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

func toUserResponse(u *domain.User) userResponse {
	return userResponse{PublicID: u.PublicID, Email: u.Email, FullName: u.FullName, Role: string(u.Role), CreatedAt: u.CreatedAt}
}

type registerResponse struct {
	User   userResponse    `json:"user"`
	Wallet accountResponse `json:"wallet"`
}

type tokenResponse struct {
	AccessToken      string    `json:"access_token"`
	TokenType        string    `json:"token_type"`
	AccessExpiresAt  time.Time `json:"access_expires_at"`
	RefreshToken     string    `json:"refresh_token"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
}

func toTokenResponse(p *service.TokenPair) tokenResponse {
	return tokenResponse{AccessToken: p.AccessToken, TokenType: "Bearer", AccessExpiresAt: p.AccessExpiresAt, RefreshToken: p.RefreshToken, RefreshExpiresAt: p.RefreshExpiresAt}
}

type accountResponse struct {
	PublicID   uuid.UUID `json:"public_id"`
	Type       string    `json:"type"`
	Status     string    `json:"status"`
	Currency   string    `json:"currency"`
	BalanceSen int64     `json:"balance_sen"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func toAccountResponse(a *domain.Account) accountResponse {
	return accountResponse{PublicID: a.PublicID, Type: string(a.Type), Status: string(a.Status), Currency: "IDR", BalanceSen: int64(a.Balance), UpdatedAt: a.UpdatedAt}
}

type entryResponse struct {
	ID              int64     `json:"id"`
	TransactionID   uuid.UUID `json:"transaction_id"`
	AccountPublicID uuid.UUID `json:"account_public_id"`
	AccountType     string    `json:"account_type"`
	Direction       string    `json:"direction"`
	AmountSen       int64     `json:"amount_sen"`
	BalanceAfterSen int64     `json:"balance_after_sen"`
	TxnType         string    `json:"txn_type,omitempty"`
	Description     string    `json:"description,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

func toEntryResponse(e domain.PostedEntry) entryResponse {
	return entryResponse{
		ID: e.ID, TransactionID: e.TransactionID, AccountPublicID: e.AccountPublicID, AccountType: string(e.AccountType),
		Direction: string(e.Direction), AmountSen: int64(e.Amount), BalanceAfterSen: int64(e.BalanceAfter),
		TxnType: string(e.TxnType), Description: e.Description, CreatedAt: e.CreatedAt,
	}
}

type transactionResponse struct {
	TransactionID uuid.UUID       `json:"transaction_id"`
	Type          string          `json:"type"`
	Status        string          `json:"status"`
	Description   string          `json:"description"`
	AmountSen     int64           `json:"amount_sen"`
	FeeSen        int64           `json:"fee_sen"`
	ReversesID    *uuid.UUID      `json:"reverses_transaction_id,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	Entries       []entryResponse `json:"entries"`
}

// toTransactionResponse merender hasil posting. amount_sen = nominal utama operasi
// (kredit dompet penerima untuk transfer; nominal dompet untuk topup/withdraw);
// fee_sen = total yang masuk SYSTEM_FEE_REVENUE.
func toTransactionResponse(p *domain.PostResult) transactionResponse {
	t := p.Transaction
	resp := transactionResponse{
		TransactionID: t.ID, Type: string(t.Type), Status: string(t.Status), Description: t.Description,
		ReversesID: t.ReversesID, CreatedAt: t.CreatedAt, Entries: make([]entryResponse, 0, len(p.Entries)),
	}
	for _, e := range p.Entries {
		resp.Entries = append(resp.Entries, toEntryResponse(e))
		switch {
		case e.AccountType == domain.AccountSystemFeeRevenue:
			resp.FeeSen += int64(e.Amount)
		case e.AccountType == domain.AccountUserWallet && (t.Type != domain.TxnTransfer || e.Direction == domain.DirectionCredit):
			resp.AmountSen = int64(e.Amount)
		}
	}
	return resp
}

type trialBalanceResponse struct {
	TotalDebitSen  int64 `json:"total_debit_sen"`
	TotalCreditSen int64 `json:"total_credit_sen"`
	DifferenceSen  int64 `json:"difference_sen"`
	EntryCount     int64 `json:"entry_count"`
	Balanced       bool  `json:"balanced"`
}
