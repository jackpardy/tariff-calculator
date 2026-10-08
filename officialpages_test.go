package main

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func TestOfficialPages(t *testing.T) {
	h := competitionServer(t)
	form := newCompetition()
	form.Set("tumbling", "Novice\nElite")
	admin := created(t, h, form)
	dash := do(t, h, http.MethodGet, admin, nil).Body.String()
	officials := admin + "/officials"
	if !strings.Contains(dash, `href="`+officials+`"`) {
		t.Error("the dashboard links to the officials")
	}
	page := do(t, h, http.MethodGet, officials, nil).Body.String()
	if !strings.Contains(page, `name="panel-trampoline-execution" value="6"`) || !strings.Contains(page, `name="panel-tumbling-chair" value="1"`) || !strings.Contains(page, "Nobody yet.") {
		t.Error("panels start as the Code of Points', for each discipline")
	}

	// A club's member offers; the comp sec corrects it; sending passes it on.
	clubLink := pathIn(t, dash, "/competitions/club/")
	club := withoutQuery(redirected(t, h, "/clubs", url.Values{"name": {"UCD"}}))
	redirected(t, h, clubLink, url.Values{"clubAdmin": {club}})
	join := pathIn(t, do(t, h, http.MethodGet, club, nil).Body.String(), "/clubs/join/")
	a := withoutQuery(redirected(t, h, join, url.Values{"name": {"A"}}))
	page = do(t, h, http.MethodGet, a, nil).Body.String()
	if !strings.Contains(page, "nothing offered") || !strings.Contains(page, `name="judge-tumbling"`) {
		t.Error("the member page asks what they can judge and help with")
	}
	offer := regexp.MustCompile(`action="(` + regexp.QuoteMeta(a) + `/competitions/[^"]+/offer)"`).FindStringSubmatch(page)[1]
	redirected(t, h, offer, url.Values{"judge-trampoline": {"1"}, "upto-trampoline": {"BUCS L3"}, "chair-trampoline": {"1"}, "recorder": {"1"}})
	if page := do(t, h, http.MethodGet, a, nil).Body.String(); !strings.Contains(page, "Judge Trampoline up to BUCS L3 (can chair); recorder") {
		t.Error("the member sees what they offer")
	}
	if rec := do(t, h, http.MethodPost, offer, url.Values{"judge-tumbling": {"1"}, "upto-tumbling": {"Master"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("a level the discipline doesn't have: %d", rec.Code)
	}
	clubPage := do(t, h, http.MethodGet, club, nil).Body.String()
	edit := regexp.MustCompile(`href="(` + regexp.QuoteMeta(club) + `/members/[^"]+/offer)"`).FindStringSubmatch(clubPage)
	if edit == nil || !strings.Contains(clubPage, "Judging and helping (1)") {
		t.Fatal("the comp sec sees the offer, with a link to change it")
	}
	redirected(t, h, edit[1], url.Values{"judge-trampoline": {"1"}, "judge-tumbling": {"1"}, "marshal": {"1"}})
	if page := do(t, h, http.MethodGet, officials, nil).Body.String(); !strings.Contains(page, "Nobody yet.") {
		t.Error("the organiser sees nothing until the club sends")
	}
	redirected(t, h, formAction(t, do(t, h, http.MethodGet, club, nil).Body.String(), club+"/competitions/"), url.Values{"which": {"all"}})
	page = do(t, h, http.MethodGet, officials, nil).Body.String()
	if !strings.Contains(page, "sent by their club") || !strings.Contains(page, "Judge Trampoline and Tumbling; marshal") {
		t.Error("the club's offer reaches the organiser, as the comp sec corrected it")
	}

	// An individual offers on their entry.
	enter := pathIn(t, dash, "/competitions/enter/")
	own := withoutQuery(redirected(t, h, enter, url.Values{"gymnast": {"I"}, "discipline": {"tumbling"}, "level": {"Novice"}}))
	redirected(t, h, own+"/offer", url.Values{"judge-tumbling": {"1"}, "upto-tumbling": {"Novice"}})
	if page := do(t, h, http.MethodGet, officials, nil).Body.String(); !strings.Contains(page, "entering on their own") || !strings.Contains(page, "Judge Tumbling up to Novice") {
		t.Error("the individual's offer")
	}

	// The organiser adds a judge with no club, marks people qualified, and removes the judge.
	if location := redirected(t, h, officials+"/add", url.Values{"name": {"J"}}); !strings.Contains(location, "Nobody+was+added") {
		t.Error("an added person needs something they can do")
	}
	redirected(t, h, officials+"/add", url.Values{"name": {"Judge J"}, "judge-trampoline": {"1"}, "chair-trampoline": {"1"}})
	page = do(t, h, http.MethodGet, officials, nil).Body.String()
	if !strings.Contains(page, "added by you") {
		t.Fatal("added")
	}
	// Trampoline: A and J can judge, J can chair; tumbling: A and I.
	if !strings.Contains(page, "<td>Trampoline</td><td>2</td><td>1</td><td>9 judges, 1 chair</td>") || !strings.Contains(page, "<td>Tumbling</td><td>2</td><td>0</td>") {
		t.Error("who can judge and chair each discipline, against a panel")
	}
	ids := regexp.MustCompile(regexp.QuoteMeta(officials)+`/([^/"]+)/qualified`).FindAllStringSubmatch(page, -1)
	redirected(t, h, officials+"/"+ids[0][1]+"/qualified", url.Values{"on": {"1"}})
	redirected(t, h, officials+"/settings", url.Values{"judge": {"qualified"},
		"panel-trampoline-chair": {"1"}, "panel-trampoline-execution": {"4"}, "panel-trampoline-difficulty": {"1"}, "panel-trampoline-recorder": {"1"}, "panel-trampoline-marshal": {"0"},
		"panel-tumbling-chair": {"1"}, "panel-tumbling-execution": {"6"}, "panel-tumbling-difficulty": {"2"}, "panel-tumbling-recorder": {"1"}, "panel-tumbling-marshal": {"1"}})
	page = do(t, h, http.MethodGet, officials, nil).Body.String()
	if strings.Contains(page, `name="panel-tumbling-hd"`) || !strings.Contains(page, `name="panel-trampoline-hd" value="0"`) {
		t.Error("HD judges are for trampoline, none until the organiser adds them")
	}
	if !strings.Contains(page, `name="panel-trampoline-execution" value="4"`) || !strings.Contains(page, "6 judges, 1 chair") {
		t.Error("trampoline's panel changed to 6 judges")
	}
	if !strings.Contains(page, "<td>Tumbling</td><td>1</td>") && !strings.Contains(page, "<td>Tumbling</td><td>0</td>") {
		t.Error("with only qualified people judging, fewer can judge")
	}
	if location := redirected(t, h, officials+"/settings", url.Values{"judge": {"anyone"}}); !strings.Contains(location, "Nothing+was+changed") {
		t.Error("an unknown rule changes nothing")
	}
	remove := regexp.MustCompile(`action="(` + regexp.QuoteMeta(officials) + `/[^"]+/remove)"`).FindStringSubmatch(page)
	redirected(t, h, remove[1], url.Values{})
	if page := do(t, h, http.MethodGet, officials, nil).Body.String(); strings.Contains(page, "Judge J") {
		t.Error("removed")
	}
}

func TestSynchroHD(t *testing.T) {
	h := competitionServer(t)
	form := newCompetition()
	form.Add("synchroLevel", "builtin-level:bucs-l3")
	admin := created(t, h, form)
	officials := admin + "/officials"
	if page := do(t, h, http.MethodGet, officials, nil).Body.String(); !strings.Contains(page, `name="panel-synchro-hd" value="0"`) {
		t.Fatal("synchro can have HD judges, none to start with")
	}
	redirected(t, h, officials+"/settings", url.Values{
		"panel-trampoline-chair": {"1"}, "panel-trampoline-execution": {"6"}, "panel-trampoline-difficulty": {"2"}, "panel-trampoline-recorder": {"1"}, "panel-trampoline-marshal": {"1"},
		"panel-synchro-chair": {"1"}, "panel-synchro-execution": {"6"}, "panel-synchro-difficulty": {"2"}, "panel-synchro-hd": {"2"}, "panel-synchro-sync": {"2"}, "panel-synchro-recorder": {"1"}, "panel-synchro-marshal": {"1"}})
	if page := do(t, h, http.MethodGet, officials, nil).Body.String(); !strings.Contains(page, `name="panel-synchro-hd" value="2"`) {
		t.Error("synchro's HD judges saved")
	}
}
