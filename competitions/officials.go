package competitions

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// Officials (ADR 0005 Decisions 5 and 13): who can judge and help, what each
// discipline's panel needs, and who may judge what. Gymnasts offer when they
// enter; the organiser can add people with no club. Spotters aren't a panel
// position and aren't scheduled.

// Panel is how many officials one panel of a discipline needs.
type Panel struct {
	Chair      int `json:"chair"`          // Chair of Judges Panel
	Execution  int `json:"execution"`      // execution judges
	Difficulty int `json:"difficulty"`     // difficulty judges
	HD         int `json:"hd,omitempty"`   // horizontal displacement judges, where no machine measures it (trampoline and synchro)
	Sync       int `json:"sync,omitempty"` // synchronisation judges, where no machine measures it (synchro)
	Recorder   int `json:"recorder"`
	Marshal    int `json:"marshal"`
}

// CodePanel is the FIG Code of Points 2025–2028's panel, the same for every
// discipline (§18.1 for trampoline and synchro, §17.1 for tumbling and DMT):
// a CJP, 6 execution and 2 difficulty judges. Trampoline and synchro also
// need horizontal displacement (HD) judges where no machine measures it:
// none by default, for the organiser to add. Helpers aren't in the Code; one
// recorder and one marshal is a starting point.
var CodePanel = Panel{Chair: 1, Execution: 6, Difficulty: 2, Recorder: 1, Marshal: 1}

// DefaultPanel is a discipline's panel until the organiser changes it: the
// Code of Points', and for synchro 2 synchronisation judges, as student
// competitions have no machine measuring synchronisation (the Code's §18.1
// leaves it to one).
func DefaultPanel(discipline string) Panel {
	p := CodePanel
	if discipline == Synchro {
		p.Sync = 2
	}
	return p
}

// Judges is how many judges the panel needs, chair included.
func (p Panel) Judges() int { return p.Chair + p.Execution + p.Difficulty + p.HD + p.Sync }

func (p Panel) check() error {
	for _, n := range []int{p.Chair, p.Execution, p.Difficulty, p.HD, p.Sync, p.Recorder, p.Marshal} {
		if n < 0 || n > 20 {
			return errors.New("each kind of official on a panel should be from 0 to 20")
		}
	}
	return nil
}

// Who may judge what: anyone, as they offer (the default), or only people the
// organiser marks as qualified (as they offer). A rule of judging only levels
// below one's own, once offered, reads as the default: the organiser's Note
// asks for that, where they want it.
const (
	JudgeDeclared  = ""
	JudgeQualified = "qualified"
)

// OfficialSettings are the organiser's settings for officials.
type OfficialSettings struct {
	Panels map[string]Panel `json:"panels,omitempty"` // by discipline; missing: the Code of Points'
	Judge  string           `json:"judge,omitempty"`  // JudgeDeclared or JudgeQualified
	// Note is the organiser's word to those offering, beside the offer form,
	// e.g. who they're short of, or who they'd rather didn't judge their own
	// level.
	Note string `json:"note,omitempty"`
	// RecordersChange and MarshalsChange let those seats change between an
	// event's flights; otherwise the whole panel stays for all of them (ADR
	// 0006).
	RecordersChange bool `json:"recorders_change,omitempty"`
	MarshalsChange  bool `json:"marshals_change,omitempty"`
}

// Panel is a discipline's panel.
func (s OfficialSettings) Panel(discipline string) Panel {
	if p, ok := s.Panels[discipline]; ok {
		return p
	}
	return DefaultPanel(discipline)
}

// Check reports what's wrong with the settings.
func (s OfficialSettings) Check() error {
	if !slices.Contains([]string{JudgeDeclared, JudgeQualified}, s.Judge) {
		return fmt.Errorf("unknown judging rule %q", s.Judge)
	}
	if utf8.RuneCountInString(s.Note) > 500 {
		return errors.New("the note to those offering is longer than 500 characters")
	}
	for d, p := range s.Panels {
		if !slices.Contains(AllDisciplines, d) {
			return fmt.Errorf("unknown discipline %q", d)
		}
		if err := p.check(); err != nil {
			return fmt.Errorf("%s: %w", DisciplineName(d), err)
		}
	}
	return nil
}

