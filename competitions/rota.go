package competitions

import (
	"cmp"
	"fmt"
	"maps"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
)

// The officials rota (ADR 0005 Decisions 5, 8, 9 and 12): each placed
// flight's panel filled from the people who can officiate, never someone
// competing or officiating elsewhere at the same time, keeping the
// organiser's rules about people. Then, as far as it can: own-club judging
// shared fairly between clubs, the work spread, panels kept together through
// an area's flights, and rest before a person competes. A seat no one can
// take is left empty and reported.

// Roles on a panel.
const (
	RoleChair      = "chair"
	RoleDifficulty = "difficulty"
	RoleHD         = "hd"
	RoleExecution  = "execution"
	RoleRecorder   = "recorder"
	RoleMarshal    = "marshal"
)

// Roles are every role, in the order a panel's seats are listed and filled:
// the chair and difficulty judges (whom fewest can be) first.
var Roles = []string{RoleChair, RoleDifficulty, RoleHD, RoleExecution, RoleRecorder, RoleMarshal}

var roleNames = map[string]string{
	RoleChair: "Chair of judges", RoleDifficulty: "Difficulty judge", RoleHD: "HD judge", RoleExecution: "Execution judge",
	RoleRecorder: "Recorder", RoleMarshal: "Marshal",
}

var roleVerbs = map[string]string{
	RoleChair: "chairs", RoleDifficulty: "judges difficulty at", RoleHD: "judges horizontal displacement at", RoleExecution: "judges execution at",
	RoleRecorder: "records at", RoleMarshal: "marshals at",
}

// RoleName is a role's name, e.g. "Chair of judges".
func RoleName(role string) string { return roleNames[role] }

// Seats are a panel's seats, by role, in the order of Roles.
func (p Panel) Seats() []string {
	var out []string
	for _, r := range Roles {
		n := map[string]int{RoleChair: p.Chair, RoleDifficulty: p.Difficulty, RoleHD: p.HD, RoleExecution: p.Execution, RoleRecorder: p.Recorder, RoleMarshal: p.Marshal}[r]
		for range n {
			out = append(out, r)
		}
	}
	return out
}

// Duty is one seat on a flight's panel and who has it; Person is "" for a
// seat no one could take.
type Duty struct {
	Role   string `json:"role"`
	Person string `json:"person,omitempty"` // their key
}

// RotaPerson is someone who can officiate, as the rota needs them: their key
// (the same as where they compete), and what they may do, event by event,
// under the competition's judging rule.
type RotaPerson struct {
	Key, Name, Club   string
	Judge             map[string]bool // events they may judge
	Chair             map[string]bool // events they may chair
	Recorder, Marshal bool
}

// CanAny says whether they may take a role at anything: for blocked time
// that needs officials (an ad hoc event), which isn't one of the events.
func (p RotaPerson) CanAny(role string) bool {
	switch role {
	case RoleChair:
		return slices.Contains(slices.Collect(maps.Values(p.Chair)), true)
	case RoleDifficulty, RoleHD, RoleExecution:
		return slices.Contains(slices.Collect(maps.Values(p.Judge)), true)
	}
	return p.Can(role, "")
}

// Can says whether they may take a role at an event.
func (p RotaPerson) Can(role, event string) bool {
	switch role {
	case RoleChair:
		return p.Chair[event]
	case RoleDifficulty, RoleHD, RoleExecution:
		return p.Judge[event]
	case RoleRecorder:
		return p.Recorder
	case RoleMarshal:
		return p.Marshal
	}
	return false
}

// jobs are what needs officials: every flight, then each block that needs
// them, as a flight named for the block across its areas. block says which
// block each job after the flights is.
func (s Schedule) jobs() (jobs []ScheduledFlight, block []int) {
	jobs = slices.Clone(s.Flights)
	for i, b := range s.Blocks {
		if len(s.blockSeats(b)) == 0 && len(b.Officials) == 0 {
			continue
		}
		jobs = append(jobs, ScheduledFlight{Flight: Flight{Level: b.Name}, Day: b.Day, Area: strings.Join(b.Areas, ", "), Start: b.Start, End: b.End, Officials: b.Officials})
		block = append(block, i)
	}
	return jobs, block
}

