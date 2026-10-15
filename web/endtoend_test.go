package web

import (
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"

	"tariffCalculator/views"
)

// myDayPath is a member's My competition page, linked from their page for
// the competition.
func myDayPath(t *testing.T, h http.Handler, member string) string {
	t.Helper()
	m := regexp.MustCompile(`href="(` + regexp.QuoteMeta(member) + `/competitions/[^"/]+/day)"`).FindStringSubmatch(memberCompetitionPage(t, h, member))
	if m == nil {
		t.Fatalf("%s's page doesn't link to their competition", member)
	}
	return m[1]
}

// The dry run's path, end to end on a fixed clock: a competition with two
// clubs and an individual; members join and enter, coaches sign off and the
// clubs send; the timetable is planned and published; on the day a flight
// starts late, the marshal taps through it, a gymnast is scratched and it
// finishes; then a message from the desk, a late change accepted and a
// payment recorded. Each stage checks the venue screen, a member's My
// competition and the organiser's On the day.
func TestCompetitionEndToEnd(t *testing.T) {
	s := newTestServer(t, atTime(dayBefore), withMail())
	h := s.h
	stage := func(name string, f func(t *testing.T)) {
		t.Helper()
		if !t.Run(name, f) {
			t.FailNow()
		}
	}
	var (
		admin, screen, day string
		clubs              = map[string]string{} // club pages by name
		members            = map[string]string{} // member pages by name
		my                 = map[string]string{} // My competition, by member
		ivy                string                // her entry
		order              []string              // the flight's running order
		flight             string                // its key
	)
	screenCard := func(t *testing.T) screenCard {
		t.Helper()
		c, ok := screenCards(t, do(t, h, http.MethodGet, screen+"/screen", nil).Body.String())["Panel 1"]
		if !ok {
			t.Fatal("no Panel 1 on the screen")
		}
		return c
	}
	myPageOf := func(t *testing.T, name string) myPage {
		t.Helper()
		return myCompetitionOf(t, do(t, h, http.MethodGet, my[name], nil).Body.String())
	}

	stage("entries", func(t *testing.T) {
		form := newCompetition()
		form.Set("signoff", "1")
		form["level"] = append(form["level"], "builtin-level:bucs-l4")
		admin = created(t, h, form)
		screen = madeLink(t, h, admin, "Hall screen", "screen")
		day = admin + "/day?day=0"
		settings := do(t, h, http.MethodGet, admin+"/settings", nil).Body.String()
		clubLink, enter := pathIn(t, settings, "/competitions/club/"), pathIn(t, settings, "/competitions/enter/")

		for club, names := range map[string][]string{"UCD": {"Ann", "Bea"}, "DCU": {"Cal", "Dee"}} {
			page := withoutQuery(redirected(t, h, "/clubs", url.Values{"name": {club}}))
			clubs[club] = page
			redirected(t, h, clubLink, url.Values{"clubAdmin": {page}})
			coach := pathIn(t, do(t, h, http.MethodPost, page+"/coaches", url.Values{"name": {"Coach " + club}}).Body.String(), "/clubs/coach/")
			join := pathIn(t, do(t, h, http.MethodGet, page+"/members", nil).Body.String(), "/clubs/join/")
			for _, name := range names {
				members[name] = withoutQuery(redirected(t, h, join, url.Values{"name": {name}}))
				redirected(t, h, formAction(t, memberCompetitionPage(t, h, members[name]), members[name]+"/competitions/"), bucsEntry(""))
				my[name] = myDayPath(t, h, members[name])
				if got := myPageOf(t, name); len(got.Events) != 0 {
					t.Errorf("%s's entry hasn't reached the competition: %+v", name, got)
				}
			}
			// The coach signs off everyone in the club, then the comp sec sends.
			signed := 0
			for _, m := range regexp.MustCompile(`href="(`+regexp.QuoteMeta(coach)+`/members/[^"]+)"`).FindAllStringSubmatch(do(t, h, http.MethodGet, coach, nil).Body.String(), -1) {
				redirected(t, h, unescape(m[1]), url.Values{"signed": {"1"}})
				signed++
			}
			if signed != len(names) {
				t.Errorf("Coach %s signed off %d of %d", club, signed, len(names))
			}
			redirected(t, h, formAction(t, clubEntriesPage(t, h, page), page+"/competitions/"), url.Values{"which": {"all"}})
		}
		ivy = withoutQuery(redirected(t, h, enter, bucsEntry("Ivy")))
		redirected(t, h, pathIn(t, do(t, h, http.MethodGet, ivy, nil).Body.String(), "/competitions/signoff/"), url.Values{"coach": {"Own coach"}, "signed": {"1"}})

		dash := do(t, h, http.MethodGet, admin, nil).Body.String()
		if strings.Contains(dash, "Waiting for sign-off") {
			t.Error("every entry is signed off")
		}
		for _, name := range []string{"Ann", "Bea", "Cal", "Dee", "Ivy"} {
			if !strings.Contains(dash, ">"+name+"</a>") {
				t.Errorf("%s's entry is on the dashboard", name)
			}
		}
		got := myPageOf(t, "Ann")
		if len(got.Events) != 1 || got.Events[0].Event != "BUCS L3" || got.Events[0].Flight != "" || !got.Waiting {
			t.Errorf("Ann's entry has reached it; no timetable yet: %+v", got)
		}
		if got := screenEmpty(t, do(t, h, http.MethodGet, screen+"/screen", nil).Body.String()); got != "The timetable isn't published yet" {
			t.Errorf("the screen before publishing: %q", got)
		}
		if rec := do(t, h, http.MethodGet, day, nil); rec.Code != http.StatusConflict {
			t.Errorf("no On the day before publishing: %d", rec.Code)
		}
	})

	stage("plan and publish", func(t *testing.T) {
		redirected(t, h, admin+"/timetable/plan", url.Values{})
		redirected(t, h, admin+"/timetable/publish", url.Values{"action": {"publish"}})
		area := panelOne(t, h, day)
		f := area.Flights[0]
		flight, order = f.Key, f.Order
		if f.Name != "BUCS L3" || f.Planned != "09:00–09:35" || len(order) != 5 || area.Status != "Nothing started yet" {
			t.Fatalf("one flight of all five on Panel 1, 09:00–09:35: %+v", area)
		}
		if got := myPageOf(t, "Ann"); len(got.Events) != 1 || !strings.HasPrefix(got.Events[0].Flight, "BUCS L3 · Panel 1") || got.Events[0].WarmUp != "09:00" || got.Waiting {
			t.Errorf("Ann's flight and warm-up: %+v", got)
		}
		if c := screenCard(t); c.Now != "—" || c.Next != "BUCS L3" || c.WarmUp != "09:00" {
			t.Errorf("before the day, the screen has the flight next: %+v", c)
		}
	})

	stage("the day", func(t *testing.T) {
		s.clock.set(onTheDay(8, 45))
		if c := screenCard(t); c.Now != "—" || c.Next != "BUCS L3" || len(c.Up) != 0 {
			t.Errorf("the morning: %+v", c)
		}
		mark := func(what string, hour, minute int) {
			t.Helper()
			s.clock.set(onTheDay(hour, minute))
			redirected(t, h, admin+"/day/flight", url.Values{"flight": {flight}, "what": {what}, "day": {"0"}})
		}
		tap := func() {
			t.Helper()
			tapUp(t, h, admin, flight, "next")
		}

		// Started ten minutes late.
		mark("start", 9, 10)
		if area := panelOne(t, h, day); area.Status != "Running 10 min late" || area.Flights[0].Started != "09:10" || area.Flights[0].UpLabel == "" || area.Flights[0].Up != order[0] {
			t.Errorf("On the day, started late, first up: %+v", area)
		}
		if c := screenCard(t); c.Now != "BUCS L3" || c.Planned || c.Late != "Running 10 min late" || !c.Behind {
			t.Errorf("the screen, started late: %+v", c)
		}
		for _, name := range []string{"Ann", "Cal"} {
			if got := myPageOf(t, name); !slices.Equal(got.Lateness, []string{"Panel 1 is running about 10 min late (as of 09:10)"}) {
				t.Errorf("%s is told Panel 1 is late: %+v", name, got.Lateness)
			}
		}

		// The marshal taps the first on; the second is scratched and skipped.
		tap()
		if f := panelOne(t, h, day).Flights[0]; f.UpLabel != "On now" || f.Up != order[0] || f.Routine != "Set" {
			t.Errorf("On now: %+v", f)
		}
		if c := screenCard(t); len(c.Up) == 0 || c.Up[0] != order[0]+" · Set" {
			t.Errorf("the screen says who's on: %+v", c.Up)
		}
		page := dayAll(t, h, day)
		redirected(t, h, admin+"/day/checkin", url.Values{"entry": {checkinEntry(t, page, order[1])}, "status": {"scratched"}, "day": {"0"}})
		if f := panelOne(t, h, day).Flights[0]; f.Gymnasts[order[1]] != "scratched" {
			t.Errorf("%s is scratched on the day page: %+v", order[1], f.Gymnasts)
		}
		if c := screenCard(t); !slices.Equal(c.Up, []string{order[0] + " · Set", order[2] + " · Set", order[3] + " · Set"}) {
			t.Errorf("who's next skips %s: %v", order[1], c.Up)
		}
		tap()
		if f := panelOne(t, h, day).Flights[0]; f.Up != order[2] {
			t.Errorf("the tap skips the scratched: %+v", f)
		}

		// Finished at 09:40, five minutes after its planned end.
		mark("finish", 9, 40)
		area := panelOne(t, h, day)
		if f := area.Flights[0]; f.Finished != "09:40" || slices.Contains(f.Buttons, "Next gymnast") || area.Status != "Running 5 min late" {
			t.Errorf("finished: %+v", area)
		}
		if c := screenCard(t); !c.Finished || c.Now != "—" || c.Late != area.Status {
			t.Errorf("the screen once it's finished: %+v", c)
		}
	})

	stage("desk, late change and payment", func(t *testing.T) {
		// A message from the desk to the flight reaches its gymnasts.
		s.clock.set(onTheDay(10, 0))
		to := redirected(t, h, admin+"/desk", url.Values{"to": {"flight"}, "flight": {flight}, "text": {"Medals at 12:00 in the main hall"}})
		if notice := noticeOf(t, to); !strings.HasPrefix(notice, "Sent to BUCS L3") {
			t.Errorf("sent: %q", notice)
		}
		for _, name := range []string{"Ann", "Dee"} {
			if got := myPageOf(t, name); !slices.Equal(got.Desk, []string{"Medals at 12:00 in the main hall"}) {
				t.Errorf("%s sees the message: %+v", name, got.Desk)
			}
		}

		// Fees: €12 an entry, €5 for a late level change.
		redirected(t, h, admin+"/fees/settings", url.Values{"fee-": {"12"}})
		redirected(t, h, admin+"/late/settings", url.Values{"late-level": {"1"}, "fee-level": {"5"}})

		// Ann asks to move to BUCS L4; the organiser accepts.
		ask := regexp.MustCompile(regexp.QuoteMeta(members["Ann"]) + `/competitions/[^/"]+/late\?discipline=`).FindString(memberCompetitionPage(t, h, members["Ann"]))
		if ask == "" {
			t.Fatal("Ann can ask for a late change")
		}
		l4 := url.Values{"level": {"BUCS L4"}, "ex1Option": {"builtin:bucs-l4-option-1"}, "ex2Option": {"builtin:bucs-l4-second"}, "ex2Skills": {voluntary}}
		redirected(t, h, unescape(ask), l4)
		late := do(t, h, http.MethodGet, admin+"/late", nil).Body.String()
		id := regexp.MustCompile(`/late/([^/"]+)/accept`).FindStringSubmatch(late)
		if id == nil {
			t.Fatalf("the organiser can accept Ann's change: %s", late)
		}
		if notice := noticeOf(t, redirected(t, h, admin+"/late/"+id[1]+"/accept", url.Values{})); !strings.HasPrefix(notice, "Accepted: Ann is in BUCS L4.") {
			t.Errorf("accepted: %q", notice)
		}
		if page := memberCompetitionPage(t, h, members["Ann"]); !strings.Contains(page, "Level change to BUCS L4: accepted (€5, on the invoice)") || !strings.Contains(page, chip(t, views.Done, "Late change")) {
			t.Error("Ann sees it accepted, with its fee")
		}
		if got := myPageOf(t, "Ann"); len(got.Events) != 1 || got.Events[0].Event != "BUCS L4" {
			t.Errorf("Ann's My competition has her in BUCS L4: %+v", got.Events)
		}
		// The published timetable doesn't change until it's published again.
		if f := panelOne(t, h, day).Flights[0]; !slices.Contains(f.Order, "Ann") {
			t.Errorf("Ann is still in the published flight: %+v", f.Order)
		}

		// UCD owes €24 for two entries and €5 for the change; pays €24.
		accounts := feeAccounts(t, do(t, h, http.MethodGet, admin+"/fees", nil).Body.String())
		ucd := accounts["UCD"]
		if ucd.Due != "€29" || ucd.Balance != "€29" || accounts["DCU"].Due != "€24" || accounts["Ivy (on their own)"].Due != "€12" {
			t.Fatalf("what each owes: %+v", accounts)
		}
		redirected(t, h, admin+"/fees/payments", url.Values{"payer": {ucd.Payer}, "amount": {"24"}, "note": {"Transfer"}})
		ucd = feeAccounts(t, do(t, h, http.MethodGet, admin+"/fees", nil).Body.String())["UCD"]
		if ucd.Paid != "€24" || ucd.Balance != "€5" || len(ucd.Payments) != 1 || !strings.HasPrefix(ucd.Payments[0], "€24 on ") {
			t.Errorf("UCD has €5 to pay: %+v", ucd)
		}
		inv := invoiceOf(t, do(t, h, http.MethodGet, ucd.Invoice, nil).Body.String())
		if inv.ToPay != "To pay €5" || !slices.Contains(inv.Lines, "Ann · BUCS L4: €12") || !slices.ContainsFunc(inv.Lines, func(l string) bool { return strings.HasSuffix(l, ": €5") }) {
			t.Errorf("UCD's invoice: %+v", inv)
		}
		// The day page and the screen carry on as they were.
		if c := screenCard(t); !c.Finished {
			t.Errorf("the screen: %+v", c)
		}
	})
}