// Offer is what a person can do as an official: judge some disciplines (HD
// and synchronisation at any level, the rest each up to a level of their
// own) and help as a recorder or marshal.
type Offer struct {
	Judge    map[string]JudgeOffer `json:"judge,omitempty"` // by discipline
	Recorder bool                  `json:"recorder,omitempty"`
	Marshal  bool                  `json:"marshal,omitempty"`
}

// JudgeOffer is judging one discipline: its HD and synchronisation at any
// level, where it has them, and each of LevelRoles up to the level they say.
// Offers from before levels were by role (up_to and chair) read as none.
type JudgeOffer struct {
	UpTo map[string]string `json:"roles,omitempty"` // by LevelRoles' role: the highest level they'll take it at, "" for any; missing: not that role
}

// Takes says whether they offer one of LevelRoles, at some level.
func (j JudgeOffer) Takes(role string) bool {
	_, ok := j.UpTo[role]
	return ok
}

// LevelRoles are the judging roles taken up to a level, which a judge offers
// each on its own: someone may judge difficulty at a level but not execution.
// HD and synchronisation judges measure, the same at every level.
var LevelRoles = []string{RoleExecution, RoleDifficulty, RoleChair}

// Measured are a discipline's roles any of its judges may take at any level.
func Measured(discipline string) []string {
	switch discipline {
	case Trampoline:
		return []string{RoleHD}
	case Synchro:
		return []string{RoleHD, RoleSync}
	}
	return nil
}

// Empty says whether the offer offers nothing.
func (o Offer) Empty() bool { return len(o.Judge) == 0 && !o.Recorder && !o.Marshal }

// ValidateOffer reports what's wrong with an offer for this competition:
// disciplines it doesn't offer, levels it doesn't have, or judging a
// discipline with nothing to take in it.
func (c Competition) ValidateOffer(o Offer) error {
	var errs []error
	for d, j := range o.Judge {
		if !slices.Contains(c.Disciplines(), d) {
			errs = append(errs, fmt.Errorf("the competition doesn't offer %s", DisciplineName(d)))
			continue
		}
		if d == Synchro {
			errs = append(errs, errors.New("synchro is judged by trampoline's judges: offer trampoline"))
			continue
		}
		for role, upTo := range j.UpTo {
			if !slices.Contains(LevelRoles, role) {
				errs = append(errs, fmt.Errorf("%s: unknown role %q", DisciplineName(d), role))
			} else if upTo != "" && !slices.Contains(c.levelNames(d), upTo) {
				errs = append(errs, fmt.Errorf("%s has no level %q", DisciplineName(d), upTo))
			}
		}
		if len(j.UpTo) == 0 && len(Measured(d)) == 0 {
			errs = append(errs, fmt.Errorf("say what you'd judge in %s: execution, difficulty or chairing", DisciplineName(d)))
		}
	}
	return errors.Join(errs...)
}

// CanJudge says whether an official may take a judging role at a
// discipline's level under the competition's rule: HD and synchronisation at
// any level of a discipline they judge; execution, difficulty and chairing up
// to the level they say for each. qualified is the organiser's mark.
//
// Synchro is judged by trampoline's judges, at its levels: an event they may
// take a role at if they may at the hardest trampoline level it pairs (chair
// up to ISTO Advanced, chair "ISTO Intervanced/Advanced"); one pairing a
// level trampoline doesn't have, only at any level.
func (c Competition) CanJudge(o Offer, qualified bool, discipline, level, role string) bool {
	j, ok := o.For(discipline)
	if !ok || (c.Officials.Judge == JudgeQualified && !qualified) {
		return false
	}
	if !slices.Contains(c.levelNames(discipline), level) {
		return false
	}
	if slices.Contains(Measured(discipline), role) {
		return true
	}
	upTo, ok := j.UpTo[role]
	if !ok {
		return false
	}
	if upTo == "" {
		return true
	}
	levels := c.levelNames(discipline)
	at := slices.Index(levels, level)
	if discipline == Synchro {
		levels, at = c.levelNames(Trampoline), -1
		for _, m := range c.synchroMembers(level) {
			i := slices.Index(levels, m)
			if i < 0 {
				return false // a level trampoline doesn't have: only at any level
			}
			at = max(at, i)
		}
	}
	top := slices.Index(levels, upTo)
	return top < 0 || at <= top
}

