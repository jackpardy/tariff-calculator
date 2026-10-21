package demo

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// The sandbox's seed (ADR 0016): the demo's clubs, with ISTO's 2025 levels
// and events, in two competitions, one open for entries and one on the
// day, its morning under way. It goes through the pages as the demo does,
// at the times things would have happened, and returns its links by role
// for the sandbox's start page.

// Clocked is the site at a time: its handler with the clock reading now.
// The sandbox seeds through it, moving the clock as it goes, so what it
// makes is stamped as if done then.
type Clocked func(now func() time.Time) http.Handler

// Copy is what a sandbox seed made: its links by role, and the admin link
// of every competition and club in it, so it can be deleted.
type Copy struct {
	Links        Links
	Competitions []string
	Clubs        []string
}

// Links are a sandbox's links by role, each a path on the site.
type Links struct {
	Open        Competition `json:"open"`        // the competition open for entries
	Day         Competition `json:"day"`         // the competition on the day
	CardChecker string      `json:"cardChecker"` // a helper checking cards, while entries are open
	Chair       string      `json:"chair"`       // a chair of judges, on the day
	CompSec     Person      `json:"compSec"`     // the club admin: the first club's link
	Coach       Person      `json:"coach"`
	Member      Person      `json:"member"`
	Own         Person      `json:"own"`        // a gymnast entering on their own: their own page
	OwnSignoff  string      `json:"ownSignoff"` // their coach's sign-off link, for an entry not signed off yet
	Screen      string      `json:"screen"`     // the venue screen, on the day
	ClubRoles   []ClubRole  `json:"clubRoles"`  // links the club admin made with a role (ADR 0015)
}

// ClubRole is a link a club admin made for someone, with a role.
type ClubRole struct {
	Role string `json:"role"`
	Person
}

// Competition is one of the sandbox's competitions.
type Competition struct {
	Name      string `json:"name"`
	ID        string `json:"id"`
	Organiser string `json:"organiser"` // the admin link
	Enter     string `json:"enter"`     // to enter on one's own
}

// Person is someone in the sandbox, with their link.
type Person struct {
	Name string `json:"name"`
	Club string `json:"club,omitempty"`
	Link string `json:"link"`
}

// dublin is the time zone the sandbox's days are in, as the site's are.
var dublin = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Dublin")
	if err != nil {
		panic(err)
	}
	return loc
}()

// The sandbox's names for its competitions.
const (
	SandboxOpen = "ISTO (sandbox): entries open"
	SandboxDay  = "ISTO (sandbox): on the day"
)

// istoLevels are ISTO's trampoline levels the seed's gymnasts enter,
// easiest first, and how many of each hundred enter each.
var istoLevels = []levelShare{{"ISTO Novice", 30}, {"ISTO Intermediate", 25}, {"ISTO Intervanced", 18}, {"ISTO Advanced", 14}, {"ISTO Elite", 9}, {"ISTO Elite-Pro", 4}}

// istoID is a level's built-in id: "ISTO Disability Level 1" is
// "isto-disability-l1".
func istoID(level string) string {
	id := strings.ToLower(strings.TrimPrefix(level, "ISTO "))
	return "isto-" + strings.ReplaceAll(strings.ReplaceAll(id, "disability level ", "disability-l"), " ", "-")
}

// istoElite says whether a level is two voluntaries under the same rules.
func istoElite(level string) bool { return level == "ISTO Elite" || level == "ISTO Elite-Pro" }

