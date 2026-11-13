package web

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// Someone else's token, as 22 characters like a real one.
const others = "OtherPersonsToken0001"

// A copy of a page with another person's links, whatever they're in, comes
// back from the maintainer's page without them: what's marked private
// blanked (even when the sender's script didn't), and any other secret link
// without its token.
func TestSnapshotHidesOtherLinks(t *testing.T) {
	s := newTestServer(t, atTime(inDublin(2030, 1, 10, 12, 0)))
	h, site := s.h, maintainerLink(t, s)
	link := "https://tariff.example/clubs/member/" + others
	page := `<!DOCTYPE html><html><head><title>t</title></head><body>
		<input id="a" value="` + link + `" readonly data-private>
		<input id="b" value="Typed /clubs/member/` + others + `">
		<textarea data-private>Words ` + others + `</textarea>
		<p data-private>Their link is ` + link + `</p>
		<p>Plain text with ` + link + ` in it</p>
		<a href="/clubs/coach/` + others + `">coach</a>
		<button data-copy="` + link + `">Copy</button>
		<span hidden data-member-link="` + link + `"></span>
		<input type="hidden" name="link" value="/competitions/gymnast/` + others + `/x">
		<img data-private src="data:image/png;base64,QVJHSEg=" alt="` + link + `">
		<img src="data:image/png;base64,QVJHSEg=" data-qr="` + link + `" alt="QR">
		<div title="/site/` + others + `/feedback">Keep this</div>
		</body></html>`
	rec := postFeedback(t, h, "/feedback", url.Values{"text": {"Hi"}, "page": {"/"}}, map[string][]byte{"snapshot": gzipped(t, page)})
	if rec.Code != http.StatusOK {
		t.Fatalf("sent: %d %s", rec.Code, rec.Body)
	}
	all, _ := s.st.AllSiteFeedback(t.Context())
	copied := do(t, h, http.MethodGet, site+"/feedback/"+all[0].ID+"/page", nil)
	if copied.Code != http.StatusOK {
		t.Fatalf("the copy: %d", copied.Code)
	}
	body := copied.Body.String()
	if strings.Contains(body, others) {
		t.Errorf("another person's token is in the copy: %s", body)
	}
	for _, kept := range []string{"Typed /clubs/member/{token}", "Plain text with", "/clubs/member/{token}", "Keep this", `data-qr="https://tariff.example/clubs/member/{token}"`} {
		if !strings.Contains(body, kept) {
			t.Errorf("the copy has %q: %s", kept, body)
		}
	}
	if strings.Contains(body, "Their link is") || strings.Contains(body, "Words") || strings.Contains(body, "QVJHSEg=\" alt=\"https") {
		t.Errorf("what's private is blanked: %s", body)
	}
	// The private picture has no source; the other picture keeps its own.
	if n := strings.Count(body, "QVJHSEg="); n != 1 {
		t.Errorf("the private picture's source is gone, the other's stays: %d in %s", n, body)
	}
}

// A copy's own styles can't load from elsewhere: no @import, no url() but of
// a data: address.
func TestSnapshotStylesLoadNothing(t *testing.T) {
	page := `<html><head><style>
		@import url(https://attacker.example/a.css);
		@import "https://attacker.example/b.css";
		body { background: url(https://attacker.example/x.png) }
		p { background: url( 'http://attacker.example/y.png' ) }
		q { background-image: image-set("https://attacker.example/z.png" 1x) }
		i { background: u\72l(https://escaped.example/w.png) }
		b { background: url(data:image/png;base64,AAAA) }
		</style></head><body style="background:url(https://attacker.example/s.png)"><p>Hi</p></body></html>`
	out, err := cleanSnapshot([]byte(page), "", 0)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(zr)
	body := string(b)
	for _, gone := range []string{"attacker.example", "@import", "image-set", `\`} {
		if strings.Contains(body, gone) {
			t.Errorf("the copy has no %q: %s", gone, body)
		}
	}
	if !strings.Contains(body, "url(data:image/png;base64,AAAA)") {
		t.Errorf("a data: url stays: %s", body)
	}
}

// The maintainer's page is sent with a policy that lets nothing load from
// elsewhere, which a frame made from a copy inherits.
func TestMaintainerPagePolicy(t *testing.T) {
	s := newTestServer(t)
	site := maintainerLink(t, s)
	rec := do(t, s.h, http.MethodGet, site, nil)
	csp := rec.Header().Get("Content-Security-Policy")
	for _, want := range []string{"default-src 'self'", "img-src 'self' data:", "object-src 'none'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("the policy has %q: %s", want, csp)
		}
	}
	for _, not := range []string{"http:", "https:", "*"} {
		if strings.Contains(csp, not) {
			t.Errorf("the policy allows %q: %s", not, csp)
		}
	}
}

// Feedback is deleted on the maintainer's page, with what came with it, and
// by itself after 90 days; newer feedback stays.
func TestFeedbackDeleted(t *testing.T) {
	s := newTestServer(t, atTime(inDublin(2030, 1, 10, 12, 0)))
	h, site := s.h, maintainerLink(t, s)
	send := func(text string) {
		t.Helper()
		rec := postFeedback(t, h, "/feedback", url.Values{"text": {text}, "reply": {text + "@example.com"}, "page": {"/"}},
			map[string][]byte{"picture": aJPEG, "snapshot": gzipped(t, "<p>"+text+"</p>")})
		if rec.Code != http.StatusOK {
			t.Fatalf("sent: %d %s", rec.Code, rec.Body)
		}
	}
	send("Old")
	s.clock.set(inDublin(2030, 4, 1, 12, 0)) // 81 days on
	send("Recent")
	send("Gone")
	send("Kept")

	// Deleted on the page, with its picture and copy; the others stay.
	list := do(t, h, http.MethodGet, site+"?all=1", nil).Body.String()
	if !strings.Contains(list, "/delete") || !strings.Contains(list, "for good?") {
		t.Error("each piece has a Delete, asking first")
	}
	all, _ := s.st.AllSiteFeedback(t.Context())
	id := ""
	for _, f := range all {
		if f.Text == "Gone" {
			id = f.ID
		}
	}
	redirected(t, h, site+"/feedback/"+id+"/delete", url.Values{"all": {"1"}})
	if rec := do(t, h, http.MethodGet, site+"/feedback/"+id+"/picture", nil); rec.Code != http.StatusNotFound {
		t.Errorf("its picture is gone: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, site+"/feedback/"+id+"/delete", url.Values{}); rec.Code != http.StatusNotFound {
		t.Errorf("deleted twice: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, "/site/nope/feedback/"+id+"/delete", url.Values{}); rec.Code != http.StatusNotFound {
		t.Errorf("without the link: %d", rec.Code)
	}
	all, _ = s.st.AllSiteFeedback(t.Context())
	if len(all) != 3 {
		t.Fatalf("one deleted: %d", len(all))
	}

	// 91 days after "Old", it goes, with its picture; the others are
	// younger than 90 days.
	deleteOldCaptures(s.st, inDublin(2030, 4, 11, 12, 0))
	all, _ = s.st.AllSiteFeedback(t.Context())
	var texts []string
	for _, f := range all {
		texts = append(texts, f.Text)
		if !f.Picture || !f.Snapshot {
			t.Errorf("the younger keep their captures: %+v", f)
		}
	}
	if len(texts) != 2 || strings.Contains(strings.Join(texts, ","), "Old") {
		t.Errorf("only the old is deleted: %v", texts)
	}
	if n, _ := s.st.CapturesBytes(t.Context()); n == 0 {
		t.Error("the newer captures stay")
	}
}