// Staffed is everything with a panel: the flights, then each block that
// needs officials, named for the block, across its areas.
func (s Schedule) Staffed() []ScheduledFlight {
	jobs, _ := s.jobs()
	return jobs
}

// blockSeats are the seats a placed block's setup asks for.
func (s Schedule) blockSeats(b ScheduledBlock) []string {
	for _, sb := range s.Setup.Blocks {
		if sb.Name == b.Name && sb.Day == b.Day {
			return sb.Officials.Seats()
		}
	}
	return nil
}

// rota is one attempt at filling the panels.
type rota struct {
	s        *Schedule
	jobs     []ScheduledFlight // the flights, then the blocks that need officials
	flights  int               // how many of the jobs are flights
	people   []RotaPerson
	busy     map[string][]interval // competing, by person
	duties   map[string][]interval // officiating, by person
	minutes  map[string]int        // officiating, by person
	own      map[string]int        // times a club's officials judged a flight with its gymnasts in
	lastOn   map[string]lastDuty   // the person's last duty
	clubsIn  []map[string]bool     // by flight
	hours    map[string][]Rule     // RuleHours, by person
	cost     float64
	empty    int
	assigned [][]Duty
	byKey    map[string]int
	rng      *rand.Rand
}

// lastDuty is where a person last officiated.
type lastDuty struct {
	day  int
	area string
	end  int
}

// Rota fills every placed flight's panel (Decision 12), replacing any it had.
// entries are the scheduled entries (for who competes when, and their clubs);
// people are those who can officiate.
func (s *Schedule) Rota(entries []SchedEntry, people []RotaPerson, settings OfficialSettings, seed uint64) {
	rng := rand.New(rand.NewPCG(seed, 11))
	byEntry := map[string]SchedEntry{}
	for _, e := range entries {
		byEntry[e.ID] = e
	}
	jobs, blocks := s.jobs()
	busy := map[string][]interval{}
	var clubsIn []map[string]bool
	for _, f := range jobs {
		iv := interval{f.Day, f.Start, f.End}
		clubs := map[string]bool{}
		for _, id := range f.Entries {
			e := byEntry[id]
			for _, p := range e.People {
				busy[p] = append(busy[p], iv)
			}
			if e.Club != "" {
				clubs[e.Club] = true
			}
		}
		clubsIn = append(clubsIn, clubs)
	}
	order := make([]int, len(jobs))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int {
		fa, fb := jobs[a], jobs[b]
		return cmp.Or(fa.Day-fb.Day, fa.Start-fb.Start, strings.Compare(fa.Area, fb.Area))
	})
	var best *rota
	for range 12 {
		r := &rota{
			s: s, jobs: jobs, flights: len(s.Flights), people: people, busy: busy, duties: map[string][]interval{}, minutes: map[string]int{},
			own: map[string]int{}, lastOn: map[string]lastDuty{}, clubsIn: clubsIn, hours: map[string][]Rule{},
			assigned: make([][]Duty, len(jobs)), byKey: map[string]int{}, rng: rng,
		}
		for i, p := range people {
			r.byKey[p.Key] = i
		}
		for _, rule := range s.Setup.Rules {
			if rule.Kind == RuleHours {
				r.hours[rule.Person] = append(r.hours[rule.Person], rule)
			}
		}
		for _, i := range order {
			seats := settings.Panel(jobs[i].Discipline).Seats()
			if i >= len(s.Flights) {
				seats = s.blockSeats(s.Blocks[blocks[i-len(s.Flights)]])
			}
			r.staff(i, seats)
		}
		r.cost += float64(r.empty) * 1000
		if best == nil || r.cost < best.cost {
			best = r
		}
	}
	for i := range s.Flights {
		s.Flights[i].Officials = best.assigned[i]
	}
	for k, b := range blocks {
		s.Blocks[b].Officials = best.assigned[len(s.Flights)+k]
	}
}

