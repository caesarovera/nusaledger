# Spesifikasi Teknis — NusaLedger Fase 1

> **Revisi 2026-09-16** — dokumen ini sudah memuat keputusan `docs/06-PLAN-EKSEKUSI-AI.md` §2 (F-01, F-02, F-03, K-01…K-09). Kalau ada perbedaan dengan versi di luar repo, versi di dalam repo yang berlaku.

Disusun dari sudut pandang 🏛️ **Senior Architect** + 🧑‍💻 **Senior Developer** + 🧪 **Senior QA**

---

## 1. Struktur Proyek

```
nusaledger/
├── cmd/
│   └── api/main.go                    # satu-satunya entrypoint di Fase 1
├── internal/
│   ├── domain/                        # lapisan terdalam, tanpa dependensi eksternal
│   │   ├── money.go                   # tipe Money + aritmetika aman
│   │   ├── account.go
│   │   ├── entry.go
│   │   ├── transaction.go             # aturan double-entry
│   │   ├── user.go
│   │   └── errors.go                  # sentinel error
│   ├── service/
│   │   ├── ledger_service.go          # topup, transfer, withdraw, reversal
│   │   ├── auth_service.go
│   │   ├── ports.go                   # interface yang DIBUTUHKAN service
│   │   └── idempotency.go
│   ├── repository/postgres/
│   │   ├── pool.go
│   │   ├── ledger_repo.go
│   │   ├── account_repo.go
│   │   ├── user_repo.go
│   │   └── idempotency_repo.go
│   ├── transport/http/
│   │   ├── router.go
│   │   ├── handler_auth.go
│   │   ├── handler_account.go
│   │   ├── handler_transaction.go
│   │   ├── middleware.go
│   │   ├── dto.go                     # request/response, TERPISAH dari domain
│   │   └── response.go                # writeJSON, writeError
│   ├── platform/
│   │   ├── logger/logger.go
│   │   ├── metrics/metrics.go
│   │   └── token/jwt.go
│   └── config/config.go
├── migrations/
├── test/
│   ├── integration/                   # build tag: integration
│   └── load/transfer.js               # skrip k6
├── docs/openapi.yaml
├── Dockerfile
├── docker-compose.yml
├── Makefile
├── CLAUDE.md
├── HANDOVER.md
└── .claude/
    ├── agents/
    ├── skills/
    └── settings.json
```

**Aturan dependensi — panah hanya boleh ke bawah:**

```
transport/http  →  service  →  domain
repository      →  domain
```

`domain` tidak boleh mengimpor apa pun dari `service`, `repository`, atau `transport`. Kalau tergoda melakukannya, itu tanda ada logika yang salah tempat.

---

## 2. Tipe Money — pertahanan pertama terhadap bug uang

```go
// internal/domain/money.go
package domain

// Money merepresentasikan rupiah dalam satuan SEN.
// Tipe tersendiri (bukan int64 telanjang) agar compiler mencegah
// pencampuran dengan angka lain seperti kuantitas atau ID.
type Money int64

const Rupiah Money = 100

var (
    ErrAmountNotPositive = errors.New("nominal harus lebih dari nol")
    ErrAmountOverflow    = errors.New("nominal melampaui batas aman")
)

const maxSafeAmount Money = 1_000_000_000_000_00   // Rp 1 triliun dalam sen

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
func (m Money) Add(o Money) (Money, error) {
    sum := m + o
    if (o > 0 && sum < m) || (o < 0 && sum > m) {
        return 0, ErrAmountOverflow
    }
    return sum, nil
}

func (m Money) Sub(o Money) (Money, error) { return m.Add(-o) }

// String menampilkan format rupiah untuk log dan debugging.
func (m Money) String() string {
    return fmt.Sprintf("Rp %d,%02d", int64(m)/100, abs(int64(m)%100))
}
```

