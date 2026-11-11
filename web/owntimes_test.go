package web

import (
	"encoding/csv"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"tariffCalculator/competitions"
)

// flightTimesSetup is a planned timetable of two BUCS L3 flights of three on
// Panel 1, on a day ending at 10:30: 09:00–09:25 and 09:25–09:50, each a
// 10-minute changeover, no warm-up and 15 minutes competing. It's published
// if publish.
func flightTimesSetup(t *testing.T, publish bool) (h http.Handler, clock *testClock, admin, tt string) {
	t.Helper()
	h, clock = competitionServerAt(t, dayBefore)
	admin = created(t, h, newCompetition())
	tt = admin + "/timetable"
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	for _, name := range []string{"Ann", "Bea", "Cal", "Dee", "Eve", "Fay"} {
		redirected(t, h, enter, bucsEntry(name))
	}
	redirected(t, h, tt+"/setup/days", url.Values{"day-0-name": {"Saturday"}, "day-0-start": {"09:00"}, "day-0-end": {"10:30"}})
	redirected(t, h, tt+"/setup/timings", timingsWith(nil))
	redirected(t, h, tt+"/plan", url.Values{})
	if publish {
		redirected(t, h, tt+"/publish", url.Values{"action": {"publish"}})
	}
	return h, clock, admin, tt
}

// timingsWith is Setup's timings form as flightTimesSetup has it (flights
// of three), with some fields changed.
func timingsWith(change url.Values) url.Values {
	f := url.Values{"per-trampoline": {"5"}, "between-trampoline": {"10"}, "max-trampoline": {"3"}, "rest": {"0"}, "restMust": {"0"}}
	for k, v := range change {
		f[k] = v
	}
	return f
}

// flightForm is a flight's Change the times form on the timetable page: its
// index and the draft's version.
func flightForm(t *testing.T, page, name string) (flight, version string) {
	t.Helper()
	i := strings.Index(page, "<strong>"+name+"</strong>")
	if i < 0 {
		t.Fatalf("no %s on the page", name)
	}
	m := regexp.MustCompile(`/timetable/times"[^>]*>\s*<input type="hidden" name="version" value="([^"]+)"[^>]*>\s*<input type="hidden" name="flight" value="(\d+)"`).FindStringSubmatch(page[i:])
	if m == nil {
		t.Fatalf("%s has no Change the times", name)
	}
	return m[2], m[1]
}

// timesPost is a flight's times changed: warm-up and competing in minutes.
func timesPost(t *testing.T, h http.Handler, tt, name, warmUp, competing string, more url.Values) *http.Response {
	t.Helper()
	flight, version := flightForm(t, do(t, h, http.MethodGet, tt, nil).Body.String(), name)
	form := url.Values{"flight": {flight}, "version": {version}, "warmUp": {warmUp}, "competing": {competing}, "action": {"save"}}
	for k, v := range more {
		form[k] = v
	}
	return do(t, h, http.MethodPost, tt+"/times", form).Result()
}

const (
	first  = "BUCS L3 · flight 1 of 2"
	second = "BUCS L3 · flight 2 of 2"
)

