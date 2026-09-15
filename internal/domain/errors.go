package domain

import "errors"

// Sentinel error domain. Lapisan lain membandingkannya dengan errors.Is,
// dan hanya transport/http/response.go yang menerjemahkannya ke status HTTP.
var (
	// pencarian data
	ErrNotFound            = errors.New("data tidak ditemukan")
	ErrAccountNotFound     = errors.New("akun tidak ditemukan")
	ErrTransactionNotFound = errors.New("transaksi tidak ditemukan")
	ErrUserNotFound        = errors.New("pengguna tidak ditemukan")

	// pengguna & akses
	ErrEmailTaken         = errors.New("email sudah terdaftar")
	ErrInvalidCredentials = errors.New("email atau kata sandi salah")
	ErrForbidden          = errors.New("tidak berhak melakukan operasi ini")

	// uang & aturan double-entry
	ErrAmountNotPositive  = errors.New("nominal harus lebih dari nol")
	ErrAmountOverflow     = errors.New("nominal melampaui batas aman")
	ErrAmountOutOfRange   = errors.New("nominal di luar batas yang diizinkan")
	ErrUnbalanced         = errors.New("transaksi tidak seimbang")
	ErrTooFewEntries      = errors.New("transaksi butuh minimal dua entry")
	ErrDuplicateAccount   = errors.New("satu akun hanya boleh muncul sekali dalam satu transaksi")
	ErrUnknownDirection   = errors.New("arah entry tidak dikenal")
	ErrDescriptionTooLong = errors.New("deskripsi maksimal 255 karakter")

	// aturan bisnis operasi
	ErrSelfTransfer           = errors.New("tidak bisa transfer ke akun sendiri")
	ErrInsufficientBalance    = errors.New("saldo tidak mencukupi")
	ErrAccountNotActive       = errors.New("akun tidak aktif")
	ErrAlreadyReversed        = errors.New("transaksi sudah pernah dibalik")
	ErrNotReversible          = errors.New("transaksi tidak bisa dibalik")
	ErrConcurrentModification = errors.New("data berubah oleh proses lain, coba lagi")

	// idempotency
	ErrIdempotencyConflict = errors.New("idempotency key sudah dipakai dengan isi permintaan berbeda")
	ErrIdempotencyInFlight = errors.New("permintaan dengan idempotency key sama sedang diproses")
)
