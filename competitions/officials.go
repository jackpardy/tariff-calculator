package competitions

import (
	"errors"
	"fmt"
	"slices"
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

// Who may judge what: anyone up to the level they say (the default), only
// levels below the one they compete at in that discipline, or only people the
// organiser marks as qualified (up to the level they say).
const (
	JudgeDeclared  = ""
	JudgeBelow     = "below"
	JudgeQualified = "qualified"
)

// OfficialSettings are the organiser's settings for officials.
type OfficialSettings struct {
	Panels map[string]Panel `json:"panels,omitempty"` // by discipline; missing: the Code of Points'
	Judge  string           `json:"judge,omitempty"`  // JudgeDeclared, JudgeBelow or JudgeQualified
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
	if !slices.Contains([]string{JudgeDeclared, JudgeBelow, JudgeQualified}, s.Judge) {
		return fmt.Errorf("unknown judging rule %q", s.Judge)
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

// Offer is what a person can do as an official: judge some disciplines, up to
// a level, perhaps chairing, and help as a recorder or marshal.
type Offer struct {
	Judge    map[string]JudgeOffer `json:"judge,omitempty"` // by discipline
	Recorder bool                  `json:"recorder,omitempty"`
	Marshal  bool                  `json:"marshal,omitempty"`
}

// JudgeOffer is judging one discipline.
type JudgeOffer struct {
	UpTo  string `json:"up_to,omitempty"` // the highest level they'll judge; "" for any
	Chair bool   `json:"chair,omitempty"` // they can chair the panel
}

// Empty says whether the offer offers nothing.
func (o Offer) Empty() bool { return len(o.Judge) == 0 && !o.Recorder && !o.Marshal }

// ValidateOffer reports what's wrong with an offer for this competition:
// disciplines it doesn't offer, or levels it doesn't have.
func (c Competition) ValidateOffer(o Offer) error {
	var errs []error
	for d, j := range o.Judge {
		if !slices.Contains(c.Disciplines(), d) {
			errs = append(errs, fmt.Errorf("the competition doesn't offer %s", DisciplineName(d)))
			continue
		}
		if j.UpTo != "" && !slices.Contains(c.levelNames(d), j.UpTo) {
			errs = append(errs, fmt.Errorf("%s has no level %q", DisciplineName(d), j.UpTo))
		}
	}
	return errors.Join(errs...)
}

// CanJudge says whether an official may judge a discipline's level under the
// competition's rule. qualified is the organiser's mark; competing are the
// levels they compete at, by discipline.
func (c Competition) CanJudge(o Offer, qualified bool, competing map[string]string, discipline, level string) bool {
	j, ok := o.Judge[discipline]
	if !ok {
		return false
	}
	levels := c.levelNames(discipline)
	at := slices.Index(levels, level)
	if at < 0 {
		return false
	}
	if j.UpTo != "" {
		if top := slices.Index(levels, j.UpTo); top >= 0 && at > top {
			return false
		}
	}
	switch c.Officials.Judge {
	case JudgeQualified:
		return qualified
	case JudgeBelow:
		own, competes := competing[discipline]
		if !competes {
			return true
		}
		return at < slices.Index(levels, own)
	}
	return true
}

// Describe says what an offer offers, e.g. "Judge trampoline up to BUCS L5
// (can chair), synchro; recorder".
func (o Offer) Describe() string {
	var parts []string
	for _, d := range AllDisciplines {
		j, ok := o.Judge[d]
		if !ok {
			continue
		}
		part := DisciplineName(d)
		if j.UpTo != "" {
			part += " up to " + j.UpTo
		}
		if j.Chair {
			part += " (can chair)"
		}
		parts = append(parts, part)
	}
	out := ""
	if len(parts) > 0 {
		out = "Judge " + joinList(parts)
	}
	var help []string
	if o.Recorder {
		help = append(help, "recorder")
	}
	if o.Marshal {
		help = append(help, "marshal")
	}
	if len(help) > 0 {
		if out != "" {
			out += "; "
		}
		out += joinList(help)
	}
	return out
}

func joinList(parts []string) string {
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
