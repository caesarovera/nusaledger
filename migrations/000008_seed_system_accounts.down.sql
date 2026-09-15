-- Gagal (dengan sengaja) bila akun sistem sudah punya entries: down tidak boleh menghapus jejak uang.
DELETE FROM accounts WHERE user_id IS NULL;
