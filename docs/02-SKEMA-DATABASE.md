# Skema Database — NusaLedger Fase 1

> **Revisi 2026-09-16** — dokumen ini sudah memuat keputusan `docs/06-PLAN-EKSEKUSI-AI.md` §2 (F-01, F-02, F-03, K-01…K-09). Kalau ada perbedaan dengan versi di luar repo, versi di dalam repo yang berlaku.

**PostgreSQL 17** · Disusun dari sudut pandang 🗄️ **Senior DBA**

Setiap tabel disertai penjelasan **kenapa dirancang begitu**, karena alasan di balik desain skema adalah yang paling sering ditanyakan saat wawancara backend perbankan.

---

## 0. Prinsip yang dipegang seluruh skema

| # | Prinsip | Alasan |
|---|---|---|
| 1 | Uang = `BIGINT` dalam sen | `0.1 + 0.2 ≠ 0.3` pada floating point. Selisih satu sen × jutaan transaksi = temuan audit |
| 2 | Aturan penting ditegakkan database, bukan aplikasi | Aplikasi bisa punya banyak jalur kode; database hanya satu pintu |
| 3 | Ledger append-only | Jejak audit harus tidak bisa dihapus, termasuk oleh developer |
| 4 | Saldo dimaterialisasi, entries tetap sumber kebenaran | Saldo cepat dibaca; kebenarannya bisa diverifikasi ulang kapan saja |
| 5 | UUID untuk ID yang terlihat publik, BIGINT untuk internal | UUID mencegah penebakan; BIGINT lebih hemat sebagai foreign key & index |
| 6 | `TIMESTAMPTZ`, bukan `TIMESTAMP` | Tanpa zona waktu, data akan salah begitu server pindah region |

---

## 1. Diagram relasi

```
users ──┬──< accounts ──< entries >── transactions
        │                                  │
        └──< refresh_tokens                └──> transactions (reverses_transaction_id)

idempotency_keys ──> transactions
outbox_events (disiapkan untuk Fase 2, belum dipakai)
```

---

## 2. DDL Lengkap

### 2.1 Extension & tipe enum

```sql
CREATE EXTENSION IF NOT EXISTS pgcrypto;   -- gen_random_uuid()

CREATE TYPE account_type    AS ENUM ('USER_WALLET','SYSTEM_CASH','SYSTEM_FEE_REVENUE','SYSTEM_SUSPENSE');
CREATE TYPE account_status  AS ENUM ('ACTIVE','FROZEN','CLOSED');
CREATE TYPE entry_direction AS ENUM ('DEBIT','CREDIT');
CREATE TYPE txn_type        AS ENUM ('TOPUP','TRANSFER','WITHDRAW','REVERSAL');
CREATE TYPE txn_status      AS ENUM ('POSTED','REVERSED');
```

> **Kenapa ENUM, bukan TEXT + CHECK (🗄️ DBA):** ENUM disimpan sebagai integer 4 byte, jadi lebih hemat ruang dan lebih cepat dibandingkan. Nilai salah ketik ditolak saat `INSERT`, bukan ditemukan enam bulan kemudian. Kelemahannya, menambah nilai baru butuh `ALTER TYPE` — tapi daftar jenis akun di sistem keuangan memang jarang berubah, jadi trade-off-nya menguntungkan di sini.

### 2.2 Tabel `users`

```sql
CREATE TABLE users (
    id            BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id     UUID        NOT NULL DEFAULT gen_random_uuid(),
    email         TEXT        NOT NULL,
    password_hash TEXT        NOT NULL,
    full_name     TEXT        NOT NULL CHECK (length(full_name) BETWEEN 1 AND 100),
    role          TEXT        NOT NULL DEFAULT 'USER' CHECK (role IN ('USER','ADMIN')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_users_email      ON users (LOWER(email));
CREATE UNIQUE INDEX idx_users_public_id  ON users (public_id);
```