// staff fills one flight's seats: the organiser's musts first, then each
// seat with whoever costs least.
func (r *rota) staff(i int, seats []string) {
	f := r.jobs[i]
	duties := make([]Duty, len(seats))
	for k, role := range seats {
		duties[k].Role = role
	}
	taken := map[string]bool{}
	take := func(k int, key string, cost float64) {
		duties[k].Person = key
		taken[key] = true
		iv := interval{f.Day, f.Start, f.End}
		r.duties[key] = append(r.duties[key], iv)
		r.minutes[key] += f.End - f.Start
		r.lastOn[key] = lastDuty{f.Day, f.Area, f.End}
		if p, ok := r.byKey[key]; ok && r.clubsIn[i][r.people[p].Club] && isJudge(duties[k].Role) {
			r.own[r.people[p].Club]++
		}
		r.cost += cost
	}
	// The organiser's musts: this person in this role, if they're free.
	for _, rule := range r.s.Setup.Rules {
		if rule.Kind != RuleRole || !rule.Must || rule.Event != f.Level || taken[rule.Person] || !r.free(rule.Person, i) {
			continue
		}
		for k := range duties {
			if duties[k].Role == rule.Role && duties[k].Person == "" {
				take(k, rule.Person, 0)
				break
			}
		}
	}
	for k := range duties {
		if duties[k].Person != "" {
			continue
		}
		bestCost, bestKey := math.Inf(1), ""
		for _, p := range r.people {
			if taken[p.Key] || !r.can(p, duties[k].Role, i) || !r.free(p.Key, i) {
				continue
			}
			if c := r.costOf(p, duties[k].Role, i) + r.rng.Float64(); c < bestCost {
				bestCost, bestKey = c, p.Key
			}
		}
		if bestKey == "" {
			r.empty++
			continue
		}
		take(k, bestKey, bestCost)
	}
	r.assigned[i] = duties
}

// can says whether a person may take a role at a job.
func (r *rota) can(p RotaPerson, role string, i int) bool {
	if i >= r.flights {
		return p.CanAny(role)
	}
	return p.Can(role, r.jobs[i].Level)
}

func isJudge(role string) bool {
	return role == RoleChair || role == RoleDifficulty || role == RoleHD || role == RoleExecution
}

// free says whether a person can officiate a flight: not competing or
// officiating then, not kept off it by a must, and not kept for another
// flight at the same time that a must gives them a role in.
func (r *rota) free(key string, i int) bool {
	f := r.jobs[i]
	iv := interval{f.Day, f.Start, f.End}
	for _, used := range r.busy[key] {
		if overlaps(iv, used) {
			return false
		}
	}
	for _, used := range r.duties[key] {
		if overlaps(iv, used) {
			return false
		}
	}
	for _, rule := range r.s.Setup.Rules {
		if rule.Person != key || !rule.Must {
			continue
		}
		if broken, _ := r.s.breaksPersonRule(rule, f, key); broken {
			return false
		}
		if rule.Kind == RuleRole && rule.Event != f.Level {
			for j, g := range r.jobs[:r.flights] {
				if j != i && g.Level == rule.Event && overlaps(iv, interval{g.Day, g.Start, g.End}) {
					return false
				}
			}
		}
	}
	return true
}

// breaksPersonRule says whether a person officiating a flight breaks a rule
// about them; for RuleRole, whether it's the rule's event (so another role
// there falls short of it).
func (s *Schedule) breaksPersonRule(rule Rule, f ScheduledFlight, key string) (bool, bool) {
	if rule.Person != key {
		return false, false
	}
	switch rule.Kind {
	case RuleOff:
		return rule.Event == "" || rule.Event == f.Level, true
	case RuleHours:
		if f.Day != rule.Day {
			return false, true
		}
		from, to := 0, 24*60
		if rule.From != "" {
			from, _ = clock(rule.From)
		}
		if rule.To != "" {
			to, _ = clock(rule.To)
		}
		return f.Start < from || f.End > to, true
	}
	return false, false
}

