package ratelimit

import (
	"sync"
	"testing"
	"time"
)

func TestLimiter_JendelaTetap(t *testing.T) {
	now := time.Now()
	l := New(3, time.Minute)
	l.now = func() time.Time { return now }

	for i := 1; i <= 3; i++ {
		if !l.Allow("andi") {
			t.Fatalf("percobaan %d harus diizinkan", i)
		}
	}
	if l.Allow("andi") {
		t.Fatal("percobaan ke-4 harus ditolak")
	}
	if !l.Allow("budi") {
		t.Fatal("key lain tidak terpengaruh")
	}
	now = now.Add(61 * time.Second)
	if !l.Allow("andi") {
		t.Fatal("setelah jendela lewat harus diizinkan lagi")
	}
	if l.Allow("") {
		t.Fatal("key kosong harus ditolak (fail-closed)")
	}
}

func TestLimiter_AmanUntukKonkurensi(t *testing.T) {
	l := New(100, time.Minute)
	var wg sync.WaitGroup
	allowed := make(chan bool, 300)
	for i := 0; i < 300; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); allowed <- l.Allow("k") }()
	}
	wg.Wait()
	close(allowed)
	n := 0
	for ok := range allowed {
		if ok {
			n++
		}
	}
	if n != 100 {
		t.Fatalf("tepat 100 harus lolos, dapat %d", n)
	}
}
