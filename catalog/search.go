package catalog

import (
	"sort"
	"strconv"
	"strings"
	"unicode"

	"tariffCalculator/skills"
)

// Result is a search match: a skill, named and priced, ready to load into the editor.
type Result struct {
	Skill        skills.TrampolineSkill
	FromNotation bool // parsed from FIG notation, so its direction is a guess the user picks between
}

// maxResults caps how many matches Search returns.
const maxResults = 12

// aliases are other names people use for common skills (by CommonSkills key).
var aliases = map[string][]string{
	"backSomersault": {"back somersault", "back salto", "back s/s"},
	"front":          {"front somersault", "front salto", "front s/s"},
	"backToSeat":     {"back s/s to seat", "back somersault to seat"},
	"frontToSeat":    {"front s/s to seat", "front somersault to seat"},
	"crashDive":      {"3/4 front"},
	"lazyBack":       {"3/4 back"},
	"halfOut":        {"barani out", "half in"},
	"halfhalf":       {"half in half out"},
	"fullFull":       {"full in full out"},
	"fullRudi":       {"full in rudy out", "full in rudi out"},
	"miller":         {"1 1/2 in 1 1/2 out"},
	"doubleBack":     {"double back somersault"},
	"tripleBack":     {"triffis"},
}

// tokenAliases rewrite single words people spell differently from the app's names.
var tokenAliases = map[string]string{
	"rudy": "rudi", "rudolph": "rudi", "randy": "randi", "randolph": "randi",
	"jumps": "jump",
}

// Search finds skills for a query: by name (each common skill in each shape it
// can be done in, plus a few aliases) or, if the query looks like FIG notation,
// by parsing it. An empty query matches nothing.
func Search(query string) []Result {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	if looksLikeNotation(query) {
		return limit(fromNotation(query))
	}
	return limit(byName(query))
}

func limit(rs []Result) []Result {
	if len(rs) > maxResults {
		return rs[:maxResults]
	}
	return rs
}

// --- By name ---

// normalize keeps only lower-case letters and digits, so "Ball-Out" matches "ballout".
func normalize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// tokens splits a query into normalised words, applying tokenAliases.
func tokens(query string) []string {
	var out []string
	for _, word := range strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '/'
	}) {
		w := normalize(word)
		if a, ok := tokenAliases[w]; ok {
			w = a
		}
		if w != "" {
			out = append(out, w)
		}
	}
	return out
}

// variants is every shape a common skill can be done in, named and priced.
// A straight jump is left out: it interrupts a routine rather than scoring.
func variants(common skills.TrampolineSkill) []skills.TrampolineSkill {
	shapes := []skills.Shape{common.Shape}
	switch {
	case common.IsBasicJump():
		shapes = []skills.Shape{skills.Tuck, skills.Pike, skills.Straddle}
	case common.ShapeIsRelevant():
		shapes = []skills.Shape{skills.Tuck, skills.Pike, skills.Straight}
	}
	out := make([]skills.TrampolineSkill, 0, len(shapes))
	for _, shape := range shapes {
		s := common
		s.Shape = shape
		out = append(out, priced(s))
	}
	return out
}

func priced(s skills.TrampolineSkill) skills.TrampolineSkill {
	s.NormalizePhases()
	s.Name = skills.FindCommonSkillName(s)
	s.SetTariff()
	return s
}

func byName(query string) []Result {
	words := tokens(query)
	if len(words) == 0 {
		return nil
	}
	whole := normalize(query)

	type match struct {
		skill  skills.TrampolineSkill
		prefix bool // the name starts with the query
	}
	var matches []match
	for key, common := range skills.CommonSkills {
		for _, s := range variants(common) {
			texts := []string{normalize(s.Name)}
			for _, a := range aliases[key] {
				texts = append(texts, normalize(a))
			}
			if !anyContainsAll(texts, words) {
				continue
			}
			matches = append(matches, match{skill: s, prefix: strings.HasPrefix(normalize(s.Name), whole) || strings.HasPrefix(normalize(shapeLast(s)), whole)})
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		a, b := matches[i], matches[j]
		if a.prefix != b.prefix {
			return a.prefix
		}
		// Shorter names are closer matches: "Barani Tuck" before "Barani To Front Tuck".
		if len(a.skill.Name) != len(b.skill.Name) {
			return len(a.skill.Name) < len(b.skill.Name)
		}
		if a.skill.Tariff != b.skill.Tariff {
			return a.skill.Tariff < b.skill.Tariff
		}
		return a.skill.Name < b.skill.Name
	})
	out := make([]Result, len(matches))
	for i, m := range matches {
		out[i] = Result{Skill: m.skill}
	}
	return out
}

// shapeLast is a skill's name with any leading shape moved to the end, so
// "barani pike" is as close a match for "Pike Barani" as "barani" is.
func shapeLast(s skills.TrampolineSkill) string {
	shape := s.Shape.String() + " "
	if strings.HasPrefix(s.Name, shape) {
		return strings.TrimPrefix(s.Name, shape) + " " + s.Shape.String()
	}
	return s.Name
}

