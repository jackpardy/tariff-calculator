package web

import (
	"cmp"
	"context"
	"fmt"
	"hash/fnv"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// The officials rota on the timetable (ADR 0005 step 5): the people who've
// offered to officiate, put on each flight's panel when the timetable is
// planned, changed seat by seat, and printed.

// officialKey is an official's person key: the same as where they compete,
// so the rota keeps them off panels while they're competing. An individual's
// offer on an entry that isn't live (held, or removed while another of theirs
// is live) has its entry's key, so they keep their seats.
func officialKey(o store.Official, keys map[string][]string) string {
	switch {
	case o.MemberID != "":
		return store.MemberKey(o.MemberID)
	case o.EntryID != "":
		if k := keys[o.EntryID]; len(k) > 0 {
			return k[0]
		}
		if o.EntryKey != "" {
			return o.EntryKey
		}
		return store.IndividualNameKey(o.Name)
	}
	return store.OfficialKey(o.ID)
}

// rotaPeople are the people who can officiate, as the rota needs them: each
// once (an individual offering on two entries is one person), with what they
// may do at each event under the competition's judging rule.
func rotaPeople(c competitions.Competition, officials []store.Official, entries []store.Entry) []competitions.RotaPerson {
	keys := personKeys(entries)
	var out []competitions.RotaPerson
	at := map[string]int{}
	for _, o := range officials {
		key := officialKey(o, keys)
		i, seen := at[key]
		if !seen {
			at[key] = len(out)
			i = len(out)
			out = append(out, competitions.RotaPerson{Key: key, Name: o.Name, Club: o.ClubName})
		}
		p := &out[i]
		if words := o.Offer.Describe(); p.Offer == "" {
			p.Offer = words
		} else if words != "" && !strings.Contains(p.Offer, words) {
			p.Offer += "; " + words
		}
		p.Unqualified = c.Officials.Judge == competitions.JudgeQualified && !o.Qualified && (!seen || p.Unqualified)
		p.Recorder = p.Recorder || o.Offer.Recorder
		p.Marshal = p.Marshal || o.Offer.Marshal
		for _, d := range c.Disciplines() {
			roles := append(slices.Clone(competitions.LevelRoles), competitions.Measured(d)...)
			for _, l := range c.LevelNames(d) {
				for _, role := range roles {
					if c.CanJudge(o.Offer, o.Qualified, d, l, role) {
						p.Allow(role, competitions.EventName(d, l))
					}
				}
			}
		}
	}
	slices.SortStableFunc(out, func(a, b competitions.RotaPerson) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// rotaFacts are a competition's people as its pages read them: the live
// entries, the officials as the rota needs them, and who is who.
type rotaFacts struct {
	entries   []store.Entry             // the live entries
	officials []competitions.RotaPerson // as the rota needs them
	same      samePeople                // who's the same person among them (newSamePeople)
	people    map[string][]string       // each entry's people, by id (peopleOf)
	names     map[string]string         // each person's name, by key (peopleOf)
	one       map[string][]string       // each entry's people, each keyed as one person (clashPeople)
	oneNames  map[string]string         // names for those keys too (clashPeople)
}

// readRota works out a competition's rotaFacts from its entries (all, as
// Store.Entries has them), for the cache (liveRota).
func (p *competitionPages) readRota(ctx context.Context, c store.Competition, all []store.Entry) (rotaFacts, error) {
	f := rotaFacts{entries: live(all)}
	var err error
	if f.officials, f.same, err = p.rotaAndSame(ctx, c, f.entries); err != nil {
		return rotaFacts{}, err
	}
	f.people, f.names = peopleOf(f.entries)
	f.one, f.oneNames = clashPeople(f.entries, f.same)
	return f, nil
}

// name names people by key (officialNames): the officials first, then
// anyone competing, with an official's club if withClub.
func (f rotaFacts) name(withClub bool) func(string) string {
	return officialNames(f.officials, f.names, withClub)
}

// whatIfBreaks are what a schedule breaks, less what the "what ifs" show
// themselves (competitions.Schedule.WhatIfBreaks), people each keyed as one
// person, every one named.
func (f rotaFacts) whatIfBreaks(s competitions.Schedule) []competitions.Break {
	return s.WhatIfBreaks(f.one, f.clashNames())
}

// manualBreaks are everything a schedule breaks (competitions.Schedule.Breaks),
// and what its rota does (RotaBreaks), for a manual change to say what it
// would newly break.
func (f rotaFacts) manualBreaks(s competitions.Schedule) []competitions.Break {
	names := f.clashNames()
	return append(s.Breaks(f.one, names), s.RotaBreaks(f.one, f.officials, names)...)
}

// clashNames are oneNames with the officials not competing named too.
func (f rotaFacts) clashNames() map[string]string {
	names := maps.Clone(f.oneNames)
	for _, o := range f.officials {
		if names[o.Key] == "" {
			names[o.Key] = o.Name
		}
	}
	return names
}

// rotaOf is the competition's officials as the rota needs them.
func (p *competitionPages) rotaOf(r *http.Request, c store.Competition, entries []store.Entry) ([]competitions.RotaPerson, error) {
	rota, _, err := p.rotaAndSame(r.Context(), c, entries)
	return rota, err
}

// rotaAndSame is the competition's officials as the rota needs them, and
// who's the same person among them and the entries (newSamePeople), for
// working out clashes and rest.
func (p *competitionPages) rotaAndSame(ctx context.Context, c store.Competition, entries []store.Entry) ([]competitions.RotaPerson, samePeople, error) {
	officials, err := p.st.Officials(ctx, c.ID)
	if err != nil {
		return nil, nil, err
	}
	groups, err := p.st.PersonGroups(ctx, c.ID)
	if err != nil {
		return nil, nil, err
	}
	rota := rotaPeople(c.Competition, officials, entries)
	return rota, newSamePeople(groups, entries, rota), nil
}

// staffing is who may judge each event, and each discipline's panel's
// judges, for placing flights where their judges are free.
func staffing(c competitions.Competition, people []competitions.RotaPerson) *competitions.Staffing {
	s := &competitions.Staffing{Judges: map[string][]string{}, Need: map[string]int{}}
	for _, d := range c.Disciplines() {
		s.Need[d] = c.Officials.Panel(d).Judges()
	}
	for _, o := range people {
		for ev, ok := range o.Judge {
			if ok {
				s.Judges[ev] = append(s.Judges[ev], o.Key)
			}
		}
	}
	return s
}

// staff fills the timetable's panels.
func (p *competitionPages) staff(s *competitions.Schedule, c store.Competition, entries []store.Entry, people []competitions.RotaPerson, same samePeople) {
	s.Rota(schedEntries(entries, same), people, c.Officials, uint64(p.now().UnixNano()))
}

// editRota assigns every panel again (action=rota), or gives one seat to a
// person (action=seat: flight, seat, person; "" empties it).
func (p *competitionPages) editRota(w http.ResponseWriter, r *http.Request) {
	c, draft, _, ok := p.timetableOf(w, r)
	if !ok {
		return
	}
	f, err := p.liveRota(r.Context(), c)
	if err != nil {
		failed(w, r, err)
		return
	}
	notice := ""
	switch r.FormValue("action") {
	case "rota":
		if draft.Planned && !asked(r) {
			p.askBefore(w, r, c, menuTimetable, views.AskFirstPage{
				What: "Assign officials again",
				Says: "Every panel is assigned afresh from who has offered, and what you changed by hand on the panels is lost.",
				Then: "The flights stay as they are.",
				Do:   "Assign officials again", Back: timetablePath(r),
			})
			return
		}
		notice = "Officials assigned again."
	case "seat":
		if !r.Form.Has("person") {
			// The dropdown was left on "Choose a person", which sends
			// nothing: not the same as choosing No one.
			back(w, r, "Choose who takes the seat; nothing was changed.")
			return
		}
	default:
		back(w, r, "")
		return
	}
	anchor := ""
	apply := func(s *competitions.Schedule) error {
		if !s.Planned {
			return refused("Plan the timetable first.")
		}
		if r.FormValue("action") == "rota" {
			p.staff(s, c, f.entries, f.officials, f.same)
			return nil
		}
		seat, err2 := strconv.Atoi(r.FormValue("seat"))
		person := r.FormValue("person")
		i := slices.IndexFunc(f.officials, func(o competitions.RotaPerson) bool { return o.Key == person })
		known, name := person == "" || i >= 0, ""
		if i >= 0 {
			name = f.officials[i].Name
		}
		set := false
		if block, err := strconv.Atoi(r.FormValue("block")); err == nil {
			set = s.SetBlockDuty(block, seat, person, name)
		} else if flight, err := strconv.Atoi(r.FormValue("flight")); err == nil {
			set = s.SetDuty(flight, seat, person, name)
			if set {
				anchor, notice = flightAnchor(s.Flights[flight]), "Seat set on "+s.Flights[flight].Name()+"."
			}
		}
		if err2 != nil || !known || !set {
			return refused("That seat has changed since; nothing was changed.")
		}
		return nil
	}
	// A seat given to someone that breaks something asks first, offering
	// others free for it.
	flight, err1 := strconv.Atoi(r.FormValue("flight"))
	seat, err2 := strconv.Atoi(r.FormValue("seat"))
	if person := r.FormValue("person"); r.FormValue("action") == "seat" && person != "" && err1 == nil && err2 == nil &&
		flight >= 0 && flight < len(draft.Flights) && seat >= 0 && seat < len(draft.Flights[flight].Officials) {
		fl, d := draft.Flights[flight], draft.Flights[flight].Officials[seat]
		name := f.name(false)
		what := "Give " + name(person) + " the " + competitions.RoleWord(d.Role) + " seat on " + fl.Name()
		if p.askFirst(w, r, c, draft, f, what, apply, func(clean func(func(*competitions.Schedule) bool) bool) []views.ChangeFix {
			var fixes []views.ChangeFix
			for _, other := range draft.FreeForSeat(flight, seat, f.one, f.officials) {
				if len(fixes) == maxFixes {
					break
				}
				if other != person && clean(func(t *competitions.Schedule) bool { return t.SetDuty(flight, seat, other, name(other)) }) {
					fixes = append(fixes, views.ChangeFix{Label: "Give it to " + name(other), Fields: withField(postedFields(r), "person", other)})
				}
			}
			return fixes
		}) {
			return
		}
	}
	err = p.changeDraft(r, c, apply)
	changedAt(w, r, err, notice, anchor)
}

// personRule is a rule about an official from the rules form (kind role, off
// or hours), with their name.
func personRule(r *http.Request, people []competitions.RotaPerson) (competitions.Rule, bool) {
	rule := competitions.Rule{Kind: r.FormValue("kind"), Must: r.FormValue("must") == "1", Person: r.FormValue("person"), Event: r.FormValue("event")}
	i := slices.IndexFunc(people, func(o competitions.RotaPerson) bool { return o.Key == rule.Person })
	if i < 0 {
		return rule, false
	}
	rule.Name = people[i].Name
	switch rule.Kind {
	case competitions.RuleRole:
		rule.Role = r.FormValue("role")
	case competitions.RuleHours:
		rule.Event = ""
		rule.Day, _ = strconv.Atoi(r.FormValue("day"))
		rule.From, rule.To = r.FormValue("from"), r.FormValue("to")
	}
	return rule, true
}

// rotaView is the panels and the rota's report, as the page shows them.
func rotaView(s competitions.Schedule, entries []store.Entry, people []competitions.RotaPerson, same samePeople) views.RotaView {
	keys, names := clashPeople(entries, same)
	clubs := map[string]string{}
	for _, e := range schedEntries(entries, same) {
		clubs[e.ID] = e.Club
	}
	for _, o := range people {
		if names[o.Key] == "" {
			names[o.Key] = o.Name
		}
	}
	rr := s.RotaReport(clubs, people)
	v := views.RotaView{People: len(people), Short: rr.Short, Empty: rr.Empty, Broken: rr.Broken,
		Problems: s.RotaProblems(keys, people, names)}
	for _, c := range rr.OwnClub {
		v.OwnClub = append(v.OwnClub, fmt.Sprintf("%s %d", c.Club, c.Times))
	}
	for _, w := range rr.Busiest {
		v.Busiest = append(v.Busiest, fmt.Sprintf("%s: %d, %s", cmp.Or(names[w.Person], w.Name, "Someone"), w.Duties, hoursWord(w.Minutes)))
	}
	for _, o := range people {
		label := o.Name
		if o.Club != "" {
			label += " (" + o.Club + ")"
		}
		v.Options = append(v.Options, views.FlightOption{Value: o.Key, Label: label, Can: seatCan(o)})
	}
	return v
}

// The seat dropdown lists first those who can take the seat chosen beside it
// (static/js/fill.js): each seat says what it needs (seatNeed) and each
// person what they can take (seatCan), as an event's code (seatCode) and a
// letter for the role, e.g. "1x2k9q:e" for an execution judge's seat.
var seatLetters = map[string]string{
	competitions.RoleChair: "c", competitions.RoleDifficulty: "d", competitions.RoleHD: "h", competitions.RoleSync: "s",
	competitions.RoleExecution: "e", competitions.RoleRecorder: "r", competitions.RoleMarshal: "m",
}

// seatCode is an event's short name for the seat dropdown: "*" for blocked
// time's (no event).
func seatCode(event string) string {
	if event == "" {
		return "*"
	}
	h := fnv.New32a()
	h.Write([]byte(event))
	return strconv.FormatUint(uint64(h.Sum32()), 36)
}

// seatNeed is what a seat needs: "r" or "m" for a recorder's or marshal's,
// otherwise its event's code and the role's letter.
func seatNeed(role, event string) string {
	if role == competitions.RoleRecorder || role == competitions.RoleMarshal {
		return seatLetters[role]
	}
	return seatCode(event) + ":" + seatLetters[role]
}

// seatCan is what seats a person can take, space-separated: each event they
// judge with the letters of its roles they may take (blocked time's, "*", as
// they may take a role at anything), then "r" and "m".
func seatCan(p competitions.RotaPerson) string {
	var out []string
	judging := []string{competitions.RoleChair, competitions.RoleDifficulty, competitions.RoleHD, competitions.RoleSync, competitions.RoleExecution}
	letters := func(can func(role string) bool) string {
		l := ""
		for _, role := range judging {
			if can(role) {
				l += seatLetters[role]
			}
		}
		return l
	}
	for _, ev := range slices.Sorted(maps.Keys(p.Judge)) {
		if l := letters(func(role string) bool { return p.Can(role, ev) }); l != "" {
			out = append(out, seatCode(ev)+":"+l)
		}
	}
	if l := letters(p.CanAny); l != "" {
		out = append(out, "*:"+l)
	}
	if p.Recorder {
		out = append(out, "r")
	}
	if p.Marshal {
		out = append(out, "m")
	}
	return strings.Join(out, " ")
}

func hoursWord(minutes int) string {
	if minutes < 60 {
		return fmt.Sprintf("%d min", minutes)
	}
	return fmt.Sprintf("%dh %02dm", minutes/60, minutes%60)
}

// seatsOf are a flight's seats as the pages show them. names are the names
// of the people in the entries, by key, so a seat says who competes.
func seatsOf(f competitions.ScheduledFlight, people []competitions.RotaPerson, names map[string]string) []views.SeatView {
	byKey := map[string]competitions.RotaPerson{}
	for _, o := range people {
		byKey[o.Key] = o
	}
	of := map[string]int{}
	for _, d := range f.Officials {
		of[d.Role]++
	}
	n := map[string]int{}
	var out []views.SeatView
	for _, d := range f.Officials {
		n[d.Role]++
		seat := views.SeatView{Role: competitions.RoleName(d.Role), Person: d.Person, Label: competitions.RoleName(d.Role), Need: seatNeed(d.Role, f.Event)}
		if of[d.Role] > 1 {
			seat.Label += " " + strconv.Itoa(n[d.Role])
		}
		if o, ok := byKey[d.Person]; ok {
			seat.Name, seat.Club = o.Name, o.Club
			if f.Event != "" && !o.Can(d.Role, f.Event) || f.Event == "" && !o.CanAny(d.Role) {
				seat.Problem = "Can't take this seat"
			}
		} else if d.Person != "" {
			seat.Name, seat.Problem = holderName(func(k string) string { return names[k] }, d), "Not an official any more"
		}
		out = append(out, seat)
	}
	return out
}

// holderName is who has a seat, as name names them, else as they were named
// when given it (for someone gone from the officials).
func holderName(name func(string) string, d competitions.Duty) string {
	if n := name(d.Person); n != "" && n != "?" {
		return n
	}
	return cmp.Or(d.Name, "?")
}

// seatRole is who fills a seat, for its role chip: a gymnast if they compete
// here; else the organiser, for someone the organiser added (who has an
// official's key); else their club, which sent their offer.
func seatRole(person string, competes bool) views.Role {
	if competes {
		return views.RoleGymnast
	}
	if kind, _ := store.KeyKind(person); kind == store.KindOfficial {
		return views.RoleOrganiser
	}
	return views.RoleClub
}

// dutiesOf are a person's seats on a published timetable, in time order,
// e.g. "Saturday 10:40–11:50 · Panel 2 · BUCS L5 · Execution judge".
func dutiesOf(c store.Competition, key string) []string {
	t := c.Published
	if t == nil || key == "" {
		return nil
	}
	return dutiesIn(*t, key)
}

// dutiesIn are a person's seats, in time order, one line for the flights of
// an event they hold a seat for, e.g. "Friday 09:30–15:15 · Panel 2 · BUCS
// L7 Men · all 5 flights · Execution judge".
func dutiesIn(s competitions.Schedule, key string) []string {
	type duty struct {
		f    competitions.ScheduledFlight
		role string
		n    int // flights in a row, of the same event, in the same seat
		last int // the last's number
	}
	jobs := s.Staffed()
	slices.SortStableFunc(jobs, func(a, b competitions.ScheduledFlight) int { return (a.Day*1440 + a.Start) - (b.Day*1440 + b.Start) })
	var held []duty
	for _, f := range jobs {
		for _, d := range f.Officials {
			if d.Person != key {
				continue
			}
			if k := len(held) - 1; k >= 0 && held[k].f.Of > 1 && held[k].f.Group() == f.Group() && held[k].f.Area == f.Area && held[k].f.Day == f.Day &&
				held[k].role == d.Role && held[k].last+1 == f.Number {
				held[k].f.End, held[k].n, held[k].last = f.End, held[k].n+1, f.Number
				continue
			}
			held = append(held, duty{f, d.Role, 1, f.Number})
		}
	}
	var out []string
	for _, h := range held {
		out = append(out, dutyLine(s, h.f, h.role, h.n, h.last))
	}
	return out
}

// dutyLine says when, where and what a duty is, over n of an event's
// flights up to its flight last.
func dutyLine(s competitions.Schedule, f competitions.ScheduledFlight, role string, n, last int) string {
	day := ""
	if f.Day < len(s.Setup.Days) {
		day = s.Setup.Days[f.Day].Name + " "
	}
	name := f.Name()
	switch {
	case n == 2 && n == f.Of:
		name = f.Group() + " · both flights"
	case n > 1 && n == f.Of:
		name = fmt.Sprintf("%s · all %d flights", f.Group(), n)
	case n > 1:
		name = fmt.Sprintf("%s · flights %d–%d of %d", f.Group(), last-n+1, last, f.Of)
	}
	return fmt.Sprintf("%s%s–%s · %s · %s · %s", day, competitions.Clock(f.Start), competitions.Clock(f.End), f.Area, name, competitions.RoleName(role))
}

// rotaSheet is each person's duties, for the noticeboard.
func rotaSheet(s competitions.Schedule, people []competitions.RotaPerson) []views.PersonDuties {
	var out []views.PersonDuties
	for _, o := range people {
		if duties := dutiesIn(s, o.Key); len(duties) > 0 {
			out = append(out, views.PersonDuties{Name: o.Name, Club: o.Club, Duties: duties})
		}
	}
	return out
}
