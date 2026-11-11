package competitions

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

// Random rotas and leave times, to check what no leave may do: put someone
// in two places, change a flight that has ended, or split a seat that stays
// with an event's whole run. The generators are the planner's property
// tests' (the same seeds give the same worlds).

type leaveWorld struct {
	setup   Setup
	entries []SchedEntry
	events  []string
	disc    map[string]string
	people  map[string][]string
}

func genLeaveWorld(r *rand.Rand) leaveWorld {
	w := leaveWorld{disc: map[string]string{}, people: map[string][]string{}}
	nd := 1 + r.IntN(3)
	for d := 0; d < nd; d++ {
		st := 8*60 + 30*r.IntN(3)
		en := st + 6*60 + 30*r.IntN(8)
		w.setup.Days = append(w.setup.Days, Day{Name: fmt.Sprintf("Day%d", d+1), Start: Clock(st), End: Clock(en)})
	}
	nt := 1 + r.IntN(4)
	for i := 0; i < nt; i++ {
		w.setup.Areas = append(w.setup.Areas, Area{Name: fmt.Sprintf("Panel %d", i+1), Discipline: Trampoline})
	}
	hasTum, hasDMT := r.IntN(2) == 0, r.IntN(2) == 0
	if hasTum {
		w.setup.Areas = append(w.setup.Areas, Area{Name: "Track 1", Discipline: Tumbling})
	}
	if hasDMT {
		w.setup.Areas = append(w.setup.Areas, Area{Name: "DMT 1", Discipline: DMT})
	}
	// day area restrictions
	if r.IntN(4) == 0 && nd > 1 {
		d := r.IntN(nd)
		for _, a := range w.setup.Areas {
			if r.IntN(3) > 0 {
				w.setup.Days[d].Areas = append(w.setup.Days[d].Areas, a.Name)
			}
		}
	}
	// blocks
	for d := 0; d < nd; d++ {
		if r.IntN(3) > 0 {
			b := Block{Name: "Lunch", Minutes: 30 + 15*r.IntN(3), Day: d}
			if r.IntN(2) == 0 {
				b.At = Clock(12*60 + 15*r.IntN(4))
			} else {
				b.From, b.To = "12:00", "14:00"
			}
			if r.IntN(3) == 0 {
				b.Areas = []string{w.setup.Areas[r.IntN(len(w.setup.Areas))].Name}
			}
			w.setup.Blocks = append(w.setup.Blocks, b)
		}
		if r.IntN(4) == 0 {
			w.setup.Blocks = append(w.setup.Blocks, Block{Name: "Awards", Minutes: 20, Day: d, At: Clock(13*60 + 15*r.IntN(8))})
		}
	}
	w.setup.Rest = []int{0, 10, 20, 30}[r.IntN(4)]
	w.setup.RestMust = r.IntN(3) == 0
	w.setup.AcrossBreaks = r.IntN(2) == 0
	w.setup.Separate = Split{Mode: []string{SplitNone, SplitAll}[r.IntN(2)]}
	// events
	type ev struct{ name, d string }
	var evs []ev
	ntr := 1 + r.IntN(6)
	for i := 0; i < ntr; i++ {
		evs = append(evs, ev{fmt.Sprintf("L%d", i+1), Trampoline})
	}
	if r.IntN(2) == 0 {
		for i := 0; i < 1+r.IntN(2); i++ {
			evs = append(evs, ev{fmt.Sprintf("Synchro S%d", i+1), Synchro})
		}
	}
	if hasTum {
		evs = append(evs, ev{"Tumbling T1", Tumbling})
	}
	if hasDMT {
		evs = append(evs, ev{"DMT D1", DMT})
	}
	for _, e := range evs {
		w.events = append(w.events, e.name)
		w.disc[e.name] = e.d
	}
	npeople := 5 + r.IntN(60)
	id := 0
	for p := 0; p < npeople; p++ {
		key := fmt.Sprintf("p%d", p)
		club := fmt.Sprintf("club%d", r.IntN(6))
		cat := Categories[r.IntN(2)]
		// each person: one trampoline level (mostly), maybe synchro, tumbling, dmt
		for _, e := range evs {
			var take bool
			switch e.d {
			case Trampoline:
				take = false
			case Synchro:
				take = r.IntN(4) == 0
			default:
				take = r.IntN(5) == 0
			}
			if !take {
				continue
			}
			id++
			se := SchedEntry{PlanEntry: PlanEntry{ID: fmt.Sprintf("e%d", id), Level: e.name, Category: cat, Club: club}, Discipline: e.d, People: []string{key}}
			if e.d == Synchro {
				se.People = append(se.People, fmt.Sprintf("p%d", r.IntN(npeople)))
			}
			w.entries = append(w.entries, se)
		}
		if r.IntN(10) > 0 {
			lv := evs[r.IntN(ntr)]
			id++
			w.entries = append(w.entries, SchedEntry{PlanEntry: PlanEntry{ID: fmt.Sprintf("e%d", id), Level: lv.name, Category: cat, Club: club}, Discipline: lv.d, People: []string{key}})
		}
	}
	for _, e := range w.entries {
		w.people[e.ID] = e.People
	}
	// rules
	nr := r.IntN(4)
	for i := 0; i < nr; i++ {
		e1 := evs[r.IntN(len(evs))]
		e2 := evs[r.IntN(len(evs))]
		must := r.IntN(2) == 0
		switch r.IntN(4) {
		case 0:
			var ok []string
			for _, a := range w.setup.Areas {
				ad := e1.d
				if ad == Synchro {
					ad = Trampoline
				}
				if a.Discipline == ad {
					ok = append(ok, a.Name)
				}
			}
			if len(ok) > 0 {
				w.setup.Rules = append(w.setup.Rules, Rule{Kind: RuleArea, Must: must, Event: e1.name, Area: ok[r.IntN(len(ok))]})
			}
		case 1:
			w.setup.Rules = append(w.setup.Rules, Rule{Kind: RuleDay, Must: must, Event: e1.name, Day: r.IntN(nd)})
		case 2:
			if e1 != e2 {
				w.setup.Rules = append(w.setup.Rules, Rule{Kind: RuleBefore, Must: must, Event: e1.name, Event2: e2.name})
			}
		case 3:
			if e1 != e2 {
				w.setup.Rules = append(w.setup.Rules, Rule{Kind: RuleApart, Must: must, Event: e1.name, Event2: e2.name})
			}
		}
	}
	return w
}
func leaveOfficials(r *rand.Rand, w leaveWorld, n int) []RotaPerson {
	var out []RotaPerson
	for j := 0; j < n; j++ {
		o := RotaPerson{Key: fmt.Sprintf("o%d", j), Name: fmt.Sprintf("Official %d", j), Club: fmt.Sprintf("club%d", j%6), Recorder: r.IntN(2) == 0, Marshal: r.IntN(2) == 0}
		if r.IntN(2) == 0 {
			o.Key = fmt.Sprintf("p%d", r.IntN(40)) // also a gymnast, perhaps
			if slices.ContainsFunc(out, func(x RotaPerson) bool { return x.Key == o.Key }) {
				continue
			}
		}
		for _, ev := range w.events {
			if r.IntN(3) == 0 {
				continue
			}
			o.Allow(RoleHD, ev)
			if r.IntN(2) == 0 {
				o.Allow(RoleExecution, ev)
			}
			if r.IntN(3) == 0 {
				o.Allow(RoleDifficulty, ev)
			}
			if r.IntN(4) == 0 {
				o.Allow(RoleChair, ev)
			}
		}
		out = append(out, o)
	}
	return out
}

