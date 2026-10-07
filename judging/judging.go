// Package judging says how execution judges assess a trampoline skill: the
// deductions that can apply to it and what each one means, from the FIG
// Trampoline Code of Points 2025–2028 (§13, §16, §17.2, §20, §22 and the
// Part II drawings).
//
// Nothing is listed per skill. Each rule below has a condition on the skill's
// attributes (rotation, twist, shape, take-off, landing), so any skill the
// builder can make, named or custom, gets its own list.
package judging

import "tariffCalculator/skills"

// Item is one deduction (or, with no Amount, one thing judges look for).
type Item struct {
	Fault   string // what the judges see, e.g. "Bent knees"
	Amount  string // e.g. "0.1" or "0.1–0.2"; empty for a note
	Explain string // what it means, in plain English
	Ref     string // Code of Points reference, e.g. "§20.2.1.2"
}

// Group is a heading and its items, e.g. "Arms".
type Group struct {
	Title string
	Intro string
	Items []Item
}

// Guide is everything that applies to one skill.
type Guide struct {
	Skill  skills.TrampolineSkill
	Groups []Group
}

// Group titles, in the order they're shown.
const (
	Shape   = "Body shape"
	Legs    = "Legs and feet"
	Arms    = "Arms"
	Opening = "Opening and twist"
	Landing = "Landing"
)

var groupOrder = []string{Shape, Legs, Arms, Opening, Landing}

var intros = map[string]string{
	Opening: "Judges picture the end of a somersault on a clock: 12 o'clock is upside down " +
		"with your feet to the ceiling, 3 o'clock is flat, parallel to the bed. Form isn't " +
		"judged after 3 o'clock, when you're getting ready to land.",
	Landing: "After the last element (the 10th), land on your feet and stand still, upright, " +
		"for about three seconds. You may do one controlled straight jump (an out-bounce) first. " +
		"Unsteadiness and the out-bounce together cost at most 0.3, for the bigger fault.",
}

// facts are the attributes the rules are written against.
type facts struct {
	jump       bool         // feet to feet with no somersault rotation (basic and twisting jumps)
	somersault bool         // ¾ somersault or more
	multiple   bool         // 1¾ somersaults (630°) or more
	shape      skills.Shape // the shape judged, or InvalidShape when none is (drops)
	twist      int          // half twists in all
	lastTwist  int          // half twists in the last somersault
	last       bool         // the final element of the exercise
}

func factsOf(s skills.TrampolineSkill, last bool) facts {
	f := facts{
		jump:       s.Rotation == 0 && s.TakeoffPosition == skills.Feet && !s.SeatLanding,
		somersault: s.Rotation >= 3,
		multiple:   s.Rotation >= 7,
		twist:      s.TotalTwist(),
		shape:      skills.InvalidShape,
		last:       last,
	}
	if n := len(s.TwistDistribution); n > 0 {
		f.lastTwist = s.TwistDistribution[n-1]
	}
	switch {
	case s.ShapeIsRelevant():
		f.shape = s.Shape
	case f.somersault || (f.jump && f.twist > 0):
		// Twisting jumps and singles with a full twist or more are done straight.
		f.shape = skills.Straight
	}
	return f
}

// rule adds item to group when the skill meets when.
type rule struct {
	group string
	when  func(f facts) bool
	item  func(f facts) Item
}

func fixed(it Item) func(facts) Item { return func(facts) Item { return it } }

func always(facts) bool { return true }

