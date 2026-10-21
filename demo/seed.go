package demo

import (
	"fmt"
	"html"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"tariffCalculator/competitions"
)

// Seed fills h's competition storage with a made-up student competition run
// the ISTO way: 25 clubs (about 400 gymnasts) and some gymnasts entering on
// their own, in trampoline (BUCS levels, men and women apart), synchro,
// tumbling and DMT, with coaches signing entries off, judging offers, the
// organiser's own judges, a venue over three days of 09:00 to 17:30 (a
// general warm-up first, an hour's lunch that the biggest events run across,
// synchro in three events of grouped levels and a display on Sunday),
// the timetable planned with its officials rota, and published. Everything goes through the pages, as
// people would use them. It writes the links to open to out.
func Seed(h http.Handler, out io.Writer, seed uint64) error {
	return SeedSize(h, out, seed, Full)
}

// Size is how much of the demo to make.
type Size struct {
	Clubs       int // the first this many clubs, of 25
	Voluntaries int // passing voluntaries found for each level's exercise, for gymnasts to choose from
	Tries       int // routines tried to find them, at most; a gymnast with none to choose from gets one that may not pass
}

var (
	// Full is the demo as it's shown.
	Full = Size{Clubs: len(clubs), Voluntaries: 8, Tries: 30000}
	// Small is a few clubs, the same in every other way, for quick tests.
	Small = Size{Clubs: 3, Voluntaries: 1, Tries: 1000}
)

// SeedSize is Seed at a size: fewer clubs (at least two), or fewer
// voluntaries to choose from.
func SeedSize(h http.Handler, out io.Writer, seed uint64, size Size) error {
	return seedTo(h, out, nil, seed, size)
}

// SeedPages is Seed, also writing to pages every page someone at the
// competition opens on the day, one a line as "kind path" (kinds: member,
// own, coach, club, organiser, day, screen), for a load test
// (cmd/loadtest).
func SeedPages(h http.Handler, out, pages io.Writer, seed uint64) error {
	return seedTo(h, out, pages, seed, Full)
}

func seedTo(h http.Handler, out, pages io.Writer, seed uint64, size Size) error {
	s := &seeder{h: h, rng: rand.New(rand.NewPCG(seed, 99)), size: size, pages: pages, v: &bucs}
	return s.run(out)
}

type seeder struct {
	h    http.Handler
	rng  *rand.Rand
	size Size
	ip   int
	err  error
	vols map[string][]string // passing voluntaries, by level and exercise ("BUCS L3/2")
	// pages, if set, has every page someone opens on the day written to it
	// (SeedPages).
	pages io.Writer
	v     *variant            // the competition to make
	clubs map[string]*clubRun // the clubs made so far, by name: a later competition enters them again
	seen  map[string]bool     // the competitions made so far, by id
	made  made                // what the last competition made (the sandbox's links)
	// admins are every competition's admin link, as soon as it's made, so a
	// sandbox seed that fails partway can delete what it made.
	admins []string
}

// fail keeps the first error.
func (s *seeder) fail(format string, args ...any) {
	if s.err == nil {
		s.err = fmt.Errorf(format, args...)
	}
}

