package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// twoFlights are timings that put the two BUCS L3 gymnasts of scratchSetup in
// a flight each (at most one trampoline gymnast to a flight), so the event is
// a run of two flights on Panel 1.
var twoFlights = url.Values{"per-trampoline": {"5"}, "between-trampoline": {"10"}, "max-trampoline": {"1"}, "per-tumbling": {"2"}, "between-tumbling": {"10"}, "max-tumbling": {"15"}, "rest": {"20"}, "restMust": {"0"}, "separate": {"all"}}

// panelSeat finds an official's person key and the key of the flight in the
// forms of the panel check-in under one flight of the day page.
func panelSeat(t *testing.T, page, flight, name string) (person, key string) {
	t.Helper()
	m := regexp.MustCompile(`(?s)<strong>` + regexp.QuoteMeta(flight) + `</strong>.*?comp-panel-name"><span>` + regexp.QuoteMeta(name) + `</span>.*?name="person" value="([^"]+)">\s*<input type="hidden" name="flight" value="([^"]+)"`).FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("no panel row for %s in %s: %s", name, flight, page)
	}
	return html2text(m[1]), html2text(m[2])
}

// notifyForm says whether the day page offers to Notify a person in a flight,
// as an official or a gymnast.
func notifyForm(page, person, flightKey, as string) bool {
	return strings.Contains(page, `name="person" value="`+person+`"> <input type="hidden" name="flight" value="`+flightKey+`"> <input type="hidden" name="as" value="`+as+`"`)
}

