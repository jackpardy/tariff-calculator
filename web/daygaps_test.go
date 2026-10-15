package web

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
)

// Payments: a bad amount and an unknown payer are refused; one recorded
// and one removed show in the balances on the Fees page and the invoice;
// removing one that's gone does nothing.
func TestPayments(t *testing.T) {
	h, _ := competitionServerAt(t, dayBefore)
	admin := created(t, h, newCompetition())
	aClub(t, h, admin, "UCD", true, "Ann", "Bea")
	fees := admin + "/fees"
	redirected(t, h, fees+"/settings", url.Values{"fee-": {"12"}, "club": {"25"}})
	ucd := feeAccounts(t, do(t, h, http.MethodGet, fees, nil).Body.String())["UCD"]
	if ucd.Due != "€49" || ucd.Balance != "€49" || ucd.Payer == "" {
		t.Fatalf("UCD owes €49: %+v", ucd)
	}
	for _, amount := range []string{"", "abc", "0", "-5"} {
		if notice := noticeOf(t, redirected(t, h, fees+"/payments", url.Values{"payer": {ucd.Payer}, "amount": {amount}})); notice != "Say how much was paid, like 12 or 12.50." {
			t.Errorf("amount %q: %q", amount, notice)
		}
	}
	for _, payer := range []string{"", "club:nonsense", "entry:nonsense", "nonsense"} {
		if rec := do(t, h, http.MethodPost, fees+"/payments", url.Values{"payer": {payer}, "amount": {"10"}}); rec.Code != http.StatusNotFound {
			t.Errorf("payer %q: %d", payer, rec.Code)
		}
	}
	if got := feeAccounts(t, do(t, h, http.MethodGet, fees, nil).Body.String())["UCD"]; got.Paid != "€0" || len(got.Payments) != 0 {
		t.Errorf("nothing recorded: %+v", got)
	}

	if notice := noticeOf(t, redirected(t, h, fees+"/payments", url.Values{"payer": {ucd.Payer}, "amount": {"20.50"}, "note": {"Cash"}})); notice != "€20.50 recorded." {
		t.Errorf("recorded: %q", notice)
	}
	redirected(t, h, fees+"/payments", url.Values{"payer": {ucd.Payer}, "amount": {"10"}})
	page := do(t, h, http.MethodGet, fees, nil).Body.String()
	ucd = feeAccounts(t, page)["UCD"]
	if ucd.Paid != "€30.50" || ucd.Balance != "€18.50" || len(ucd.Payments) != 2 || !strings.Contains(ucd.Payments[0], "· Cash ·") {
		t.Errorf("two payments, €18.50 to come: %+v", ucd)
	}
	if inv := invoiceOf(t, do(t, h, http.MethodGet, ucd.Invoice, nil).Body.String()); inv.ToPay != "To pay €18.50" || !slices.Contains(inv.Lines, "Club fee: €25") {
		t.Errorf("the invoice: %+v", inv)
	}

	removes := regexp.MustCompile(`action="(`+regexp.QuoteMeta(fees)+`/payments/[^"]+/remove)"`).FindAllStringSubmatch(page, -1)
	if len(removes) != 2 {
		t.Fatalf("each payment can be removed: %d", len(removes))
	}
	if notice := noticeOf(t, redirected(t, h, removes[1][1], nil)); notice != "Payment removed." {
		t.Errorf("removed: %q", notice)
	}
	ucd = feeAccounts(t, do(t, h, http.MethodGet, fees, nil).Body.String())["UCD"]
	if ucd.Paid != "€20.50" || ucd.Balance != "€28.50" || len(ucd.Payments) != 1 {
		t.Errorf("one payment left: %+v", ucd)
	}
	if inv := invoiceOf(t, do(t, h, http.MethodGet, ucd.Invoice, nil).Body.String()); inv.ToPay != "To pay €28.50" {
		t.Errorf("the invoice after removing: %+v", inv)
	}
	// Removed already, or never there: nothing changes, nothing breaks.
	for _, path := range []string{removes[1][1], fees + "/payments/nonsense/remove"} {
		if notice := noticeOf(t, redirected(t, h, path, nil)); notice != "Payment removed." {
			t.Errorf("%s: %q", path, notice)
		}
	}
	if got := feeAccounts(t, do(t, h, http.MethodGet, fees, nil).Body.String())["UCD"]; got.Balance != "€28.50" {
		t.Errorf("still €28.50: %+v", got)
	}
}

