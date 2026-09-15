-- ============================================================
-- BR-03: total debit = total kredit untuk setiap transaksi.
-- Diperiksa saat COMMIT (deferred), karena entry disisipkan satu per satu.
-- ============================================================
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
        RAISE EXCEPTION 'transaksi % tidak seimbang: debit=% kredit=%',
            NEW.transaction_id, total_debit, total_credit
            USING ERRCODE = 'check_violation',
                  CONSTRAINT = 'trg_entries_balanced';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER trg_entries_balanced
    AFTER INSERT ON entries
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW
    EXECUTE FUNCTION assert_transaction_balanced();

-- ============================================================
-- BR-04: ledger append-only.
-- ============================================================
CREATE OR REPLACE FUNCTION forbid_mutation() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'tabel % bersifat append-only: % ditolak', TG_TABLE_NAME, TG_OP
        USING ERRCODE = 'insufficient_privilege';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_entries_immutable
    BEFORE UPDATE OR DELETE ON entries
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

CREATE TRIGGER trg_transactions_no_delete
    BEFORE DELETE ON transactions
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

-- ============================================================
-- K-05: transactions hanya boleh mengubah kolom status,
-- dan hanya dari POSTED ke REVERSED (tidak bisa kembali).
-- ============================================================
CREATE OR REPLACE FUNCTION transactions_status_only() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.id                      IS DISTINCT FROM OLD.id
    OR NEW.txn_type                IS DISTINCT FROM OLD.txn_type
    OR NEW.description             IS DISTINCT FROM OLD.description
    OR NEW.initiated_by_user_id    IS DISTINCT FROM OLD.initiated_by_user_id
    OR NEW.reverses_transaction_id IS DISTINCT FROM OLD.reverses_transaction_id
    OR NEW.created_at              IS DISTINCT FROM OLD.created_at THEN
        RAISE EXCEPTION 'tabel transactions hanya boleh mengubah kolom status'
            USING ERRCODE = 'insufficient_privilege';
    END IF;

    IF OLD.status = 'REVERSED' AND NEW.status <> 'REVERSED' THEN
        RAISE EXCEPTION 'transaksi % sudah REVERSED dan tidak bisa dikembalikan', OLD.id
            USING ERRCODE = 'insufficient_privilege';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_transactions_status_only
    BEFORE UPDATE ON transactions
    FOR EACH ROW EXECUTE FUNCTION transactions_status_only();