// isto is an ISTO competition for the sandbox: on date, closing on
// deadline, open for entries or past its deadline and published.
func isto(name string, date, deadline time.Time, open bool) *variant {
	return &variant{
		name: name, date: date.Format("2006-01-02"), deadline: deadline.Format("2006-01-02"),
		levels:  istoLevels,
		offered: []string{"ISTO Disability Level 1", "ISTO Disability Level 2", "ISTO Disability Level 3", "ISTO Disability Level 4"},
		chairs:  []string{"ISTO Advanced", "ISTO Elite", "ISTO Elite-Pro"},
		ref:     func(level string) string { return "builtin-level:" + istoID(level) },
		option: func(level string, exercise, n int) string {
			id := "builtin:" + istoID(level)
			switch {
			case istoElite(level):
				return id
			case exercise == 1:
				return id + "-vol"
			case strings.Contains(level, "Disability"):
				return id + "-set"
			}
			return id + "-set-" + []string{"a", "b"}[max(n, 1)-1]
		},
		voluntary: func(level string, exercise int) bool { return exercise == 1 || istoElite(level) },
		// Synchro's three levels pair the trampoline levels.
		synchro:      "ISTO Novice + ISTO Intermediate\nISTO Intervanced + ISTO Advanced\nISTO Elite + ISTO Elite-Pro",
		tumbling:     []string{"Level 1", "Level 2", "Level 3", "Level 4"},
		dmt:          []string{"Level 1", "Level 2", "Level 3", "Level 4", "Level 5", "Level 6"},
		tumblingPick: []string{"Level 1", "Level 1", "Level 2", "Level 2", "Level 3", "Level 4"},
		dmtPick:      []string{"Level 1", "Level 1", "Level 2", "Level 2", "Level 3", "Level 4", "Level 5", "Level 6"},
		ownTumbling:  "Level 2",
		// Its days move, so they're named by number, not the day of the week.
		days: []string{"Day 1", "Day 2", "Day 3"},
		rules: []url.Values{
			{"kind": {"day"}, "must": {"1"}, "event": {"Synchro ISTO Novice/Intermediate"}, "day": {"2"}},
			{"kind": {"day"}, "must": {"1"}, "event": {"Synchro ISTO Intervanced/Advanced"}, "day": {"2"}},
			{"kind": {"day"}, "must": {"1"}, "event": {"Synchro ISTO Elite/Elite-Pro"}, "day": {"2"}},
			{"kind": {"day"}, "must": {"0"}, "event": {"ISTO Elite-Pro"}, "day": {"2"}},
			{"kind": {"before"}, "must": {"0"}, "event": {"ISTO Novice"}, "event2": {"ISTO Intermediate"}},
		},
		chairFor: "ISTO Elite",
		limits:   map[string]string{"ISTO Elite-Pro": "12", "DMT Level 1": "15"},
		fees:     "Bank transfer to ISTO (sandbox), IBAN IE00 SAND 0000 0000 0000 00, with your club's name as the reference. It's made up: don't pay anything.",
		open:     open,
	}
}

// Sandbox seeds a sandbox copy at size, as of now, through the site h, and
// returns its links. The competition on the day was set up and entered over
// the weeks before, and its morning has been tapped through (Morning). If
// it fails partway, what it made so far is returned with the error, to be
// deleted.
func Sandbox(h Clocked, size Size, now time.Time) (Copy, error) {
	at := now
	s := &seeder{h: h(func() time.Time { return at }), rng: rand.New(rand.NewPCG(uint64(now.UnixNano()), 99)), size: size}
	var cp Copy
	sofar := func() Copy {
		cp.Competitions = slices.Clone(s.admins)
		cp.Clubs = nil
		for _, cl := range clubs {
			if run, ok := s.clubs[cl.name]; ok && run.admin != "" {
				cp.Clubs = append(cp.Clubs, run.admin)
			}
		}
		return cp
	}
	y, m, d := now.In(dublin).Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, dublin)

	// The competition on the day: set up, entered and published four weeks
	// ago, closing a fortnight before today.
	at = now.AddDate(0, 0, -28)
	s.v = isto(SandboxDay, today, today.AddDate(0, 0, -14), false)
	if err := s.run(io.Discard); err != nil {
		return sofar(), fmt.Errorf("the competition on the day: %w", err)
	}
	day := s.made

	// The competition open for entries, closing in three weeks.
	at = now
	s.v = isto(SandboxOpen, today.AddDate(0, 0, 35), today.AddDate(0, 0, 21), true)
	if err := s.run(io.Discard); err != nil {
		return sofar(), fmt.Errorf("the competition open for entries: %w", err)
	}
	open := s.made

	// Routines kept on members' pages and the skills they've marked, over
	// the last fortnight.
	at = now.AddDate(0, 0, -14)
	s.keptRoutines(open.runs)
	if s.err != nil {
		return sofar(), fmt.Errorf("routines and skills: %w", s.err)
	}

	// Messages to the organisers, at every stage, sent lately.
	at = now.Add(-3 * time.Hour)
	s.messages(day)
	s.messages(open)
	if s.err != nil {
		return sofar(), fmt.Errorf("messages to the organiser: %w", s.err)
	}

	first := open.runs[0]
	coach := first.coaches[clubs[0].coaches[0]]
	member := first.members[0]
	cp.Links = Links{
		Open:        Competition{Name: open.name, ID: open.id, Organiser: open.admin, Enter: open.enter},
		Day:         Competition{Name: day.name, ID: day.id, Organiser: day.admin, Enter: day.enter},
		CardChecker: open.judges, Chair: day.chairs, Screen: day.screen,
		CompSec:    Person{Name: "The club admin", Club: first.name, Link: first.admin},
		Coach:      Person{Name: clubs[0].coaches[0], Club: first.name, Link: coach},
		Member:     Person{Name: member.name, Club: first.name, Link: member.link},
		Own:        Person{Name: open.ownName, Link: open.ownPage},
		OwnSignoff: open.ownSignoff,
	}
	for _, r := range first.roles {
		cp.Links.ClubRoles = append(cp.Links.ClubRoles, ClubRole{Role: r.role, Person: Person{Name: r.person, Club: first.name, Link: r.link}})
	}
	if err := Morning(h, day.admin, now); err != nil {
		return sofar(), fmt.Errorf("the morning on the day: %w", err)
	}
	return sofar(), nil
}