func TestSetupTimingsOwnTimes(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	tt := admin + "/timetable"
	page := do(t, h, http.MethodGet, tt+"/setup", nil).Body.String()
	for _, want := range []string{"Warm-up per competitor", "Least warm-up", "Competing per competitor", "Changeover", "Largest flight",
		`name="warm-trampoline" value="0"`, `name="least-trampoline" value="0"`, `name="per-trampoline" value="5"`, `name="between-trampoline" value="10"`,
		"Add an event's own times", "then its event's, then its discipline's"} {
		if !strings.Contains(page, want) {
			t.Errorf("Setup's timings should have %q", want)
		}
	}

	if !strings.Contains(addChoices(page), `<option value="BUCS L3">`) {
		t.Error("BUCS L3 can have times of its own")
	}

	// The five figures saved.
	redirected(t, h, tt+"/setup/timings", timingsWith(url.Values{"warm-trampoline": {"1.5"}, "least-trampoline": {"8"}}))
	page = do(t, h, http.MethodGet, tt+"/setup", nil).Body.String()
	if !strings.Contains(page, `name="warm-trampoline" value="1.5"`) || !strings.Contains(page, `name="least-trampoline" value="8"`) || !strings.Contains(page, "Trampoline 1.5 warm-up and 5 competing") {
		t.Error("the warm-up figures are saved and shown")
	}
	// A form from before (no warm-up fields) leaves them as they are.
	redirected(t, h, tt+"/setup/timings", timingsWith(url.Values{"per-trampoline": {"4"}}))
	page = do(t, h, http.MethodGet, tt+"/setup", nil).Body.String()
	if !strings.Contains(page, `name="warm-trampoline" value="1.5"`) || !strings.Contains(page, `name="per-trampoline" value="4"`) {
		t.Error("an old form keeps the warm-up figures")
	}

	// An event's own times: added from the discipline's, changed, removed.
	location := redirected(t, h, tt+"/setup/timings", timingsWith(url.Values{"warm-trampoline": {"1.5"}, "least-trampoline": {"8"}, "per-trampoline": {"4"}, "add-trampoline": {"BUCS L3"}, "addEvent": {"trampoline"}}))
	if noticeOf(t, location) != "BUCS L3 has times of its own: change them on its row." {
		t.Errorf("added: %s", location)
	}
	page = do(t, h, http.MethodGet, tt+"/setup", nil).Body.String()
	if !strings.Contains(page, `name="event-0" value="BUCS L3"`) || !strings.Contains(page, `name="event-0-warm" value="1.5"`) || !strings.Contains(page, `name="event-0-per" value="4"`) ||
		strings.Contains(addChoices(page), "BUCS L3") || !strings.Contains(page, "1 event with times of its own") {
		t.Error("BUCS L3 has a row of its own, starting from trampoline's")
	}
	redirected(t, h, tt+"/setup/timings", timingsWith(url.Values{"event-0": {"BUCS L3"}, "event-0-warm": {"2"}, "event-0-least": {"0"}, "event-0-per": {"6"}}))
	page = do(t, h, http.MethodGet, tt+"/setup", nil).Body.String()
	if !strings.Contains(page, `name="event-0-per" value="6"`) || !strings.Contains(page, `name="warm-trampoline" value="1.5"`) {
		t.Error("its own times change; trampoline's stay")
	}
	if location := redirected(t, h, tt+"/setup/timings", timingsWith(url.Values{"event-0": {"BUCS L3"}, "event-0-warm": {"40"}, "event-0-per": {"6"}})); !strings.Contains(location, "Nothing+was+changed") {
		t.Errorf("out of range is refused: %s", location)
	}
	location = redirected(t, h, tt+"/setup/timings", timingsWith(url.Values{"event-0": {"BUCS L3"}, "event-0-warm": {"2"}, "event-0-per": {"6"}, "removeEvent": {"BUCS L3"}}))
	if noticeOf(t, location) != "BUCS L3 is back on its discipline's times." {
		t.Errorf("removed: %s", location)
	}
	if page = do(t, h, http.MethodGet, tt+"/setup", nil).Body.String(); strings.Contains(page, `name="event-0"`) {
		t.Error("its row is gone")
	}
}

// Each page and sheet showing a flight's times says when competing starts.
func TestThreeTimes(t *testing.T) {
	h, _, admin, tt := flightTimesSetup(t, false)
	// Warm-up: a minute a competitor, at least 5: each flight warms up 5.
	redirected(t, h, tt+"/setup/timings", timingsWith(url.Values{"warm-trampoline": {"1"}, "least-trampoline": {"5"}}))
	redirected(t, h, tt+"/publish", url.Values{"action": {"publish"}})
	line := "Warm-up 09:00 · competing 09:15 · finishes about 09:30"
	for _, c := range []struct{ what, path, want string }{
		{"the timetable", tt, line},
		{"the marshal sheets", tt + "/print?sheet=marshal", line},
		{"the chair of judges sheets", tt + "/print?sheet=judges", line},
		{"the panel timeline", tt + "/timeline", "09:00–09:30 · competing 09:15"},
		{"the timeline CSV", tt + "/timeline.csv", "09:00,09:15,09:30,BUCS L3 · flight 1 of 2"},
		{"On the day", admin + "/day?day=0&area=Panel+1", "Planned: warm-up 09:00 · competing 09:15 · finishes about 09:30"},
		{"the venue screen", admin + "/screen?day=0", "competing 09:15"},
	} {
		if body := do(t, h, http.MethodGet, c.path, nil).Body.String(); !strings.Contains(body, c.want) {
			t.Errorf("%s should say %q", c.what, c.want)
		}
	}
	if scores := do(t, h, http.MethodGet, tt+"/print?sheet=scores", nil).Body.String(); !regexp.MustCompile(`warm-up 09:00\s*· competing 09:15`).MatchString(scores) {
		t.Error("the score sheets give competing")
	}
	// A gymnast's own page.
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	own := withoutQuery(redirected(t, h, enter, bucsEntry("Gus")))
	redirected(t, h, tt+"/plan", url.Values{"anyway": {"1"}})
	redirected(t, h, tt+"/publish", url.Values{"action": {"publish"}})
	if page := do(t, h, http.MethodGet, own, nil).Body.String(); !regexp.MustCompile(`warm-up Saturday \d\d:\d\d\s*· competing \d\d:\d\d`).MatchString(page) {
		t.Error("a gymnast's page gives competing")
	}
}

