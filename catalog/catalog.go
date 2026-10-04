// Package catalog organises the common skills for choosing them: grouped into
// categories for the skill picker, and searchable by name or FIG notation.
package catalog

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	"tariffCalculator/skills"
)

// Entry is a common skill, named and priced in its usual shape. The shaped
// jumps are an entry each (Tuck, Pike and Straddle Jump), as the shape is the
// skill.
type Entry struct {
	Key   string // key in skills.CommonSkills
	Skill skills.TrampolineSkill
	Label string // its name within a Group, e.g. "½ Twist To Feet" under "From seat"
}

// ID tells entries apart in the picker: the key, plus the shape for a jump.
func (e Entry) ID() string {
	if e.Skill.IsBasicJump() {
		return e.Key + "-" + strings.ToLower(e.Skill.Shape.String())
	}
	return e.Key
}

// pickerShapes are the shapes a somersault can be chosen in from the picker.
var pickerShapes = []skills.Shape{skills.Tuck, skills.Pike, skills.Straight}

// Shapes are the shapes the picker offers for the entry: tuck, pike and
// straight where the shape makes a different skill, none otherwise (twisting
// jumps, drops, singles with a full twist or more, and the jumps, whose shape
// is their name).
func (e Entry) Shapes() []skills.Shape {
	if e.Skill.IsBasicJump() || !e.Skill.ShapeIsRelevant() {
		return nil
	}
	return pickerShapes
}

// ShapeOption is a shape the picker offers for an entry, with what it adds to
// the base (tuck) tariff, e.g. +0.1 for a single somersault piked (CoP §17.1.4).
type ShapeOption struct {
	Shape    skills.Shape
	Modifier float64
}

// BaseTariff is the entry's tariff in its base shape: tuck where the shape
// makes a different skill, otherwise its own.
func (e Entry) BaseTariff() float64 {
	if len(e.Shapes()) == 0 {
		return e.Skill.Tariff
	}
	return tariffIn(e.Skill, skills.Tuck)
}

// ShapeOptions are the entry's shapes with what each adds to the base tariff.
func (e Entry) ShapeOptions() []ShapeOption {
	var options []ShapeOption
	for _, shape := range e.Shapes() {
		options = append(options, ShapeOption{shape, math.Round((tariffIn(e.Skill, shape)-e.BaseTariff())*10) / 10})
	}
	return options
}

// ShapeTariffs is the entry's tariff in each shape the picker offers, by shape
// name, for the page to show differences from whichever shape is chosen.
func (e Entry) ShapeTariffs() map[string]float64 {
	tariffs := map[string]float64{}
	for _, shape := range e.Shapes() {
		tariffs[shape.String()] = tariffIn(e.Skill, shape)
	}
	return tariffs
}

func tariffIn(s skills.TrampolineSkill, shape skills.Shape) float64 {
	s.Shape = shape
	return s.SetTariff()
}

// jumpShapes are the shaped jumps, each its own entry.
var jumpShapes = []skills.Shape{skills.Tuck, skills.Pike, skills.Straddle}

// sortName orders entries of the same tariff: the shaped jumps first, in
// jumpShapes order (the tuck jump is usually learned first), then by name.
func sortName(s skills.TrampolineSkill) string {
	if s.IsBasicJump() {
		return fmt.Sprintf("0 %d", slices.Index(jumpShapes, s.Shape))
	}
	return "1 " + s.Name
}

// Category is the entries shown under one picker tab: its groups first, then
// the skills that aren't in one.
type Category struct {
	Key     string
	Label   string
	Groups  []Group
	Entries []Entry
}

// Group is a set of related skills shown together in the picker, e.g. those
// from seat, so a tab of similar drops isn't a wall of boxes.
type Group struct {
	Title   string
	Options []Entry
}

// groups are the picker's groups, in order, with each option's label.
var groups = []struct {
	category, title string
	options         [][2]string // CommonSkills key, label
}{
	{"drops", "To seat", [][2]string{{"seatDrop", "Seat Drop"}, {"halfToSeat", "½ Twist To Seat"}}},
	{"drops", "From seat", [][2]string{{"seatToFeet", "To Feet"}, {"seatHalfToFeet", "½ Twist To Feet"}, {"seatHalfToSeat", "½ Twist To Seat"}, {"seatHalfToFront", "½ Twist To Front"}}},
	{"drops", "To back or front", [][2]string{{"backDrop", "Back Drop"}, {"halfToBack", "½ Twist To Back"}, {"frontDrop", "Front Drop"}, {"halfToFront", "½ Twist To Front"}}},
	{"drops", "From back or front", [][2]string{{"backToFeet", "Back To Feet"}, {"backHalfToFeet", "Back ½ Twist To Feet"}, {"frontToFeet", "Front To Feet"}}},
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

// somersaultNames name base names that don't say they're somersaults once the
// shape is dropped, as rule books write them ("Back Tuck" becomes "Back s/s",
// not "Back").
var somersaultNames = map[string]string{
	"Back":          "Back s/s",
	"Front":         "Front s/s",
	"Back To Seat":  "Back s/s To Seat",
	"Front To Seat": "Front s/s To Seat",
}

// PickerName is a skill's name in the picker: without its shape, which is
// chosen on the skill card, except for shaped jumps, where the shape is the
// skill ("Tuck Jump").
func PickerName(s skills.TrampolineSkill) string {
	name := skills.FindCommonSkillName(s)
	if !s.IsBasicJump() && s.ShapeIsRelevant() {
		name = strings.TrimPrefix(strings.TrimSuffix(name, " "+s.Shape.String()), s.Shape.String()+" ")
	}
	if full, ok := somersaultNames[name]; ok {
		return full
	}
	return name
}

// Categories lists the common skills by category, in picker order, each
// category sorted by tariff and then name. Empty categories are left out.
func Categories() []Category {
	grouped := map[string]bool{}
	byCategory := map[string][]Group{}
	for _, g := range groups {
		group := Group{Title: g.title}
		for _, o := range g.options {
			s := skills.CommonSkills[o[0]]
			s.Name = skills.FindCommonSkillName(s)
			s.SetTariff()
			group.Options = append(group.Options, Entry{Key: o[0], Skill: s, Label: o[1]})
			grouped[o[0]] = true
		}
		byCategory[g.category] = append(byCategory[g.category], group)
	}

	byKey := map[string][]Entry{}
	add := func(key string, s skills.TrampolineSkill) {
		s.Name = skills.FindCommonSkillName(s)
		s.SetTariff()
		byKey[categoryOf(s)] = append(byKey[categoryOf(s)], Entry{Key: key, Skill: s})
	}
	for key, s := range skills.CommonSkills {
		if grouped[key] {
			continue
		}
		if s.IsBasicJump() {
			for _, shape := range jumpShapes {
				s.Shape = shape
				add(key, s)
			}
			continue
		}
		add(key, s)
	}

	var cats []Category
	for _, c := range categoryOrder {
		entries := byKey[c.Key]
		if len(entries) == 0 && len(byCategory[c.Key]) == 0 {
			continue
		}
		sort.Slice(entries, func(i, j int) bool {
			a, b := entries[i].Skill, entries[j].Skill
			if a.Tariff != b.Tariff {
				return a.Tariff < b.Tariff
			}
			return sortName(a) < sortName(b)
		})
		cats = append(cats, Category{Key: c.Key, Label: c.Label, Groups: byCategory[c.Key], Entries: entries})
	}
	return cats
}
