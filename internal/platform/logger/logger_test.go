package logger_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/caesarovera/nusaledger/internal/platform/logger"
)

func TestRedaksiFieldSensitif(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(&buf, "info")

	log.Info("login", "email", "andi@x.com", "password", "rahasia123", "Authorization", "Bearer abc")
	log.With("group", "auth").Info("refresh", "refresh_token", "tok-xyz")

	out := buf.String()
	for _, leak := range []string{"rahasia123", "Bearer abc", "tok-xyz"} {
		if strings.Contains(out, leak) {
			t.Errorf("nilai sensitif %q bocor ke log: %s", leak, out)
		}
	}
	if !strings.Contains(out, "[REDACTED]") || !strings.Contains(out, "andi@x.com") {
		t.Errorf("redaksi salah sasaran: %s", out)
	}

	// harus JSON valid per baris
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Errorf("baris log bukan JSON: %s", line)
		}
	}
}

func TestLevel(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(&buf, "warn")
	log.Info("tidak tampil")
	log.Warn("tampil")
	if strings.Contains(buf.String(), "tidak tampil") || !strings.Contains(buf.String(), "tampil") {
		t.Errorf("filter level salah: %s", buf.String())
	}
}
