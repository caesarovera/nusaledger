//go:build integration

package integration

import (
	"errors"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/caesarovera/nusaledger/internal/domain"
	"github.com/caesarovera/nusaledger/internal/repository/postgres"
)

const codeDeadlock = "40P01"

func isDeadlock(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codeDeadlock
}

// T-04 — test paling penting di seluruh proyek.
// 100 goroutine transfer Rp 50.000 (+fee Rp 1.000) dari saldo Rp 1.000.000, key BERBEDA per goroutine.
// 1.000.000 / 51.000 = 19,6 → TEPAT 19 sukses, sisa Rp 31.000. Uang tidak boleh tercipta atau hilang.
func TestConcurrency_T04_TransferParalelDariSatuAkun(t *testing.T) {
	resetDB(t)
	ctx := testCtx(t)
	repo := postgres.NewLedgerRepo(testPool)
	andi := seedUser(t, "andi@test.local", domain.RoleUser)
	budi := seedUser(t, "budi@test.local", domain.RoleUser)
	from := seedWallet(t, andi, 1_000_000*domain.Rupiah)
	to := seedWallet(t, budi, 0)

	const (
		workers  = 100
		amount   = 50_000 * domain.Rupiah
		fee      = 1_000 * domain.Rupiah
		expected = 19
	)

	var wg sync.WaitGroup
	var okCount, insufficient, conflict atomic.Int64
	errCh := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			txn, err := domain.NewTransfer(from, to, sysFeeID, amount, fee, andi, "paralel")
			if err != nil {
				errCh <- err
				return
			}
			_, err = repo.Post(ctx, txn, newClaim(andi)) // key berbeda: ini BUKAN retry
			switch {
			case err == nil:
				okCount.Add(1)
			case errors.Is(err, domain.ErrInsufficientBalance):
				insufficient.Add(1)
			case errors.Is(err, domain.ErrConcurrentModification):
				conflict.Add(1)
			default:
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("error tak terduga: %v", err)
	}

	t.Logf("sukses=%d saldo_kurang=%d konflik=%d saldo_pengirim=%s", okCount.Load(), insufficient.Load(), conflict.Load(), balanceOf(t, from))

	if conflict.Load() != 0 {
		t.Fatalf("mau 0 konflik optimistic lock (FOR UPDATE seharusnya mencegahnya), dapat %d", conflict.Load())
	}

	// Invariant diperiksa PERTAMA: berapa yang sukses boleh berbeda antar implementasi,
	// tetapi uang tercipta/hilang tidak boleh terjadi apa pun yang terjadi.
	assertAllInvariants(t)

	if got := okCount.Load(); got != expected {
		t.Fatalf("transfer sukses: mau %d, dapat %d", expected, got)
	}
	if got := balanceOf(t, from); got != 31_000*domain.Rupiah {
		t.Fatalf("sisa saldo pengirim: mau %s, dapat %s", 31_000*domain.Rupiah, got)
	}
	if got := balanceOf(t, to); got != expected*amount {
		t.Fatalf("saldo penerima: mau %s, dapat %s", expected*amount, got)
	}
	if got := balanceOf(t, sysFeeID); got != expected*fee {
		t.Fatalf("pendapatan fee: mau %s, dapat %s", expected*fee, got)
	}
}

// T-05 — 10 goroutine dengan Idempotency-Key yang SAMA bersamaan → tepat 1 transaksi.
func TestConcurrency_T05_IdempotencyParalel(t *testing.T) {
	resetDB(t)
	ctx := testCtx(t)
	repo := postgres.NewLedgerRepo(testPool)
	idem := postgres.NewIdempotencyRepo(testPool)
	andi := seedUser(t, "andi@test.local", domain.RoleUser)
	w := seedWallet(t, andi, 0)

	claim := newClaim(andi) // SATU key untuk semua goroutine
	const workers = 10

	var wg sync.WaitGroup
	var okCount, inFlight atomic.Int64
	errCh := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.Post(ctx, domain.NewTopup(sysCashID, w, 10_000*domain.Rupiah, andi, "topup"), claim)
			switch {
			case err == nil:
				okCount.Add(1)
			case errors.Is(err, domain.ErrIdempotencyInFlight):
				inFlight.Add(1)
			default:
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("error tak terduga: %v", err)
	}

	if okCount.Load() != 1 || inFlight.Load() != workers-1 {
		t.Fatalf("mau 1 sukses + %d in-flight, dapat %d + %d", workers-1, okCount.Load(), inFlight.Load())
	}
	if got := balanceOf(t, w); got != 10_000*domain.Rupiah {
		t.Fatalf("saldo harus naik SEKALI: mau %s, dapat %s", 10_000*domain.Rupiah, got)
	}
	if n := countRows(t, "transactions"); n != 1 {
		t.Fatalf("mau 1 transaksi, dapat %d", n)
	}
	// setelah selesai, semua peserta yang kalah bisa mengambil hasil yang sama
	rec, err := idem.Find(ctx, andi, claim.Key)
	if err != nil || rec.InFlight() || rec.StatusCode != 201 {
		t.Fatalf("hasil idempotency harus tersimpan: %+v, %v", rec, err)
	}
	assertAllInvariants(t)
}