func TestChangeFlightTimes(t *testing.T) {
	h, _, admin, tt := flightTimesSetup(t, true)
	page := do(t, h, http.MethodGet, tt, nil).Body.String()
	for _, want := range []string{"Change the times", "the averages give 0", "the averages give 15"} {
		if !strings.Contains(page, want) {
			t.Errorf("a flight's box should have %q", want)
		}
	}
	if strings.Contains(page, "Back to the averages") || strings.Contains(page, "its own times") {
		t.Error("no own times yet")
	}

	// It fits: saved, saying what moved, in the draft only.
	res := timesPost(t, h, tt, first, "0", "25", nil)
	if res.StatusCode != http.StatusSeeOther || noticeOf(t, res.Header.Get("Location")) != "Times saved. 1 flight on Panel 1 starts 10 minutes later." {
		t.Fatalf("fits: %d %s", res.StatusCode, res.Header.Get("Location"))
	}
	page = do(t, h, http.MethodGet, tt, nil).Body.String()
	for _, want := range []string{"Warm-up 09:00 · competing 09:10 · finishes about 09:35", "its own times", "Back to the averages", "Warm-up 09:35 · competing 09:45 · finishes about 10:00", "Changes not published"} {
		if !strings.Contains(page, want) {
			t.Errorf("after the change, the timetable should say %q", want)
		}
	}
	if marshal := do(t, h, http.MethodGet, tt+"/print?sheet=marshal", nil).Body.String(); !strings.Contains(marshal, "finishes about 09:25") {
		t.Error("what's published is as it was until published")
	}
	if !historyHas(t, h, admin, "Organiser", "Changed a flight's times") {
		t.Error("the history says so")
	}
	// Moving a gymnast keeps the flight's own times.
	m := regexp.MustCompile(`data-entry="([^"]+)"`).FindStringSubmatch(page[strings.Index(page, "<strong>"+second+"</strong>"):])
	flight, version := flightForm(t, page, first)
	location := redirected(t, h, tt+"/entry", url.Values{"entry": {m[1]}, "to": {flight}, "version": {version}, "anyway": {"1"}})
	if n := noticeOf(t, location); !strings.HasSuffix(n, first+" keeps its own times.") {
		t.Errorf("the notice says it keeps them: %s", n)
	}
	if page = do(t, h, http.MethodGet, tt, nil).Body.String(); !strings.Contains(page, "its own times") || !strings.Contains(page, "competing 09:10 · finishes about 09:35") {
		t.Error("a gymnast moved in, the flight keeps its own times")
	}

	// Back to the averages.
	res = timesPost(t, h, tt, first, "", "", url.Values{"action": {"averages"}})
	if !strings.HasPrefix(noticeOf(t, res.Header.Get("Location")), "Back on the averages.") {
		t.Errorf("back on the averages: %s", res.Header.Get("Location"))
	}
	if strings.Contains(do(t, h, http.MethodGet, tt, nil).Body.String(), "its own times") {
		t.Error("no own times now")
	}

	// A form from a draft changed since is refused, and not recorded.
	flight, _ = flightForm(t, do(t, h, http.MethodGet, tt, nil).Body.String(), first)
	location = redirected(t, h, tt+"/times", url.Values{"flight": {flight}, "version": {"old"}, "warmUp": {"5"}, "competing": {"15"}, "action": {"save"}})
	if noticeOf(t, location) != string(staleTimetable) {
		t.Errorf("stale: %s", location)
	}
	if location := redirected(t, h, tt+"/times", url.Values{"flight": {flight}, "warmUp": {"500"}, "competing": {"15"}, "action": {"save"}}); !strings.Contains(location, "Nothing+was+changed") {
		t.Errorf("out of range: %s", location)
	}

	// The timetable helper's link may change the times; the cards link may not.
	timetable, cards := madeLink(t, h, admin, "Timetable team", "timetable"), madeLink(t, h, admin, "Judges", "cards")
	form := url.Values{"flight": {flight}, "warmUp": {"0"}, "competing": {"16"}, "action": {"save"}}
	if rec := do(t, h, http.MethodPost, timetable+"/timetable/times", form); rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "Times+saved") {
		t.Errorf("the timetable link: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	if rec := do(t, h, http.MethodPost, cards+"/timetable/times", form); rec.Code != http.StatusForbidden {
		t.Errorf("the cards link: %d", rec.Code)
	}
}