// rules is every deduction the guide can show, in the order it shows them.
var rules = []rule{
	// Body shape: §13.2–13.5, §20.2.1.1–2 and the Part II position drawings.
	{Shape, func(f facts) bool { return f.shape == skills.Tuck }, fixed(Item{
		Fault:   "Open tuck",
		Amount:  "0.1–0.2",
		Explain: "Knees and thighs should be pulled in close to the chest. The further the tuck opens, the bigger the deduction.",
		Ref:     "§13.4, Part II A",
	})},
	{Shape, func(f facts) bool { return f.shape == skills.Tuck }, fixed(Item{
		Fault:   "Hands behind the knees",
		Amount:  "0.1",
		Explain: "Hold the shins, below the knees, not the backs of the thighs.",
		Ref:     "§13.5, §20.2.1.1",
	})},
	{Shape, func(f facts) bool { return f.shape == skills.Pike && !f.jump }, fixed(Item{
		Fault:   "Open pike",
		Amount:  "0.1–0.2",
		Explain: "Fold at the hips with the chest close to the legs and hands on the legs below the knees. The more open the pike, the bigger the deduction.",
		Ref:     "§13.4–13.5, Part II A",
	})},
	{Shape, func(f facts) bool { return f.jump && (f.shape == skills.Pike || f.shape == skills.Straddle) }, fixed(Item{
		Fault:   "Legs below horizontal",
		Amount:  "0.1–0.2",
		Explain: "Lift the legs to horizontal. A little below horizontal costs 0.1; well below, 0.2.",
		Ref:     "Part II A, pike and straddle jumps",
	})},
	{Shape, func(f facts) bool { return f.shape == skills.Straight }, fixed(Item{
		Fault:   "Piked or arched body",
		Amount:  "0.1–0.2",
		Explain: "Keep a straight line from shoulders to feet, without bending at the hips or arching the back.",
		Ref:     "§13.2.1, Part II A",
	})},
	{Shape, func(f facts) bool {
		return f.shape == skills.Pike || f.shape == skills.Straight || f.shape == skills.Straddle
	}, fixed(Item{
		Fault:   "Bent knees",
		Amount:  "0.1–0.2",
		Explain: "Legs stay straight in pike and straight shapes, all through the flight.",
		Ref:     "§20.2.1.2",
	})},
	{Shape, func(f facts) bool { return f.multiple }, fixed(Item{
		Fault: "When the shape is judged",
		Explain: "In multiple somersaults the shape isn't judged at take-off until your upper body passes 90° " +
			"(backward) or 135° (forward), so you can leave the bed stretched.",
		Ref: "Part II A, multiple somersault take-off",
	})},

	// Legs and feet: §13.3, §20.2.1.2.
	{Legs, func(f facts) bool { return f.shape != skills.Straddle }, fixed(Item{
		Fault:   "Feet apart",
		Amount:  "0.1",
		Explain: "Keep the feet together all through the skill.",
		Ref:     "§13.3, §20.2.1.2",
	})},
	{Legs, func(f facts) bool { return f.shape != skills.Straddle }, fixed(Item{
		Fault:   "Knees apart",
		Amount:  "0.1",
		Explain: "Keep the knees together, in tuck as well.",
		Ref:     "§13.3, §20.2.1.2",
	})},
	{Legs, always, fixed(Item{
		Fault:   "Toes not pointed",
		Amount:  "0.1",
		Explain: "Point the feet and toes.",
		Ref:     "§13.3, §20.2.1.2",
	})},

	// Arms: §13.6, §20.2.1.1 and the Part II arm drawings.
	{Arms, always, fixed(Item{
		Fault:   "Elbows away from the body",
		Amount:  "0.1",
		Explain: "Keep the arms close to the body.",
		Ref:     "§13.6, §20.2.1.1",
	})},
	{Arms, func(f facts) bool { return f.twist <= 3 }, fixed(Item{
		Fault:   "Bent elbows",
		Amount:  "0.1",
		Explain: "With 1½ twists or less, the arms should be straight.",
		Ref:     "§20.2.1.1",
	})},
	{Arms, func(f facts) bool { return f.twist > 3 }, fixed(Item{
		Fault:   "Bent elbows are allowed",
		Explain: "With 2 twists or more you may bend the elbows, but they must stay close to the body.",
		Ref:     "§20.2.1.1",
	})},
	{Arms, func(f facts) bool { return f.somersault && f.twist > 0 }, func(f facts) Item {
		angle := "90°"
		if halfOut(f) {
			angle = "45°"
		}
		return Item{
			Fault:  "Arms too wide stopping the twist",
			Amount: "0.1",
			Explain: "You may open the arms to stop twisting, up to " + angle + " from the body for this skill. " +
				"That's 45° for a barani, full or multiple somersault with a half out, and 90° for more twist.",
			Ref: "§13.6, §20.2.1.1, Part II A",
		}
	}},

	// Opening and twist: §20.2.1, §20.2.1.3–5 and the Part II opening drawings.
	{Opening, func(f facts) bool { return f.somersault && (f.shape == skills.Tuck || f.shape == skills.Pike) }, func(f facts) Item {
		explain := "Open to a straight body (180° at the hips) by 1 o'clock. Open between 1 and 2 o'clock: 0.1; " +
			"between 2 and 3 o'clock: 0.2; never open: 0.3."
		if f.multiple {
			explain += " In a multiple somersault, open no earlier than 10 o'clock of the last somersault."
		}
		return Item{Fault: "Late or no opening", Amount: "0.1–0.3", Explain: explain, Ref: "§20.2.1.3, Part II A"}
	}},
	{Opening, func(f facts) bool { return f.somersault && f.shape == skills.Straight }, fixed(Item{
		Fault:   "No opening needed",
		Explain: "A straight somersault is already open, so there's no opening deduction.",
		Ref:     "§20.2.1.3",
	})},
	{Opening, func(f facts) bool { return f.somersault }, fixed(Item{
		Fault:  "Piking down",
		Amount: "0.1–0.2",
		Explain: "Once open, keep straight until 3 o'clock. Bending the hips to 170°–136° costs 0.1; " +
			"to 135° or less, 0.2.",
		Ref: "§20.2.1.5",
	})},
	{Opening, func(f facts) bool { return f.somersault && f.lastTwist > 2 }, fixed(Item{
		Fault:   "Twist finishing late",
		Amount:  "0.3",
		Explain: "With more than a full twist in the last somersault, the last quarter of the twist must finish before 3 o'clock.",
		Ref:     "§20.2.1.4",
	})},

	// Landing: §16, §20.2.2.
	{Landing, func(f facts) bool { return f.last }, fixed(Item{
		Fault:   "Steps or bounces",
		Amount:  "0.1 each",
		Explain: "Each step, and each bounce where a foot fully leaves the bed. Stepping back to where you were with the same foot doesn't count.",
		Ref:     "§20.2.2.1.2",
	})},
	{Landing, func(f facts) bool { return f.last }, fixed(Item{
		Fault:   "Arm movements",
		Amount:  "0.1 each",
		Explain: "Uncontrolled arm movements while standing on the bed.",
		Ref:     "§20.2.2.1.2",
	})},
	{Landing, func(f facts) bool { return f.last }, fixed(Item{
		Fault:   "Not standing upright",
		Amount:  "0.1",
		Explain: "Holding a squat for more than two seconds before standing up.",
		Ref:     "§20.2.2.1.2",
	})},
	{Landing, func(f facts) bool { return f.last }, fixed(Item{
		Fault:   "Turning to the judges too soon",
		Amount:  "0.2",
		Explain: "Turning before you've stood still for about three seconds.",
		Ref:     "§16.3, §20.2.2.1.2",
	})},
	{Landing, func(f facts) bool { return f.last }, fixed(Item{
		Fault:   "Uncontrolled out-bounce",
		Amount:  "0.1",
		Explain: "More than one arm circle, bent knees or a bent body in the out-bounce. Celebrating with your arms or one arm circle is fine.",
		Ref:     "§16.2, §20.2.2.1.1",
	})},
	{Landing, func(f facts) bool { return f.last }, fixed(Item{
		Fault:   "Touching the bed with your hands",
		Amount:  "0.5",
		Explain: "One or both hands, after landing.",
		Ref:     "§20.2.2.2",
	})},
	{Landing, func(f facts) bool { return f.last }, fixed(Item{
		Fault:   "Touching anything but the bed",
		Amount:  "0.5",
		Explain: "The frame, pads or anything else, after landing.",
		Ref:     "§20.2.2.3",
	})},
	{Landing, func(f facts) bool { return f.last }, fixed(Item{
		Fault:   "Falling",
		Amount:  "1.0",
		Explain: "Touching or falling to the knees, hands and knees, front, back or seat.",
		Ref:     "§20.2.2.4",
	})},
	{Landing, func(f facts) bool { return f.last }, fixed(Item{
		Fault:   "Off the bed, or a somersault to save it",
		Amount:  "1.0",
		Explain: "Landing or falling off the bed, leaving the trampoline area, or doing another somersault (a whip-back, say) because you can't stop.",
		Ref:     "§20.2.2.5",
	})},
	{Landing, func(f facts) bool { return f.last }, fixed(Item{
		Fault:   "An extra element",
		Amount:  "1.0",
		Explain: "Doing an 11th element. A tuck, pike or straddle jump as the out-bounce isn't counted as one.",
		Ref:     "§16.5, §20.2.2.6",
	})},
}

