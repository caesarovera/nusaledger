// Package app berisi perakitan proses: server HTTP dengan graceful shutdown dan job latar.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Serve menjalankan srv di atas ln sampai ctx dibatalkan, lalu berhenti dengan rapi:
// onStop dipanggil dulu (mis. readiness=false), request yang sedang jalan diselesaikan
// paling lama shutdownWait, baru kembali. Ini yang membuat deploy siang hari aman.
func Serve(ctx context.Context, srv *http.Server, ln net.Listener, shutdownWait time.Duration, log *slog.Logger, onStop func()) error {
	errCh := make(chan error, 1)
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()
	log.Info("server berjalan", "addr", ln.Addr().String())

	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("server berhenti tidak wajar: %w", err)
		}
		return nil
	case <-ctx.Done():
	}

	log.Info("sinyal berhenti diterima, menolak request baru", "tunggu_maks", shutdownWait.String())
	if onStop != nil {
		onStop()
	}
	// ctx induk sudah selesai; shutdown butuh context BARU dengan batas waktunya sendiri.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownWait)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil { //nolint:contextcheck // sengaja tidak mewarisi ctx yang sudah selesai
		return fmt.Errorf("shutdown: %w", err)
	}
	log.Info("shutdown selesai")
	return nil
}

// RunPeriodic menjalankan fn setiap interval sampai ctx selesai. Dimiliki pemanggil lewat ctx.
func RunPeriodic(ctx context.Context, interval time.Duration, log *slog.Logger, name string, fn func(ctx context.Context) error) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			runCtx, cancel := context.WithTimeout(ctx, interval)
			if err := fn(runCtx); err != nil && !errors.Is(err, context.Canceled) {
				log.Error("job gagal", "job", name, "err", err)
			}
			cancel()
		}
	}
}
