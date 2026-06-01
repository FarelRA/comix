package logger

import (
	"log/slog"
	"testing"
)

func TestConfigure(t *testing.T) {
	Configure("debug", "text", false)
	if slog.Default() == nil {
		t.Fatal("expected non-nil default logger")
	}

	Configure("error", "json", true)
	if slog.Default() == nil {
		t.Fatal("expected non-nil default logger after reconfigure")
	}
}