**Kenapa begitu:**
- `GENERATED ALWAYS AS IDENTITY` menggantikan `SERIAL` — standar SQL, dan mencegah penyisipan nilai id secara manual yang bisa merusak sequence.
- **Dua ID**: `id` (BIGINT) untuk relasi internal, `public_id` (UUID) untuk dipakai di URL. Kalau API memakai `/users/1`, siapa pun bisa menebak `/users/2` dan mengukur jumlah pengguna. UUID menutup itu.
- `LOWER(email)` pada unique index membuat `Andi@x.com` dan `andi@x.com` dianggap sama — mencegah duplikasi akun yang sangat merepotkan untuk dibereskan belakangan.

### 2.3 Tabel `accounts`

```sql
CREATE TABLE accounts (
    id             BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id      UUID        NOT NULL DEFAULT gen_random_uuid(),
    user_id        BIGINT      REFERENCES users(id),      -- NULL untuk akun sistem
    account_type   account_type   NOT NULL,
    normal_balance entry_direction NOT NULL,
    status         account_status NOT NULL DEFAULT 'ACTIVE',
    currency       CHAR(3)     NOT NULL DEFAULT 'IDR' CHECK (currency = 'IDR'),

    balance        BIGINT      NOT NULL DEFAULT 0,        -- sen; saldo termaterialisasi
    version        BIGINT      NOT NULL DEFAULT 1,        -- optimistic lock

    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- BR-05: dompet pengguna tidak boleh negatif; akun sistem boleh
    CONSTRAINT chk_wallet_non_negative
        CHECK (account_type <> 'USER_WALLET' OR balance >= 0),

    -- konsistensi sisi normal per jenis akun
    CONSTRAINT chk_normal_balance CHECK (
        (account_type = 'SYSTEM_CASH'        AND normal_balance = 'DEBIT')  OR
        (account_type IN ('USER_WALLET','SYSTEM_FEE_REVENUE','SYSTEM_SUSPENSE')
                                             AND normal_balance = 'CREDIT')
    ),

    -- satu pengguna tepat satu dompet
    CONSTRAINT uq_user_wallet UNIQUE (user_id, account_type)
);

CREATE UNIQUE INDEX idx_accounts_public_id ON accounts (public_id);
CREATE INDEX idx_accounts_user            ON accounts (user_id) WHERE user_id IS NOT NULL;
CREATE UNIQUE INDEX idx_accounts_system_type
    ON accounts (account_type) WHERE user_id IS NULL;
```

**Kenapa saldo disimpan padahal entries adalah sumber kebenaran:**

Ada dua mazhab dan keduanya sah. Perbandingannya:

| | Saldo dihitung dari entries | Saldo dimaterialisasi (dipilih) |
|---|---|---|
| Baca saldo | `SUM()` atas jutaan baris — lambat | Satu kolom — instan |
| Risiko tidak sinkron | Tidak ada | Ada, harus diverifikasi berkala |
| Cek saldo saat transfer | Mahal, sulit dikunci | `WHERE balance >= x` — atomik & murah |

Pilihan ini disertai dua pengaman wajib: kolom `version` untuk optimistic lock, dan **job verifikasi** yang membandingkan `accounts.balance` dengan `SUM(entries)`. Selisih apa pun = alert keras.

> Jawaban untuk wawancara: *"Saldo dimaterialisasi demi performa baca dan agar pengecekan kecukupan saldo bisa atomik, tapi entries tetap sumber kebenaran. Ada job rekonsiliasi internal yang membuktikan keduanya cocok, dan endpoint trial balance yang membuktikan seluruh ledger seimbang."*

**Kenapa `idx_accounts_system_type` partial unique:** memastikan hanya ada **satu** akun SYSTEM_CASH, satu SYSTEM_FEE_REVENUE, dan seterusnya. Duplikat akun sistem akan memecah pencatatan tanpa ada yang menyadarinya.

### 2.4 Tabel `transactions`

