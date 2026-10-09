// setform.go: reading the requirement set editor's form back into a set.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"tariffCalculator/requirements"
)

// The editor names its fields by rule: r<i>.type, r<i>.label, r<i>.min,
// r<i>.max, r<i>.cap, r<i>.position, r<i>.m.<field> for a rule's matcher,
// r<i>.s<j>.<field> for element j of a set routine or requirement j of a
// separate rule (with r<i>.s<j>.label its wording), and r<i>.o<k>.s<j>.<field>
// for element j of option k of an includes rule. "rules", r<i>.seq, r<i>.opts
// and r<i>.o<k>.seq give the counts, and r<i>.was is the rule's kind when the
// form was drawn. Matcher fields: rotmin, rotmax, dir, twmin, twmax, shape,
// takeoff, landing (repeatable checkboxes), tarmin, tarmax, fig.

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
		Format:         requirements.Format,
		Name:           strings.TrimSpace(r.FormValue("name")),
		Description:    strings.TrimSpace(r.FormValue("description")),
		Source:         strings.TrimSpace(r.FormValue("source")),
		NoDifficulty:   r.FormValue("no_difficulty") != "",
		RepeatsAllowed: r.FormValue("repeats_allowed") != "",
		Rules:          []requirements.Rule{},
	}
	if n := intField("scored_elements"); n != nil {
		set.ScoredElements = *n
	}
	for i := range count("rules") {
		p := fmt.Sprintf("r%d.", i)
		rule := requirements.Rule{
			Type:  r.FormValue(p + "type"),
			Label: strings.TrimSpace(r.FormValue(p + "label")),
		}
		switch rule.Type {
		case requirements.Count, requirements.Linked, requirements.Every, requirements.Position:
			m := matcher(p + "m.")
			rule.Match = &m
		case requirements.Sequence:
			// A set routine's elements aren't edited here: they come back as they went out.
			if err := json.Unmarshal([]byte(r.FormValue(p+"seqjson")), &rule.Sequence); err != nil {
				problems = append(problems, fmt.Sprintf("rule %d: the set routine's elements were lost; open it from Your set routines", i+1))
			}
		case requirements.Separate:
			var items []requirements.Matcher
			for j := range count(p + "seq") {
				prefix := fmt.Sprintf("%ss%d.", p, j)
				m := matcher(prefix)
				m.Label = strings.TrimSpace(r.FormValue(prefix + "label"))
				items = append(items, m)
			}
			rule.Each = items
		case requirements.Includes:
			rule.Options = [][]requirements.Matcher{}
			for k := range count(p + "opts") {
				var steps []requirements.Matcher
				for j := range count(fmt.Sprintf("%so%d.seq", p, k)) {
					prefix := fmt.Sprintf("%so%d.s%d.", p, k, j)
					m := matcher(prefix)
					m.Label = strings.TrimSpace(r.FormValue(prefix + "label"))
					steps = append(steps, m)
				}
				rule.Options = append(rule.Options, steps)
			}
		}
		if rule.Type == requirements.Count || rule.Type == requirements.Linked || rule.Type == requirements.Elements || rule.Type == requirements.Difficulty {
			rule.Min, rule.Max = floatField(p+"min"), floatField(p+"max")
		}
		if rule.Type == requirements.Difficulty {
			rule.Cap = floatField(p + "cap")
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
	case requirements.Linked:
		somersault, none := 3, 0.0
		return requirements.Rule{Type: ruleType, Match: &requirements.Matcher{Rotation: &requirements.Range{Min: &somersault}}, Max: &none}
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
	case requirements.Separate:
		return requirements.Rule{Type: ruleType, Each: []requirements.Matcher{{}}}
	case requirements.Includes:
		return requirements.Rule{Type: ruleType, Options: [][]requirements.Matcher{{{}}}}
	}
	return requirements.Rule{Type: ruleType}
}

// applySetAction applies an editor button ("delete:2", "up:1", "down:0",
// "add-element:3", "delete-element:3:1", "add-option:3", "delete-option:3:0",
// "add-step:3:0", "delete-step:3:0:1") or the "Add a rule" choice to a set.
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
	case "add-element", "delete-element":
		// The elements of a set routine, or the requirements of a separate rule.
		items := &set.Rules[i].Sequence
		if set.Rules[i].Type == requirements.Separate {
			items = &set.Rules[i].Each
		}
		if parts[0] == "add-element" {
			*items = append(*items, requirements.Matcher{})
		} else if j := index(2); j >= 0 && j < len(*items) {
			*items = slices.Delete(*items, j, j+1)
		}
	case "add-option":
		set.Rules[i].Options = append(set.Rules[i].Options, []requirements.Matcher{{}})
	case "delete-option", "add-step", "delete-step":
		options := set.Rules[i].Options
		k := index(2)
		if k < 0 || k >= len(options) {
			return
		}
		switch parts[0] {
		case "delete-option":
			set.Rules[i].Options = slices.Delete(options, k, k+1)
		case "add-step":
			options[k] = append(options[k], requirements.Matcher{})
		case "delete-step":
			if j := index(3); j >= 0 && j < len(options[k]) {
				options[k] = slices.Delete(options[k], j, j+1)
			}
		}
	}
}