func TestFlightTimesDontFit(t *testing.T) {
	h, _, admin, tt := flightTimesSetup(t, true)
	// 60 minutes competing: the second flight would finish at 10:35, past
	// the day's end.
	res := timesPost(t, h, tt, first, "0", "60", nil)
	body := readBody(t, res)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("asks first: %d", res.StatusCode)
	}
	for _, want := range []string{"Before you change it", "Change the times of " + first, second, "Plan again with these times", "Make the change anyway", "Leave it", "Planning again starts afresh"} {
		if !strings.Contains(body, want) {
			t.Errorf("the ask should have %q", want)
		}
	}
	if historyHas(t, h, admin, "Organiser", "Changed a flight's times") {
		t.Error("asking changes nothing")
	}
	// Leave it: back to the timetable, nothing changed.
	if !strings.Contains(body, `href="`+tt+`#day-0-area-Panel_201"`) || strings.Contains(do(t, h, http.MethodGet, tt, nil).Body.String(), "its own times") {
		t.Error("Leave it goes back, nothing changed")
	}
	fields := planFields(t, body)
	if fields.Get("plan") != "1" {
		t.Fatalf("Plan again posts plan=1: %v", fields)
	}

	// Plan again with these times, with no room for the event on the day:
	// it's left out, not run past the end, and the notice says so.
	location := redirected(t, h, tt+"/times", fields)
	if n := noticeOf(t, location); n != "Times saved. Planned the timetable again with them. Some of it doesn't fit: see below." {
		t.Errorf("planned again: %s", n)
	}
	page := do(t, h, http.MethodGet, tt, nil).Body.String()
	if strings.Contains(page, "runs past the end") {
		t.Error("nothing runs past the end of the day")
	}
	if !historyHas(t, h, admin, "Organiser", "Changed a flight's times and planned the timetable again") {
		t.Error("the history says so")
	}

	// With a Sunday too, a long warm-up is planned in like any other
	// flight: the event goes on Sunday, nothing past the end of a day.
	h, _, _, tt = flightTimesSetup(t, true)
	redirected(t, h, tt+"/setup/days", url.Values{"day-0-name": {"Saturday"}, "day-0-start": {"09:00"}, "day-0-end": {"10:30"}, "add": {"1"}})
	redirected(t, h, tt+"/setup/days", url.Values{"day-0-name": {"Saturday"}, "day-0-start": {"09:00"}, "day-0-end": {"10:30"},
		"day-1-name": {"Sunday"}, "day-1-start": {"09:00"}, "day-1-end": {"12:00"}})
	res = timesPost(t, h, tt, first, "60", "15", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("a 60-minute warm-up asks first: %d", res.StatusCode)
	}
	location = redirected(t, h, tt+"/times", planFields(t, readBody(t, res)))
	if n := noticeOf(t, location); n != "Times saved. Planned the timetable again with them." {
		t.Errorf("planned again, all of it fitting: %s", n)
	}
	page = do(t, h, http.MethodGet, tt, nil).Body.String()
	if !strings.Contains(page, "its own times") || !strings.Contains(page, "Warm-up 09:00 · competing 10:10 · finishes about 10:25") ||
		strings.Contains(page, "runs past the end") || strings.Contains(page, "Problems to look at") {
		t.Error("the flight keeps its own times, placed at that length on Sunday")
	}

	// Made anyway: what it breaks shows with the problems.
	h, _, admin, tt = flightTimesSetup(t, true)
	location = redirected(t, h, tt+"/times", func() url.Values {
		f, v := flightForm(t, do(t, h, http.MethodGet, tt, nil).Body.String(), first)
		return url.Values{"flight": {f}, "version": {v}, "warmUp": {"0"}, "competing": {"60"}, "action": {"save"}, "anyway": {"1"}}
	}())
	if n := noticeOf(t, location); n != "Times saved. 1 flight on Panel 1 starts 45 minutes later." {
		t.Errorf("made anyway: %s", n)
	}
	if page := do(t, h, http.MethodGet, tt, nil).Body.String(); !strings.Contains(page, "Problems to look at") {
		t.Error("what it broke is with the timetable's problems")
	}

	// Plan again (the ordinary button) carries own times over, and names
	// those it drops: with flights of six, there's no second flight.
	h, _, _, tt = flightTimesSetup(t, false)
	timesPost(t, h, tt, second, "4", "15", nil)
	redirected(t, h, tt+"/setup/timings", timingsWith(url.Values{"max-trampoline": {"6"}}))
	if page := do(t, h, http.MethodGet, tt, nil).Body.String(); !strings.Contains(page, "The setup has changed since this was planned.") {
		t.Error("a new largest flight marks it out of date")
	}
	rec := do(t, h, http.MethodPost, tt+"/plan", url.Values{})
	if !strings.Contains(rec.Body.String(), "It drops the own times of these flights") || !strings.Contains(rec.Body.String(), "<li>"+second+"</li>") {
		t.Errorf("Plan again names the own times it drops: %s", rec.Body.String())
	}
	if n := noticeOf(t, redirected(t, h, tt+"/plan", url.Values{"anyway": {"1"}})); !strings.Contains(n, "Own times dropped: "+second+".") {
		t.Errorf("planned: %s", n)
	}
}

