package web

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// People (roadmap: all people, for the organiser): everyone at a competition
// once, whether they compete, officiate or coach, and a page for each that
// shows what they're doing, as they would see it on their own page.

func (p *competitionPages) registerPeople(handle func(string, http.HandlerFunc)) {
	handle("GET /competitions/admin/{token}/people", p.people)
	handle("GET /competitions/admin/{token}/people/{key}", p.person)
}

// personInfo is one person, with everything the competition holds about them.
type personInfo struct {
	key, name, club, clubID string
	individual              bool
	entries                 []store.Entry    // the ones they compete in
	offers                  []store.Official // what they offered, if anything
	coaching                []store.Entry    // the entries they coach
	sent                    *store.SentCoach // the club sent them for approval
	named                   *store.EntryCoach
}

// personPath is a person's page, under the admin link. A key has colons and
// spaces (and could have anything else a name can), so it's escaped as one
// path segment.
func personPath(base, key string) string { return base + "/people/" + url.PathEscape(key) }

// nameKey is a name as keys write it: lower case, one space between words.
func nameKey(name string) string { return strings.ToLower(strings.Join(strings.Fields(name), " ")) }

// peopleIn are the people of a competition, by name: each key of personKeys
// (gymnasts and partners), each official, and each coach, whether named on an
// entry or sent or named for approval.
func peopleIn(entries []store.Entry, officials []store.Official, sent []store.SentCoach, named []store.EntryCoach) []*personInfo {
	keys := personKeys(entries)
	byKey := map[string]*personInfo{}
	var all []*personInfo
	get := func(key string) *personInfo {
		if byKey[key] == nil {
			byKey[key] = &personInfo{key: key}
			all = append(all, byKey[key])
		}
		return byKey[key]
	}
	for _, e := range entries {
		for i, k := range keys[e.ID] {
			who := get(k)
			who.entries = append(who.entries, e)
			name, club := e.Entry.Gymnast, e.ClubName
			if i == 1 && e.Entry.Partner != nil {
				name, club = e.Entry.Partner.Name, e.Entry.Partner.Club
				if club == "" && strings.HasPrefix(k, "m:") {
					club = e.ClubName
				}
			}
			if who.name == "" || i == 0 {
				who.name = name
			}
			if who.club == "" {
				who.club = club
			}
			if who.clubID == "" && i == 0 {
				who.clubID = e.ClubID
			}
			who.individual = who.individual || e.Individual && i == 0
		}
		if k := coachKey(e); k != "" {
			who := get(k)
			who.coaching = append(who.coaching, e)
			who.name, who.club, who.clubID = e.CoachName(), e.ClubName, e.ClubID
		}
	}
	for _, o := range officials {
		who := get(officialKey(o, keys))
		who.offers = append(who.offers, o)
		if who.name == "" {
			who.name = o.Name
		}
		if who.club == "" {
			who.club = o.ClubName
		}
	}
	for i, s := range sent {
		who := get("c:" + s.ClubID + ":" + nameKey(s.Name))
		who.sent = &sent[i]
		if who.name == "" {
			who.name, who.club, who.clubID = s.Name, s.ClubName, s.ClubID
		}
	}
	for i, n := range named {
		who := get("c::" + nameKey(n.Name))
		who.named = &named[i]
		if who.name == "" {
			who.name = n.Name
		}
	}
	slices.SortStableFunc(all, func(a, b *personInfo) int {
		if c := strings.Compare(strings.ToLower(a.name), strings.ToLower(b.name)); c != 0 {
			return c
		}
		return strings.Compare(a.key, b.key)
	})
	return all
}

