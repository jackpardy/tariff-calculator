package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// The timetable carries the flights a gymnast can move to once, not once per
// gymnast, and a move with no flight chosen changes nothing.
func TestMoveOptionsOnce(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	for _, name := range []string{"Ann", "Bea", "Cal", "Dan"} {
		redirected(t, h, enter, url.Values{"gymnast": {name}, "level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}})
	}
	tt := admin + "/timetable"
	redirected(t, h, tt+"/plan", url.Values{})
	page := do(t, h, http.MethodGet, tt, nil).Body.String()
	template := regexp.MustCompile(`(?s)<template id="move-options">(.*?)</template>`).FindStringSubmatch(page)
	if template == nil || !strings.Contains(template[1], "<option value=") || !strings.Contains(page, "js/fill.js") {
		t.Fatalf("the flights to move to, once, with the script that fills the dropdowns: %s", page)
	}
	selects := regexp.MustCompile(`(?s)<select name="to"[^>]*data-fill="move-options"[^>]*>(.*?)</select>`).FindAllStringSubmatch(page, -1)
	if len(selects) < 1 || strings.Count(page, "Move a gymnast") != len(selects) {
		t.Fatalf("a Move a gymnast form per flight, not a dropdown per gymnast: %d", len(selects))
	}
	for _, s := range selects {
		if strings.Count(s[1], "<option") != 2 || !strings.Contains(s[1], `<option value="" disabled selected>Choose a flight</option>`) {
			t.Errorf("a dropdown holds only its placeholder and Take out until it's opened: %s", s[1])
		}
	}

	// Each option says its flight's discipline, and each dropdown its own,
	// to offer only flights a gymnast can move to (fill.js).
	if !strings.Contains(template[1], `data-group="`) || !strings.Contains(page, `data-fill="move-options" data-group="`) || !strings.Contains(page, `data-except="`) {
		t.Error("moves are only to flights of the same discipline, not the one they're in")
	}

	// Left on "Choose a flight" (nothing sent): nothing changes.
	entry := regexp.MustCompile(`<option value="([^"]+)">\d+\. `).FindStringSubmatch(page)[1]
	// Left on "Choose a gymnast": nothing changes either.
	if to := redirected(t, h, tt+"/entry", url.Values{"to": {""}}); !strings.Contains(to, "Choose+a+gymnast") {
		t.Errorf("a move with no gymnast chosen says so: %s", to)
	}
	if to := redirected(t, h, tt+"/entry", url.Values{"entry": {entry}}); !strings.Contains(to, "Choose+a+flight") {
		t.Errorf("a move with no flight chosen says so: %s", to)
	}
	if after := do(t, h, http.MethodGet, tt, nil).Body.String(); strings.Contains(after, "Not in the timetable") && !strings.Contains(page, "Not in the timetable") {
		t.Error("nobody was taken out")
	}
	// The seat dropdowns fill from one list of people too, and a seat left
	// on "Choose a person" (nothing sent) isn't emptied.
	if !strings.Contains(page, `<template id="seat-options">`) || !regexp.MustCompile(`data-fill="seat-options"><option value="" disabled selected>Choose a person</option>\s*<option value="">No one</option></select>`).MatchString(page) {
		t.Error("seat dropdowns hold only their placeholder and No one until opened")
	}
	if to := redirected(t, h, tt+"/officials", url.Values{"action": {"seat"}, "flight": {"0"}, "seat": {"0"}}); !strings.Contains(to, "Choose+who+takes+the+seat") {
		t.Errorf("a seat with no one chosen says so: %s", to)
	}
	// Choosing "Take out of the timetable" (sent empty) still takes them out.
	redirected(t, h, tt+"/entry", url.Values{"entry": {entry}, "to": {""}})
	if after := do(t, h, http.MethodGet, tt, nil).Body.String(); after == page {
		t.Error("taking someone out changes the timetable")
	}
}

// A gymnast moves only to a flight of their own discipline.
func TestMoveKeepsDiscipline(t *testing.T) {
	h := competitionServer(t)
	form := newCompetition()
	form.Set("tumbling", "Novice")
	admin := created(t, h, form)
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	redirected(t, h, enter, bucsEntry("Eve"))
	redirected(t, h, enter, url.Values{"gymnast": {"Dara"}, "discipline": {"tumbling"}, "level": {"Novice"}})
	tt := admin + "/timetable"
	redirected(t, h, tt+"/plan", url.Values{})
	page := do(t, h, http.MethodGet, tt, nil).Body.String()
	trampoline := regexp.MustCompile(`<option value="(\d+)" data-group="">`).FindStringSubmatch(page)
	if trampoline == nil {
		t.Fatalf("each flight to move to says its discipline: %s", page)
	}
	id := regexp.MustCompile(`<option value="([^"]+)">1\. Dara`).FindStringSubmatch(page)[1]
	if to := redirected(t, h, tt+"/entry", url.Values{"entry": {id}, "to": {trampoline[1]}}); !strings.Contains(to, "own+discipline") {
		t.Errorf("a tumbler can't move to a trampoline flight: %s", to)
	}
	if after := do(t, h, http.MethodGet, tt, nil).Body.String(); after != page {
		t.Error("nothing was changed")
	}
}
