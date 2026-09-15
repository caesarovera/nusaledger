// Package logger membangun slog JSON dengan redaksi terpusat untuk field sensitif.
package logger

import (
	"io"
	"log/slog"
	"strings"
)

// sensitiveKeys tidak pernah masuk log, apa pun lapisannya.
var sensitiveKeys = map[string]struct{}{
	"password":      {},
	"password_hash": {},
	"token":         {},
	"access_token":  {},
	"refresh_token": {},
	"authorization": {},
	"jwt_secret":    {},
	"secret":        {},
}

// New membuat logger JSON. level: debug | info | warn | error.
func New(w io.Writer, level string) *slog.Logger {
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:       parseLevel(level),
		ReplaceAttr: redact,
	})
	return slog.New(h)
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// redact mengganti nilai field sensitif dengan "[REDACTED]" di semua level grup.
func redact(_ []string, a slog.Attr) slog.Attr {
	if _, ok := sensitiveKeys[strings.ToLower(a.Key)]; ok {
		return slog.String(a.Key, "[REDACTED]")
	}
	return a
}
