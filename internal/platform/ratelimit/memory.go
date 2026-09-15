// Package ratelimit menyediakan pembatas laju in-memory (Fase 1, satu instance — docs/06 F-04).
// Fase 2 mengganti implementasi ini dengan Redis di balik interface yang sama.
package ratelimit

import (
	"sync"
	"time"
)

// Limiter membatasi N kejadian per jendela waktu tetap untuk setiap key.
type Limiter struct {
	limit  int
	window time.Duration
	now    func() time.Time

	mu      sync.Mutex
	buckets map[string]*bucket
	lastGC  time.Time
}

type bucket struct {
	count   int
	resetAt time.Time
}

func New(limit int, window time.Duration) *Limiter {
	return &Limiter{limit: limit, window: window, now: time.Now, buckets: map[string]*bucket{}}
}

// Allow mencatat satu kejadian untuk key dan mengembalikan apakah masih di bawah batas.
// Fail-closed: key kosong selalu ditolak (lebih baik menolak daripada membuka pintu).
func (l *Limiter) Allow(key string) bool {
	if key == "" {
		return false
	}
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	if now.Sub(l.lastGC) > l.window {
		l.gc(now)
	}
	b, ok := l.buckets[key]
	if !ok || now.After(b.resetAt) {
		l.buckets[key] = &bucket{count: 1, resetAt: now.Add(l.window)}
		return l.limit >= 1
	}
	b.count++
	return b.count <= l.limit
}

// gc membuang bucket yang sudah lewat jendelanya agar memori tidak tumbuh tanpa batas.
func (l *Limiter) gc(now time.Time) {
	for k, b := range l.buckets {
		if now.After(b.resetAt) {
			delete(l.buckets, k)
		}
	}
	l.lastGC = now
}
