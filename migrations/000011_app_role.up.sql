-- Pertahanan lapis kedua untuk BR-04 (docs/02 §2.6, direkomendasikan sejak Fase 1,
-- baru diimplementasikan Fase 2): trigger forbid_mutation melindungi dari BUG KODE;
-- pencabutan hak akses di level role melindungi dari OPERATOR yang membuka psql
-- langsung (sengaja atau tidak) dan mengetik UPDATE/DELETE pada entries. Dua ancaman
-- berbeda, dua lapis berbeda — trigger tetap ada, ini menambah, bukan menggantikan.
--
-- Superuser 'nusa' (dibuat POSTGRES_USER compose) HANYA dipakai migrasi & administrasi.
-- Runtime cmd/api dan cmd/worker WAJIB connect sebagai peran ini (MIGRATION_DATABASE_URL
-- vs DATABASE_URL terpisah di config — lihat cmd/api/main.go, cmd/worker/main.go).
-- Nama database TIDAK di-hardcode ('nusaledger' di dev, tapi 'testdb' di testcontainers
-- — migration yang sama harus benar di keduanya): pakai current_database() lewat SQL
-- dinamis, karena GRANT ... ON DATABASE mensyaratkan identifier literal, bukan ekspresi.
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'nusaledger_app') THEN
        -- Password dev-only, sama pola dengan JWT_SECRET/RABBITMQ_DEFAULT_PASS di
        -- docker-compose.yml: jelas berlabel, WAJIB diganti di produksi.
        CREATE ROLE nusaledger_app LOGIN PASSWORD 'app_dev_only_ganti_di_produksi';
    END IF;
    EXECUTE format('GRANT CONNECT ON DATABASE %I TO nusaledger_app', current_database());
END$$;

GRANT USAGE ON SCHEMA public TO nusaledger_app;

-- Baseline: baca+tulis penuh. Cukup untuk users, accounts, transactions (header),
-- idempotency_keys, refresh_tokens, outbox_events, processed_events — semua tabel
-- yang memang BUTUH UPDATE (mis. accounts.balance, outbox_events.published_at).
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO nusaledger_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO nusaledger_app;

-- entries: HANYA SELECT + INSERT. TRUNCATE tidak pernah diberikan di atas (baseline
-- tidak menyebutnya), jadi tidak perlu di-REVOKE terpisah — absennya GRANT sudah cukup.
REVOKE UPDATE, DELETE ON entries FROM nusaledger_app;

-- transactions: DELETE dicabut (trigger sudah menolak, ini menutup jalurnya lebih dulu).
-- UPDATE tetap diizinkan di level GRANT — trigger transactions_status_only yang menegakkan
-- KOLOM mana yang boleh diubah (hanya status, hanya POSTED->REVERSED), bukan hak akses ini.
REVOKE DELETE ON transactions FROM nusaledger_app;

-- Hak default untuk OBJEK BARU (tabel dari migration berikutnya) mengikuti pola yang
-- sama tanpa perlu migration terpisah tiap kali menambah tabel.
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO nusaledger_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO nusaledger_app;