> **Kenapa tipe sendiri, bukan `int64` (🧑‍💻 DEV):** dengan `int64`, tidak ada yang mencegah `transfer(userID, amount)` dipanggil terbalik menjadi `transfer(amount, userID)` — keduanya `int64`, compiler diam. Dengan tipe `Money`, kesalahan itu ditolak saat kompilasi. Biayanya nol saat runtime karena Go tidak membungkus apa pun.
>
> **Kenapa overflow diperiksa manual:** Go tidak melempar error saat `int64` meluap; nilainya diam-diam berputar menjadi negatif. Pada sistem uang, saldo yang tiba-tiba negatif triliunan adalah bug yang harus mustahil terjadi, bukan sekadar tidak mungkin.

---

## 3. Aturan double-entry di lapisan domain

```go
// internal/domain/transaction.go

type Entry struct {
    AccountID int64
    Direction Direction        // DEBIT atau CREDIT
    Amount    Money
}

type Transaction struct {
    ID          uuid.UUID
    Type        TxnType
    Entries     []Entry
    Description string
}

var (
    ErrUnbalanced     = errors.New("transaksi tidak seimbang")
    ErrTooFewEntries  = errors.New("transaksi butuh minimal dua entry")
)

// Validate menegakkan BR-02 dan BR-03 SEBELUM menyentuh database.
func (t *Transaction) Validate() error {
    if len(t.Entries) < 2 {
        return ErrTooFewEntries
    }
    var debit, credit Money
    for _, e := range t.Entries {
        if e.Amount <= 0 {
            return ErrAmountNotPositive
        }
        var err error
        switch e.Direction {
        case DirectionDebit:
            debit, err = debit.Add(e.Amount)
        case DirectionCredit:
            credit, err = credit.Add(e.Amount)
        default:
            return fmt.Errorf("arah entry tidak dikenal: %s", e.Direction)
        }
        if err != nil {
            return err
        }
    }
    if debit != credit {
        return fmt.Errorf("%w: debit=%s kredit=%s", ErrUnbalanced, debit, credit)
    }
    return nil
}

// BalanceDelta menghitung perubahan saldo satu entry terhadap akunnya.
func BalanceDelta(e Entry, normalBalance Direction) Money {
    if e.Direction == normalBalance {
        return e.Amount
    }
    return -e.Amount
}
```

> **Kenapa validasi ada di dua tempat — domain dan trigger database (🧪 QA + 🗄️ DBA):** ini bukan duplikasi sia-sia, melainkan pertahanan berlapis dengan peran berbeda. Validasi domain memberi **pesan error yang berguna** dan bisa diuji dalam mikrodetik tanpa database. Trigger database menjadi **jaminan terakhir** yang berlaku bahkan untuk skrip migrasi, perbaikan data manual, atau jalur kode baru yang lupa memanggil `Validate()`. Kalau harus memilih satu, pilih trigger — tapi jangan memilih.

---

## 4. Kontrak API

Base path: `/api/v1`. Semua respons `application/json`.

### 4.1 Daftar endpoint

| Method | Path | Auth | Idempotency-Key | Keterangan |
|---|---|---|---|---|
| POST | `/auth/register` | — | — | Buat user + dompet otomatis |
| POST | `/auth/login` | — | — | Access + refresh token |
| POST | `/auth/refresh` | — | — | Tukar refresh token |
| POST | `/auth/logout` | ✅ | — | Cabut refresh token |
| GET | `/accounts/me` | ✅ | — | Saldo & info dompet |
| GET | `/accounts/me/entries` | ✅ | — | Mutasi, cursor pagination |
| POST | `/transactions/topup` | ✅ | **wajib** | Isi saldo (simulasi) |
| POST | `/transactions/transfer` | ✅ | **wajib** | Transfer antar dompet |
| POST | `/transactions/withdraw` | ✅ | **wajib** | Tarik dana (simulasi) |
| GET | `/transactions/{id}` | ✅ | — | Detail + semua entry |
| POST | `/transactions/{id}/reverse` | ✅ ADMIN | **wajib** | Pembatalan |
| GET | `/internal/ledger/trial-balance` | ✅ ADMIN | — | Bukti ledger seimbang |
| GET | `/healthz` | — | — | Liveness — jangan cek DB |
| GET | `/readyz` | — | — | Readiness — cek DB |
| GET | `/metrics` | — | — | Prometheus |

