package web

import (
	"bytes"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// An unexpected error is logged by its route, never by its path, which
// holds the secret link.
func TestFailedLogsTheRouteNotTheLink(t *testing.T) {
	var logged bytes.Buffer
	was := log.Writer()
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(was) })

	mux := http.NewServeMux()
	mux.HandleFunc("GET /clubs/member/{token}", func(w http.ResponseWriter, r *http.Request) {
		failed(w, r, errors.New("the disk is on fire"))
	})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/clubs/member/s3cr3t-t0ken", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("a plain error page: %d", rec.Code)
	}
	if got := logged.String(); !strings.Contains(got, "GET /clubs/member/{token}: the disk is on fire") || strings.Contains(got, "s3cr3t-t0ken") {
		t.Errorf("logged: %q", got)
	}
}
