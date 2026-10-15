package web

import (
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
	box := firstByClass(parsePage(t, page), "comp-feedback")
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

// sendFeedbackFrom sends feedback with a page's Feedback button, as its
// script does, and returns the response.
func sendFeedbackFrom(t *testing.T, h http.Handler, path, text string) (int, string) {
	t.Helper()
	action, fields, ok := feedbackFormOf(t, do(t, h, http.MethodGet, path, nil).Body.String())
	if !ok {
		t.Fatalf("%s has a Feedback button", path)
	}
	fields.Set("text", text)
	req := httptest.NewRequest(http.MethodPost, action, strings.NewReader(fields.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Feedback", "fetch")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

// feedbackItems are the Feedback page's messages, newest first: role, page
// and what was said.
func feedbackItems(t *testing.T, h http.Handler, admin string) [][3]string {
	t.Helper()
	rec := do(t, h, http.MethodGet, admin+"/feedback", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("the Feedback page: %d", rec.Code)
	}
	var out [][3]string
	for _, item := range byClass(parsePage(t, rec.Body.String()), "comp-feedback-item") {
		role := textOf(firstByClass(item, "comp-feedback-role"))
		page := textOf(firstByClass(item, "comp-feedback-page"))
		out = append(out, [3]string{role, page, textOf(firstByClass(item, "comp-feedback-text"))})
	}
	return out
}

// Feedback can be sent from the pages of every kind of link, and is kept
// with the role and the page, never the link.
func TestFeedbackFromEveryLink(t *testing.T) {
	s := newTestServer(t, atTime(inDublin(2030, 1, 10, 12, 0)))
	h := s.h
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
	compSec := regexp.MustCompile(`href="(` + regexp.QuoteMeta(ucd.page) + `/members/[^"]+/competitions/[^"]+)"`).FindStringSubmatch(do(t, h, http.MethodGet, ucd.page, nil).Body.String())
	if compSec == nil {
		t.Fatal("the comp sec sees Ann's entry")
	}
	own := anIndividual(t, h, admin, "Bea Byrne")
	chair := madeLink(t, h, admin, "Chairs", "chair")

	for _, tc := range []struct {
		from, path, role, page string
	}{
		{"the organiser", admin, "Organiser", "Entries"},
		{"a helper link", chair, "Helper link: Chairs (chairs of judges)", "Entries"},
		{"a comp sec", unescape(compSec[1]), "Comp sec", "A member's entry"},
		{"a member", annPage, "Member", "Their entries"},
		{"a member's My competition", annPage + "/day", "Member", "My competition"},
		{"a coach", unescape(coachEntry[1]), "Coach", "Signing off an entry"},
		{"an individual", own, "Individual", "Their entry"},
	} {
		if code, said := sendFeedbackFrom(t, h, tc.path, "From "+tc.from); code != http.StatusOK || said != "Thanks: your feedback is sent." {
			t.Errorf("from %s: %d %q", tc.from, code, said)
		}
		items := feedbackItems(t, h, admin)
		if len(items) == 0 || items[0] != [3]string{tc.role, tc.page, "From " + tc.from} {
			t.Errorf("from %s, the newest is %v", tc.from, items[0])
		}
	}

	// What's kept is the page's pattern: no link, in any column.
	all, err := s.st.FeedbackFor(t.Context(), competitionIDOf(t, s, admin))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range all {
		for _, v := range []string{f.Text, f.ReplyTo, f.Role, f.Page} {
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
}

// competitionIDOf is the id of the competition an admin link opens.
func competitionIDOf(t *testing.T, s *testServer, admin string) string {
	t.Helper()
	c, err := s.st.CompetitionByAdmin(t.Context(), strings.TrimPrefix(admin, "/competitions/admin/"))
	if err != nil {
		t.Fatal(err)
	}
	return c.ID
}

// Entering pages have the button too: an individual entering, a comp sec
// entering their club, and a synchro partner confirming.
func TestFeedbackWhileEntering(t *testing.T) {
	h, _ := competitionServerAt(t, inDublin(2030, 1, 10, 12, 0))
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
		if items := feedbackItems(t, h, admin); items[0] != [3]string{tc.role, tc.page, "Hello"} {
			t.Errorf("from %s: %v", tc.page, items[0])
		}
	}
}

// The organiser turns the button off, and it's gone from every page; sending
// anyway is refused.
func TestFeedbackButtonOff(t *testing.T) {
	h, _ := competitionServerAt(t, inDublin(2030, 1, 10, 12, 0))
	admin := created(t, h, newCompetition())
	ucd := aClub(t, h, admin, "UCD", true, "Ann")
	ann := withoutQuery(formAction(t, memberCompetitionPage(t, h, ucd.members["Ann"]), ucd.members["Ann"]+"/competitions/"))
	own := anIndividual(t, h, admin, "Bea Byrne")
	pages := []string{admin, admin + "/settings", ann, ann + "/day", own}
	for _, p := range pages {
		if _, _, ok := feedbackFormOf(t, do(t, h, http.MethodGet, p, nil).Body.String()); !ok {
			t.Errorf("%s has the button while it's on", p)
		}
	}
	action, fields, _ := feedbackFormOf(t, do(t, h, http.MethodGet, ann, nil).Body.String())

	redirected(t, h, admin+"/feedback-button", url.Values{"on": {"0"}})
	for _, p := range pages {
		if _, _, ok := feedbackFormOf(t, do(t, h, http.MethodGet, p, nil).Body.String()); ok {
			t.Errorf("%s has no button once it's off", p)
		}
	}
	fields.Set("text", "Still here?")
	if rec := do(t, h, http.MethodPost, action, fields); rec.Code != http.StatusForbidden {
		t.Errorf("sending once it's off: %d", rec.Code)
	}
	if !historyHas(t, h, admin, "Organiser", "Turned the Feedback button off") {
		t.Error("the history says it was turned off")
	}

	redirected(t, h, admin+"/feedback-button", url.Values{"on": {"1"}})
	if _, _, ok := feedbackFormOf(t, do(t, h, http.MethodGet, ann, nil).Body.String()); !ok {
		t.Error("back on")
	}

	// Not on the venue screen or the printed sheets.
	redirected(t, h, admin+"/timetable/plan", url.Values{})
	for _, p := range []string{admin + "/screen", admin + "/timetable/print?sheet=scores", admin + "/cards"} {
		if _, _, ok := feedbackFormOf(t, do(t, h, http.MethodGet, p, nil).Body.String()); ok {
			t.Errorf("%s has no button", p)
		}
	}
}

// Empty or overlong feedback is refused, and a competition takes only so
// much a day.
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
	if rec := do(t, h, http.MethodPost, action, fields); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("an overlong reply-to: %d", rec.Code)
	}

	// A day's worth, then no more until a day has passed.
	fields.Del("reply")
	fields.Set("text", "Again")
	for range store.MaxFeedbackPerDay - 1 {
		do(t, h, http.MethodPost, action, fields)
	}
	if rec := do(t, h, http.MethodPost, action, fields); rec.Code != http.StatusTooManyRequests {
		t.Errorf("one too many: %d", rec.Code)
	}
	s.clock.set(inDublin(2030, 1, 11, 12, 1))
	if rec := do(t, h, http.MethodPost, action, fields); rec.Code != http.StatusSeeOther {
		t.Errorf("the next day: %d", rec.Code)
	}
}

// The organiser reads the feedback and marks it dealt with; a helper link
// that can't resolve concerns can't read it.
func TestReadingFeedback(t *testing.T) {
	h, _ := competitionServerAt(t, inDublin(2030, 1, 10, 12, 0))
	admin := created(t, h, newCompetition())
	own := anIndividual(t, h, admin, "Bea Byrne")
	cards := madeLink(t, h, admin, "Difficulty judges", "cards")
	mary := madeLink(t, h, admin, "Mary", "everything")
	sendFeedbackFrom(t, h, own, "Where do I see my time?")
	sendFeedbackFrom(t, h, cards, "The Check button is small")

	if items := feedbackItems(t, h, admin); len(items) != 2 || items[0][2] != "The Check button is small" || items[1][2] != "Where do I see my time?" {
		t.Errorf("newest first: %v", items)
	}
	if page := do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(); !strings.Contains(page, `href="`+admin+`/feedback"`) {
		t.Error("the organiser's menu has Feedback, beside Settings")
	}
	if rec := do(t, h, http.MethodGet, cards+"/feedback", nil); rec.Code != http.StatusForbidden {
		t.Errorf("checking cards can't read it: %d", rec.Code)
	}
	if page := do(t, h, http.MethodGet, cards+"/history", nil).Body.String(); strings.Contains(page, `href="`+cards+`/feedback"`) {
		t.Error("nor is it in their menu")
	}
	if rec := do(t, h, http.MethodGet, mary+"/feedback", nil); rec.Code != http.StatusOK {
		t.Errorf("a link for everything reads it: %d", rec.Code)
	}

	page := do(t, h, http.MethodGet, admin+"/feedback", nil).Body.String()
	done := regexp.MustCompile(`action="([^"]+/feedback/[^"]+/done)"`).FindAllStringSubmatch(page, -1)
	if len(done) != 2 {
		t.Fatalf("each can be marked: %v", done)
	}
	redirected(t, h, unescape(done[1][1]), url.Values{"done": {"1"}})
	items := byClass(parsePage(t, do(t, h, http.MethodGet, admin+"/feedback", nil).Body.String()), "comp-feedback-item")
	if hasClass(items[0], "is-done") || !hasClass(items[1], "is-done") || !strings.Contains(textOf(items[1]), "Dealt with (Organiser)") {
		t.Error("the older one is dealt with")
	}
	if !historyHas(t, h, admin, "Organiser", "Marked feedback dealt with") || !historyHas(t, h, admin, "Difficulty judges", "Sent feedback about the app") {
		t.Error("the history has both")
	}
	redirected(t, h, unescape(done[1][1]), url.Values{"done": {"0"}})
	if items := byClass(parsePage(t, do(t, h, http.MethodGet, admin+"/feedback", nil).Body.String()), "comp-feedback-item"); hasClass(items[1], "is-done") {
		t.Error("and not again")
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
	if rec := do(t, h, http.MethodPost, action, fields); rec.Code != http.StatusSeeOther {
		t.Fatalf("sent: %d", rec.Code)
	}
	select {
	case e := <-sent:
		if e.to != "maintainer@example.com" || e.subject != "Feedback: Student Open" || e.headers["Reply-To"] != "bea@example.com" {
			t.Errorf("the email: %+v", e)
		}
		for _, want := range []string{"Where's my time?", "From: Individual", "Page: Their entry (/competitions/entry/{token})", "Reply to: bea@example.com"} {
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
