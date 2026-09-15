DROP TRIGGER  IF EXISTS trg_transactions_status_only ON transactions;
DROP FUNCTION IF EXISTS transactions_status_only();

DROP TRIGGER  IF EXISTS trg_transactions_no_delete ON transactions;
DROP TRIGGER  IF EXISTS trg_entries_immutable      ON entries;
DROP FUNCTION IF EXISTS forbid_mutation();

DROP TRIGGER  IF EXISTS trg_entries_balanced ON entries;
DROP FUNCTION IF EXISTS assert_transaction_balanced();
