-- Tiga akun sistem, masing-masing tepat satu (dijaga idx_accounts_system_type).
INSERT INTO accounts (account_type, normal_balance, user_id) VALUES
    ('SYSTEM_CASH',        'DEBIT',  NULL),
    ('SYSTEM_FEE_REVENUE', 'CREDIT', NULL),
    ('SYSTEM_SUSPENSE',    'CREDIT', NULL)
ON CONFLICT DO NOTHING;
