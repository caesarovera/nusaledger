//go:build integration

package integration

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/caesarovera/nusaledger/internal/domain"
	"github.com/caesarovera/nusaledger/internal/repository/postgres"
)

func newClaim(userID int64) domain.IdempotencyClaim {
	return domain.IdempotencyClaim{UserID: userID, Key: uuid.NewString(), Endpoint: "test", RequestHash: "h"}
}

// Jalur bahagia docs/01 §2.4 B: Andi kirim Rp 50.000 ke Budi, fee Rp 1.000.
func TestLedgerRepo_Post_Transfer(t *testing.T) {
	resetDB(t)
	ctx := testCtx(t)
	repo := postgres.NewLedgerRepo(testPool)

	andi := seedUser(t, "andi@test.local", domain.RoleUser)
	budi := seedUser(t, "budi@test.local", domain.RoleUser)
	wAndi := seedWallet(t, andi, 100_000*domain.Rupiah)
	wBudi := seedWallet(t, budi, 0)

	txn, err := domain.NewTransfer(wAndi, wBudi, sysFeeID, 50_000*domain.Rupiah, 1_000*domain.Rupiah, andi, "bayar kos")
	if err != nil {
		t.Fatal(err)
	}
	res, err := repo.Post(ctx, txn, newClaim(andi))
	if err != nil {
		t.Fatalf("Post: %v", err)
	}

	if res.Transaction.ID == uuid.Nil || res.Transaction.Status != domain.TxnPosted {
		t.Fatalf("transaksi harus punya id dan status POSTED: %+v", res.Transaction)
	}
	if len(res.Entries) != 3 {
		t.Fatalf("mau 3 entry, dapat %d", len(res.Entries))
	}
	wantAfter := map[int64]domain.Money{wAndi: 49_000 * domain.Rupiah, wBudi: 50_000 * domain.Rupiah, sysFeeID: 1_000 * domain.Rupiah}
	for id, want := range wantAfter {
		got, ok := res.BalanceAfterFor(id)
		if !ok || got != want {
			t.Errorf("balance_after akun %d: mau %s, dapat %s (ada=%v)", id, want, got, ok)
		}
		if db := balanceOf(t, id); db != want {
			t.Errorf("saldo db akun %d: mau %s, dapat %s", id, want, db)
		}
	}
	if got := countRows(t, "outbox_events"); got != 1 {
		t.Errorf("outbox: mau 1 event, dapat %d", got)
	}
	assertAllInvariants(t)

	// GetTransaction mengembalikan hal yang sama
	got, err := repo.GetTransaction(ctx, res.Transaction.ID)
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}
	if got.Transaction.Type != domain.TxnTransfer || len(got.Entries) != 3 || got.Transaction.Description != "bayar kos" {
		t.Fatalf("GetTransaction tidak cocok: %+v", got.Transaction)
	}
	if _, err := repo.GetTransaction(ctx, uuid.New()); !errors.Is(err, domain.ErrTransactionNotFound) {
		t.Fatalf("id acak: mau ErrTransactionNotFound, dapat %v", err)
	}
}

func TestLedgerRepo_Post_SaldoKurang(t *testing.T) {
	resetDB(t)
	ctx := testCtx(t)
	repo := postgres.NewLedgerRepo(testPool)
	andi := seedUser(t, "andi@test.local", domain.RoleUser)
	budi := seedUser(t, "budi@test.local", domain.RoleUser)
	wAndi := seedWallet(t, andi, 50_000*domain.Rupiah) // pas amount, kurang untuk fee
	wBudi := seedWallet(t, budi, 0)

	txn, _ := domain.NewTransfer(wAndi, wBudi, sysFeeID, 50_000*domain.Rupiah, 1_000*domain.Rupiah, andi, "")
	claim := newClaim(andi)
	_, err := repo.Post(ctx, txn, claim)
	if !errors.Is(err, domain.ErrInsufficientBalance) {
		t.Fatalf("mau ErrInsufficientBalance, dapat %v", err)
	}
	// seluruh transaksi rollback: tidak ada header, entry, maupun klaim idempotency yang tersisa
	if countRows(t, "transactions") != 1 || countRows(t, "entries") != 2 { // hanya seed topup
		t.Fatalf("rollback tidak bersih: txn=%d entries=%d", countRows(t, "transactions"), countRows(t, "entries"))
	}
	if _, err := postgres.NewIdempotencyRepo(testPool).Find(ctx, andi, claim.Key); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("klaim idempotency harus ikut rollback, dapat %v", err)
	}
	if balanceOf(t, wAndi) != 50_000*domain.Rupiah {
		t.Fatal("saldo Andi tidak boleh berubah")
	}
	assertAllInvariants(t)
}

