package requirements

import (
	"fmt"
	"strconv"
	"strings"
)

// Describe is the rule in plain English: its label if the author gave one,
// otherwise generated from the rule, e.g. "At least 1 element: backward, at
// least 1 somersault".
func Describe(r Rule) string {
	if strings.TrimSpace(r.Label) != "" {
		return r.Label
	}
	switch r.Type {
	case Count:
		return countPhrase(r.Min, r.Max) + ": " + DescribeMatcher(*r.Match)
	case Every:
		return "Every element: " + DescribeMatcher(*r.Match)
	case Elements:
		return capitalise(bounds(r.Min, r.Max, wholeNumber, "elements"))
	case Difficulty:
		return "Difficulty " + bounds(r.Min, r.Max, tenths, "")
	case Position:
		return fmt.Sprintf("Element %d: %s", r.Position, DescribeMatcher(*r.Match))
	case Sequence:
		return fmt.Sprintf("Set routine of %d elements", len(r.Sequence))
	}
	return r.Type
}

// DescribeMatcher lists a matcher's conditions, e.g. "backward, in tuck or pike".
func DescribeMatcher(m Matcher) string {
	var parts []string
	if m.FIG != "" {
		parts = append(parts, "FIG "+m.FIG)
	}
	if m.Direction != "" {
		parts = append(parts, m.Direction)
	}
	if m.Rotation != nil {
		parts = append(parts, rangePhrase(m.Rotation, somersaults, "somersault", 4))
	}
	if m.Twist != nil {
		if m.Twist.Max != nil && *m.Twist.Max == 0 && m.Twist.Min == nil {
			parts = append(parts, "no twist")
		} else {
			parts = append(parts, rangePhrase(m.Twist, twists, "twist", 2))
		}
	}
	if len(m.Shapes) > 0 {
		parts = append(parts, "in "+orList(m.Shapes))
	}
	if len(m.Takeoff) > 0 {
		parts = append(parts, "from "+orList(m.Takeoff))
	}
	if len(m.Landing) > 0 {
		parts = append(parts, "landing on "+orList(m.Landing))
	}
	if m.Tariff != nil {
		parts = append(parts, "tariff "+bounds(m.Tariff.Min, m.Tariff.Max, tenths, ""))
	}
	if len(parts) == 0 {
		return "any element"
	}
	return strings.Join(parts, ", ")
}

func countPhrase(min, max *float64) string {
	switch {
	case max != nil && *max == 0:
		return "No elements"
	case min != nil && max != nil && *min == *max:
		return fmt.Sprintf("Exactly %s", elementsWord(*min))
	case min != nil && max != nil:
		return fmt.Sprintf("%s to %s", wholeNumber(*min), elementsWord(*max))
	case min != nil:
		return "At least " + elementsWord(*min)
	default:
		return "At most " + elementsWord(*max)
	}
}

func elementsWord(n float64) string {
	return wholeNumber(n) + " " + plural(int(n), "element", "elements")
}

// bounds phrases a min/max pair, e.g. "at least 3.0", "2 to 4 elements".
func bounds(min, max *float64, format func(float64) string, unit string) string {
	suffix := ""
	if unit != "" {
		suffix = " " + unit
	}
	switch {
	case min != nil && max != nil && *min == *max:
		return "exactly " + format(*min) + suffix
	case min != nil && max != nil:
		return format(*min) + " to " + format(*max) + suffix
	case min != nil:
		return "at least " + format(*min) + suffix
	case max != nil:
		return "at most " + format(*max) + suffix
	}
	return "any"
}

// rangePhrase phrases a whole-number range read through format (e.g. quarters
// as somersaults), e.g. "at least 1¼ somersaults". one is the value that reads
// as one unit (4 quarters, 2 half twists); anything more, or 0, is plural.
func rangePhrase(r *Range, format func(int) string, unit string, one int) string {
	word := func(v int) string {
		if v == 0 || v > one {
			return format(v) + " " + unit + "s"
		}
		return format(v) + " " + unit
	}
	switch {
	case r.Min != nil && r.Max != nil && *r.Min == *r.Max:
		return word(*r.Min)
	case r.Min != nil && r.Max != nil:
		return format(*r.Min) + " to " + word(*r.Max)
	case r.Min != nil:
		return "at least " + word(*r.Min)
	case r.Max != nil:
		return "at most " + word(*r.Max)
	}
	return "any " + unit
}

// somersaults reads quarter somersaults, e.g. 5 -> "1¼".
func somersaults(q int) string { return quarters(q) }

// twists reads half twists, e.g. 3 -> "1½".
func twists(h int) string { return quarters(h * 2) }

func quarters(n int) string {
	if n == 0 {
		return "0"
	}
	whole, frac := n/4, []string{"", "¼", "½", "¾"}[n%4]
	if whole == 0 {
		return frac
	}
	return strconv.Itoa(whole) + frac
}

func wholeNumber(v float64) string { return strconv.Itoa(int(v)) }
func tenths(v float64) string      { return strconv.FormatFloat(v, 'f', 1, 64) }

func orList(items []string) string {
	if len(items) <= 1 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " or " + items[len(items)-1]
}

func capitalise(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
