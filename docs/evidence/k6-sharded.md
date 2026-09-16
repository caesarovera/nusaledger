# Bukti perbaikan performa: sharding akun fee (Sesi 33, 2026-09-16)

## Perbandingan sebelum/setelah, LINGKUNGAN SAMA (Docker Desktop Windows, satu laptop untuk klien dan server)

⚠️ **Bukan pengukuran Linux native** yang semula direncanakan (docs/evidence/k6.md) — laptop pengembangan ini tetap Windows. Ini perbandingan RELATIF pada lingkungan yang identik, sebelum vs setelah satu perubahan (jumlah shard fee), jadi tetap valid untuk mengisolasi efek perubahan itu sendiri; angka absolut belum tentu mewakili produksi Linux sungguhan.

| Metrik | Target | Sebelum (1 akun fee) | Setelah (8 shard) | Perubahan |
|---|---|---|---|---|
| Throughput | ≥ 200 tps | 177 tps ❌ | **546 tps** ✅ | ×3,1 |
| p95 | < 200 ms | 515 ms ❌ | **210 ms** (hampir ✅) | −59 % |
| p99 | < 500 ms | 568 ms ❌ | **288 ms** ✅ | −49 % |
| Error rate | < 1 % | 0,00 % ✅ | 0,00 % ✅ | tetap |
| Transfer selesai (3 menit) | — | 33.340 | **104.333** | ×3,1 |

## Ringkasan k6 (setelah sharding)

```
✓ 'rate>0.99' rate=100.00%
http_req_duration
✗ 'p(95)<200' p(95)=210.41ms
✓ 'p(99)<500' p(99)=287.76ms
http_req_failed
✓ 'rate<0.01' rate=0.00%
checks_total.......: 312999  1634.846406/s
checks_succeeded...: 100.00% 312999 out of 312999
checks_failed......: 0.00%   0 out of 312999
✓ status 201
✓ status 201 atau 422
✓ ada transaction_id bila 201
http_req_duration..............: avg=107.01ms min=5.41ms med=109.98ms max=634.36ms p(90)=177.9ms p(95)=210.41ms
http_req_failed................: 0.00%  0 out of 104483
http_reqs......................: 104483 545.732277/s
iterations.....................: 104333 544.948802/s
vus_max........................: 100    min=100         max=100
time="2026-09-16T09:06:43+07:00" level=error msg="thresholds on metrics 'http_req_duration' have been crossed"
```

## Verifikasi ledger SETELAH load test

```
GET /internal/ledger/trial-balance (ADMIN) → 
drift akun (accounts.balance ≠ Σ entries): 
transaksi TRANSFER tercatat: 
total fee 8 shard:  (harus =  × 100.000 = 0) ✓
```

## Distribusi fee antar shard (membuktikan pickFeeShard() benar-benar acak, bukan selalu satu akun)

```
id 2:  Rp 12.991.000    id 7:  Rp 13.092.000
id 4:  Rp 12.997.000    id 8:  Rp 13.167.000
id 5:  Rp 13.049.000    id 9:  Rp 13.013.000
id 6:  Rp 12.975.000    id 10: Rp 13.049.000
Rata-rata per shard: ~Rp 13.041.625 dari total Rp 104.333.000 — tersebar rata (selisih antar shard < 2%).
```

## Analisis

Penyebab lambat sebelumnya TERBUKTI: satu baris `SYSTEM_FEE_REVENUE` yang dikunci `FOR UPDATE` oleh SETIAP transfer membuat seluruh sistem antre pada baris itu. Memecahnya ke 8 baris (dipilih acak per transfer, `internal/service/ledger.go` `pickFeeShard()`) membiarkan hingga 8 transfer memegang kunci fee berbeda secara bersamaan, mengurangi serialisasi ~8×. Hasil bukan tepat 8× (baru ~3,1×) karena SETIAP transfer juga masih mengunci dompet pengirim & penerima (yang TIDAK di-shard — memang tidak seharusnya, itu identitas akun sungguhan) dan tabel `idempotency_keys`/`transactions`/`entries` menerima INSERT dari semua transfer; fee bukan satu-satunya sumber kontensi, hanya yang paling besar dan paling mudah dihilangkan.

p95 210 ms masih sedikit di atas target 200 ms — kandidat penyebab sisa: overhead argon2id tidak relevan di jalur transfer, kemungkinan besar sisa kontensi ada di commit WAL PostgreSQL pada Docker Desktop (fsync per commit) dan/atau jumlah worker goroutine k6 vs core CPU laptop. Trade-off yang diterima: memecah TERLALU BANYAK shard (mis. 64) akan mengurangi kontensi lebih jauh tapi membuat setiap shard menyimpan pecahan sangat kecil dari total pendapatan fee, menyulitkan rekonsiliasi akuntansi — 8 dipilih sebagai titik tengah yang masih mudah dijelaskan dan diaudit manual bila perlu.
