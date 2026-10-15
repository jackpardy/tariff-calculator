package web

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"tariffCalculator/views"
)

func TestScreenFit(t *testing.T) {
	names := func(n int) []views.FlightGymnast {
		out := make([]views.FlightGymnast, n)
		for i := range out {
			out[i].Name = "G"
		}
		return out
	}
	busy := func(n int) []views.ScreenArea {
		out := make([]views.ScreenArea, n)
		for i := range out {
			out[i] = views.ScreenArea{Name: "Area", Gymnasts: names(12), NextGymnasts: names(10)}
		}
		return out
	}

	// Seven busy areas, as at ISTO: two rows of four, fewer names each.
	p := views.ScreenPage{Areas: busy(7)}
	p.Fit()
	if p.Rows != 2 || p.Columns != 4 || p.Size != 4 {
		t.Errorf("seven: %d rows of %d, size %d", p.Rows, p.Columns, p.Size)
	}
	if a := p.Areas[0]; len(a.Gymnasts) != 5 || a.More != 7 || len(a.NextGymnasts) != 4 || a.NextMore != 6 {
		t.Errorf("seven: %d named and %d more now, %d and %d next", len(a.Gymnasts), a.More, len(a.NextGymnasts), a.NextMore)
	}

	// Three busy and two quiet: one row of three; the quiet ones are strips.
	p = views.ScreenPage{Areas: append(busy(3), views.ScreenArea{Idle: true}, views.ScreenArea{Idle: true, Finished: true})}
	p.Fit()
	if p.Rows != 1 || p.Columns != 3 || p.Size != 2 || p.Areas[0].More != 4 || len(p.Areas[0].Gymnasts) != 8 {
		t.Errorf("three: %d rows of %d, size %d, %d named", p.Rows, p.Columns, p.Size, len(p.Areas[0].Gymnasts))
	}

	// All quiet (before the day, a long break): they all keep their cards.
	p = views.ScreenPage{Areas: []views.ScreenArea{{Idle: true}, {Idle: true}}}
	p.Fit()
	if p.Areas[0].Idle || p.Areas[1].Idle || p.Columns != 2 || p.Size != 1 {
		t.Errorf("all quiet: %+v", p)
	}
}

func TestScreenCards(t *testing.T) {
	p := views.ScreenPage{Competition: "ISTO", Published: true, At: 1, Zone: "Europe/Dublin", Notices: []string{"Lunch extended by 15 minutes"}, Areas: []views.ScreenArea{
		{Name: "Panel 1", Colour: 0, Now: "BUCS L7 Women · flight 2 of 5", NowTimes: "10:40–11:50", Planned: true, Late: "Running 1 h 5 min late", Behind: true,
			Gymnasts: []views.FlightGymnast{{Name: "Aoife O'Brien", Club: "UCD"}, {Name: "Seán Ó Briain", Club: "DCU", Scratched: true}, {Name: "Kit Lee"}},
			Next:     "BUCS L7 Women · flight 3 of 5", NextWarmUp: "11:50",
			NextGymnasts: []views.FlightGymnast{{Name: "Méabh Ryan", Club: "UL"}}},
		{Name: "Panel 2", Colour: 1, Idle: true, Next: "BUCS L6 Men · flight 1 of 2", NextWarmUp: "14:00"},
		{Name: "Track 1", Colour: 2, Idle: true, Finished: true},
	}}
	p.Fit()
	var b bytes.Buffer
	if err := views.Screen(p).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	page := b.String()
	for _, want := range []string{
		`<p class="screen-notice" data-notice><span class="screen-notice-text">Lunch extended by 15 minutes</span> </p>`, // one: no count
		`<section class="screen-card screen-colour-0">`, `<h2 class="screen-area">Panel 1</h2>`,
		`<span class="screen-late is-behind">Running 1 h 5 min late</span>`, "· not started yet",
		`<span class="screen-club">UCD</span>`, `<li class="comp-scratched">`,
		`Next · warm-up 11:50`, "Méabh Ryan", `<span class="screen-club">UL</span>`,
		`<div class="screen-strip screen-colour-1"><strong class="screen-band">Panel 2</strong> <span>Next: BUCS L6 Men · flight 1 of 2 · warm-up 14:00</span>`,
		`<div class="screen-strip screen-colour-2"><strong class="screen-band">Track 1</strong> <span>Finished for the day</span>`,
		`style="--screen-columns: 1; --screen-rows: 1;"`, `data-zone="Europe/Dublin"`, `js/screen.js`,
		`<noscript><meta http-equiv="refresh" content="30"></noscript>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the screen has %s", want)
		}
	}
	if strings.Contains(page, `screen-club"></span>`) {
		t.Error("no club shown for someone without one")
	}
}