// Marking an official here or missing on one flight of a run does it for the
// event's other flights where they sit, and is ticked on the chair's sheets.
func TestPanelCheckinRun(t *testing.T) {
	h := competitionServer(t)
	admin, tt, _, _ := scratchSetupOn(t, h, twoFlights)
	day := admin + "/day?day=0"
	get := func(path string) string { return do(t, h, http.MethodGet, path, nil).Body.String() }
	page := get(day)
	const first, second = "BUCS L3 · flight 1 of 2", "BUCS L3 · flight 2 of 2"
	// BUCS L3 is a run of two flights on Panel 1, with Mary, Tom and Dara
	// sitting on both of them; tumbling is a flight of its own.
	if n := strings.Count(page, "Panel · 0 of 3 here</summary>"); n != 2 {
		t.Fatalf("both flights of the run have a panel of three, none here: %d %s", n, page)
	}
	if !strings.Contains(page, "Panel · 0 of 1 here</summary>") {
		t.Error("so does tumbling's, of one")
	}
	if strings.Contains(page, ">Clear</button>") {
		t.Error("nothing to clear yet")
	}
	dara, flight1 := panelSeat(t, page, first, "Dara")
	_, flight2 := panelSeat(t, page, second, "Dara")
	mark := func(link, person, flight, status string) string {
		t.Helper()
		return redirected(t, h, link+"/day/officials", url.Values{"person": {person}, "flight": {flight}, "status": {status}, "day": {"0"}})
	}

	// Here on the first flight is here on the second too.
	if to := mark(admin, dara, flight1, "here"); !strings.HasPrefix(to, day) || strings.Contains(to, "notice=") {
		t.Errorf("back to the day: %s", to)
	}
	page = get(day)
	if n := strings.Count(page, "Panel · 1 of 3 here</summary>"); n != 2 {
		t.Errorf("Dara is here for the run: %d %s", n, page)
	}
	if !strings.Contains(page, "Panel · 0 of 1 here</summary>") {
		t.Error("tumbling's panel isn't touched")
	}
	if !strings.Contains(page, `<span class="tag is-success is-light">Here</span>`) || !strings.Contains(page, ">Clear</button>") {
		t.Errorf("her state, and Clear: %s", page)
	}
	if notifyForm(page, dara, flight1, "official") || notifyForm(page, dara, flight2, "official") {
		t.Error("no Notify for an official who is here")
	}
	if to := mark(admin, dara, flight2, "here"); !strings.Contains(to, "notice=") {
		t.Errorf("already here on every flight of the run: %s", to)
	}

	// The chair of judges sheets tick her.
	print := get(admin + "/timetable/print?sheet=judges")
	if !strings.Contains(print, "Execution judge: Dara ✓") || strings.Contains(print, "Tom ✓") || strings.Contains(print, "Mary ✓") {
		t.Errorf("only Dara is ticked: %s", print)
	}
	if print := get(admin + "/timetable/print?sheet=marshal"); strings.Contains(print, "✓") {
		t.Error("the marshal sheets don't tick anyone")
	}

	// Missing, from the second flight, is missing on both; the organiser can
	// see what if she leaves, and she can be notified again.
	mark(admin, dara, flight2, "missing")
	page = get(day)
	if n := strings.Count(page, "Panel · 0 of 3 here, 1 missing</summary>"); n != 2 {
		t.Errorf("Dara is missing for the run: %d %s", n, page)
	}
	leave := regexp.MustCompile(`href="([^"]*timetable/leave\?[^"]*)">What if they leave`).FindStringSubmatch(page)
	if leave == nil || strings.Count(page, "What if they leave") != 2 {
		t.Fatalf("a what-if link under each of her seats: %s", page)
	}
	if to := html2text(leave[1]); !strings.HasPrefix(to, tt+"/leave?") || !strings.Contains(to, "person=i%3Adara") || !strings.Contains(to, "day=0") {
		t.Errorf("the link is for Dara on that day: %s", to)
	}
	if !notifyForm(page, dara, flight1, "official") || !notifyForm(page, dara, flight2, "official") {
		t.Error("a missing official can be notified")
	}
	if print := get(admin + "/timetable/print?sheet=judges"); strings.Contains(print, "Dara ✓") {
		t.Error("missing isn't ticked")
	}

	// A chair sees the state but isn't sent to the what-if page.
	chair := madeLink(t, h, admin, "Chris", "chair")
	if page := get(chair + "/day?day=0"); !strings.Contains(page, "Panel · 0 of 3 here, 1 missing</summary>") || strings.Contains(page, "What if they leave") {
		t.Errorf("a chair sees Dara missing, without the link: %s", page)
	}

	// Clear takes it back on the whole run.
	mark(admin, dara, flight1, "")
	page = get(day)
	if n := strings.Count(page, "Panel · 0 of 3 here</summary>"); n != 2 || strings.Contains(page, "What if they leave") || strings.Contains(page, ">Clear</button>") {
		t.Errorf("cleared on both flights: %s", page)
	}
	if to := mark(admin, dara, flight1, ""); !strings.Contains(to, "notice=") {
		t.Errorf("nothing to clear: %s", to)
	}

	// Not a status, not a flight, not someone with a seat on it.
	for _, form := range []url.Values{
		{"person": {dara}, "flight": {flight1}, "status": {"late"}},
		{"person": {dara}, "flight": {"nonsense"}, "status": {"here"}},
		{"person": {"i:finn"}, "flight": {flight1}, "status": {"here"}},
		{"person": {""}, "flight": {flight1}, "status": {"here"}},
	} {
		if rec := do(t, h, http.MethodPost, admin+"/day/officials", form); rec.Code != http.StatusBadRequest {
			t.Errorf("%v: %d", form, rec.Code)
		}
	}

	// The history says what was done, in words, once each; a chair can do it.
	mark(chair, dara, flight1, "here")
	history := get(admin + "/history")
	for _, want := range []string{"Checked in official: Dara (BUCS L3, Panel 1)", "Marked an official missing: Dara (BUCS L3, Panel 1)", "Cleared an official&#39;s check-in: Dara (BUCS L3, Panel 1)", "Chris"} {
		if !strings.Contains(history, want) {
			t.Errorf("the history has %q: %s", want, history)
		}
	}
	if n := strings.Count(history, "Cleared an official&#39;s check-in"); n != 1 {
		t.Errorf("the clear that did nothing isn't recorded: %d", n)
	}
	if n := strings.Count(history, "Checked in official: Dara"); n != 2 {
		t.Errorf("the check-in that did nothing isn't recorded: %d", n)
	}
}