### 4.2 Contoh: transfer

**Request**
```http
POST /api/v1/transactions/transfer
Authorization: Bearer <access_token>
Idempotency-Key: 7f3c1b9a-2e44-4d18-9f21-8b6c5a0d3e77
Content-Type: application/json

{
  "to_account_public_id": "9c1e5d2b-7a34-4f81-b0c6-2d9e4a7f1b53",
  "amount_sen": 5000000,
  "description": "bayar kos"
}
```

**Respons 201**
```json
{
  "data": {
    "transaction_id": "1a2b3c4d-...",
    "type": "TRANSFER",
    "status": "POSTED",
    "amount_sen": 5000000,
    "fee_sen": 100000,
    "total_debited_sen": 5100000,
    "balance_after_sen": 12400000,
    "created_at": "2026-09-13T10:22:31+07:00"
  }
}
```

**Respons error — format seragam di seluruh API**
```json
{
  "error": {
    "code": "INSUFFICIENT_BALANCE",
    "message": "saldo tidak mencukupi",
    "fields": null,
    "request_id": "01JBXR7K9M2Q"
  }
}
```

> **Kenapa nominal bernama `amount_sen` (🏛️ ARC):** menyematkan satuan di nama field membuat kesalahan interpretasi hampir mustahil. Field bernama `amount` akan ditanyakan berulang kali oleh tim frontend — "ini rupiah atau sen?" — dan cepat atau lambat ada yang salah menebak. Nama yang eksplisit menghemat lebih banyak waktu daripada yang terlihat.

### 4.3 Katalog kode error

| Kode | HTTP | Kapan |
|---|---|---|
| `VALIDATION_ERROR` | 400 | Format request salah |
| `IDEMPOTENCY_KEY_REQUIRED` | 400 | Header hilang pada endpoint uang |
| `UNAUTHENTICATED` | 401 | Token tidak ada/kedaluwarsa/invalid |
| `FORBIDDEN` | 403 | Peran tidak mencukupi |
| `ACCOUNT_NOT_FOUND` | 404 | Akun tujuan tidak ada |
| `TRANSACTION_NOT_FOUND` | 404 | — |
| `IDEMPOTENCY_CONFLICT` | 409 | Key sama, body beda |
| `IDEMPOTENCY_IN_FLIGHT` | 409 | Key sama sedang diproses permintaan lain; kirim `Retry-After: 1`, klien boleh retry dengan key sama |
| `ALREADY_REVERSED` | 409 | Transaksi sudah dibalik |
| `CONCURRENT_MODIFICATION` | 409 | Optimistic lock gagal, klien boleh retry |
| `INSUFFICIENT_BALANCE` | 422 | Saldo kurang |
| `ACCOUNT_NOT_ACTIVE` | 422 | Akun beku/tutup |
| `SELF_TRANSFER` | 422 | Transfer ke diri sendiri |
| `AMOUNT_OUT_OF_RANGE` | 422 | Di luar batas BR-14 |
| `RATE_LIMITED` | 429 | Melebihi batas laju |
| `INTERNAL_ERROR` | 500 | Selain di atas |

> **Kenapa 422, bukan 400, untuk saldo kurang (🏛️ ARC):** 400 berarti "permintaanmu tidak bisa saya pahami" — masalah format. 422 berarti "saya paham, tetapi tidak bisa memprosesnya" — masalah keadaan. Perbedaan ini penting bagi klien: 400 tidak pernah layak diulang, sedangkan 422 mungkin berhasil setelah pengguna mengisi saldo.

---

## 5. Implementasi transfer — kode acuan

