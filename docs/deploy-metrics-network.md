# Membatasi `/metrics` di jaringan (Fase 2, sisa item terakhir)

## Kenapa ini BUKAN perubahan kode

`/metrics` (format Prometheus, `internal/platform/metrics`) sengaja **tanpa autentikasi**
di level aplikasi. Menambah cek token/JWT di endpoint ini akan merusak model *scraping*
Prometheus standar — server Prometheus melakukan GET polling berkala tanpa kredensial,
dan menambah auth berarti menyimpan/mengedarkan kredensial scraper yang justru menambah
permukaan serangan baru untuk masalah yang sudah punya solusi yang lebih baik: **jaringan**.

Prinsip yang sama sudah dipakai untuk `/healthz` dan `/readyz` — endpoint operasional
dipisahkan secara SENGAJA dari model otorisasi endpoint bisnis (`/api/v1/...`), dan
dilindungi lewat lapis yang tepat untuk jenis ancamannya: `/metrics` membocorkan
INFORMASI (nama endpoint, tingkat trafik, distribusi latensi) — bukan KENDALI atas
data — jadi lapis yang tepat adalah "siapa yang boleh menjangkau port ini", bukan
"siapa yang punya token yang valid".

## Opsi 1 — reverse proxy (produksi sungguhan)

Container `api` HANYA expose port aplikasi (mis. `8080`) ke reverse proxy; reverse
proxy itulah yang expose ke publik, dan HANYA reverse proxy yang boleh menjangkau
`/metrics` (server Prometheus scrape lewat reverse proxy, atau lewat jaringan
internal terpisah — pilih salah satu, jangan dua-duanya terbuka).

**Nginx** — blokir `/metrics` dari luar CIDR internal:

```nginx
location /metrics {
    allow 10.0.0.0/8;      # ganti dengan CIDR jaringan internal/VPC Anda
    allow 172.16.0.0/12;   # rentang default Docker bridge network
    deny all;
    proxy_pass http://api:8080;
}

location / {
    proxy_pass http://api:8080;
}
```

**Caddy** — pola yang sama, sintaks berbeda:

```caddyfile
handle /metrics {
    @internal remote_ip 10.0.0.0/8 172.16.0.0/12
    handle @internal {
        reverse_proxy api:8080
    }
    respond 404
}

handle {
    reverse_proxy api:8080
}
```

## Opsi 2 — jaringan terpisah di orkestrator (lebih kuat dari CIDR di proxy)

Kalau orkestrator (Kubernetes, ECS, dst.) mendukung network policy, itu lebih kuat
daripada mengandalkan proxy: `NetworkPolicy` Kubernetes yang hanya mengizinkan
namespace `monitoring` (tempat Prometheus berjalan) menjangkau port `8080` pod `api`
menutup jalur ini di level jaringan itu sendiri, sebelum request sempat sampai ke
proxy atau aplikasi sama sekali — tidak ada konfigurasi yang bisa "lupa" ditambahkan
di setiap proxy baru, karena aturannya menempel pada Pod, bukan pada instance proxy.

## Dev/portofolio (repo ini)

`docker-compose.yml` sudah membatasi SEMUA port (`api`, `postgres`, `rabbitmq`, `redis`)
ke `127.0.0.1` (temuan audit F-2, lihat README §Audit Keamanan) — jadi di lingkungan
dev repo ini, `/metrics` sudah TIDAK terjangkau dari LAN/Wi-Fi sama sekali, hanya dari
mesin yang sama. Dokumen ini menunjukkan langkah TAMBAHAN yang benar untuk deploy
produksi sungguhan (banyak instance di belakang load balancer, cluster orkestrator),
bukan untuk repo portofolio ini sendiri.
