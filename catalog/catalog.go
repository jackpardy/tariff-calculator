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
	Label string // its name within a Group, e.g. "½ Twist To Back" under "To back"
	// Tile, for a body landing, is where it lands and its twist: it takes off
	// from wherever the skill before it landed (Follow), and Key is its usual
	// skill, for when nothing lands like that from there.
	Tile *Tile
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

// Tile is a body-landing tile in the picker: where the skill lands and how
// much it twists, but not where it takes off from, which follows the skill
// before it. "½ Twist To Seat" is a swivel hips after a seat drop, and a half
// twist to seat from the feet.
type Tile struct {
	To    skills.BodyPosition
	Twist int // half twists
}

// landingGroups are the Body landings tab: a group for each place to land,
// a tile for each twist, with its usual skill (a CommonSkills key) and label.
var landingGroups = []struct {
	title string
	tiles []landingTile
}{
	{"To feet", []landingTile{{"seatToFeet", "To Feet", skills.Feet, 0}, {"seatHalfToFeet", "½ Twist To Feet", skills.Feet, 1}}},
	{"To seat", []landingTile{{"seatDrop", "To Seat", skills.Seat, 0}, {"halfToSeat", "½ Twist To Seat", skills.Seat, 1}}},
	{"To back", []landingTile{{"backDrop", "To Back", skills.Back, 0}, {"halfToBack", "½ Twist To Back", skills.Back, 1}, {"catTwist", "Full Twist To Back", skills.Back, 2}, {"corkscrew", "1½ Twist To Back", skills.Back, 3}}},
	{"To front", []landingTile{{"frontDrop", "To Front", skills.Front, 0}, {"halfToFront", "½ Twist To Front", skills.Front, 1}}},
}

type landingTile struct {
	key, label string
	to         skills.BodyPosition
	twist      int
}

// Takeoffs are the places a skill can take off from, as the builder lists them.
var Takeoffs = []skills.BodyPosition{skills.Feet, skills.Front, skills.Back, skills.Seat}

// LandingTile is the Body landings tile whose usual skill is key, if any.
func LandingTile(key string) (Entry, bool) {
	for _, g := range landingGroups {
		for _, t := range g.tiles {
			if t.key == key {
				return t.entry(), true
			}
		}
	}
	return Entry{}, false
}

func (t landingTile) entry() Entry {
	s := skills.CommonSkills[t.key]
	s.Name = skills.FindCommonSkillName(s)
	s.SetTariff()
	return Entry{Key: t.key, Skill: s, Label: t.label, Tile: &Tile{To: t.to, Twist: t.twist}}
}

// Follow is a body-landing tile's skill taking off from where the skill
// before it landed (from), named and priced: the one with that take-off,
// landing and twist that turns least, as the skills package lands it.
// Between two that turn as little (a half somersault either way), it's the
// one in the catalogue, or else forwards. It must have a name of its own,
// from the catalogue or from its parts, that isn't another skill's: "Back
// To Seat" from a back drop would read as the somersault. Without one, or
// from the feet back to the feet (a jump), it's the tile's usual skill and
// ok is false.
func (e Entry) Follow(from skills.BodyPosition) (s skills.TrampolineSkill, ok bool) {
	if e.Tile == nil || (from == skills.Feet && e.Tile.To == skills.Feet) {
		return e.Skill, false
	}
	// Seat to seat, back to back or front to front with no twist isn't a
	// skill: it's a bounce, like a straight jump (the owner, 2026-10-19).
	if from == e.Tile.To && e.Tile.Twist == 0 {
		return e.Skill, false
	}
	// Up to a half somersault: any more is a somersault, not a body landing.
	for rotation := 0; rotation <= 2; rotation++ {
		var found []skills.TrampolineSkill
		for _, backward := range []bool{false, true} {
			if rotation == 0 && backward {
				continue // no somersault, no direction
			}
			c := skills.TrampolineSkill{
				Rotation:          rotation,
				TwistDistribution: []int{e.Tile.Twist},
				TakeoffPosition:   from,
				Backward:          backward,
				Shape:             skills.Straight,
				SeatLanding:       e.Tile.To == skills.Seat,
			}
			if c.LandingPosition() != e.Tile.To {
				continue
			}
			c.Name = skills.FindCommonSkillName(c)
			if c.Name == skills.UnnamedSkill || (!InCatalogue(c) && commonNames[c.Name]) {
				continue
			}
			c.SetTariff()
			found = append(found, c)
		}
		if len(found) > 0 {
			if i := slices.IndexFunc(found, InCatalogue); i >= 0 {
				return found[i], true
			}
			return found[0], true
		}
	}
	return e.Skill, false
}

