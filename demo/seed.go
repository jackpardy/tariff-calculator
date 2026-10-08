package demo

import (
	"fmt"
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
	s := &seeder{h: h, rng: rand.New(rand.NewPCG(seed, 99))}
	if err := s.run(out); err != nil {
		return err
	}
	return nil
}

type seeder struct {
	h    http.Handler
	rng  *rand.Rand
	ip   int
	err  error
	vols map[string][]string // passing voluntaries, by level and exercise ("BUCS L3/2")
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

// clubRun is a club as the demo made it: its admin link, coaches' links by
// name, and members.
type clubRun struct {
	name, admin string
	coaches     map[string]string
	members     []person
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

// levelIndex is a trampoline level's place in levels, easiest first.
func levelIndex(name string) int {
	return slices.IndexFunc(levels, func(l levelShare) bool { return l.name == name })
}

func (s *seeder) level() string {
	r := s.rng.IntN(100)
	for _, l := range levels {
		if r < l.weight {
			return l.name
		}
		r -= l.weight
	}
	return levels[0].name
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
// (or a voluntary, at L1 and L2) and a voluntary that passes, or now and
// then one that doesn't, as happens.
func (s *seeder) routine(form url.Values, level string, careless bool) {
	form.Set("level", level)
	form.Set("ex1Option", option(level, 0, 1+s.rng.IntN(2)))
	form.Set("ex2Option", option(level, 1, 0))
	for ex := range 2 {
		if ex == 0 && level != "BUCS L1" && level != "BUCS L2" {
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

func (s *seeder) run(out io.Writer) error {
	// Voluntaries that pass, for every level's voluntary exercises.
	var c competitions.Competition
	for _, l := range levels {
		c.Levels = append(c.Levels, competitions.Level{Ref: "builtin-level:bucs-" + strings.ToLower(strings.TrimPrefix(l.name, "BUCS "))})
	}
	s.vols = map[string][]string{}
	for _, l := range levels {
		for ex := range 2 {
			if ex == 0 && l.name != "BUCS L1" && l.name != "BUCS L2" {
				continue
			}
			opts := [2]string{option(l.name, 0, 1), option(l.name, 1, 0)}
			s.vols[fmt.Sprintf("%s/%d", l.name, ex+1)] = voluntaries(c, l.name, ex, opts, 8, 30000, s.rng)
		}
	}

	// The organiser creates the competition.
	form := url.Values{
		"name": {"ISTO 2027 (demo)"}, "date": {"2027-02-27"}, "deadlineDate": {"2027-02-13"}, "deadlineTime": {"23:59"},
		"individuals": {"1"}, "signoff": {"1"},
		// Synchro on three panels: three events, levels grouped.
		"synchroPairs": {"BUCS L6 + BUCS L7\nBUCS L4 + BUCS L5\nBUCS L1 + BUCS L2 + BUCS L3"},
		"tumbling":     {"Novice\nIntermediate\nAdvanced"}, "dmt": {"Novice\nAdvanced"},
	}
	for _, l := range levels {
		ref := "builtin-level:bucs-" + strings.ToLower(strings.TrimPrefix(l.name, "BUCS "))
		form.Add("level", ref)
		form.Add("synchroLevel", ref)
	}
	admin := s.post("/competitions", form)
	// Men and women rank apart, except in synchro, where pairs can be mixed.
	split := url.Values{"split": {competitions.SplitSome}}
	for _, l := range levels {
		split.Add("splitLevel", l.name)
	}
	for _, ev := range []string{"Tumbling Novice", "Tumbling Intermediate", "Tumbling Advanced", "DMT Novice", "DMT Advanced"} {
		split.Add("splitLevel", ev)
	}
	s.post(admin+"/split", split)
	dash := s.get(admin)
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

	used := map[string]bool{}
	var clubAdmins []string
	var everyone []person
	var firstMember, firstCoach string
	var runs []clubRun
	for _, cl := range clubs {
		club := s.post("/clubs", url.Values{"name": {cl.name}})
		clubAdmins = append(clubAdmins, cl.name+": "+club)
		s.post(clubLink, url.Values{"clubAdmin": {club}})
		page := s.get(club)
		join := s.link(page, "/clubs/join/")
		// Coaches.
		coachLinks := map[string]string{}
		for _, name := range cl.coaches {
			rec := s.do(http.MethodPost, club+"/coaches", url.Values{"name": {name}})
			coachLinks[name] = s.link(rec.Body.String(), "/clubs/coach/")
			if firstCoach == "" {
				firstCoach = name + " (" + cl.name + "): " + coachLinks[name]
			}
		}
		// Members join and enter.
		var members []person
		for range cl.members {
			cat := []string{"Women", "Men"}[s.rng.IntN(2)]
			m := person{name: s.name(used, cat), category: cat, level: s.level(), club: cl.name}
			m.link = s.post(join, url.Values{"name": {m.name}})
			page := s.get(m.link)
			if compID == "" {
				if id := regexp.MustCompile(regexp.QuoteMeta(m.link) + `/competitions/([A-Za-z0-9_-]+)`).FindStringSubmatch(page); id != nil {
					compID = id[1]
				} else {
					s.fail("no competition on %s's page", m.name)
				}
			}
			if firstMember == "" {
				firstMember = m.name + " (" + cl.name + "): " + m.link
			}
			m.coach = cl.coaches[s.rng.IntN(len(cl.coaches))]
			if id := regexp.MustCompile(`<option value="([^"]+)">` + regexp.QuoteMeta(m.coach) + `</option>`).FindStringSubmatch(page); id != nil {
				s.post(m.link+"/coach", url.Values{"coach": {id[1]}})
			}
			save := m.link + "/competitions/" + compID
			entry := url.Values{"discipline": {""}, "category": {m.category}}
			s.routine(entry, m.level, s.rng.IntN(100) < 7)
			s.post(save, entry)
			if s.rng.IntN(100) < 28 {
				s.post(save, url.Values{"discipline": {"tumbling"}, "category": {m.category}, "level": {[]string{"Novice", "Novice", "Intermediate", "Advanced"}[s.rng.IntN(4)]}})
			}
			if s.rng.IntN(100) < 16 {
				s.post(save, url.Values{"discipline": {"dmt"}, "category": {m.category}, "level": {[]string{"Novice", "Novice", "Advanced"}[s.rng.IntN(3)]}})
			}
			// What they'll judge or help with.
			offer := url.Values{}
			if s.rng.IntN(100) < 55 {
				offer.Set("judge-trampoline", "1")
				offer.Set("upto-trampoline", m.level)
				if m.level == "BUCS L1" || m.level == "BUCS L2" || s.rng.IntN(100) < 8 {
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
			members = append(members, m)
		}
		runs = append(runs, clubRun{name: cl.name, admin: club, coaches: coachLinks, members: members})
		everyone = append(everyone, members...)
		if s.err != nil {
			return s.err
		}
	}

	// Synchro pairs, up to four a club: men and women alike, about a third
	// with a gymnast from another club, two gymnasts no more than a level
	// apart individually, doing the level they share or the easier of the
	// two. Each is confirmed by the partner.
	paired := map[string]bool{}
	partnerFor := func(a person, from []person) (person, bool) {
		for _, b := range from {
			ai, bi := levelIndex(a.level), levelIndex(b.level)
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
			s.routine(entry, levels[min(levelIndex(a.level), levelIndex(b.level))].name, false)
			s.post(a.link+"/competitions/"+compID, entry)
			partner := s.link(s.get(a.link), "/competitions/partner/")
			s.post(partner, url.Values{"link": {"http://example.com" + b.link}})
		}
	}
	// The coaches sign off most entries, and each comp sec sends them all.
	for _, run := range runs {
		for _, coach := range run.coaches {
			page := s.get(coach)
			for _, m := range regexp.MustCompile(`href="(`+regexp.QuoteMeta(coach)+`/members/[^"]+)"`).FindAllStringSubmatch(page, -1) {
				if s.rng.IntN(100) < 88 {
					s.post(strings.ReplaceAll(m[1], "&amp;", "&"), url.Values{"signed": {"1"}, "note": {[]string{"", "", "Good luck!", "Watch your arms in the set"}[s.rng.IntN(4)]}})
				}
			}
		}
		s.post(run.admin+"/competitions/"+compID+"/send", url.Values{"which": {"all"}})
	}
	if s.err != nil {
		return s.err
	}

	// Gymnasts entering on their own, some offering to judge.
	var firstOwn string
	for i := range 6 {
		cat := []string{"Women", "Men"}[i%2]
		p := person{name: s.name(used, cat), category: cat, level: s.level()}
		entry := url.Values{"gymnast": {p.name}, "category": {cat}}
		s.routine(entry, p.level, false)
		own := s.post(enter, entry)
		if firstOwn == "" {
			firstOwn = p.name + ": " + own
		}
		if i%2 == 0 {
			s.post(own+"/offer", url.Values{"judge-trampoline": {"1"}, "upto-trampoline": {p.level}})
		}
		s.post("/competitions/signoff/"+strings.TrimPrefix(s.link(s.get(own), "/competitions/signoff/"), "/competitions/signoff/"),
			url.Values{"coach": {"Own coach"}, "signed": {"1"}})
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
	for i, name := range []string{"Friday", "Saturday", "Sunday"} {
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
	for _, r := range []url.Values{
		{"kind": {"day"}, "must": {"1"}, "event": {"Synchro BUCS L6/L7"}, "day": {"2"}},
		{"kind": {"day"}, "must": {"1"}, "event": {"Synchro BUCS L4/L5"}, "day": {"2"}},
		{"kind": {"day"}, "must": {"1"}, "event": {"Synchro BUCS L1/L2/L3"}, "day": {"2"}},
		{"kind": {"day"}, "must": {"0"}, "event": {"BUCS L1"}, "day": {"2"}},
		{"kind": {"before"}, "must": {"0"}, "event": {"BUCS L7"}, "event2": {"BUCS L6"}},
	} {
		r.Set("add", "1")
		s.post(tt+"/setup/rules", r)
	}
	page := s.get(tt)
	if m := regexp.MustCompile(`<option value="([^"]+)">Gráinne Walsh</option>`).FindStringSubmatch(page); m != nil {
		s.post(tt+"/setup/rules", url.Values{"add": {"1"}, "kind": {"role"}, "must": {"1"}, "person": {m[1]}, "role": {"chair"}, "event": {"BUCS L1"}})
	}
	if m := regexp.MustCompile(`<option value="([^"]+)">Frank Lyons</option>`).FindStringSubmatch(page); m != nil {
		s.post(tt+"/setup/rules", url.Values{"add": {"1"}, "kind": {"hours"}, "must": {"0"}, "person": {m[1]}, "day": {"1"}, "from": {"12:00"}})
	}
	s.post(tt+"/plan", url.Values{})
	s.post(tt+"/publish", url.Values{"on": {"1"}})
	if s.err != nil {
		return s.err
	}

	fmt.Fprintf(out, "Made ISTO 2027 (demo): %d club members and 6 gymnasts on their own.\n\n", len(everyone))
	fmt.Fprintf(out, "Organiser (dashboard):  %s\n", admin)
	fmt.Fprintf(out, "Timetable:              %s\n", tt)
	fmt.Fprintf(out, "Officials:              %s/officials\n\n", admin)
	fmt.Fprintln(out, "Clubs (comp sec pages):")
	for _, c := range clubAdmins {
		fmt.Fprintf(out, "  %s\n", c)
	}
	fmt.Fprintf(out, "\nA member:               %s\n", firstMember)
	fmt.Fprintf(out, "Their competition:      %s\n", strings.SplitN(firstMember, ": ", 2)[1]+"/competitions/"+compID+"/day")
	fmt.Fprintf(out, "A coach:                %s\n", firstCoach)
	fmt.Fprintf(out, "Entering on their own:  %s\n", firstOwn)
	return nil
}