```go
// internal/service/ledger_service.go

func (s *LedgerService) Transfer(ctx context.Context, in TransferInput) (*TransferResult, error) {
    // 1. Validasi yang tidak butuh database — gagal cepat & murah
    if in.FromAccountID == in.ToAccountID {
        return nil, domain.ErrSelfTransfer
    }
    amount, err := domain.NewMoney(in.AmountSen)
    if err != nil {
        return nil, err
    }
    if amount < s.cfg.MinTransfer || amount > s.cfg.MaxTransfer {
        return nil, domain.ErrAmountOutOfRange
    }

    // 2. Jalur cepat idempotency — hindari membuka transaksi bila sudah pernah
    if rec, err := s.idem.Find(ctx, in.IdempotencyKey); err == nil {
        if rec.RequestHash != in.RequestHash {
            return nil, domain.ErrIdempotencyConflict
        }
        return rec.AsTransferResult()
    } else if !errors.Is(err, domain.ErrNotFound) {
        return nil, fmt.Errorf("cek idempotency: %w", err)
    }

    // 3. Susun transaksi di domain, validasi SEBELUM menyentuh database
    fee := s.cfg.TransferFee
    total, err := amount.Add(fee)
    if err != nil {
        return nil, err
    }

    txn := &domain.Transaction{
        Type:        domain.TxnTransfer,
        Description: in.Description,
        Entries: []domain.Entry{
            {AccountID: in.FromAccountID,  Direction: domain.DirectionDebit,  Amount: total},
            {AccountID: in.ToAccountID,    Direction: domain.DirectionCredit, Amount: amount},
            {AccountID: s.feeAccountID,    Direction: domain.DirectionCredit, Amount: fee},
        },
    }
    if err := txn.Validate(); err != nil {
        return nil, fmt.Errorf("transaksi tidak valid: %w", err)
    }

    // 4. Serahkan ke repository untuk diposting secara atomik
    return s.repo.PostTransaction(ctx, txn, in.IdempotencyKey, in.RequestHash)
}
```

```go
// internal/repository/postgres/ledger_repo.go

func (r *LedgerRepo) PostTransaction(
    ctx context.Context, txn *domain.Transaction, idemKey, reqHash string,
) (*service.TransferResult, error) {

    tx, err := r.db.Begin(ctx)
    if err != nil {
        return nil, fmt.Errorf("begin: %w", err)
    }
    defer tx.Rollback(ctx)      // no-op setelah commit; jaring pengaman

    // 4a. Klaim idempotency key di dalam transaksi yang sama
    ct, err := tx.Exec(ctx, `
        INSERT INTO idempotency_keys (key, user_id, endpoint, request_hash, status_code, response_body)
        VALUES ($1,$2,$3,$4,0,'{}'::jsonb)
        ON CONFLICT (user_id, key) DO NOTHING`,   // K-01: PK komposit
        idemKey, txn.InitiatedBy, "transfer", reqHash)
    if err != nil {
        return nil, fmt.Errorf("klaim idempotency: %w", err)
    }
    if ct.RowsAffected() == 0 {
        return nil, domain.ErrIdempotencyInFlight   // proses lain sedang mengerjakan
    }

    // 4b. Kunci semua akun terlibat — URUT berdasarkan id, mencegah deadlock
    ids := txn.AccountIDs()
    slices.Sort(ids)

    rows, err := tx.Query(ctx, `
        SELECT id, balance, version, status, normal_balance, account_type
        FROM accounts WHERE id = ANY($1) ORDER BY id FOR UPDATE`, ids)
    if err != nil {
        return nil, fmt.Errorf("kunci akun: %w", err)
    }
    accounts, err := pgx.CollectRows(rows, pgx.RowToStructByName[accountRow])
    if err != nil {
        return nil, fmt.Errorf("scan akun: %w", err)
    }
    if len(accounts) != len(ids) {
        return nil, domain.ErrAccountNotFound
    }

    byID := make(map[int64]accountRow, len(accounts))
    for _, a := range accounts {
        if a.Status != "ACTIVE" {
            return nil, fmt.Errorf("%w: akun %d berstatus %s",
                domain.ErrAccountNotActive, a.ID, a.Status)
        }
        byID[a.ID] = a
    }

    // 4c. Sisipkan header transaksi
    var txnID uuid.UUID
    var createdAt time.Time
    err = tx.QueryRow(ctx, `
        INSERT INTO transactions (txn_type, description, initiated_by_user_id)
        VALUES ($1,$2,$3) RETURNING id, created_at`,
        txn.Type, txn.Description, txn.InitiatedBy).Scan(&txnID, &createdAt)
    if err != nil {
        return nil, fmt.Errorf("insert transaksi: %w", err)
    }

    // 4d. Untuk tiap entry: hitung saldo baru, update akun, sisipkan entry
    balancesAfter := make(map[int64]domain.Money, len(txn.Entries))
    for _, e := range txn.Entries {
        acc := byID[e.AccountID]
        delta := domain.BalanceDelta(e, acc.NormalBalance)

        var newBalance domain.Money
        err = tx.QueryRow(ctx, `
            UPDATE accounts
            SET balance = balance + $1, version = version + 1, updated_at = now()
            WHERE id = $2 AND version = $3 AND status = 'ACTIVE'
            RETURNING balance`,
            int64(delta), e.AccountID, acc.Version).Scan(&newBalance)

        if errors.Is(err, pgx.ErrNoRows) {
            return nil, domain.ErrConcurrentModification
        }
        if err != nil {
            // CHECK constraint chk_wallet_non_negative memicu error ini
            if isCheckViolation(err, "chk_wallet_non_negative") {
                return nil, domain.ErrInsufficientBalance
            }
            return nil, fmt.Errorf("update saldo akun %d: %w", e.AccountID, err)
        }

        _, err = tx.Exec(ctx, `
            INSERT INTO entries (transaction_id, account_id, direction, amount, balance_after)
            VALUES ($1,$2,$3,$4,$5)`,
            txnID, e.AccountID, e.Direction, int64(e.Amount), int64(newBalance))
        if err != nil {
            return nil, fmt.Errorf("insert entry: %w", err)
        }
        balancesAfter[e.AccountID] = newBalance
    }

    // 4e. Simpan respons final untuk retry idempoten
    result := buildResult(txnID, createdAt, txn, balancesAfter)
    body, err := json.Marshal(result)
    if err != nil {
        return nil, fmt.Errorf("marshal respons: %w", err)
    }
    _, err = tx.Exec(ctx, `
        UPDATE idempotency_keys
        SET transaction_id = $1, status_code = 201, response_body = $2
        WHERE user_id = $4 AND key = $3`, txnID, body, idemKey, txn.InitiatedBy)
    if err != nil {
        return nil, fmt.Errorf("simpan respons idempotency: %w", err)
    }

    // COMMIT — di sinilah trigger memverifikasi debit = kredit
    if err := tx.Commit(ctx); err != nil {
        if isCheckViolation(err, "") {
            return nil, fmt.Errorf("%w: %v", domain.ErrUnbalanced, err)
        }
        return nil, fmt.Errorf("commit: %w", err)
    }
    return result, nil
}
```