func TestLedgerRepo_Post_IdempotencyTersimpanDanInFlight(t *testing.T) {
	resetDB(t)
	ctx := testCtx(t)
	repo := postgres.NewLedgerRepo(testPool)
	idem := postgres.NewIdempotencyRepo(testPool)
	andi := seedUser(t, "andi@test.local", domain.RoleUser)
	wAndi := seedWallet(t, andi, 100_000*domain.Rupiah)

	claim := newClaim(andi)
	txn := domain.NewTopup(sysCashID, wAndi, 10_000*domain.Rupiah, andi, "isi")
	res, err := repo.Post(ctx, txn, claim)
	if err != nil {
		t.Fatal(err)
	}

	rec, err := idem.Find(ctx, andi, claim.Key)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if rec.InFlight() || rec.StatusCode != 201 || rec.Result == nil || rec.Result.Transaction.ID != res.Transaction.ID {
		t.Fatalf("rekaman idempotency salah: %+v", rec)
	}
	if after, _ := rec.Result.BalanceAfterFor(wAndi); after != 110_000*domain.Rupiah {
		t.Fatalf("hasil tersimpan: balance_after mau %s, dapat %s", 110_000*domain.Rupiah, after)
	}

	// key yang sama dipakai lagi → klaim gagal → in-flight, TANPA transaksi baru
	_, err = repo.Post(ctx, domain.NewTopup(sysCashID, wAndi, 10_000*domain.Rupiah, andi, "isi lagi"), claim)
	if !errors.Is(err, domain.ErrIdempotencyInFlight) {
		t.Fatalf("mau ErrIdempotencyInFlight, dapat %v", err)
	}
	if countRows(t, "transactions") != 2 { // seed + 1 topup
		t.Fatalf("transaksi kedua tidak boleh tercipta, ada %d", countRows(t, "transactions"))
	}
	assertAllInvariants(t)
}

func TestLedgerRepo_Post_AkunTidakAktifDanTidakAda(t *testing.T) {
	resetDB(t)
	ctx := testCtx(t)
	repo := postgres.NewLedgerRepo(testPool)
	andi := seedUser(t, "andi@test.local", domain.RoleUser)
	wAndi := seedWallet(t, andi, 100_000*domain.Rupiah)

	if _, err := testPool.Exec(ctx, `UPDATE accounts SET status = 'FROZEN' WHERE id = $1`, wAndi); err != nil {
		t.Fatal(err)
	}
	_, err := repo.Post(ctx, domain.NewTopup(sysCashID, wAndi, 1_000*domain.Rupiah, andi, ""), newClaim(andi))
	if !errors.Is(err, domain.ErrAccountNotActive) {
		t.Fatalf("akun FROZEN: mau ErrAccountNotActive, dapat %v", err)
	}

	_, err = repo.Post(ctx, domain.NewTopup(sysCashID, 999_999, 1_000*domain.Rupiah, andi, ""), newClaim(andi))
	if !errors.Is(err, domain.ErrAccountNotFound) {
		t.Fatalf("akun tidak ada: mau ErrAccountNotFound, dapat %v", err)
	}
	assertAllInvariants(t)
}

