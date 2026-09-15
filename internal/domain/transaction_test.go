package domain_test

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/caesarovera/nusaledger/internal/domain"
)

const (
	acctAndi = int64(10)
	acctBudi = int64(11)
	acctCash = int64(1)
	acctFee  = int64(2)
)

// T-01: Validate menegakkan BR-02, BR-03, K-02 sebelum database disentuh.
func TestTransaction_Validate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		entries []domain.Entry
		desc    string
		wantErr error
	}{
		{"transfer seimbang dengan fee", []domain.Entry{
			{AccountID: acctAndi, Direction: domain.DirectionDebit, Amount: 51_000_00},
			{AccountID: acctBudi, Direction: domain.DirectionCredit, Amount: 50_000_00},
			{AccountID: acctFee, Direction: domain.DirectionCredit, Amount: 1_000_00},
		}, "", nil},
		{"tidak seimbang", []domain.Entry{
			{AccountID: acctAndi, Direction: domain.DirectionDebit, Amount: 50_000_00},
			{AccountID: acctBudi, Direction: domain.DirectionCredit, Amount: 40_000_00},
		}, "", domain.ErrUnbalanced},
		{"amount nol ditolak", []domain.Entry{
			{AccountID: acctAndi, Direction: domain.DirectionDebit, Amount: 0},
			{AccountID: acctBudi, Direction: domain.DirectionCredit, Amount: 0},
		}, "", domain.ErrAmountNotPositive},
		{"amount negatif ditolak", []domain.Entry{
			{AccountID: acctAndi, Direction: domain.DirectionDebit, Amount: -100},
			{AccountID: acctBudi, Direction: domain.DirectionCredit, Amount: -100},
		}, "", domain.ErrAmountNotPositive},
		{"satu entry ditolak", []domain.Entry{
			{AccountID: acctAndi, Direction: domain.DirectionDebit, Amount: 1000},
		}, "", domain.ErrTooFewEntries},
		{"tanpa entry ditolak", nil, "", domain.ErrTooFewEntries},
		{"akun sama dua kali ditolak (K-02)", []domain.Entry{
			{AccountID: acctAndi, Direction: domain.DirectionDebit, Amount: 1000},
			{AccountID: acctAndi, Direction: domain.DirectionCredit, Amount: 1000},
		}, "", domain.ErrDuplicateAccount},
		{"arah tidak dikenal ditolak", []domain.Entry{
			{AccountID: acctAndi, Direction: "KIRI", Amount: 1000},
			{AccountID: acctBudi, Direction: domain.DirectionCredit, Amount: 1000},
		}, "", domain.ErrUnknownDirection},
		{"overflow saat menjumlahkan sisi debit", []domain.Entry{
			{AccountID: acctAndi, Direction: domain.DirectionDebit, Amount: domain.Money(math.MaxInt64)},
			{AccountID: acctCash, Direction: domain.DirectionDebit, Amount: 1},
			{AccountID: acctBudi, Direction: domain.DirectionCredit, Amount: 1},
		}, "", domain.ErrAmountOverflow},
		{"deskripsi terlalu panjang", []domain.Entry{
			{AccountID: acctAndi, Direction: domain.DirectionDebit, Amount: 1000},
			{AccountID: acctBudi, Direction: domain.DirectionCredit, Amount: 1000},
		}, strings.Repeat("x", 256), domain.ErrDescriptionTooLong},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			txn := &domain.Transaction{Entries: tt.entries, Description: tt.desc}
			if err := txn.Validate(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("mau %v, dapat %v", tt.wantErr, err)
			}
		})
	}
}

// G-2 #4: Validate menegakkan konsistensi Type <-> ReversesID sebelum menyentuh database.
func TestTransaction_Validate_KonsistensiReversalLink(t *testing.T) {
	t.Parallel()
	seimbang := []domain.Entry{
		{AccountID: acctAndi, Direction: domain.DirectionDebit, Amount: 1000},
		{AccountID: acctBudi, Direction: domain.DirectionCredit, Amount: 1000},
	}
	origID := uuid.New()
	tests := []struct {
		name       string
		txnType    domain.TxnType
		reversesID *uuid.UUID
		wantErr    error
	}{
		{"REVERSAL tanpa ReversesID ditolak", domain.TxnReversal, nil, domain.ErrInvalidReversalLink},
		{"TRANSFER dengan ReversesID terisi ditolak", domain.TxnTransfer, &origID, domain.ErrInvalidReversalLink},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			txn := &domain.Transaction{Type: tt.txnType, Entries: seimbang, ReversesID: tt.reversesID}
			if err := txn.Validate(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("mau %v, dapat %v", tt.wantErr, err)
			}
		})
	}
}