// The desk refuses what it can't send, saying why: no flight picked, a
// panel with no officials, and a competition's messages all used.
func TestDeskRefusals(t *testing.T) {
	s := newTestServer(t, atTime(dayBefore))
	h := s.h
	admin := created(t, h, newCompetition())
	publishedWith(t, h, admin, "Ann", "Bea")
	desk := admin + "/desk"
	for _, tc := range []struct {
		form url.Values
		want string
	}{
		{url.Values{"to": {"flight"}, "flight": {""}, "text": {"Hello"}}, "Pick a flight. Nothing was sent."},
		{url.Values{"to": {"flight"}, "flight": {"nonsense"}, "text": {"Hello"}}, "Pick a flight. Nothing was sent."},
		{url.Values{"to": {"panel"}, "panel": {"0|Panel 1"}, "text": {"Hello"}}, "No officials have a seat on that panel yet. Nothing was sent."},
		{url.Values{"to": {"panel"}, "panel": {"7|Panel 1"}, "text": {"Hello"}}, "Pick a panel. Nothing was sent."},
		{url.Values{"to": {"flight"}, "flight": {firstFlightKey(t, h, admin+"/day")}, "flightChoice": {"1"}, "text": {"Hello"}}, "Tick gymnasts, officials or both for the flight. Nothing was sent."},
	} {
		if notice := noticeOf(t, redirected(t, h, desk, tc.form)); notice != tc.want {
			t.Errorf("%v: %q, want %q", tc.form, notice, tc.want)
		}
	}

	// The competition's last message, then none.
	ctx := context.Background()
	c, err := s.st.CompetitionByAdmin(ctx, strings.TrimPrefix(admin, "/competitions/admin/"))
	if err != nil {
		t.Fatal(err)
	}
	for range store.MaxDeskMessages - 1 {
		if _, err := s.st.SendDeskMessage(ctx, c.ID, store.DeskMessage{Text: "Filler", Audience: "Nobody"}); err != nil {
			t.Fatal(err)
		}
	}
	flight := url.Values{"to": {"flight"}, "flight": {firstFlightKey(t, h, admin+"/day")}, "text": {"The last one"}}
	if notice := noticeOf(t, redirected(t, h, desk, flight)); !strings.HasPrefix(notice, "Sent to BUCS L3.") {
		t.Errorf("the last message is sent: %q", notice)
	}
	if notice := noticeOf(t, redirected(t, h, desk, flight)); notice != "A competition can keep 500 messages. Nothing was sent." {
		t.Errorf("then no more: %q", notice)
	}
}

