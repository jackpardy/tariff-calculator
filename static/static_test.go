package static

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func get(t *testing.T, url string, header ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, req)
	return rec
}

func TestHandlerCaching(t *testing.T) {
	versioned := URL("js/htmx.min.js")

	rec := get(t, versioned)
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Errorf("versioned: status %d, Cache-Control %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/javascript") {
		t.Errorf("Content-Type = %q, want JavaScript", ct)
	}

	stale := Prefix + "js/htmx.min.js?v=old"
	if rec := get(t, stale); rec.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("an old or missing version must be revalidated, got %q", rec.Header().Get("Cache-Control"))
	}

	etag := rec.Header().Get("ETag")
	if rec := get(t, Prefix+"js/htmx.min.js", "If-None-Match", etag); rec.Code != http.StatusNotModified {
		t.Errorf("revalidating with the current ETag: status %d, want 304", rec.Code)
	}
}

func TestHandlerOnlyServesAssets(t *testing.T) {
	for _, path := range []string{"js/", "js/nope.js", "static.go", "../main.go"} {
		if rec := get(t, Prefix+path); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: status %d, want 404", path, rec.Code)
		}
	}
}

func TestURLPanicsForMissingAssets(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Errorf("URL of a missing asset should panic")
		}
	}()
	URL("js/missing.js")
}
