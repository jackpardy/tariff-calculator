// Package catalog organises the common skills for choosing them: grouped into
// categories for the skill picker, and searchable by name or FIG notation.
package catalog

import (
	"sort"
	"strings"

	"tariffCalculator/skills"
)

// Entry is a common skill, named and priced in its usual shape.
type Entry struct {
	Key   string // key in skills.CommonSkills
	Skill skills.TrampolineSkill
}

// Category is a group of entries shown under one picker tab.
type Category struct {
	Key     string
	Label   string
	Entries []Entry
}

// categoryOrder is the picker's tab order, from the simplest skills up.
var categoryOrder = []struct{ Key, Label string }{
	{"jumps", "Jumps"},
	{"drops", "Drops & seat"},
	{"somersaults", "Somersaults"},
	{"twists", "Twists"},
	{"doubles", "Doubles"},
	{"triples", "Triples"},
}

// categoryOf places a skill by what it is: its rotation, twist and whether it
// involves front, back or seat.
func categoryOf(s skills.TrampolineSkill) string {
	switch {
	case s.Rotation >= 12:
		return "triples"
	case s.Rotation >= 8:
		return "doubles"
	case s.Rotation >= 3 && s.TotalTwist() > 0:
		return "twists"
	case s.Rotation >= 3:
		return "somersaults"
	case s.Rotation == 0 && s.TakeoffPosition == skills.Feet && !s.SeatLanding:
		return "jumps"
	default:
		return "drops"
	}
}

// somersaultNames spell out base names that don't say they're somersaults once
// the shape is dropped ("Back Tuck" becomes "Back Somersault", not "Back").
var somersaultNames = map[string]string{
	"Back":          "Back Somersault",
	"Front":         "Front Somersault",
	"Back To Seat":  "Back Somersault To Seat",
	"Front To Seat": "Front Somersault To Seat",
}

// PickerName is a skill's name in the picker: without its shape, which is
// chosen on the skill card, except for shaped jumps, where the shape is the
// skill ("Tuck Jump").
func PickerName(s skills.TrampolineSkill) string {
	name := skills.FindCommonSkillName(s)
	if !s.IsBasicJump() && s.ShapeIsRelevant() {
		name = strings.TrimSuffix(name, " "+s.Shape.String())
	}
	if full, ok := somersaultNames[name]; ok {
		return full
	}
	return name
}

// Categories lists the common skills by category, in picker order, each
// category sorted by tariff and then name. Empty categories are left out.
func Categories() []Category {
	byKey := map[string][]Entry{}
	for key, s := range skills.CommonSkills {
		s.Name = skills.FindCommonSkillName(s)
		s.SetTariff()
		byKey[categoryOf(s)] = append(byKey[categoryOf(s)], Entry{Key: key, Skill: s})
	}

	var cats []Category
	for _, c := range categoryOrder {
		entries := byKey[c.Key]
		if len(entries) == 0 {
			continue
		}
		sort.Slice(entries, func(i, j int) bool {
			a, b := entries[i].Skill, entries[j].Skill
			if a.Tariff != b.Tariff {
				return a.Tariff < b.Tariff
			}
			return a.Name < b.Name
		})
		cats = append(cats, Category{Key: c.Key, Label: c.Label, Entries: entries})
	}
	return cats
}
