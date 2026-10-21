package web

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"tariffCalculator/store"
	"tariffCalculator/views"
)

// The maintainer's page (migration 47): the feedback sent about the app from
// every page, with what came with it, and the choices for how it's taken.
// It's opened with the maintainer's link, made on the server
// (`tariffCalculator maintainer-link`); anything else at /site/ is a page
// that isn't there.

func (p *competitionPages) registerMaintainer(mux *http.ServeMux) {
	handle := func(pattern string, h http.HandlerFunc) { mux.Handle(pattern, p.maintainer(h)) }
	handle("GET /site/{token}", p.maintainerPage)
	handle("POST /site/{token}/settings", p.feedbackChoices)
	handle("POST /site/{token}/feedback/{id}/done", p.markSiteFeedback)
	handle("POST /site/{token}/captures/delete", p.deleteCaptures)
	handle("GET /site/{token}/feedback/{id}/picture", p.feedbackPicture)
	handle("GET /site/{token}/feedback/{id}/page", p.feedbackSnapshot)
}

// maintainer lets only the maintainer's link through, its pages kept
// private as a link's are (secret).
func (p *competitionPages) maintainer(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Robots-Tag", "noindex")
		if p.st == nil {
			notFound(w, r)
			return
		}
		ok, err := p.st.IsMaintainerLink(r.Context(), r.PathValue("token"))
		if err != nil {
			failed(w, r, err)
			return
		}
		if !ok {
			notFound(w, r)
			return
		}
		h(w, r)
	})
}

// maintainerPath is the maintainer's page.
func maintainerPath(token string) string { return "/site/" + token }

// maintainerPage lists the feedback, newest first: what's not dealt with,
// or all of it (?all=1).
func (p *competitionPages) maintainerPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	all, err := p.st.AllSiteFeedback(ctx)
	if err != nil {
		failed(w, r, err)
		return
	}
	kept, err := p.st.CapturesBytes(ctx)
	if err != nil {
		failed(w, r, err)
		return
	}
	s := p.feedbackSettings()
	page := views.MaintainerPage{Base: maintainerPath(r.PathValue("token")), Notice: r.URL.Query().Get("notice"),
		Off: s.Off, Light: s.Light, Size: megabytes(kept), All: r.URL.Query().Get("all") == "1", Total: len(all),
		Sandbox: p.sandboxPanel(ctx, maintainerPath(r.PathValue("token")))}
	for _, f := range all {
		if f.Picture || f.Snapshot {
			page.Captures++
		}
		if !f.Done() {
			page.Waiting++
		} else if !page.All {
			continue
		}
		var d feedbackDetails
		if f.Details != "" {
			json.Unmarshal([]byte(f.Details), &d)
		}
		item := views.SiteFeedbackItem{ID: f.ID, At: f.At.In(local).Format("Mon 2 Jan, 15:04"), Role: f.Role, Page: pageName(f.Page), Pattern: f.Page,
			Competition: f.Competition, Text: f.Text, ReplyTo: f.ReplyTo, Done: f.Done(), Picture: f.Picture, Snapshot: f.Snapshot,
			Details: detailLines(d)}
		_, item.ReplyEmail = emailAddress(f.ReplyTo)
		if len(d.Viewport) == 2 {
			item.Width, item.Height = d.Viewport[0], d.Viewport[1]
		}
		if len(d.Scroll) == 2 {
			item.ScrollY = d.Scroll[1]
		}
		item.PageHeight = d.Height
		page.Items = append(page.Items, item)
	}
	render(w, r, views.Maintainer(page))
}

// detailLines are the details as the maintainer's page shows them.
func detailLines(d feedbackDetails) []views.DetailLine {
	var out []views.DetailLine
	add := func(label, value string) {
		if value != "" {
			out = append(out, views.DetailLine{Label: label, Value: value})
		}
	}
	pair := func(v []int, sep string) string {
		if len(v) != 2 {
			return ""
		}
		return fmt.Sprintf("%d%s%d", v[0], sep, v[1])
	}
	add("Window", pair(d.Viewport, " × "))
	if d.Ratio > 0 {
		add("Screen", strings.TrimSpace(pair(d.Screen, " × ")+fmt.Sprintf(" at %g×", d.Ratio)))
	}
	add("Browser", d.Browser)
	add("Language", d.Language)
	if d.Online != nil {
		add("Signal", map[bool]string{true: "online", false: "offline"}[*d.Online])
	}
	if d.Installed {
		add("Opened", "from the home screen")
	}
	if d.Scroll != nil {
		down := fmt.Sprintf("%d px down", d.Scroll[1])
		if d.Height > 0 {
			down += fmt.Sprintf(" of %d", d.Height)
		}
		add("Scrolled", down)
	}
	if d.Queued > 0 {
		add("Waiting for signal", views.Count(d.Queued, "tap", "taps"))
	}
	add("Dropdowns open", strings.Join(d.Open, " · "))
	add("Before this, in that tab", strings.Join(namedPages(d.Recent), " → "))
	add("Errors", strings.Join(d.Errors, "\n"))
	add("App version", d.Version)
	add("Page made", d.PageTime)
	add("No picture", d.NoPicture)
	return out
}

