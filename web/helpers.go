package web

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"tariffCalculator/store"
	"tariffCalculator/views"
)

// Small helpers the pages share: answering a request (a redirect with a
// notice, a message, a failure), where it came from and how often, and
// tidying what's posted or downloaded.

// seeOther redirects after a form to the page to, with a notice saying what
// happened ("" to say nothing), first in its query.
func seeOther(w http.ResponseWriter, r *http.Request, to, notice string) {
	if notice != "" {
		page, part, hasPart := strings.Cut(to, "#")
		path, query, hasQuery := strings.Cut(page, "?")
		to = path + "?notice=" + url.QueryEscape(notice)
		if hasQuery {
			to += "&" + query
		}
		if hasPart {
			to += "#" + part
		}
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

// message renders a page that only says something.
func message(w http.ResponseWriter, r *http.Request, status int, title, text string) {
	w.WriteHeader(status)
	render(w, r, views.Message(title, text))
}

// failed reports a store error as a page: a link that leads nowhere, a
// deadline passed, a limit reached, or something unexpected.
func failed(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		w.WriteHeader(http.StatusNotFound)
		render(w, r, views.DeadLink("Link not found", "This link doesn't lead anywhere. It may have been replaced, or the competition deleted."))
	case errors.Is(err, store.ErrNotOpen):
		message(w, r, http.StatusConflict, "Entries aren't open", "The organiser hasn't opened entries for this competition, so they can't be sent or changed just now.")
	case errors.Is(err, store.ErrClosed):
		message(w, r, http.StatusConflict, "Entries have closed", "The deadline for this competition has passed, so entries can't be changed.")
	case errors.Is(err, store.ErrLimit):
		message(w, r, http.StatusConflict, "Competition full", "This competition has as many entries as it can take.")
	case errors.Is(err, store.ErrRemoved):
		message(w, r, http.StatusConflict, "Entry removed", "The organiser has removed this entry, so it can't be changed. Ask them if you think it should be back in.")
	default:
		// The route's pattern, not its path, which holds the secret link.
		log.Printf("Competition storage: %s: %v", r.Pattern, err)
		message(w, r, http.StatusInternalServerError, "Something went wrong", "That didn't work. Please try again in a minute.")
	}
}

func intPtr(n int) *int { return &n }

// sentences splits a validation error into its problems, each a sentence.
func sentences(err error) []string {
	var out []string
	for _, line := range strings.Split(err.Error(), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, strings.ToUpper(line[:1])+line[1:]+".")
		}
	}
	return out
}

// origin is the scheme and host the request came to, for full links.
func origin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// csvSafe stops a spreadsheet reading a cell as a formula.
func csvSafe(v string) string {
	if v != "" && strings.ContainsRune("=+-@\t\r", rune(v[0])) {
		return "'" + v
	}
	return v
}

// fileName is a name made safe for a downloaded file.
func fileName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	if b.Len() == 0 {
		return "entries"
	}
	return b.String()
}

// clubOf is who sent a stored entry, as the pages say it.
func clubOf(e store.Entry) string {
	if e.Individual {
		return onTheirOwn
	}
	return e.ClubName
}

// deleteExpired deletes competitions 120 days after their date (ADR 0004
// Decision 6) and clubs unused for 13 months (ADR 0011 Decision 7), now and
// every few hours.
func deleteExpired(st *store.Store) {
	for {
		comps, clubs, err := st.DeleteExpired(context.Background())
		switch {
		case err != nil:
			log.Printf("Deleting expired competitions and clubs: %v", err)
		case comps+clubs > 0:
			log.Printf("Deleted %d expired competitions and %d unused clubs", comps, clubs)
		}
		time.Sleep(6 * time.Hour)
	}
}

// lines are a text's non-blank lines, trimmed.
func lines(text string) []string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// limitNote is a note trimmed to at most 300 characters.
func limitNote(note string) string {
	note = strings.TrimSpace(note)
	if r := []rune(note); len(r) > 300 {
		note = string(r[:300])
	}
	return note
}

// limiter allows each address n actions per window.
type limiter struct {
	mu     sync.Mutex
	n      int
	window time.Duration
	seen   map[string][]time.Time
}

func newLimiter(n int, window time.Duration) *limiter {
	return &limiter{n: n, window: window, seen: map[string][]time.Time{}}
}

// allow records an action by ip at now, unless it has had n in the window.
func (l *limiter) allow(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	recent := l.seen[ip][:0]
	for _, t := range l.seen[ip] {
		if now.Sub(t) < l.window {
			recent = append(recent, t)
		}
	}
	if len(recent) >= l.n {
		l.seen[ip] = recent
		return false
	}
	l.seen[ip] = append(recent, now)
	// Forget addresses with nothing recent, so the map doesn't grow forever.
	for k, ts := range l.seen {
		if len(ts) == 0 || now.Sub(ts[len(ts)-1]) >= l.window {
			delete(l.seen, k)
		}
	}
	return true
}

// clientIP is the address a request came from. Behind Caddy, that's the
// rightmost X-Forwarded-For value, the one Caddy added (ADR 0004 Decision 7).
func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		parts := strings.Split(fwd, ",")
		return strings.TrimSpace(parts[len(parts)-1])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
