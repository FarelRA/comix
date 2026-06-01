package logger

import (
	"testing"
)

func TestDefault(t *testing.T) {
	l := Default()
	if l == nil {
		t.Fatal("expected non-nil default logger")
	}
}

func TestConfigure_Levels(t *testing.T) {
	tests := []struct {
		name    string
		level   string
		format  string
		verbose bool
	}{
		{"debug text", "debug", "text", false},
		{"info text", "info", "text", false},
		{"warn text", "warn", "text", false},
		{"error text", "error", "text", false},
		{"debug json", "debug", "json", false},
		{"info json", "info", "json", false},
		{"verbose overrides", "info", "text", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Configure(tt.level, tt.format, tt.verbose)
			l := Default()
			if l == nil {
				t.Error("logger should not be nil after Configure")
			}
		})
	}
}

func TestLogFunctions(t *testing.T) {
	Configure("debug", "text", false)

	Debug("debug message", "key", "value")
	Info("info message", "key", "value")
	Warn("warn message", "key", "value")
	Error("error message", "key", "value")
}

func TestLogFunctionsNoArgs(t *testing.T) {
	Debug("debug message")
	Info("info message")
	Warn("warn message")
	Error("error message")
}

func TestWith(t *testing.T) {
	l := With("component", "test")
	if l == nil {
		t.Error("expected non-nil logger from With")
	}
}

func TestWriter(t *testing.T) {
	w := Writer()
	if w == nil {
		t.Error("expected non-nil writer")
	}
}
