package web

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// The screen names who's on and the next two, with their club and routine,
// skipping the scratched and crossing into the 2nd round.
func TestScreenUp(t *testing.T) {
	c := competitions.Competition{Levels: []competitions.Level{{Ref: "builtin-level:isto-novice"}}}
	entry := func(id, name, club string) store.Entry {
		return store.Entry{ID: id, ClubName: club, Individual: club == "", Entry: competitions.Entry{Gymnast: name, Level: "ISTO Novice"}}
	}
	byID := map[string]store.Entry{"a": entry("a", "Áine Ní Bhriain", "UCD"), "b": entry("b", "Bea", "DCU"), "c": entry("c", "Cal", "")}
	f := competitions.ScheduledFlight{Flight: competitions.Flight{Entries: []string{"a", "b", "c", "gone"}}}
	scratched := map[string]bool{"b": true}
	show := func(u *views.ScreenUp) string {
		if u == nil {
			return "nothing"
		}
		out := fmt.Sprintf("%s (%s, %s)", u.On.Name, u.On.Club, u.On.Routine)
		for _, g := range u.Next {
			out += fmt.Sprintf(" / %s (%s, %s)", g.Name, g.Club, g.Routine)
		}
		return out
	}
	r := func(entry string, round int) competitions.Routine {
		return competitions.Routine{Entry: entry, Round: round}
	}
	for at, want := range map[competitions.Routine]string{
		r("a", 1):    "Áine Ní Bhriain (UCD, Set) / Cal (, Set) / Áine Ní Bhriain (UCD, Voluntary)",
		r("c", 1):    "Cal (, Set) / Áine Ní Bhriain (UCD, Voluntary) / Cal (, Voluntary)",
		r("c", 2):    "Cal (, Voluntary)",
		r("b", 1):    "nothing", // scratched since
		r("gone", 2): "nothing", // no longer entered
		r("x", 1):    "nothing", // moved out of the flight
	} {
		if got := show(screenUp(c, f, at, byID, scratched)); got != want {
			t.Errorf("at %v: %s, want %s", at, got, want)
		}
	}
}

// upOn is who the day page says is on in the flight, and their routine; ""
// when nobody is.
func upOn(page string) (name, routine string) {
	m := regexp.MustCompile(`(?s)class="comp-up mb-3">.*?has-text-weight-bold mb-0">\s*(?:<span class="comp-run-no"[^>]*>\d+</span>\s*)?<span[^>]*>([^<]+)</span>.*?<strong>([^<]+)</strong>`).FindStringSubmatch(page)
	if m == nil {
		return "", ""
	}
	return unescape(m[1]), m[2]
}

// screenUpIn is who the venue screen says is on, and who's next, each as
// "name · routine"; nil when it says nothing.
func screenUpIn(body string) []string {
	body = regexp.MustCompile(`<span class="screen-no">\d+</span>\s*`).ReplaceAllString(body, "")
	var out []string
	for _, m := range regexp.MustCompile(`class="screen-up-(?:next-)?name">([^<]+)</(?:p|span)>\s*<(?:p|span) class="screen-up-detail">([^<]+)<`).FindAllStringSubmatch(body, -1) {
		detail := strings.Fields(unescape(m[2]))
		out = append(out, unescape(m[1])+" · "+detail[len(detail)-1])
	}
	return out
}