// Empat kombinasi arah × normal balance. Salah tanda di sini = seluruh ledger salah.
func TestBalanceDelta(t *testing.T) {
	t.Parallel()
	const amt = domain.Money(1000)
	tests := []struct {
		name   string
		dir    domain.Direction
		normal domain.Direction
		want   domain.Money
	}{
		{"kredit dompet (kewajiban) → bertambah", domain.DirectionCredit, domain.DirectionCredit, +amt},
		{"debit dompet (kewajiban) → berkurang", domain.DirectionDebit, domain.DirectionCredit, -amt},
		{"debit kas (aset) → bertambah", domain.DirectionDebit, domain.DirectionDebit, +amt},
		{"kredit kas (aset) → berkurang", domain.DirectionCredit, domain.DirectionDebit, -amt},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := domain.BalanceDelta(domain.Entry{Direction: tt.dir, Amount: amt}, tt.normal)
			if got != tt.want {
				t.Fatalf("mau %d, dapat %d", tt.want, got)
			}
		})
	}
}

func TestAccountType_NormalBalance(t *testing.T) {
	t.Parallel()
	if domain.AccountSystemCash.NormalBalance() != domain.DirectionDebit {
		t.Error("SYSTEM_CASH (aset) harus bernormal DEBIT")
	}
	for _, at := range []domain.AccountType{domain.AccountUserWallet, domain.AccountSystemFeeRevenue, domain.AccountSystemSuspense} {
		if at.NormalBalance() != domain.DirectionCredit {
			t.Errorf("%s harus bernormal CREDIT", at)
		}
	}
}

func TestTransaction_AccountIDs_TerurutTanpaDuplikat(t *testing.T) {
	t.Parallel()
	txn := &domain.Transaction{Entries: []domain.Entry{
		{AccountID: 42}, {AccountID: 7}, {AccountID: 19}, {AccountID: 7},
	}}
	got := txn.AccountIDs()
	want := []int64{7, 19, 42}
	if len(got) != len(want) {
		t.Fatalf("mau %v, dapat %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("mau %v, dapat %v", want, got)
		}
	}
}

// Skenario docs/01 §2.4 B: Andi kirim Rp 50.000 ke Budi, fee Rp 1.000.
func TestNewTransfer(t *testing.T) {
	t.Parallel()
	txn, err := domain.NewTransfer(acctAndi, acctBudi, acctFee, 50_000*domain.Rupiah, 1_000*domain.Rupiah, 1, "bayar kos")
	if err != nil {
		t.Fatalf("NewTransfer: %v", err)
	}
	if err := txn.Validate(); err != nil {
		t.Fatalf("transfer hasil builder harus valid, dapat %v", err)
	}
	if txn.Type != domain.TxnTransfer || len(txn.Entries) != 3 {
		t.Fatalf("mau TRANSFER dengan 3 entry, dapat %s dengan %d", txn.Type, len(txn.Entries))
	}

	wallet := domain.AccountUserWallet.NormalBalance()
	revenue := domain.AccountSystemFeeRevenue.NormalBalance()
	wantDelta := map[int64]domain.Money{
		acctAndi: -51_000 * domain.Rupiah,
		acctBudi: +50_000 * domain.Rupiah,
		acctFee:  +1_000 * domain.Rupiah,
	}
	normals := map[int64]domain.Direction{acctAndi: wallet, acctBudi: wallet, acctFee: revenue}
	for id, want := range wantDelta {
		e, ok := txn.EntryFor(id)
		if !ok {
			t.Fatalf("entry akun %d tidak ada", id)
		}
		if got := domain.BalanceDelta(e, normals[id]); got != want {
			t.Errorf("delta akun %d: mau %s, dapat %s", id, want, got)
		}
	}

	t.Run("fee nol → hanya dua entry", func(t *testing.T) {
		t.Parallel()
		txn, err := domain.NewTransfer(acctAndi, acctBudi, acctFee, 1000, 0, 1, "")
		if err != nil || len(txn.Entries) != 2 || txn.Validate() != nil {
			t.Fatalf("mau 2 entry valid, dapat %d, err=%v", len(txn.Entries), err)
		}
	})
	t.Run("transfer ke diri sendiri ditolak (BR-09)", func(t *testing.T) {
		t.Parallel()
		if _, err := domain.NewTransfer(acctAndi, acctAndi, acctFee, 1000, 0, 1, ""); !errors.Is(err, domain.ErrSelfTransfer) {
			t.Fatalf("mau ErrSelfTransfer, dapat %v", err)
		}
	})
	t.Run("fee negatif ditolak", func(t *testing.T) {
		t.Parallel()
		if _, err := domain.NewTransfer(acctAndi, acctBudi, acctFee, 1000, -1, 1, ""); !errors.Is(err, domain.ErrAmountNotPositive) {
			t.Fatalf("mau ErrAmountNotPositive, dapat %v", err)
		}
	})
	t.Run("amount + fee overflow ditolak", func(t *testing.T) {
		t.Parallel()
		if _, err := domain.NewTransfer(acctAndi, acctBudi, acctFee, domain.Money(math.MaxInt64), 1, 1, ""); !errors.Is(err, domain.ErrAmountOverflow) {
			t.Fatalf("mau ErrAmountOverflow, dapat %v", err)
		}
	})
}

