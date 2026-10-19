package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime/debug"
	"testing"

	"tariffCalculator/store"
)

func TestHealthz(t *testing.T) {
	st, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	status := func(h http.HandlerFunc) int {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		return rec.Code
	}
	if got := status(healthz(false, nil)); got != http.StatusOK {
		t.Errorf("the calculator alone: %d", got)
	}
	if got := status(healthz(true, st)); got != http.StatusOK {
		t.Errorf("storage open: %d", got)
	}
	if got := status(healthz(true, nil)); got != http.StatusServiceUnavailable {
		t.Errorf("storage wanted but off: %d", got)
	}
	st.Close()
	if got := status(healthz(true, st)); got != http.StatusServiceUnavailable {
		t.Errorf("storage closed: %d", got)
	}
}

// The app's garbage collector defaults apply unless the environment sets
// its own.
func TestGCDefaults(t *testing.T) {
	percent, limit := debug.SetGCPercent(100), debug.SetMemoryLimit(-1)
	t.Cleanup(func() {
		debug.SetGCPercent(percent)
		debug.SetMemoryLimit(limit)
	})
	t.Setenv("GOGC", "")
	t.Setenv("GOMEMLIMIT", "")
	gcDefaults()
	if got := debug.SetGCPercent(150); got != gcPercent {
		t.Errorf("GOGC unset: %d, want %d", got, gcPercent)
	}
	if got := debug.SetMemoryLimit(1 << 30); got != memoryLimit {
		t.Errorf("GOMEMLIMIT unset: %d, want %d", got, memoryLimit)
	}
	t.Setenv("GOGC", "150")
	t.Setenv("GOMEMLIMIT", "1GiB")
	gcDefaults()
	if got := debug.SetGCPercent(150); got != 150 {
		t.Errorf("GOGC set: %d, want it left at 150", got)
	}
	if got := debug.SetMemoryLimit(-1); got != 1<<30 {
		t.Errorf("GOMEMLIMIT set: %d, want it left at 1 GiB", got)
	}
}
