package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"tariffCalculator/store"
)

// competitionServer is the app with storage in a fresh directory.
func competitionServer(t *testing.T) http.Handler {
	t.Helper()
	st, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return routesWith(st)
}

func do(t *testing.T, h http.Handler, method, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if form != nil {
		req = httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func newCompetition() url.Values {
	return url.Values{
		"name": {"Student Open"}, "date": {"2030-03-16"}, "deadlineDate": {"2030-03-09"}, "deadlineTime": {"23:59"},
		"individuals": {"1"}, "level": {"builtin-level:bucs-l3", "builtin-level:fig-ag3"},
	}
}

// created creates a competition and returns its admin link's path.
func created(t *testing.T, h http.Handler, form url.Values) string {
	t.Helper()
	rec := do(t, h, http.MethodPost, "/competitions", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("creating: %d %s", rec.Code, rec.Body.String())
	}
	return strings.TrimSuffix(rec.Header().Get("Location"), "?new=created")
}

// pathIn finds the first link in a page with this prefix.
func pathIn(t *testing.T, html, prefix string) string {
	t.Helper()
	m := regexp.MustCompile(`http://example\.com(` + regexp.QuoteMeta(prefix) + `[A-Za-z0-9_-]+)`).FindStringSubmatch(html)
	if m == nil {
		t.Fatalf("no %s link in the page", prefix)
	}
	return m[1]
}

const voluntary = `[{"rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck","backward":true},
	{"rotation":4,"twist_distribution":[1],"takeoff_position":"Feet","shape":"Tuck"}]`

func TestCompetitionPagesNeedStorage(t *testing.T) {
	rec := do(t, routes(), http.MethodGet, "/competitions/new", nil)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "aren&#39;t available on this server yet") {
		t.Errorf("without DATA_DIR the pages say storage is off: %d", rec.Code)
	}
	if rec := do(t, routes(), http.MethodGet, "/", nil); rec.Code != http.StatusOK {
		t.Error("the calculator works without storage")
	}
}

func TestCreateCompetition(t *testing.T) {
	h := competitionServer(t)
	page := do(t, h, http.MethodGet, "/competitions/new", nil)
	if body := page.Body.String(); page.Code != http.StatusOK || !strings.Contains(body, `value="builtin-level:bucs-l3"`) || !strings.Contains(body, `content="noindex"`) {
		t.Errorf("the form lists built-in levels: %d", page.Code)
	}
	if page.Header().Get("Referrer-Policy") != "no-referrer" || page.Header().Get("Cache-Control") != "no-store" {
		t.Error("competition pages don't leak links through referrers or caches")
	}

	bad := newCompetition()
	bad.Set("name", "")
	bad.Set("deadlineDate", "2020-01-01")
	bad.Del("level")
	rec := do(t, h, http.MethodPost, "/competitions", bad)
	if body := rec.Body.String(); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(body, "The competition needs a name.") ||
		!strings.Contains(body, "The closing time has already passed.") || !strings.Contains(body, "The competition needs at least one level.") {
		t.Errorf("problems are listed: %d %s", rec.Code, body)
	}

	custom := newCompetition()
	custom.Add("custom", `{"level":{"format":1,"name":"Club novice","first":{"options":["set-x"]}},"sets":{"set-x":{"format":1,"name":"Novice voluntary","rules":[]}}}`)
	admin := created(t, h, custom)
	if page := do(t, h, http.MethodGet, admin+"?new=created", nil).Body.String(); !strings.Contains(page, "Competition created.") || !strings.Contains(page, "http://example.com"+admin) {
		t.Error("just created, the dashboard says to save the admin link")
	}
	dash := do(t, h, http.MethodGet, admin, nil).Body.String()
	if strings.Contains(dash, "Competition created.") {
		t.Error("only just after creating")
	}
	for _, want := range []string{"Saturday 16 March 2030", "Entries close Saturday 9 March 2030, 23:59", "BUCS L3", "FIG AG3", "Club novice", "No entries yet"} {
		if !strings.Contains(dash, want) {
			t.Errorf("the dashboard should say %q", want)
		}
	}
	if rec := do(t, h, http.MethodGet, "/competitions/admin/wrong", nil); rec.Code != http.StatusNotFound {
		t.Errorf("a wrong admin link: %d", rec.Code)
	}

	// Replacing the admin link ends the old one.
	rec = do(t, h, http.MethodPost, admin+"/replace-link", url.Values{})
	fresh := strings.TrimSuffix(rec.Header().Get("Location"), "?new=replaced")
	if page := do(t, h, http.MethodGet, rec.Header().Get("Location"), nil).Body.String(); !strings.Contains(page, "Your new admin link.") {
		t.Error("the new link is shown")
	}
	if do(t, h, http.MethodGet, admin, nil).Code != http.StatusNotFound || do(t, h, http.MethodGet, fresh, nil).Code != http.StatusOK {
		t.Error("the replaced admin link stops working; the new one works")
	}

	// Deleting needs the box ticked.
	do(t, h, http.MethodPost, fresh+"/delete", url.Values{})
	if do(t, h, http.MethodGet, fresh, nil).Code != http.StatusOK {
		t.Error("not deleted without confirming")
	}
	do(t, h, http.MethodPost, fresh+"/delete", url.Values{"confirm": {"1"}})
	if do(t, h, http.MethodGet, fresh, nil).Code != http.StatusNotFound {
		t.Error("deleted")
	}
}

func TestCompetitionRateLimit(t *testing.T) {
	h := competitionServer(t)
	for i := range maxCompetitionsPerHour {
		if rec := do(t, h, http.MethodPost, "/competitions", newCompetition()); rec.Code != http.StatusSeeOther {
			t.Fatalf("competition %d: %d", i+1, rec.Code)
		}
	}
	if rec := do(t, h, http.MethodPost, "/competitions", newCompetition()); rec.Code != http.StatusTooManyRequests {
		t.Errorf("one address can create %d an hour: %d", maxCompetitionsPerHour, rec.Code)
	}
	req := httptest.NewRequest(http.MethodPost, "/competitions", strings.NewReader(newCompetition().Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Forwarded-For", "203.0.113.9, 198.51.100.7")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Errorf("another address (the rightmost forwarded one) can: %d", rec.Code)
	}
}

func TestIndividualEntry(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	enter := pathIn(t, do(t, h, http.MethodGet, admin, nil).Body.String(), "/competitions/enter/")

	form := do(t, h, http.MethodGet, enter, nil).Body.String()
	for _, want := range []string{"Enter Student Open", `value="builtin:bucs-l3-option-1"`, `data-set-routine="true"`, "deleted 14 July 2030"} {
		if !strings.Contains(form, want) {
			t.Errorf("the entry form should have %q", want)
		}
	}

	// A voluntary needs a routine.
	rec := do(t, h, http.MethodPost, enter, url.Values{"gymnast": {"C. Ryan"}, "level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}})
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Choose a routine for the second exercise.") {
		t.Errorf("a voluntary without a routine: %d", rec.Code)
	}

	rec = do(t, h, http.MethodPost, enter, url.Values{
		"gymnast": {"C. Ryan"}, "level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex1Skills": {"ignored for a set routine"},
		"ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("entered: %d %s", rec.Code, rec.Body.String())
	}
	own := strings.TrimSuffix(rec.Header().Get("Location"), "?saved=1")
	page := do(t, h, http.MethodGet, own+"?saved=1", nil).Body.String()
	for _, want := range []string{"Entry saved.", "C. Ryan", "Individual · BUCS L3", "set routine as written", "Tuck Back", "Change the entry", "Keep the routine entered (2 skills)"} {
		if !strings.Contains(page, want) {
			t.Errorf("the entry page should say %q", want)
		}
	}

	// The organiser sees it, checked, with its problems.
	dash := do(t, h, http.MethodGet, admin, nil).Body.String()
	if !strings.Contains(dash, "C. Ryan") || !strings.Contains(dash, "individual") || !strings.Contains(dash, "+") {
		t.Errorf("the dashboard lists the entry with its problems")
	}
	if problems := do(t, h, http.MethodGet, admin+"?problems=1", nil).Body.String(); !strings.Contains(problems, "C. Ryan") {
		t.Error("a two-skill voluntary has problems, so it's listed under problems only")
	}
	if other := do(t, h, http.MethodGet, admin+"?club=UCD", nil).Body.String(); strings.Contains(other, "C. Ryan") {
		t.Error("filtered to another club, it's not listed")
	}
	entry := regexp.MustCompile(`href="(` + regexp.QuoteMeta(admin) + `/entries/[A-Za-z0-9_-]+)"`).FindStringSubmatch(dash)
	if entry == nil {
		t.Fatal("no link to the entry")
	}
	if detail := do(t, h, http.MethodGet, entry[1], nil).Body.String(); !strings.Contains(detail, "First exercise") || !strings.Contains(detail, "Second exercise") || !strings.Contains(detail, "← All entries") {
		t.Error("the organiser sees both exercises")
	}

	// Changing it keeps the routine already entered.
	rec = do(t, h, http.MethodPost, own, url.Values{
		"gymnast": {"C. Ryan"}, "level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-2"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("changed: %d", rec.Code)
	}
	if page := do(t, h, http.MethodGet, own, nil).Body.String(); !strings.Contains(page, "option 2") {
		t.Error("the change is saved")
	}

	// Individual entry can be turned off.
	do(t, h, http.MethodPost, admin+"/individuals", url.Values{"on": {"0"}})
	if do(t, h, http.MethodGet, enter, nil).Code != http.StatusNotFound {
		t.Error("the individual link stops working")
	}

	do(t, h, http.MethodPost, own+"/withdraw", url.Values{"confirm": {"1"}})
	if do(t, h, http.MethodGet, own, nil).Code != http.StatusNotFound {
		t.Error("withdrawn")
	}
}

func TestOrganiserTools(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	enter := pathIn(t, do(t, h, http.MethodGet, admin, nil).Body.String(), "/competitions/enter/")
	entry := url.Values{
		"gymnast": {"=SUM(A1)"}, "level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"},
		"ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary},
	}
	own := withoutQuery(redirected(t, h, enter, entry))

	dash := do(t, h, http.MethodGet, admin, nil).Body.String()
	id := regexp.MustCompile(regexp.QuoteMeta(admin) + `/entries/([A-Za-z0-9_-]+)`).FindStringSubmatch(dash)[1]

	// Printing: two exercises, two cards, filled in.
	cards := do(t, h, http.MethodGet, admin+"/cards", nil).Body.String()
	if strings.Count(cards, `class="card-page"`) != 2 || !strings.Contains(cards, `value="=SUM(A1)"`) ||
		!strings.Contains(cards, `value="1st exercise · BUCS L3 · option 1"`) || !strings.Contains(cards, `value="Student Open"`) {
		t.Error("each exercise prints on its own card, with the gymnast, level, competition and exercise filled in")
	}
	if one := do(t, h, http.MethodGet, admin+"/cards?entry="+id, nil).Body.String(); strings.Count(one, `class="card-page"`) != 2 {
		t.Error("one entry's cards")
	}
	if other := do(t, h, http.MethodGet, admin+"/cards?level=FIG+AG3+%2817%E2%80%9321%29", nil).Body.String(); strings.Contains(other, `class="card-page"`) {
		t.Error("another level's cards don't include it")
	}

	// CSV, with formulas made harmless.
	rec := do(t, h, http.MethodGet, admin+"/entries.csv", nil)
	csv := rec.Body.String()
	if rec.Header().Get("Content-Type") != "text/csv; charset=utf-8" || !strings.Contains(rec.Header().Get("Content-Disposition"), `filename="Student-Open.csv"`) {
		t.Errorf("a CSV download: %v", rec.Header())
	}
	if !strings.HasPrefix(csv, "Gymnast,Club,Level,1st exercise,1st difficulty,2nd exercise,2nd difficulty,Problems,Checked,Note,Sent,Video,Withdrawn,Signed off by,Category\n") ||
		!strings.Contains(csv, "'=SUM(A1),Individual,BUCS L3,BUCS L3 · option 1,,BUCS L3 · second exercise,1.1,2,,,") {
		t.Errorf("CSV:\n%s", csv)
	}

	// Marking it checked, with a note the gymnast sees.
	redirected(t, h, admin+"/entries/"+id+"/check", url.Values{"checked": {"1"}, "note": {"Please add 8 elements"}})
	if page := do(t, h, http.MethodGet, admin+"/entries/"+id, nil).Body.String(); !strings.Contains(page, "✓ Checked") || !strings.Contains(page, "Please add 8 elements") {
		t.Error("the entry shows it's checked, with the note")
	}
	if dash := do(t, h, http.MethodGet, admin, nil).Body.String(); !strings.Contains(dash, `title="Please add 8 elements"`) {
		t.Error("the dashboard shows the check and note")
	}
	if page := do(t, h, http.MethodGet, own, nil).Body.String(); !strings.Contains(page, "Checked by the organiser.") || !strings.Contains(page, "Please add 8 elements") {
		t.Error("the gymnast sees the check and the note")
	}
	if none := do(t, h, http.MethodGet, admin+"/cards?unchecked=1", nil).Body.String(); !strings.Contains(none, "No entries to print.") {
		t.Error("nothing left unchecked to print")
	}
	// Changing the entry means checking it again.
	entry.Set("ex1Option", "builtin:bucs-l3-option-2")
	redirected(t, h, own, entry)
	if page := do(t, h, http.MethodGet, own, nil).Body.String(); strings.Contains(page, "Checked by the organiser.") || !strings.Contains(page, "Please add 8 elements") {
		t.Error("a changed entry is no longer checked; the note stays")
	}

	// Closing entries, and changing when they close.
	location := redirected(t, h, admin+"/deadline", url.Values{"close": {"1"}})
	if page := do(t, h, http.MethodGet, location, nil).Body.String(); !strings.Contains(page, "Entries are closed.") || !strings.Contains(page, "Entries closed") {
		t.Error("closed")
	}
	if rec := do(t, h, http.MethodPost, own, entry); rec.Code != http.StatusConflict {
		t.Errorf("no changes once closed: %d", rec.Code)
	}
	location = redirected(t, h, admin+"/deadline", url.Values{"deadlineDate": {"2030-03-20"}, "deadlineTime": {"12:00"}})
	if page := do(t, h, http.MethodGet, location, nil).Body.String(); !strings.Contains(page, "the closing time wasn") {
		t.Error("entries can't close after the competition date")
	}
	location = redirected(t, h, admin+"/deadline", url.Values{"deadlineDate": {"2030-03-12"}, "deadlineTime": {"18:00"}})
	if page := do(t, h, http.MethodGet, location, nil).Body.String(); !strings.Contains(page, "Entries now close Tuesday 12 March 2030, 18:00.") {
		t.Error("reopened")
	}
	redirected(t, h, own, entry)
}

func TestVideoProof(t *testing.T) {
	h := competitionServer(t)
	form := newCompetition()
	form.Set("video", "skills")
	if rec := do(t, h, http.MethodPost, "/competitions", form); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Video for some skills needs to say which.") {
		t.Errorf("video for some skills needs at least one: %d", rec.Code)
	}
	form.Set("video", "") // ticking a skill is enough
	form.Set("videoDoubles", "1")
	admin := created(t, h, form)
	dash := do(t, h, http.MethodGet, admin, nil).Body.String()
	if !strings.Contains(dash, "Video of any double somersault or more, as a link.") || !strings.Contains(dash, `name="videoDoubles" value="1" checked`) {
		t.Error("the dashboard says what video is asked for, and offers to change it")
	}
	enter := pathIn(t, dash, "/competitions/enter/")
	if page := do(t, h, http.MethodGet, enter, nil).Body.String(); !strings.Contains(page, `name="ex1Video"`) || !strings.Contains(page, "unlisted, not private") {
		t.Error("the entry form asks for video links, with the YouTube hint")
	}

	doubleBack := `[{"rotation":8,"twist_distribution":[0,0],"takeoff_position":"Feet","shape":"Tuck","backward":true}]`
	entry := url.Values{"gymnast": {"D"}, "level": {"FIG AG3 (17–21)"}, "ex1Skills": {doubleBack}, "ex2Skills": {voluntary}, "ex1Video": {"https://example.com/v"}}
	if rec := do(t, h, http.MethodPost, enter, entry); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "should be to YouTube") {
		t.Errorf("a link that isn't a video host is refused: %d", rec.Code)
	}
	entry.Del("ex1Video")
	own := withoutQuery(redirected(t, h, enter, entry))
	if page := do(t, h, http.MethodGet, own, nil).Body.String(); !strings.Contains(page, "Video needed for:") || !strings.Contains(page, "No video yet.") {
		t.Error("the gymnast sees which skill needs video")
	}
	dash = do(t, h, http.MethodGet, admin, nil).Body.String()
	if !strings.Contains(dash, "<th>Video</th>") || !strings.Contains(dash, ">missing</td>") {
		t.Error("the dashboard shows the video missing")
	}

	entry.Set("ex1Video", "https://youtu.be/abc")
	entry.Set("ex1VideoNote", "Double back at 0:12")
	redirected(t, h, own, entry)
	id := regexp.MustCompile(regexp.QuoteMeta(admin) + `/entries/([A-Za-z0-9_-]+)`).FindStringSubmatch(do(t, h, http.MethodGet, admin, nil).Body.String())[1]
	detail := do(t, h, http.MethodGet, admin+"/entries/"+id, nil).Body.String()
	if !strings.Contains(detail, `href="https://youtu.be/abc" target="_blank" rel="noopener noreferrer nofollow"`) || !strings.Contains(detail, "Double back at 0:12") {
		t.Error("the organiser opens the video in a new tab; it's never embedded")
	}
	if strings.Contains(detail, "<iframe") || strings.Contains(detail, "<video") {
		t.Error("no video is embedded")
	}
	if dash := do(t, h, http.MethodGet, admin, nil).Body.String(); !strings.Contains(dash, ">provided</td>") {
		t.Error("provided")
	}

	redirected(t, h, admin+"/entries/"+id+"/video", url.Values{"review": {"more"}, "note": {"The double isn't in it"}})
	if page := do(t, h, http.MethodGet, own, nil).Body.String(); !strings.Contains(page, "The organiser needs more video:") || !strings.Contains(page, "The double isn&#39;t in it") {
		t.Error("the gymnast sees what more is needed")
	}
	if dash := do(t, h, http.MethodGet, admin, nil).Body.String(); !strings.Contains(dash, ">need more</td>") {
		t.Error("need more")
	}
	redirected(t, h, admin+"/entries/"+id+"/video", url.Values{"review": {"ok"}})
	if page := do(t, h, http.MethodGet, own, nil).Body.String(); !strings.Contains(page, "Video accepted by the organiser.") {
		t.Error("accepted")
	}
	if rec := do(t, h, http.MethodPost, admin+"/entries/"+id+"/video", url.Values{"review": {"maybe"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("an unknown review: %d", rec.Code)
	}

	// The organiser can change what's asked for.
	location := redirected(t, h, admin+"/video", url.Values{"video": {"routine"}})
	if page := do(t, h, http.MethodGet, location, nil).Body.String(); !strings.Contains(page, "Video proof changed.") || !strings.Contains(page, "Video of each whole routine") {
		t.Error("changed to the whole routine")
	}
	location = redirected(t, h, admin+"/video", url.Values{"video": {"skills"}, "videoTariff": {"lots"}})
	if page := do(t, h, http.MethodGet, location, nil).Body.String(); !strings.Contains(page, "Video proof wasn") || !strings.Contains(page, "Video of each whole routine") {
		t.Error("a bad tariff changes nothing")
	}
}
