package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"tariffCalculator/competitions"
)

func TestOfficialsRota(t *testing.T) {
	h := competitionServer(t)
	form := newCompetition()
	form.Set("tumbling", "Novice")
	admin := created(t, h, form)
	officials := admin + "/officials"
	redirected(t, h, officials+"/settings", url.Values{
		"panel-trampoline-chair": {"1"}, "panel-trampoline-execution": {"2"}, "panel-trampoline-difficulty": {"0"}, "panel-trampoline-recorder": {"0"}, "panel-trampoline-marshal": {"0"},
		"panel-tumbling-chair": {"1"}, "panel-tumbling-execution": {"1"}, "panel-tumbling-difficulty": {"0"}, "panel-tumbling-recorder": {"0"}, "panel-tumbling-marshal": {"0"}})
	page := do(t, h, http.MethodGet, officials, nil).Body.String()
	if !strings.Contains(page, `name="recordersChange"`) || strings.Contains(page, `name="recordersChange" value="1" checked`) || strings.Contains(page, `name="marshalsChange" value="1" checked`) {
		t.Error("recorders and marshals stay by default")
	}
	// Marshals can change between flights; recorders still stay.
	settings := url.Values{
		"panel-trampoline-chair": {"1"}, "panel-trampoline-execution": {"2"}, "panel-trampoline-difficulty": {"0"}, "panel-trampoline-recorder": {"0"}, "panel-trampoline-marshal": {"0"},
		"panel-tumbling-chair": {"1"}, "panel-tumbling-execution": {"1"}, "panel-tumbling-difficulty": {"0"}, "panel-tumbling-recorder": {"0"}, "panel-tumbling-marshal": {"0"}, "marshalsChange": {"1"}}
	redirected(t, h, officials+"/settings", settings)
	if page := do(t, h, http.MethodGet, officials, nil).Body.String(); strings.Contains(page, `name="recordersChange" value="1" checked`) || !strings.Contains(page, `name="marshalsChange" value="1" checked`) {
		t.Error("each chosen on its own")
	}
	redirected(t, h, officials+"/add", url.Values{"name": {"Mary"}, "judge-trampoline": {"1"}, "chair-trampoline": {"1"}})
	redirected(t, h, officials+"/add", url.Values{"name": {"Tom"}, "judge-trampoline": {"1"}})
	redirected(t, h, officials+"/add", url.Values{"name": {"Ann"}, "judge-tumbling": {"1"}, "chair-tumbling": {"1"}})
	enter := pathIn(t, do(t, h, http.MethodGet, admin, nil).Body.String(), "/competitions/enter/")
	redirected(t, h, enter, url.Values{"gymnast": {"Eve"}, "level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}})
	// Dara competes in tumbling and judges trampoline: tumbling goes first.
	dara := withoutQuery(redirected(t, h, enter, url.Values{"gymnast": {"Dara"}, "discipline": {"tumbling"}, "level": {"Novice"}}))
	redirected(t, h, dara+"/offer", url.Values{"judge-trampoline": {"1"}})
	tt := admin + "/timetable"
	redirected(t, h, tt+"/setup/rules", url.Values{"add": {"1"}, "kind": {"before"}, "must": {"1"}, "event": {"Tumbling Novice"}, "event2": {"BUCS L3"}})

	page = do(t, h, http.MethodGet, redirected(t, h, tt+"/plan", url.Values{}), nil).Body.String()
	if !strings.Contains(page, "4 can officiate") || !strings.Contains(page, "· 3 of 3 seats filled") || !strings.Contains(page, "· 1 of 2 seats filled") {
		t.Fatalf("the panels are filled from the officials: %s", page)
	}
	if !strings.Contains(page, "Tumbling Novice flight 1 on Track 1: no one for 1 execution judge") && !strings.Contains(page, "Tumbling Novice on Track 1: no one for 1 execution judge") {
		t.Error("a seat no one can take is reported")
	}
	if !strings.Contains(page, "<strong>Seats no one could take:</strong> 1, on 1 panel</summary>") {
		t.Error("the empty seats are counted, with the panels they're on")
	}
	l3 := page[strings.Index(page, "<strong>BUCS L3"):]
	l3 = strings.Join(strings.Fields(l3[:strings.Index(l3, "Redraw order")]), " ")
	for _, who := range []string{"Chair of judges: Mary", "Execution judge: Dara", "Execution judge: Tom"} {
		if !strings.Contains(l3, who) {
			t.Errorf("BUCS L3's panel has %s: %s", who, l3)
		}
	}

	// Tom mustn't officiate; assigning again leaves his seat empty.
	tom := regexp.MustCompile(`<option value="([^"]+)">Tom</option>`).FindStringSubmatch(page)
	if tom == nil {
		t.Fatal("Tom is in the rules form")
	}
	redirected(t, h, tt+"/setup/rules", url.Values{"add": {"1"}, "kind": {"off"}, "must": {"1"}, "person": {tom[1]}, "event": {""}})
	page = do(t, h, http.MethodGet, redirected(t, h, tt+"/officials", url.Values{"action": {"rota"}}), nil).Body.String()
	if !strings.Contains(page, "Tom doesn&#39;t officiate (must)") || !strings.Contains(page, "· 2 of 3 seats filled") || strings.Contains(page, "The setup has changed") {
		t.Errorf("Tom is off the panels: %s", page)
	}
	// Put back by hand, the rule is reported as not kept.
	flight := regexp.MustCompile(`name="flight" value="(\d+)">\s*<div class="select is-small"><select name="seat" aria-label="Seat on BUCS L3`).FindStringSubmatch(page)
	if flight == nil {
		t.Fatal("BUCS L3's seat form")
	}
	for seat := range 3 {
		redirected(t, h, tt+"/officials", url.Values{"action": {"seat"}, "flight": {flight[1]}, "seat": {string(rune('0' + seat))}, "person": {tom[1]}})
		page = do(t, h, http.MethodGet, tt, nil).Body.String()
		if strings.Contains(page, "Your rules about officials not kept") {
			break
		}
	}
	if !strings.Contains(page, "Your rules about officials not kept") {
		t.Error("a broken rule about an official is reported")
	}
	if rec := do(t, h, http.MethodPost, tt+"/officials", url.Values{"action": {"seat"}, "flight": {flight[1]}, "seat": {"0"}, "person": {"m:nobody"}}); !strings.Contains(rec.Header().Get("Location"), "nothing+was+changed") {
		t.Error("only officials can be given a seat")
	}

	// The sheets and the rota.
	if sheet := do(t, h, http.MethodGet, tt+"/print?sheet=marshal", nil).Body.String(); !strings.Contains(sheet, "Execution judge: Dara") || !strings.Contains(sheet, "Chair of judges: Ann") {
		t.Error("the marshal sheet lists the panel")
	}
	rota := do(t, h, http.MethodGet, tt+"/print?sheet=rota", nil).Body.String()
	if !strings.Contains(rota, "Officials rota") || !strings.Contains(rota, "Track 1 · Tumbling Novice · Chair of judges") {
		t.Errorf("the rota sheet: %s", rota)
	}

	// Published, Dara sees her duties.
	if strings.Contains(do(t, h, http.MethodGet, dara, nil).Body.String(), "Officiating") {
		t.Error("not before publishing")
	}
	redirected(t, h, tt+"/publish", url.Values{"on": {"1"}})
	if !strings.Contains(do(t, h, http.MethodGet, dara, nil).Body.String(), "BUCS L3 · Execution judge") {
		t.Error("Dara sees she judges BUCS L3")
	}
	// And the score sheets of the flights she's on the panel for, only.
	if !strings.Contains(do(t, h, http.MethodGet, dara, nil).Body.String(), dara+"/score-sheets") {
		t.Fatal("Dara's page links to her panels' score sheets")
	}
	sheets := do(t, h, http.MethodGet, dara+"/score-sheets", nil).Body.String()
	if !strings.Contains(sheets, "<h1 class=\"title is-5 mb-1\">BUCS L3") || strings.Contains(sheets, "Tumbling Novice</h1>") || !strings.Contains(sheets, "you don&#39;t need to") && !strings.Contains(sheets, "you don't need to") {
		t.Errorf("BUCS L3's sheet, not tumbling's: %s", sheets)
	}
	if strings.Contains(do(t, h, http.MethodGet, withoutQuery(redirected(t, h, enter, url.Values{"gymnast": {"Fay"}, "level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}})), nil).Body.String(), "/score-sheets") {
		t.Error("no link for someone who doesn't officiate")
	}
}

func TestBlockOfficials(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	redirected(t, h, admin+"/officials/add", url.Values{"name": {"Mary"}, "judge-trampoline": {"1"}, "chair-trampoline": {"1"}})
	tt := admin + "/timetable"
	loc := redirected(t, h, tt+"/setup/blocks", url.Values{"add": {"1"}, "name": {"Ad hoc"}, "minutes": {"30"}, "day": {"0"}, "at": {"10:00"}, "chair": {"1"}, "judges": {"1"}})
	if !strings.Contains(loc, "needs+2+officials") {
		t.Errorf("the block says what it needs: %s", loc)
	}
	page := do(t, h, http.MethodGet, redirected(t, h, tt+"/plan", url.Values{}), nil).Body.String()
	if !strings.Contains(page, "· 1 of 2 seats filled") || !strings.Contains(page, "Ad hoc on Panel 1: no one for 1 execution judge") {
		t.Fatalf("the block's panel: %s", page)
	}
	mary := regexp.MustCompile(`<option value="([^"]+)">Mary</option>`).FindStringSubmatch(page)
	redirected(t, h, tt+"/officials", url.Values{"action": {"seat"}, "block": {"0"}, "seat": {"1"}, "person": {mary[1]}})
	page = do(t, h, http.MethodGet, tt, nil).Body.String()
	if !strings.Contains(page, "Mary has two seats on Ad hoc&#39;s panel") || !strings.Contains(page, "· 2 of 2 seats filled") {
		t.Errorf("a seat set by hand: %s", page)
	}
	if rota := do(t, h, http.MethodGet, tt+"/print?sheet=rota", nil).Body.String(); !strings.Contains(rota, "Panel 1 · Ad hoc · Chair of judges") {
		t.Error("the rota lists block duties")
	}
}

func TestDutiesByEvent(t *testing.T) {
	flight := func(n, start int) competitions.ScheduledFlight {
		return competitions.ScheduledFlight{Flight: competitions.Flight{Level: "BUCS L7", Category: "Men", Number: n, Of: 3}, Area: "Panel 2", Start: start, End: start + 60,
			Officials: []competitions.Duty{{Role: competitions.RoleExecution, Person: "m:1"}}}
	}
	s := competitions.Schedule{Setup: competitions.Setup{Days: []competitions.Day{{Name: "Friday"}}}, Flights: []competitions.ScheduledFlight{flight(1, 570), flight(2, 630), flight(3, 690)}}
	if got := dutiesIn(s, "m:1"); len(got) != 1 || got[0] != "Friday 09:30–12:30 · Panel 2 · BUCS L7 Men · all 3 flights · Execution judge" {
		t.Errorf("one line for the event: %q", got)
	}
	s.Flights[2].Officials[0].Person = "m:2"
	if got := dutiesIn(s, "m:1"); len(got) != 1 || got[0] != "Friday 09:30–11:30 · Panel 2 · BUCS L7 Men · flights 1–2 of 3 · Execution judge" {
		t.Errorf("two of its flights: %q", got)
	}
}