**Enam keputusan yang membuat kode ini benar:**

| # | Keputusan | Kalau tidak dilakukan |
|---|---|---|
| 1 | Klaim idempotency di dalam transaksi yang sama | Key tercatat tapi transaksi rollback → retry ditolak padahal uang belum pindah |
| 2 | `slices.Sort(ids)` sebelum `FOR UPDATE` | Deadlock pada transfer silang |
| 3 | `version` pada `WHERE` update saldo | Lost update saat dua transfer bersamaan |
| 4 | Cek `status = 'ACTIVE'` di SQL, bukan hanya di Go | Ada jendela waktu antara pengecekan dan update |
| 5 | Menerjemahkan pelanggaran CHECK jadi error domain | Klien menerima 500 padahal seharusnya 422 |
| 6 | `defer tx.Rollback` sebelum semua operasi | Satu `return` yang terlewat meninggalkan transaksi menggantung |

---

## 6. Keamanan Fase 1

| Area | Keputusan | Alasan |
|---|---|---|
| Hash password | argon2id (`golang.org/x/crypto/argon2`) | Tahan serangan GPU; bcrypt juga boleh dengan cost 12 |
| Access token | JWT HS256, umur 15 menit | Stateless & cepat; umur pendek membatasi dampak kebocoran |
| Refresh token | Acak 32 byte, disimpan sebagai SHA-256, umur 30 hari | Bisa dicabut; hash melindungi saat database bocor |
| Validasi JWT | Wajib memeriksa tipe algoritma secara eksplisit | Mencegah serangan `alg: none` dan algorithm confusion |
| IDOR | Kepemilikan difilter di klausa `WHERE` | Data milik orang lain tidak pernah keluar dari database |
| Rate limit login | 5 percobaan / 15 menit per akun + per IP, **fail-closed** | Menutup brute force; lebih baik menolak daripada membuka pintu |
| Rate limit transfer | 20 / menit per user, fail-closed | Membatasi dampak akun yang diambil alih |
| Body size | `http.MaxBytesReader` 1 MB | Mencegah DoS memori |
| Timeout server | ReadHeader 5s, Read 15s, Write 30s | Menutup serangan Slowloris |
| Enumerasi user | Pesan login selalu sama + dummy hash | Mencegah pemetaan daftar email |
| Log | Redaksi terpusat untuk `password`, `token`, `authorization` | PII & kredensial tidak boleh masuk log |
| Dependency | `govulncheck` di CI | Kerentanan pustaka terdeteksi otomatis |

