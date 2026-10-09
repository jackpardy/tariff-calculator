package main

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
)

// icsEvents are a calendar's VEVENT blocks, as lines.
func icsEvents(body string) [][]string {
	var out [][]string
	var cur []string
	for _, l := range strings.Split(body, "\r\n") {
		switch {
		case l == "BEGIN:VEVENT":
			cur = []string{}
		case l == "END:VEVENT":
			out = append(out, cur)
			cur = nil
		case cur != nil:
			cur = append(cur, l)
		}
	}
	return out
}

// icsField is an event's property, "" if it has none.
func icsField(event []string, name string) string {
	for _, l := range event {
		if v, ok := strings.CutPrefix(l, name+":"); ok {
			return v
		}
	}
	return ""
}

func TestCalendars(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	dash := do(t, h, http.MethodGet, admin, nil).Body.String()
	clubLink, enter := pathIn(t, dash, "/competitions/club/"), pathIn(t, dash, "/competitions/enter/")
	club := withoutQuery(redirected(t, h, "/clubs", url.Values{"name": {"UCD"}}))
	redirected(t, h, clubLink, url.Values{"clubAdmin": {club}})
	join := pathIn(t, do(t, h, http.MethodGet, club, nil).Body.String(), "/clubs/join/")
	entry := url.Values{"level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}}
	ann := withoutQuery(redirected(t, h, join, url.Values{"name": {"Ann"}}))
	redirected(t, h, formAction(t, do(t, h, http.MethodGet, ann, nil).Body.String(), ann+"/competitions/"), entry)
	redirected(t, h, formAction(t, do(t, h, http.MethodGet, club, nil).Body.String(), club+"/competitions/"), url.Values{"which": {"all"}})
	ind := url.Values{"gymnast": {"Ivy"}}
	for k, v := range entry {
		ind[k] = v
	}
	ivy := withoutQuery(redirected(t, h, enter, ind))
	compID := strings.Split(strings.Split(do(t, h, http.MethodGet, ann, nil).Body.String(), ann+"/competitions/")[1], "/")[0]

	// Not published: no link, but a valid empty calendar, to subscribe to.
	if strings.Contains(do(t, h, http.MethodGet, ivy, nil).Body.String(), "Add to calendar") {
		t.Error("no calendar link before the timetable is published")
	}
	rec := do(t, h, http.MethodGet, ivy+"/calendar.ics", nil)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.HasPrefix(body, "BEGIN:VCALENDAR\r\n") || !strings.HasSuffix(body, "END:VCALENDAR\r\n") || strings.Contains(body, "BEGIN:VEVENT") {
		t.Errorf("an empty calendar before publishing: %d %q", rec.Code, body)
	}

	tt := admin + "/timetable"
	redirected(t, h, tt+"/plan", url.Values{})
	redirected(t, h, tt+"/publish", url.Values{"action": {"publish"}})

	// Ivy's: her flight, in UTC, with a UID.
	rec = do(t, h, http.MethodGet, ivy+"/calendar.ics", nil)
	body = rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("Ivy's calendar: %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/calendar; charset=utf-8" {
		t.Errorf("content type %q", got)
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(got, `inline; filename="`) || !strings.HasSuffix(got, `.ics"`) {
		t.Errorf("content disposition %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("cache control %q", got)
	}
	if strings.Count(body, "\r\n") != strings.Count(body, "\n") {
		t.Error("lines end in CRLF")
	}
	for _, want := range []string{"VERSION:2.0\r\n", "PRODID:-//tariff.pardy.ie//competitions//EN\r\n", "METHOD:PUBLISH\r\n", "X-WR-CALNAME:"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	events := icsEvents(body)
	if len(events) != 1 {
		t.Fatalf("Ivy has one flight, got %d:\n%s", len(events), body)
	}
	ev := events[0]
	if !regexp.MustCompile(`^20300316T\d{6}Z$`).MatchString(icsField(ev, "DTSTART")) {
		t.Errorf("DTSTART %q", icsField(ev, "DTSTART"))
	}
	if !strings.HasSuffix(icsField(ev, "DTEND"), "Z") || !strings.HasSuffix(icsField(ev, "DTSTAMP"), "Z") {
		t.Error("DTEND and DTSTAMP in UTC")
	}
	if uid := icsField(ev, "UID"); !strings.HasSuffix(uid, "@tariff.pardy.ie") || len(uid) < 20 {
		t.Errorf("UID %q", uid)
	}
	if s := icsField(ev, "SUMMARY"); !strings.HasPrefix(s, "Ivy · BUCS L3") {
		t.Errorf("SUMMARY %q", s)
	}
	if icsField(ev, "LOCATION") == "" {
		t.Error("the area as the location")
	}
	if !strings.Contains(icsField(ev, "DESCRIPTION"), "Warm-up ") {
		t.Error("warm-up in the description")
	}

	// A republish keeps the UIDs.
	first := icsField(ev, "UID")
	redirected(t, h, tt+"/plan", url.Values{})
	redirected(t, h, tt+"/publish", url.Values{"action": {"publish"}})
	if again := icsEvents(do(t, h, http.MethodGet, ivy+"/calendar.ics", nil).Body.String()); len(again) != 1 || icsField(again[0], "UID") != first {
		t.Error("the UID stays the same when the timetable is published again")
	}

	// Ann's links, and her own and the club's calendars.
	page := do(t, h, http.MethodGet, ann, nil).Body.String()
	if !strings.Contains(page, ">Add to calendar</a>") || !strings.Contains(page, ">Club calendar</a>") {
		t.Fatal("Ann's page links to her calendar and the club's")
	}
	if !strings.Contains(do(t, h, http.MethodGet, club, nil).Body.String(), ">Club calendar</a>") {
		t.Error("the club page links to its calendar")
	}
	if !strings.Contains(do(t, h, http.MethodGet, ivy, nil).Body.String(), ">Add to calendar</a>") {
		t.Error("Ivy's page links to her calendar")
	}
	mine := icsEvents(do(t, h, http.MethodGet, ann+"/competitions/"+compID+"/calendar.ics", nil).Body.String())
	if len(mine) != 1 || !strings.HasPrefix(icsField(mine[0], "SUMMARY"), "Ann · BUCS L3") {
		t.Errorf("Ann's calendar has her flight: %v", mine)
	}
	for name, path := range map[string]string{
		"member": ann + "/competitions/" + compID + "/club-calendar.ics",
		"admin":  club + "/competitions/" + compID + "/calendar.ics",
	} {
		rec := do(t, h, http.MethodGet, path, nil)
		events := icsEvents(rec.Body.String())
		if rec.Code != http.StatusOK || len(events) != 1 || !strings.HasPrefix(icsField(events[0], "SUMMARY"), "Ann · BUCS L3") {
			t.Errorf("the club's calendar (%s) has Ann's flight, not Ivy's: %d %v", name, rec.Code, events)
		}
		if !strings.Contains(rec.Body.String(), "X-WR-CALNAME:") || !strings.Contains(rec.Body.String(), "UCD") {
			t.Errorf("the club's calendar (%s) is called after the club", name)
		}
	}

	// Another club can't get UCD's.
	other := withoutQuery(redirected(t, h, "/clubs", url.Values{"name": {"DCU"}}))
	if rec := do(t, h, http.MethodGet, other+"/competitions/"+compID+"/calendar.ics", nil); rec.Code == http.StatusOK {
		t.Error("DCU isn't entered")
	}
}

func TestCalendarText(t *testing.T) {
	if got := icsText("a,b;c\\d\r\ne\nf\x01"); got != `a\,b\;c\\d\ne\nf` {
		t.Errorf("escaped: %q", got)
	}
	long := "SUMMARY:" + strings.Repeat("é", 100) + strings.Repeat("x", 100)
	lines := strings.Split(icsFold(long), "\r\n")
	if len(lines) < 4 {
		t.Fatalf("folded into %d lines", len(lines))
	}
	var unfolded string
	for i, l := range lines {
		if len(l) > 75 {
			t.Errorf("line %d is %d octets", i, len(l))
		}
		if !utf8.ValidString(l) {
			t.Errorf("line %d splits a character", i)
		}
		if i > 0 && !strings.HasPrefix(l, " ") {
			t.Errorf("line %d isn't a continuation", i)
		}
		unfolded += strings.TrimPrefix(l, " ")
	}
	if unfolded != long {
		t.Error("unfolding gives the line back")
	}
	if short := "SUMMARY:short"; icsFold(short) != short {
		t.Error("short lines stay")
	}
	if got := calendarFileName("BUCS Champs 2030 · Ann Ryan"); got != "bucs-champs-2030-ann-ryan" {
		t.Errorf("file name %q", got)
	}
}

func TestCalendarEvents(t *testing.T) {
	c := store.Competition{ID: "c1", Competition: competitions.Competition{Name: "Summer, Open", Date: "2030-07-13"}}
	s := competitions.Schedule{
		Setup: competitions.Setup{Days: []competitions.Day{{Name: "Saturday"}, {Name: "Sunday"}}},
		Flights: []competitions.ScheduledFlight{
			{Flight: competitions.Flight{Level: "BUCS L4", Number: 2, Of: 3, Entries: []string{"e1"}}, Day: 1, Area: "Panel 2", Start: 9*60 + 30, End: 10*60 + 40,
				Officials: []competitions.Duty{{Role: competitions.RoleExecution, Person: "m:ann"}, {Role: competitions.RoleChair}}},
		},
	}
	mine := func(sl calendarSlot) []calendarClaim {
		var out []calendarClaim
		for _, d := range sl.officials {
			if d.Person == "m:ann" {
				out = append(out, calendarClaim{kind: "duty", role: d.Role})
			}
		}
		return out
	}
	events := calendarEventsOf(c, s, "m:ann", mine)
	if len(events) != 1 {
		t.Fatalf("one seat, got %d", len(events))
	}
	e := events[0]
	if e.summary != "Execution judge · BUCS L4 · flight 2 of 3" || e.location != "Panel 2" {
		t.Errorf("summary %q, location %q", e.summary, e.location)
	}
	// Sunday 14 July, 09:30 in Dublin (summer time) is 08:30 UTC.
	if got := icsTime(e.start); got != "20300714T083000Z" {
		t.Errorf("start %s", got)
	}
	if got := icsTime(e.end); got != "20300714T094000Z" {
		t.Errorf("end %s", got)
	}
	if !strings.Contains(e.description, "Warm-up 09:30, finishes about 10:40") || !strings.Contains(e.description, "Sunday") {
		t.Errorf("description %q", e.description)
	}
	if again := calendarEventsOf(c, s, "m:ann", mine); again[0].uid != e.uid {
		t.Error("the UID is stable")
	}
	if other := calendarEventsOf(c, s, "m:bea", mine); other[0].uid == e.uid {
		t.Error("the UID is for whose it is")
	}
	if !strings.Contains(string(icsFile("Summer, Open", events, time.Now())), "X-WR-CALNAME:Summer\\, Open\r\n") {
		t.Error("the name is escaped")
	}
}
