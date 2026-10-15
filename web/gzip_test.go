package web

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tariffCalculator/static"
)

// gzipGet sends a request through withGzip to a handler that answers with
// the given type, status and body.
func gzipGet(t *testing.T, method, acceptEncoding, contentType string, status int, body string, mutate ...func(http.Header)) *httptest.ResponseRecorder {
	t.Helper()
	h := withGzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		for _, m := range mutate {
			m(w.Header())
		}
		w.WriteHeader(status)
		io.WriteString(w, body)
	}))
	req := httptest.NewRequest(method, "/x", nil)
	if acceptEncoding != "" {
		req.Header.Set("Accept-Encoding", acceptEncoding)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func gunzip(t *testing.T, b []byte) string {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("not gzip: %v", err)
	}
	out, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestGzipCompressesTextWhenAsked(t *testing.T) {
	page := strings.Repeat("<p>Hello, gymnasts</p>\n", 500)
	rec := gzipGet(t, http.MethodGet, "gzip, br", "text/html; charset=utf-8", 200, page, func(h http.Header) {
		h.Set("Content-Length", "11500")
		h.Set("ETag", `"abc"`)
	})
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", rec.Header().Get("Content-Encoding"))
	}
	if rec.Header().Get("Vary") != "Accept-Encoding" {
		t.Errorf("Vary = %q", rec.Header().Get("Vary"))
	}
	if rec.Header().Get("Content-Length") != "" {
		t.Errorf("Content-Length = %q, want none", rec.Header().Get("Content-Length"))
	}
	if rec.Header().Get("ETag") != `W/"abc"` {
		t.Errorf("ETag = %q, want it made weak", rec.Header().Get("ETag"))
	}
	if got := gunzip(t, rec.Body.Bytes()); got != page {
		t.Errorf("decompressed page differs from the plain one")
	}
	if rec.Body.Len() >= len(page)/5 {
		t.Errorf("body is %d bytes for %d plain: not compressed well", rec.Body.Len(), len(page))
	}
}

func TestGzipTypes(t *testing.T) {
	big := strings.Repeat("abcdefghij", 300)
	for _, ct := range []string{"text/html; charset=utf-8", "text/css", "text/javascript; charset=utf-8", "application/json",
		"image/svg+xml", "text/plain; charset=utf-8", "text/calendar; charset=utf-8", "application/manifest+json", "text/csv; charset=utf-8"} {
		if rec := gzipGet(t, http.MethodGet, "gzip", ct, 200, big); rec.Header().Get("Content-Encoding") != "gzip" {
			t.Errorf("%s: not compressed", ct)
		}
	}
	for _, ct := range []string{"image/png", "application/pdf", "image/jpeg", "application/octet-stream"} {
		rec := gzipGet(t, http.MethodGet, "gzip", ct, 200, big)
		if rec.Header().Get("Content-Encoding") != "" || rec.Body.String() != big {
			t.Errorf("%s: should be left alone", ct)
		}
		if rec.Header().Get("Vary") != "" {
			t.Errorf("%s: Vary = %q, want none for a binary type", ct, rec.Header().Get("Vary"))
		}
	}
}

func TestGzipLeavesOtherResponsesAlone(t *testing.T) {
	big := strings.Repeat("abcdefghij", 300)
	cases := []struct {
		name   string
		rec    *httptest.ResponseRecorder
		status int
		body   string
	}{
		{"not asked", gzipGet(t, http.MethodGet, "", "text/html", 200, big), 200, big},
		{"asked for br only", gzipGet(t, http.MethodGet, "br, deflate", "text/html", 200, big), 200, big},
		{"gzip refused", gzipGet(t, http.MethodGet, "gzip;q=0", "text/html", 200, big), 200, big},
		{"small", gzipGet(t, http.MethodGet, "gzip", "text/html", 200, "<p>short</p>"), 200, "<p>short</p>"},
		{"head", gzipGet(t, http.MethodHead, "gzip", "text/html", 200, big), 200, big},
		{"not modified", gzipGet(t, http.MethodGet, "gzip", "text/html", 304, ""), 304, ""},
		{"no content", gzipGet(t, http.MethodGet, "gzip", "text/html", 204, ""), 204, ""},
		{"already encoded", gzipGet(t, http.MethodGet, "gzip", "text/html", 200, big, func(h http.Header) { h.Set("Content-Encoding", "br") }), 200, big},
		{"partial", gzipGet(t, http.MethodGet, "gzip", "text/html", 206, big, func(h http.Header) { h.Set("Content-Range", "bytes 0-9/10") }), 206, big},
	}
	for _, c := range cases {
		if c.rec.Code != c.status {
			t.Errorf("%s: status %d, want %d", c.name, c.rec.Code, c.status)
		}
		if enc := c.rec.Header().Get("Content-Encoding"); enc == "gzip" {
			t.Errorf("%s: was compressed", c.name)
		}
		// A HEAD recorder keeps the body; what matters is that it isn't encoded.
		if c.name != "head" && c.rec.Body.String() != c.body {
			t.Errorf("%s: body changed", c.name)
		}
	}
	// Vary still says the answer depends on the header, where it could have been compressed.
	for _, name := range []string{"not asked", "small", "head"} {
		for _, c := range cases {
			if c.name == name && c.rec.Header().Get("Vary") != "Accept-Encoding" {
				t.Errorf("%s: Vary = %q, want Accept-Encoding", name, c.rec.Header().Get("Vary"))
			}
		}
	}
}