// messages sends the organiser of a competition the seed made a message at
// every stage: just sent, received, replied to and answered (waiting for
// them again), resolved with a reply and a note, and resolved then opened
// again by the sender. Each is from someone else: members, a club admin, a
// club's team manager, a coach and a gymnast on their own.
func (s *seeder) messages(m made) {
	if s.err != nil {
		return
	}
	ids := func() map[string]bool {
		out := map[string]bool{}
		for _, id := range regexp.MustCompile(`id="concern-([^"]+)"`).FindAllStringSubmatch(s.get(m.admin), -1) {
			out[id[1]] = true
		}
		return out
	}
	send := func(from, text string) string {
		before := ids()
		s.post(from+"/organiser", url.Values{"text": {text}})
		for id := range ids() {
			if !before[id] {
				return id
			}
		}
		s.fail("no message sent from %s", from)
		return ""
	}
	run := func(i int) clubRun { return m.runs[i%len(m.runs)] }
	in := func(link string) string { return link + "/competitions/" + m.id }
	concern := func(id string) string { return m.admin + "/concerns/" + id }
	names := make([]string, 0, len(run(3).coaches))
	for name := range run(3).coaches {
		names = append(names, name)
	}
	slices.Sort(names)

	// Just sent: waiting for the organiser.
	send(in(run(1).members[0].link), "Is there anywhere to leave bags during the warm-up?")
	// From a club's team manager, a link with a role (ADR 0015): it says
	// who, and the club's other links that talk to the organiser see it.
	for _, r := range run(0).roles {
		if r.role == "Team manager" {
			send(in(r.link), "Our coach is driving the minibus: is there parking for one at the venue?")
		}
	}
	// Received.
	if id := send(in(run(2).admin), "Two of our gymnasts may have placement exams that week. Could we swap them for others after the deadline?"); id != "" {
		s.post(concern(id)+"/received", url.Values{})
	}
	// Replied to, and answered: waiting for the organiser again.
	from := in(run(3).coaches[names[0]])
	if id := send(from, "Could my Novice and Intermediate gymnasts' flights be kept apart? I coach both."); id != "" {
		s.post(concern(id)+"/reply", url.Values{"reply": {"We'll try. Which gymnasts?"}})
		s.post(from+"/organiser/"+id, url.Values{"text": {"Mostly the women's flights, at both levels."}})
	}
	// Resolved, with a reply and a note.
	if id := send(m.owns[1%len(m.owns)], "Can I pay my entry fee on the day?"); id != "" {
		s.post(concern(id)+"/resolve", url.Values{"reply": {"Yes: at the desk, by card or cash."}, "note": {"Told the desk to expect them."}})
	}
	// Resolved, then opened again by the sender.
	from = in(run(4).members[1].link)
	if id := send(from, "My card shows the wrong club."); id != "" {
		s.post(concern(id)+"/resolve", url.Values{"note": {"Fixed: it was under the wrong club."}})
		s.post(from+"/organiser/"+id, url.Values{"text": {"Thanks, but my page still shows the old club."}})
	}
}

