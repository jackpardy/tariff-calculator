package main

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func TestRemovingEntriesPages(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	dash := do(t, h, http.MethodGet, admin, nil).Body.String()
	clubLink, enter := pathIn(t, dash, "/competitions/club/"), pathIn(t, dash, "/competitions/enter/")
	club := withoutQuery(redirected(t, h, "/clubs", url.Values{"name": {"UCD"}}))
	redirected(t, h, clubLink, url.Values{"clubAdmin": {club}})
	join := pathIn(t, do(t, h, http.MethodGet, club, nil).Body.String(), "/clubs/join/")
	entry := url.Values{"level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}}
	members := map[string]string{}
	for _, name := range []string{"X", "Y"} {
		m := withoutQuery(redirected(t, h, join, url.Values{"name": {name}}))
		redirected(t, h, formAction(t, do(t, h, http.MethodGet, m, nil).Body.String(), m+"/competitions/"), entry)
		members[name] = m
	}
	send := regexp.MustCompile(`action="(` + regexp.QuoteMeta(club) + `/competitions/[^"]+/send)"`).FindStringSubmatch(do(t, h, http.MethodGet, club, nil).Body.String())[1]
	redirected(t, h, send, url.Values{"which": {"all"}})
	ind := url.Values{"gymnast": {"I"}}
	for k, v := range entry {
		ind[k] = v
	}
	own := withoutQuery(redirected(t, h, enter, ind))

	ids := map[string]string{}
	for _, m := range regexp.MustCompile(`name="entry" value="([^"]+)" form="remove-form" aria-label="Tick ([^"]+)"`).FindAllStringSubmatch(do(t, h, http.MethodGet, admin, nil).Body.String(), -1) {
		ids[m[2]] = m[1]
	}
	if len(ids) != 3 {
		t.Fatalf("each entry can be ticked: %v", ids)
	}

	// X removed, the reason for the club only.
	if location := redirected(t, h, admin+"/remove", url.Values{"entry": {ids["X"]}, "action": {"remove"}, "note": {"Not paid"}, "to": {"club"}}); !strings.Contains(location, "Removed+1+entry") {
		t.Errorf("removed: %s", location)
	}
	dash = do(t, h, http.MethodGet, admin, nil).Body.String()
	if strings.Contains(dash, `aria-label="Tick X"`) || !strings.Contains(dash, "Removed and on hold") || !strings.Contains(dash, "Not paid (the club sees it)") {
		t.Error("X is out of the tables, listed to restore")
	}
	if csv := do(t, h, http.MethodGet, admin+"/entries.csv", nil).Body.String(); strings.Contains(csv, "\nX,") {
		t.Error("nor in the CSV")
	}
	if page := do(t, h, http.MethodGet, club, nil).Body.String(); !strings.Contains(page, "Removed by the organiser") || !strings.Contains(page, "Not paid") {
		t.Error("the club sees X removed, and why")
	}
	if page := do(t, h, http.MethodGet, members["X"], nil).Body.String(); !strings.Contains(page, "Removed by the organiser") || strings.Contains(page, "Not paid") {
		t.Error("X sees it's removed, not the club's reason")
	}

	// Y held for changes: Y changes it, the club sends it, the organiser accepts it.
	redirected(t, h, admin+"/remove", url.Values{"entry": {ids["Y"]}, "action": {"hold"}, "note": {"Fix the voluntary"}, "to": {"both"}})
	if page := do(t, h, http.MethodGet, members["Y"], nil).Body.String(); !strings.Contains(page, "On hold: the organiser asks for changes") || !strings.Contains(page, "Fix the voluntary") {
		t.Error("Y sees the hold and why")
	}
	changed := url.Values{"level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-2"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}}
	redirected(t, h, formAction(t, do(t, h, http.MethodGet, members["Y"], nil).Body.String(), members["Y"]+"/competitions/"), changed)
	if page := do(t, h, http.MethodGet, club, nil).Body.String(); !strings.Contains(page, "On hold: changed, send it again") {
		t.Error("the club sees Y changed, to send")
	}
	redirected(t, h, send, url.Values{"which": {"changed"}})
	dash = do(t, h, http.MethodGet, admin, nil).Body.String()
	if !strings.Contains(dash, "Changed and sent again: waiting for you") || !strings.Contains(dash, "Accept back in") {
		t.Fatal("the organiser sees Y changed, to accept")
	}
	redirected(t, h, admin+"/entries/"+ids["Y"]+"/restore", url.Values{})
	if dash := do(t, h, http.MethodGet, admin, nil).Body.String(); !strings.Contains(dash, `aria-label="Tick Y"`) {
		t.Error("Y back in")
	}

	// X stays removed when the club sends everyone again, until restored.
	redirected(t, h, send, url.Values{"which": {"all"}})
	if dash := do(t, h, http.MethodGet, admin, nil).Body.String(); strings.Contains(dash, `aria-label="Tick X"`) {
		t.Error("a removed entry isn't sent again")
	}

	// I removed: their page says so, and can't be changed.
	redirected(t, h, admin+"/remove", url.Values{"entry": {ids["I"]}, "action": {"remove"}})
	page := do(t, h, http.MethodGet, own, nil).Body.String()
	if !strings.Contains(page, "The organiser removed this entry") || strings.Contains(page, "Save changes") {
		t.Error("I sees it's removed, with nothing to change")
	}
	if rec := do(t, h, http.MethodPost, own, ind); rec.Code != http.StatusConflict {
		t.Errorf("changing it is refused: %d", rec.Code)
	}
}
