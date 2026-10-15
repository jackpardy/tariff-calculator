package main

import (
	"context"
	"net/http"
	"net/http/httptest"
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
