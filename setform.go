// setform.go: reading the requirement set editor's form back into a set.
package main

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"tariffCalculator/requirements"
)

// The editor names its fields by rule: r<i>.type, r<i>.label, r<i>.min,
// r<i>.max, r<i>.position, r<i>.m.<field> for a rule's matcher, and
// r<i>.s<j>.<field> for element j of a set routine. "rules" and r<i>.seq give
// the counts, and r<i>.was is the rule's kind when the form was drawn. Matcher
// fields: rotmin, rotmax, dir, twmin, twmax, shape, takeoff, landing
// (repeatable checkboxes), tarmin, tarmax, fig.

// parseSetForm reads the editor's form into a set, collecting any number that
// couldn't be read; validation of the set itself happens afterwards.
func parseSetForm(r *http.Request) (requirements.Set, []string) {
	var problems []string
	intField := func(name string) *int {
		v := strings.TrimSpace(r.FormValue(name))
		if v == "" {
			return nil
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%q isn't a whole number", v))
			return nil
		}
		return &n
	}
	floatField := func(name string) *float64 {
		v := strings.TrimSpace(r.FormValue(name))
		if v == "" {
			return nil
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%q isn't a number", v))
			return nil
		}
		return &f
	}
	count := func(name string) int {
		n, _ := strconv.Atoi(r.FormValue(name))
		return max(n, 0)
	}
	matcher := func(prefix string) requirements.Matcher {
		m := requirements.Matcher{
			Direction: r.FormValue(prefix + "dir"),
			Shapes:    r.Form[prefix+"shape"],
			Takeoff:   r.Form[prefix+"takeoff"],
			Landing:   r.Form[prefix+"landing"],
			FIG:       strings.TrimSpace(r.FormValue(prefix + "fig")),
		}
		if lo, hi := intField(prefix+"rotmin"), intField(prefix+"rotmax"); lo != nil || hi != nil {
			m.Rotation = &requirements.Range{Min: lo, Max: hi}
		}
		if lo, hi := intField(prefix+"twmin"), intField(prefix+"twmax"); lo != nil || hi != nil {
			m.Twist = &requirements.Range{Min: lo, Max: hi}
		}
		if lo, hi := floatField(prefix+"tarmin"), floatField(prefix+"tarmax"); lo != nil || hi != nil {
			m.Tariff = &requirements.TariffRange{Min: lo, Max: hi}
		}
		return m
	}

	set := requirements.Set{
		Format:      requirements.Format,
		Name:        strings.TrimSpace(r.FormValue("name")),
		Description: strings.TrimSpace(r.FormValue("description")),
		Source:      strings.TrimSpace(r.FormValue("source")),
		Rules:       []requirements.Rule{},
	}
	for i := range count("rules") {
		p := fmt.Sprintf("r%d.", i)
		rule := requirements.Rule{
			Type:  r.FormValue(p + "type"),
			Label: strings.TrimSpace(r.FormValue(p + "label")),
		}
		switch rule.Type {
		case requirements.Count, requirements.Every, requirements.Position:
			m := matcher(p + "m.")
			rule.Match = &m
		case requirements.Sequence:
			for j := range count(p + "seq") {
				rule.Sequence = append(rule.Sequence, matcher(fmt.Sprintf("%ss%d.", p, j)))
			}
		}
		if rule.Type == requirements.Count || rule.Type == requirements.Elements || rule.Type == requirements.Difficulty {
			rule.Min, rule.Max = floatField(p+"min"), floatField(p+"max")
		}
		if rule.Type == requirements.Position {
			if n := intField(p + "position"); n != nil {
				rule.Position = *n
			}
		}
		// A rule whose kind was just changed starts from that kind's defaults,
		// keeping its wording and, where the new kind has one, its matcher.
		if was := r.FormValue(p + "was"); was != "" && was != rule.Type {
			fresh := newRule(rule.Type)
			fresh.Label = rule.Label
			if fresh.Match != nil && rule.Match != nil {
				fresh.Match = rule.Match
			}
			rule = fresh
		}
		set.Rules = append(set.Rules, rule)
	}
	return set, problems
}

// newRule is a rule of the given type with sensible starting values.
func newRule(ruleType string) requirements.Rule {
	one, ten := 1.0, 10.0
	switch ruleType {
	case requirements.Count:
		return requirements.Rule{Type: ruleType, Match: &requirements.Matcher{}, Min: &one}
	case requirements.Every:
		return requirements.Rule{Type: ruleType, Match: &requirements.Matcher{}}
	case requirements.Elements:
		return requirements.Rule{Type: ruleType, Min: &ten, Max: &ten}
	case requirements.Difficulty:
		return requirements.Rule{Type: ruleType, Min: new(float64)}
	case requirements.Position:
		return requirements.Rule{Type: ruleType, Position: 10, Match: &requirements.Matcher{}}
	case requirements.Sequence:
		return requirements.Rule{Type: ruleType, Sequence: []requirements.Matcher{{}}}
	}
	return requirements.Rule{Type: ruleType}
}

// applySetAction applies an editor button ("delete:2", "up:1", "down:0",
// "add-element:3", "delete-element:3:1") or the "Add a rule" choice to a set.
func applySetAction(set *requirements.Set, action, add string) {
	if slices.Contains(requirements.RuleTypes, add) {
		set.Rules = append(set.Rules, newRule(add))
	}
	parts := strings.Split(action, ":")
	index := func(k int) int {
		if k >= len(parts) {
			return -1
		}
		n, err := strconv.Atoi(parts[k])
		if err != nil {
			return -1
		}
		return n
	}
	i := index(1)
	if i < 0 || i >= len(set.Rules) {
		return
	}
	switch parts[0] {
	case "delete":
		set.Rules = slices.Delete(set.Rules, i, i+1)
	case "up":
		if i > 0 {
			set.Rules[i-1], set.Rules[i] = set.Rules[i], set.Rules[i-1]
		}
	case "down":
		if i < len(set.Rules)-1 {
			set.Rules[i+1], set.Rules[i] = set.Rules[i], set.Rules[i+1]
		}
	case "add-element":
		set.Rules[i].Sequence = append(set.Rules[i].Sequence, requirements.Matcher{})
	case "delete-element":
		if j := index(2); j >= 0 && j < len(set.Rules[i].Sequence) {
			set.Rules[i].Sequence = slices.Delete(set.Rules[i].Sequence, j, j+1)
		}
	}
}