// T-07 — 50× A→B dan 50× B→A bersamaan → tidak ada deadlock (SQLSTATE 40P01), semua sukses.
func TestConcurrency_T07_TransferSilangTanpaDeadlock(t *testing.T) {
	resetDB(t)
	ctx := testCtx(t)
	repo := postgres.NewLedgerRepo(testPool)
	andi := seedUser(t, "andi@test.local", domain.RoleUser)
	budi := seedUser(t, "budi@test.local", domain.RoleUser)
	wA := seedWallet(t, andi, 10_000_000*domain.Rupiah)
	wB := seedWallet(t, budi, 10_000_000*domain.Rupiah)

	const rounds = 50
	var wg sync.WaitGroup
	var okCount, deadlocks atomic.Int64
	errCh := make(chan error, rounds*2)

	transfer := func(from, to, by int64) {
		defer wg.Done()
		txn, _ := domain.NewTransfer(from, to, sysFeeID, 10_000*domain.Rupiah, 1_000*domain.Rupiah, by, "silang")
		_, err := repo.Post(ctx, txn, newClaim(by))
		switch {
		case err == nil:
			okCount.Add(1)
		case isDeadlock(err):
			deadlocks.Add(1)
		default:
			errCh <- err
		}
	}
	for i := 0; i < rounds; i++ {
		wg.Add(2)
		go transfer(wA, wB, andi)
		go transfer(wB, wA, budi)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("error tak terduga: %v", err)
	}

	if deadlocks.Load() != 0 {
		t.Fatalf("terjadi %d deadlock — urutan penguncian rusak", deadlocks.Load())
	}
	if okCount.Load() != rounds*2 {
		t.Fatalf("mau %d sukses, dapat %d", rounds*2, okCount.Load())
	}
	// masing-masing kirim 50×11.000 dan terima 50×10.000 → bersih −50.000
	want := (10_000_000 - 50_000) * domain.Rupiah
	if balanceOf(t, wA) != want || balanceOf(t, wB) != want {
		t.Fatalf("saldo akhir: mau %s/%s, dapat %s/%s", want, want, balanceOf(t, wA), balanceOf(t, wB))
	}
	if balanceOf(t, sysFeeID) != 100*1_000*domain.Rupiah {
		t.Fatalf("fee: mau %s, dapat %s", 100*1_000*domain.Rupiah, balanceOf(t, sysFeeID))
	}
	assertAllInvariants(t)
}

// T-08 & T-09 — 1.000 transaksi acak (topup/transfer/withdraw) dari 8 worker:
// trial balance tetap 0 dan accounts.balance = Σ entries untuk semua akun.
func TestConcurrency_T08_T09_SeribuTransaksiAcak(t *testing.T) {
	resetDB(t)
	ctx := testCtx(t)
	repo := postgres.NewLedgerRepo(testPool)

	const (
		users   = 5
		total   = 1000
		workers = 8
	)
	userIDs := make([]int64, users)
	wallets := make([]int64, users)
	for i := range users {
		userIDs[i] = seedUser(t, uuid.NewString()+"@test.local", domain.RoleUser)
		wallets[i] = seedWallet(t, userIDs[i], 500_000*domain.Rupiah)
	}

	jobs := make(chan int, total)
	for i := 0; i < total; i++ {
		jobs <- i
	}
	close(jobs)

	var wg sync.WaitGroup
	var okCount, rejected atomic.Int64
	errCh := make(chan error, total)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(seed uint64) {
			defer wg.Done()
			rng := rand.New(rand.NewPCG(seed, seed*7+1)) //nolint:gosec // acak untuk test, bukan kripto
			for range jobs {
				i := rng.IntN(users)
				j := (i + 1 + rng.IntN(users-1)) % users
				amt := domain.Money(rng.Int64N(200_000)+1_000) * domain.Rupiah
				var txn *domain.Transaction
				switch rng.IntN(3) {
				case 0:
					txn = domain.NewTopup(sysCashID, wallets[i], amt, userIDs[i], "acak")
				case 1:
					txn, _ = domain.NewTransfer(wallets[i], wallets[j], sysFeeID, amt, 1_000*domain.Rupiah, userIDs[i], "acak")
				default:
					txn = domain.NewWithdraw(wallets[i], sysCashID, amt, userIDs[i], "acak")
				}
				_, err := repo.Post(ctx, txn, newClaim(userIDs[i]))
				switch {
				case err == nil:
					okCount.Add(1)
				case errors.Is(err, domain.ErrInsufficientBalance):
					rejected.Add(1)
				default:
					errCh <- err
				}
			}
		}(uint64(w + 1))
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("error tak terduga: %v", err)
	}

	t.Logf("sukses=%d ditolak_saldo=%d", okCount.Load(), rejected.Load())
	if okCount.Load()+rejected.Load() != total {
		t.Fatalf("semua %d transaksi harus sukses atau ditolak saldo", total)
	}
	if okCount.Load() == 0 {
		t.Fatal("tidak ada transaksi yang sukses; test tidak bermakna")
	}
	tb, err := repo.TrialBalance(ctx)
	if err != nil || !tb.Balanced() {
		t.Fatalf("trial balance: %+v, %v", tb, err)
	}
	drift, err := repo.BalanceDrift(ctx)
	if err != nil || drift != 0 {
		t.Fatalf("drift: mau 0, dapat %d (%v)", drift, err)
	}
	assertAllInvariants(t)
}