---

## 7. Strategi Pengujian (🧪 Senior QA)

### 7.1 Piramida untuk Fase 1

| Lapisan | Porsi | Isi | Waktu jalan |
|---|---|---|---|
| Unit | 70% | `domain` (Money, Validate, BalanceDelta), `service` dengan stub | < 5 detik |
| Integration | 25% | repository + PostgreSQL asli via testcontainers | 30–60 detik |
| E2E | 5% | HTTP end-to-end lewat `httptest` + DB | 10–20 detik |

### 7.2 Test wajib — bukan opsional

| Kode | Test | Membuktikan |
|---|---|---|
| **T-01** | Transaksi tidak seimbang ditolak `Validate()` | BR-03 di domain |
| **T-02** | Trigger menolak entry tidak seimbang saat COMMIT | BR-03 di database |
| **T-03** | `UPDATE`/`DELETE` pada `entries` ditolak | BR-04 |
| **T-04** | 100 goroutine transfer dari saldo Rp 1.000.000 @ Rp 50.000 → tepat **19** sukses (fee Rp 1.000 ikut didebit: 19 × 51.000 = 969.000), sisa saldo Rp 31.000 | BR-05, optimistic lock |
| **T-05** | 10 goroutine, `Idempotency-Key` sama, bersamaan → tepat 1 transaksi | BR-07 |
| **T-06** | Key sama + body beda → `409` | BR-08 |
| **T-07** | 50× transfer A→B dan B→A bersamaan → tanpa deadlock | Urutan penguncian |
| **T-08** | 1.000 transaksi acak → trial balance tetap 0 | BR-15 |
| **T-09** | 1.000 transaksi acak → `accounts.balance` = `SUM(entries)` untuk semua akun | Integritas materialisasi |
| **T-10** | Reversal kedua pada transaksi yang sama → `409` | BR-11 |
| **T-11** | User A tidak bisa membaca akun/mutasi User B | BR-12, IDOR |
| **T-12** | Context dibatalkan → query berhenti, `context.Canceled` | Higienis context |
| **T-13** | SIGTERM saat request 10 detik → request tetap selesai | Graceful shutdown |
| **T-14** | Overflow `Money.Add` terdeteksi | Keamanan aritmetika |

**T-04 adalah test paling penting di seluruh proyek.** Ini yang membuktikan sistemmu benar di bawah tekanan, dan ini pula yang paling menarik diceritakan saat wawancara.