// T-10 & T-10b: reversal membuat transaksi baru, asli utuh; kedua kali → 409; penerima bangkrut → 422.
func TestLedgerRepo_Post_Reversal(t *testing.T) {
	resetDB(t)
	ctx := testCtx(t)
	repo := postgres.NewLedgerRepo(testPool)
	andi := seedUser(t, "andi@test.local", domain.RoleUser)
	budi := seedUser(t, "budi@test.local", domain.RoleUser)
	admin := seedUser(t, "admin@test.local", domain.RoleAdmin)
	wAndi := seedWallet(t, andi, 100_000*domain.Rupiah)
	wBudi := seedWallet(t, budi, 0)

	orig, _ := domain.NewTransfer(wAndi, wBudi, sysFeeID, 50_000*domain.Rupiah, 1_000*domain.Rupiah, andi, "salah kirim")
	res, err := repo.Post(ctx, orig, newClaim(andi))
	if err != nil {
		t.Fatal(err)
	}

	rev, err := domain.NewReversal(res.Transaction, admin, "pembatalan")
	if err != nil {
		t.Fatal(err)
	}
	revRes, err := repo.Post(ctx, rev, newClaim(admin))
	if err != nil {
		t.Fatalf("reversal: %v", err)
	}
	if balanceOf(t, wAndi) != 100_000*domain.Rupiah || balanceOf(t, wBudi) != 0 || balanceOf(t, sysFeeID) != 0 {
		t.Fatalf("saldo setelah reversal harus kembali semula: andi=%s budi=%s fee=%s", balanceOf(t, wAndi), balanceOf(t, wBudi), balanceOf(t, sysFeeID))
	}
	got, _ := repo.GetTransaction(ctx, res.Transaction.ID)
	if got.Transaction.Status != domain.TxnReversed || len(got.Entries) != 3 {
		t.Fatalf("transaksi asli harus REVERSED dengan entry utuh: %+v", got.Transaction)
	}
	if revRes.Transaction.ReversesID == nil || *revRes.Transaction.ReversesID != res.Transaction.ID {
		t.Fatal("reversal harus menunjuk transaksi asal")
	}
	assertAllInvariants(t)

	// T-10: reversal kedua ditolak (status sudah REVERSED)
	rev2, _ := domain.NewReversal(res.Transaction, admin, "lagi")
	if _, err := repo.Post(ctx, rev2, newClaim(admin)); !errors.Is(err, domain.ErrAlreadyReversed) {
		t.Fatalf("reversal kedua: mau ErrAlreadyReversed, dapat %v", err)
	}

	// T-10b: Budi menerima 50.000 lalu membelanjakannya; reversal atas transfer itu → saldo kurang
	orig2, _ := domain.NewTransfer(wAndi, wBudi, sysFeeID, 50_000*domain.Rupiah, 1_000*domain.Rupiah, andi, "")
	res2, err := repo.Post(ctx, orig2, newClaim(andi))
	if err != nil {
		t.Fatal(err)
	}
	spend := domain.NewWithdraw(wBudi, sysCashID, 50_000*domain.Rupiah, budi, "tarik")
	if _, err := repo.Post(ctx, spend, newClaim(budi)); err != nil {
		t.Fatal(err)
	}
	rev3, _ := domain.NewReversal(res2.Transaction, admin, "")
	if _, err := repo.Post(ctx, rev3, newClaim(admin)); !errors.Is(err, domain.ErrInsufficientBalance) {
		t.Fatalf("T-10b: mau ErrInsufficientBalance, dapat %v", err)
	}
	got2, _ := repo.GetTransaction(ctx, res2.Transaction.ID)
	if got2.Transaction.Status != domain.TxnPosted {
		t.Fatal("reversal yang gagal tidak boleh mengubah status transaksi asal")
	}
	assertAllInvariants(t)
}

func TestLedgerRepo_TrialBalanceDanDrift(t *testing.T) {
	resetDB(t)
	ctx := testCtx(t)
	repo := postgres.NewLedgerRepo(testPool)
	andi := seedUser(t, "andi@test.local", domain.RoleUser)
	seedWallet(t, andi, 75_000*domain.Rupiah)

	tb, err := repo.TrialBalance(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !tb.Balanced() || tb.TotalDebit != 75_000*domain.Rupiah || tb.EntryCount != 2 {
		t.Fatalf("trial balance salah: %+v", tb)
	}
	drift, err := repo.BalanceDrift(ctx)
	if err != nil || drift != 0 {
		t.Fatalf("drift mau 0, dapat %d (%v)", drift, err)
	}
}