// A scratched gymnast who is checked in for their panel has turned up, so
// their warning goes; missing doesn't count.
func TestPanelCheckinClearsScratchWarning(t *testing.T) {
	h, admin, _, _ := scratchSetup(t)
	day := admin + "/day?day=0"
	get := func(path string) string { return do(t, h, http.MethodGet, path, nil).Body.String() }
	page := get(day)
	redirected(t, h, admin+"/day/checkin", url.Values{"entry": {checkinEntry(t, page, "Dara")}, "status": {"scratched"}, "day": {"0"}})
	if page := get(day); !strings.Contains(page, "Dara is scratched</strong>") {
		t.Fatalf("Dara is scratched but officiates: %s", page)
	}
	page = get(day)
	dara, flight := panelSeat(t, page, "BUCS L3", "Dara")
	mark := func(status string) {
		t.Helper()
		redirected(t, h, admin+"/day/officials", url.Values{"person": {dara}, "flight": {flight}, "status": {status}, "day": {"0"}})
	}
	mark("missing")
	if page := get(day); !strings.Contains(page, "Dara is scratched</strong>") {
		t.Error("missing isn't here")
	}
	mark("here")
	if page := get(day); strings.Contains(page, "is scratched") {
		t.Errorf("she's here for her panel: %s", page)
	}
	mark("")
	if page := get(day); !strings.Contains(page, "Dara is scratched</strong>") {
		t.Error("the warning is back once that's taken back")
	}
}

// Chairs and timetable links can check officials in and notify; a cards link
// can't.
func TestPanelCheckinAccess(t *testing.T) {
	h, admin, _, _ := scratchSetup(t)
	page := do(t, h, http.MethodGet, admin+"/day?day=0", nil).Body.String()
	dara, flight := panelSeat(t, page, "BUCS L3", "Dara")
	officials := url.Values{"person": {dara}, "flight": {flight}, "status": {"here"}, "day": {"0"}}
	notify := url.Values{"person": {dara}, "flight": {flight}, "as": {"official"}, "them": {"1"}, "day": {"0"}}

	cards := madeLink(t, h, admin, "Carl", "cards")
	for path, form := range map[string]url.Values{"/day/officials": officials, "/day/notify": notify} {
		if rec := do(t, h, http.MethodPost, cards+path, form); rec.Code != http.StatusForbidden {
			t.Errorf("a cards link can't use %s: %d", path, rec.Code)
		}
	}
	if page := do(t, h, http.MethodGet, admin+"/day?day=0", nil).Body.String(); strings.Contains(page, "Panel · 1 of") {
		t.Error("nothing was checked in")
	}

	chair := madeLink(t, h, admin, "Chris", "chair")
	if to := redirected(t, h, chair+"/day/officials", officials); !strings.HasPrefix(to, chair+"/day?day=0") {
		t.Errorf("a chair can check an official in: %s", to)
	}
	if to := redirected(t, h, chair+"/day/notify", notify); !strings.HasPrefix(to, chair+"/day?day=0") || !strings.Contains(to, "notice=Sent+to+Dara") {
		t.Errorf("a chair can notify: %s", to)
	}
	timetable := madeLink(t, h, admin, "Tina", "timetable")
	officials.Set("status", "missing")
	if to := redirected(t, h, timetable+"/day/officials", officials); !strings.HasPrefix(to, timetable+"/day?day=0") {
		t.Errorf("a timetable link can too: %s", to)
	}
	if rec := do(t, h, http.MethodPost, timetable+"/day/notify", notify); rec.Code != http.StatusSeeOther {
		t.Errorf("and notify: %d", rec.Code)
	}
}