// Skenario docs/01 §2.4 A dan C.
func TestNewTopupDanWithdraw(t *testing.T) {
	t.Parallel()
	const amt = 100_000 * domain.Rupiah
	cashNormal := domain.AccountSystemCash.NormalBalance()
	walletNormal := domain.AccountUserWallet.NormalBalance()

	topup := domain.NewTopup(acctCash, acctAndi, amt, 1, "isi saldo")
	if err := topup.Validate(); err != nil {
		t.Fatalf("topup harus valid: %v", err)
	}
	if e, _ := topup.EntryFor(acctCash); domain.BalanceDelta(e, cashNormal) != +amt {
		t.Error("topup: kas perusahaan harus bertambah")
	}
	if e, _ := topup.EntryFor(acctAndi); domain.BalanceDelta(e, walletNormal) != +amt {
		t.Error("topup: dompet pengguna harus bertambah")
	}

	wd := domain.NewWithdraw(acctAndi, acctCash, amt, 1, "tarik dana")
	if err := wd.Validate(); err != nil {
		t.Fatalf("withdraw harus valid: %v", err)
	}
	if e, _ := wd.EntryFor(acctCash); domain.BalanceDelta(e, cashNormal) != -amt {
		t.Error("withdraw: kas perusahaan harus berkurang")
	}
	if e, _ := wd.EntryFor(acctAndi); domain.BalanceDelta(e, walletNormal) != -amt {
		t.Error("withdraw: dompet pengguna harus berkurang")
	}
}

// Skenario docs/01 §2.4 D: reversal = transaksi baru dengan arah dibalik (BR-11).
func TestNewReversal(t *testing.T) {
	t.Parallel()
	orig, _ := domain.NewTransfer(acctAndi, acctBudi, acctFee, 50_000*domain.Rupiah, 1_000*domain.Rupiah, 1, "")
	orig.ID = uuid.New()

	rev, err := domain.NewReversal(orig, 99, "salah kirim")
	if err != nil {
		t.Fatalf("NewReversal: %v", err)
	}
	if err := rev.Validate(); err != nil {
		t.Fatalf("reversal harus valid: %v", err)
	}
	if rev.Type != domain.TxnReversal || rev.ReversesID == nil || *rev.ReversesID != orig.ID {
		t.Fatal("reversal harus bertipe REVERSAL dan menunjuk transaksi asal")
	}
	for i, e := range orig.Entries {
		r := rev.Entries[i]
		if r.AccountID != e.AccountID || r.Amount != e.Amount || r.Direction != e.Direction.Opposite() {
			t.Errorf("entry %d: arah harus dibalik dengan akun & nominal sama", i)
		}
	}
	// transaksi asli tidak disentuh
	if orig.Status != domain.TxnPosted || len(orig.Entries) != 3 {
		t.Error("transaksi asli tidak boleh berubah")
	}

	t.Run("transaksi yang sudah REVERSED ditolak", func(t *testing.T) {
		t.Parallel()
		done := *orig
		done.Status = domain.TxnReversed
		if _, err := domain.NewReversal(&done, 99, ""); !errors.Is(err, domain.ErrAlreadyReversed) {
			t.Fatalf("mau ErrAlreadyReversed, dapat %v", err)
		}
	})
	t.Run("reversal atas reversal ditolak", func(t *testing.T) {
		t.Parallel()
		if _, err := domain.NewReversal(rev, 99, ""); !errors.Is(err, domain.ErrNotReversible) {
			t.Fatalf("mau ErrNotReversible, dapat %v", err)
		}
	})
}

func TestDirection_Opposite(t *testing.T) {
	t.Parallel()
	if domain.DirectionDebit.Opposite() != domain.DirectionCredit || domain.DirectionCredit.Opposite() != domain.DirectionDebit {
		t.Fatal("Opposite salah")
	}
}

func TestAccount_OwnedBy(t *testing.T) {
	t.Parallel()
	uid := int64(5)
	a := domain.Account{UserID: &uid}
	if !a.OwnedBy(5) || a.OwnedBy(6) {
		t.Fatal("OwnedBy salah untuk dompet")
	}
	sys := domain.Account{}
	if sys.OwnedBy(5) {
		t.Fatal("akun sistem tidak dimiliki siapa pun")
	}
}
