package web

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// TestPanicGivesTheErrorPage: a handler that panics gives the app's
// "Something went wrong" page, logged with its pattern and not its path;
// one that had begun its answer is left as it was.
func TestPanicGivesTheErrorPage(t *testing.T) {
	var logged bytes.Buffer
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	mux := http.NewServeMux()
	mux.HandleFunc("GET /secret/{token}", func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	})
	mux.HandleFunc("GET /begun", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("half"))
		panic("boom")
	})
	h := withRecover(mux)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/secret/abc123", nil))
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "Something went wrong") {
		t.Errorf("got %d %q, want the error page at 500", rec.Code, rec.Body.String())
	}
	if l := logged.String(); !strings.Contains(l, "GET /secret/{token}") || !strings.Contains(l, "boom") || !strings.Contains(l, "goroutine") {
		t.Errorf("logged %q, want the pattern, the panic and a stack", l)
	} else if strings.Contains(l, "abc123") {
		t.Errorf("logged the secret link: %q", l)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/begun", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "half" {
		t.Errorf("begun answer: got %d %q, want it left as it was", rec.Code, rec.Body.String())
	}
}

// TestAbortHandlerPanicsOn: http.ErrAbortHandler is net/http's, so it goes on up.
func TestAbortHandlerPanicsOn(t *testing.T) {
	h := withRecover(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(http.ErrAbortHandler)
	}))
	defer func() {
		if v := recover(); v != http.ErrAbortHandler {
			t.Errorf("recovered %v, want http.ErrAbortHandler", v)
		}
	}()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}