// The marshal taps through a flight's gymnasts as they compete: both rounds,
// skipping the scratched, with Back, and the venue screen shows who's on and
// next once they do.
func TestWhoIsUpNow(t *testing.T) {
	// Entered beforehand and run at 09:00 on the day, so flights marked
	// count on the screen.
	h, clock := competitionServerAt(t, dayBefore)
	form := newCompetition()
	form["level"] = []string{"builtin-level:isto-novice"}
	admin := created(t, h, form)
	screen := madeLink(t, h, admin, "Main hall screen", "screen")
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	for _, name := range []string{"Seán O'Brien", "Bea", "Cal", "Dee"} {
		redirected(t, h, enter, url.Values{"gymnast": {name}, "level": {"ISTO Novice"}, "ex1Option": {"builtin:isto-novice-set-a"}, "ex2Option": {"builtin:isto-novice-vol"}, "ex2Skills": {voluntary}})
	}
	redirected(t, h, admin+"/timetable/plan", url.Values{})
	redirected(t, h, admin+"/timetable/publish", url.Values{"action": {"publish"}})
	clock.set(onTheDay(9, 0))

	day := admin + "/day?day=0"
	get := func() string { return dayAll(t, h, day) }
	shown := func() []string {
		return screenUpIn(dayAll(t, h, screen+"/screen?day=0"))
	}
	page := get()
	key := regexp.MustCompile(`name="flight" value="([^"]+)"`).FindStringSubmatch(page)
	if key == nil {
		t.Fatalf("no flight: %s", page)
	}
	flight := unescape(key[1])
	var order, escaped []string // the running order, and as the page has it
	for _, m := range regexp.MustCompile(`comp-checkin-name">\s*(?:<span class="comp-run-no"[^>]*>\d+</span>\s*)?<span[^>]*>([^<]+)</span>`).FindAllStringSubmatch(page, -1) {
		order, escaped = append(order, unescape(m[1])), append(escaped, m[1])
	}
	if len(order) != 4 {
		t.Fatalf("one flight of four: %v", order)
	}
	// tapForm is the Next gymnast form as the day page shows it now.
	tapForm := func(what string) url.Values {
		t.Helper()
		form := url.Values{"flight": {flight}, "what": {what}, "day": {"0"}}
		up := regexp.MustCompile(`(?s)<form[^>]*/day/up"[^>]*>.*?</form>`).FindString(get())
		for _, field := range []string{"entry", "round"} {
			if m := regexp.MustCompile(`name="` + field + `" value="([^"]*)"`).FindStringSubmatch(up); m != nil {
				form.Set(field, unescape(m[1]))
			}
		}
		return form
	}
	tap := func(link, what string) string {
		t.Helper()
		return redirected(t, h, link+"/day/up", tapForm(what))
	}
	on := func(name, routine string) {
		t.Helper()
		if gotName, gotRoutine := upOn(get()); gotName != name || gotRoutine != routine {
			t.Errorf("on now: %s (%s), want %s (%s)", gotName, gotRoutine, name, routine)
		}
	}

	// Not started: no taps, and a tap anyway changes nothing.
	// (Its taps are there but hidden, for Started tapped without signal.)
	if strings.Contains(page, `class="comp-up mb-3">`) || !strings.Contains(page, `class="comp-up mb-3" hidden>`) {
		t.Error("a flight not started has no taps showing")
	}
	if to := tap(admin, "next"); !strings.Contains(noticeOf(t, to), "isn't running") || !strings.HasSuffix(to, "#"+flightID(flight)) {
		t.Errorf("a tap before the start says so: %s", to)
	}
	redirected(t, h, admin+"/day/flight", url.Values{"flight": {flight}, "what": {"start"}, "day": {"0"}})
	page = get()
	if !strings.Contains(page, ">Next gymnast</button>") || strings.Contains(page, ">Back</button>") || !strings.Contains(page, "First up") {
		t.Errorf("started: Next gymnast, no Back yet, and who's first: %s", page)
	}
	on(order[0], "Set")
	if got := shown(); got != nil {
		t.Errorf("the screen doesn't guess before the first tap: %v", got)
	}
	if to := tap(admin, "back"); !strings.Contains(noticeOf(t, to), "Nothing to go back to") {
		t.Errorf("back before the first tap is ignored: %s", to)
	}

	// The second in the running order is scratched: both rounds skip them.
	redirected(t, h, admin+"/day/checkin", url.Values{"entry": {checkinEntry(t, page, escaped[1])}, "status": {"scratched"}, "day": {"0"}})
	if to := tap(admin, "next"); noticeOf(t, to) != "" {
		t.Errorf("a tap that moves says nothing: %s", to)
	}
	on(order[0], "Set")
	if page := get(); strings.Contains(page, "First up") || !strings.Contains(page, "On now") || strings.Contains(page, ">Back</button>") {
		t.Error("on now, and the first can't go back")
	}
	if got, want := strings.Join(shown(), " / "), order[0]+" · Set / "+order[2]+" · Set / "+order[3]+" · Set"; got != want {
		t.Errorf("the screen: %s, want %s", got, want)
	}
	if to := tap(admin, "back"); !strings.Contains(noticeOf(t, to), "Nothing to go back to") {
		t.Errorf("back at the first is ignored: %s", to)
	}
	tap(admin, "next")
	on(order[2], "Set")
	// Running numbers are places in the flight: scratching the 2nd doesn't
	// make the 3rd a 2, on the screen or the day page.
	if body := dayAll(t, h, screen+"/screen?day=0"); !strings.Contains(body, `<p class="screen-up-name"><span class="screen-no">3</span> `+escaped[2]+`</p>`) {
		t.Errorf("the screen numbers who's on by their place: %s", body)
	}
	if page := get(); !strings.Contains(page, `<span class="comp-run-no" title="Running number">3</span> <span class="">`+escaped[2]+`</span>`) &&
		!regexp.MustCompile(`comp-run-no" title="Running number">3</span>\s*<span[^>]*>`+regexp.QuoteMeta(escaped[2])+`<`).MatchString(page) {
		t.Errorf("the day page numbers them the same: %s", page)
	}
	// The same tap sent twice (its answer lost on poor wifi, say) moves once,
	// and the repeat says nothing.
	again := tapForm("next")
	redirected(t, h, admin+"/day/up", again)
	if to := redirected(t, h, admin+"/day/up", again); noticeOf(t, to) != "" {
		t.Errorf("a repeated Next says nothing: %s", to)
	}
	on(order[3], "Set")
	again = tapForm("back")
	redirected(t, h, admin+"/day/up", again)
	if to := redirected(t, h, admin+"/day/up", again); noticeOf(t, to) != "" {
		t.Errorf("a repeated Back says nothing: %s", to)
	}
	on(order[2], "Set")
	tap(admin, "next")
	on(order[3], "Set")
	// Who's next crosses into the 2nd round.
	if got, want := strings.Join(shown(), " / "), order[3]+" · Set / "+order[0]+" · Voluntary / "+order[2]+" · Voluntary"; got != want {
		t.Errorf("the screen: %s, want %s", got, want)
	}
	tap(admin, "next")
	on(order[0], "Voluntary")
	tap(admin, "next")
	on(order[2], "Voluntary")
	tap(admin, "next")
	on(order[3], "Voluntary")
	page = get()
	if !strings.Contains(page, "last routine") || !strings.Contains(page, ">Finished</button>") {
		t.Errorf("the last routine says to finish the flight: %s", page)
	}
	// Finished takes Next gymnast's place, once, with Back beside it.
	if up := page[strings.Index(page, `class="comp-up mb-3"`):]; strings.Contains(page, ">Next gymnast</button>") ||
		strings.Count(page, ">Finished</button>") != 1 || !strings.Contains(up[:strings.Index(up, "last routine")], ">Finished</button>") ||
		!strings.Contains(page, ">Back</button>") || !strings.Contains(page, `class="button is-success is-medium"`) {
		t.Errorf("the last routine: Finished where Next gymnast was, and Back: %s", page)
	}
	if got, want := strings.Join(shown(), " / "), order[3]+" · Voluntary"; got != want {
		t.Errorf("nobody next after the last: %s", got)
	}
	if to := tap(admin, "next"); !strings.Contains(noticeOf(t, to), "That was the last routine") {
		t.Errorf("next after the last says the flight is done: %s", to)
	}
	on(order[3], "Voluntary")
	if page := get(); !strings.Contains(page, ">Finished</button>") {
		t.Error("and doesn't finish the flight itself")
	}
	tap(admin, "back")
	on(order[2], "Voluntary")

	// Chairs and the timetable link can tap; the screen and cards links can't.
	tap(madeLink(t, h, admin, "Chris", "chair"), "back")
	on(order[0], "Voluntary")
	tap(madeLink(t, h, admin, "Tina", "timetable"), "next")
	on(order[2], "Voluntary")
	for _, link := range []string{screen, madeLink(t, h, admin, "Cards", "cards")} {
		if rec := do(t, h, http.MethodPost, link+"/day/up", url.Values{"flight": {flight}, "what": {"next"}, "day": {"0"}}); rec.Code != http.StatusForbidden {
			t.Errorf("%s can't tap: %d", link, rec.Code)
		}
	}
	if rec := do(t, h, http.MethodPost, admin+"/day/up", url.Values{"flight": {flight}, "what": {"skip"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("not a tap: %d", rec.Code)
	}
	if strings.Contains(do(t, h, http.MethodGet, admin+"/history", nil).Body.String(), "/day/up") {
		t.Error("taps aren't in the history")
	}

	// Only this flight's area shows who's on.
	if body := dayAll(t, h, screen+"/screen?day=0"); strings.Count(body, `class="screen-up"`) != 1 {
		t.Errorf("one area shows who's on: %s", body)
	}

	// Undoing the start clears the taps; a finished flight has none.
	if page := get(); !strings.Contains(page, `value="undo-start">Undo</button>`) {
		t.Error("Undo takes back the start of a running flight")
	}
	redirected(t, h, admin+"/day/flight", url.Values{"flight": {flight}, "what": {"undo-start"}, "day": {"0"}})
	redirected(t, h, admin+"/day/flight", url.Values{"flight": {flight}, "what": {"start"}, "day": {"0"}})
	if page := get(); !strings.Contains(page, "First up") {
		t.Error("started again from the first")
	}
	if got := shown(); got != nil {
		t.Errorf("and the screen is back to saying nothing: %v", got)
	}
	tap(admin, "next")
	redirected(t, h, admin+"/day/flight", url.Values{"flight": {flight}, "what": {"finish"}, "day": {"0"}})
	if page := get(); strings.Contains(page, "Next gymnast") {
		t.Error("a finished flight has no taps")
	}
	if got := shown(); got != nil {
		t.Errorf("nor anything on the screen: %v", got)
	}
	// Undoing the finish keeps the place, even sent twice.
	if page := get(); !strings.Contains(page, `value="undo-finish">Undo</button>`) {
		t.Error("Undo takes back the finish of a finished flight")
	}
	redirected(t, h, admin+"/day/flight", url.Values{"flight": {flight}, "what": {"undo-finish"}, "day": {"0"}})
	if to := redirected(t, h, admin+"/day/flight", url.Values{"flight": {flight}, "what": {"undo-finish"}, "day": {"0"}}); noticeOf(t, to) != "" {
		t.Errorf("a repeated Undo says nothing: %s", to)
	}
	on(order[0], "Set")
}