// Notify from the day page can tell the club of a judge who is one of its
// members but isn't competing; an official the organiser added has no club.
func TestNotifyClubOfJudge(t *testing.T) {
	h, _ := competitionServerAt(t, dayBefore)
	admin := created(t, h, newCompetition())
	redirected(t, h, admin+"/officials/settings", url.Values{
		"panel-trampoline-chair": {"1"}, "panel-trampoline-execution": {"1"}, "panel-trampoline-difficulty": {"0"}, "panel-trampoline-recorder": {"0"}, "panel-trampoline-marshal": {"0"}})
	ucd := aClub(t, h, admin, "UCD", false, "Ann", "Jude")
	redirected(t, h, formAction(t, memberCompetitionPage(t, h, ucd.members["Ann"]), ucd.members["Ann"]+"/competitions/"), bucsEntry(""))
	redirected(t, h, formAction(t, memberCompetitionPage(t, h, ucd.members["Jude"]), ucd.members["Jude"]+"/competitions/")+"/offer", url.Values{"judge-trampoline": {"1"}})
	redirected(t, h, formAction(t, do(t, h, http.MethodGet, ucd.page, nil).Body.String(), ucd.page+"/competitions/"), url.Values{"which": {"all"}})
	redirected(t, h, admin+"/officials/add", url.Values{"name": {"Mary"}, "judge-trampoline": {"1"}, "chair-trampoline": {"1"}})
	redirected(t, h, admin+"/timetable/plan", url.Values{})
	redirected(t, h, admin+"/timetable/publish", url.Values{"action": {"publish"}})

	page := do(t, h, http.MethodGet, admin+"/day?day=0", nil).Body.String()
	jude, flight := panelSeat(t, page, "BUCS L3", "Jude")
	mary, _ := panelSeat(t, page, "BUCS L3", "Mary")
	if !strings.HasPrefix(jude, "m:") {
		t.Fatalf("Jude is known by her member key: %s", jude)
	}
	notify := func(person string) string {
		t.Helper()
		return noticeOf(t, redirected(t, h, admin+"/day/notify", url.Values{"person": {person}, "flight": {flight}, "as": {"official"}, "club": {"1"}, "day": {"0"}}))
	}
	if notice := notify(jude); !strings.HasPrefix(notice, "Sent to Jude's club") {
		t.Errorf("Jude's club is told: %q", notice)
	}
	if got := myCompetitionOf(t, do(t, h, http.MethodGet, ucd.page, nil).Body.String()).Desk; len(got) != 1 || !strings.HasPrefix(got[0], "Jude should be at Panel 1 now to officiate BUCS L3") {
		t.Errorf("UCD's comp sec sees it: %q", got)
	}
	if notice := notify(mary); notice != "Mary doesn't belong to a club. Nothing was sent." {
		t.Errorf("Mary has no club: %q", notice)
	}
}

// Which days are the competition's, in Irish and UK time.
func TestCompetitionDay(t *testing.T) {
	two := &competitions.Schedule{Setup: competitions.Setup{Days: []competitions.Day{{Name: "Saturday"}, {Name: "Sunday"}}}}
	at := func(c store.Competition, date string, timetable *competitions.Schedule) store.Competition {
		c.Date, c.Timetable = date, timetable
		return c
	}
	var c store.Competition
	for _, tc := range []struct {
		what string
		c    store.Competition
		now  time.Time
		want bool
	}{
		{"the day before, last thing", at(c, "2030-03-16", nil), inDublin(2030, time.March, 15, 23, 59), false},
		{"the day, first thing", at(c, "2030-03-16", nil), inDublin(2030, time.March, 16, 0, 0), true},
		{"the next day, with one day", at(c, "2030-03-16", nil), inDublin(2030, time.March, 17, 10, 0), false},
		{"the next day, with two", at(c, "2030-03-16", two), inDublin(2030, time.March, 17, 10, 0), true},
		{"the day after two", at(c, "2030-03-16", two), inDublin(2030, time.March, 18, 0, 0), false},
		{"in summer time: 23:30 UTC is the next day in Dublin", at(c, "2030-06-16", nil), time.Date(2030, time.June, 15, 23, 30, 0, 0, time.UTC), true},
		{"no date", at(c, "", nil), inDublin(2030, time.March, 16, 10, 0), false},
	} {
		if got := competitionDay(tc.c, tc.now); got != tc.want {
			t.Errorf("%s: %v", tc.what, got)
		}
	}
}

