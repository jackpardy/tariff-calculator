package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"tariffCalculator/views"
)

func TestMyCompetition(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	clubLink := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/club/")
	club := withoutQuery(redirected(t, h, "/clubs", url.Values{"name": {"UCD"}}))
	redirected(t, h, clubLink, url.Values{"clubAdmin": {club}})
	join := pathIn(t, do(t, h, http.MethodGet, club+"/members", nil).Body.String(), "/clubs/join/")
	member := withoutQuery(redirected(t, h, join, url.Values{"name": {"Mo"}}))
	save := formAction(t, memberCompetitionPage(t, h, member), member+"/competitions/")
	redirected(t, h, save, bucsEntry(""))

	m := regexp.MustCompile(`href="(` + regexp.QuoteMeta(member) + `/competitions/[^"/]+/day)"`).FindStringSubmatch(memberCompetitionPage(t, h, member))
	if m == nil {
		t.Fatal("the member page links to their competition")
	}
	day := m[1]
	if page := do(t, h, http.MethodGet, day, nil).Body.String(); !strings.Contains(page, "No entries for you have reached the competition yet") {
		t.Error("nothing sent yet")
	}
	redirected(t, h, formAction(t, do(t, h, http.MethodGet, club, nil).Body.String(), club+"/competitions/"), url.Values{"which": {"all"}})

	// An individual who judges, in the same event.
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	own := withoutQuery(redirected(t, h, enter, bucsEntry("Ivy")))
	redirected(t, h, admin+"/officials/add", url.Values{"name": {"Mary"}, "judge-trampoline": {"1"}, "chair-trampoline": {"1"}})

	page := do(t, h, http.MethodGet, day, nil).Body.String()
	if !strings.Contains(page, "2 problems to sort out") || !strings.Contains(page, "Second exercise: Exactly 10 elements") || !strings.Contains(page, "The timetable isn't published yet") {
		i := strings.Index(page, "<h1")
		t.Errorf("sent, not yet published: %s", page[i:min(len(page), i+1500)])
	}
	tt := admin + "/timetable"
	redirected(t, h, tt+"/setup/days", url.Values{"day-0-name": {"Saturday"}, "day-0-start": {"09:00"}, "day-0-end": {"18:00"}})
	redirected(t, h, tt+"/plan", url.Values{})
	redirected(t, h, tt+"/publish", url.Values{"on": {"1"}})

	page = do(t, h, http.MethodGet, day, nil).Body.String()
	for _, want := range []string{"<strong>BUCS L3</strong> · Panel 1 · Saturday", "Warm-up <strong>09:00</strong>", "of 2", "Routines at about 09:1", "<strong>Mo</strong>", "Chair of judges: Mary"} {
		if !strings.Contains(page, want) {
			t.Errorf("the member's page has %q", want)
		}
	}
	if strings.Contains(page, "<strong>Ivy</strong>") {
		t.Error("only the member is picked out")
	}
	if page := do(t, h, http.MethodGet, own, nil).Body.String(); !strings.Contains(page, own+"/day") {
		t.Error("the individual's entry links to their competition")
	}
	if page := do(t, h, http.MethodGet, own+"/day", nil).Body.String(); !strings.Contains(page, "<strong>Ivy</strong>") || !strings.Contains(page, ": your entry</a>") {
		t.Error("the individual's competition")
	}
}

// Once the timetable is published, My competition lists someone's events
// in the order they warm up, a later day after an earlier one, and any not
// in a flight last.
func TestMyEventsByWarmUp(t *testing.T) {
	events := []views.MyEvent{{ID: "tumbling"}, {ID: "none"}, {ID: "synchro"}, {ID: "trampoline"}}
	byWarmUp(events, map[string]int{"tumbling": 1440 + 9*60, "synchro": 14 * 60, "trampoline": 9*60 + 30})
	var got []string
	for _, ev := range events {
		got = append(got, ev.ID)
	}
	if strings.Join(got, " ") != "trampoline synchro tumbling none" {
		t.Errorf("by warm-up: %v", got)
	}
}