```sql
CREATE TABLE transactions (
    id                      UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    txn_type                txn_type    NOT NULL,
    status                  txn_status  NOT NULL DEFAULT 'POSTED',
    description             TEXT        NOT NULL DEFAULT '',
    initiated_by_user_id    BIGINT      REFERENCES users(id),
    reverses_transaction_id UUID        REFERENCES transactions(id),
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- BR-11: satu transaksi hanya boleh dibalik sekali
    CONSTRAINT uq_reversal UNIQUE (reverses_transaction_id)
);

CREATE INDEX idx_txn_created ON transactions (created_at DESC);
CREATE INDEX idx_txn_type_created ON transactions (txn_type, created_at DESC);
```

**Kenapa UUID sebagai primary key di sini** (berbeda dengan tabel lain): ID transaksi dikirim ke klien dan muncul di URL. UUID mencegah penghitungan volume transaksi oleh pihak luar. Konsekuensi performanya dapat diterima karena tabel ini ditulis jauh lebih jarang daripada `entries`.

**Kenapa `UNIQUE (reverses_transaction_id)`:** ini mencegah *double reversal* di level database. Tanpa constraint ini, dua permintaan reversal bersamaan akan lolos keduanya dan uang tercipta dari udara. Validasi di aplikasi saja tidak cukup — ada jendela waktu antara pengecekan dan penyisipan. NULL diizinkan berkali-kali oleh PostgreSQL, jadi transaksi biasa tidak terganggu.

### 2.5 Tabel `entries` — jantung sistem

```sql
CREATE TABLE entries (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    transaction_id UUID   NOT NULL REFERENCES transactions(id),
    account_id     BIGINT NOT NULL REFERENCES accounts(id),
    direction      entry_direction NOT NULL,
    amount         BIGINT NOT NULL CHECK (amount > 0),      -- BR-02
    balance_after  BIGINT NOT NULL,                          -- snapshot saldo
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Query mutasi rekening: akun tertentu, terbaru dulu, cursor pagination
CREATE INDEX idx_entries_account_id_desc ON entries (account_id, id DESC);

-- Ambil semua entry milik satu transaksi
CREATE INDEX idx_entries_txn ON entries (transaction_id);

-- Tabel besar & terurut waktu: BRIN jauh lebih kecil daripada B-tree
CREATE INDEX idx_entries_created_brin ON entries USING BRIN (created_at);
```

**Kenapa ada `balance_after`:** ini snapshot saldo akun tepat setelah entry ini diposting. Tanpa kolom ini, menampilkan kolom "saldo" di mutasi rekening membutuhkan penjumlahan berjalan atas seluruh riwayat — mahal dan lambat. Dengan kolom ini, mutasi rekening hanyalah satu `SELECT` ber-index. Bonus: menjadi alat forensik. Kalau saldo hari ini salah, kamu bisa menelusuri entry mana yang pertama kali menyimpang.

**Kenapa index `(account_id, id DESC)` dan bukan `(account_id, created_at DESC)`:** dua entry bisa punya `created_at` identik (presisi mikrodetik tetap bisa bentrok pada operasi batch). Kalau cursor pagination memakai timestamp, baris bisa terlewat atau terduplikasi di halaman berikutnya. `id` selalu unik dan monoton, jadi cursor-nya stabil.

### 2.6 Trigger: penegak invariant

```sql
-- BR-03: total debit harus sama dengan total kredit per transaksi
CREATE OR REPLACE FUNCTION assert_transaction_balanced() RETURNS TRIGGER AS $$
DECLARE
    total_debit  BIGINT;
    total_credit BIGINT;
BEGIN
    SELECT COALESCE(SUM(amount) FILTER (WHERE direction = 'DEBIT'),  0),
           COALESCE(SUM(amount) FILTER (WHERE direction = 'CREDIT'), 0)
      INTO total_debit, total_credit
      FROM entries
     WHERE transaction_id = NEW.transaction_id;

    IF total_debit <> total_credit THEN
        RAISE EXCEPTION
            'transaksi % tidak seimbang: debit=% kredit=%',
            NEW.transaction_id, total_debit, total_credit
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER trg_entries_balanced
    AFTER INSERT ON entries
    DEFERRABLE INITIALLY DEFERRED       -- ← diperiksa saat COMMIT
    FOR EACH ROW
    EXECUTE FUNCTION assert_transaction_balanced();
```