// Notify tells an official where to be, at once, with their own words added;
// it needs someone to tell, and the history says who was told.
func TestNotifyAnOfficial(t *testing.T) {
	h, _, mail := notifyServer(t)
	admin, _, _, dara := scratchSetupOn(t, h, nil)
	subscribeByEmail(t, h, mail, dara+"/notify", "dara@example.com")
	day := admin + "/day?day=0"
	page := do(t, h, http.MethodGet, day, nil).Body.String()
	person, flight := panelSeat(t, page, "BUCS L3", "Dara")
	if !notifyForm(page, person, flight, "official") || !strings.Contains(page, `name="them" value="1" checked>`) || !strings.Contains(page, ">Send</button>") {
		t.Fatalf("an official who isn't here can be notified: %s", page)
	}
	if strings.Contains(page, `name="club"`) {
		t.Error("no one on this panel belongs to a club")
	}

	// Nobody ticked: nothing is sent.
	form := url.Values{"person": {person}, "flight": {flight}, "as": {"official"}, "day": {"0"}}
	if to := redirected(t, h, admin+"/day/notify", form); !strings.Contains(to, "Nothing+was+sent") || len(mail.sent) != 0 {
		t.Errorf("no one to tell: %s %+v", to, mail.sent)
	}
	// Their club, but they have none: told themselves if they are ticked, else nothing.
	form.Set("club", "1")
	if to := redirected(t, h, admin+"/day/notify", form); !strings.Contains(to, "doesn%27t+belong+to+a+club") || len(mail.sent) != 0 {
		t.Errorf("Dara has no club: %s %+v", to, mail.sent)
	}
	form.Del("club")

	form.Set("them", "1")
	form.Set("note", "  Bring your badge.  ")
	to := redirected(t, h, admin+"/day/notify", form)
	if !strings.HasPrefix(to, day+"&notice=Sent+to+Dara") || !strings.Contains(to, "1+by+email") {
		t.Errorf("the notice: %s", to)
	}
	if len(mail.sent) != 1 || mail.sent[0].to != "dara@example.com" ||
		!strings.HasPrefix(mail.sent[0].body, "Dara should be at Panel 1 now to officiate BUCS L3 (Execution judge). Bring your badge.\n") {
		t.Fatalf("Dara is emailed at once: %+v", mail.sent)
	}
	// She sees it on her page, and the desk lists it.
	if body := do(t, h, http.MethodGet, dara, nil).Body.String(); !strings.Contains(body, deskBox) || !strings.Contains(body, "should be at Panel 1 now to officiate BUCS L3") {
		t.Errorf("her page shows it: %s", body)
	}
	if desk := do(t, h, http.MethodGet, admin+"/desk", nil).Body.String(); !strings.Contains(desk, "to Dara") || !strings.Contains(desk, "told 0 by push, 1 by email") {
		t.Errorf("the desk lists it: %s", desk)
	}
	if history := do(t, h, http.MethodGet, admin+"/history", nil).Body.String(); !strings.Contains(history, "Notified Dara: should be at Panel 1") {
		t.Errorf("the history: %s", history)
	}
	// Not a person on that flight, or not a way to be told.
	for _, f := range []url.Values{
		{"person": {"i:finn"}, "flight": {flight}, "as": {"official"}, "them": {"1"}},
		{"person": {person}, "flight": {"nonsense"}, "as": {"official"}, "them": {"1"}},
		{"person": {person}, "flight": {flight}, "as": {"nonsense"}, "them": {"1"}},
		{"person": {"i:nobody"}, "flight": {flight}, "as": {"gymnast"}, "them": {"1"}},
	} {
		if rec := do(t, h, http.MethodPost, admin+"/day/notify", f); rec.Code != http.StatusBadRequest {
			t.Errorf("%v: %d", f, rec.Code)
		}
	}
}

