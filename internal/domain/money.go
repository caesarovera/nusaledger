package domain

import "fmt"

// Money merepresentasikan rupiah dalam satuan SEN (Rp 1 = 100 sen).
// Tipe tersendiri, bukan int64 telanjang, agar compiler menolak
// pencampuran dengan angka lain seperti kuantitas atau ID.
type Money int64

// Rupiah adalah pengali untuk menulis nominal dengan jelas: 50_000 * Rupiah.
const Rupiah Money = 100

// maxSafeAmount membatasi satu nominal pada Rp 1 triliun. Jauh di bawah
// MaxInt64 (±Rp 92 triliun), sehingga penjumlahan beberapa nominal pun aman.
const maxSafeAmount Money = 1_000_000_000_000 * Rupiah

// NewMoney memvalidasi nominal dari luar (request, database) sebelum dipakai.
func NewMoney(sen int64) (Money, error) {
	m := Money(sen)
	if m <= 0 {
		return 0, ErrAmountNotPositive
	}
	if m > maxSafeAmount {
		return 0, ErrAmountOverflow
	}
	return m, nil
}

// Add menjumlahkan dengan deteksi overflow eksplisit.
// Go tidak melempar error saat int64 meluap; nilainya diam-diam berputar.
func (m Money) Add(o Money) (Money, error) {
	sum := m + o
	if (o > 0 && sum < m) || (o < 0 && sum > m) {
		return 0, ErrAmountOverflow
	}
	return sum, nil
}

// Sub mengurangkan dengan deteksi overflow.
func (m Money) Sub(o Money) (Money, error) {
	if o == Money(-1<<63) { // -o akan meluap
		return 0, ErrAmountOverflow
	}
	return m.Add(-o)
}

// IsPositive benar bila nominal lebih dari nol.
func (m Money) IsPositive() bool { return m > 0 }

// String menampilkan format rupiah untuk log dan debugging, misal "Rp 50000,50".
func (m Money) String() string {
	sign := ""
	v := int64(m)
	if v < 0 {
		sign = "-"
		v = -v
	}
	return fmt.Sprintf("%sRp %d,%02d", sign, v/100, v%100)
}