func TestMinutesText(t *testing.T) {
	for m, want := range map[int]string{5: "5 min", 59: "59 min", 60: "1 h", 65: "1 h 5 min", 653: "10 h 53 min"} {
		if got := minutesText(m); got != want {
			t.Errorf("%d: %q, want %q", m, got, want)
		}
	}
}

func TestScreenAnnouncements(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	screen := madeLink(t, h, admin, "Hall screen", "screen")
	if page := do(t, h, http.MethodGet, admin+"/desk", nil).Body.String(); !strings.Contains(page, "On the venue screen") || !strings.Contains(page, `action="`+admin+`/desk/screen"`) {
		t.Fatal("the desk offers announcements")
	}
	for _, bad := range []url.Values{{"text": {"  "}, "for": {"15"}}, {"text": {strings.Repeat("x", maxNotice+1)}, "for": {"15"}}, {"text": {"Hi"}, "for": {"never"}}} {
		if loc := redirected(t, h, admin+"/desk/screen", bad); !strings.Contains(loc, "Nothing+was+shown") {
			t.Errorf("%v: %s", bad, loc)
		}
	}
	redirected(t, h, admin+"/desk/screen", url.Values{"text": {"Lunch   extended by 15 minutes"}, "for": {"30"}})
	chair := madeLink(t, h, admin, "Chair", "chair")
	redirected(t, h, chair+"/desk/screen", url.Values{"text": {"Panel 2 judges to the desk, O'Neill's too"}, "for": {"0"}})
	for _, link := range []string{screen, madeLink(t, h, admin, "Cards", "cards")} {
		if rec := do(t, h, http.MethodPost, link+"/desk/screen", url.Values{"text": {"x"}, "for": {"15"}}); rec.Code != http.StatusForbidden {
			t.Errorf("%s can't put one up: %d", link, rec.Code)
		}
	}

	// The screen shows them before there's a timetable, latest first.
	page := do(t, h, http.MethodGet, screen+"/screen", nil).Body.String()
	i, j := strings.Index(page, "Panel 2 judges to the desk, O&#39;Neill&#39;s too"), strings.Index(page, "Lunch extended by 15 minutes")
	if i < 0 || j < 0 || i > j {
		t.Errorf("both announcements, latest first: %s", page)
	}
	// One at a time: the latest shows, the other waits its turn, each counted.
	if !strings.Contains(page, `<p class="screen-notice" data-notice><span class="screen-notice-text">Panel 2 judges`) || !strings.Contains(page, `<p class="screen-notice" data-notice hidden><span class="screen-notice-text">Lunch`) ||
		!strings.Contains(page, `<span class="screen-notice-count">1 of 2</span>`) || !strings.Contains(page, `<span class="screen-notice-count">2 of 2</span>`) {
		t.Errorf("the announcements take turns, counted: %s", page)
	}
	desk := do(t, h, http.MethodGet, admin+"/desk", nil).Body.String()
	if !strings.Contains(desk, "by Chair") || !strings.Contains(desk, "until it's taken down") || !strings.Contains(desk, "· until ") {
		t.Errorf("the desk lists what's up, by whom, and until when: %s", desk)
	}
	if history := do(t, h, http.MethodGet, admin+"/history", nil).Body.String(); !strings.Contains(history, "Put up on the venue screen: Lunch extended by 15 minutes") {
		t.Error("the history says what was put up")
	}

	// Taking one down.
	remove := formAction(t, desk, admin+"/desk/screen/")
	redirected(t, h, remove, url.Values{})
	if page := do(t, h, http.MethodGet, screen+"/screen", nil).Body.String(); strings.Count(page, `class="screen-notice"`) != 1 || strings.Contains(page, "screen-notice-count") {
		t.Error("one taken down, one left")
	}
}