func TestFlightTimesOnceBegun(t *testing.T) {
	h, clock, admin, tt := flightTimesSetup(t, true)
	clock.set(onTheDay(9, 0))
	redirected(t, h, admin+"/day/flight", url.Values{"flight": {firstFlightKey(t, h, admin+"/day?day=0")}, "what": {"start"}, "day": {"0"}})
	page := do(t, h, http.MethodGet, tt, nil).Body.String()
	box := page[strings.Index(page, "<strong>"+first+"</strong>"):strings.Index(page, "<strong>"+second+"</strong>")]
	if !strings.Contains(box, "Correct the times is on On the day.") || strings.Contains(box, "Change the times") {
		t.Error("a started flight doesn't offer Change the times")
	}
	// Posted all the same, it's refused.
	m := regexp.MustCompile(`name="flight" value="(\d+)"`).FindStringSubmatch(box)
	location := redirected(t, h, tt+"/times", url.Values{"flight": {m[1]}, "warmUp": {"0"}, "competing": {"20"}, "action": {"save"}})
	if !strings.Contains(noticeOf(t, location), "has started, so its times stay as they are; nothing was changed.") {
		t.Errorf("refused: %s", location)
	}
	if historyHas(t, h, admin, "Organiser", "Changed a flight's times") {
		t.Error("a refusal isn't recorded")
	}
	// What doesn't fit offers no planning again.
	res := timesPost(t, h, tt, second, "0", "60", nil)
	body := readBody(t, res)
	if res.StatusCode != http.StatusOK || strings.Contains(body, "Plan again with these times") || !strings.Contains(body, "The competition has begun, so the timetable isn&#39;t planned again") ||
		!strings.Contains(body, "Make the change anyway") {
		t.Errorf("once begun, make it anyway or leave it: %d %s", res.StatusCode, body)
	}
	if rec := do(t, h, http.MethodPost, tt+"/times", url.Values{"flight": {m[1]}, "plan": {"1"}, "warmUp": {"0"}, "competing": {"20"}}); !strings.Contains(rec.Header().Get("Location"), "nothing+was+changed") {
		t.Errorf("no planning again: %s", rec.Header().Get("Location"))
	}
	// Setup's timings leave the started flight as it is.
	location = redirected(t, h, tt+"/setup/timings", timingsWith(url.Values{"between-trampoline": {"15"}}))
	// The second flight still starts when the first ends, only longer.
	if n := noticeOf(t, location); n != "Timings saved. Nothing else moved." {
		t.Errorf("the started flight stays: %s", n)
	}
	if page := do(t, h, http.MethodGet, tt, nil).Body.String(); !strings.Contains(page, "Warm-up 09:00 · competing 09:10 · finishes about 09:25") ||
		!strings.Contains(page, "Warm-up 09:25 · competing 09:40 · finishes about 09:55") {
		t.Error("the started flight keeps its times")
	}
}

