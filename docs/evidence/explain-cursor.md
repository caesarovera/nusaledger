# Bukti EXPLAIN — cursor pagination mutasi rekening (Sesi 14, 2026-09-16)

Data: 100 dompet, 100.000 transaksi, 200.000 entries (generate_series, lalu ANALYZE). Query docs/02 §3.3 dengan `account_id = 50`, cursor `id < 150000`, `LIMIT 20`.

```
                                                                        QUERY PLAN                                                                        
----------------------------------------------------------------------------------------------------------------------------------------------------------
 Limit  (cost=0.84..152.01 rows=20 width=61) (actual time=0.096..0.353 rows=20 loops=1)
   Buffers: shared hit=106
   ->  Nested Loop  (cost=0.84..5616.84 rows=743 width=61) (actual time=0.094..0.349 rows=20 loops=1)
         Buffers: shared hit=106
         ->  Index Scan using idx_entries_account_id_desc on entries e  (cost=0.42..1489.63 rows=743 width=52) (actual time=0.068..0.103 rows=20 loops=1)
               Index Cond: ((account_id = 50) AND (id < 150000))
               Buffers: shared hit=26
         ->  Index Scan using transactions_pkey on transactions t  (cost=0.42..5.55 rows=1 width=25) (actual time=0.012..0.012 rows=1 loops=20)
               Index Cond: (id = e.transaction_id)
               Buffers: shared hit=80
 Planning:
   Buffers: shared hit=369
 Planning Time: 2.906 ms
 Execution Time: 0.489 ms
(14 rows)

```

Halaman pertama (cursor NULL):

```
                                                                       QUERY PLAN                                                                       
--------------------------------------------------------------------------------------------------------------------------------------------------------
 Limit  (cost=0.42..38.75 rows=20 width=8) (actual time=0.125..0.165 rows=20 loops=1)
   Buffers: shared hit=26
   ->  Index Only Scan using idx_entries_account_id_desc on entries e  (cost=0.42..1893.70 rows=988 width=8) (actual time=0.124..0.163 rows=20 loops=1)
         Index Cond: (account_id = 50)
         Heap Fetches: 20
         Buffers: shared hit=26
 Planning:
   Buffers: shared hit=136
 Planning Time: 1.020 ms
 Execution Time: 0.201 ms
(10 rows)

```

Pembanding yang DITOLAK — OFFSET memaksa membaca lalu membuang baris (akun 1 = SYSTEM_CASH, 100.000 entry):

```
                                                                     QUERY PLAN                                                                      
-----------------------------------------------------------------------------------------------------------------------------------------------------
 Limit  (cost=7163.30..7164.89 rows=20 width=8) (actual time=24.408..24.412 rows=20 loops=1)
   Buffers: shared hit=3201
   ->  Index Scan Backward using entries_pkey on entries e  (cost=0.42..7980.42 rows=100267 width=8) (actual time=10.216..21.996 rows=90020 loops=1)
         Filter: (account_id = 1)
         Rows Removed by Filter: 100000
         Buffers: shared hit=3201
 Planning:
   Buffers: shared hit=136
 Planning Time: 1.029 ms
 Execution Time: 24.453 ms
(10 rows)

```

Trial balance pada data bulk:

```
  debit   |  credit  | difference 
----------+----------+------------
 10000000 | 10000000 |          0
(1 row)

```
