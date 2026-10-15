package web

import (
	"compress/gzip"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"tariffCalculator/static"
)

// gzipMinBytes is the size below which a response goes out plain: the gzip
// header and the work cost more than they save.
const gzipMinBytes = 1024

var gzipPool = sync.Pool{New: func() any {
	return gzip.NewWriter(nil)
}}

// withGzip compresses the text responses of next (pages, JSON, SVG, calendar
// files and so on) for browsers that accept gzip. Pages are mostly repeated
// markup, so they shrink to a twentieth, which is most of the wait on a slow
// connection. Binary types, small responses, HEAD requests, partial content,
// 204s and 304s, and anything already encoded (the static files, which are
// compressed once) are passed through untouched.
func withGzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead || !static.AcceptsGzip(r.Header.Get("Accept-Encoding")) {
			// Say the response depends on the header even when this one wasn't
			// compressed, so a shared cache doesn't hand it to a gzip client.
			next.ServeHTTP(&varyWriter{ResponseWriter: w}, r)
			return
		}
		gw := &gzipWriter{ResponseWriter: w}
		defer gw.finish()
		next.ServeHTTP(gw, r)
	})
}

// compressible reports whether a Content-Type is text worth compressing.
func compressible(contentType string) bool {
	mt, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	switch {
	case strings.HasPrefix(mt, "text/"):
		return true
	case mt == "application/json", mt == "application/javascript", mt == "application/xml",
		mt == "image/svg+xml", strings.HasSuffix(mt, "+json"), strings.HasSuffix(mt, "+xml"):
		return true
	}
	return false
}

// addVary notes that the response depends on Accept-Encoding, once.
func addVary(h http.Header) {
	for _, v := range h.Values("Vary") {
		if strings.EqualFold(strings.TrimSpace(v), "Accept-Encoding") {
			return
		}
	}
	h.Add("Vary", "Accept-Encoding")
}

// varyWriter adds Vary: Accept-Encoding to the responses that could have been
// compressed, for the requests that weren't.
type varyWriter struct {
	http.ResponseWriter
	wrote bool
}

func (v *varyWriter) WriteHeader(status int) {
	if !v.wrote {
		v.wrote = true
		if compressible(v.Header().Get("Content-Type")) {
			addVary(v.Header())
		}
	}
	v.ResponseWriter.WriteHeader(status)
}

func (v *varyWriter) Write(p []byte) (int, error) {
	if !v.wrote {
		// Settle the type the way net/http would, so Vary follows it.
		if v.Header().Get("Content-Type") == "" {
			v.Header().Set("Content-Type", http.DetectContentType(p))
		}
		v.WriteHeader(http.StatusOK)
	}
	return v.ResponseWriter.Write(p)
}

func (v *varyWriter) Unwrap() http.ResponseWriter { return v.ResponseWriter }

// gzipWriter holds back the start of a response until it knows whether to
// compress it: it decides from the status, the headers and the first
// gzipMinBytes of the body.
type gzipWriter struct {
	http.ResponseWriter
	status  int
	decided bool
	buf     []byte // the body so far, until decided
	zw      *gzip.Writer
}

func (g *gzipWriter) WriteHeader(status int) {
	if g.decided || g.status != 0 {
		return
	}
	if status >= 100 && status < 200 {
		g.ResponseWriter.WriteHeader(status) // interim responses pass straight through
		return
	}
	g.status = status
	h := g.Header()
	switch {
	case status == http.StatusNoContent, status == http.StatusNotModified, status == http.StatusPartialContent,
		h.Get("Content-Encoding") != "", h.Get("Content-Range") != "":
		g.decide(false)
	default:
		// A handler that sent a Content-Length already knows the size.
		if n, err := strconv.Atoi(h.Get("Content-Length")); err == nil && n < gzipMinBytes {
			g.decide(false)
		}
	}
}

func (g *gzipWriter) Write(p []byte) (int, error) {
	if g.status == 0 {
		if g.Header().Get("Content-Type") == "" {
			g.Header().Set("Content-Type", http.DetectContentType(p))
		}
		g.WriteHeader(http.StatusOK)
	}
	if !g.decided {
		g.buf = append(g.buf, p...)
		g.sniff()
		if len(g.buf) >= gzipMinBytes {
			g.decide(compressible(g.Header().Get("Content-Type")))
		}
		return len(p), nil
	}
	if g.zw != nil {
		return g.zw.Write(p)
	}
	return g.ResponseWriter.Write(p)
}

// sniff settles a missing Content-Type from the body so far, as net/http would.
func (g *gzipWriter) sniff() {
	if g.Header().Get("Content-Type") == "" && len(g.buf) > 0 {
		g.Header().Set("Content-Type", http.DetectContentType(g.buf))
	}
}

// decide commits to compressing or not, sends the header and any body held back.
func (g *gzipWriter) decide(compress bool) {
	g.decided = true
	h := g.Header()
	if compressible(h.Get("Content-Type")) && h.Get("Content-Encoding") == "" {
		addVary(h)
	}
	if compress {
		h.Set("Content-Encoding", "gzip")
		h.Del("Content-Length")
		// A strong validator would name the plain bytes; the gzipped ones differ.
		if etag := h.Get("ETag"); etag != "" && !strings.HasPrefix(etag, "W/") {
			h.Set("ETag", "W/"+etag)
		}
		g.zw = gzipPool.Get().(*gzip.Writer)
		g.zw.Reset(g.ResponseWriter)
	}
	g.ResponseWriter.WriteHeader(g.status)
	buf := g.buf
	g.buf = nil
	if len(buf) > 0 {
		g.write(buf)
	}
}

func (g *gzipWriter) write(p []byte) {
	if g.zw != nil {
		g.zw.Write(p)
	} else {
		g.ResponseWriter.Write(p)
	}
}

// finish ends the response: a body that stayed under the threshold goes out
// plain; a compressed one has its gzip stream closed.
func (g *gzipWriter) finish() {
	if !g.decided {
		if g.status == 0 {
			return // the handler wrote nothing: net/http sends its own 200
		}
		g.sniff()
		g.decide(false)
	}
	if g.zw != nil {
		g.zw.Close()
		g.zw.Reset(nil)
		gzipPool.Put(g.zw)
		g.zw = nil
	}
}

// Flush sends what has been written so far, for a handler that streams. None
// does today; a flush means the body should start now, so it is compressed.
func (g *gzipWriter) Flush() {
	if g.status == 0 {
		g.WriteHeader(http.StatusOK)
	}
	if !g.decided {
		g.decide(compressible(g.Header().Get("Content-Type")))
	}
	if g.zw != nil {
		g.zw.Flush()
	}
	http.NewResponseController(g.ResponseWriter).Flush()
}

func (g *gzipWriter) Unwrap() http.ResponseWriter { return g.ResponseWriter }