// events says what they compete in, e.g. "Trampoline BUCS L7, Synchro L6/L7".
func (w *personInfo) events() string {
	var out []string
	for _, e := range w.entries {
		s := competitions.DisciplineName(e.Entry.Discipline) + " " + e.Entry.Level
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return strings.Join(out, ", ")
}

// officiating says what they offered, in a few words: "judge, can chair",
// "recorder"; "" for nothing.
func (w *personInfo) officiating() string {
	var judge, chair, recorder, marshal bool
	for _, o := range w.offers {
		judge = judge || len(o.Offer.Judge) > 0
		for _, j := range o.Offer.Judge {
			chair = chair || j.Chair
		}
		recorder = recorder || o.Offer.Recorder
		marshal = marshal || o.Offer.Marshal
	}
	var out []string
	for _, f := range []struct {
		on   bool
		what string
	}{{judge, "judge"}, {chair, "can chair"}, {recorder, "recorder"}, {marshal, "marshal"}} {
		if f.on {
			out = append(out, f.what)
		}
	}
	return strings.Join(out, ", ")
}

// coaches says how many entries they coach, e.g. "coaches 6 entries".
func (w *personInfo) coaches() string {
	switch n := len(w.coaching); {
	case n == 1:
		return "coaches 1 entry"
	case n > 1:
		return fmt.Sprintf("coaches %d entries", n)
	case w.sent != nil || w.named != nil:
		return "coach"
	}
	return ""
}

// does is what they do, as a line of the list.
func (w *personInfo) does() string {
	var out []string
	for _, s := range []string{w.events(), w.officiating(), w.coaches()} {
		if s != "" {
			out = append(out, s)
		}
	}
	return strings.Join(out, " · ")
}

// roles are the kinds of thing they do, for finding them later.
func (w *personInfo) roles() []string {
	var out []string
	if len(w.entries) > 0 {
		out = append(out, "compete")
	}
	if len(w.offers) > 0 {
		out = append(out, "official")
	}
	if w.coaches() != "" {
		out = append(out, "coach")
	}
	return out
}

// people is the list of everyone.
func (p *competitionPages) people(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	who, ok := p.peopleOf(w, r, c)
	if !ok {
		return
	}
	base := adminPath(r.PathValue("token"))
	page := views.PeoplePage{Base: base, Competition: summary(c.Competition, p.now()), Menu: p.menuFor(r, c, menuPeople)}
	for _, x := range who {
		club := x.club
		if club == "" && x.individual {
			club = "individual"
		}
		page.People = append(page.People, views.PersonRow{Name: x.name, Club: club, Does: x.does(), Roles: strings.Join(x.roles(), " "), Href: personPath(base, x.key)})
	}
	render(w, r, views.People(page))
}

// peopleOf are a competition's people, and the live entries they are from.
func (p *competitionPages) peopleOf(w http.ResponseWriter, r *http.Request, c store.Competition) ([]*personInfo, bool) {
	all, err := p.st.Entries(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return nil, false
	}
	officials, err := p.st.Officials(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return nil, false
	}
	sent, err := p.st.CompetitionCoaches(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return nil, false
	}
	named, err := p.st.CompetitionEntryCoaches(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return nil, false
	}
	return peopleIn(live(all), officials, sent, named), true
}

// person is one person's page: what they compete in, officiate and coach, and
// once the timetable is published how their day is going.
func (p *competitionPages) person(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	everyone, ok := p.peopleOf(w, r, c)
	if !ok {
		return
	}
	i := slices.IndexFunc(everyone, func(x *personInfo) bool { return x.key == r.PathValue("key") })
	if i < 0 {
		failed(w, r, store.ErrNotFound)
		return
	}
	who := everyone[i]
	all, err := p.st.Entries(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	entries := live(all)
	base := adminPath(r.PathValue("token"))
	page, err := p.dayPage(r, c, entries, who.key, who.clubID, who.name, false)
	if err != nil {
		failed(w, r, err)
		return
	}
	page.Back, page.BackLabel = base+"/people", "← People"
	page.Menu = p.menuFor(r, c, menuPeople)
	v := &views.PersonView{Club: who.club, Offer: officialOffer(who)}
	if who.club == "" && who.individual {
		v.Club = "Individual"
	}
	if len(who.entries) > 0 {
		v.Roles = append(v.Roles, "Competes")
	}
	if len(who.offers) > 0 {
		v.Roles = append(v.Roles, "Official")
	}
	if who.coaches() != "" {
		v.Roles = append(v.Roles, "Coach")
	}
	for i := range page.Events {
		page.Events[i].Link = base + "/entries/" + page.Events[i].ID
	}
	if c.Officials.Judge == competitions.JudgeQualified && v.Offer != "" {
		v.Qualified = "Not marked qualified to judge yet."
		if slices.ContainsFunc(who.offers, func(o store.Official) bool { return o.Qualified }) {
			v.Qualified = "Marked qualified to judge."
		}
	}

	// Seats: the published timetable's, or else the planned one's.
	v.Seats, page.Duties = page.Duties, nil
	if s := schedule(c); c.Published == nil && s.Planned {
		v.Seats, v.SeatsNote = dutiesIn(s, who.key), "· planned, not published yet"
	}

	// Coaching: their gymnasts, each with their own page.
	keys := personKeys(entries)
	for _, e := range who.coaching {
		g := views.PersonCoached{Name: e.Entry.Gymnasts(), Event: e.Entry.Event()}
		if k := keys[e.ID]; len(k) > 0 {
			g.Href = personPath(base, k[0])
		}
		v.Coaching = append(v.Coaching, g)
	}
	switch {
	case who.sent != nil:
		v.Approval = "Sent by their club for approval: " + coachStatus(who.sent.Status) + "."
	case who.named != nil:
		v.Approval = "Named by the gymnast for approval: " + coachStatus(who.named.Status) + "."
	}

	if c.Published != nil {
		if err := p.checkinLines(r, c, entries, who, keys, &page, v); err != nil {
			failed(w, r, err)
			return
		}
	}
	page.Person = v
	render(w, r, views.MyCompetition(page))
}

// officialOffer is what they offered to officiate, in full.
func officialOffer(w *personInfo) string {
	var out []string
	for _, o := range w.offers {
		if s := o.Offer.Describe(); s != "" && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return strings.Join(out, "; ")
}

// checkinLines fills in how their check-in is going: each of their events,
// their panel seats, and whether they are scratched with the rest still to
// say.
func (p *competitionPages) checkinLines(r *http.Request, c store.Competition, entries []store.Entry, who *personInfo, keys map[string][]string, page *views.MyCompetitionPage, v *views.PersonView) error {
	checkins, err := p.st.Checkins(r.Context(), c.ID)
	if err != nil {
		return err
	}
	official, err := p.st.OfficialCheckins(r.Context(), c.ID)
	if err != nil {
		return err
	}
	clears, err := p.st.ScratchClears(r.Context(), c.ID)
	if err != nil {
		return err
	}
	for i, ev := range page.Events {
		if ev.Flight == "" {
			continue
		}
		switch checkins[ev.ID].Status {
		case store.CheckedIn:
			page.Events[i].Here = "Checked in: here."
		case store.Scratched:
			page.Events[i].Here = "Scratched."
		default:
			page.Events[i].Here = "Not checked in yet."
		}
	}
	var here, missing, seats int
	for _, f := range c.Published.Staffed() {
		for _, d := range f.Officials {
			if d.Person != who.key {
				continue
			}
			seats++
			switch official[competitions.FlightKey(f)][who.key].Status {
			case store.OfficialHere:
				here++
			case store.OfficialMissing:
				missing++
			}
		}
	}
	if seats > 0 {
		v.Panel = fmt.Sprintf("Panel check-in: here for %d of %d seats, missing for %d, not marked for %d.", here, seats, missing, seats-here-missing)
	}
	if box := hereBox(entries, keys, who.key, seats > 0, checkins, official, clears, ""); box != nil {
		v.Warning = fmt.Sprintf("Scratched from %s. Nobody has told the organisers whether they are still here for %s.", box.Scratched, strings.ReplaceAll(box.For, "your ", "their "))
	}
	return nil
}
