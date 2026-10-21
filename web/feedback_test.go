package web

import (
	"bytes"
	"compress/gzip"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"tariffCalculator/store"
)

// feedbackFormOf is the Feedback button's form on a page: where it posts
// and its hidden fields. ok is false for a page without one.
func feedbackFormOf(t *testing.T, page string) (action string, fields url.Values, ok bool) {
	t.Helper()
	box := firstByClass(parsePage(t, page), "site-feedback")
	if box == nil {
		return "", nil, false
	}
	forms := byTag(box, "form")
	if len(forms) != 1 {
		t.Fatalf("the Feedback button has one form: %d", len(forms))
	}
	fields = url.Values{}
	for _, in := range byTag(forms[0], "input") {
		if attrOf(in, "type") == "hidden" {
			fields.Set(attrOf(in, "name"), attrOf(in, "value"))
		}
	}
	return attrOf(forms[0], "action"), fields, true
}

// withoutFeedback is a page without its Feedback button, which comes last.
func withoutFeedback(page string) string {
	before, _, _ := strings.Cut(page, `<div class="site-feedback"`)
	return before
}

// sendFeedbackFrom sends feedback with a page's Feedback button, as its
// script does, and returns the response.
func sendFeedbackFrom(t *testing.T, h http.Handler, path, text string) (int, string) {
	t.Helper()
	action, fields, ok := feedbackFormOf(t, do(t, h, http.MethodGet, path, nil).Body.String())
	if !ok {
		t.Fatalf("%s has a Feedback button", path)
	}
	fields.Set("text", text)
	rec := postFeedback(t, h, action, fields, nil)
	return rec.Code, rec.Body.String()
}

