// Package migrations menyematkan file SQL ke dalam binary, sehingga test dan
// proses startup menjalankan migration yang PERSIS sama dengan yang di-review di repo.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