// anyContainsAll reports whether one of texts contains every word.
func anyContainsAll(texts, words []string) bool {
	for _, text := range texts {
		all := true
		for _, w := range words {
			if !strings.Contains(text, normalize(w)) {
				all = false
				break
			}
		}
		if all {
			return true
		}
	}
	return false
}

// --- By FIG notation ---

// shapeSymbols are the FIG notation shape marks.
var shapeSymbols = map[rune]skills.Shape{'o': skills.Tuck, '<': skills.Pike, '/': skills.Straight, 'v': skills.Straddle}

// looksLikeNotation reports whether a query is FIG notation rather than a name:
// it starts with a digit (after an optional opening bracket) and has no letters
// other than shape marks.
func looksLikeNotation(q string) bool {
	q = strings.TrimLeft(q, "( ")
	if q == "" || !unicode.IsDigit(rune(q[0])) {
		return false
	}
	for _, r := range q {
		if unicode.IsLetter(r) && r != 'o' && r != 'v' {
			return false
		}
	}
	return true
}

// fromNotation parses FIG notation such as "8 1 1 <", "(4 - o)", "811<" or the
// CoP's "8 01 <": quarter somersaults, then half twists per somersault ("-" is
// none), then an optional shape mark. Notation doesn't record direction, so each
// reading is offered forwards and backwards (named skills first); without a
// shape mark, a skill whose shape matters is offered in each shape. Skills take
// off from feet; readings that can't land are left out.
func fromNotation(q string) []Result {
	q = strings.TrimSpace(strings.Trim(strings.TrimSpace(q), "()"))
	var shape *skills.Shape
	if r := []rune(q); len(r) > 0 {
		if sh, ok := shapeSymbols[r[len(r)-1]]; ok {
			shape = &sh
			q = strings.TrimSpace(string(r[:len(r)-1]))
		}
	}
	q = strings.ReplaceAll(q, "-", " 0 ")

	var readings []skills.TrampolineSkill
	for _, rt := range rotationReadings(q) {
		phases := skills.CalculatePhases(rt.rotation)
		if len(rt.twists) > phases {
			continue
		}
		twists := make([]int, phases)
		copy(twists, rt.twists)
		readings = append(readings, skills.TrampolineSkill{Rotation: rt.rotation, TwistDistribution: twists, TakeoffPosition: skills.Feet})
	}

	var named, unnamed []Result
	seen := map[string]bool{}
	for _, base := range readings {
		for _, backward := range []bool{false, true} {
			for _, sh := range shapesToTry(base, shape) {
				s := base
				s.Backward, s.Shape = backward, sh
				// Skip what can't be scored or can't land (e.g. "20", landing on the head).
				if s.Validate() != nil || s.LandingPosition() == skills.Invalid {
					continue
				}
				s = priced(s)
				key := s.FIGNotation() + strconv.FormatBool(backward)
				if seen[key] {
					continue
				}
				seen[key] = true
				if s.Name == "Custom Skill" {
					unnamed = append(unnamed, Result{Skill: s, FromNotation: true})
				} else {
					named = append(named, Result{Skill: s, FromNotation: true})
				}
			}
		}
	}
	return append(named, unnamed...)
}

// shapesToTry is the given shape, or each shape that makes a different skill.
func shapesToTry(s skills.TrampolineSkill, given *skills.Shape) []skills.Shape {
	if given != nil {
		return []skills.Shape{*given}
	}
	if !s.ShapeIsRelevant() {
		return []skills.Shape{skills.Straight}
	}
	if s.IsBasicJump() {
		return []skills.Shape{skills.Tuck, skills.Pike, skills.Straddle}
	}
	return []skills.Shape{skills.Tuck, skills.Pike, skills.Straight}
}

type rotationReading struct {
	rotation int
	twists   []int
}

// rotationReadings splits the number part of notation into a rotation and its
// twist digits. With spaces the first group is the rotation; run together
// ("811") the rotation may be one digit or, from 10 to 16, two.
func rotationReadings(q string) []rotationReading {
	groups := strings.Fields(q)
	if len(groups) == 0 {
		return nil
	}
	for _, g := range groups {
		if _, err := strconv.Atoi(g); err != nil {
			return nil
		}
	}
	twistDigits := func(gs []string) []int {
		var d []int
		for _, g := range gs {
			for _, r := range g {
				d = append(d, int(r-'0'))
			}
		}
		return d
	}

	if len(groups) > 1 {
		rot, _ := strconv.Atoi(groups[0])
		return []rotationReading{{rot, twistDigits(groups[1:])}}
	}
	digits := groups[0]
	var out []rotationReading
	for n := 1; n <= 2 && n <= len(digits); n++ {
		rot, _ := strconv.Atoi(digits[:n])
		if n == 2 && rot < 10 {
			continue
		}
		out = append(out, rotationReading{rot, twistDigits([]string{digits[n:]})})
	}
	return out
}
