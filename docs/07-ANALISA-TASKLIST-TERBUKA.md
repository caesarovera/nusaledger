# 07 — Analisa Tasklist & Keputusan yang Belum Selesai

Tanggal analisa: 2026-09-22
Sumber: `HANDOVER.md`, `git status`, `git log`, tag lokal & remote, `gh run list`, `go.mod`, `docs/JURNAL-BELAJAR.md` (entri terakhir: Sesi 41).

## Ringkasan posisi proyek

| Hal | Status |
|---|---|
| Fase 1 (termasuk 1.1) | Selesai total — tag `v1.0.3` |
| Fase 2 | Selesai total — tag `v1.2.0` (sudah di `origin`) |
| `main` vs `origin/main` | Sejajar di `fa6cafd` (di-push 2026-09-22) |
| CI untuk `fa6cafd` | Lulus — run `35686768475` (3m16s) |
| Perubahan lokal belum di-commit | `CLAUDE.md`, `.claude/settings.json` |
| Jurnal | Terakhir Sesi 41; perubahan 2026-09-22 belum dicatat |

---

## A. Pekerjaan yang belum selesai

### A1. Perubahan aturan push belum di-commit

Dilakukan pada 2026-09-22 atas permintaan pemilik proyek:

- `CLAUDE.md` §Larangan: "Jangan git push. Push adalah keputusan manusia." diganti menjadi push oleh Claude **hanya boleh dengan persetujuan eksplisit per kejadian** (bukan izin baku).
- `.claude/settings.json`: `"Bash(git push:*)"` dihapus dari `deny`. Perintah ini juga tidak ada di `allow`, jadi setiap `git push` memicu prompt persetujuan manual. Tidak diblokir otomatis, tidak juga disetujui otomatis.

**Kenapa dua-duanya perlu diubah:** deny rule yang tersisa akan memblokir push walaupun sudah disetujui. Sebaliknya, kalau hanya settings yang diubah, CLAUDE.md masih melarang push, sehingga aturan tertulis dan konfigurasi jadi saling bertentangan.

**Sisa pekerjaan:** tambah entri jurnal Sesi 42 (Apa / Kenapa / Contoh / Bukti), update HANDOVER, lalu commit (`chore:`).

### A2. HANDOVER.md tidak sesuai kenyataan

| Lokasi di HANDOVER | Isi sekarang | Kenyataan |
|---|---|---|
| Baris 15 (entri 2026-09-20) | "Status: belum di-commit — menunggu keputusan Anda" | Sudah di-commit (`fa6cafd`) dan di-push 2026-09-22 |
| §Berikutnya poin 1b | Perubahan Sesi 41 belum di-commit | Sama: sudah commit dan push |
| §Belum diputuskan | "Username GitHub untuk module path (asumsi: caesarovera)" | Sudah final: `go.mod` = `github.com/caesarovera/nusaledger`, remote dan push berfungsi. Pindahkan ke §Keputusan yang sudah diambil |

Ditambah: entri baru untuk perubahan aturan push (A1).

### A3. CI untuk commit terakhir — sudah beres

Saat analisa awal, run `35686768475` masih `in_progress`. Sekarang sudah **lulus**. Tidak ada tindakan lanjutan.

---

## B. Keputusan yang menunggu pemilik proyek

### B1. Arah Fase 3 (keputusan terbesar)

- Fase 1 dan Fase 2 sudah selesai. Belum ada PRD atau rencana untuk tahap berikutnya.
- Fase 2 dulu dikerjakan tanpa PRD, hanya dengan asumsi eksplisit dari docs/02 §2.9 dan docs/06 (dicatat di jurnal Sesi 34). HANDOVER meminta langkah berikutnya **ditanyakan dulu**, bukan diasumsikan lagi.
- Contoh pilihan yang mungkin (bukan rekomendasi yang sudah dikunci):
  - Fitur baru di domain ledger (misalnya jenis transaksi baru, laporan/rekonsiliasi).
  - Hardening dan operasional (misalnya deployment sungguhan, backup/restore, observability lanjutan).
  - Menyatakan proyek selesai sebagai portofolio dan fokus ke dokumentasi/presentasi.