```go
func TestTransfer_ConcurrentDebit(t *testing.T) {
    db := setupDB(t)
    svc := newLedgerService(db)
    ctx := context.Background()

    from := seedWallet(t, db, 1_000_000_00)   // Rp 1.000.000 dalam sen
    to   := seedWallet(t, db, 0)

    const (
        workers = 100
        amount  = 50_000_00                    // Rp 50.000
        fee     = 1_000_00                     // Rp 1.000
    )

    var wg sync.WaitGroup
    var okCount, insufficientCount atomic.Int64

    for i := 0; i < workers; i++ {
        wg.Add(1)
        go func(i int) {
            defer wg.Done()
            _, err := svc.Transfer(ctx, TransferInput{
                FromAccountID:  from,
                ToAccountID:    to,
                AmountSen:      amount,
                IdempotencyKey: uuid.NewString(),   // key BERBEDA: ini bukan retry
            })
            switch {
            case err == nil:
                okCount.Add(1)
            case errors.Is(err, domain.ErrInsufficientBalance),
                 errors.Is(err, domain.ErrConcurrentModification):
                insufficientCount.Add(1)
            default:
                t.Errorf("error tak terduga: %v", err)
            }
        }(i)
    }
    wg.Wait()

    // Rp 1.000.000 / (50.000 + 1.000) = 19 transfer penuh
    const expected = 19
    if got := okCount.Load(); got != expected {
        t.Fatalf("transfer sukses: mau %d, dapat %d", expected, got)
    }

    // Invariant yang tidak boleh dilanggar apa pun yang terjadi
    assertNoNegativeBalance(t, db)
    assertTrialBalanceZero(t, db)
    assertMaterializedBalanceMatchesEntries(t, db)
}
```

> **Kenapa test ini memakai key berbeda per goroutine (🧪 QA):** dengan key berbeda, ini menguji **konkurensi saldo**. Dengan key sama, ini menguji **idempotency** (T-05). Dua hal berbeda, dua test terpisah. Mencampurnya akan membuat kegagalan sulit didiagnosis.
>
> **Kenapa `ErrConcurrentModification` diterima sebagai kegagalan yang sah:** optimistic lock memang menolak sebagian permintaan saat kontensi tinggi. Yang penting bukan semua berhasil, tetapi **tidak ada uang yang tercipta atau hilang**. Tiga assert invariant di akhir itulah inti pengujiannya.

### 7.3 Load test

```javascript
// test/load/transfer.js
export const options = {
  stages: [
    { duration: '30s', target: 50  },
    { duration: '2m',  target: 100 },
    { duration: '30s', target: 0   },
  ],
  thresholds: {
    http_req_duration: ['p(95)<200', 'p(99)<500'],
    http_req_failed:   ['rate<0.01'],
  },
};
```

Setelah load test, jalankan verifikasi ledger. **Sistem yang cepat tetapi salah lebih buruk daripada sistem yang lambat dan benar.**

---

## 8. Observability

```go
// Metrik minimum Fase 1
http_request_duration_seconds{method,route,status}   // histogram
http_requests_total{method,route,status}
db_pool_conns_in_use                                  // gauge — alert dini
db_pool_conns_max
ledger_transactions_total{type,status}
ledger_balance_drift_total                            // HARUS selalu 0
ledger_trial_balance_difference                       // HARUS selalu 0
auth_login_failures_total{reason}
```

> 🔴 **`ledger_balance_drift_total` adalah metrik paling penting di sistem ini.** Nilai selain 0 berarti ada uang yang tidak bisa dipertanggungjawabkan. Ini bukan alert "lihat besok pagi" — ini alert yang menghentikan penerimaan transaksi baru.

Log terstruktur wajib memuat: `request_id`, `user_id`, `transaction_id`, `duration_ms`. Tanpa `request_id` yang mengalir di semua lapisan, menelusuri satu transfer bermasalah di antara jutaan baris log tidak mungkin dilakukan.

---

## 9. Rencana 4 minggu

| Minggu | Keluaran | Selesai bila |
|---|---|---|
| **1** | Skema + migration + domain + unit test | T-01, T-14 hijau; `go test ./...` < 5 detik |
| **2** | Repository + posting transaksi + testcontainers | T-02, T-03, T-08, T-09 hijau |
| **3** | Service + idempotency + HTTP + auth | T-04 – T-07, T-10, T-11 hijau |
| **4** | Observability, load test, Docker, CI, README | T-12, T-13 hijau; SLO tercapai; `docker compose up` jalan |

**Aturan yang menjaga proyek tetap hidup:** setiap akhir minggu, repo harus dalam keadaan **bisa dijalankan dan bisa dipamerkan**. Jangan pernah meninggalkan cabang setengah jadi di akhir pekan — itu titik di mana motivasi paling sering putus.
