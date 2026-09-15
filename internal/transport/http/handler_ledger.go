package http

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/caesarovera/nusaledger/internal/domain"
	"github.com/caesarovera/nusaledger/internal/service"
)

type ledgerHandler struct {
	ledger *service.Ledger
	onTxn  func(txnType string, ok bool)
}

func (h *ledgerHandler) record(res *domain.PostResult, err error, fallbackType string) {
	if h.onTxn == nil {
		return
	}
	if err != nil {
		h.onTxn(fallbackType, false)
		return
	}
	h.onTxn(string(res.Transaction.Type), true)
}

// GET /accounts/me
func (h *ledgerHandler) myWallet(w http.ResponseWriter, r *http.Request) {
	actor, _ := actorFrom(r.Context())
	acc, err := h.ledger.MyWallet(r.Context(), actor)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, toAccountResponse(acc))
}

// GET /accounts/me/entries?cursor=&limit=
func (h *ledgerHandler) myEntries(w http.ResponseWriter, r *http.Request) {
	actor, _ := actorFrom(r.Context())
	limit := 0
	if s := r.URL.Query().Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			writeError(w, r, domain.NewValidationError(map[string]string{"limit": "harus bilangan bulat ≥ 0"}))
			return
		}
		limit = n
	}
	page, err := h.ledger.MyEntries(r.Context(), actor, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		writeError(w, r, err)
		return
	}
	out := make([]entryResponse, 0, len(page.Entries))
	for _, e := range page.Entries {
		out = append(out, toEntryResponse(e))
	}
	writePage(w, out, page.NextCursor)
}

func (h *ledgerHandler) moneyRequest(r *http.Request, req moneyRequest) service.MoneyRequest {
	actor, _ := actorFrom(r.Context())
	key, hash := idempotencyFrom(r.Context())
	return service.MoneyRequest{Actor: actor, IdempotencyKey: key, RequestHash: hash, AmountSen: req.AmountSen, Description: req.Description}
}

// POST /transactions/topup
func (h *ledgerHandler) topup(w http.ResponseWriter, r *http.Request) {
	var req moneyRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, r, err)
		return
	}
	res, err := h.ledger.Topup(r.Context(), h.moneyRequest(r, req))
	h.record(res, err, "TOPUP")
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeData(w, http.StatusCreated, toTransactionResponse(res))
}

// POST /transactions/withdraw
func (h *ledgerHandler) withdraw(w http.ResponseWriter, r *http.Request) {
	var req moneyRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, r, err)
		return
	}
	res, err := h.ledger.Withdraw(r.Context(), h.moneyRequest(r, req))
	h.record(res, err, "WITHDRAW")
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeData(w, http.StatusCreated, toTransactionResponse(res))
}

// POST /transactions/transfer
func (h *ledgerHandler) transfer(w http.ResponseWriter, r *http.Request) {
	var req transferRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	to, err := req.validate()
	if err != nil {
		writeError(w, r, err)
		return
	}
	res, err := h.ledger.Transfer(r.Context(), service.TransferRequest{MoneyRequest: h.moneyRequest(r, req.moneyRequest), ToAccountPublicID: to})
	h.record(res, err, "TRANSFER")
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeData(w, http.StatusCreated, toTransactionResponse(res))
}

// GET /transactions/{id}
func (h *ledgerHandler) getTransaction(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, domain.NewValidationError(map[string]string{"id": "harus UUID"}))
		return
	}
	actor, _ := actorFrom(r.Context())
	res, err := h.ledger.GetTransaction(r.Context(), actor, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, toTransactionResponse(res))
}

// POST /transactions/{id}/reverse (ADMIN)
func (h *ledgerHandler) reverse(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, domain.NewValidationError(map[string]string{"id": "harus UUID"}))
		return
	}
	var req reverseRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	actor, _ := actorFrom(r.Context())
	key, hash := idempotencyFrom(r.Context())
	res, err := h.ledger.Reverse(r.Context(), service.ReverseRequest{Actor: actor, IdempotencyKey: key, RequestHash: hash, TransactionID: id, Description: req.Description})
	h.record(res, err, "REVERSAL")
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeData(w, http.StatusCreated, toTransactionResponse(res))
}

// GET /internal/ledger/trial-balance (ADMIN)
func (h *ledgerHandler) trialBalance(w http.ResponseWriter, r *http.Request) {
	actor, _ := actorFrom(r.Context())
	tb, err := h.ledger.TrialBalance(r.Context(), actor)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, trialBalanceResponse{
		TotalDebitSen: int64(tb.TotalDebit), TotalCreditSen: int64(tb.TotalCredit), DifferenceSen: int64(tb.Difference),
		EntryCount: tb.EntryCount, Balanced: tb.Balanced(),
	})
}