**Yang dibutuhkan:** keputusan arah. Kalau ada fitur baru, sebaiknya ada PRD singkat sebelum menulis kode yang menyentuh uang (sesuai alur kerja CLAUDE.md: rencanakan dulu).

### B2. Pengukuran ulang k6 di Linux native

Satu-satunya item teknis lama yang masih terbuka di seluruh proyek.

| Metrik (Sesi 33, Docker Desktop Windows) | Hasil | Target | Status |
|---|---|---|---|
| Throughput | 546 tps | — | Naik ×3,1 dari 177 tps sebelum sharding |
| p95 | 210 ms | 200 ms | **Meleset 10 ms** |
| p99 | 288 ms | — | Lulus |
| Trial balance / drift | 0 | 0 | Lulus |

- **Dugaan:** selisih 10 ms berasal dari overhead Docker Desktop di Windows (lapisan VM dan jaringan), bukan dari kode. **Belum terbukti.**
- **Kendala:** mesin dev hanya punya distro WSL2 internal `docker-desktop`, tanpa distro biasa. Harus pasang distro (misalnya Ubuntu) dulu, dan itu keputusan infrastruktur di mesin pemilik.
- **Bukan blocker.** Sudah dicatat jujur di README dan `docs/evidence/k6-sharded.md`.

**Pilihan:**
1. Pasang WSL2 Ubuntu, jalankan k6 dengan `docker-compose.load.yml` di sana, lalu bandingkan hasilnya.
2. Terima selisih 10 ms sebagai keterbatasan yang sudah terdokumentasi dan tutup item ini secara resmi di HANDOVER.

### B3. Guard untuk kredensial production (sengaja ditunda)

- Aturan dari Sesi 41: kredensial production **tidak pernah** masuk `.env` repo ini atau environment sesi Claude Code.
- Kalau suatu saat butuh guard, sasarannya *connection string/host* dan berlaku untuk semua perintah, bukan hanya `psql`. Guard itu tetap lapis kedua di bawah isolasi kredensial.
- Tidak ada kode, dan itu **disengaja**: repo ini belum punya production.
- Kontrol yang sekarang benar-benar melindungi: `Read(./.env)` / `Read(./.env.*)` di `deny`, dan role DB `nusaledger_app` (`REVOKE UPDATE/DELETE` pada `entries`, ditegakkan Postgres).

**Kapan harus diputuskan:** hanya kalau proyek benar-benar akan di-deploy ke production.

---

## C. Catatan kecil (opsional, bukan blocker)

### C1. `make` dan `migrate` berjalan otomatis tanpa prompt

- `Bash(make:*)` dan `Bash(migrate:*)` ada di `allow`. Artinya `make migrate-up`, `make db-reset` (DROP SCHEMA), dan `migrate down` berjalan tanpa persetujuan.
- Temuan ini dari review Sesi 41: perintah yang paling bisa merusak data justru yang paling longgar.
- Sekarang aman karena hanya ada DB dev dan kredensialnya bukan rahasia.
- **Opsi pengetatan:** tambahkan `Bash(make db-reset:*)`, `Bash(make migrate-down:*)`, dan `Bash(migrate:*)` ke `ask`, lalu hapus `Bash(migrate:*)` dari `allow`. `make test`/`make lint` tetap otomatis.

### C2. Peringatan "Node.js 20 deprecated" di GitHub Actions

Hanya anotasi, bukan kegagalan. Runner otomatis memakai Node 24. Baru perlu ditindaklanjuti kalau `actions/checkout` atau `actions/setup-go` merilis versi yang mengharuskan update.

---

## Urutan yang disarankan

1. **A1 + A2:** jurnal Sesi 42, rapikan HANDOVER, commit. Push hanya dengan persetujuan.
2. **B2:** putuskan WSL2 atau terima selisih 10 ms. Dua-duanya menutup item terakhir yang tersisa.
3. **B1:** tentukan arah Fase 3 sebelum ada kode baru.
4. **C1:** opsional, bisa digabung dengan langkah 1 kalau disetujui.
5. **B3:** tunda sampai ada rencana production.
