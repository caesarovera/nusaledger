package domain

// PostResult adalah hasil posting: transaksi tersimpan beserta entry
// dengan snapshot saldo sesudahnya.
//
// ViewerAccountID adalah anotasi presentasi (BUKAN kebenaran domain, tidak pernah
// disimpan/direplay): nil berarti pemanggil ADMIN dan boleh melihat balance_after
// SEMUA entry; selain itu, hanya entry milik akun ini yang balance_after-nya boleh
// ditampilkan ke klien (BR-12). Diisi oleh service, dibaca oleh transport/http/dto.go.
// Tanpa ini, transaksi apa pun yang melibatkan lebih dari satu akun (transfer,
// topup, withdraw) membocorkan saldo pihak lain — termasuk saldo akun sistem
// (SYSTEM_CASH, SYSTEM_FEE_REVENUE) ke pengguna biasa (temuan audit keamanan G-4/B-1).
type PostResult struct {
	Transaction     *Transaction
	Entries         []PostedEntry
	ViewerAccountID *int64
}

// BalanceAfterFor mengembalikan saldo akun tertentu tepat setelah posting.
func (p *PostResult) BalanceAfterFor(accountID int64) (Money, bool) {
	for _, e := range p.Entries {
		if e.AccountID == accountID {
			return e.BalanceAfter, true
		}
	}
	return 0, false
}

// TrialBalance adalah uji keseimbangan seluruh ledger (BR-15). Difference HARUS 0.
type TrialBalance struct {
	TotalDebit  Money
	TotalCredit Money
	Difference  Money
	EntryCount  int64
}

// Balanced benar bila Σ debit = Σ kredit.
func (t TrialBalance) Balanced() bool { return t.Difference == 0 }