// costOf is what giving a person a seat costs: lower is better.
func (r *rota) costOf(p RotaPerson, role string, i int) float64 {
	f := r.jobs[i]
	cost := float64(r.minutes[p.Key]) * 0.1 // spread the work
	// Keep a panel together on its area from one flight to the next.
	if last, ok := r.lastOn[p.Key]; ok && last.day == f.Day && last.area == f.Area && f.Start-last.end <= 30 {
		cost -= 10
	}
	// Own-club judging, shared between clubs: each time costs more for a club
	// that's done it more.
	if isJudge(role) && p.Club != "" && r.clubsIn[i][p.Club] {
		cost += 4 + 2*float64(r.own[p.Club])
	}
	// Helpers who could judge here are better kept for judging.
	if !isJudge(role) && p.Judge[f.Level] {
		cost += 5
	}
	// Rest before (and, less, after) their own turn to compete.
	if rest := r.s.Setup.Rest; rest > 0 {
		for _, iv := range r.busy[p.Key] {
			if iv.day != f.Day {
				continue
			}
			if gap := iv.start - f.End; gap >= 0 && gap < rest {
				cost += 2 * float64(rest-gap)
			}
			if gap := f.Start - iv.end; gap >= 0 && gap < rest {
				cost += float64(rest - gap)
			}
		}
	}
	// The organiser's prefers.
	for _, rule := range r.s.Setup.Rules {
		if rule.Must || rule.Person != p.Key {
			continue
		}
		if rule.Kind == RuleRole && rule.Event == f.Level {
			if rule.Role == role {
				cost -= 200
			}
			continue
		}
		if broken, _ := r.s.breaksPersonRule(rule, f, p.Key); broken {
			cost += 200
		}
	}
	return cost
}

// RotaReport is how the rota meets its goals.
type RotaReport struct {
	Short   []string    // seats no one could take, by flight
	OwnClub []ClubCount // times each club's officials judged a flight with their club's gymnasts in
	Busiest []Workload  // who officiates most
	Broken  []string    // the organiser's rules about people that aren't kept
}

// ClubCount is a club and a number of times.
type ClubCount struct {
	Club  string
	Times int
}

// Workload is a person's officiating.
type Workload struct {
	Person  string
	Duties  int
	Minutes int
}