// Versions are the skills a body-landing tile gives after each take-off,
// each once, its usual one first.
func (e Entry) Versions() []skills.TrampolineSkill {
	versions := []skills.TrampolineSkill{e.Skill}
	for _, from := range Takeoffs {
		if s, ok := e.Follow(from); ok && !slices.ContainsFunc(versions, func(v skills.TrampolineSkill) bool { return v.Equal(&s) }) {
			versions = append(versions, s)
		}
	}
	return versions
}

// InCatalogue reports whether the skill is one of the common skills, rather
// than named from its parts.
func InCatalogue(s skills.TrampolineSkill) bool {
	for _, c := range skills.CommonSkills {
		if c.Equal(&s) {
			return true
		}
	}
	return false
}

// commonNames are the common skills' names.
var commonNames = func() map[string]bool {
	names := map[string]bool{}
	for _, c := range skills.CommonSkills {
		names[c.Name] = true
	}
	return names
}()

// categoryOrder is the picker's tab order, from the simplest skills up.
var categoryOrder = []struct{ Key, Label string }{
	{"jumps", "Jumps"},
	{"drops", "Body landings"},
	{"singles", "Singles"},
	{"doubles", "Doubles"},
	{"triples", "Triples"},
}

// categoryOf places a skill by what it is: its rotation and whether it
// involves front, back or seat.
func categoryOf(s skills.TrampolineSkill) string {
	switch {
	case s.Rotation >= 12:
		return "triples"
	case s.Rotation >= 8:
		return "doubles"
	case s.Rotation >= 3:
		return "singles"
	case s.Rotation == 0 && s.TakeoffPosition == skills.Feet && !s.SeatLanding:
		return "jumps"
	default:
		return "drops"
	}
}

// otherNames are what rule books and coaches call skills where it isn't the
// app's name: the somersaults whose names don't say so ("Back s/s" for a
// Back), and body landings with names of their own ("Cradle"). Said beside
// the app's own name in a search (AlsoCalled), so either finds it and the
// card's name is the one it goes by everywhere.
var otherNames = map[string]string{
	"Back":                    "Back s/s",
	"Front":                   "Front s/s",
	"Back To Seat":            "Back s/s To Seat",
	"Front To Seat":           "Front s/s To Seat",
	"Seat Half Twist To Seat": "Swivel Hips",
	"Back Half Twist To Back": "Cradle",
	"Back 1½ Twist To Back":   "Corkscrew",
	"Back Full Twist To Back": "Cat Twist",
}

// PickerName is a skill's name in the picker: the card's name without its
// shape, which is chosen beside it, except for shaped jumps, where the shape
// is the skill ("Tuck Jump").
func PickerName(s skills.TrampolineSkill) string {
	name := skills.FindCommonSkillName(s)
	if !s.IsBasicJump() && s.ShapeIsRelevant() {
		name = strings.TrimPrefix(strings.TrimSuffix(name, " "+s.Shape.String()), s.Shape.String()+" ")
	}
	return name
}

// AlsoCalled is what rule books or coaches call a skill where it isn't the
// app's name, with its shape ("Back s/s Tuck" for a Tuck Back); "" where it's
// the same.
func AlsoCalled(s skills.TrampolineSkill) string {
	full, ok := otherNames[PickerName(s)]
	if !ok {
		return ""
	}
	if s.ShapeIsRelevant() {
		return full + " " + s.Shape.String()
	}
	return full
}

// Categories lists the common skills by category, in picker order, each
// category sorted by tariff and then name. Empty categories are left out.
func Categories() []Category {
	// A common skill a tile gives, after one take-off or another, isn't a
	// box of its own as well.
	grouped := map[string]bool{}
	byCategory := map[string][]Group{}
	for _, g := range landingGroups {
		group := Group{Title: g.title}
		for _, t := range g.tiles {
			e := t.entry()
			group.Options = append(group.Options, e)
			for _, v := range e.Versions() {
				for key, c := range skills.CommonSkills {
					if c.Equal(&v) {
						grouped[key] = true
					}
				}
			}
		}
		byCategory["drops"] = append(byCategory["drops"], group)
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
