package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestPersonalAndClubTimetables(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	dash := do(t, h, http.MethodGet, admin+"/settings", nil).Body.String()
	clubLink, enter := pathIn(t, dash, "/competitions/club/"), pathIn(t, dash, "/competitions/enter/")
	club := withoutQuery(redirected(t, h, "/clubs", url.Values{"name": {"UCD"}}))
	redirected(t, h, clubLink, url.Values{"clubAdmin": {club}})
	join := pathIn(t, do(t, h, http.MethodGet, club+"/members", nil).Body.String(), "/clubs/join/")
	entry := bucsEntry("")
	ann := withoutQuery(redirected(t, h, join, url.Values{"name": {"Ann"}}))
	redirected(t, h, formAction(t, memberCompetitionPage(t, h, ann), ann+"/competitions/"), entry)
	redirected(t, h, formAction(t, clubEntriesPage(t, h, club), club+"/competitions/"), url.Values{"which": {"all"}})
	ind := url.Values{"gymnast": {"Ivy"}}
	for k, v := range entry {
		ind[k] = v
	}
	ivy := withoutQuery(redirected(t, h, enter, ind))

	// Not published: nothing to show, and no links.
	if strings.Contains(memberCompetitionPage(t, h, ann), "Your timetable") || strings.Contains(do(t, h, http.MethodGet, ann, nil).Body.String(), "Timetable published") {
		t.Error("no link before the timetable is published")
	}
	tt := admin + "/timetable"
	redirected(t, h, tt+"/plan", url.Values{})
	redirected(t, h, tt+"/publish", url.Values{"on": {"1"}})

	// Ann's page links to hers and the club's; hers picks out her flight.
	if !strings.Contains(do(t, h, http.MethodGet, ann, nil).Body.String(), "Timetable published") {
		t.Error("Ann's list of competitions says the timetable is published")
	}
	page := do(t, h, http.MethodGet, myDayPath(t, h, ann), nil).Body.String()
	if !strings.Contains(page, ">Your timetable</a>") || !strings.Contains(page, ">Club timetable</a>") {
		t.Fatal("Ann's My competition links to her timetable and the club's")
	}
	if page := memberCompetitionPage(t, h, ann); strings.Contains(page, ">Your timetable</a>") || strings.Contains(page, ">Add to calendar</a>") {
		t.Error("her page for the competition has one link, to My competition, not the timetables too")
	}
	compID := strings.Split(strings.TrimPrefix(myDayPath(t, h, ann), ann+"/competitions/"), "/")[0]
	mine := do(t, h, http.MethodGet, ann+"/competitions/"+compID+"/timeline", nil).Body.String()
	if !strings.Contains(mine, "Your timetable") || !strings.Contains(mine, "comp-tl-mine") || !strings.Contains(mine, "You compete") {
		t.Error("Ann's flight picked out")
	}
	if only := do(t, h, http.MethodGet, ann+"/competitions/"+compID+"/timeline?only=1", nil).Body.String(); strings.Count(only, "comp-tl-mine") != 1 || strings.Contains(only, `comp-tl-flight comp-tl-faint`) {
		t.Error("only Ann's")
	}

	// The club's: Ann named, Ivy's flight (if apart) not picked out.
	clubPage := clubEntriesPage(t, h, club)
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
	if page := dayAll(t, h, ivy+"/day"); !strings.Contains(page, ">Your timetable</a>") {
		t.Error("Ivy's My competition links to hers")
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