// With no waiting on the day, a change is told at once on the day, but
// still after ten minutes before it.
func TestNotifyNoWaitOnTheDay(t *testing.T) {
	s := newTestServer(t, atTime(dayBefore), withMail())
	h := s.h
	admin := created(t, h, newCompetition())
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	own := withoutQuery(redirected(t, h, enter, bucsEntry("Ann Ryan")))
	subscribeByEmail(t, h, s.mail, own+"/notify", "ann@example.com")
	redirected(t, h, admin+"/no-wait", url.Values{"on": {"1"}})
	entry := regexp.MustCompile(`href="(` + regexp.QuoteMeta(admin) + `/entries/[^"]+)"`).FindStringSubmatch(do(t, h, http.MethodGet, admin, nil).Body.String())[1]
	check := func(checked, note string) {
		t.Helper()
		redirected(t, h, entry+"/check", url.Values{"checked": {checked}, "note": {note}})
		s.p.notify.tell(context.Background())
	}

	check("1", "Element 7 repeats 3")
	if len(s.mail.sent) != 0 {
		t.Errorf("before the day, changes wait: %+v", s.mail.sent)
	}
	s.clock.set(dayBefore.Add(11 * time.Minute))
	s.p.notify.tell(context.Background())
	if len(s.mail.sent) != 1 {
		t.Fatalf("told after the wait: %+v", s.mail.sent)
	}

	s.mail.sent = nil
	s.clock.set(onTheDay(9, 0))
	check("0", "")
	check("1", "Element 8 repeats 4")
	if len(s.mail.sent) != 1 || s.mail.sent[0].to != "ann@example.com" || !strings.Contains(s.mail.sent[0].body, "Ann Ryan") {
		t.Errorf("on the day, told at once: %+v", s.mail.sent)
	}
}

// The marshal's taps in a tumbling flight name passes, and when everyone is
// scratched there's no one to tap on to.
func TestTapsTumblingAndAllScratched(t *testing.T) {
	h, clock := competitionServerAt(t, dayBefore)
	form := newCompetition()
	form.Set("tumbling", "Novice")
	admin := created(t, h, form)
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	for _, name := range []string{"Tara", "Tom"} {
		redirected(t, h, enter, url.Values{"gymnast": {name}, "discipline": {"tumbling"}, "level": {"Novice"}})
	}
	redirected(t, h, admin+"/timetable/plan", url.Values{})
	redirected(t, h, admin+"/timetable/publish", url.Values{"action": {"publish"}})
	clock.set(onTheDay(9, 0))
	day := admin + "/day?day=0"
	track := func() dayFlight {
		t.Helper()
		a := dayPageAreas(t, do(t, h, http.MethodGet, day, nil).Body.String())["Track 1"]
		if len(a.Flights) != 1 {
			t.Fatalf("one flight on Track 1: %+v", a)
		}
		return a.Flights[0]
	}
	f := track()
	redirected(t, h, admin+"/day/flight", url.Values{"flight": {f.Key}, "what": {"start"}, "day": {"0"}})
	tap := func() string {
		t.Helper()
		return noticeOf(t, tapUp(t, h, admin, f.Key, "next"))
	}
	tap()
	if got := track(); got.Up != f.Order[0] || got.Routine != "1st pass" {
		t.Errorf("tumbling has passes: %+v", got)
	}

	page := do(t, h, http.MethodGet, day, nil).Body.String()
	for _, name := range f.Order {
		redirected(t, h, admin+"/day/checkin", url.Values{"entry": {checkinEntry(t, page, name)}, "status": {"scratched"}, "day": {"0"}})
	}
	// Tara was on when she was scratched: she stays shown, and there's
	// no one to tap on to.
	if got := track(); got.Up != f.Order[0] {
		t.Errorf("who was on stays: %+v", got)
	}
	if notice := tap(); notice != "No one left to compete in Tumbling Novice: everyone is scratched." {
		t.Errorf("a tap with everyone scratched: %q", notice)
	}
	// Started again, there's no one to start with.
	redirected(t, h, admin+"/day/flight", url.Values{"flight": {f.Key}, "what": {"undo-start"}, "day": {"0"}})
	redirected(t, h, admin+"/day/flight", url.Values{"flight": {f.Key}, "what": {"start"}, "day": {"0"}})
	if got := track(); got.UpLabel != "No one left to compete: everyone is scratched." || got.Up != "" {
		t.Errorf("the day page says so: %+v", got)
	}
}

