package catalog

import (
	"slices"
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

// aliases are other names people use for common skills (by CommonSkills key),
// besides the rule books' names in otherNames. Written as people type them:
// they're read the same way as a query ("½", "1/2" and "half" alike).
var aliases = map[string][]string{
	"halfTwist":         {"half twist jump"},
	"fullTwist":         {"full twist jump"},
	"seatToFeet":        {"seat drop to feet"},
	"seatHalfToFeet":    {"seat drop half twist to feet"},
	"backToFeet":        {"back drop to feet"},
	"backHalfToFeet":    {"back drop half twist to feet"},
	"frontToFeet":       {"front drop to feet"},
	"backSomersault":    {"back somersault", "back salto"},
	"front":             {"front somersault", "front salto"},
	"backToSeat":        {"back somersault to seat"},
	"frontToSeat":       {"front somersault to seat"},
	"crashDive":         {"3/4 front"},
	"lazyBack":          {"3/4 back"},
	"backHalf":          {"back half twist", "back somersault with 1/2 twist", "back s/s half twist"},
	"frontFull":         {"front full twist", "full front"},
	"fullBack":          {"back full", "full twist back"},
	"doubleFullBack":    {"double full", "back double full"},
	"tripleFullBack":    {"triple full", "back triple full"},
	"ada":               {"adolph", "adolf", "3 1/2 twisting front"},
	"doubleBack":        {"double back somersault", "double back s/s"},
	"doubleFront":       {"double front somersault", "double front s/s"},
	"halfOut":           {"barani out"},
	"halfhalf":          {"half in half out"},
	"fullInBack":        {"full in", "full in double back"},
	"fullOutBack":       {"full out", "back in full out", "full out double back"},
	"fullFull":          {"full in full out"},
	"fullRudi":          {"full in rudi out"},
	"miller":            {"1 1/2 in 1 1/2 out"},
	"tripleBack":        {"triple back somersault"},
	"trifHalfOut":       {"half out triffis", "barani out triffis", "front front half"},
	"trifRudiOut":       {"rudi out triffis", "front front rudi"},
	"trifHalfHalf":      {"half in half out triffis", "half front half"},
	"trifHalfInRudiOut": {"half in rudi out triffis", "half front rudi"},
	"trifFullInHalfOut": {"full in half out triffis", "full front half"},
	"fullInHalfOut":     {"full half"},
	"fullRandi":         {"full in randi out"},
}

// tokenAliases rewrite single words people spell differently from the app's
// names, and the other words for a shape.
var tokenAliases = map[string]string{
	"rudy": "rudi", "rudolph": "rudi", "randy": "randi", "randolph": "randi",
	"jumps":  "jump",
	"tucked": "tuck", "piked": "pike", "layout": "straight", "straddled": "straddle",
}

// phrases rewrite what's written more than one way before it's split into
// words: "½ twist", "1/2 twist" and "half twist" are the same.
var phrases = strings.NewReplacer("½", " half ", "1/2", " half ", "laid out", "layout")

// shapeWords are the words that name a shape (after tokenAliases).
var shapeWords = map[string]skills.Shape{"tuck": skills.Tuck, "pike": skills.Pike, "straight": skills.Straight, "straddle": skills.Straddle}

// Search finds skills for a query: by name (each common skill in each shape it
// can be done in, plus aliases) or, if the query looks like FIG notation, by
// parsing it. An empty query matches nothing.
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

// tokens splits text into normalised words, after phrases and applying
// tokenAliases.
func tokens(text string) []string {
	var out []string
	for _, word := range strings.FieldsFunc(phrases.Replace(strings.ToLower(text)), func(r rune) bool {
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

// texts are what a query is matched against for a skill, as runs of
// normalised words: its name without its shape (which a query's shape word
// picks), its rule books' name and its aliases.
func texts(key string, s skills.TrampolineSkill) []string {
	shaped := s.IsBasicJump() || s.ShapeIsRelevant()
	var name []string
	for _, w := range tokens(s.Name) {
		if shaped && w == strings.ToLower(s.Shape.String()) {
			shaped = false // only the once: the shape in "Tuck Jump", not a word of the name
			continue
		}
		name = append(name, w)
	}
	out := []string{strings.Join(name, "")}
	others := aliases[key]
	if other, ok := otherNames[PickerName(s)]; ok {
		others = append([]string{other}, others...)
	}
	for _, a := range others {
		out = append(out, strings.Join(tokens(a), ""))
	}
	return out
}

// byName matches each common skill in each shape against a query's words:
// any shape word picks the shape, and the other words must all be in the
// skill's name or one of its other names. Exact names come first, then names
// starting with the query, then the rest, shorter names first.
func byName(query string) []Result {
	var words []string
	var shape *skills.Shape
	for _, w := range tokens(query) {
		if sh, ok := shapeWords[w]; ok {
			shape = &sh
			continue
		}
		words = append(words, w)
	}
	if len(words) == 0 && shape == nil {
		return nil
	}
	whole := strings.Join(words, "")

	type match struct {
		skill skills.TrampolineSkill
		rank  int // 0 an exact name, 1 one starting with the query, 2 the rest
	}
	var matches []match
	for key, common := range skills.CommonSkills {
		for _, s := range variants(common) {
			if shape != nil && (s.Shape != *shape || !(s.IsBasicJump() || s.ShapeIsRelevant())) {
				continue
			}
			rank, found := 2, false
			for _, text := range texts(key, s) {
				if !containsAll(text, words) {
					continue
				}
				found = true
				switch {
				case text == whole:
					rank = 0
				case strings.HasPrefix(text, whole):
					rank = min(rank, 1)
				}
			}
			if found {
				matches = append(matches, match{s, rank})
			}
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		a, b := matches[i], matches[j]
		if a.rank != b.rank {
			return a.rank < b.rank
		}
		// Shorter names are closer matches: "Barani Tuck" before "Barani To Front Tuck".
		if len(a.skill.Name) != len(b.skill.Name) {
			return len(a.skill.Name) < len(b.skill.Name)
		}
		if a.skill.Tariff != b.skill.Tariff {
			return a.skill.Tariff < b.skill.Tariff
		}
		// Tuck, pike, then straight, as the picker has them.
		if sa, sb := slices.Index(pickerShapes, a.skill.Shape), slices.Index(pickerShapes, b.skill.Shape); sa != sb {
			return sa < sb
		}
		return a.skill.Name < b.skill.Name
	})
	out := make([]Result, len(matches))
	for i, m := range matches {
		out[i] = Result{Skill: m.skill}
	}
	return out
}

// containsAll reports whether text contains every word.
func containsAll(text string, words []string) bool {
	for _, w := range words {
		if !strings.Contains(text, w) {
			return false
		}
	}
	return true
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
// none), then an optional shape mark. Notation doesn't record direction or
// take-off, so each reading is offered forwards and backwards, from feet first
// and then from the back, front and seat (named skills first); without a
// shape mark, a skill whose shape matters is offered in each shape. Readings
// that can't land are left out, and so are unnamed ones from the body when
// another reading has a name.
func fromNotation(q string) []Result {
	readings, shape := notationReadings(q)
	var named, unnamed []Result
	seen := map[string]bool{}
	for _, takeoff := range []skills.BodyPosition{skills.Feet, skills.Back, skills.Front, skills.Seat} {
		for _, reading := range readings {
			base := reading
			base.TakeoffPosition = takeoff
			for _, backward := range []bool{false, true} {
				for _, sh := range shapesToTry(base, shape) {
					s := base
					s.Backward, s.Shape = backward, sh
					// Skip what can't be scored or can't land (e.g. "20", landing on the head).
					if s.Validate() != nil || s.LandingPosition() == skills.Invalid {
						continue
					}
					s = priced(s)
					key := s.FIGNotation() + strconv.FormatBool(backward) + takeoff.String()
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
	}
	// Where a reading has a name, the unnamed ones from the back, front or
	// seat are only noise: "8 2 - o" is a Full-In Back, or a front one.
	if len(named) > 0 {
		unnamed = slices.DeleteFunc(unnamed, func(r Result) bool { return r.Skill.TakeoffPosition != skills.Feet })
	}
	return append(named, unnamed...)
}

// notationReadings are the skills FIG notation could be, from feet (its
// rotation and twists read each way they can be), and its shape mark, if any.
func notationReadings(q string) ([]skills.TrampolineSkill, *skills.Shape) {
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
	return readings, shape
}

// NotationReading is what FIG notation says of a skill, for the builder to
// finish when no reading of it can land: its first reading, from feet, in its
// shape (straight, or tuck where the shape matters, without a mark). ok is
// false when the query isn't notation it can read.
func NotationReading(q string) (skills.TrampolineSkill, bool) {
	q = strings.TrimSpace(q)
	if !looksLikeNotation(q) {
		return skills.TrampolineSkill{}, false
	}
	readings, shape := notationReadings(q)
	if len(readings) == 0 {
		return skills.TrampolineSkill{}, false
	}
	s := readings[0]
	s.Shape = skills.Straight
	if shape != nil {
		s.Shape = *shape
	} else if s.ShapeIsRelevant() {
		s.Shape = skills.Tuck
	}
	if s.Validate() != nil {
		return skills.TrampolineSkill{}, false
	}
	s.NormalizePhases()
	return s, true
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