// leaveRun is one random rota, one leave and what became of it.
type leaveRun struct {
	w        leaveWorld
	offs     []RotaPerson
	settings OfficialSettings
	before   Schedule
	after    Schedule
	leave    Leave
	fills    []SeatFill
}

// randomLeave plans a random timetable, staffs it, and has a random seated
// official leave at a random time; ok is false if there was nothing to do.
func randomLeave(seed uint64) (run leaveRun, ok bool) {
	r := rand.New(rand.NewPCG(seed, 99))
	w := genLeaveWorld(r)
	s := PlanSchedule(w.entries, w.events, w.setup, seed)
	if len(s.Flights) < 2 {
		return run, false
	}
	offs := leaveOfficials(r, w, 12+r.IntN(25))
	settings := OfficialSettings{Panels: map[string]Panel{}, RecordersChange: r.IntN(2) == 0, MarshalsChange: r.IntN(2) == 0}
	for _, d := range AllDisciplines {
		settings.Panels[d] = Panel{Chair: 1, Execution: 1 + r.IntN(3), Difficulty: 1, HD: r.IntN(2), Recorder: 1, Marshal: 1}
	}
	s.Rota(w.entries, offs, settings, seed)
	var seated []string
	for _, f := range s.Flights {
		for _, d := range f.Officials {
			if d.Person != "" && !slices.Contains(seated, d.Person) {
				seated = append(seated, d.Person)
			}
		}
	}
	if len(seated) == 0 {
		return run, false
	}
	who := seated[r.IntN(len(seated))]
	day := r.IntN(len(s.Setup.Days))
	start, end := s.Setup.Days[day].Hours()
	l := Leave{Person: who, Day: day, From: start + r.IntN(end-start), ForGood: r.IntN(2) == 0, Settings: settings}
	out, fills, err := s.Left(l, w.people, offs)
	if err != nil {
		panic(err)
	}
	return leaveRun{w, offs, settings, s, out, l, fills}, true
}

