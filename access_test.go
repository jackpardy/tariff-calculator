package main

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// madeLink makes an extra admin link and returns its path.
func madeLink(t *testing.T, h http.Handler, admin, name, kind string) string {
	t.Helper()
	rec := do(t, h, http.MethodPost, admin+"/links", url.Values{"name": {name}, "kind": {kind}})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "only shown this once") {
		t.Fatalf("making a link: %d", rec.Code)
	}
	return pathIn(t, rec.Body.String(), "/competitions/admin/")
}

func TestAdminLinks(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	enter := pathIn(t, do(t, h, http.MethodGet, admin, nil).Body.String(), "/competitions/enter/")
	redirected(t, h, enter, url.Values{"gymnast": {"Ann Ryan"}, "level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}})
	entry := strings.TrimPrefix(entryLink(t, h, admin), admin)
	redirected(t, h, admin+"/timetable/plan", url.Values{})

	cards := madeLink(t, h, admin, "Difficulty judges", "cards")
	chair := madeLink(t, h, admin, "Chairs", "chair")
	timetable := madeLink(t, h, admin, "Timetable team", "timetable")
	co := madeLink(t, h, admin, "Mary", "everything")
	if page := do(t, h, http.MethodGet, admin, nil).Body.String(); !strings.Contains(page, "<strong>Difficulty judges</strong> · checking cards") {
		t.Error("the organiser sees the links")
	}

	code := func(method, path string, form url.Values) int {
		t.Helper()
		return do(t, h, method, path, form).Code
	}
	for _, tc := range []struct {
		link, method, path string
		want               int
	}{
		// Checking cards: see the entries, check, review videos; nothing else.
		{cards, "GET", "", 200},
		{cards, "GET", entry, 200},
		{cards, "POST", entry + "/check", 303},
		{cards, "POST", "/remove", 403},
		{cards, "GET", "/timetable", 403},
		{cards, "POST", "/deadline", 403},
		{cards, "POST", "/links", 403},
		// Chairs: see, and print the sheets.
		{chair, "GET", "/timetable/print?sheet=scores", 200},
		{chair, "GET", "/timetable/print?sheet=judges", 200},
		{chair, "GET", "/timetable", 403},
		{chair, "POST", entry + "/check", 403},
		// Timetable and officials.
		{timetable, "GET", "/timetable", 200},
		{timetable, "GET", "/officials", 200},
		{timetable, "POST", "/timetable/plan", 303},
		{timetable, "POST", entry + "/check", 403},
		{timetable, "POST", "/deadline", 403},
		// Everything but links and deleting.
		{co, "POST", "/deadline", 303},
		{co, "POST", "/links", 403},
		{co, "POST", "/replace-link", 403},
		{co, "POST", "/delete", 403},
	} {
		form := url.Values{"checked": {"1"}, "close": {"1"}}
		if tc.method == "GET" {
			form = nil
		}
		if got := code(tc.method, tc.link+tc.path, form); got != tc.want {
			t.Errorf("%s %s by %s: %d, want %d", tc.method, tc.path, tc.link, got, tc.want)
		}
	}

	// The checker's dashboard offers only what they can do.
	page := do(t, h, http.MethodGet, cards, nil).Body.String()
	if !strings.Contains(page, "You're using the link for <strong>Difficulty judges</strong>") || strings.Contains(page, "Links and settings") ||
		strings.Contains(page, `aria-label="Tick Ann Ryan"`) || strings.Contains(page, ">Timetable</a>") {
		t.Error("no settings, removing or timetable for checking cards")
	}
	if page := do(t, h, http.MethodGet, chair, nil).Body.String(); !strings.Contains(page, ">Score sheets</a>") {
		t.Error("chairs get the sheets")
	}

	// A chair flags a concern about Ann's entry; the organiser resolves it.
	redirected(t, h, chair+"/concerns", url.Values{"entry": {strings.TrimPrefix(entry, "/entries/")}, "text": {"Element 7 isn't on the card"}})
	page = do(t, h, http.MethodGet, admin, nil).Body.String()
	if !strings.Contains(page, "1 open") || !strings.Contains(page, "Element 7 isn&#39;t on the card") && !strings.Contains(page, "Element 7 isn't on the card") || !strings.Contains(page, "Ann Ryan · BUCS L3</a>") {
		t.Fatalf("the organiser sees the concern: %s", page)
	}
	if strings.Contains(do(t, h, http.MethodGet, chair, nil).Body.String(), "/resolve") {
		t.Error("only the organiser resolves")
	}
	id := regexp.MustCompile(`/concerns/([^/"]+)/resolve`).FindStringSubmatch(page)[1]
	redirected(t, h, admin+"/concerns/"+id+"/resolve", url.Values{"note": {"Asked UCD to fix it"}})
	if page := do(t, h, http.MethodGet, admin, nil).Body.String(); !strings.Contains(page, "none open") || !strings.Contains(page, "Resolved by Organiser") {
		t.Error("resolved")
	}

	// The history says who did what.
	history := do(t, h, http.MethodGet, admin+"/history", nil).Body.String()
	for _, want := range []string{
		"<td>Difficulty judges</td><td>Marked a card checked: Ann Ryan · BUCS L3</td>",
		"<td>Organiser</td><td>Made a link for Difficulty judges (checking cards)</td>",
		"<td>Chairs</td><td>Flagged a concern</td>",
		"<td>Timetable team</td><td>Planned the timetable</td>",
		"<td>Mary</td><td>Closed entries</td>",
	} {
		if !strings.Contains(history, want) {
			t.Errorf("the history has %q", want)
		}
	}
	if strings.Contains(history, "Changed the settings (links)") {
		t.Error("refused changes aren't recorded")
	}

	// Removed, a link stops working.
	linkID := regexp.MustCompile(`/links/([^/"]+)/remove`).FindStringSubmatch(do(t, h, http.MethodGet, admin, nil).Body.String())[1]
	redirected(t, h, admin+"/links/"+linkID+"/remove", nil)
	if got := code("GET", cards, nil); got != http.StatusNotFound {
		t.Errorf("a removed link: %d", got)
	}
}