func namedPages(patterns []string) []string {
	out := make([]string, len(patterns))
	for i, p := range patterns {
		out[i] = pageName(p)
	}
	return out
}

func megabytes(n int64) string {
	if n < 1<<20 {
		return fmt.Sprintf("%d KB", (n+1023)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
}

// feedbackChoices turns the button on or off everywhere (button=on|off), or
// chooses Full or Light (capture=full|light).
func (p *competitionPages) feedbackChoices(w http.ResponseWriter, r *http.Request) {
	s := p.feedbackSettings()
	notice := ""
	switch {
	case r.FormValue("button") == "on":
		s.Off, notice = false, "The Feedback button is on every page."
	case r.FormValue("button") == "off":
		s.Off, notice = true, "The Feedback button is off everywhere."
	case r.FormValue("capture") == "full":
		s.Light, notice = false, "Full: a picture and a copy of the page go with every piece."
	case r.FormValue("capture") == "light":
		s.Light, notice = true, "Light: details, and a picture only if they tick for it."
	default:
		seeOther(w, r, maintainerPath(r.PathValue("token")), "Nothing was changed.")
		return
	}
	if err := p.st.SetSiteFeedbackSettings(r.Context(), s); err != nil {
		failed(w, r, err)
		return
	}
	p.siteFeedback.Store(&s)
	seeOther(w, r, maintainerPath(r.PathValue("token")), notice)
}

// markSiteFeedback marks feedback dealt with (done=1), or not.
func (p *competitionPages) markSiteFeedback(w http.ResponseWriter, r *http.Request) {
	if err := p.st.MarkSiteFeedback(r.Context(), r.PathValue("id"), r.FormValue("done") == "1"); err != nil {
		failed(w, r, err)
		return
	}
	to := maintainerPath(r.PathValue("token"))
	if r.FormValue("all") == "1" {
		to += "?all=1"
	}
	seeOther(w, r, to, "")
}

// deleteCaptures deletes every picture and copy of a page kept.
func (p *competitionPages) deleteCaptures(w http.ResponseWriter, r *http.Request) {
	n, err := p.st.DeleteCaptures(r.Context(), time.Time{})
	if err != nil {
		failed(w, r, err)
		return
	}
	seeOther(w, r, maintainerPath(r.PathValue("token")), fmt.Sprintf("Deleted what came with %s; what they wrote is kept.", views.Count(int(n), "piece of feedback", "pieces of feedback")))
}

// feedbackPicture is the picture of their screen.
func (p *competitionPages) feedbackPicture(w http.ResponseWriter, r *http.Request) {
	picture, kind, err := p.st.SiteFeedbackPicture(r.Context(), r.PathValue("id"))
	if err != nil {
		failed(w, r, err)
		return
	}
	w.Header().Set("Content-Type", kind)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Write(picture)
}

// snapshotPolicy is how a copy of a page is shown: as a document that can't
// run anything, send a form or load anything but the app's own styles and
// pictures, framed only by the maintainer's page.
const snapshotPolicy = "sandbox; default-src 'none'; style-src 'unsafe-inline'; img-src 'self' data:; font-src data:; form-action 'none'; base-uri 'none'; frame-ancestors 'self'"

// feedbackSnapshot is the copy of the page, cleaned when it arrived
// (cleanSnapshot), with the app's styles in it (inlineStyles). The
// maintainer's page shows it in a sandboxed frame by fetching it
// (maintainer.js); opened on its own, the policy keeps it as safe.
func (p *competitionPages) feedbackSnapshot(w http.ResponseWriter, r *http.Request) {
	snapshot, err := p.st.SiteFeedbackSnapshot(r.Context(), r.PathValue("id"))
	if err != nil {
		failed(w, r, err)
		return
	}
	zr, err := gzip.NewReader(bytes.NewReader(snapshot))
	if err != nil {
		failed(w, r, err)
		return
	}
	page, err := io.ReadAll(io.LimitReader(zr, maxSnapshotHTML))
	if err != nil {
		failed(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", snapshotPolicy)
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	w.Write(inlineStyles(page))
}

// deleteOldCaptures deletes the pictures and copies of pages kept longer
// than keepCaptures, with the rest of what expires (deleteExpired).
func deleteOldCaptures(st *store.Store, now time.Time) {
	n, err := st.DeleteCaptures(context.Background(), now.Add(-keepCaptures))
	switch {
	case err != nil:
		log.Printf("Deleting old pictures and copies of pages sent with feedback: %v", err)
	case n > 0:
		log.Printf("Deleted the pictures and copies of pages sent with %d pieces of feedback", n)
	}
}