func TestGzipSniffsAMissingType(t *testing.T) {
	page := "<!DOCTYPE html><html><body>" + strings.Repeat("<p>x</p>", 400) + "</body></html>"
	rec := gzipGet(t, http.MethodGet, "gzip", "", 200, page)
	if rec.Header().Get("Content-Encoding") != "gzip" || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
		t.Errorf("encoding %q, type %q", rec.Header().Get("Content-Encoding"), rec.Header().Get("Content-Type"))
	}
}

func TestGzipFlushStillWorks(t *testing.T) {
	h := withGzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, "first\n")
		w.(http.Flusher).Flush()
		io.WriteString(w, "second\n")
	}))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !rec.Flushed || gunzip(t, rec.Body.Bytes()) != "first\nsecond\n" {
		t.Errorf("flushed %v, body %q", rec.Flushed, rec.Body.String())
	}
}

func TestPagesAreGzippedAndIdentical(t *testing.T) {
	s := newTestServer(t)
	get := func(accept string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if accept != "" {
			req.Header.Set("Accept-Encoding", accept)
		}
		rec := httptest.NewRecorder()
		s.h.ServeHTTP(rec, req)
		return rec
	}
	plain, zipped := get(""), get("gzip")
	if plain.Code != 200 || zipped.Header().Get("Content-Encoding") != "gzip" || plain.Header().Get("Content-Encoding") != "" {
		t.Fatalf("plain %d %q, gzipped %q", plain.Code, plain.Header().Get("Content-Encoding"), zipped.Header().Get("Content-Encoding"))
	}
	if gunzip(t, zipped.Body.Bytes()) != plain.Body.String() {
		t.Errorf("the gzipped page decompresses to something else")
	}
	if zipped.Header().Get(static.VersionHeader) == "" {
		t.Errorf("the version header was lost")
	}
}

func TestStaticFilesGzipAndETags(t *testing.T) {
	s := newTestServer(t)
	url := static.URL("js/competitions.js")
	get := func(accept, inm string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, url, nil)
		if accept != "" {
			req.Header.Set("Accept-Encoding", accept)
		}
		if inm != "" {
			req.Header.Set("If-None-Match", inm)
		}
		rec := httptest.NewRecorder()
		s.h.ServeHTTP(rec, req)
		return rec
	}
	plain, zipped := get("", ""), get("gzip", "")
	if zipped.Header().Get("Content-Encoding") != "gzip" || plain.Header().Get("Content-Encoding") != "" {
		t.Fatalf("encodings: plain %q, gzipped %q", plain.Header().Get("Content-Encoding"), zipped.Header().Get("Content-Encoding"))
	}
	if gunzip(t, zipped.Body.Bytes()) != plain.Body.String() {
		t.Errorf("gzipped file differs from the plain one")
	}
	if zipped.Header().Get("ETag") == plain.Header().Get("ETag") || zipped.Header().Get("ETag") == "" {
		t.Errorf("ETags %q and %q must differ", plain.Header().Get("ETag"), zipped.Header().Get("ETag"))
	}
	for _, rec := range []*httptest.ResponseRecorder{plain, zipped} {
		if n := len(rec.Header().Values("Vary")); n != 1 {
			t.Errorf("Vary has %d values, want one", n)
		}
		if rec.Header().Get("Vary") != "Accept-Encoding" || rec.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
			t.Errorf("Vary %q, Cache-Control %q", rec.Header().Get("Vary"), rec.Header().Get("Cache-Control"))
		}
	}
	// Each copy revalidates against its own ETag.
	if rec := get("gzip", zipped.Header().Get("ETag")); rec.Code != http.StatusNotModified {
		t.Errorf("gzipped revalidation: status %d, want 304", rec.Code)
	}
	if rec := get("", plain.Header().Get("ETag")); rec.Code != http.StatusNotModified {
		t.Errorf("plain revalidation: status %d, want 304", rec.Code)
	}
	if rec := get("gzip", plain.Header().Get("ETag")); rec.Code != http.StatusOK {
		t.Errorf("a plain ETag must not match the gzipped copy: status %d", rec.Code)
	}
}
