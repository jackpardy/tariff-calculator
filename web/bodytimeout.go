package web

import (
	"context"
	"io"
	"net/http"
	"time"
)

// uploadReadTimeout is how long a request that sends a file or a picture
// has to send all of its body. The server's own read timeout is short, for
// ordinary requests; a 3 MB certificate over phone data takes longer.
var uploadReadTimeout = 90 * time.Second

// slowBody is said when a body didn't all arrive in time, or the
// connection dropped part-way. It follows "Nothing was added: " too.
const slowBody = "the connection was too slow, or dropped, before it all arrived: nothing was saved. Try again, on wifi if you can."

// allowSlowBody gives a request that sends a file or picture longer to send
// its body, in place of the server's short read timeout, and notes how
// reading the body fails. Every route that reads a large body goes through
// it (see Handler); use the request it returns.
func allowSlowBody(w http.ResponseWriter, r *http.Request) *http.Request {
	// A writer that can't set it (a test's recorder) just keeps the server's.
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(uploadReadTimeout))
	note := &bodyNote{body: r.Body}
	r.Body = note
	return r.WithContext(context.WithValue(r.Context(), bodyNoteKey{}, note))
}

type bodyNoteKey struct{}

// bodyNote is a request body that keeps the error reading it failed with,
// since the form parsers hide it (a stalled upload comes back as a
// malformed part).
type bodyNote struct {
	body io.ReadCloser
	err  error
}

func (b *bodyNote) Read(p []byte) (int, error) {
	n, err := b.body.Read(p)
	if err != nil && err != io.EOF {
		b.err = err
	}
	return n, err
}

func (b *bodyNote) Close() error { return b.body.Close() }

// bodyCutShort says reading the request's body failed because time ran out
// or the connection dropped, not because the body was too big (that error
// comes from the size limit, outside what's noted).
func bodyCutShort(r *http.Request) bool {
	note, _ := r.Context().Value(bodyNoteKey{}).(*bodyNote)
	return note != nil && note.err != nil
}