// halfOut reports whether the arms may open only 45° to stop the twist: a
// barani or full (a single with up to a full twist), or a multiple somersault
// whose only twist is a half out (§13.6).
func halfOut(f facts) bool {
	if !f.multiple {
		return f.twist <= 2
	}
	return f.twist == 1 && f.lastTwist == 1
}

// For is the judging guide for s. last says whether s ends the exercise, which
// adds the landing deductions.
func For(s skills.TrampolineSkill, last bool) Guide {
	f := factsOf(s, last)
	byGroup := map[string][]Item{}
	for _, r := range rules {
		if r.when(f) {
			byGroup[r.group] = append(byGroup[r.group], r.item(f))
		}
	}
	g := Guide{Skill: s}
	for _, title := range groupOrder {
		if items := byGroup[title]; len(items) > 0 {
			g.Groups = append(g.Groups, Group{Title: title, Intro: intros[title], Items: items})
		}
	}
	return g
}

// Scoring is how execution and the other per-element scores are worked out,
// shown under every skill's guide.
var Scoring = []Item{
	{
		Fault: "Up to 0.5 per element",
		Explain: "Each execution judge takes 0.0 to 0.5 off every element, in tenths. The landing is " +
			"judged separately.",
		Ref: "§20.1",
	},
	{
		Fault: "Out of 20",
		Explain: "For each element and the landing, the two middle judges' deductions are added. " +
			"Your execution score is 20 minus all of those.",
		Ref: "§17.2.3.2",
	},
	{
		Fault: "Also scored: where you land",
		Explain: "Horizontal displacement takes 0.0 to 0.3 off 10 for each element, depending on " +
			"which zone of the bed you land in. Stay in the middle.",
		Ref: "§17.2.4, §22",
	},
	{
		Fault:   "Also scored: time of flight",
		Explain: "The time you spend in the air is measured and added to your score.",
		Ref:     "§17.2.5",
	},
}
