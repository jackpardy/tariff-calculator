package main

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func eventsCompetition() url.Values {
	form := newCompetition()
	form.Set("level", "builtin-level:bucs-l3")
	form.Set("synchroLevel", "builtin-level:bucs-l3")
	form.Set("tumbling", "Novice\n\nElite ")
	form.Set("dmt", "Open")
	return form
}

func TestEvents(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, eventsCompetition())
	dash := do(t, h, http.MethodGet, admin, nil).Body.String()
	for _, want := range []string{">BUCS L3 ", ">Synchro BUCS L3 ", ">Tumbling Novice ", ">Tumbling Elite ", ">DMT Open "} {
		if !strings.Contains(dash, want) {
			t.Errorf("the dashboard has a table for %q", want)
		}
	}
	if !strings.Contains(dash, "Novice\nElite</textarea>") {
		t.Error("the settings show the tumbling levels to change")
	}

	// Individuals choose a discipline.
	enter := pathIn(t, dash, "/competitions/enter/")
	page := do(t, h, http.MethodGet, enter, nil).Body.String()
	for _, want := range []string{">Trampoline</a>", ">Synchro</a>", ">Tumbling</a>", ">DMT</a>"} {
		if !strings.Contains(page, want) {
			t.Errorf("a tab for %s", want)
		}
	}
	if page := do(t, h, http.MethodGet, enter+"?discipline=synchro", nil).Body.String(); !strings.Contains(page, `name="partnerName"`) {
		t.Error("synchro asks for a partner")
	}
	if page := do(t, h, http.MethodGet, enter+"?discipline=tumbling", nil).Body.String(); strings.Contains(page, `name="ex1Option"`) || !strings.Contains(page, "a level only") {
		t.Error("tumbling is a level only")
	}

	// Tumbling: entered, not checked.
	tumbling := withoutQuery(redirected(t, h, enter, url.Values{"gymnast": {"T"}, "discipline": {"tumbling"}, "level": {"Novice"}}))
	if page := do(t, h, http.MethodGet, tumbling, nil).Body.String(); !strings.Contains(page, "Tumbling and DMT passes aren") || !strings.Contains(page, "Tumbling Novice") {
		t.Error("the tumbling entry says it isn't checked")
	}
	if rec := do(t, h, http.MethodPost, enter, url.Values{"gymnast": {"T"}, "discipline": {"tumbling"}, "level": {"Advanced"}}); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("a tumbling level the competition doesn't offer: %d", rec.Code)
	}

	// Synchro: the partner confirms through the partner link.
	if rec := do(t, h, http.MethodPost, enter, url.Values{"gymnast": {"S"}, "discipline": {"synchro"}, "level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}}); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "partner") {
		t.Errorf("synchro needs a partner: %d", rec.Code)
	}
	pair := withoutQuery(redirected(t, h, enter, url.Values{"gymnast": {"S"}, "discipline": {"synchro"}, "level": {"BUCS L3"}, "partnerName": {"P"},
		"ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}}))
	page = do(t, h, http.MethodGet, pair, nil).Body.String()
	if !strings.Contains(page, "P hasn't confirmed the pair yet.") || !strings.Contains(page, "S &amp; P") {
		t.Error("the pair is shown, waiting for the partner")
	}
	partner := pathIn(t, page, "/competitions/partner/")
	if page := do(t, h, http.MethodGet, partner, nil).Body.String(); !strings.Contains(page, "Synchro BUCS L3 with S") {
		t.Error("the partner page")
	}
	if rec := do(t, h, http.MethodPost, partner, url.Values{"link": {"http://example.com" + pair}}); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("the pair's own entry isn't the partner's: %d", rec.Code)
	}
	redirected(t, h, partner, url.Values{"link": {"http://example.com" + tumbling}}) // P is the tumbler T
	if page := do(t, h, http.MethodGet, pair, nil).Body.String(); !strings.Contains(page, "P has confirmed the pair.") {
		t.Error("confirmed")
	}

	// The dashboard and timetable see the events.
	dash = do(t, h, http.MethodGet, admin, nil).Body.String()
	if !strings.Contains(dash, "S &amp; P") || !strings.Contains(dash, ">T</a>") {
		t.Error("the dashboard lists the pair and the tumbler")
	}
	page = do(t, h, http.MethodGet, redirected(t, h, admin+"/timetable/plan", url.Values{"action": {"plan"}, "panels": {"2"}, "start": {"09:00"}, "perGymnast": {"5"}, "between": {"10"}, "maxFlight": {"12"}}), nil).Body.String()
	if !strings.Contains(page, "<strong>Tumbling Novice</strong>") || !strings.Contains(page, "<strong>Synchro BUCS L3</strong>") {
		t.Error("the timetable plans each event")
	}

	// The organiser changes the events.
	location := redirected(t, h, admin+"/events", url.Values{"synchroLevel": {"builtin-level:bucs-l3"}, "tumbling": {"Novice"}, "dmt": {"Open\nElite"}})
	if page := do(t, h, http.MethodGet, location, nil).Body.String(); !strings.Contains(page, "Events changed.") || !strings.Contains(page, ">DMT Elite ") || strings.Contains(page, ">Tumbling Elite ") {
		t.Error("events changed")
	}
}

func TestMemberDisciplines(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, eventsCompetition())
	clubLink := pathIn(t, do(t, h, http.MethodGet, admin, nil).Body.String(), "/competitions/club/")
	club := withoutQuery(redirected(t, h, "/clubs", url.Values{"name": {"UCD"}}))
	redirected(t, h, clubLink, url.Values{"clubAdmin": {club}})
	join := pathIn(t, do(t, h, http.MethodGet, club, nil).Body.String(), "/clubs/join/")
	a := withoutQuery(redirected(t, h, join, url.Values{"name": {"A"}}))
	b := withoutQuery(redirected(t, h, join, url.Values{"name": {"B"}}))

	page := do(t, h, http.MethodGet, a, nil).Body.String()
	for _, want := range []string{"Enter Trampoline", "Enter Synchro", "Enter Tumbling", "Enter DMT"} {
		if !strings.Contains(page, want) {
			t.Errorf("the member page offers %q", want)
		}
	}
	save := formAction(t, page, a+"/competitions/")
	redirected(t, h, save, url.Values{"discipline": {""}, "level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}})
	redirected(t, h, save, url.Values{"discipline": {"tumbling"}, "level": {"Novice"}})
	redirected(t, h, save, url.Values{"discipline": {"synchro"}, "level": {"BUCS L3"}, "partnerName": {"B"}, "partnerClub": {"UCD"},
		"ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}})
	page = do(t, h, http.MethodGet, a, nil).Body.String()
	if strings.Count(page, "Saved, not sent yet") < 3 || !strings.Contains(page, "B hasn't confirmed the pair yet.") {
		t.Error("three entries saved, the pair waiting for B")
	}

	// B confirms with their own member link.
	partner := pathIn(t, page, "/competitions/partner/")
	redirected(t, h, partner, url.Values{"link": {"http://example.com" + b}})
	if page := do(t, h, http.MethodGet, a, nil).Body.String(); !strings.Contains(page, "B has confirmed the pair.") {
		t.Error("B confirmed")
	}

	// The comp sec sends all three; A withdraws tumbling only.
	redirected(t, h, formAction(t, do(t, h, http.MethodGet, club, nil).Body.String(), club+"/competitions/"), url.Values{"which": {"all"}})
	dash := do(t, h, http.MethodGet, admin, nil).Body.String()
	if strings.Count(dash, ">A</a>") != 2 || !strings.Contains(dash, "A &amp; B") {
		t.Errorf("A's trampoline and tumbling, and the pair, are on the dashboard")
	}
	withdraw := regexp.MustCompile(`action="(` + regexp.QuoteMeta(a) + `/competitions/[^"]+/withdraw)"`).FindStringSubmatch(page)
	if withdraw == nil {
		t.Fatal("a withdraw form")
	}
	redirected(t, h, withdraw[1], url.Values{"discipline": {"tumbling"}, "confirm": {"1"}})
	if page := do(t, h, http.MethodGet, a, nil).Body.String(); strings.Count(page, "Sent by your club") != 2 {
		t.Error("only tumbling is withdrawn")
	}
	if dash := do(t, h, http.MethodGet, admin, nil).Body.String(); !strings.Contains(dash, ">Withdrawn</span>") {
		t.Error("the organiser sees tumbling withdrawn until the club sends again")
	}

	// The comp sec changes A's synchro entry from the club page.
	clubPage := do(t, h, http.MethodGet, club, nil).Body.String()
	edit := regexp.MustCompile(`href="(` + regexp.QuoteMeta(club) + `/members/[^"]+discipline=synchro)"`).FindStringSubmatch(clubPage)
	if edit == nil {
		t.Fatal("a link to change A's synchro entry")
	}
	if page := do(t, h, http.MethodGet, strings.ReplaceAll(edit[1], "&amp;", "&"), nil).Body.String(); !strings.Contains(page, `name="partnerName"`) || !strings.Contains(page, `value="B"`) {
		t.Error("the comp sec edits the synchro entry, partner and all")
	}
}
