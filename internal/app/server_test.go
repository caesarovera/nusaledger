package app_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/caesarovera/nusaledger/internal/app"
)

// T-13: request yang sedang berjalan saat sinyal berhenti tetap selesai; request baru ditolak.
func TestServe_GracefulShutdown(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/slow", func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(1500 * time.Millisecond)
		_, _ = w.Write([]byte("selesai"))
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var stopped atomic.Bool
	done := make(chan error, 1)
	go func() {
		done <- app.Serve(ctx, srv, ln, 10*time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)), func() { stopped.Store(true) })
	}()
	get := func() (*http.Response, error) {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+ln.Addr().String()+"/slow", nil)
		return http.DefaultClient.Do(req)
	}

	// request lambat dimulai...
	type result struct {
		body string
		err  error
	}
	resCh := make(chan result, 1)
	go func() {
		resp, err := get()
		if err != nil {
			resCh <- result{err: err}
			return
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		resCh <- result{body: string(b)}
	}()

	// ...lalu sinyal berhenti datang di tengah jalan
	time.Sleep(300 * time.Millisecond)
	cancel()

	res := <-resCh
	if res.err != nil || res.body != "selesai" {
		t.Fatalf("request yang sedang jalan harus selesai: body=%q err=%v", res.body, res.err)
	}
	if err := <-done; err != nil {
		t.Fatalf("Serve harus kembali tanpa error: %v", err)
	}
	if !stopped.Load() {
		t.Fatal("onStop (readiness=false) harus dipanggil")
	}
	if resp, err := get(); err == nil {
		_ = resp.Body.Close()
		t.Fatal("setelah shutdown, koneksi baru harus ditolak")
	}
}

func TestRunPeriodic_BerhentiSaatCtxSelesai(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var runs atomic.Int32
	done := make(chan struct{})
	go func() {
		app.RunPeriodic(ctx, 20*time.Millisecond, slog.New(slog.NewTextHandler(io.Discard, nil)), "uji", func(context.Context) error {
			runs.Add(1)
			return nil
		})
		close(done)
	}()
	time.Sleep(120 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("RunPeriodic tidak berhenti setelah ctx dibatalkan")
	}
	if runs.Load() < 3 {
		t.Fatalf("job harus jalan berkali-kali, dapat %d", runs.Load())
	}
}
