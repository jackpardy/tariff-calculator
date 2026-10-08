package main

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func TestDraftTimetable(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	enter := pathIn(t, do(t, h, http.MethodGet, admin, nil).Body.String(), "/competitions/enter/")
	ann := withoutQuery(redirected(t, h, enter, url.Values{"gymnast": {"Ann"}, "level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}}))
	tt := admin + "/timetable"
	redirected(t, h, tt+"/setup/areas", url.Values{"area-0-name": {"Panel 1"}, "area-0-discipline": {"trampoline"}, "add": {"1"}})
	redirected(t, h, tt+"/setup/areas", url.Values{"area-0-name": {"Panel 1"}, "area-0-discipline": {"trampoline"}, "area-1-name": {"Panel 2"}, "area-1-discipline": {"trampoline"}})
	page := do(t, h, http.MethodGet, redirected(t, h, tt+"/plan", url.Values{}), nil).Body.String()
	where := func() string {
		t.Helper()
		m := regexp.MustCompile(`· (Panel \d) · warm-up`).FindStringSubmatch(do(t, h, http.MethodGet, ann, nil).Body.String())
		if m == nil {
			return ""
		}
		return m[1]
	}
	if where() != "" {
		t.Error("nothing to see before publishing")
	}
	redirected(t, h, tt+"/publish", url.Values{"action": {"publish"}})
	first := where()
	if first == "" {
		t.Fatal("Ann sees her flight")
	}
	other := map[string]string{"Panel 1": "Panel 2", "Panel 2": "Panel 1"}[first]

	// Moving her flight changes the draft: Ann still sees the published one.
	flight := regexp.MustCompile(`name="flight" value="(\d+)"`).FindStringSubmatch(page)[1]
	redirected(t, h, tt+"/flight", url.Values{"flight": {flight}, "action": {"move"}, "to": {"0:" + other}})
	if page := do(t, h, http.MethodGet, tt, nil).Body.String(); !strings.Contains(page, "Changes not published yet") || !strings.Contains(page, "Publish changes") {
		t.Error("the organiser sees changes not published")
	}
	if where() != first {
		t.Error("Ann still sees the published timetable")
	}
	// Discarded: back as published.
	redirected(t, h, tt+"/publish", url.Values{"action": {"discard"}})
	if page := do(t, h, http.MethodGet, tt, nil).Body.String(); strings.Contains(page, "Changes not published yet") {
		t.Error("discarded")
	}
	// Moved again and published: Ann sees the move.
	redirected(t, h, tt+"/flight", url.Values{"flight": {flight}, "action": {"move"}, "to": {"0:" + other}})
	redirected(t, h, tt+"/publish", url.Values{"action": {"publish"}})
	if where() != other {
		t.Error("published: Ann sees the move")
	}

	// A delay kept goes into the draft, not to Ann.
	delay := tt + "/delay?" + url.Values{"day": {"0"}, "from": {"09:05"}, "minutes": {"60"}}.Encode()
	if page := do(t, h, http.MethodGet, delay, nil).Body.String(); !strings.Contains(page, "Keep this in the draft timetable") {
		t.Fatal("a delay can be kept")
	}
	if location := redirected(t, h, delay, url.Values{}); !strings.Contains(location, "now+the+draft") {
		t.Errorf("kept: %s", location)
	}
	if page := do(t, h, http.MethodGet, tt, nil).Body.String(); !strings.Contains(page, "Changes not published yet") {
		t.Error("the delay is a change to publish")
	}

	// Unpublished: Ann sees nothing.
	redirected(t, h, tt+"/publish", url.Values{"action": {"unpublish"}})
	if where() != "" {
		t.Error("unpublished")
	}
}
