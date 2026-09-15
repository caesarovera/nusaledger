package domain

// PostResult adalah hasil posting: transaksi tersimpan beserta entry
// dengan snapshot saldo sesudahnya.
type PostResult struct {
	Transaction *Transaction
	Entries     []PostedEntry
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
