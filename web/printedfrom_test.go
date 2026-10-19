package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// The printed sheets, the panel timeline and its CSV are of what's
// published, so the paper matches the phones; the draft only when asked for,
// marked on every page.
func TestPrintedFromPublished(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	redirected(t, h, enter, bucsEntry("Ann"))
	tt := admin + "/timetable"
	redirected(t, h, tt+"/setup/areas", url.Values{"area-0-name": {"Panel 1"}, "area-0-discipline": {"trampoline"}, "add": {"1"}})
	redirected(t, h, tt+"/setup/areas", url.Values{"area-0-name": {"Panel 1"}, "area-0-discipline": {"trampoline"}, "area-1-name": {"Panel 2"}, "area-1-discipline": {"trampoline"}})
	page := do(t, h, http.MethodGet, redirected(t, h, tt+"/plan", url.Values{}), nil).Body.String()
	chair := madeLink(t, h, admin, "Chair", "chair")
	scores := func(link string) (string, string) {
		t.Helper()
		page := do(t, h, http.MethodGet, link, nil).Body.String()
		m := regexp.MustCompile(`· (Panel \d) · [^·<]* · warm-up`).FindStringSubmatch(page)
		if m == nil {
			return "", page
		}
		return m[1], page
	}

	// Not published: the draft, marked, for the organiser and a chair alike.
	for _, link := range []string{tt, chair + "/timetable"} {
		if _, page := scores(link + "/print?sheet=scores"); !strings.Contains(page, "Draft · timetable not published") || strings.Contains(page, "Use what") {
			t.Errorf("%s: an unpublished timetable prints marked as a draft", link)
		}
	}

	redirected(t, h, tt+"/publish", url.Values{"action": {"publish"}})
	first, sheet := scores(tt + "/print?sheet=scores")
	if first == "" || strings.Contains(sheet, "comp-draft-mark") || strings.Contains(sheet, "Use the draft instead") {
		t.Fatalf("published, nothing changed: the sheet is of it, unmarked, with no draft to offer (%q)", first)
	}
	other := map[string]string{"Panel 1": "Panel 2", "Panel 2": "Panel 1"}[first]

	// Her flight moved in the draft: the sheets are still of what's published.
	flight := regexp.MustCompile(`name="flight" value="(\d+)"`).FindStringSubmatch(page)[1]
	redirected(t, h, tt+"/flight", url.Values{"flight": {flight}, "action": {"move"}, "to": {"0:" + other}})
	where, sheet := scores(tt + "/print?sheet=scores")
	if where != first || strings.Contains(sheet, "comp-draft-mark") {
		t.Errorf("with changes not published, the sheet is of what's published: %q", where)
	}
	if !strings.Contains(sheet, "Use the draft instead") || !strings.Contains(sheet, "print?draft=1&amp;sheet=scores") {
		t.Error("the sheet offers the draft")
	}
	if !strings.Contains(do(t, h, http.MethodGet, tt, nil).Body.String(), "These print what's published") {
		t.Error("the Print menu says the sheets are of what's published")
	}
	// Asked for, the draft, marked on every page.
	where, sheet = scores(tt + "/print?sheet=scores&draft=1")
	if where != other || !strings.Contains(sheet, "Draft · not published") || !strings.Contains(sheet, "Use what&#39;s published") {
		t.Errorf("the draft, marked: %q", where)
	}
	// A chair only ever gets what's published.
	if where, sheet := scores(chair + "/timetable/print?sheet=scores&draft=1"); where != first || strings.Contains(sheet, "Use the draft instead") {
		t.Errorf("a chair gets what's published: %q", where)
	}

	// The panel timeline and its CSV too.
	timeline := do(t, h, http.MethodGet, tt+"/timeline", nil).Body.String()
	if strings.Contains(timeline, "· DRAFT") || !strings.Contains(timeline, "/timeline?draft=1") {
		t.Error("the panel timeline is of what's published, offering the draft")
	}
	if draft := do(t, h, http.MethodGet, tt+"/timeline?officials=1&draft=1", nil).Body.String(); !strings.Contains(draft, "· DRAFT") || !strings.Contains(draft, "timeline.csv?draft=1") || !strings.Contains(draft, "/timeline?draft=1") {
		t.Error("the draft timeline is marked and keeps to the draft in its links")
	}
	if csv := do(t, h, http.MethodGet, tt+"/timeline.csv", nil).Body.String(); strings.HasPrefix(csv, "DRAFT") || !strings.Contains(csv, first) || strings.Contains(csv, other) {
		t.Errorf("the CSV is of what's published:\n%s", csv)
	}
	rec := do(t, h, http.MethodGet, tt+"/timeline.csv?draft=1", nil)
	if csv := rec.Body.String(); !strings.HasPrefix(csv, "DRAFT: changes not published") || !strings.Contains(rec.Header().Get("Content-Disposition"), "(draft)") {
		t.Errorf("the draft CSV says so first, and in its name:\n%s", csv)
	}
}
