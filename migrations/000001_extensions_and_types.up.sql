-- Extension & tipe enum. Dibuat terpisah karena dipakai semua tabel berikutnya.
CREATE EXTENSION IF NOT EXISTS pgcrypto;   -- gen_random_uuid()

CREATE TYPE account_type    AS ENUM ('USER_WALLET', 'SYSTEM_CASH', 'SYSTEM_FEE_REVENUE', 'SYSTEM_SUSPENSE');
CREATE TYPE account_status  AS ENUM ('ACTIVE', 'FROZEN', 'CLOSED');
CREATE TYPE entry_direction AS ENUM ('DEBIT', 'CREDIT');
CREATE TYPE txn_type        AS ENUM ('TOPUP', 'TRANSFER', 'WITHDRAW', 'REVERSAL');
CREATE TYPE txn_status      AS ENUM ('POSTED', 'REVERSED');
