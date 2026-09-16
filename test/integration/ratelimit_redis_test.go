//go:build integration

package integration

import (
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/caesarovera/nusaledger/internal/platform/ratelimit"
)

// setupRedis menyalakan Redis sungguhan untuk SATU test — pola sama dengan setupRabbit
// (outbox_test.go): hanya test di file ini yang butuh Redis, tidak sepadan membebani
// TestMain (dipakai semua test) dengan container tambahan.
func setupRedis(t *testing.T) *redis.Client {
	t.Helper()
	ctx := testCtx(t)
	ctr, err := tcredis.Run(ctx, "redis:7-alpine")
	if err != nil {
		t.Fatalf("menjalankan redis: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(testCtx(t)) })

	uri, err := ctr.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("connection string redis: %v", err)
	}
	opts, err := redis.ParseURL(uri)
	if err != nil {
		t.Fatalf("parse url redis: %v", err)
	}
	client := redis.NewClient(opts)
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("ping redis: %v", err)
	}
	return client
}

// Sengaja BUKAN mengulang TestLimiter_JendelaTetap (memory_test.go) dengan clock palsu —
// RedisLimiter tidak punya field `now` yang bisa disuntik (TTL-nya milik Redis, bukan
// proses Go), jadi jendela waktu di sini memakai waktu ASLI (jendela pendek + sleep
// sungguhan), bukti bahwa PEXPIRE yang dipasang skrip Lua benar-benar berlaku, bukan
// hanya lolos test karena clock ditipu.
func TestRedisLimiter_JendelaTetap(t *testing.T) {
	client := setupRedis(t)
	l := ratelimit.NewRedis(client, 3, 500*time.Millisecond)

	for i := 1; i <= 3; i++ {
		if !l.Allow("andi") {
			t.Fatalf("percobaan %d harus diizinkan", i)
		}
	}
	if l.Allow("andi") {
		t.Fatal("percobaan ke-4 harus ditolak")
	}
	if !l.Allow("budi") {
		t.Fatal("key lain (hitungan Redis terpisah per key) tidak boleh terpengaruh")
	}

	time.Sleep(600 * time.Millisecond) // jendela 500ms harus sudah lewat (PEXPIRE dari skrip Lua)
	if !l.Allow("andi") {
		t.Fatal("setelah TTL lewat, key harus ter-reset dan diizinkan lagi")
	}
	if l.Allow("") {
		t.Fatal("key kosong harus ditolak (fail-closed)")
	}
}

// Membuktikan hitungan BENAR-BENAR dibagi lewat Redis (bukan per-proses seperti Limiter
// in-memory): dua instance RedisLimiter Go yang BERBEDA, menyambung ke Redis yang SAMA,
// harus melihat hitungan yang sama — ini skenario nyata dua instance cmd/api di belakang
// load balancer.
func TestRedisLimiter_DibagiLintasInstance(t *testing.T) {
	client := setupRedis(t)
	instanceA := ratelimit.NewRedis(client, 2, time.Minute)
	instanceB := ratelimit.NewRedis(client, 2, time.Minute)

	if !instanceA.Allow("citra") {
		t.Fatal("percobaan 1 (instance A) harus diizinkan")
	}
	if !instanceB.Allow("citra") {
		t.Fatal("percobaan 2 (instance B) harus diizinkan — batas belum tercapai")
	}
	if instanceA.Allow("citra") {
		t.Fatal("percobaan 3 (instance A) harus ditolak — instance B sudah memakai kuota yang SAMA")
	}
}

// Fail-closed (docs/03 §6): Redis yang tidak terjangkau harus DITOLAK, bukan diloloskan.
// Kalau ini fail-open, satu Redis yang mati berarti SEMUA rate limit hilang serentak —
// kebalikan dari tujuan rate limit itu sendiri.
func TestRedisLimiter_FailClosedSaatRedisMati(t *testing.T) {
	unreachable := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 200 * time.Millisecond})
	t.Cleanup(func() { _ = unreachable.Close() })
	l := ratelimit.NewRedis(unreachable, 100, time.Minute)

	if l.Allow("siapa-saja") {
		t.Fatal("Redis tidak terjangkau harus fail-closed (tolak), bukan meloloskan request")
	}
}