// postFeedback posts feedback as the page's script does: a multipart form,
// with files (name to content) if any.
func postFeedback(t *testing.T, h http.Handler, action string, fields url.Values, files map[string][]byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for k, vs := range fields {
		for _, v := range vs {
			mw.WriteField(k, v)
		}
	}
	for name, b := range files {
		fw, err := mw.CreateFormFile(name, name)
		if err != nil {
			t.Fatal(err)
		}
		fw.Write(b)
	}
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, action, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Feedback", "fetch")
	req.RemoteAddr = "192.0.2.1:1234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// maintainerLink makes the maintainer's link and returns their page.
func maintainerLink(t *testing.T, s *testServer) string {
	t.Helper()
	token, err := s.st.NewMaintainerLink(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return maintainerPath(token)
}

// feedbackItems are the maintainer's page's messages, all of them, newest
// first: role, page, competition and what was said.
func feedbackItems(t *testing.T, h http.Handler, site string) [][4]string {
	t.Helper()
	rec := do(t, h, http.MethodGet, site+"?all=1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("the maintainer's page: %d", rec.Code)
	}
	var out [][4]string
	for _, item := range byClass(parsePage(t, rec.Body.String()), "comp-feedback-item") {
		out = append(out, [4]string{textOf(firstByClass(item, "comp-feedback-role")), textOf(firstByClass(item, "comp-feedback-page")),
			textOf(firstByClass(item, "comp-feedback-competition")), textOf(firstByClass(item, "comp-feedback-text"))})
	}
	return out
}

// Feedback can be sent from the pages of every kind of link, home pages
// too, and reaches the maintainer with the role, the page and the
// competition, never the link.
func TestFeedbackFromEveryLink(t *testing.T) {
	s := newTestServer(t, atTime(inDublin(2030, 1, 10, 12, 0)))
	h, site := s.h, maintainerLink(t, s)
	admin := created(t, h, newCompetition())
	ucd := aClub(t, h, admin, "UCD", true, "Ann")
	ann := ucd.members["Ann"]
	annPage := withoutQuery(formAction(t, memberCompetitionPage(t, h, ann), ann+"/competitions/"))
	coach := pathIn(t, do(t, h, http.MethodPost, ucd.page+"/coaches", url.Values{"name": {"Cara"}}).Body.String(), "/clubs/coach/")
	redirected(t, h, ucd.page+"/coaches-see-all", url.Values{"on": {"1"}})
	coachEntry := regexp.MustCompile(`href="(` + regexp.QuoteMeta(coach) + `/members/[^"]+)"`).FindStringSubmatch(do(t, h, http.MethodGet, coach, nil).Body.String())
	if coachEntry == nil {
		t.Fatal("the coach sees Ann's entry")
	}
	compSec := regexp.MustCompile(`href="(` + regexp.QuoteMeta(ucd.page) + `/members/[^"]+/competitions/[^"]+)"`).FindStringSubmatch(clubEntriesPage(t, h, ucd.page))
	if compSec == nil {
		t.Fatal("the comp sec sees Ann's entry")
	}
	own := anIndividual(t, h, admin, "Bea Byrne")
	chair := madeLink(t, h, admin, "Chairs", "chair")

	for _, tc := range []struct {
		from, path, role, page, competition string
	}{
		{"the organiser", admin, "Organiser", "Entries", "Student Open"},
		{"a helper link", chair, "Helper link: Chairs (chairs of judges)", "Entries", "Student Open"},
		{"a comp sec", unescape(compSec[1]), "Comp sec", "A member's entry", "Student Open"},
		{"a club's home", ucd.page, "Comp sec", "Club home", ""},
		{"a member", annPage, "Member", "Their entries", "Student Open"},
		{"a member's My competition", annPage + "/day", "Member", "My competition", "Student Open"},
		{"a member's home", ann, "Member", "Member home", ""},
		{"a coach", unescape(coachEntry[1]), "Coach", "Signing off an entry", "Student Open"},
		{"a coach's home", coach, "Coach", "Coach home", ""},
		{"an individual", own, "Individual", "Their entries", "Student Open"},
	} {
		if code, said := sendFeedbackFrom(t, h, tc.path, "From "+tc.from); code != http.StatusOK || said != "Thanks: your feedback is sent." {
			t.Errorf("from %s: %d %q", tc.from, code, said)
		}
		items := feedbackItems(t, h, site)
		if want := [4]string{tc.role, tc.page, tc.competition, "From " + tc.from}; len(items) == 0 || items[0] != want {
			t.Errorf("from %s, the newest is %v, want %v", tc.from, items[0], want)
		}
	}

	// What's kept is the page's pattern: no link, in any column.
	all, err := s.st.AllSiteFeedback(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range all {
		for _, v := range []string{f.Text, f.ReplyTo, f.Role, f.Page, f.Details} {
			if regexp.MustCompile(`[A-Za-z0-9_-]{20,}`).MatchString(v) {
				t.Errorf("a token is kept: %q", v)
			}
		}
		if !strings.Contains(f.Page, "{token}") {
			t.Errorf("the page is its pattern: %q", f.Page)
		}
	}

	// Without JavaScript, it comes back to the page, at the thanks.
	action, fields, _ := feedbackFormOf(t, do(t, h, http.MethodGet, annPage, nil).Body.String())
	fields.Set("text", "No script")
	if to := redirected(t, h, action, fields); to != annPage+"#feedback-sent" {
		t.Errorf("back to the page: %s", to)
	}
	// ...never anywhere but the link's own pages.
	fields.Set("back", admin)
	if to := redirected(t, h, action, fields); !strings.HasPrefix(to, ann+"/") && to != ann+"#feedback-sent" {
		t.Errorf("back to the member's own link only: %s", to)
	}

	// The organiser no longer has a Feedback page of their own.
	if rec := do(t, h, http.MethodGet, admin+"/feedback", nil); rec.Code == http.StatusOK {
		t.Error("the organiser's Feedback page is gone")
	}
}

// Entering pages have the button too: an individual entering, a comp sec
// entering their club, and a synchro partner confirming.
func TestFeedbackWhileEntering(t *testing.T) {
	s := newTestServer(t, atTime(inDublin(2030, 1, 10, 12, 0)))
	h, site := s.h, maintainerLink(t, s)
	admin := created(t, h, eventsCompetition())
	settings := do(t, h, http.MethodGet, admin+"/settings", nil).Body.String()
	enter := pathIn(t, settings, "/competitions/enter/")
	clubLink := pathIn(t, settings, "/competitions/club/")
	pair := withoutQuery(redirected(t, h, enter, url.Values{"gymnast": {"S"}, "discipline": {"synchro"}, "level": {"BUCS L3"}, "partnerName": {"P"},
		"ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}}))
	partner := pathIn(t, do(t, h, http.MethodGet, pair, nil).Body.String(), "/competitions/partner/")
	for _, tc := range []struct{ path, role, page string }{
		{enter, "Individual", "Entering on their own"},
		{clubLink, "Comp sec", "Entering a club"},
		{partner, "Synchro partner", "Confirming a synchro partner"},
	} {
		if code, _ := sendFeedbackFrom(t, h, tc.path, "Hello"); code != http.StatusOK {
			t.Errorf("from %s: %d", tc.page, code)
		}
		if items := feedbackItems(t, h, site); items[0] != [4]string{tc.role, tc.page, "Student Open", "Hello"} {
			t.Errorf("from %s: %v", tc.page, items[0])
		}
	}
}

// The calculator's pages, and the pages before a link, post to /feedback:
// kept as from the calculator or without a link, an address sending only so
// much an hour, and coming back only to a page of the app's.
func TestFeedbackWithoutALink(t *testing.T) {
	s := newTestServer(t, atTime(inDublin(2030, 1, 10, 12, 0)))
	h, site := s.h, maintainerLink(t, s)
	for _, tc := range []struct{ path, role, page string }{
		{"/", "Calculator", "Routine builder"},
		{"/compare", "Calculator", "Compare routines"},
		{"/view", "Calculator", "Viewing a routine"},
		{"/tariff-sheet", "Calculator", "Tariff sheet"},
		{"/requirements", "Calculator", "Requirements"},
		{"/competitions/new", "Without a link", "Making a competition"},
		{"/clubs/new", "Without a link", "Making a club"},
	} {
		action, _, ok := feedbackFormOf(t, do(t, h, http.MethodGet, tc.path, nil).Body.String())
		if !ok || action != "/feedback" {
			t.Errorf("%s posts to /feedback: %q %v", tc.path, action, ok)
			continue
		}
		if code, said := sendFeedbackFrom(t, h, tc.path, "From "+tc.path); code != http.StatusOK {
			t.Errorf("from %s: %d %q", tc.path, code, said)
		}
		if items := feedbackItems(t, h, site); items[0] != [4]string{tc.role, tc.page, "", "From " + tc.path} {
			t.Errorf("from %s: %v", tc.path, items[0])
		}
	}

	// A page that isn't one is kept as not known.
	postFeedback(t, h, "/feedback", url.Values{"text": {"Odd"}, "page": {"/clubs/member/abcdefghijklmnopqrstuv"}}, nil)
	if items := feedbackItems(t, h, site); items[0][1] != "Page not known" {
		t.Errorf("an unknown page: %v", items[0])
	}

	// Without JavaScript: back to the page, never to another site.
	for back, want := range map[string]string{"/compare": "/compare#feedback-sent", "//evil.example": "/#feedback-sent", "https://evil.example": "/#feedback-sent"} {
		if to := redirected(t, h, "/feedback", url.Values{"text": {"Hi"}, "page": {"/compare"}, "back": {back}}); to != want {
			t.Errorf("back %q: %s", back, to)
		}
	}

	// An address sends only so much in an hour.
	from := func(addr, text string) int {
		req := httptest.NewRequest(http.MethodPost, "/feedback", strings.NewReader(url.Values{"text": {text}, "page": {"/"}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Feedback", "fetch")
		req.RemoteAddr = addr
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	for range maxFeedbackPerHour {
		if code := from("203.0.113.9:1", "Again"); code != http.StatusOK {
			t.Fatalf("within the hour's: %d", code)
		}
	}
	if code := from("203.0.113.9:1", "One more"); code != http.StatusTooManyRequests {
		t.Errorf("one too many: %d", code)
	}
	if code := from("203.0.113.10:1", "Another address"); code != http.StatusOK {
		t.Errorf("another address: %d", code)
	}
	s.clock.set(inDublin(2030, 1, 10, 13, 1))
	if code := from("203.0.113.9:1", "Later"); code != http.StatusOK {
		t.Errorf("an hour later: %d", code)
	}
}

// The maintainer turns the button off, and it's gone from every page;
// sending anyway is refused. Never on the venue screen or the printouts.
func TestFeedbackButtonOff(t *testing.T) {
	s := newTestServer(t, atTime(inDublin(2030, 1, 10, 12, 0)))
	h, site := s.h, maintainerLink(t, s)
	admin := created(t, h, newCompetition())
	ucd := aClub(t, h, admin, "UCD", true, "Ann")
	ann := withoutQuery(formAction(t, memberCompetitionPage(t, h, ucd.members["Ann"]), ucd.members["Ann"]+"/competitions/"))
	own := anIndividual(t, h, admin, "Bea Byrne")
	pages := []string{"/", "/compare", admin, admin + "/settings", ann, ann + "/day", own, ucd.page}
	for _, p := range pages {
		if _, _, ok := feedbackFormOf(t, do(t, h, http.MethodGet, p, nil).Body.String()); !ok {
			t.Errorf("%s has the button while it's on", p)
		}
	}
	action, fields, _ := feedbackFormOf(t, do(t, h, http.MethodGet, ann, nil).Body.String())

	redirected(t, h, site+"/settings", url.Values{"button": {"off"}})
	for _, p := range pages {
		if _, _, ok := feedbackFormOf(t, do(t, h, http.MethodGet, p, nil).Body.String()); ok {
			t.Errorf("%s has no button once it's off", p)
		}
	}
	fields.Set("text", "Still here?")
	if rec := postFeedback(t, h, action, fields, nil); rec.Code != http.StatusForbidden {
		t.Errorf("sending once it's off: %d", rec.Code)
	}
	if !strings.Contains(do(t, h, http.MethodGet, site, nil).Body.String(), "not shown anywhere") {
		t.Error("the maintainer's page says it's off")
	}

	redirected(t, h, site+"/settings", url.Values{"button": {"on"}})
	if _, _, ok := feedbackFormOf(t, do(t, h, http.MethodGet, ann, nil).Body.String()); !ok {
		t.Error("back on")
	}

	redirected(t, h, admin+"/timetable/plan", url.Values{})
	for _, p := range []string{admin + "/screen", admin + "/timetable/print?sheet=scores", admin + "/cards"} {
		if _, _, ok := feedbackFormOf(t, do(t, h, http.MethodGet, p, nil).Body.String()); ok {
			t.Errorf("%s has no button", p)
		}
	}
}

// Empty or overlong feedback is refused, and the app takes only so much a
// day.
func TestFeedbackLimits(t *testing.T) {
	s := newTestServer(t, atTime(inDublin(2030, 1, 10, 12, 0)))
	h := s.h
	admin := created(t, h, newCompetition())
	own := anIndividual(t, h, admin, "Bea Byrne")
	for _, tc := range []struct {
		text string
		want int
	}{
		{"   ", http.StatusUnprocessableEntity},
		{strings.Repeat("é", maxFeedbackText+1), http.StatusUnprocessableEntity},
		{strings.Repeat("é", maxFeedbackText), http.StatusOK},
	} {
		if code, said := sendFeedbackFrom(t, h, own, tc.text); code != tc.want {
			t.Errorf("%d characters: %d %q", len([]rune(tc.text)), code, said)
		}
	}
	action, fields, _ := feedbackFormOf(t, do(t, h, http.MethodGet, own, nil).Body.String())
	fields.Set("text", "Hi")
	fields.Set("reply", strings.Repeat("a", maxFeedbackReply+1))
	if rec := postFeedback(t, h, action, fields, nil); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("an overlong reply-to: %d", rec.Code)
	}

	// A day's worth, then no more until a day has passed.
	fields.Del("reply")
	fields.Set("text", "Again")
	for range store.MaxSiteFeedbackPerDay - 1 {
		postFeedback(t, h, action, fields, nil)
	}
	if rec := postFeedback(t, h, action, fields, nil); rec.Code != http.StatusTooManyRequests {
		t.Errorf("one too many: %d", rec.Code)
	}
	s.clock.set(inDublin(2030, 1, 11, 12, 1))
	if rec := postFeedback(t, h, action, fields, nil); rec.Code != http.StatusOK {
		t.Errorf("the next day: %d", rec.Code)
	}
}

// aJPEG is enough of a JPEG for its type to be known.
var aJPEG = append([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0x10, 'J', 'F', 'I', 'F', 0}, make([]byte, 64)...)

func gzipped(t *testing.T, s string) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := gzip.NewWriter(&b)
	zw.Write([]byte(s))
	zw.Close()
	return b.Bytes()
}

// The maintainer's page: only with their link; each piece with its picture,
// its copy of the page (cleaned) and details; marked dealt with; Full or
// Light; what was captured deleted.
func TestMaintainerPage(t *testing.T) {
	s := newTestServer(t, atTime(inDublin(2030, 1, 10, 12, 0)))
	h, site := s.h, maintainerLink(t, s)
	admin := created(t, h, newCompetition())
	own := anIndividual(t, h, admin, "Bea Byrne")
	token := strings.TrimPrefix(own, "/competitions/entry/")

	for _, path := range []string{"/site/nope", "/site/", "/site/nope/feedback/x/page"} {
		if rec := do(t, h, http.MethodGet, path, nil); rec.Code != http.StatusNotFound {
			t.Errorf("%s without the link: %d", path, rec.Code)
		}
	}

	// Full: the picture, the copy and the details go with it.
	action, fields, _ := feedbackFormOf(t, do(t, h, http.MethodGet, own, nil).Body.String())
	fields.Set("text", "The button did nothing")
	fields.Set("details", `{"viewport":[390,844],"online":false,"queued":2,"browser":"Phone","errors":["12:00:01 failed `+own+`"],
		"recent":["/competitions/entry/{token}/day","`+own+`"],"open":["Check-in"],"scroll":[0,1200],"made":"up"}`)
	page := `<!DOCTYPE html><html><head><link rel="stylesheet" href="/static/css/styles.css?v=old"><link rel="stylesheet" href="https://evil.example/x.css">
		<script>alert(1)</script></head><body><a href="` + own + `/day" onclick="go()">My day</a>
		<form action="` + own + `/withdraw" method="post"><button formaction="/x">Withdraw</button></form>
		<iframe src="https://evil.example"></iframe><p hx-post="/x" x-data="{}">Hi ` + token + `</p></body></html>`
	rec := postFeedback(t, h, action, fields, map[string][]byte{"picture": aJPEG, "snapshot": gzipped(t, page)})
	if rec.Code != http.StatusOK {
		t.Fatalf("sent: %d %s", rec.Code, rec.Body)
	}
	all, err := s.st.AllSiteFeedback(t.Context())
	if err != nil || len(all) != 1 || !all[0].Picture || !all[0].Snapshot {
		t.Fatalf("kept with both: %+v %v", all, err)
	}
	if strings.Contains(all[0].Details, token) || strings.Contains(all[0].Details, `"made"`) || !strings.Contains(all[0].Details, `"open":["Check-in"]`) {
		t.Errorf("details kept, without the link or what isn't known: %s", all[0].Details)
	}
	if !strings.Contains(all[0].Details, `"recent":["/competitions/entry/{token}/day"]`) {
		t.Errorf("only pages' patterns before: %s", all[0].Details)
	}

	list := do(t, h, http.MethodGet, site, nil).Body.String()
	for _, want := range []string{"The button did nothing", "390 × 844", "offline", "2 taps", "Check-in", "My competition", "Full: a picture"} {
		if !strings.Contains(list, want) {
			t.Errorf("the page shows %q", want)
		}
	}
	id := all[0].ID
	picture := do(t, h, http.MethodGet, site+"/feedback/"+id+"/picture", nil)
	if picture.Code != http.StatusOK || picture.Header().Get("Content-Type") != "image/jpeg" || !bytes.Equal(picture.Body.Bytes(), aJPEG) {
		t.Errorf("the picture: %d %s", picture.Code, picture.Header().Get("Content-Type"))
	}
	copied := do(t, h, http.MethodGet, site+"/feedback/"+id+"/page", nil)
	if copied.Code != http.StatusOK || !strings.HasPrefix(copied.Header().Get("Content-Security-Policy"), "sandbox;") {
		t.Fatalf("the copy: %d %q", copied.Code, copied.Header().Get("Content-Security-Policy"))
	}
	body := copied.Body.String()
	for _, gone := range []string{token, "<script", "alert(1)", "onclick", "evil.example", "<iframe", "hx-post", "x-data", "formaction", `action="`, "v=old", "<link"} {
		if strings.Contains(body, gone) {
			t.Errorf("the copy has no %q: %s", gone, body)
		}
	}
	for _, kept := range []string{`<a href="#">My day</a>`, "<style>/* General styles", `id="feedback-scroll"`, `top:1200px`, "Withdraw", "Hi {token}"} {
		if !strings.Contains(body, kept) {
			t.Errorf("the copy has %q: %s", kept, body)
		}
	}

	// Dealt with: off the first tab, still under All.
	redirected(t, h, site+"/feedback/"+id+"/done", url.Values{"done": {"1"}})
	if strings.Contains(do(t, h, http.MethodGet, site, nil).Body.String(), "The button did nothing") {
		t.Error("dealt with, it's off the first tab")
	}
	if !strings.Contains(do(t, h, http.MethodGet, site+"?all=1", nil).Body.String(), "The button did nothing") {
		t.Error("still under All")
	}

	// Light: no copy of the page, and a picture only when ticked.
	redirected(t, h, site+"/settings", url.Values{"capture": {"light"}})
	_, lightFields, _ := feedbackFormOf(t, do(t, h, http.MethodGet, own, nil).Body.String())
	if !strings.Contains(do(t, h, http.MethodGet, own, nil).Body.String(), `name="with-picture"`) {
		t.Error("Light asks for the picture")
	}
	lightFields.Set("text", "Unticked")
	postFeedback(t, h, action, lightFields, map[string][]byte{"picture": aJPEG, "snapshot": gzipped(t, page)})
	lightFields.Set("text", "Ticked")
	lightFields.Set("with-picture", "1")
	postFeedback(t, h, action, lightFields, map[string][]byte{"picture": aJPEG, "snapshot": gzipped(t, page)})
	all, _ = s.st.AllSiteFeedback(t.Context())
	if all[0].Text != "Ticked" || !all[0].Picture || all[0].Snapshot || all[1].Picture || all[1].Snapshot {
		t.Errorf("Light keeps a ticked picture only: %+v", all[:2])
	}

	// Deleting them all keeps the words.
	redirected(t, h, site+"/captures/delete", url.Values{})
	all, _ = s.st.AllSiteFeedback(t.Context())
	for _, f := range all {
		if f.Picture || f.Snapshot {
			t.Errorf("deleted: %+v", f)
		}
	}
	if len(all) != 3 {
		t.Errorf("the words stay: %d", len(all))
	}
	if rec := do(t, h, http.MethodGet, site+"/feedback/"+id+"/page", nil); rec.Code != http.StatusNotFound {
		t.Errorf("the copy is gone: %d", rec.Code)
	}
}

// A copy of the page sent without gzip is taken too, and what can't be a
// picture is left off without refusing the words.
func TestFeedbackCaptureAsSent(t *testing.T) {
	s := newTestServer(t, atTime(inDublin(2030, 1, 10, 12, 0)))
	h := s.h
	rec := postFeedback(t, h, "/feedback", url.Values{"text": {"Hi"}, "page": {"/"}},
		map[string][]byte{"picture": []byte("<svg onload=alert(1)>"), "snapshot": []byte("<p>Plain</p>")})
	if rec.Code != http.StatusOK {
		t.Fatalf("sent: %d", rec.Code)
	}
	all, _ := s.st.AllSiteFeedback(t.Context())
	if all[0].Picture || !all[0].Snapshot {
		t.Fatalf("the copy, not the picture: %+v", all[0])
	}
	snapshot, _ := s.st.SiteFeedbackSnapshot(t.Context(), all[0].ID)
	zr, err := gzip.NewReader(bytes.NewReader(snapshot))
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := io.ReadAll(zr); !strings.Contains(string(b), "<p>Plain</p>") {
		t.Errorf("kept gzipped: %s", b)
	}
}

// emailCatcher is a mailer that hands each email over as it's sent, from
// whatever goroutine sends it.
type emailCatcher chan sentEmail

func (c emailCatcher) send(to, subject, body string, headers map[string]string) error {
	c <- sentEmail{to, subject, body, headers}
	return nil
}

// With FEEDBACK_EMAIL set, each piece of feedback is emailed there too:
// the words, the role, the page and a reply-to, never a link.
func TestFeedbackEmailed(t *testing.T) {
	s := newTestServer(t, atTime(inDublin(2030, 1, 10, 12, 0)))
	sent := make(emailCatcher, 1)
	s.p.notify.mail, s.p.feedbackTo = sent, "maintainer@example.com"
	h := s.h
	admin := created(t, h, newCompetition())
	own := anIndividual(t, h, admin, "Bea Byrne")
	action, fields, _ := feedbackFormOf(t, do(t, h, http.MethodGet, own, nil).Body.String())
	fields.Set("text", "Where's my time?")
	fields.Set("reply", "bea@example.com")
	fields.Set("details", `{"viewport":[390,844],"online":false}`)
	if rec := postFeedback(t, h, action, fields, map[string][]byte{"picture": aJPEG}); rec.Code != http.StatusOK {
		t.Fatalf("sent: %d", rec.Code)
	}
	select {
	case e := <-sent:
		if e.to != "maintainer@example.com" || e.subject != "Feedback: Their entries" || e.headers["Reply-To"] != "bea@example.com" {
			t.Errorf("the email: %+v", e)
		}
		for _, want := range []string{"Where's my time?", "Competition: Student Open", "From: Individual", "Page: Their entries (/competitions/gymnast/{token}/competitions/{id})",
			"Reply to: bea@example.com", "Browser: 390 × 844 window, offline", "A picture of the screen is on your feedback page."} {
			if !strings.Contains(e.body, want) {
				t.Errorf("the email says %q: %s", want, e.body)
			}
		}
		if strings.Contains(e.body, strings.TrimPrefix(own, "/competitions/entry/")) {
			t.Error("the email has no link")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no email")
	}
}