// keptRoutines keeps routines on about two members' pages in three (ADR
// 0011): the voluntary they'd compete, and an Elite's other one. About half
// of them mark the skills in it Can do, and one or two from the next level
// up Learning, for their coach to confirm. Each club's first member has
// both, so the start page's member does. The club's Members, a coach's
// gymnasts and the member's own Routines and Skills then show them.
func (s *seeder) keptRoutines(runs []clubRun) {
	mark := func(link, skill, state string) {
		s.post(link+"/skills", url.Values{"skill": {skill}, "state": {state}})
	}
	for _, run := range runs {
		for i, m := range run.members {
			vols := s.vols[m.level+"/2"]
			if s.err != nil || len(vols) == 0 || i > 0 && s.rng.IntN(3) == 0 {
				continue
			}
			raw := vols[s.rng.IntN(len(vols))]
			routines := []keep{{ID: "sandbox-2", Name: "Voluntary", Skills: json.RawMessage(raw)}}
			if first := s.vols[m.level+"/1"]; len(first) > 0 {
				routines[0].Name = "Second voluntary"
				routines = append([]keep{{ID: "sandbox-1", Name: "First voluntary", Skills: json.RawMessage(first[s.rng.IntN(len(first))])}}, routines...)
			}
			data, _ := json.Marshal(routines)
			s.post(m.link+"/routines", url.Values{"routines": {string(data)}})
			if i > 0 && s.rng.IntN(2) == 0 {
				continue
			}
			var can []json.RawMessage
			json.Unmarshal([]byte(raw), &can)
			seen := map[string]bool{}
			for _, skill := range can {
				if !seen[string(skill)] {
					seen[string(skill)] = true
					mark(m.link, string(skill), "can")
				}
			}
			next := s.levelIndex(m.level) + 1
			if next >= len(s.v.levels) {
				continue
			}
			harder := s.vols[s.v.levels[next].name+"/2"]
			if len(harder) == 0 {
				continue
			}
			var learning []json.RawMessage
			json.Unmarshal([]byte(harder[s.rng.IntN(len(harder))]), &learning)
			// The skills of a harder voluntary they can't do yet, up to two.
			for n := 0; len(learning) > 0 && n < 2; learning = learning[1:] {
				if skill := string(learning[0]); !seen[skill] {
					seen[skill] = true
					mark(m.link, skill, "learning")
					n++
				}
			}
		}
	}
}

// keep is a routine kept on a member's page, as the browser posts it.
type keep struct {
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	Skills json.RawMessage `json:"skills"`
}

// morningUntil is when the seeded morning stops, in minutes after midnight:
// flights finished by then are finished, and the one on then is under way.
const morningUntil = 11*60 + 15

// The areas the seed's venue has (run), and how late each is running on the
// morning, in minutes.
var morningAreas = []struct {
	name string
	late int
}{{"Panel 1", 10}, {"Panel 2", 0}, {"Panel 3", 5}, {"Track 1", 0}, {"DMT 1", 0}}

