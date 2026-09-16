package ratelimit

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// incrScript menaikkan hitungan lalu memasang TTL HANYA pada percobaan pertama (n==1).
// Dua langkah ini WAJIB atomik: kalau INCR dan PEXPIRE dua panggilan Redis terpisah,
// proses bisa mati atau koneksi putus di antara keduanya, meninggalkan key tanpa TTL
// yang membuatnya menghitung SELAMANYA (bocor memori Redis, dan key itu tidak akan
// pernah "reset" jendelanya lagi). EVAL menjalankan skrip ini sebagai satu unit atomik
// di sisi server Redis — pola standar untuk fixed-window counter.
const incrScript = `
local n = redis.call("INCR", KEYS[1])
if n == 1 then
	redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
return n
`

// RedisLimiter adalah pembatas laju fixed-window yang hitungannya dibagi lewat Redis —
// beda dari Limiter (memory.go) yang hitungannya per-PROSES. Dipakai saat cmd/api
// diskalakan lebih dari satu instance: tanpa penyimpanan bersama, tiap instance punya
// jendela hitungannya sendiri dan batas gabungan jadi (limit × jumlah instance), bukan
// limit yang sebenarnya diniatkan.
type RedisLimiter struct {
	client  *redis.Client
	limit   int
	window  time.Duration
	prefix  string
	timeout time.Duration
}

func NewRedis(client *redis.Client, limit int, window time.Duration) *RedisLimiter {
	return &RedisLimiter{client: client, limit: limit, window: window, prefix: "ratelimit:", timeout: 500 * time.Millisecond}
}

// Allow menaikkan hitungan key di Redis dan mengembalikan apakah masih di bawah batas.
// Fail-closed (docs/03 §6, "lebih baik menolak daripada membuka pintu"): key kosong
// ATAU Redis yang error/timeout/tidak terjangkau SELALU ditolak, bukan diloloskan diam-diam.
// Timeout pendek (500ms, bukan mengikuti context permintaan HTTP) memastikan satu Redis
// yang lambat tidak ikut membuat SETIAP request HTTP lambat menunggunya.
func (l *RedisLimiter) Allow(key string) bool {
	if key == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), l.timeout)
	defer cancel()

	n, err := l.client.Eval(ctx, incrScript, []string{l.prefix + key}, l.window.Milliseconds()).Int64()
	if err != nil {
		return false
	}
	return n <= int64(l.limit)
}