> 🔴 **`DEFERRABLE INITIALLY DEFERRED` adalah kuncinya.** Entry disisipkan satu per satu. Setelah baris pertama (debit 51.000), transaksi belum seimbang — kalau trigger diperiksa saat itu juga, semua transaksi akan selalu ditolak. Dengan *deferred*, pemeriksaan ditunda sampai `COMMIT`, saat semua entry sudah masuk. Ini fitur PostgreSQL yang jarang dipakai dan hampir selalu mengesankan saat dijelaskan di wawancara.

```sql
-- BR-04: ledger append-only
CREATE OR REPLACE FUNCTION forbid_mutation() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'tabel % bersifat append-only: % ditolak',
        TG_TABLE_NAME, TG_OP
        USING ERRCODE = 'insufficient_privilege';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_entries_immutable
    BEFORE UPDATE OR DELETE ON entries
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

CREATE TRIGGER trg_transactions_no_delete
    BEFORE DELETE ON transactions
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();
```

Catatan: `transactions.status` masih perlu di-`UPDATE` saat transaksi dibalik, jadi tabel itu hanya dilindungi dari `DELETE`.

**Pengaman lapis kedua (opsional tapi disarankan):** cabut hak akses di level role, sehingga user aplikasi secara fisik tidak punya izin.

```sql
REVOKE UPDATE, DELETE ON entries FROM app_user;
```

> **Kenapa dua lapis (🏛️ ARC):** trigger melindungi dari kesalahan kode. Pencabutan hak melindungi dari orang yang membuka `psql` di produksi jam 2 pagi. Keduanya menangani ancaman berbeda.

### 2.7 Tabel `idempotency_keys`

```sql
CREATE TABLE idempotency_keys (
    key            TEXT        NOT NULL,
    user_id        BIGINT      NOT NULL REFERENCES users(id),
    endpoint       TEXT        NOT NULL,
    request_hash   TEXT        NOT NULL,              -- SHA-256 body
    transaction_id UUID        REFERENCES transactions(id),
    status_code    INT         NOT NULL,
    response_body  JSONB       NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- K-01 (docs/06 §2.2): key berlaku PER PENGGUNA, bukan global
    PRIMARY KEY (user_id, key)
);

CREATE INDEX idx_idem_cleanup ON idempotency_keys (created_at);
```

**Kenapa menyimpan `response_body` utuh:** saat klien mengirim ulang permintaan yang sama, dia harus menerima **respons yang persis sama**, bukan sekadar "sudah pernah diproses". Aplikasi mobile biasanya memarsing respons untuk menampilkan struk transaksi — respons berbeda akan membuat tampilannya rusak.

**Kenapa `request_hash`:** membedakan dua kasus yang sangat berbeda. Key sama + body sama = retry yang sah, kembalikan hasil lama. Key sama + body beda = klien salah pakai key, dan diam-diam mengembalikan transaksi lama akan menjadi bug yang nyaris mustahil dilacak. Kasus kedua ditolak `409`.

**Pembersihan** (cron harian, retensi 30 hari):
```sql
DELETE FROM idempotency_keys WHERE created_at < now() - INTERVAL '30 days';
```

### 2.8 Tabel `refresh_tokens`

```sql
CREATE TABLE refresh_tokens (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id     BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT        NOT NULL,              -- SHA-256, bukan token asli
    expires_at  TIMESTAMPTZ NOT NULL,
    revoked_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_refresh_hash ON refresh_tokens (token_hash);
CREATE INDEX idx_refresh_active ON refresh_tokens (user_id)
    WHERE revoked_at IS NULL;
```

**Kenapa hash, bukan token asli:** kalau database bocor, token asli bisa langsung dipakai menyamar sebagai pengguna. Hash tidak bisa. Prinsipnya sama dengan password — apa pun yang berfungsi sebagai kredensial tidak disimpan dalam bentuk yang bisa dipakai.

### 2.9 Tabel `outbox_events` (disiapkan, belum dipakai Fase 1)