// problems are what's wrong with a leave's result, by kind: someone in two
// places or seated where they can't be, a flight that had ended changed, a
// seat that stays with an event's whole run split among the flights still to
// come when it wasn't before.
func (run leaveRun) problems() map[string][]string {
	bad := map[string][]string{}
	add := func(kind, f string, a ...any) { bad[kind] = append(bad[kind], fmt.Sprintf(f, a...)) }
	by := map[string]RotaPerson{}
	for _, o := range run.offs {
		by[o.Key] = o
	}
	s, out, l := run.before, run.after, run.leave
	for i, f := range out.Flights {
		seated := map[string]bool{}
		for k, d := range f.Officials {
			if !l.ahead(f) && d != s.Flights[i].Officials[k] {
				add("ended flight changed", "%s %s: %s -> %s (ended %s, leave %s)", f.Name(), d.Role, s.Flights[i].Officials[k].Person, d.Person, Clock(f.End), Clock(l.From))
			}
			if d.Person == "" {
				continue
			}
			if !by[d.Person].Can(d.Role, f.Event) {
				add("can't take it", "%s as %s at %s", d.Person, d.Role, f.Event)
			}
			if seated[d.Person] {
				add("two seats", "%s on %s", d.Person, f.Name())
			}
			seated[d.Person] = true
			for j, g := range out.Flights {
				if !overlaps(interval{f.Day, f.Start, f.End}, interval{g.Day, g.Start, g.End}) {
					continue
				}
				for _, id := range g.Entries {
					if slices.Contains(run.w.people[id], d.Person) {
						add("seated while competing", "%s officiates %s while competing in %s", d.Person, f.Name(), g.Name())
					}
				}
				if j > i && f.Group() != g.Group() && slices.ContainsFunc(g.Officials, func(e Duty) bool { return e.Person == d.Person }) {
					add("two places", "%s officiates %s and %s at once", d.Person, f.Name(), g.Name())
				}
			}
		}
	}
	// one says whether a seat that stays with an event's run has one holder
	// for what's left of it.
	one := func(sc Schedule, group string, seat int) bool {
		holders := map[string]bool{}
		for _, f := range sc.Flights {
			if f.Group() == group && l.ahead(f) && seat < len(f.Officials) && run.settings.Stays(f.Officials[seat].Role) {
				holders[f.Officials[seat].Person] = true
			}
		}
		return len(holders) <= 1
	}
	seen := map[string]bool{}
	for _, f := range s.Flights {
		for seat := range f.Officials {
			key := fmt.Sprint(f.Group(), seat)
			if seen[key] {
				continue
			}
			seen[key] = true
			if one(s, f.Group(), seat) && !one(out, f.Group(), seat) {
				add("whole-run seat split", "%s seat %d (%s)", f.Group(), seat, f.Officials[seat].Role)
			}
		}
	}
	return bad
}

func TestOfficialLeaveRandom(t *testing.T) {
	counts := map[string]int{}
	runs := 0
	for seed := uint64(1); seed <= 1500; seed++ {
		run, ok := randomLeave(seed)
		if !ok {
			continue
		}
		runs++
		for kind, ls := range run.problems() {
			counts[kind]++
			t.Errorf("seed %d, %s leaves day %d from %s (for good: %v): %s: %s", seed, run.leave.Person, run.leave.Day, Clock(run.leave.From), run.leave.ForGood, kind, strings.Join(ls[:min(2, len(ls))], "; "))
		}
	}
	t.Logf("%d leaves tried; problems %v", runs, counts)
}

// The two leaves the audit found: a recorder moved into a marshal seat for a
// whole run when they compete in one of its flights, and a flight that had
// ended given new people.
func TestOfficialLeaveSeeds(t *testing.T) {
	for _, seed := range []uint64{1279, 41} {
		run, ok := randomLeave(seed)
		if !ok {
			t.Fatalf("seed %d: nothing to test", seed)
		}
		for kind, ls := range run.problems() {
			t.Errorf("seed %d: %s: %s", seed, kind, strings.Join(ls, "; "))
		}
	}
}