func (s *seeder) do(method, path string, form url.Values) *httptest.ResponseRecorder {
	var req *http.Request
	if form != nil {
		req = httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	s.ip++ // each person from their own address, as they would be
	req.RemoteAddr = fmt.Sprintf("198.51.100.%d:1234", s.ip%250+1)
	rec := httptest.NewRecorder()
	s.h.ServeHTTP(rec, req)
	return rec
}

// post posts a form and returns where it redirects, without its query.
func (s *seeder) post(path string, form url.Values) string {
	if s.err != nil {
		return ""
	}
	rec := s.do(http.MethodPost, path, form)
	if rec.Code != http.StatusSeeOther {
		s.fail("POST %s: %d %s", path, rec.Code, excerpt(rec.Body.String()))
		return ""
	}
	loc := rec.Header().Get("Location")
	if strings.Contains(loc, "Nothing+was+changed") || strings.Contains(loc, "nothing+was+changed") || strings.Contains(loc, "Nobody+was+added") {
		s.fail("POST %s: %s", path, loc)
	}
	return strings.SplitN(loc, "?", 2)[0]
}

func (s *seeder) get(path string) string {
	if s.err != nil {
		return ""
	}
	rec := s.do(http.MethodGet, path, nil)
	if rec.Code == http.StatusSeeOther {
		// Where the page now lives, as a browser would follow: an entry's
		// link opens its person's page for the competition.
		rec = s.do(http.MethodGet, rec.Header().Get("Location"), nil)
	}
	if rec.Code != http.StatusOK {
		s.fail("GET %s: %d", path, rec.Code)
	}
	return rec.Body.String()
}

// link finds a link starting with prefix in a page.
func (s *seeder) link(page, prefix string) string {
	if s.err != nil {
		return ""
	}
	m := regexp.MustCompile(`(?:http://example\.com)?(` + regexp.QuoteMeta(prefix) + `[A-Za-z0-9_-]+)`).FindStringSubmatch(page)
	if m == nil {
		s.fail("no %s link", prefix)
		return ""
	}
	return m[1]
}

func excerpt(body string) string {
	if i := strings.Index(body, "notification is-danger"); i >= 0 {
		body = body[i:]
	}
	re := regexp.MustCompile(`<[^>]+>`)
	body = strings.Join(strings.Fields(re.ReplaceAllString(body, " ")), " ")
	return body[:min(len(body), 300)]
}

// --- The people ---

var (
	women = []string{"Aoife", "Ciara", "Niamh", "Siobhán", "Orla", "Gráinne", "Caoimhe", "Róisín", "Sinéad", "Aisling", "Clodagh", "Méabh", "Saoirse", "Éabha", "Laura", "Emma", "Sophie", "Hannah", "Rachel", "Katie", "Megan", "Chloe", "Ellen", "Ava", "Leah", "Molly", "Ruth", "Fiona", "Jane", "Holly"}
	men   = []string{"Cian", "Oisín", "Darragh", "Eoin", "Seán", "Conor", "Niall", "Cathal", "Ruairí", "Tadhg", "Fionn", "Pádraig", "Diarmuid", "Liam", "Jack", "James", "Adam", "Luke", "Ben", "Mark", "David", "Rory", "Ciarán", "Kevin", "Aaron", "Dylan", "Shane", "Paul", "Evan", "Colm"}
	last  = []string{"Murphy", "Kelly", "O'Sullivan", "Walsh", "Smith", "O'Brien", "Byrne", "Ryan", "O'Connor", "O'Neill", "O'Reilly", "Doyle", "McCarthy", "Gallagher", "Doherty", "Kennedy", "Lynch", "Murray", "Quinn", "Moore", "McLoughlin", "Carroll", "Connolly", "Daly", "Brennan", "Burke", "Collins", "Campbell", "Clarke", "Johnston", "Hughes", "Farrell", "Fitzgerald", "Brown", "Martin", "Maguire", "Nolan", "Flynn", "Thompson", "Callaghan"}
	clubs = []struct {
		name    string
		members int
		coaches []string
	}{
		{"UCD", 32, []string{"Declan Ward", "Maria Costa"}},
		{"DCU", 28, []string{"Alan Power", "Sinéad Kirwan"}},
		{"Trinity", 30, []string{"Helen Byrne", "Tom Keogh"}},
		{"UL", 24, []string{"Brian Ahern"}},
		{"UCC", 22, []string{"Nora Healy", "Fergal Twomey"}},
		{"Galway", 20, []string{"Ger Fahy"}},
		{"Maynooth", 17, []string{"Ita Dunne"}},
		{"TU Dublin", 16, []string{"Paddy Kerr"}},
		{"Queen's", 23, []string{"Lorraine McAllister"}},
		{"Ulster", 14, []string{"Gavin Hamill"}},
		{"MTU Cork", 13, []string{"Brendan Lucey"}},
		{"SETU", 12, []string{"Orla Phelan"}},
		{"ATU Sligo", 10, []string{"Dermot Feeney"}},
		{"TUS Athlone", 11, []string{"Marie Claffey"}},
		{"Mary Immaculate", 9, []string{"Tríona Hogan"}},
		{"RCSI", 10, []string{"Neil Coughlan"}},
		{"Griffith", 8, []string{"Úna Brady"}},
		{"DkIT", 9, []string{"Seamus McEvoy"}},
		{"IADT", 8, []string{"Karen Mooney"}},
		{"Edinburgh", 20, []string{"Fraser Ross", "Kirsty Hay"}},
		{"Glasgow", 16, []string{"Iain Morrison"}},
		{"Strathclyde", 13, []string{"Alison Kerr"}},
		{"Stirling", 11, []string{"Ewan Fraser"}},
		{"St Andrews", 10, []string{"Morag Reid"}},
		{"Aberdeen", 10, []string{"Callum Grant"}},
	}
	// Trampoline levels, easiest first, as the competition lists them, and
	// how many of each hundred gymnasts enter each.
	levels = []levelShare{{"BUCS L7", 24}, {"BUCS L6", 22}, {"BUCS L5", 18}, {"BUCS L4", 14}, {"BUCS L3", 11}, {"BUCS L2", 7}, {"BUCS L1", 4}}
)

// clubRun is a club as the demo made it: its admin and join links,
// coaches' links by name, members, and the links its club admin made with a
// role (ADR 0015).
type clubRun struct {
	name, admin, join string
	coaches           map[string]string
	members           []person
	roles             []roleLink
}

// roleLink is a link a club admin made for someone, with a role.
type roleLink struct {
	role, person, link string
}

// clubRoles are the club's roles (ADR 0015), made by its club admin on
// Settings: a Treasurer, who sees the fees, and a Team manager, who talks
// to the organiser, each with a link for one of its members.
func (s *seeder) clubRoles(run clubRun) []roleLink {
	var out []roleLink
	for i, r := range []struct {
		name        string
		permissions []string
	}{
		{"Treasurer", []string{"fees"}},
		{"Team manager", []string{"organiser"}},
	} {
		s.post(run.admin+"/roles", url.Values{"name": {r.name}, "permission": r.permissions})
		// Settings lists the roles in the order they were made.
		ids := regexp.MustCompile(`action="`+regexp.QuoteMeta(run.admin)+`/roles/([^"/]+)"`).FindAllStringSubmatch(s.get(run.admin+"/settings"), -1)
		if s.err != nil {
			return out
		}
		if len(ids) <= i {
			s.fail("no role %s on %s's Settings", r.name, run.name)
			return out
		}
		id := ids[i]
		who := r.name // a member of the club, where there are enough
		if len(run.members) > 4+i {
			who = run.members[4+i].name
		}
		rec := s.do(http.MethodPost, run.admin+"/links", url.Values{"name": {who}, "role": {id[1]}})
		if rec.Code != http.StatusOK {
			s.fail("making %s's link: %d", who, rec.Code)
			return out
		}
		// The new link is the page's one full address under /clubs/admin/:
		// the page's own links are paths.
		m := regexp.MustCompile(`http://example\.com(/clubs/admin/[A-Za-z0-9_-]+)`).FindStringSubmatch(rec.Body.String())
		if m == nil {
			s.fail("no link shown for %s", who)
			return out
		}
		out = append(out, roleLink{role: r.name, person: who, link: m[1]})
	}
	return out
}

// levelShare is a level and how many of each hundred gymnasts enter it.
type levelShare struct {
	name   string
	weight int
}

type person struct {
	name, category string
	level          string // trampoline
	link           string // their member page, or own entry
	club           string
	coach          string
}

func (s *seeder) name(used map[string]bool, category string) string {
	for {
		first := women[s.rng.IntN(len(women))]
		if category == "Men" {
			first = men[s.rng.IntN(len(men))]
		}
		n := first + " " + last[s.rng.IntN(len(last))]
		if !used[n] {
			used[n] = true
			return n
		}
	}
}

// levelIndex is a trampoline level's place in the competition's levels,
// easiest first.
func (s *seeder) levelIndex(name string) int {
	return slices.IndexFunc(s.v.levels, func(l levelShare) bool { return l.name == name })
}

func (s *seeder) level() string {
	r := s.rng.IntN(100)
	for _, l := range s.v.levels {
		if r < l.weight {
			return l.name
		}
		r -= l.weight
	}
	return s.v.levels[0].name
}

// option is the form value for a level's exercise's option: "builtin:bucs-l3-option-1".
func option(level string, exercise int, n int) string {
	id := "bucs-" + strings.ToLower(strings.TrimPrefix(level, "BUCS "))
	switch {
	case level == "BUCS L1" || level == "BUCS L2":
		return "builtin:" + id + [...]string{"-first", "-second"}[exercise]
	case exercise == 0:
		return fmt.Sprintf("builtin:%s-option-%d", id, n)
	}
	return "builtin:" + id + "-second"
}

// routine fills an entry form's exercises at a level: a set routine first
// (or a voluntary, at the hardest levels) and a voluntary that passes, or
// now and then one that doesn't, as happens.
func (s *seeder) routine(form url.Values, level string, careless bool) {
	form.Set("level", level)
	form.Set("ex1Option", s.v.option(level, 0, 1+s.rng.IntN(2)))
	form.Set("ex2Option", s.v.option(level, 1, 0))
	for ex := range 2 {
		if !s.v.voluntary(level, ex) {
			continue // a set routine
		}
		key := fmt.Sprintf("%s/%d", level, ex+1)
		vols := s.vols[key]
		raw := ""
		if len(vols) > 0 {
			raw = vols[s.rng.IntN(len(vols))]
		}
		if careless || raw == "" {
			st := style[level]
			r, _ := jsonRoutine(walk(s.rng, 9+s.rng.IntN(3), st[0], st[1], int(st[2])))
			raw = r
		}
		form.Set(fmt.Sprintf("ex%dSkills", ex+1), raw)
	}
}

// variant is the competition a seed makes: its name and dates, its levels
// and events and how it's set up, and how far it has got. The demo's is
// bucs; the sandbox's are ISTO's (sandbox.go).
type variant struct {
	name, date, deadline string                                     // the dates as the form takes them, "2027-02-26"
	levels               []levelShare                               // trampoline, easiest first
	offered              []string                                   // more levels it has, that no one in the seed enters
	chairs               []string                                   // the levels whose gymnasts may chair
	ref                  func(level string) string                  // a level's reference, "builtin-level:bucs-l3"
	option               func(level string, exercise, n int) string // an exercise's option (n: which set routine), "builtin:bucs-l3-option-1"
	voluntary            func(level string, exercise int) bool      // whether an exercise is a voluntary rather than a set routine
	synchro              string                                     // the synchro events, a line each, levels joined by " + "
	tumbling, dmt        []string                                   // their levels
	// What a gymnast enters, each as often as it's listed, and what the
	// first gymnast on their own enters.
	tumblingPick, dmtPick []string
	ownTumbling           string
	days                  []string     // the venue's days, by name
	rules                 []url.Values // the timetable's rules, as the form posts them
	chairFor              string       // the event the organiser's first judge must chair
	limits                map[string]string
	fees                  string // how to pay
	// open is a competition still open for entries: some members haven't
	// entered, some entries aren't signed off and some clubs haven't sent
	// theirs, and the timetable is set up but not planned or published.
	open bool
}

// bucs is the demo's competition: ISTO 2027 with BUCS levels, men and women
// apart, closed and published.
var bucs = variant{
	name: "ISTO 2027 (demo)", date: "2027-02-26", deadline: "2027-02-13",
	levels: levels, chairs: []string{"BUCS L1", "BUCS L2"},
	ref: func(level string) string {
		return "builtin-level:bucs-" + strings.ToLower(strings.TrimPrefix(level, "BUCS "))
	},
	option: option,
	voluntary: func(level string, exercise int) bool {
		return exercise == 1 || level == "BUCS L1" || level == "BUCS L2"
	},
	// Synchro on three panels: three events, levels grouped.
	synchro:  "BUCS L6 + BUCS L7\nBUCS L4 + BUCS L5\nBUCS L1 + BUCS L2 + BUCS L3",
	tumbling: []string{"Novice", "Intermediate", "Advanced"}, dmt: []string{"Novice", "Advanced"},
	tumblingPick: []string{"Novice", "Novice", "Intermediate", "Advanced"}, dmtPick: []string{"Novice", "Novice", "Advanced"},
	ownTumbling: "Intermediate",
	days:        []string{"Friday", "Saturday", "Sunday"},
	rules: []url.Values{
		{"kind": {"day"}, "must": {"1"}, "event": {"Synchro BUCS L6/L7"}, "day": {"2"}},
		{"kind": {"day"}, "must": {"1"}, "event": {"Synchro BUCS L4/L5"}, "day": {"2"}},
		{"kind": {"day"}, "must": {"1"}, "event": {"Synchro BUCS L1/L2/L3"}, "day": {"2"}},
		{"kind": {"day"}, "must": {"0"}, "event": {"BUCS L1"}, "day": {"2"}},
		{"kind": {"before"}, "must": {"0"}, "event": {"BUCS L7"}, "event2": {"BUCS L6"}},
	},
	chairFor: "BUCS L1",
	limits:   map[string]string{"BUCS L1": "12", "DMT Advanced": "17"},
	fees:     "Bank transfer to ISTO 2027, IBAN IE00 DEMO 0000 0000 0000 00, with your club's name as the reference, by 20 February.",
}

// made is what a seed made of a competition: the links the sandbox hands
// out, and the clubs and gymnasts in it.
type made struct {
	name, admin, id     string // the organiser's link and the competition's id
	enter               string // the link to enter on one's own
	judges, chairs      string // helpers' links: checking cards, and chairs of judges
	screen              string // the venue screen's link
	runs                []clubRun
	owns                []string // the entries of gymnasts on their own (their entry links)
	ownPage, ownSignoff string   // the first one's own page, and their coach's sign-off link
	ownName             string
}

func (s *seeder) run(out io.Writer) error {
	// Voluntaries that pass, for every level's voluntary exercises.
	var c competitions.Competition
	for _, l := range s.v.levels {
		c.Levels = append(c.Levels, competitions.Level{Ref: s.v.ref(l.name)})
	}
	if s.vols == nil {
		s.vols = map[string][]string{}
	}
	for _, l := range s.v.levels {
		for ex := range 2 {
			key := fmt.Sprintf("%s/%d", l.name, ex+1)
			if _, found := s.vols[key]; found || !s.v.voluntary(l.name, ex) {
				continue
			}
			opts := [2]string{s.v.option(l.name, 0, 1), s.v.option(l.name, 1, 0)}
			s.vols[key] = voluntaries(c, l.name, ex, opts, s.size.Voluntaries, s.size.Tries, s.rng)
		}
	}

	// The organiser creates the competition.
	form := url.Values{
		"name": {s.v.name}, "date": {s.v.date}, "deadlineDate": {s.v.deadline}, "deadlineTime": {"23:59"}, "opens": {"now"},
		"individuals": {"1"}, "signoff": {"1"},
		"synchroPairs": {s.v.synchro},
		"tumbling":     {strings.Join(s.v.tumbling, "\n")}, "dmt": {strings.Join(s.v.dmt, "\n")},
	}
	for _, l := range s.v.levels {
		ref := s.v.ref(l.name)
		form.Add("level", ref)
		form.Add("synchroLevel", ref)
	}
	for _, l := range s.v.offered {
		form.Add("level", s.v.ref(l))
	}
	admin := s.post("/competitions", form)
	if admin != "" {
		s.admins = append(s.admins, admin)
	}
	// Men and women rank apart, except in synchro, where pairs can be mixed.
	split := url.Values{"split": {competitions.SplitSome}}
	for _, l := range s.v.levels {
		split.Add("splitLevel", l.name)
	}
	for _, ev := range s.v.tumbling {
		split.Add("splitLevel", "Tumbling "+ev)
	}
	for _, ev := range s.v.dmt {
		split.Add("splitLevel", "DMT "+ev)
	}
	s.post(admin+"/split", split)
	dash := s.get(admin + "/settings")
	clubLink := s.link(dash, "/competitions/club/")
	enter := s.link(dash, "/competitions/enter/")
	if s.err != nil {
		return s.err
	}
	compID := ""

	// Panels as a student competition runs them, with HD judges (no machine).
	s.post(admin+"/officials/settings", url.Values{"judge": {""},
		"panel-trampoline-chair": {"1"}, "panel-trampoline-execution": {"4"}, "panel-trampoline-difficulty": {"1"}, "panel-trampoline-hd": {"2"}, "panel-trampoline-recorder": {"1"}, "panel-trampoline-marshal": {"1"},
		"panel-synchro-chair": {"1"}, "panel-synchro-execution": {"4"}, "panel-synchro-difficulty": {"1"}, "panel-synchro-hd": {"2"}, "panel-synchro-recorder": {"1"}, "panel-synchro-marshal": {"0"},
		"panel-tumbling-chair": {"1"}, "panel-tumbling-execution": {"3"}, "panel-tumbling-difficulty": {"1"}, "panel-tumbling-recorder": {"1"}, "panel-tumbling-marshal": {"1"},
		"panel-dmt-chair": {"1"}, "panel-dmt-execution": {"3"}, "panel-dmt-difficulty": {"1"}, "panel-dmt-recorder": {"1"}, "panel-dmt-marshal": {"0"},
	})

	if s.clubs == nil {
		s.clubs, s.seen = map[string]*clubRun{}, map[string]bool{}
	}
	used := map[string]bool{}
	for _, run := range s.clubs {
		for _, m := range run.members {
			used[m.name] = true
		}
	}
	var clubAdmins []string
	var everyone []person
	var firstMember, firstCoach string
	var runs []clubRun
	newClubs := false
	for _, cl := range clubs[:min(s.size.Clubs, len(clubs))] {
		// A club made for a competition before enters this one too.
		run, again := s.clubs[cl.name]
		if !again {
			newClubs = true
			run = &clubRun{name: cl.name, admin: s.post("/clubs", url.Values{"name": {cl.name}}), coaches: map[string]string{}}
			s.clubs[cl.name] = run
		}
		club := run.admin
		clubAdmins = append(clubAdmins, cl.name+": "+club)
		s.post(clubLink, url.Values{"clubAdmin": {club}})
		if !again {
			run.join = s.link(s.get(club+"/members"), "/clubs/join/")
			// Coaches.
			for _, name := range cl.coaches {
				rec := s.do(http.MethodPost, club+"/coaches", url.Values{"name": {name}})
				run.coaches[name] = s.link(rec.Body.String(), "/clubs/coach/")
			}
			// Members join, each with a coach.
			for range cl.members {
				cat := []string{"Women", "Men"}[s.rng.IntN(2)]
				m := person{name: s.name(used, cat), category: cat, level: s.level(), club: cl.name}
				m.link = s.post(run.join, url.Values{"name": {m.name}})
				m.coach = cl.coaches[s.rng.IntN(len(cl.coaches))]
				if id := regexp.MustCompile(`<option value="([^"]+)">` + regexp.QuoteMeta(m.coach) + `</option>`).FindStringSubmatch(s.get(m.link + "/details")); id != nil {
					s.post(m.link+"/coach", url.Values{"coach": {id[1]}})
				}
				run.members = append(run.members, m)
			}
		}
		if firstCoach == "" {
			firstCoach = cl.coaches[0] + " (" + cl.name + "): " + run.coaches[cl.coaches[0]]
		}
		// Members enter.
		for i, m := range run.members {
			if compID == "" {
				// The competition on their page that the seed hasn't made before.
				for _, id := range regexp.MustCompile(regexp.QuoteMeta(m.link)+`/competitions/([A-Za-z0-9_-]+)`).FindAllStringSubmatch(s.get(m.link), -1) {
					if !s.seen[id[1]] {
						compID = id[1]
						break
					}
				}
				if compID == "" {
					s.fail("no competition on %s's page", m.name)
				}
				s.seen[compID] = true
			}
			if firstMember == "" {
				firstMember = m.name + " (" + cl.name + "): " + m.link
			}
			if s.v.open && i > 3 && s.rng.IntN(100) < 15 {
				continue // not entered yet
			}
			save := m.link + "/competitions/" + compID
			entry := url.Values{"discipline": {""}, "category": {m.category}}
			s.routine(entry, m.level, s.rng.IntN(100) < 7)
			s.post(save, entry)
			if s.rng.IntN(100) < 28 {
				s.post(save, url.Values{"discipline": {"tumbling"}, "category": {m.category}, "level": {s.v.tumblingPick[s.rng.IntN(len(s.v.tumblingPick))]}})
			}
			if s.rng.IntN(100) < 16 {
				s.post(save, url.Values{"discipline": {"dmt"}, "category": {m.category}, "level": {s.v.dmtPick[s.rng.IntN(len(s.v.dmtPick))]}})
			}
			// What they'll judge or help with.
			offer := url.Values{}
			if s.rng.IntN(100) < 55 {
				offer.Set("judge-trampoline", "1")
				offer.Set("upto-trampoline", m.level)
				if slices.Contains(s.v.chairs, m.level) || s.rng.IntN(100) < 8 {
					offer.Set("chair-trampoline", "1")
				}
				if s.rng.IntN(100) < 40 {
					offer.Set("judge-synchro", "1")
				}
			}
			if s.rng.IntN(100) < 15 {
				offer.Set("judge-tumbling", "1")
			}
			if s.rng.IntN(100) < 10 {
				offer.Set("judge-dmt", "1")
			}
			if s.rng.IntN(100) < 20 {
				offer.Set("recorder", "1")
			}
			if s.rng.IntN(100) < 25 {
				offer.Set("marshal", "1")
			}
			if len(offer) > 0 {
				s.post(save+"/offer", offer)
			}
		}
		runs = append(runs, *run)
		everyone = append(everyone, run.members...)
		if s.err != nil {
			return s.err
		}
	}

	// One link, several roles (ADR 0010): in the first club, the comp sec
	// makes a member a coach too, and another member, who is the club's
	// comp sec, adds its admin link to their page.
	first := runs[0]
	coachToo, compSec := first.members[1], first.members[2]
	coached := first.members[3] // and is coached by the member who coaches
	if newClubs {
		if id := regexp.MustCompile(`(?s)` + regexp.QuoteMeta(html.EscapeString(coachToo.name)) + `.*?name="member" value="([^"]+)"`).FindStringSubmatch(s.get(first.admin + "/members")); id != nil {
			s.post(first.admin+"/coaches/from-member", url.Values{"member": {id[1]}})
		} else {
			s.fail("no %s on the Members page", coachToo.name)
		}
		s.post(compSec.link+"/links", url.Values{"link": {"http://example.com" + first.admin}})
		if id := regexp.MustCompile(`<option value="([^"]+)">` + regexp.QuoteMeta(html.EscapeString(coachToo.name)) + `</option>`).FindStringSubmatch(s.get(coached.link + "/details")); id != nil {
			s.post(coached.link+"/coach", url.Values{"coach": {id[1]}})
		} else {
			s.fail("no coach %s for %s", coachToo.name, coached.name)
		}
		// Its roles (ADR 0015), made once, with the club.
		s.clubs[first.name].roles = s.clubRoles(first)
	}
	runs[0].roles = s.clubs[first.name].roles

	// Synchro pairs, up to four a club: men and women alike, about a third
	// with a gymnast from another club, two gymnasts no more than a level
	// apart individually, doing the level they share or the easier of the
	// two. Each is confirmed by the partner.
	paired := map[string]bool{}
	partnerFor := func(a person, from []person) (person, bool) {
		for _, b := range from {
			ai, bi := s.levelIndex(a.level), s.levelIndex(b.level)
			if b.name != a.name && !paired[b.name] && ai-bi <= 1 && bi-ai <= 1 {
				return b, true
			}
		}
		return person{}, false
	}
	for ci, run := range runs {
		pairs := 0
		for _, a := range run.members {
			if pairs == 4 {
				break
			}
			if paired[a.name] {
				continue
			}
			other := runs[(ci+1+s.rng.IntN(len(runs)-1))%len(runs)].members
			from := [][]person{run.members, other}
			if s.rng.IntN(3) == 0 {
				from[0], from[1] = other, run.members
			}
			b, ok := partnerFor(a, from[0])
			if !ok {
				b, ok = partnerFor(a, from[1])
			}
			if !ok {
				continue
			}
			paired[a.name], paired[b.name] = true, true
			pairs++
			entry := url.Values{"discipline": {"synchro"}, "partnerName": {b.name}, "partnerClub": {b.club}}
			s.routine(entry, s.v.levels[min(s.levelIndex(a.level), s.levelIndex(b.level))].name, false)
			s.post(a.link+"/competitions/"+compID, entry)
			partner := s.link(s.get(a.link+"/competitions/"+compID), "/competitions/partner/")
			s.post(partner, url.Values{"link": {"http://example.com" + b.link}})
		}
	}
	// The coaches sign off most entries, and each comp sec sends them all;
	// while entries are open, coaches have signed off about half, and every
	// other club, from the second, has sent its entries.
	signs := 88
	if s.v.open {
		signs = 55
	}
	for i, run := range runs {
		for _, coach := range run.coaches {
			page := s.get(coach)
			for _, m := range regexp.MustCompile(`href="(`+regexp.QuoteMeta(coach)+`/members/[^"/]+/competitions/`+regexp.QuoteMeta(compID)+`[^"]*)"`).FindAllStringSubmatch(page, -1) {
				if s.rng.IntN(100) < signs {
					s.post(strings.ReplaceAll(m[1], "&amp;", "&"), url.Values{"signed": {"1"}, "note": {[]string{"", "", "Good luck!", "Watch your arms in the set"}[s.rng.IntN(4)]}})
				}
			}
		}
		if !s.v.open || i%2 == 1 {
			s.post(run.admin+"/competitions/"+compID+"/send", url.Values{"which": {"all"}})
		}
	}
	if s.err != nil {
		return s.err
	}

	// Gymnasts entering on their own, some offering to judge. The first
	// enters tumbling too, from their own page, so it has two entries.
	var firstOwn, firstOwnEntry, ownPage, ownSignoff, ownName string
	var owns []string // their entries' pages
	for i := range 6 {
		cat := []string{"Women", "Men"}[i%2]
		p := person{name: s.name(used, cat), category: cat, level: s.level()}
		entry := url.Values{"gymnast": {p.name}, "category": {cat}}
		s.routine(entry, p.level, false)
		own := s.post(enter, entry)
		owns = append(owns, own)
		signoff := s.link(s.get(own), "/competitions/signoff/")
		if firstOwn == "" {
			page := s.link(s.get(own), "/competitions/gymnast/")
			s.post(page+"/competitions/"+compID+"/enter", url.Values{"gymnast": {p.name}, "category": {cat}, "discipline": {"tumbling"}, "level": {s.v.ownTumbling}})
			firstOwn, firstOwnEntry = p.name+": "+page, own
			ownPage, ownSignoff, ownName = page, signoff, p.name
		}
		if i%2 == 0 {
			s.post(own+"/offer", url.Values{"judge-trampoline": {"1"}, "upto-trampoline": {p.level}})
		}
		if s.v.open && i == 0 {
			continue // their coach hasn't signed it off yet
		}
		s.post(signoff, url.Values{"coach": {"Own coach"}, "signed": {"1"}})
	}

	// The organiser's own judges: unattached, some qualified to chair.
	for i, j := range []struct {
		name, club string
		offer      url.Values
	}{
		{"Gráinne Walsh", "", url.Values{"judge-trampoline": {"1"}, "chair-trampoline": {"1"}, "judge-synchro": {"1"}, "chair-synchro": {"1"}}},
		{"Michael Doyle", "", url.Values{"judge-trampoline": {"1"}, "chair-trampoline": {"1"}}},
		{"Anne-Marie Keane", "Gymnastics Ireland", url.Values{"judge-trampoline": {"1"}, "chair-trampoline": {"1"}, "judge-tumbling": {"1"}, "chair-tumbling": {"1"}}},
		{"Pat Hennessy", "", url.Values{"judge-tumbling": {"1"}, "chair-tumbling": {"1"}, "judge-dmt": {"1"}, "chair-dmt": {"1"}}},
		{"Joan Furlong", "", url.Values{"judge-dmt": {"1"}, "chair-dmt": {"1"}, "judge-trampoline": {"1"}}},
		{"Kieran Tobin", "", url.Values{"judge-trampoline": {"1"}, "judge-synchro": {"1"}, "chair-synchro": {"1"}}},
		{"Deirdre Ní Bhriain", "", url.Values{"judge-tumbling": {"1"}, "judge-dmt": {"1"}, "recorder": {"1"}}},
		{"Frank Lyons", "", url.Values{"recorder": {"1"}, "marshal": {"1"}}},
	} {
		f := j.offer
		f.Set("name", j.name)
		f.Set("club", j.club)
		s.post(admin+"/officials/add", f)
		_ = i
	}

	// The venue: Friday, Saturday and Sunday, 09:00 to 17:30, every area.
	tt := admin + "/timetable"
	s.post(tt+"/setup/areas", url.Values{"area-0-name": {"Panel 1"}, "area-0-discipline": {"trampoline"}, "area-1-name": {"Track 1"}, "area-1-discipline": {"tumbling"}, "area-2-name": {"DMT 1"}, "area-2-discipline": {"dmt"}})
	s.post(tt+"/setup/areas", url.Values{"area-0-name": {"Panel 1"}, "area-0-discipline": {"trampoline"}, "area-1-name": {"Track 1"}, "area-1-discipline": {"tumbling"}, "area-2-name": {"DMT 1"}, "area-2-discipline": {"dmt"}, "add": {"1"}})
	s.post(tt+"/setup/areas", url.Values{"area-0-name": {"Panel 1"}, "area-0-discipline": {"trampoline"}, "area-1-name": {"Track 1"}, "area-1-discipline": {"tumbling"}, "area-2-name": {"DMT 1"}, "area-2-discipline": {"dmt"}, "area-3-name": {"Panel 2"}, "area-3-discipline": {"trampoline"}, "add": {"1"}})
	s.post(tt+"/setup/areas", url.Values{"area-0-name": {"Panel 1"}, "area-0-discipline": {"trampoline"}, "area-1-name": {"Track 1"}, "area-1-discipline": {"tumbling"}, "area-2-name": {"DMT 1"}, "area-2-discipline": {"dmt"}, "area-3-name": {"Panel 2"}, "area-3-discipline": {"trampoline"}, "area-4-name": {"Panel 3"}, "area-4-discipline": {"trampoline"}})
	days := url.Values{}
	for i, name := range s.v.days {
		days.Set(fmt.Sprintf("day-%d-name", i), name)
		days.Set(fmt.Sprintf("day-%d-start", i), "09:00")
		days.Set(fmt.Sprintf("day-%d-end", i), "17:30")
		if i < 2 {
			days.Set("add", "1")
		} else {
			days.Del("add")
		}
		s.post(tt+"/setup/days", days)
	}
	// Each day: a general warm-up on every area before the panels start at
	// 09:30, and an hour's lunch. Sunday: a display taking the whole venue,
	// fun synchro, then awards.
	var blocks []url.Values
	for day := range 3 {
		d := fmt.Sprint(day)
		blocks = append(blocks,
			url.Values{"name": {"General warm-up"}, "minutes": {"30"}, "day": {d}, "at": {"09:00"}},
			url.Values{"name": {"Lunch"}, "minutes": {"60"}, "day": {d}, "from": {"12:00"}, "to": {"14:00"}},
		)
	}
	blocks = append(blocks,
		url.Values{"name": {"Display"}, "minutes": {"60"}, "day": {"2"}, "at": {"14:00"}},
		url.Values{"name": {"Fun synchro (entries on the day)"}, "minutes": {"45"}, "day": {"2"}, "at": {"16:00"}, "area": {"Panel 1"}, "chair": {"1"}, "judges": {"3"}, "recorder": {"1"}},
		url.Values{"name": {"Awards"}, "minutes": {"40"}, "day": {"2"}, "at": {"16:50"}},
	)
	for _, b := range blocks {
		b.Set("add", "1")
		s.post(tt+"/setup/blocks", b)
	}
	// The biggest events (five flights, five hours or more) are longer than
	// the time between breaks, so they pause for lunch.
	s.post(tt+"/setup/blocks", url.Values{"breaks": {"1"}, "across": {"1"}})
	for _, rule := range s.v.rules {
		r := url.Values{"add": {"1"}}
		for k, v := range rule {
			r[k] = v
		}
		s.post(tt+"/setup/rules", r)
	}
	page := s.get(tt)
	if m := regexp.MustCompile(`<option value="([^"]+)">Gráinne Walsh</option>`).FindStringSubmatch(page); m != nil {
		s.post(tt+"/setup/rules", url.Values{"add": {"1"}, "kind": {"role"}, "must": {"1"}, "person": {m[1]}, "role": {"chair"}, "event": {s.v.chairFor}})
	}
	if m := regexp.MustCompile(`<option value="([^"]+)">Frank Lyons</option>`).FindStringSubmatch(page); m != nil {
		s.post(tt+"/setup/rules", url.Values{"add": {"1"}, "kind": {"hours"}, "must": {"0"}, "person": {m[1]}, "day": {"1"}, "from": {"12:00"}})
	}
	// Two events are full, with a waiting list.
	limits, limitForm := url.Values{}, s.get(admin+"/settings") // the limits are in Settings
	for event, limit := range s.v.limits {
		if m := regexp.MustCompile(`name="(limit-\d+)"[^>]*aria-label="Limit for ` + regexp.QuoteMeta(event) + `"`).FindStringSubmatch(limitForm); m != nil {
			limits.Set(m[1], limit)
		}
	}
	limits.Set("anyway", "1") // past the question: it moves some to the waiting lists
	s.post(admin+"/limits", limits)
	if !s.v.open {
		s.post(tt+"/plan", url.Values{})
		s.post(tt+"/publish", url.Values{"on": {"1"}})
	}

	// Helpers' links (ADR 0009): the difficulty judges check a few cards,
	// and a chair of judges flags a concern about one.
	helper := func(name, kind string) string {
		if s.err != nil {
			return ""
		}
		rec := s.do(http.MethodPost, admin+"/links", url.Values{"name": {name}, "kind": {kind}})
		if rec.Code != http.StatusOK {
			s.fail("making a link for %s: %d", name, rec.Code)
			return ""
		}
		m := regexp.MustCompile(`http://example\.com(/competitions/admin/[A-Za-z0-9_-]+)`).FindStringSubmatch(rec.Body.String())
		if m == nil {
			s.fail("no link for %s", name)
			return ""
		}
		return m[1]
	}
	judges, chairs := helper("Difficulty judges", "cards"), helper("Chairs of judges", "chair")
	helper("Timetable team", "timetable")
	screen := helper("Venue screen", "screen")
	entryLinks := regexp.MustCompile(`href="`+regexp.QuoteMeta(admin)+`/entries/([^"#]+)"`).FindAllStringSubmatch(s.get(admin), 4)
	for i, m := range entryLinks {
		if i < 3 {
			s.post(judges+"/entries/"+m[1]+"/check", url.Values{"checked": {"1"}})
		} else {
			s.post(chairs+"/concerns", url.Values{"entry": {m[1]}, "text": {"The second routine's element 7 isn't what's on the card: check with the coach before the flight."}})
		}
	}
	s.post(chairs+"/concerns", url.Values{"text": {"Panel 2's trampoline has a loose spring cover: can someone look at it at lunch?"}})
	// A member tells the organiser something, from their own page; the
	// organiser replies, and the member answers: a thread, waiting for the
	// organiser again.
	s.post(coached.link+"/competitions/"+compID+"/organiser", url.Values{"text": {"I'm doing my exams on the Friday morning: could I be in an afternoon flight?"}})
	if m := regexp.MustCompile(`id="concern-([^"]+)"`).FindStringSubmatch(s.get(admin)); m != nil {
		s.post(admin+"/concerns/"+m[1]+"/reply", url.Values{"reply": {"We'll try. Which exam, and when does it finish?"}})
		s.post(coached.link+"/competitions/"+compID+"/organiser/"+m[1], url.Values{"text": {"Maths, finishing at 12:30. Anything after 2 would be great."}})
	}

	// Feedback about the app, from a member's My competition and a chair's
	// page; the organiser has dealt with neither yet.
	s.post(coached.link+"/feedback", url.Values{"competition": {compID}, "page": {"/clubs/member/{token}/competitions/{id}/day"},
		"text": {"I couldn't find what time my warm-up is. Is it the time beside my flight?"}, "reply": {coached.name}})
	s.post(chairs+"/feedback", url.Values{"page": {"/competitions/admin/{token}/day"},
		"text": {"On a phone the Started button is right beside Finished: I nearly tapped the wrong one."}})

	// Entry fees: some clubs have paid, some in part.
	s.post(admin+"/fees/settings", url.Values{"fee-": {"12"}, "fee-synchro": {"15"}, "fee-tumbling": {"10"}, "fee-dmt": {"10"}, "club": {"25"},
		"instructions": {s.v.fees}})
	paid := 14
	if s.v.open {
		paid = 3
	}
	payers := regexp.MustCompile(`name="payer" value="([^"]+)"`).FindAllStringSubmatch(s.get(admin+"/fees"), paid)
	for i, m := range payers {
		inv := s.get(admin + "/fees/invoice?payer=" + url.QueryEscape(m[1]))
		due := regexp.MustCompile(`To pay\s*</th>\s*<th class="has-text-right">€(\d+)`).FindStringSubmatch(inv)
		if due == nil {
			continue
		}
		amount, note := due[1], "Bank transfer"
		if i%3 == 2 {
			amount, note = "50", "Part paid at the club fair"
		}
		s.post(admin+"/fees/payments", url.Values{"payer": {m[1]}, "amount": {amount}, "note": {note}})
	}
	if s.err != nil {
		return s.err
	}

	s.made = made{name: s.v.name, admin: admin, id: compID, enter: enter, judges: judges, chairs: chairs, screen: screen,
		runs: runs, owns: owns, ownPage: ownPage, ownSignoff: ownSignoff, ownName: ownName}
	fmt.Fprintf(out, "Made %s: %d club members and 6 gymnasts on their own.\n\n", s.v.name, len(everyone))
	fmt.Fprintf(out, "Organiser (dashboard):  %s\n", admin)
	fmt.Fprintf(out, "Timetable:              %s\n", tt)
	fmt.Fprintf(out, "Officials:              %s/officials\n", admin)
	fmt.Fprintf(out, "Difficulty judges:      %s\n", judges)
	fmt.Fprintf(out, "Chairs of judges:       %s\n", chairs)
	fmt.Fprintf(out, "Venue screen:           %s\n\n", screen)
	fmt.Fprintln(out, "Clubs (club admin pages):")
	for _, c := range clubAdmins {
		fmt.Fprintf(out, "  %s\n", c)
	}
	for _, r := range runs[0].roles {
		fmt.Fprintf(out, "%-24s%s (%s): %s\n", "A "+strings.ToLower(r.role)+":", r.person, runs[0].name, r.link)
	}
	fmt.Fprintf(out, "\nA member:               %s\n", firstMember)
	fmt.Fprintf(out, "Their competition:      %s\n", strings.SplitN(firstMember, ": ", 2)[1]+"/competitions/"+compID+"/day")
	fmt.Fprintf(out, "A coach:                %s\n", firstCoach)
	fmt.Fprintf(out, "A member, coach too:    %s (%s): %s\n", coachToo.name, first.name, coachToo.link)
	fmt.Fprintf(out, "A member, comp sec too: %s (%s): %s\n", compSec.name, first.name, compSec.link)
	fmt.Fprintf(out, "Entering on their own:  %s\n", firstOwn)
	fmt.Fprintf(out, "Their trampoline entry: %s\n", firstOwnEntry)
	if s.pages != nil {
		page := func(kind, path string) { fmt.Fprintf(s.pages, "%s %s\n", kind, path) }
		page("organiser", admin)
		page("day", admin+"/day")
		page("screen", admin+"/screen")
		for _, run := range runs {
			page("club", run.admin)
			for _, link := range run.coaches {
				page("coach", link)
			}
			for _, m := range run.members {
				page("member", m.link+"/competitions/"+compID+"/day")
			}
		}
		for _, own := range owns {
			page("own", own+"/day")
		}
	}
	return nil
}