// After planning, Setup's timings time the flights again where they are,
// without marking the timetable out of date.
func TestSetupTimingsAfterPlanning(t *testing.T) {
	h, _, admin, tt := flightTimesSetup(t, false)
	location := redirected(t, h, tt+"/setup/timings", timingsWith(url.Values{"warm-trampoline": {"1"}, "least-trampoline": {"5"}}))
	if n := noticeOf(t, location); n != "Timings saved. 1 flight on Panel 1 starts 5 minutes later." || !strings.Contains(location, "/timetable/setup?") {
		t.Errorf("timed again: %s", location)
	}
	page := do(t, h, http.MethodGet, tt, nil).Body.String()
	if strings.Contains(page, "The setup has changed since this was planned.") || !strings.Contains(page, "Warm-up 09:30 · competing 09:45 · finishes about 10:00") {
		t.Error("timed again, not out of date")
	}
	if !historyHas(t, h, admin, "Organiser", "Changed the timetable's timings and rest") {
		t.Error("the history says so")
	}
	// Too long for the day: asked first, on Setup.
	rec := do(t, h, http.MethodPost, tt+"/setup/timings", timingsWith(url.Values{"per-trampoline": {"15"}}))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Change the timings") || !strings.Contains(rec.Body.String(), "Plan again with these times") ||
		!strings.Contains(rec.Body.String(), `href="`+tt+`/setup"`) {
		t.Errorf("asks first: %d", rec.Code)
	}
}

func TestSameTimetableWithOldOne(t *testing.T) {
	published := competitions.Schedule{Planned: true, Flights: []competitions.ScheduledFlight{{Area: "Panel 1", Start: 540, End: 565}}}
	draft := published.Copy()
	draft.Flights[0].CompetingAt = 550 // timed again since
	if !sameTimetable(draft, published) {
		t.Error("a competing time only one side keeps isn't a difference")
	}
	published.Flights[0].CompetingAt = 555
	if sameTimetable(draft, published) {
		t.Error("two competing times that differ are")
	}
}

// The timeline's CSV has when competing starts, a flight's only.
func TestTimelineCSVCompeting(t *testing.T) {
	h, _, _, tt := flightTimesSetup(t, false)
	rows, err := csv.NewReader(strings.NewReader(do(t, h, http.MethodGet, tt+"/timeline.csv", nil).Body.String())).ReadAll()
	// After the line saying it's the draft.
	if err != nil || len(rows) < 3 || rows[1][3] != "Competing from" || rows[2][3] != "09:10" {
		t.Errorf("the CSV: %v %v", rows, err)
	}
}

// planFields are the fields Plan again with these times posts, from the
// ask's page.
func planFields(t *testing.T, body string) url.Values {
	t.Helper()
	i := strings.Index(body, "Plan again with these times")
	if i < 0 {
		t.Fatal("no Plan again with these times")
	}
	f := url.Values{}
	for _, m := range regexp.MustCompile(`<input type="hidden" name="([^"]+)" value="([^"]*)"`).FindAllStringSubmatch(body[:i], -1) {
		f.Set(m[1], unescape(m[2]))
	}
	return f
}

// readBody is a response's body, as text.
func readBody(t *testing.T, res *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// addChoices is the events Setup offers to give times of their own, as the
// dropdown lists them.
func addChoices(page string) string {
	i := strings.Index(page, `name="add-trampoline"`)
	if i < 0 {
		return ""
	}
	return page[i : i+strings.Index(page[i:], "</select>")]
}
