package web

import (
	"cmp"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// All people, for the organiser (roadmap: all people): everyone at a
// competition once, whether they compete, officiate, coach or all three, and
// a page for each with what they're doing, like their own My competition page
// but read-only.

func (p *competitionPages) registerPeople(handle func(string, http.HandlerFunc)) {
	handle("GET /competitions/admin/{token}/people", p.people)
	handle("GET /competitions/admin/{token}/people/{key}", p.person)
}

// personPath is a person's page, for a competition's admin path. Their key
// has a colon, and may have a slash or a space in a name, so it's escaped as
// one path segment; the mux gives it back unescaped.
func personPath(base, key string) string { return base + "/people/" + url.PathEscape(key) }

// dirPerson is one person at a competition, with everything the entries, the
// officials and the coaches say about them.
type dirPerson struct {
	key, name, club string
	entries         []store.Entry    // those they compete in, as gymnast or partner
	officials       []store.Official // what they offered: more than one for an individual on several entries
	coached         []store.Entry    // those they coach
	coachees        []string         // the people they coach, once each
	sent            *store.SentCoach // the coach a club sent to be approved
	named           *store.EntryCoach
}

// directory is everyone at a competition, by name, with entries' people.
type directory struct {
	list   []*dirPerson
	byKey  map[string]*dirPerson
	keys   map[string][]string // each entry's people, as personKeys
	groups map[string]string   // key to person (store.PersonGroups)
}

// newDirectory finds everyone in a competition's live entries, officials and
// coaches, each once under their person key. A member who competes and judges
// is one person; a club's coach is one by their record, an individual's by
// name. Keys of one person (ADR
// 0010: groups, key to person), such as a member who is a coach too, are one
// row, under the first key found, and each key finds it.
func newDirectory(entries []store.Entry, officials []store.Official, sent []store.SentCoach, named []store.EntryCoach, groups map[string]string) *directory {
	d := &directory{byKey: map[string]*dirPerson{}, keys: personKeys(entries), groups: groups}
	byPerson := map[string]*dirPerson{}
	get := func(key string) *dirPerson {
		if dp := d.byKey[key]; dp != nil {
			return dp
		}
		person, known := groups[key]
		if dp := byPerson[person]; known && dp != nil {
			d.byKey[key] = dp
			return dp
		}
		dp := &dirPerson{key: key}
		d.byKey[key] = dp
		d.list = append(d.list, dp)
		if known {
			byPerson[person] = dp
		}
		return dp
	}
	for _, e := range entries {
		for i, k := range d.keys[e.ID] {
			dp := get(k)
			dp.entries = append(dp.entries, e)
			if i == 0 {
				dp.name, dp.club = e.Entry.Gymnast, clubOf(e)
			}
		}
	}
	// A partner with no entry of their own is only known by their partner's.
	for _, e := range entries {
		if ks := d.keys[e.ID]; len(ks) > 1 && e.Entry.Partner != nil {
			if dp := d.byKey[ks[1]]; dp.name == "" {
				dp.name = e.Entry.Partner.Name
				if !e.Individual {
					dp.club = e.ClubName
				} else {
					dp.club = e.Entry.Partner.Club
				}
			}
		}
	}
	for _, o := range officials {
		dp := get(officialKey(o, d.keys))
		dp.officials = append(dp.officials, o)
		if dp.name == "" {
			dp.name = o.Name
		}
		if dp.club == "" {
			dp.club = o.ClubName
		}
	}
	coach := func(key, name, club string) *dirPerson {
		dp := get(key)
		if dp.name == "" {
			dp.name = name
		}
		if dp.club == "" {
			dp.club = club
		}
		return dp
	}
	coaching := func(dp *dirPerson, e store.Entry) {
		if slices.ContainsFunc(dp.coached, func(c store.Entry) bool { return c.ID == e.ID }) {
			return
		}
		dp.coached = append(dp.coached, e)
		for _, k := range d.keys[e.ID] {
			if !slices.Contains(dp.coachees, k) {
				dp.coachees = append(dp.coachees, k)
			}
		}
	}
	// Coaches sent to the competition first, so they're named as their club
	// names them, not as a sign-off was typed.
	for i := range sent {
		if k := store.CoachKey(sent[i].CoachID); k != "" {
			coach(k, sent[i].Name, sent[i].ClubName).sent = &sent[i]
		}
	}
	byID := entriesByID(entries)
	for _, e := range entries {
		if k := e.CoachKey(); k != "" {
			coaching(coach(k, e.CoachName(), e.ClubName), e)
		}
	}
	// A coach who signed an entry off though the competition hasn't approved
	// them is someone at it too, to see before approving them.
	for _, e := range entries {
		if k := e.SignerKey(); k != "" && e.Unapproved {
			coaching(coach(k, e.SignedBy, e.ClubName), e)
		}
	}
	for i := range named {
		if k := store.NamedCoachKey(named[i].Name); k != "" {
			dp := coach(k, named[i].Name, "")
			dp.named = &named[i]
			if e, ok := byID[named[i].EntryID]; ok {
				coaching(dp, e)
			}
		}
	}
	slices.SortStableFunc(d.list, func(a, b *dirPerson) int {
		if x := competitions.CompareNames(a.name, b.name); x != 0 {
			return x
		}
		return strings.Compare(a.key, b.key)
	})
	return d
}

// directoryOf is everyone at a competition.
func (p *competitionPages) directoryOf(r *http.Request, c store.Competition, entries []store.Entry) (*directory, error) {
	officials, err := p.st.Officials(r.Context(), c.ID)
	if err != nil {
		return nil, err
	}
	sent, err := p.st.CompetitionCoaches(r.Context(), c.ID)
	if err != nil {
		return nil, err
	}
	named, err := p.st.CompetitionEntryCoaches(r.Context(), c.ID)
	if err != nil {
		return nil, err
	}
	groups, err := p.st.PersonGroups(r.Context(), c.ID)
	if err != nil {
		return nil, err
	}
	return newDirectory(entries, officials, sent, named, groups), nil
}

// events are the events someone competes in, each once, e.g. "Trampoline
// BUCS L7".
func (dp *dirPerson) events() []string {
	var out []string
	// In the usual order of disciplines, whatever order they came in.
	entries := slices.Clone(dp.entries)
	slices.SortStableFunc(entries, func(a, b store.Entry) int {
		return cmp.Compare(slices.Index(competitions.AllDisciplines, a.Entry.Discipline), slices.Index(competitions.AllDisciplines, b.Entry.Discipline))
	})
	for _, e := range entries {
		ev := competitions.DisciplineName(e.Entry.Discipline) + " " + e.Entry.Level
		if !slices.Contains(out, ev) {
			out = append(out, ev)
		}
	}
	return out
}

// offered is what they offered to officiate, over everything they sent:
// whether they judge, can chair, and are a recorder or marshal.
func (dp *dirPerson) offered() (judge, chair, recorder, marshal bool) {
	for _, o := range dp.officials {
		judge = judge || len(o.Offer.Judge) > 0
		for _, j := range o.Offer.Judge {
			chair = chair || j.Chair
		}
		recorder, marshal = recorder || o.Offer.Recorder, marshal || o.Offer.Marshal
	}
	return judge, chair, recorder, marshal
}

// coaches says whether they coach, as a coach a club sent, one an individual
// named, or one entries name.
func (dp *dirPerson) coaches() bool {
	return len(dp.coached) > 0 || dp.sent != nil || dp.named != nil
}

// roles are what they do, as words to filter by (gymnast, judge, chair,
// recorder, marshal, coach), and as the tags on their page.
func (dp *dirPerson) roles() (words, tags []string) {
	add := func(word, tag string) { words, tags = append(words, word), append(tags, tag) }
	if len(dp.entries) > 0 {
		add("gymnast", "Gymnast")
	}
	judge, chair, recorder, marshal := dp.offered()
	if judge {
		add("judge", "Judge")
	}
	if chair {
		add("chair", "Can chair")
	}
	if recorder {
		add("recorder", "Recorder")
	}
	if marshal {
		add("marshal", "Marshal")
	}
	if dp.coaches() {
		add("coach", "Coach")
	}
	return words, tags
}

// does says what they do in a line, e.g. "Trampoline BUCS L7, Synchro BUCS L6
// · judge, can chair · coaches 6".
func (dp *dirPerson) does() string {
	var parts []string
	if events := dp.events(); len(events) > 0 {
		parts = append(parts, strings.Join(events, ", "))
	}
	judge, chair, recorder, marshal := dp.offered()
	var help []string
	for _, h := range []struct {
		on   bool
		word string
	}{{judge, "judge"}, {chair, "can chair"}, {recorder, "recorder"}, {marshal, "marshal"}} {
		if h.on {
			help = append(help, h.word)
		}
	}
	if len(help) > 0 {
		parts = append(parts, strings.Join(help, ", "))
	}
	switch {
	case len(dp.coachees) > 0:
		parts = append(parts, "coaches "+views.Count(len(dp.coachees), "gymnast", "gymnasts"))
	case dp.coaches():
		parts = append(parts, "coach")
	}
	return strings.Join(parts, " · ")
}

// people lists everyone at a competition by name, each linking to their page.
func (p *competitionPages) people(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	all, err := p.entriesOf(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	dir, err := p.directoryOf(r, c, live(all))
	if err != nil {
		failed(w, r, err)
		return
	}
	base := adminPath(r.PathValue("token"))
	page := views.PeoplePage{Competition: summary(c, p.now()), Menu: p.menuFor(r, c, menuPeople)}
	var clubs []string
	for _, dp := range dir.list {
		clubs = append(clubs, dp.club)
	}
	page.Tools = views.ListTools{ID: "people", Noun: "people", Total: len(dir.list), Filters: []views.ListFilter{clubFilter(clubs), peopleDoFilter()}}
	for _, dp := range dir.list {
		words, _ := dp.roles()
		page.People = append(page.People, views.PersonRow{Name: dp.name, Club: dp.club, Href: personPath(base, dp.key), Does: dp.does(),
			List: views.ListRow{Name: dp.name, Values: map[string][]string{"club": {clubValue(dp.club)}, "does": peopleDo(words)}}})
	}
	render(w, r, views.People(page))
}

// person is one person's page for the organiser: everything on their own My
// competition page but what they do themselves (telling the organisers
// they're here, hearing about changes, the calendar), and what they offer,
// coach, and where they are on the day.
func (p *competitionPages) person(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	key := r.PathValue("key")
	page, f, err := p.dayOf(r, c, key, "")
	if err != nil {
		failed(w, r, err)
		return
	}
	dir, err := p.directoryOf(r, c, f.entries)
	if err != nil {
		failed(w, r, err)
		return
	}
	dp := dir.byKey[key]
	if dp == nil {
		message(w, r, http.StatusNotFound, "Person not found", "Nobody with that link is at this competition now. They may have been taken off it: go back to the list of people.")
		return
	}
	base := adminPath(r.PathValue("token"))
	if dp.key != key { // another of their keys, such as a member's coach (ADR 0010)
		http.Redirect(w, r, personPath(base, dp.key), http.StatusFound)
		return
	}
	page.Name, page.Back, page.BackLabel, page.Menu = dp.name, base+"/people", "← People", p.menuFor(r, c, menuPeople)
	_, tags := dp.roles()
	page.Admin = &views.PersonAdmin{Club: dp.club, Roles: tags}
	byID := entriesByID(f.entries)
	for i, ev := range page.Events {
		ev.Link = base + "/entries/" + ev.ID
		if k := byID[ev.ID].CoachKey(); k != "" && dir.byKey[k] != nil {
			ev.Coach, ev.CoachLink = dir.byKey[k].name, personPath(base, k)
		}
		if ev.Flight != "" {
			switch f.checkins[ev.ID].Status {
			case store.CheckedIn:
				ev.Checkin = "Here"
			case store.Scratched:
				ev.Checkin = "Scratched"
			default:
				ev.Checkin = "Not marked yet"
			}
		}
		page.Events[i] = ev
	}
	page.Admin.Officiating = p.officiatingOf(c, dp, f, dir.groups)
	if dp.coaches() {
		page.Admin.Coaching = coachingOf(c, dp, dir, base)
	}
	page.Admin.Scratch = scratchOf(c, key, f)
	people, clubs, staff := deskReach(dp)
	page.Desk = p.deskNotes(r.Context(), c.ID, people, clubs, staff)
	render(w, r, views.MyCompetition(page))
}

// deskReach are who a desk message can be sent to for it to reach someone:
// the people (themselves, and for a coach the gymnasts they coach, as on a
// coach's own page), the clubs (a member's club) and the staff clubs (a
// coach's club, for its coaches).
func deskReach(dp *dirPerson) (people, clubs, staff []string) {
	people = []string{dp.key}
	if parts := strings.SplitN(dp.key, ":", 3); parts[0] == "c" && len(parts) == 3 {
		return append(people, dp.coachees...), clubIDs(parts[1]), clubIDs(parts[1])
	}
	for _, e := range dp.entries {
		if !e.Individual && e.ClubID != "" && !slices.Contains(clubs, e.ClubID) {
			clubs = append(clubs, e.ClubID)
		}
	}
	return people, clubs, nil
}

// judgeRule says in words who may judge what, under a competition's rule.
func judgeRule(rule string) string {
	switch rule {
	case competitions.JudgeBelow:
		return "only levels below the one they compete at"
	case competitions.JudgeQualified:
		return "only people you mark qualified"
	}
	return "anyone, up to the level they say"
}

// officiatingOf is what someone offered to officiate, which events they may
// judge, and their seats, planned or published; nil for someone who offered
// nothing and has no seat.
func (p *competitionPages) officiatingOf(c store.Competition, dp *dirPerson, f dayFacts, groups map[string]string) *views.PersonOfficiating {
	var seats []string
	note := "No timetable is planned yet."
	switch s := schedule(c); {
	case c.Published != nil:
		seats, note = dutiesOf(c, dp.key), "from the published timetable"
		if len(seats) == 0 {
			note = "Not on a panel in the published timetable."
		}
	case s.Planned:
		seats, note = dutiesIn(s, dp.key), "planned, not published yet"
		if len(seats) == 0 {
			note = "Not on a panel in the planned timetable."
		}
	}
	if len(dp.officials) == 0 && len(seats) == 0 {
		return nil
	}
	o := &views.PersonOfficiating{Seats: seats, SeatsNote: note, Rule: judgeRule(c.Officials.Judge)}
	qualified := false
	for _, off := range dp.officials {
		source := "sent by their club"
		switch {
		case off.Added:
			source = "added by you"
		case off.EntryID != "":
			source = "entering on their own"
		}
		offer := off.Offer.Describe()
		if offer == "" {
			offer = "Nothing offered"
		}
		o.Offers = append(o.Offers, offer+" · "+source)
		qualified = qualified || off.Qualified
	}
	if len(dp.officials) == 0 {
		o.Offers = []string{"Not on the officials list now"}
	}
	if c.Officials.Judge == competitions.JudgeQualified {
		o.Qualified = "Not marked qualified yet: mark them on the Officials page."
		if qualified {
			o.Qualified = "Marked qualified."
		}
	}
	for _, rp := range rotaPeople(c.Competition, dp.officials, f.entries, groups) {
		if rp.Key != dp.key {
			continue
		}
		var events []string
		for _, ev := range c.EventNames() {
			if rp.Judge[ev] {
				if rp.Chair[ev] {
					ev += " (can chair)"
				}
				events = append(events, ev)
			}
		}
		o.Judges = strings.Join(events, ", ")
	}
	if c.Published != nil {
		o.Panel = panelStatus(*c.Published, dp.key, f.official)
	}
	return o
}

// panelStatus says whether someone is here for the panels they sit on, in
// words; "" if they sit on none.
func panelStatus(s competitions.Schedule, person string, official map[string]map[string]store.Checkin) string {
	here, missing, total := 0, 0, 0
	for _, fl := range s.Flights {
		if !holdsSeat(fl, person) {
			continue
		}
		total++
		switch official[competitions.FlightKey(fl)][person].Status {
		case store.OfficialHere:
			here++
		case store.OfficialMissing:
			missing++
		}
	}
	switch {
	case total == 0:
		return ""
	case here == total:
		return "Here"
	case missing == total:
		return "Missing"
	case here == 0 && missing == 0:
		return "Not marked yet"
	}
	out := fmt.Sprintf("Here for %d of %s", here, views.Count(total, "flight", "flights"))
	if missing > 0 {
		out += fmt.Sprintf(", missing for %d", missing)
	}
	return out
}

// coachingOf is who someone coaches, and whether the competition has approved
// them to sign off, where it approves coaches.
func coachingOf(c store.Competition, dp *dirPerson, dir *directory, base string) *views.PersonCoaching {
	k := &views.PersonCoaching{}
	if c.Signoff && c.ApproveCoaches {
		switch {
		case dp.sent != nil:
			k.Approval = "Sent by their club: " + coachStatus(dp.sent.Status) + "."
		case dp.named != nil:
			k.Approval = "Named by the gymnast they coach: " + coachStatus(dp.named.Status) + "."
		default:
			k.Approval = "Not sent for approval."
		}
	}
	for _, e := range dp.coached {
		pc := views.PersonCoached{Event: competitions.DisciplineName(e.Entry.Discipline) + " " + e.Entry.Level, Entry: base + "/entries/" + e.ID}
		// Where its sign-off stands, where the competition asks for one.
		if c.Signoff {
			pc.Signoff = storedSignoff(c.Competition, e)
		}
		for _, key := range dir.keys[e.ID] {
			if g := dir.byKey[key]; g != nil {
				pc.People = append(pc.People, views.PersonLink{Name: g.name, Href: personPath(base, key)})
			}
		}
		k.Entries = append(k.Entries, pc)
	}
	return k
}

// scratchOf says, for someone scratched from an event of a published
// timetable, what they're still down for on the day.
func scratchOf(c store.Competition, key string, f dayFacts) []string {
	t := c.Published
	if t == nil {
		return nil
	}
	var officiating, competing []string
	for day := range t.Setup.Days {
		for _, w := range scratchWarnings(*t, day, f.entries, f.checkins, f.official, f.clears, "") {
			if w.Person == key {
				officiating, competing = append(officiating, w.Officiating...), append(competing, w.Competing...)
			}
		}
	}
	var out []string
	if len(officiating) > 0 {
		out = append(out, "Scratched, but still down to officiate: "+strings.Join(officiating, "; ")+".")
	}
	if len(competing) > 0 {
		out = append(out, "Scratched, but still down to compete in: "+strings.Join(competing, "; ")+".")
	}
	return out
}
