package main

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestPersonalAndClubTimetables(t *testing.T) {
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

	// Not published: nothing to show, and no links.
	if strings.Contains(do(t, h, http.MethodGet, ann, nil).Body.String(), "Your timetable") {
		t.Error("no link before the timetable is published")
	}
	tt := admin + "/timetable"
	redirected(t, h, tt+"/plan", url.Values{})
	redirected(t, h, tt+"/publish", url.Values{"on": {"1"}})

	// Ann's page links to hers and the club's; hers picks out her flight.
	page := do(t, h, http.MethodGet, ann, nil).Body.String()
	if !strings.Contains(page, ">Your timetable</a>") || !strings.Contains(page, ">Club timetable</a>") {
		t.Fatal("Ann's page links to her timetable and the club's")
	}
	compID := strings.Split(strings.Split(page, ann+"/competitions/")[1], "/")[0]
	mine := do(t, h, http.MethodGet, ann+"/competitions/"+compID+"/timeline", nil).Body.String()
	if !strings.Contains(mine, "Your timetable") || !strings.Contains(mine, "comp-tl-mine") || !strings.Contains(mine, "You compete") {
		t.Error("Ann's flight picked out")
	}
	if only := do(t, h, http.MethodGet, ann+"/competitions/"+compID+"/timeline?only=1", nil).Body.String(); strings.Count(only, "comp-tl-mine") != 1 || strings.Contains(only, `comp-tl-flight comp-tl-faint`) {
		t.Error("only Ann's")
	}

	// The club's: Ann named, Ivy's flight (if apart) not picked out.
	clubPage := do(t, h, http.MethodGet, club, nil).Body.String()
	if !strings.Contains(clubPage, ">Club timetable</a>") {
		t.Error("the club page links to its timetable")
	}
	ours := do(t, h, http.MethodGet, club+"/competitions/"+compID+"/timeline", nil).Body.String()
	if !strings.Contains(ours, "UCD&#39;s timetable") && !strings.Contains(ours, "UCD's timetable") || !strings.Contains(ours, "Ann") {
		t.Error("UCD's timetable names Ann")
	}
	if strings.Contains(ours, `comp-tl-note">Ivy`) {
		t.Error("Ivy isn't UCD's")
	}

	// Ivy's own.
	if page := do(t, h, http.MethodGet, ivy, nil).Body.String(); !strings.Contains(page, ">Your timetable</a>") {
		t.Error("Ivy's page links to hers")
	}
	if mine := do(t, h, http.MethodGet, ivy+"/timeline", nil).Body.String(); !strings.Contains(mine, "You compete") {
		t.Error("Ivy's flight picked out")
	}
	// Another club can't see UCD's.
	other := withoutQuery(redirected(t, h, "/clubs", url.Values{"name": {"DCU"}}))
	if rec := do(t, h, http.MethodGet, other+"/competitions/"+compID+"/timeline", nil); rec.Code == http.StatusOK {
		t.Error("DCU isn't entered")
	}
}