// For is the offer's judging for a discipline: trampoline's for synchro,
// which trampoline's judges judge.
func (o Offer) For(discipline string) (JudgeOffer, bool) {
	if discipline == Synchro {
		discipline = Trampoline
	}
	j, ok := o.Judge[discipline]
	return j, ok
}

// synchroMembers are a synchro event's levels, by its name: "ISTO
// Intervanced/Advanced" is ISTO Intervanced and ISTO Advanced.
func (c Competition) synchroMembers(event string) []string {
	for _, l := range c.Synchro {
		if name, err := l.eventName(); err == nil && name == event {
			return MemberNames(l)
		}
	}
	return []string{event}
}

// Describe says what an offer offers, e.g. "Judge Trampoline (execution and
// difficulty up to BUCS L5, chair at any level, and HD) and Synchro (HD and
// synchronisation only); Recorder".
func (o Offer) Describe() string {
	var parts []string
	for _, d := range AllDisciplines {
		j, ok := o.Judge[d]
		if !ok || d == Synchro { // trampoline's judges judge synchro; an offer from before for it counts for nothing
			continue
		}
		parts = append(parts, DisciplineName(d)+" ("+j.describe(d)+")")
	}
	out := ""
	if len(parts) > 0 {
		out = "Judge " + JoinList(parts)
	}
	var help []string
	if o.Recorder {
		help = append(help, "Recorder")
	}
	if o.Marshal {
		help = append(help, "Marshal")
	}
	if len(help) > 0 {
		if out != "" {
			out += "; "
		}
		out += JoinList(help)
	}
	return out
}

// describe says what judging a discipline takes in, e.g. "execution and
// difficulty up to BUCS L5, chair at any level, and HD": roles at the same
// level together, in LevelRoles' order.
func (j JudgeOffer) describe(discipline string) string {
	var levels []string
	roles := map[string][]string{}
	for _, role := range LevelRoles {
		upTo, ok := j.UpTo[role]
		if !ok {
			continue
		}
		if roles[upTo] == nil {
			levels = append(levels, upTo)
		}
		roles[upTo] = append(roles[upTo], roleWords[role])
	}
	var parts []string
	for _, l := range levels {
		at := "at any level"
		if l != "" {
			at = "up to " + l
		}
		parts = append(parts, JoinList(roles[l])+" "+at)
	}
	var measured []string
	for _, role := range Measured(discipline) {
		measured = append(measured, roleWords[role])
	}
	if m := JoinList(measured); m != "" {
		if len(parts) > 0 {
			m = "and " + m
		} else {
			m += " only"
		}
		parts = append(parts, m)
	}
	return strings.Join(parts, ", ")
}

// JudgingWord is a judging role as an offer says it, e.g. "execution".
func JudgingWord(role string) string { return roleWords[role] }

// roleWords are the judging roles as an offer says them.
var roleWords = map[string]string{RoleExecution: "execution", RoleDifficulty: "difficulty", RoleChair: "chair", RoleHD: "HD", RoleSync: "synchronisation"}

// JoinList joins words as "a, b and c".
func JoinList(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	}
	out := parts[0]
	for _, p := range parts[1 : len(parts)-1] {
		out += ", " + p
	}
	return out + " and " + parts[len(parts)-1]
}
