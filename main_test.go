package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime/debug"
	"testing"
	"time"

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

// TestServeLetsRequestsFinish: a request still running when the stop comes
// gets its whole answer, and serve returns (and the store may be closed)
// only after it has.
func TestServeLetsRequestsFinish(t *testing.T) {
	started, finished := make(chan struct{}), make(chan struct{})
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		time.Sleep(500 * time.Millisecond)
		io.WriteString(w, "all of it")
		close(finished)
	})
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	served := make(chan error)
	go func() { served <- serve(&http.Server{Handler: h}, l, stop, 15*time.Second) }()

	answer := make(chan string)
	go func() {
		resp, err := http.Get("http://" + l.Addr().String() + "/")
		if err != nil {
			answer <- "error: " + err.Error()
			return
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		answer <- string(b)
	}()
	<-started
	close(stop)
	if err := <-served; err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	default:
		t.Fatal("serve returned with the request still running")
	}
	if got := <-answer; got != "all of it" {
		t.Errorf("answer %q, want all of it", got)
	}
}

// TestServeWaitsOnlySoLong: a request that doesn't finish holds up the
// stop no longer than the wait.
func TestServeWaitsOnlySoLong(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
	})
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	served := make(chan error)
	go func() { served <- serve(&http.Server{Handler: h}, l, stop, 200*time.Millisecond) }()
	go http.Get("http://" + l.Addr().String() + "/")
	<-started
	close(stop)
	select {
	case err := <-served:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve waited past its limit")
	}
}