```sql
CREATE TABLE outbox_events (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    event_id       UUID        NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    aggregate_type TEXT        NOT NULL,
    aggregate_id   TEXT        NOT NULL,
    event_type     TEXT        NOT NULL,
    payload        JSONB       NOT NULL,
    trace_id       TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at   TIMESTAMPTZ,
    attempts       INT         NOT NULL DEFAULT 0,
    last_error     TEXT
);

CREATE INDEX idx_outbox_unpublished ON outbox_events (created_at)
    WHERE published_at IS NULL;
```

> **Kenapa dibuat sekarang padahal dipakai Fase 2 (🏛️ ARC):** menambah tabel baru itu murah; mengubah kode transfer yang sudah teruji itu mahal. Dengan tabel sudah ada, service transfer bisa menulis event sejak Fase 1 tanpa konsumen — dan saat Fase 2 tiba, yang perlu ditambahkan hanya relay. Tidak ada perubahan pada jalur uang yang sudah terbukti benar.

---

## 3. Query kritis & pembuktian performanya

### 3.1 Mengunci akun untuk transfer — anti-deadlock

```sql
SELECT id, balance, version, status, normal_balance, account_type
FROM accounts
WHERE id = ANY($1)
ORDER BY id            -- ← WAJIB: urutan kunci selalu sama
FOR UPDATE;
```

> 🔴 **`ORDER BY id` bukan kosmetik.** Andi transfer ke Budi sementara Budi transfer ke Andi. Tanpa urutan tetap: transaksi 1 mengunci Andi lalu menunggu Budi; transaksi 2 mengunci Budi lalu menunggu Andi. Keduanya macet selamanya sampai PostgreSQL membunuh salah satunya. Dengan `ORDER BY id`, kedua transaksi mengunci akun ber-id kecil lebih dulu, sehingga yang kedua hanya **menunggu**, tidak deadlock. Ini satu baris yang menghilangkan seluruh kelas bug.

### 3.2 Memperbarui saldo — atomik + optimistic lock

```sql
UPDATE accounts
SET balance    = balance + $1,     -- delta bisa negatif
    version    = version + 1,
    updated_at = now()
WHERE id = $2
  AND version = $3                 -- optimistic lock
  AND status  = 'ACTIVE'           -- BR-10
RETURNING balance, version;
```

Kalau `RowsAffected = 0`, ada tiga kemungkinan: akun berubah sejak dibaca, akun tidak aktif, atau id salah. Aplikasi mengembalikan konflik dan klien boleh retry.

`CHECK (balance >= 0)` pada tabel menjadi jaring pengaman terakhir: sekalipun ada bug di kode, database menolak saldo negatif.

### 3.3 Mutasi rekening — cursor pagination

```sql
SELECT e.id, e.transaction_id, e.direction, e.amount, e.balance_after,
       e.created_at, t.txn_type, t.description
FROM entries e
JOIN transactions t ON t.id = e.transaction_id
WHERE e.account_id = $1
  AND ($2::BIGINT IS NULL OR e.id < $2)    -- cursor
ORDER BY e.id DESC
LIMIT $3;
```

> **Kenapa cursor, bukan `OFFSET` (🗄️ DBA):** `OFFSET 100000` memaksa PostgreSQL membaca lalu membuang 100.000 baris sebelum mengembalikan 20 baris yang diminta. Halaman ke-5000 akan lambat sekali. Cursor memakai index secara langsung, sehingga halaman ke-1 dan ke-5000 sama cepatnya. Bonus: tidak ada baris yang terlewat saat data baru masuk di tengah paginasi.

Verifikasi:
```sql
EXPLAIN (ANALYZE, BUFFERS)
SELECT ... WHERE e.account_id = 42 AND e.id < 9999 ORDER BY e.id DESC LIMIT 20;
-- Harus: Index Scan Backward using idx_entries_account_id_desc
-- Tidak boleh: Seq Scan
```

### 3.4 Trial balance — pembuktian integritas ledger

