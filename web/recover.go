package web

import (
	"log"
	"net/http"
	"runtime/debug"
)

// withRecover is next, with a panic in it logged and answered with the
// app's "Something went wrong" page, rather than the visitor's connection
// dropped (and, on a competition page, the kept copy shown as if offline).
func withRecover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &startedWriter{ResponseWriter: w}
		defer func() {
			v := recover()
			if v == nil {
				return
			}
			if v == http.ErrAbortHandler {
				panic(v) // net/http's own way to stop a response: leave it to net/http
			}
			// The route's pattern, not its path, which holds the secret link.
			stack := debug.Stack()
			log.Printf("Panic in %s: %v\n%s", r.Pattern, v, stack)
			problemsOf(r.Context()).panicked(r, v, stack)
			if !rw.started {
				message(rw, r, http.StatusInternalServerError, "Something went wrong", "That didn't work. Please try again in a minute.")
			}
		}()
		next.ServeHTTP(rw, r)
	})
}

// startedWriter is a response writer that knows whether anything has been
// written yet.
type startedWriter struct {
	http.ResponseWriter
	started bool
}

func (w *startedWriter) WriteHeader(status int) {
	w.started = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *startedWriter) Write(b []byte) (int, error) {
	w.started = true
	return w.ResponseWriter.Write(b)
}

// Flush sends what's written so far, as the writer underneath would.
func (w *startedWriter) Flush() {
	w.started = true
	http.NewResponseController(w.ResponseWriter).Flush()
}

// Unwrap lets http.ResponseController reach the writer underneath.
func (w *startedWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