// What the delay page says about each flight that changes, and each clash.
func TestDelayView(t *testing.T) {
	change := func(c competitions.DelayChange) competitions.DelayChange {
		c.Flight, c.Area, c.NewArea = "BUCS L3", "Panel 1", "Panel 1"
		if c.Start == 0 {
			c.Start, c.End = 600, 660
		}
		if c.NewStart == 0 {
			c.NewStart, c.NewEnd = c.Start, c.End
		}
		return c
	}
	for _, tc := range []struct {
		c   competitions.DelayChange
		why string
	}{
		{change(competitions.DelayChange{Moved: true, PastEnd: true, NewArea: "Panel 2"}), "moved: it would have run past the end of the day"},
		{change(competitions.DelayChange{Moved: true, Clashes: true}), "moved: someone in it would have been needed in two places"},
		{change(competitions.DelayChange{After: "Lunch", NewStart: 690, NewEnd: 750}), "90 min later: it wouldn't finish before Lunch, so it waits for it to end"},
		{change(competitions.DelayChange{NewStart: 620, NewEnd: 680}), "20 min later"},
		{change(competitions.DelayChange{NewStart: 590, NewEnd: 650}), "10 min earlier"},
		{change(competitions.DelayChange{NewStart: 600, NewEnd: 675}), "under way: finishes 15 min late"},
		{change(competitions.DelayChange{NewStart: 600, NewEnd: 650}), "10 min shorter"},
		{change(competitions.DelayChange{NewStart: 600, NewEnd: 660}), ""},
	} {
		v := delayView(competitions.DelayReport{Changes: []competitions.DelayChange{tc.c}}, nil)
		if len(v.Changes) != 1 || v.Changes[0].Why != tc.why {
			t.Errorf("%+v: %+v, want %q", tc.c, v.Changes, tc.why)
		}
	}
	v := delayView(competitions.DelayReport{Changes: []competitions.DelayChange{change(competitions.DelayChange{NewStart: 620, NewEnd: 680})}}, nil)
	if c := v.Changes[0]; c.Was != "Panel 1 10:00–11:00" || c.Now != "Panel 1 10:20–11:20" || c.Later != 20 {
		t.Errorf("was and now: %+v", c)
	}
	v = delayView(competitions.DelayReport{Clashes: []competitions.Clash{
		{Person: "m:1", First: "BUCS L3", Second: "Tumbling Novice", At: 630},
		{Person: "m:2", First: "BUCS L4", Second: "DMT Open", At: 700},
	}}, map[string]string{"m:1": "Ann"})
	if want := []string{"Ann: BUCS L3 and Tumbling Novice at 10:30", "Someone: BUCS L4 and DMT Open at 11:40"}; !slices.Equal(v.Clashes, want) {
		t.Errorf("clashes: %q", v.Clashes)
	}
	day := competitions.DayReport{Name: "Saturday", FlightsEnd: "17:00", FreeMinutes: 30, End: "17:30"}
	later := day
	later.FlightsEnd, later.FreeMinutes = "17:20", 10
	v = delayView(competitions.DelayReport{Before: []competitions.DayReport{day, day}, After: []competitions.DayReport{later, day}}, nil)
	if len(v.Days) != 1 || v.Days[0].WasEnd != "17:00" || v.Days[0].NowEnd != "17:20" || v.Days[0].NowFree != 10 {
		t.Errorf("only the day that changes: %+v", v.Days)
	}
}