// Morning taps through the first morning of the competition whose admin
// link is admin, on the date of today, with the clock set to when each tap
// would be made: the panels and gymnasts checking in (an official late,
// a gymnast scratched and another arriving late), the flights started and
// finished, who's up in those under way, messages from the desk and notices
// on the venue screen. Restarting the day replays it.
func Morning(h Clocked, admin string, today time.Time) error {
	y, mo, d := today.In(dublin).Date()
	at := time.Date(y, mo, d, 8, 30, 0, 0, dublin)
	s := &seeder{h: h(func() time.Time { return at })}
	set := func(minute int) { at = time.Date(y, mo, d, 0, minute, 0, 0, dublin) }

	set(8*60 + 40)
	s.post(admin+"/desk/screen", url.Values{"text": {"Welcome to ISTO! The general warm-up is on every area from 09:00."}, "for": {"0"}})

	underWay := map[string]string{} // the flight on each area at the end of the morning, by key
	for ai, area := range morningAreas {
		done := map[string]bool{}
		for n := 0; s.err == nil; n++ {
			page := s.get(admin + "/day?day=0&area=" + url.QueryEscape(area.name))
			i := strings.Index(page, "comp-day-now")
			if i < 0 {
				break // finished for the day, or no flights
			}
			// The flight it's on, without its later flights and those finished.
			box := page[i:]
			for _, next := range []string{"Later on ", ">Finished</h3>"} {
				if j := strings.Index(box, next); j >= 0 {
					box = box[:j]
				}
			}
			key, start, end, ok := flightIn(box)
			if !ok || done[key] {
				s.fail("%s: no flight to start, or the last one didn't finish", area.name)
				break
			}
			done[key] = true
			if start+area.late > morningUntil {
				break
			}
			entries := matches(box, `data-tap="checkin:([^"]+)"`)
			officials := matches(box, `data-tap="official:`+regexp.QuoteMeta(html.EscapeString(key))+`:([^"]+)"`)
			first, lastFlight := ai == 0 && n == 0, end+area.late > morningUntil
			late := ai == 0 && lastFlight && len(entries) > 2 // a gymnast arriving after the flight's started

			// The panel and gymnasts check in from a quarter of an hour
			// before; on the first flight, one judge is late and one
			// gymnast doesn't come.
			set(start - 15)
			for k, o := range officials {
				status := "here"
				if first && k == 1 {
					status = "missing"
				}
				s.post(admin+"/day/officials", url.Values{"person": {o}, "flight": {key}, "status": {status}})
			}
			for k, e := range entries {
				status := "here"
				switch {
				case first && k == 1:
					status = "scratched"
				case late && k == 2:
					continue
				}
				s.post(admin+"/day/checkin", url.Values{"entry": {e}, "status": {status}})
			}
			if first && len(officials) > 1 {
				set(start - 2)
				s.post(admin+"/day/officials", url.Values{"person": {officials[1]}, "flight": {key}, "status": {"here"}})
			}
			set(start + area.late)
			s.post(admin+"/day/flight", url.Values{"flight": {key}, "what": {"start"}})
			if !lastFlight {
				set(end + area.late)
				s.post(admin+"/day/flight", url.Values{"flight": {key}, "what": {"finish"}})
				continue
			}
			// The flight on now: a few routines in.
			underWay[area.name] = key
			for k := range 3 {
				set(start + area.late + 4 + 3*k)
				box := s.get(admin + "/day?day=0&area=" + url.QueryEscape(area.name))
				form := box[max(strings.Index(box, "/day/up\""), 0):]
				entry, round := matches(form, `name="entry" value="([^"]*)"`), matches(form, `name="round" value="([^"]*)"`)
				if len(entry) == 0 || len(round) == 0 {
					break
				}
				s.post(admin+"/day/up", url.Values{"flight": {key}, "what": {"next"}, "entry": {entry[0]}, "round": {round[0]}})
			}
			if late {
				set(start + area.late + 8)
				s.post(admin+"/day/checkin", url.Values{"entry": {entries[2]}, "status": {"here"}})
			}
			break
		}
	}

	// Messages from the desk: to a club, to the panel of a flight on now,
	// to every official, and asking someone to come to the desk.
	set(10*60 + 5)
	desk := s.get(admin + "/desk")
	if club := matches(desk[max(strings.Index(desk, `name="club"`), 0):], `<option value="([^"]+)"`); len(club) > 0 {
		s.post(admin+"/desk", url.Values{"to": {"club"}, "club": {club[0]}, "text": {"Please collect your club's competitor numbers from the desk before 11:00."}})
	}
	if key := underWay["Panel 2"]; key != "" {
		set(10*60 + 50)
		s.post(admin+"/desk", url.Values{"to": {"flight"}, "flight": {key}, "flightChoice": {"1"}, "flightOfficials": {"1"},
			"text": {"Please stay on for the next flight: your lunch is at 12:30."}})
	}
	set(11*60 + 0)
	s.post(admin+"/desk", url.Values{"to": {"officials"}, "text": {"Officials' lunch is in the café upstairs, from 12:00."}})
	if person := matches(desk, `name="person" value="([^"]+)"`); len(person) > 0 {
		set(11*60 + 5)
		s.post(admin+"/desk", url.Values{"to": {"people"}, "person": {person[0]}, "come": {"1"}})
	}
	if area := morningAreas[0]; area.late > 0 {
		set(11*60 + 10)
		s.post(admin+"/desk/screen", url.Values{"text": {area.name + " is running about " + strconv.Itoa(area.late) + " minutes late."}, "for": {"60"}})
	}
	return s.err
}

// flightIn is the flight in a flight's box on the day page: its key, and
// when it's planned to start and end, in minutes after midnight.
func flightIn(box string) (key string, start, end int, ok bool) {
	keys := matches(box, `data-tap="flight:([^"]+)"`)
	m := regexp.MustCompile(`Planned (\d\d):(\d\d)–(\d\d):(\d\d)`).FindStringSubmatch(box)
	if len(keys) == 0 || m == nil {
		return "", 0, 0, false
	}
	n := func(i int) int { v, _ := strconv.Atoi(m[i]); return v }
	return keys[0], n(1)*60 + n(2), n(3)*60 + n(4), true
}

// matches are the first group of each match of a pattern in a page, as
// text.
func matches(page, pattern string) []string {
	var out []string
	for _, m := range regexp.MustCompile(pattern).FindAllStringSubmatch(page, -1) {
		out = append(out, html.UnescapeString(m[1]))
	}
	return out
}
