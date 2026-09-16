//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/caesarovera/nusaledger/internal/domain"
	"github.com/caesarovera/nusaledger/internal/platform/metrics"
	"github.com/caesarovera/nusaledger/internal/platform/password"
	"github.com/caesarovera/nusaledger/internal/platform/ratelimit"
	"github.com/caesarovera/nusaledger/internal/platform/token"
	"github.com/caesarovera/nusaledger/internal/repository/postgres"
	"github.com/caesarovera/nusaledger/internal/service"
	httptransport "github.com/caesarovera/nusaledger/internal/transport/http"
)

// E2E: HTTP → service → repository → PostgreSQL sungguhan, dalam satu proses (httptest).

type apiServer struct {
	*httptest.Server
	t *testing.T
}

type serverOpts struct {
	loginLimit    int
	transferLimit int
	registerLimit int
	refreshLimit  int
	moneyLimit    int
}

func newAPIServer(t *testing.T, o serverOpts) *apiServer {
	t.Helper()
	resetDB(t)
	ctx := testCtx(t)
	if o.loginLimit == 0 {
		o.loginLimit = 1000
	}
	if o.transferLimit == 0 {
		o.transferLimit = 1000
	}
	if o.registerLimit == 0 {
		o.registerLimit = 1000
	}
	if o.refreshLimit == 0 {
		o.refreshLimit = 1000
	}
	if o.moneyLimit == 0 {
		o.moneyLimit = 1000
	}

	jwt, err := token.NewJWT("secret-untuk-test-yang-panjang-32b!", 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	ledgerSvc, err := service.NewLedger(ctx, postgres.NewLedgerRepo(testPool), postgres.NewAccountRepo(testPool), postgres.NewIdempotencyRepo(testPool),
		service.LedgerConfig{TransferFee: 1_000 * domain.Rupiah, MinTransfer: 10_000 * domain.Rupiah, MaxTransfer: 50_000_000 * domain.Rupiah})
	if err != nil {
		t.Fatal(err)
	}
	authSvc, err := service.NewAuth(postgres.NewUserRepo(testPool), postgres.NewRefreshTokenRepo(testPool), password.NewArgon2(password.FastParams), jwt, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	router := httptransport.NewRouter(httptransport.Deps{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Metrics: metrics.New(), JWT: jwt, Auth: authSvc, Ledger: ledgerSvc,
		LoginLimiter: ratelimit.New(o.loginLimit, time.Minute), TransferLimiter: ratelimit.New(o.transferLimit, time.Minute),
		RegisterLimiter: ratelimit.New(o.registerLimit, time.Minute),
		RefreshLimiter:  ratelimit.New(o.refreshLimit, time.Minute), MoneyLimiter: ratelimit.New(o.moneyLimit, time.Minute),
		Ready: testPool.Ping, Readiness: httptransport.NewReadiness(), Timeout: 10 * time.Second,
	})
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return &apiServer{Server: srv, t: t}
}

type resp struct {
	status int
	raw    []byte
	body   map[string]any
	header http.Header
}

func (s *apiServer) do(method, path string, body any, headers map[string]string) resp {
	s.t.Helper()
	var buf io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		buf = strings.NewReader(b)
	case []byte:
		buf = bytes.NewReader(b)
	default:
		j, _ := json.Marshal(b)
		buf = bytes.NewReader(j)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, s.URL+path, buf)
	if err != nil {
		s.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	r, err := s.Client().Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer r.Body.Close()
	raw, _ := io.ReadAll(r.Body)
	out := resp{status: r.StatusCode, raw: raw, header: r.Header}
	if len(raw) > 0 && strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		_ = json.Unmarshal(raw, &out.body)
	}
	return out
}

func (r resp) data() map[string]any {
	d, _ := r.body["data"].(map[string]any)
	return d
}

func (r resp) errCode() string {
	e, _ := r.body["error"].(map[string]any)
	c, _ := e["code"].(string)
	return c
}

func bearer(tok string) map[string]string { return map[string]string{"Authorization": "Bearer " + tok} }

// firstLines mengambil baris yang mengandung substring, untuk pesan kegagalan yang ringkas.
func firstLines(raw []byte, contains string) string {
	var out []string
	for _, l := range strings.Split(string(raw), "\n") {
		if strings.Contains(l, contains) {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

func withKey(tok, key string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + tok, "Idempotency-Key": key}
}

// registerAndLogin mendaftarkan user lalu login; mengembalikan access token dan public_id dompet.
func (s *apiServer) registerAndLogin(email string) (tok, walletID string) {
	s.t.Helper()
	r := s.do("POST", "/api/v1/auth/register", map[string]string{"email": email, "password": "rahasia123", "full_name": "Uji"}, nil)
	if r.status != 201 {
		s.t.Fatalf("register %s: %d %s", email, r.status, r.raw)
	}
	wallet, _ := r.data()["wallet"].(map[string]any)
	walletID, _ = wallet["public_id"].(string)
	l := s.do("POST", "/api/v1/auth/login", map[string]string{"email": email, "password": "rahasia123"}, nil)
	if l.status != 200 {
		s.t.Fatalf("login %s: %d %s", email, l.status, l.raw)
	}
	tok, _ = l.data()["access_token"].(string)
	return tok, walletID
}

func (s *apiServer) makeAdmin(email string) {
	s.t.Helper()
	if _, err := testPool.Exec(testCtx(s.t), `UPDATE users SET role = 'ADMIN' WHERE email = $1`, email); err != nil {
		s.t.Fatal(err)
	}
}

func expect(t *testing.T, r resp, status int, code string) {
	t.Helper()
	if r.status != status || (code != "" && r.errCode() != code) {
		t.Fatalf("mau %d %s, dapat %d %s: %s", status, code, r.status, r.errCode(), r.raw)
	}
}

func TestHTTP_Operasional(t *testing.T) {
	s := newAPIServer(t, serverOpts{})
	hz := s.do("GET", "/healthz", nil, nil)
	expect(t, hz, 200, "")
	// Header keamanan dipasang di ROOT router: /healthz pun harus ikut terlindungi (temuan audit).
	if hz.header.Get("X-Content-Type-Options") != "nosniff" || hz.header.Get("Content-Security-Policy") == "" {
		t.Fatalf("header keamanan tidak terpasang di /healthz: %v", hz.header)
	}
	expect(t, s.do("GET", "/readyz", nil, nil), 200, "")
	// counter berlabel baru muncul setelah ada request; gauge selalu ada sejak awal
	m := s.do("GET", "/metrics", nil, nil)
	if m.status != 200 || !strings.Contains(string(m.raw), "ledger_balance_drift_total") {
		t.Fatalf("metrics: %d", m.status)
	}
	s.do("GET", "/api/v1/accounts/me", nil, nil) // 401, tapi tercatat
	if m2 := s.do("GET", "/metrics", nil, nil); !strings.Contains(string(m2.raw), `http_requests_total{method="GET",route="/api/v1/accounts/me",status="401"}`) {
		t.Fatalf("metrik request harus berlabel route berpola: %s", firstLines(m2.raw, "http_requests_total"))
	}
}

func TestHTTP_AuthAlur(t *testing.T) {
	s := newAPIServer(t, serverOpts{})

	// validasi & duplikat
	r := s.do("POST", "/api/v1/auth/register", map[string]string{"email": "x", "password": "p", "full_name": ""}, nil)
	expect(t, r, 400, "VALIDATION_ERROR")
	if f, _ := r.body["error"].(map[string]any)["fields"].(map[string]any); len(f) != 3 {
		t.Fatalf("mau 3 field bermasalah, dapat %v", f)
	}
	expect(t, s.do("POST", "/api/v1/auth/register", `{"email":"a@x.com","password":"rahasia123","full_name":"A","hacker":true}`, nil), 400, "VALIDATION_ERROR")
	tokA, _ := s.registerAndLogin("andi@test.local")
	expect(t, s.do("POST", "/api/v1/auth/register", map[string]string{"email": "ANDI@test.local", "password": "rahasia123", "full_name": "Dup"}, nil), 409, "EMAIL_TAKEN")

	// login gagal & pesan seragam
	expect(t, s.do("POST", "/api/v1/auth/login", map[string]string{"email": "andi@test.local", "password": "salah"}, nil), 401, "UNAUTHENTICATED")
	expect(t, s.do("POST", "/api/v1/auth/login", map[string]string{"email": "tidakada@test.local", "password": "salah"}, nil), 401, "UNAUTHENTICATED")

	// akses tanpa / dengan token rusak
	expect(t, s.do("GET", "/api/v1/accounts/me", nil, nil), 401, "UNAUTHENTICATED")
	expect(t, s.do("GET", "/api/v1/accounts/me", nil, bearer(tokA+"x")), 401, "UNAUTHENTICATED")
	me := s.do("GET", "/api/v1/accounts/me", nil, bearer(tokA))
	expect(t, me, 200, "")
	if me.data()["balance_sen"].(float64) != 0 || me.data()["type"] != "USER_WALLET" {
		t.Fatalf("dompet baru: %s", me.raw)
	}
	if me.header.Get("X-Request-ID") == "" {
		t.Fatal("X-Request-ID harus ada")
	}

	// refresh rotasi & logout
	l := s.do("POST", "/api/v1/auth/login", map[string]string{"email": "andi@test.local", "password": "rahasia123"}, nil)
	refresh := l.data()["refresh_token"].(string)
	r2 := s.do("POST", "/api/v1/auth/refresh", map[string]string{"refresh_token": refresh}, nil)
	expect(t, r2, 200, "")
	expect(t, s.do("POST", "/api/v1/auth/refresh", map[string]string{"refresh_token": refresh}, nil), 401, "UNAUTHENTICATED")
	newRefresh := r2.data()["refresh_token"].(string)
	expect(t, s.do("POST", "/api/v1/auth/logout", map[string]string{"refresh_token": newRefresh}, bearer(tokA)), 204, "")
	expect(t, s.do("POST", "/api/v1/auth/refresh", map[string]string{"refresh_token": newRefresh}, nil), 401, "UNAUTHENTICATED")
}

func TestHTTP_LoginRateLimit(t *testing.T) {
	s := newAPIServer(t, serverOpts{loginLimit: 3})
	s.registerAndLogin("andi@test.local") // memakai 1 percobaan
	for i := 0; i < 2; i++ {
		expect(t, s.do("POST", "/api/v1/auth/login", map[string]string{"email": "andi@test.local", "password": "salah"}, nil), 401, "UNAUTHENTICATED")
	}
	expect(t, s.do("POST", "/api/v1/auth/login", map[string]string{"email": "andi@test.local", "password": "rahasia123"}, nil), 429, "RATE_LIMITED")
}

// Temuan G-3: /auth/register memanggil argon2id (mahal) tanpa pembatas. Dibatasi per IP.
func TestHTTP_RegisterRateLimit(t *testing.T) {
	s := newAPIServer(t, serverOpts{registerLimit: 3})
	for i := 0; i < 3; i++ {
		r := s.do("POST", "/api/v1/auth/register", map[string]string{
			"email": fmt.Sprintf("u%d@test.local", i), "password": "rahasia123", "full_name": "Uji",
		}, nil)
		expect(t, r, 201, "")
	}
	expect(t, s.do("POST", "/api/v1/auth/register", map[string]string{
		"email": "u4@test.local", "password": "rahasia123", "full_name": "Uji",
	}, nil), 429, "RATE_LIMITED")
}

// Temuan audit G-3: /auth/refresh tidak butuh auth dan melakukan lookup DB tak
// terbatas. Dibatasi per IP.
func TestHTTP_RefreshRateLimit(t *testing.T) {
	s := newAPIServer(t, serverOpts{refreshLimit: 3})
	_, _ = s.registerAndLogin("andi@test.local")
	for i := 0; i < 3; i++ {
		// token asing pun tetap dihitung ke limiter — pemeriksaan validitas ada SETELAH limiter
		expect(t, s.do("POST", "/api/v1/auth/refresh", map[string]string{"refresh_token": "asing"}, nil), 401, "UNAUTHENTICATED")
	}
	expect(t, s.do("POST", "/api/v1/auth/refresh", map[string]string{"refresh_token": "asing"}, nil), 429, "RATE_LIMITED")
}

// Temuan audit G-3: topup & withdraw sama-sama menulis penuh dengan row lock, sama
// seperti transfer, tapi sebelumnya tidak punya limiter sendiri.
func TestHTTP_MoneyRateLimit(t *testing.T) {
	s := newAPIServer(t, serverOpts{moneyLimit: 2})
	tokA, _ := s.registerAndLogin("andi@test.local")

	body := map[string]any{"amount_sen": 1_000_000}
	expect(t, s.do("POST", "/api/v1/transactions/topup", body, withKey(tokA, uuid.NewString())), 201, "")
	expect(t, s.do("POST", "/api/v1/transactions/withdraw", body, withKey(tokA, uuid.NewString())), 201, "") // bucket sama, sudah 2/2
	expect(t, s.do("POST", "/api/v1/transactions/topup", body, withKey(tokA, uuid.NewString())), 429, "RATE_LIMITED")
}

func TestHTTP_UangIdempotencyDanTransfer(t *testing.T) {
	s := newAPIServer(t, serverOpts{})
	tokA, walletA := s.registerAndLogin("andi@test.local")
	tokB, walletB := s.registerAndLogin("budi@test.local")
	_, _ = walletA, tokB

	// BR-07: tanpa key → 400
	expect(t, s.do("POST", "/api/v1/transactions/topup", map[string]any{"amount_sen": 10_000_000}, bearer(tokA)), 400, "IDEMPOTENCY_KEY_REQUIRED")

	// topup 100.000 → 201
	key := uuid.NewString()
	body := map[string]any{"amount_sen": 10_000_000, "description": "isi"}
	r1 := s.do("POST", "/api/v1/transactions/topup", body, withKey(tokA, key))
	expect(t, r1, 201, "")
	if r1.data()["amount_sen"].(float64) != 10_000_000 || r1.data()["type"] != "TOPUP" {
		t.Fatalf("topup: %s", r1.raw)
	}
	for _, e := range r1.data()["entries"].([]any) {
		if _, has := e.(map[string]any)["account_id"]; has {
			t.Fatal("id internal akun tidak boleh bocor ke API")
		}
	}

	// replay: key + body sama → respons IDENTIK, saldo tidak naik dua kali (T-05 via HTTP)
	r1b := s.do("POST", "/api/v1/transactions/topup", body, withKey(tokA, key))
	expect(t, r1b, 201, "")
	if !bytes.Equal(r1.raw, r1b.raw) {
		t.Fatalf("replay harus identik:\n%s\n%s", r1.raw, r1b.raw)
	}
	// T-06: key sama, body beda → 409
	expect(t, s.do("POST", "/api/v1/transactions/topup", map[string]any{"amount_sen": 1}, withKey(tokA, key)), 409, "IDEMPOTENCY_CONFLICT")
	if me := s.do("GET", "/api/v1/accounts/me", nil, bearer(tokA)); me.data()["balance_sen"].(float64) != 10_000_000 {
		t.Fatalf("saldo harus tepat 100.000: %s", me.raw)
	}

	// transfer A→B 50.000: 201, fee 1.000
	tr := s.do("POST", "/api/v1/transactions/transfer", map[string]any{"to_account_public_id": walletB, "amount_sen": 5_000_000, "description": "bayar kos"}, withKey(tokA, uuid.NewString()))
	expect(t, tr, 201, "")
	if tr.data()["amount_sen"].(float64) != 5_000_000 || tr.data()["fee_sen"].(float64) != 100_000 {
		t.Fatalf("transfer: %s", tr.raw)
	}
	txnID := tr.data()["transaction_id"].(string)
	if s.do("GET", "/api/v1/accounts/me", nil, bearer(tokA)).data()["balance_sen"].(float64) != 4_900_000 {
		t.Fatal("saldo A harus 49.000")
	}
	if s.do("GET", "/api/v1/accounts/me", nil, bearer(tokB)).data()["balance_sen"].(float64) != 5_000_000 {
		t.Fatal("saldo B harus 50.000")
	}

	// aturan bisnis → 422
	expect(t, s.do("POST", "/api/v1/transactions/transfer", map[string]any{"to_account_public_id": walletB, "amount_sen": 500}, withKey(tokA, uuid.NewString())), 422, "AMOUNT_OUT_OF_RANGE")
	expect(t, s.do("POST", "/api/v1/transactions/transfer", map[string]any{"to_account_public_id": walletA, "amount_sen": 1_000_000}, withKey(tokA, uuid.NewString())), 422, "SELF_TRANSFER")
	expect(t, s.do("POST", "/api/v1/transactions/transfer", map[string]any{"to_account_public_id": walletB, "amount_sen": 9_000_000}, withKey(tokA, uuid.NewString())), 422, "INSUFFICIENT_BALANCE")
	expect(t, s.do("POST", "/api/v1/transactions/transfer", map[string]any{"to_account_public_id": uuid.NewString(), "amount_sen": 1_000_000}, withKey(tokA, uuid.NewString())), 404, "ACCOUNT_NOT_FOUND")
	expect(t, s.do("POST", "/api/v1/transactions/transfer", map[string]any{"to_account_public_id": "bukan-uuid", "amount_sen": 1_000_000}, withKey(tokA, uuid.NewString())), 400, "VALIDATION_ERROR")

	// T-11 via HTTP: pihak ketiga tidak bisa melihat transaksi A→B
	tokC, _ := s.registerAndLogin("citra@test.local")
	expect(t, s.do("GET", "/api/v1/transactions/"+txnID, nil, bearer(tokA)), 200, "")
	expect(t, s.do("GET", "/api/v1/transactions/"+txnID, nil, bearer(tokB)), 200, "")
	expect(t, s.do("GET", "/api/v1/transactions/"+txnID, nil, bearer(tokC)), 404, "TRANSACTION_NOT_FOUND")
	expect(t, s.do("GET", "/api/v1/transactions/bukan-uuid", nil, bearer(tokA)), 400, "VALIDATION_ERROR")

	// mutasi rekening: cursor pagination
	p1 := s.do("GET", "/api/v1/accounts/me/entries?limit=1", nil, bearer(tokA))
	expect(t, p1, 200, "")
	next, _ := p1.body["next_cursor"].(string)
	if len(p1.body["data"].([]any)) != 1 || next == "" {
		t.Fatalf("halaman 1: %s", p1.raw)
	}
	p2 := s.do("GET", "/api/v1/accounts/me/entries?limit=1&cursor="+next, nil, bearer(tokA))
	expect(t, p2, 200, "")
	if p2.body["data"].([]any)[0].(map[string]any)["id"] == p1.body["data"].([]any)[0].(map[string]any)["id"] {
		t.Fatal("halaman 2 harus berbeda dari halaman 1")
	}
	if p2.body["next_cursor"] != nil {
		t.Fatalf("A hanya punya 2 entry; next_cursor harus null: %s", p2.raw)
	}
	// cursor rusak (bukan base64url) → 400. Catatan: "%%%" tidak bisa dipakai — parser URL membuang pasangan query yang rusak.
	expect(t, s.do("GET", "/api/v1/accounts/me/entries?cursor=bukan-base64!!", nil, bearer(tokA)), 400, "VALIDATION_ERROR")

	// reversal: USER 403, ADMIN 201, kedua 409
	expect(t, s.do("POST", "/api/v1/transactions/"+txnID+"/reverse", map[string]string{"description": "batal"}, withKey(tokA, uuid.NewString())), 403, "FORBIDDEN")
	s.makeAdmin("citra@test.local")
	tokAdmin, _ := s.registerAndLoginExisting("citra@test.local")
	expect(t, s.do("GET", "/api/v1/internal/ledger/trial-balance", nil, bearer(tokA)), 403, "FORBIDDEN")
	rev := s.do("POST", "/api/v1/transactions/"+txnID+"/reverse", map[string]string{"description": "batal"}, withKey(tokAdmin, uuid.NewString()))
	expect(t, rev, 201, "")
	if rev.data()["type"] != "REVERSAL" || rev.data()["reverses_transaction_id"] != txnID {
		t.Fatalf("reversal: %s", rev.raw)
	}
	expect(t, s.do("POST", "/api/v1/transactions/"+txnID+"/reverse", map[string]string{"description": "lagi"}, withKey(tokAdmin, uuid.NewString())), 409, "ALREADY_REVERSED")
	if s.do("GET", "/api/v1/accounts/me", nil, bearer(tokA)).data()["balance_sen"].(float64) != 10_000_000 {
		t.Fatal("saldo A harus kembali 100.000 setelah reversal")
	}

	tb := s.do("GET", "/api/v1/internal/ledger/trial-balance", nil, bearer(tokAdmin))
	expect(t, tb, 200, "")
	if tb.data()["balanced"] != true || tb.data()["difference_sen"].(float64) != 0 {
		t.Fatalf("trial balance: %s", tb.raw)
	}
	assertAllInvariants(t)
}

// registerAndLoginExisting login ulang untuk user yang sudah ada (mis. setelah jadi ADMIN).
func (s *apiServer) registerAndLoginExisting(email string) (string, string) {
	s.t.Helper()
	l := s.do("POST", "/api/v1/auth/login", map[string]string{"email": email, "password": "rahasia123"}, nil)
	if l.status != 200 {
		s.t.Fatalf("login %s: %d %s", email, l.status, l.raw)
	}
	return l.data()["access_token"].(string), ""
}

// balancesSeenIn mengembalikan set account_public_id yang balance_after_sen-nya TERLIHAT
// (bukan null) pada daftar entries suatu respons transaksi.
func balancesSeenIn(entries []any) map[string]bool {
	seen := map[string]bool{}
	for _, raw := range entries {
		e := raw.(map[string]any)
		if e["balance_after_sen"] != nil {
			seen[e["account_public_id"].(string)] = true
		}
	}
	return seen
}

// Audit keamanan (temuan B-1, High): sebelum perbaikan, respons transaksi apa pun
// membocorkan balance_after SEMUA pihak yang terlibat — termasuk saldo akun sistem
// (SYSTEM_CASH, SYSTEM_FEE_REVENUE) ke pengguna biasa, dan saldo pihak lawan transfer
// ke pengirim/penerima. Test ini membuktikan setiap pemanggil HANYA melihat saldo
// akunnya sendiri, baik di respons POST langsung maupun di GET belakangan; admin tetap
// melihat semua (dibutuhkan untuk reversal & audit).
func TestHTTP_B1_SaldoPihakLainTidakBocor(t *testing.T) {
	s := newAPIServer(t, serverOpts{})
	tokA, walletA := s.registerAndLogin("andi@test.local")
	tokB, walletB := s.registerAndLogin("budi@test.local")
	tokC, _ := s.registerAndLogin("citra@test.local")
	s.makeAdmin("citra@test.local")
	tokAdmin, _ := s.registerAndLoginExisting("citra@test.local")
	_ = tokC

	// 1. Respons LANGSUNG topup: saldo SYSTEM_CASH (akun sistem) tidak boleh terlihat,
	//    saldo dompet pemanggil sendiri WAJIB terlihat.
	top := s.do("POST", "/api/v1/transactions/topup", map[string]any{"amount_sen": 10_000_000}, withKey(tokA, uuid.NewString()))
	expect(t, top, 201, "")
	seenTop := balancesSeenIn(top.data()["entries"].([]any))
	if seenTop[walletA] != true {
		t.Fatalf("topup: saldo dompet sendiri (Andi) harus terlihat: %s", top.raw)
	}
	if len(seenTop) != 1 {
		t.Fatalf("topup: HANYA saldo dompet sendiri yang boleh terlihat, dapat %d akun: %s", len(seenTop), top.raw)
	}

	// 2. Respons LANGSUNG transfer A->B: Andi (pengirim) TIDAK boleh melihat saldo Budi
	//    maupun saldo akun fee — hanya saldo dompetnya sendiri.
	tr := s.do("POST", "/api/v1/transactions/transfer",
		map[string]any{"to_account_public_id": walletB, "amount_sen": 5_000_000, "description": "bayar kos"},
		withKey(tokA, uuid.NewString()))
	expect(t, tr, 201, "")
	txnID := tr.data()["transaction_id"].(string)
	seenTr := balancesSeenIn(tr.data()["entries"].([]any))
	if seenTr[walletA] != true {
		t.Fatalf("transfer (pengirim): saldo sendiri harus terlihat: %s", tr.raw)
	}
	if seenTr[walletB] {
		t.Fatalf("BUG B-1: pengirim (Andi) melihat saldo penerima (Budi) di respons transfer: %s", tr.raw)
	}
	if len(seenTr) != 1 {
		t.Fatalf("transfer (pengirim): HANYA 1 saldo (miliknya sendiri) yang boleh terlihat, dapat %d: %s", len(seenTr), tr.raw)
	}

	// 3. GET belakangan oleh Budi (penerima, pihak sah dalam transaksi ini): boleh
	//    melihat saldo SENDIRI, TIDAK boleh melihat saldo Andi maupun akun fee.
	getB := s.do("GET", "/api/v1/transactions/"+txnID, nil, bearer(tokB))
	expect(t, getB, 200, "")
	seenB := balancesSeenIn(getB.data()["entries"].([]any))
	if seenB[walletB] != true {
		t.Fatalf("GET oleh Budi: saldo sendiri harus terlihat: %s", getB.raw)
	}
	if seenB[walletA] {
		t.Fatalf("BUG B-1: Budi melihat saldo Andi lewat GET /transactions/{id}: %s", getB.raw)
	}
	if len(seenB) != 1 {
		t.Fatalf("GET oleh Budi: HANYA saldo sendiri yang boleh terlihat, dapat %d: %s", len(seenB), getB.raw)
	}

	// 4. GET oleh Andi (pengirim, lewat jalur GET terpisah dari respons POST asli):
	//    perilaku sama — hanya saldo sendiri.
	getA := s.do("GET", "/api/v1/transactions/"+txnID, nil, bearer(tokA))
	expect(t, getA, 200, "")
	seenA := balancesSeenIn(getA.data()["entries"].([]any))
	if seenA[walletA] != true || seenA[walletB] {
		t.Fatalf("GET oleh Andi: mau hanya saldo sendiri terlihat, dapat set=%v: %s", seenA, getA.raw)
	}

	// 5. ADMIN tetap melihat SEMUA saldo (perlu untuk reversal & audit) — pastikan
	//    perbaikan ini tidak diam-diam menutup akses admin yang sah.
	getAdmin := s.do("GET", "/api/v1/transactions/"+txnID, nil, bearer(tokAdmin))
	expect(t, getAdmin, 200, "")
	seenAdmin := balancesSeenIn(getAdmin.data()["entries"].([]any))
	if len(seenAdmin) != len(getAdmin.data()["entries"].([]any)) {
		t.Fatalf("ADMIN harus melihat SEMUA saldo entry, terlihat %d dari %d: %s", len(seenAdmin), len(getAdmin.data()["entries"].([]any)), getAdmin.raw)
	}
	if !seenAdmin[walletA] || !seenAdmin[walletB] {
		t.Fatalf("ADMIN harus melihat saldo Andi DAN Budi: %s", getAdmin.raw)
	}

	// amount_sen/fee_sen tetap benar untuk SEMUA pihak walau balance_after disembunyikan —
	// redaksi tidak boleh merusak informasi transaksi yang memang boleh diketahui.
	for _, r := range []resp{tr, getA, getB, getAdmin} {
		if r.data()["amount_sen"].(float64) != 5_000_000 || r.data()["fee_sen"].(float64) != 100_000 {
			t.Fatalf("amount_sen/fee_sen harus tetap benar meski balance disembunyikan: %s", r.raw)
		}
	}
	assertAllInvariants(t)
}

func TestHTTP_TransferRateLimitDanBodyBesar(t *testing.T) {
	s := newAPIServer(t, serverOpts{transferLimit: 2})
	tokA, _ := s.registerAndLogin("andi@test.local")
	_, walletB := s.registerAndLogin("budi@test.local")
	s.do("POST", "/api/v1/transactions/topup", map[string]any{"amount_sen": 100_000_000}, withKey(tokA, uuid.NewString()))

	body := map[string]any{"to_account_public_id": walletB, "amount_sen": 1_000_000}
	expect(t, s.do("POST", "/api/v1/transactions/transfer", body, withKey(tokA, uuid.NewString())), 201, "")
	expect(t, s.do("POST", "/api/v1/transactions/transfer", body, withKey(tokA, uuid.NewString())), 201, "")
	expect(t, s.do("POST", "/api/v1/transactions/transfer", body, withKey(tokA, uuid.NewString())), 429, "RATE_LIMITED")

	// body 2 MB → 413, server tetap hidup
	huge := []byte(`{"amount_sen":1,"description":"` + strings.Repeat("x", 2<<20) + `"}`)
	expect(t, s.do("POST", "/api/v1/transactions/topup", huge, withKey(tokA, uuid.NewString())), 413, "PAYLOAD_TOO_LARGE")
	expect(t, s.do("GET", "/healthz", nil, nil), 200, "")
	assertAllInvariants(t)
}