```sql
SELECT COALESCE(SUM(amount) FILTER (WHERE direction = 'DEBIT'),  0) AS total_debit,
       COALESCE(SUM(amount) FILTER (WHERE direction = 'CREDIT'), 0) AS total_credit,
       COALESCE(SUM(amount) FILTER (WHERE direction = 'DEBIT'),  0)
     - COALESCE(SUM(amount) FILTER (WHERE direction = 'CREDIT'), 0) AS difference
FROM entries;
-- difference HARUS 0. Selalu. Tanpa pengecualian.
```

### 3.5 Verifikasi saldo termaterialisasi

```sql
WITH computed AS (
    SELECT e.account_id,
           SUM(CASE WHEN e.direction = a.normal_balance THEN e.amount ELSE -e.amount END) AS calc
    FROM entries e
    JOIN accounts a ON a.id = e.account_id
    GROUP BY e.account_id
)
SELECT a.id, a.balance AS stored, c.calc AS computed, a.balance - c.calc AS drift
FROM accounts a
JOIN computed c ON c.account_id = a.id
WHERE a.balance <> c.calc;
-- Harus mengembalikan 0 baris. Kalau tidak → alert kritis, hentikan sistem.
```

Jadikan ini job terjadwal + metrik Prometheus `ledger_balance_drift_total`.

---

## 4. File Migration

```
migrations/
├── 000001_extensions_and_types.up.sql / .down.sql
├── 000002_users_and_auth.up.sql       / .down.sql
├── 000003_accounts.up.sql             / .down.sql
├── 000004_transactions_and_entries.up.sql / .down.sql
├── 000005_ledger_triggers.up.sql      / .down.sql
├── 000006_idempotency.up.sql          / .down.sql
├── 000007_outbox.up.sql               / .down.sql
└── 000008_seed_system_accounts.up.sql / .down.sql
```

Contoh seeder akun sistem:

```sql
-- 000008_seed_system_accounts.up.sql
INSERT INTO accounts (account_type, normal_balance, user_id) VALUES
    ('SYSTEM_CASH',        'DEBIT',  NULL),
    ('SYSTEM_FEE_REVENUE', 'CREDIT', NULL),
    ('SYSTEM_SUSPENSE',    'CREDIT', NULL)
ON CONFLICT DO NOTHING;
```

**Aturan migration yang dipatuhi:**
1. Setiap `up` punya `down` yang benar-benar berfungsi — tanpa itu, rollback deploy mustahil
2. Tidak ada migration yang mengubah data uang; migration hanya mengubah struktur
3. `CREATE INDEX CONCURRENTLY` untuk index pada tabel besar di produksi (tidak perlu di Fase 1 karena tabel masih kosong)
4. Satu migration = satu tujuan; jangan gabungkan tabel dan trigger dalam satu file

---

## 5. Setelan PostgreSQL & connection pool

```go
cfg.MaxConns        = 25
cfg.MinConns        = 5
cfg.MaxConnLifetime = 30 * time.Minute
cfg.MaxConnIdleTime = 5 * time.Minute
```

Perhitungan: `max_connections` PostgreSQL default 100. Sisakan ~20 untuk admin & tooling. Dengan 3 instance aplikasi: `(100 − 20) / 3 ≈ 26`, dibulatkan 25.

Setelan PostgreSQL yang relevan untuk beban ledger:

| Parameter | Nilai saran | Alasan |
|---|---|---|
| `default_transaction_isolation` | `read committed` | Cukup, karena kita memakai kunci eksplisit dan atomic update |
| `statement_timeout` | `30s` | Query nyangkut tidak boleh menahan koneksi selamanya |
| `idle_in_transaction_session_timeout` | `60s` | Transaksi yang lupa di-commit akan menahan kunci — ini membunuhnya |
| `log_min_duration_statement` | `200ms` | Semua query lambat tercatat untuk ditelaah |
| `deadlock_timeout` | `1s` | Default; naikkan hanya kalau deadlock sering dan sudah pasti wajar |

> **`idle_in_transaction_session_timeout` sering menyelamatkan sistem (🗄️ DBA):** satu bug yang membuka transaksi tanpa commit akan mengunci baris akun, dan seluruh transfer ke akun itu berhenti. Timeout ini mengubah insiden berjam-jam menjadi gangguan 60 detik.