// RotaReport reports on the rota. clubs are each entry's club; officials
// are the people who can officiate.
func (s Schedule) RotaReport(clubs map[string]string, officials []RotaPerson) RotaReport {
	var rr RotaReport
	clubOf := map[string]string{}
	for _, p := range officials {
		clubOf[p.Key] = p.Club
	}
	own := map[string]int{}
	work := map[string]*Workload{}
	jobs, _ := s.jobs()
	for _, f := range jobs {
		short := map[string]int{}
		in := map[string]bool{}
		for _, id := range f.Entries {
			in[clubs[id]] = true
		}
		for _, d := range f.Officials {
			if d.Person == "" {
				short[d.Role]++
				continue
			}
			w := work[d.Person]
			if w == nil {
				w = &Workload{Person: d.Person}
				work[d.Person] = w
			}
			w.Duties++
			w.Minutes += f.End - f.Start
			if c := clubOf[d.Person]; c != "" && in[c] && isJudge(d.Role) {
				own[c]++
			}
		}
		var parts []string
		for _, role := range Roles {
			if n := short[role]; n > 0 {
				parts = append(parts, fmt.Sprintf("%d %s", n, strings.ToLower(roleNames[role])+plural(n)))
			}
		}
		if len(parts) > 0 {
			rr.Short = append(rr.Short, fmt.Sprintf("%s on %s: no one for %s", f.Name(), f.Area, joinList(parts)))
		}
	}
	for c, n := range own {
		rr.OwnClub = append(rr.OwnClub, ClubCount{c, n})
	}
	slices.SortFunc(rr.OwnClub, func(a, b ClubCount) int { return cmp.Or(b.Times-a.Times, strings.Compare(a.Club, b.Club)) })
	for _, w := range work {
		rr.Busiest = append(rr.Busiest, *w)
	}
	slices.SortFunc(rr.Busiest, func(a, b Workload) int { return cmp.Or(b.Minutes-a.Minutes, strings.Compare(a.Person, b.Person)) })
	rr.Busiest = rr.Busiest[:min(len(rr.Busiest), 5)]

	for _, rule := range s.Setup.Rules {
		if !personRule(rule.Kind) {
			continue
		}
		broken := false
		for _, f := range jobs {
			has, inRole := false, false
			for _, d := range f.Officials {
				if d.Person == rule.Person {
					has, inRole = true, inRole || d.Role == rule.Role
				}
			}
			if rule.Kind == RuleRole {
				broken = broken || (f.Level == rule.Event && !inRole)
				continue
			}
			if b, _ := s.breaksPersonRule(rule, f, rule.Person); has && b {
				broken = true
			}
		}
		if broken {
			rr.Broken = append(rr.Broken, rule.Describe(s.Setup.Days))
		}
	}
	return rr
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// SetBlockDuty gives a seat on blocked time's panel to a person ("" to empty it).
func (s *Schedule) SetBlockDuty(block, seat int, person string) bool {
	if block < 0 || block >= len(s.Blocks) || seat < 0 || seat >= len(s.Blocks[block].Officials) {
		return false
	}
	s.Blocks[block].Officials[seat].Person = person
	return true
}

// SetDuty gives a seat on a flight's panel to a person ("" to empty it).
func (s *Schedule) SetDuty(flight, seat int, person string) bool {
	if flight < 0 || flight >= len(s.Flights) || seat < 0 || seat >= len(s.Flights[flight].Officials) {
		return false
	}
	s.Flights[flight].Officials[seat].Person = person
	return true
}

// RotaProblems are what hand changes (to flights or seats) have broken in the
// rota: someone officiating while competing or officiating elsewhere, or in a
// role they can't take. people are each entry's people; names name people.
func (s Schedule) RotaProblems(people map[string][]string, officials []RotaPerson, names map[string]string) []string {
	byKey := map[string]RotaPerson{}
	for _, p := range officials {
		byKey[p.Key] = p
	}
	name := func(key string) string {
		if n := names[key]; n != "" {
			return n
		}
		return byKey[key].Name
	}
	var out []string
	seen := map[string]bool{}
	add := func(msg string) {
		if !seen[msg] {
			seen[msg] = true
			out = append(out, msg)
		}
	}
	jobs, _ := s.jobs()
	for i, f := range jobs {
		iv := interval{f.Day, f.Start, f.End}
		seated := map[string]bool{}
		for _, d := range f.Officials {
			if d.Person == "" {
				continue
			}
			if seated[d.Person] {
				add(fmt.Sprintf("%s has two seats on %s's panel", name(d.Person), f.Name()))
			}
			seated[d.Person] = true
			if p, ok := byKey[d.Person]; !ok {
				add(fmt.Sprintf("%s isn't an official any more (%s, %s)", name(d.Person), strings.ToLower(roleNames[d.Role]), f.Name()))
			} else if (i < len(s.Flights) && !p.Can(d.Role, f.Level)) || (i >= len(s.Flights) && !p.CanAny(d.Role)) {
				add(fmt.Sprintf("%s can't be %s at %s", name(d.Person), strings.ToLower(roleNames[d.Role]), f.Level))
			}
			for j, g := range jobs {
				if !overlaps(iv, interval{g.Day, g.Start, g.End}) {
					continue
				}
				for _, id := range g.Entries {
					if slices.Contains(people[id], d.Person) {
						add(fmt.Sprintf("%s officiates %s while competing in %s", name(d.Person), f.Name(), g.Name()))
					}
				}
				if j > i {
					for _, e := range g.Officials {
						if e.Person == d.Person {
							add(fmt.Sprintf("%s officiates %s and %s at once", name(d.Person), f.Name(), g.Name()))
						}
					}
				}
			}
		}
	}
	return out
}
