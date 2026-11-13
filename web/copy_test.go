package web

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"tariffCalculator/store"
)

// copied is the admin link a copy opens at, from a copy's redirect.
func copiedTo(t *testing.T, h http.Handler, admin string, form url.Values) string {
	t.Helper()
	to := redirected(t, h, admin+"/copy", form)
	path := withoutQuery(to)
	return strings.TrimSuffix(path, "/settings")
}

// Settings has a row copying the competition; copying asks first on a page
// of its own, and the copy has the set-up, private, without deadlines.
func TestCopyingASetup(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	settings := do(t, h, http.MethodGet, admin+"/settings", nil).Body.String()
	for _, want := range []string{"Copy this competition", "Its set-up, for another competition, or with its entries, for a rehearsal",
		admin + "/copy?as=setup", admin + "/copy?as=rehearsal"} {
		if !strings.Contains(unescape(settings), want) {
			t.Errorf("Settings: no %q", want)
		}
	}
	ask := unescape(do(t, h, http.MethodGet, admin+"/copy?as=setup", nil).Body.String())
	for _, want := range []string{"Copy the set-up", "Copied", "Not copied", "The timetable's Setup", "Entries, clubs, coaches sent, officials and their offers",
		"Deadlines and when it goes live: none are set on the copy", `value="Student Open (copy)"`, `value="2031-03-16"`, "Make the copy"} {
		if !strings.Contains(ask, want) {
			t.Errorf("the ask: no %q", want)
		}
	}
	if rec := do(t, h, http.MethodPost, admin+"/copy", url.Values{"as": {"setup"}, "name": {""}, "date": {"2031-03-15"}}); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("no name: %d", rec.Code)
	}
	rec := do(t, h, http.MethodPost, admin+"/copy", url.Values{"as": {"setup"}, "name": {"Student Open 2031"}, "date": {"2031-03-15"}})
	if rec.Code != http.StatusSeeOther || !strings.HasSuffix(rec.Header().Get("Location"), "/settings?new=copied") {
		t.Fatalf("copied: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	copyAdmin := strings.TrimSuffix(withoutQuery(rec.Header().Get("Location")), "/settings")
	page := unescape(do(t, h, http.MethodGet, rec.Header().Get("Location"), nil).Body.String())
	for _, want := range []string{"Copy made.", "Save this page's link", "Student Open 2031", "not set yet", "Go live"} {
		if !strings.Contains(page, want) {
			t.Errorf("the copy's Settings: no %q", want)
		}
	}
	if strings.Contains(page, "Rehearsal copy of") {
		t.Error("a plain copy has no rehearsal line")
	}
	if to := redirected(t, h, copyAdmin+"/live", url.Values{"opens": {"now"}}); !strings.Contains(noticeOf(t, to), "Set when entries close first") {
		t.Errorf("going live without a deadline: %s", to)
	}
	if !historyHas(t, h, admin, "Organiser", "Copied this competition's set-up to Student Open 2031") {
		t.Error("the original's history says so")
	}
	if dash := do(t, h, http.MethodGet, copyAdmin, nil).Body.String(); strings.Contains(dash, "Ann") {
		t.Error("nobody in the copy")
	}
}

// The organiser's link and an everything link may copy; the others can't.
func TestWhoMayCopy(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	everything := madeLink(t, h, admin, "Co-organiser", "everything")
	if page := do(t, h, http.MethodGet, everything+"/settings", nil).Body.String(); !strings.Contains(page, "Copy this competition") {
		t.Error("a co-organiser sees the row")
	}
	copiedTo(t, h, everything, url.Values{"as": {"setup"}, "name": {"By the co-organiser"}, "date": {"2031-03-15"}})
	if !historyHas(t, h, admin, "Co-organiser", "Copied this competition's set-up to By the co-organiser") {
		t.Error("the history says who")
	}
	for _, kind := range []string{"cards", "chair", "timetable"} {
		l := madeLink(t, h, admin, "A "+kind, kind)
		for _, rec := range []int{do(t, h, http.MethodGet, l+"/copy?as=setup", nil).Code,
			do(t, h, http.MethodPost, l+"/copy", url.Values{"as": {"setup"}, "name": {"No"}, "date": {"2031-03-15"}}).Code} {
			if rec != http.StatusForbidden {
				t.Errorf("a %s link copying: %d", kind, rec)
			}
		}
	}
}

// A rehearsal copy has the entries; only its own admin link opens it, its
// pages say so, nobody's link sees it, and nothing is sent from it.
func TestRehearsalCopy(t *testing.T) {
	s := newTestServer(t, withMail())
	h, mail := s.h, s.mail
	admin := created(t, h, newCompetition())
	ucd := aClub(t, h, admin, "UCD", true, "Ann Ryan", "Bea Byrne")
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	gymnast := withoutQuery(redirected(t, h, enter, bucsEntry("Dee Doyle")))
	coach := pathIn(t, do(t, h, http.MethodPost, ucd.page+"/coaches", url.Values{"name": {"Cora Coach"}}).Body.String(), "/clubs/coach/")
	subscribeByEmail(t, h, mail, notifyPageOf(t, h, ucd.page, ucd.page), "ucd@example.com")
	redirected(t, h, admin+"/timetable/plan", url.Values{})
	redirected(t, h, admin+"/timetable/publish", url.Values{"action": {"publish"}})

	ask := unescape(do(t, h, http.MethodGet, admin+"/copy?as=rehearsal", nil).Body.String())
	for _, want := range []string{"Make a rehearsal copy", "Its entries as you have them", "Nobody can see it but you",
		"Nothing is emailed or pushed from it", "It's deleted 30 days after it's made", "Make the rehearsal copy"} {
		if !strings.Contains(ask, want) {
			t.Errorf("the ask: no %q", want)
		}
	}
	mail.sent = nil
	rec := do(t, h, http.MethodPost, admin+"/copy", url.Values{"as": {"rehearsal"}, "name": {"Rehearsal Open"}, "date": {"2030-03-16"}})
	if rec.Code != http.StatusSeeOther || !strings.HasSuffix(rec.Header().Get("Location"), "?new=rehearsal") {
		t.Fatalf("made: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	r := withoutQuery(rec.Header().Get("Location"))
	if !historyHas(t, h, admin, "Organiser", "Made a rehearsal copy") {
		t.Error("the original's history says so")
	}
	dash := unescape(do(t, h, http.MethodGet, rec.Header().Get("Location"), nil).Body.String())
	line := "Rehearsal copy of Student Open, made "
	for _, want := range []string{"Rehearsal copy made.", line, "nobody can see it but you, nothing is sent, and it's deleted on", "Ann Ryan", "Dee Doyle"} {
		if !strings.Contains(dash, want) {
			t.Errorf("its entries: no %q", want)
		}
	}
	for _, path := range []string{"/settings", "/timetable", "/day", "/cards", "/desk", "/screen", "/timetable/print"} {
		if page := unescape(do(t, h, http.MethodGet, r+path, nil).Body.String()); !strings.Contains(page, line) {
			t.Errorf("%s has no rehearsal line", path)
		}
	}
	if page := unescape(do(t, h, http.MethodGet, admin+"/settings", nil).Body.String()); strings.Contains(page, line) {
		t.Error("the original has no rehearsal line")
	}
	settings := unescape(do(t, h, http.MethodGet, r+"/settings", nil).Body.String())
	for _, want := range []string{"A rehearsal copy has no club link", "A rehearsal copy stays private", "Keep it 30 days longer", "Delete the rehearsal copy"} {
		if !strings.Contains(settings, want) {
			t.Errorf("its Settings: no %q", want)
		}
	}
	if strings.Contains(settings, "or with its entries, for a rehearsal") {
		t.Error("no rehearsal copy of a rehearsal copy")
	}
	if to := redirected(t, h, r+"/live", url.Values{"opens": {"now"}}); !strings.Contains(noticeOf(t, to), "A rehearsal copy stays private") {
		t.Errorf("going live: %s", to)
	}

	// Nobody else's link sees it, and it's on none of their pages.
	for _, page := range []string{ucd.page, ucd.page + "/competitions", ucd.members["Ann Ryan"], coach, gymnast} {
		if body := do(t, h, http.MethodGet, page, nil).Body.String(); strings.Contains(body, "Rehearsal Open") {
			t.Errorf("%s shows the rehearsal copy", page)
		}
	}
	if body := memberCompetitionPage(t, h, ucd.members["Ann Ryan"]); strings.Contains(body, "Rehearsal Open") || !strings.Contains(body, "Student Open") {
		t.Error("Ann's competition is the original")
	}
	for _, path := range []string{"/clubs/member/", "/clubs/coach/", "/competitions/gymnast/", "/clubs/"} {
		for _, link := range regexp.MustCompile(regexp.QuoteMeta(path)+`[A-Za-z0-9_-]{22}`).FindAllString(settings+dash, -1) {
			if body := do(t, h, http.MethodGet, link, nil).Body.String(); strings.Contains(body, "Rehearsal Open") {
				t.Errorf("a link in the copy opens it: %s", link)
			}
		}
	}

	// Nothing is sent from it: not to those who asked in the original, nor
	// to anyone asking in the copy.
	desk := unescape(do(t, h, http.MethodGet, r+"/desk", nil).Body.String())
	clubID := regexp.MustCompile(`<option value="([^"]+)"[^>]*>UCD</option>`).FindStringSubmatch(desk)
	if clubID == nil {
		t.Fatal("the copy's desk lists UCD")
	}
	c, _, err := s.st.AdminLink(context.Background(), strings.TrimPrefix(r, "/competitions/admin/"))
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := s.st.Subscribe(context.Background(), store.Subscription{CompetitionID: c.ID, Kind: store.NotifyClub, OwnerID: clubID[1], Channel: store.ByEmail, Address: "rehearsal@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.st.ConfirmSubscription(context.Background(), token); err != nil {
		t.Fatal(err)
	}
	mail.sent = nil
	redirected(t, h, r+"/desk", url.Values{"to": {"club"}, "club": {clubID[1]}, "text": {"Warm-up is moved to hall 2"}})
	redirected(t, h, r+"/notify-now", url.Values{})
	tellDue(s.p)
	if len(mail.sent) != 0 {
		t.Errorf("nothing is sent from a rehearsal copy: %+v", mail.sent)
	}
	if _, waiting, _ := s.st.NextDelivery(context.Background()); waiting {
		t.Error("nothing is kept to send from a rehearsal copy")
	}

	// Tapping on the day in the copy leaves the original's day as it was.
	orig, _, err := s.st.AdminLink(context.Background(), strings.TrimPrefix(admin, "/competitions/admin/"))
	if err != nil {
		t.Fatal(err)
	}
	before := do(t, h, http.MethodGet, admin+"/day?day=0", nil).Body.String()
	flight := regexp.MustCompile(`name="flight" value="([^"]+)"`).FindStringSubmatch(dayAll(t, h, r+"/day?day=0"))
	if flight == nil {
		t.Fatal("the copy's day has its flights")
	}
	redirected(t, h, r+"/day/flight", url.Values{"flight": {unescape(flight[1])}, "what": {"start"}, "day": {"0"}})
	if mine, _ := s.st.FlightTimes(context.Background(), c.ID); len(mine) != 1 {
		t.Errorf("the copy's flight started: %+v", mine)
	}
	if theirs, _ := s.st.FlightTimes(context.Background(), orig.ID); len(theirs) != 0 {
		t.Errorf("the original's flights aren't: %+v", theirs)
	}
	if after := do(t, h, http.MethodGet, admin+"/day?day=0", nil).Body.String(); stripTimes(after) != stripTimes(before) {
		t.Error("the original's day isn't changed by the copy's")
	}

	// Kept longer once; deleted when the organiser says.
	if to := redirected(t, h, r+"/keep-longer", url.Values{}); !strings.Contains(noticeOf(t, to), "Kept 30 days longer") {
		t.Errorf("kept longer: %s", to)
	}
	if to := redirected(t, h, r+"/keep-longer", url.Values{}); !strings.Contains(noticeOf(t, to), "kept longer already") {
		t.Errorf("once: %s", to)
	}
	if page := do(t, h, http.MethodPost, r+"/delete", url.Values{"confirm": {"delete"}}).Body.String(); !strings.Contains(page, "Rehearsal copy deleted") {
		t.Error("deleted")
	}
	if do(t, h, http.MethodGet, r, nil).Code != http.StatusNotFound {
		t.Error("its link leads nowhere once deleted")
	}
	if page := do(t, h, http.MethodGet, admin, nil).Body.String(); !strings.Contains(page, "Ann Ryan") {
		t.Error("the original is as it was")
	}
}

// stripTimes takes the page's own times out, which move with the clock.
func stripTimes(page string) string {
	return regexp.MustCompile(`content="[^"]*"|\d\d:\d\d(:\d\d)?`).ReplaceAllString(page, "")
}

// A finished competition's admin link opens its kept set-up: what it is,
// to copy or delete, and nothing else.
func TestKeptSetupPage(t *testing.T) {
	s := newTestServer(t, atTime(inDublin(2030, 1, 10, 12, 0)))
	h := s.h
	admin := created(t, h, newCompetition())
	aClub(t, h, admin, "UCD", true, "Ann Ryan")
	s.clock.set(time.Date(2030, 7, 14, 12, 0, 0, 0, time.UTC)) // 120 days after 16 March
	if _, _, err := s.st.DeleteExpired(context.Background()); err != nil {
		t.Fatal(err)
	}
	page := unescape(do(t, h, http.MethodGet, admin, nil).Body.String())
	for _, want := range []string{"Student Open was on 16 March 2030. Its entries and everything about the people in it were deleted on 14 July 2030; its set-up is kept for you to copy.",
		"Copy this set-up", "Delete it", "16 March 2033"} {
		if !strings.Contains(page, want) {
			t.Errorf("the kept set-up's page: no %q", want)
		}
	}
	if strings.Contains(page, "Ann Ryan") {
		t.Error("nobody on it")
	}
	if rec := do(t, h, http.MethodGet, admin+"/settings", nil); rec.Code != http.StatusFound || rec.Header().Get("Location") != admin {
		t.Errorf("its other pages go to it: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	if rec := do(t, h, http.MethodPost, admin+"/live", url.Values{"opens": {"now"}}); rec.Code != http.StatusConflict {
		t.Errorf("nothing else can be changed: %d", rec.Code)
	}
	if ask := do(t, h, http.MethodGet, admin+"/copy?as=rehearsal", nil); ask.Code != http.StatusSeeOther {
		t.Errorf("no rehearsal copy of a kept set-up: %d", ask.Code)
	}
	next := copiedTo(t, h, admin, url.Values{"as": {"setup"}, "name": {"Student Open 2031"}, "date": {"2031-03-15"}})
	if page := do(t, h, http.MethodGet, next+"/settings", nil).Body.String(); !strings.Contains(page, "Student Open 2031") {
		t.Error("the copy of the kept set-up")
	}
	if page := do(t, h, http.MethodPost, admin+"/delete", url.Values{"confirm": {"delete"}}).Body.String(); !strings.Contains(page, "Set-up deleted") {
		t.Error("deleted when the organiser says")
	}
}
