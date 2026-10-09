package main

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
)

func TestOnTheDay(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	if rec := do(t, h, http.MethodGet, admin+"/day", nil); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "Publish the timetable first") {
		t.Errorf("nothing to run until the timetable is published: %d", rec.Code)
	}
	enter := pathIn(t, do(t, h, http.MethodGet, admin, nil).Body.String(), "/competitions/enter/")
	for _, name := range []string{"Ann", "Bea", "Cal"} {
		redirected(t, h, enter, url.Values{"gymnast": {name}, "level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}})
	}
	redirected(t, h, admin+"/timetable/plan", url.Values{})
	if page := do(t, h, http.MethodGet, admin, nil).Body.String(); strings.Contains(page, ">On the day<") {
		t.Error("no On the day button before the timetable is published")
	}
	redirected(t, h, admin+"/timetable/publish", url.Values{"action": {"publish"}})
	if page := do(t, h, http.MethodGet, admin, nil).Body.String(); !strings.Contains(page, admin+"/day") {
		t.Error("an On the day button once it's published")
	}

	day := admin + "/day?day=0"
	page := do(t, h, http.MethodGet, day, nil).Body.String()
	if !strings.Contains(page, "BUCS L3") || !strings.Contains(page, ">Started</button>") || strings.Contains(page, ">Finished</button>") || strings.Contains(page, ">Undo</button>") {
		t.Errorf("each flight has a Started button only: %s", page)
	}
	if !strings.Contains(page, "Nothing started yet") || !strings.Contains(page, "Planned ") {
		t.Error("planned times, and nothing started yet")
	}
	key := regexp.MustCompile(`name="flight" value="([^"]+)"`).FindStringSubmatch(page)
	if key == nil {
		t.Fatal("no flight to mark")
	}
	flight := html2text(key[1])
	mark := func(what string) string {
		t.Helper()
		return redirected(t, h, admin+"/day/flight", url.Values{"flight": {flight}, "what": {what}, "day": {"0"}})
	}

	if to := mark("start"); !strings.HasPrefix(to, day) {
		t.Errorf("back to the day: %s", to)
	}
	page = do(t, h, http.MethodGet, day, nil).Body.String()
	if !strings.Contains(page, ">Finished</button>") || !strings.Contains(page, ">Undo</button>") || !strings.Contains(page, "Organiser") {
		t.Error("started: Finished and Undo are offered, and who marked it")
	}
	if !strings.Contains(page, " late") && !strings.Contains(page, " early") && !strings.Contains(page, "On time") {
		t.Errorf("the area says how it's running: %s", page)
	}
	mark("finish")
	page = do(t, h, http.MethodGet, day, nil).Body.String()
	if strings.Contains(page, "· finished —") || !strings.Contains(page, "measured from") {
		t.Errorf("finished shows its time: %s", page)
	}
	if strings.Contains(page, ">Finished</button>") {
		t.Error("a finished flight can't be finished again")
	}
	// Doing it twice does nothing, and says so.
	if to := mark("finish"); !strings.Contains(to, "notice=") {
		t.Errorf("already finished: %s", to)
	}
	mark("undo")
	page = do(t, h, http.MethodGet, day, nil).Body.String()
	if !strings.Contains(page, ">Finished</button>") || !strings.Contains(page, "· finished —") {
		t.Error("undo takes back the finish")
	}
	mark("undo")
	page = do(t, h, http.MethodGet, day, nil).Body.String()
	if !strings.Contains(page, "Nothing started yet") || strings.Contains(page, ">Undo</button>") {
		t.Error("undo again takes back the start")
	}
	if rec := do(t, h, http.MethodPost, admin+"/day/flight", url.Values{"flight": {"9|Nowhere|X||1"}, "what": {"start"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("not a flight: %d", rec.Code)
	}
	mark("start")

	// The history says what was marked, in words, once each.
	history := do(t, h, http.MethodGet, admin+"/history", nil).Body.String()
	for _, want := range []string{"Marked a flight started: BUCS L3", "Marked a flight finished: BUCS L3", "Undid a flight time: BUCS L3"} {
		if !strings.Contains(history, want) {
			t.Errorf("the history has %q", want)
		}
	}
	if n := strings.Count(history, "Marked a flight finished"); n != 1 {
		t.Errorf("the finish that did nothing isn't recorded again: %d", n)
	}

	// Who can: chairs and the timetable link, not the cards link.
	chair := madeLink(t, h, admin, "Chris", "chair")
	if rec := do(t, h, http.MethodGet, chair+"/day?day=0", nil); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<h1 class=\"title is-4\">On the day</h1>") {
		t.Errorf("a chair sees the day: %d", rec.Code)
	}
	if page := do(t, h, http.MethodGet, chair, nil).Body.String(); !strings.Contains(page, chair+"/day") {
		t.Error("a chair's dashboard offers On the day")
	}
	redirected(t, h, chair+"/day/flight", url.Values{"flight": {flight}, "what": {"finish"}, "day": {"0"}})
	if history := do(t, h, http.MethodGet, admin+"/history", nil).Body.String(); !strings.Contains(history, "Chris") {
		t.Error("the history says the chair did it")
	}
	if rec := do(t, h, http.MethodGet, chair+"/timetable/delay", nil); rec.Code != http.StatusForbidden {
		t.Errorf("a chair can't plan: %d", rec.Code)
	}
	if page := do(t, h, http.MethodGet, chair+"/day?day=0", nil).Body.String(); strings.Contains(page, "What if") {
		t.Error("a chair isn't sent to a page they can't use")
	}
	timetable := madeLink(t, h, admin, "Tina", "timetable")
	if rec := do(t, h, http.MethodGet, timetable+"/day", nil); rec.Code != http.StatusOK {
		t.Errorf("the timetable link sees the day: %d", rec.Code)
	}
	cards := madeLink(t, h, admin, "Carl", "cards")
	if rec := do(t, h, http.MethodGet, cards+"/day", nil); rec.Code != http.StatusForbidden {
		t.Errorf("a cards link can't see the day: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, cards+"/day/flight", url.Values{"flight": {flight}, "what": {"start"}}); rec.Code != http.StatusForbidden {
		t.Errorf("a cards link can't mark flights: %d", rec.Code)
	}
	if page := do(t, h, http.MethodGet, cards, nil).Body.String(); strings.Contains(page, ">On the day<") {
		t.Error("a cards link isn't offered On the day")
	}
}

// html2text undoes the escaping in an attribute.
func html2text(s string) string {
	return strings.NewReplacer("&#34;", `"`, "&amp;", "&", "&#39;", "'", "&lt;", "<", "&gt;", ">").Replace(s)
}

// A late area is told to attendees, and the day page links to the delay page
// filled in.
func TestLateNotes(t *testing.T) {
	now := time.Now().In(local)
	minutes := now.Hour()*60 + now.Minute()
	if minutes < 60 {
		t.Skip("too soon after midnight to plan a flight an hour ago")
	}
	st, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	p := newCompetitionPages(st)
	comp, _, err := st.CreateCompetition(context.Background(), competitions.Competition{
		Name: "Today", Date: now.Format(competitions.DateLayout), Deadline: now.Add(-time.Hour), LiveAt: now.Add(-time.Hour), Individuals: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Two flights on Panel 1, planned an hour before now and then an hour on;
	// the first only just started, so the panel is 60 minutes late; Panel 2
	// has nothing recorded.
	first := competitions.ScheduledFlight{Flight: competitions.Flight{Level: "BUCS L3", Number: 1, Of: 1}, Area: "Panel 1", Start: minutes - 60, End: minutes}
	second := competitions.ScheduledFlight{Flight: competitions.Flight{Level: "FIG AG3", Number: 1, Of: 1}, Area: "Panel 1", Start: minutes, End: minutes + 60}
	other := competitions.ScheduledFlight{Flight: competitions.Flight{Level: "Other", Number: 1, Of: 1}, Area: "Panel 2", Start: minutes - 60, End: minutes}
	comp.Published = &competitions.Schedule{
		Setup:   competitions.Setup{Days: []competitions.Day{{Name: "Today"}}, Areas: []competitions.Area{{Name: "Panel 1"}, {Name: "Panel 2"}}},
		Flights: []competitions.ScheduledFlight{first, second, other},
	}
	if err := st.MarkFlight(context.Background(), comp.ID, competitions.FlightKey(first), "start", "Marshal"); err != nil {
		t.Fatal(err)
	}
	notes, err := p.lateNotes(context.Background(), comp, []competitions.ScheduledFlight{second, first, other})
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 || !regexp.MustCompile(`^Panel 1 is running about 6\d min late \(as of \d\d:\d\d\)$`).MatchString(notes[0]) {
		t.Errorf("one line for Panel 1, once: %q", notes)
	}
	if notes, _ := p.lateNotes(context.Background(), comp, []competitions.ScheduledFlight{other}); len(notes) != 0 {
		t.Errorf("Panel 2 has nothing recorded: %q", notes)
	}
	comp.Date = now.AddDate(0, 0, 1).Format(competitions.DateLayout)
	if notes, _ := p.lateNotes(context.Background(), comp, []competitions.ScheduledFlight{first}); len(notes) != 0 {
		t.Errorf("only on the day: %q", notes)
	}
}