// Notify a club member's club and only its comp sec and coaches hear, not the
// other members; and it shows on the club's and its coaches' pages, not the
// members'.
func TestNotifyAClub(t *testing.T) {
	h, _, mail := notifyServer(t)
	admin := created(t, h, newCompetition())
	club, members := aClubWithMembers(t, h, admin, "Ann", "Bea")
	bob := pathIn(t, do(t, h, http.MethodPost, club+"/coaches", url.Values{"name": {"Bob"}}).Body.String(), "/clubs/coach/")
	subscribeByEmail(t, h, mail, notifyPageOf(t, h, club, club), "ucd@example.com")
	subscribeByEmail(t, h, mail, notifyPageOf(t, h, bob, bob), "bob@example.com")
	subscribeByEmail(t, h, mail, notifyPageOf(t, h, members["Ann"], members["Ann"]), "ann@example.com")
	subscribeByEmail(t, h, mail, notifyPageOf(t, h, members["Bea"], members["Bea"]), "bea@example.com")
	redirected(t, h, admin+"/timetable/plan", url.Values{})
	redirected(t, h, admin+"/timetable/publish", url.Values{"action": {"publish"}})

	day := admin + "/day?day=0"
	page := do(t, h, http.MethodGet, day, nil).Body.String()
	m := regexp.MustCompile(`(?s)<span class="">Ann</span>.*?name="person" value="([^"]+)">\s*<input type="hidden" name="flight" value="([^"]+)">`).FindStringSubmatch(page)
	if m == nil || !strings.Contains(page, `name="club" value="1"> Their club`) {
		t.Fatalf("Ann can be notified, and so can her club: %s", page)
	}
	ann, flight := html2text(m[1]), html2text(m[2])
	text := "Ann should be at Panel 1 now to compete in BUCS L3, warm-up "

	// Their club only: the comp sec and the coach hear, no member.
	form := url.Values{"person": {ann}, "flight": {flight}, "as": {"gymnast"}, "club": {"1"}, "day": {"0"}}
	to := redirected(t, h, admin+"/day/notify", form)
	if notice, _ := url.QueryUnescape(to); !strings.Contains(notice, "notice=Sent to Ann's club. Told 0 by push and 2 by email") {
		t.Errorf("the notice: %s", notice)
	}
	got := map[string]sentEmail{}
	for _, e := range mail.sent {
		got[e.to] = e
	}
	if len(mail.sent) != 2 || !strings.HasPrefix(got["ucd@example.com"].body, text) || !strings.HasPrefix(got["bob@example.com"].body, text) {
		t.Fatalf("the comp sec and the coach are emailed, and no one else: %+v", mail.sent)
	}
	if !strings.Contains(got["ucd@example.com"].body, "https://tariff.example"+club) || !strings.Contains(got["bob@example.com"].body, "https://tariff.example"+bob) {
		t.Errorf("each is linked to their page: %+v", mail.sent)
	}
	for _, p := range []string{club, bob} {
		if body := do(t, h, http.MethodGet, p, nil).Body.String(); !strings.Contains(body, deskBox) || !strings.Contains(body, text) {
			t.Errorf("%s shows it: %s", p, body)
		}
	}
	for name, mine := range members {
		day := regexp.MustCompile(`/competitions/[^/"]+/day`).FindString(do(t, h, http.MethodGet, mine, nil).Body.String())
		for _, p := range []string{mine, mine + day} {
			if body := do(t, h, http.MethodGet, p, nil).Body.String(); strings.Contains(body, text) {
				t.Errorf("%s's page %s doesn't show what went to the club's staff: %s", name, p, body)
			}
		}
	}

	// Them and their club: Ann hears too (and the comp sec, once), Bea still doesn't.
	mail.sent = nil
	form.Set("them", "1")
	form.Set("note", "Hall 2.")
	redirected(t, h, admin+"/day/notify", form)
	got = map[string]sentEmail{}
	for _, e := range mail.sent {
		got[e.to] = e
	}
	if len(mail.sent) != 3 || !strings.HasPrefix(got["ann@example.com"].body, text) || !strings.Contains(got["ann@example.com"].body, "Hall 2.\n") || got["bea@example.com"].body != "" {
		t.Errorf("Ann, the comp sec and the coach, once each: %+v", mail.sent)
	}
	if body := do(t, h, http.MethodGet, members["Ann"], nil).Body.String(); !strings.Contains(body, text) {
		t.Error("Ann is shown what went to her")
	}
	if body := do(t, h, http.MethodGet, members["Bea"], nil).Body.String(); strings.Contains(body, text) {
		t.Error("Bea isn't")
	}

	// A gymnast who is here has nothing to be told.
	redirected(t, h, admin+"/day/checkin", url.Values{"entry": {checkinEntry(t, page, "Ann")}, "status": {"here"}, "day": {"0"}})
	bea := regexp.MustCompile(`(?s)<span class="">Bea</span>.*?name="person" value="([^"]+)">`).FindStringSubmatch(page)[1]
	if page := do(t, h, http.MethodGet, day, nil).Body.String(); notifyForm(page, ann, flight, "gymnast") || !notifyForm(page, html2text(bea), flight, "gymnast") {
		t.Errorf("no Notify for Ann once she's here, still one for Bea: %s", page)
	}

	history := do(t, h, http.MethodGet, admin+"/history", nil).Body.String()
	for _, want := range []string{"Notified Ann&#39;s club: should be at Panel 1", "Notified Ann (and their club): should be at Panel 1"} {
		if !strings.Contains(history, want) {
			t.Errorf("the history has %q: %s", want, history)
		}
	}
}
