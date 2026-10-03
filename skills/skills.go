package skills

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

type TrampolineSkill struct {
	Name              string       `json:"name"`                  // official name, derived by FindCommonSkillName
	CustomName        string       `json:"custom_name,omitempty"` // optional user label, shown alongside Name
	Rotation          int          `json:"rotation"`              // 1/4 of a rotation/90 degrees
	TwistDistribution []int        `json:"twist_distribution"`    // 1/2 of a twist/180 degrees per rotation
	TakeoffPosition   BodyPosition `json:"takeoff_position"`
	Shape             Shape        `json:"shape"`
	Tariff            float64      `json:"tariff,omitempty"`
	Backward          bool         `json:"backward"`
	SeatLanding       bool         `json:"seat_landing"`
}

func (skill *TrampolineSkill) TotalTwist() int {
	totalTwist := 0
	for _, twist := range skill.TwistDistribution {
		totalTwist += twist
	}
	return totalTwist
}
func (skill *TrampolineSkill) LandingPosition() BodyPosition {
	totalRotation := 0
	if skill.Backward {
		totalRotation = skill.TakeoffPosition.Angle() - skill.Rotation
	} else {
		totalRotation = skill.TakeoffPosition.Angle() + skill.Rotation
	}
	var positionByRotation BodyPosition
	if skill.TotalTwist()%2 == 0 {
		positionByRotation = bodyPosition(totalRotation)
	} else {
		positionByRotation = bodyPosition(totalRotation * -1)
	}
	if skill.SeatLanding {
		if positionByRotation == Feet {
			return Seat
		} else {
			return Invalid
		}
	}
	return positionByRotation
}

func (skill *TrampolineSkill) SetTariff() float64 {
	tariff := 0.0
	switch {
	case skill.Rotation == 0:
		tariff = noSomersaultTariff(skill)
	case skill.Rotation < 8:
		tariff = singleSomersaultTariff(skill)
	case skill.Rotation < 12:
		tariff = doubleSomersaultTariff(skill)
	case skill.Rotation < 16:
		tariff = tripleSomersaultTariff(skill)
	default:
		tariff = quadSomersaultTariff(skill)
	}
	skill.Tariff = tariff
	return tariff
}
func noSomersaultTariff(skill *TrampolineSkill) float64 {
	if skill.TotalTwist() != 0 {
		return float64(skill.TotalTwist()) / 10
	}
	if skill.Shape != Straight {
		return 0.1
	}
	if (skill.TakeoffPosition != Seat && skill.SeatLanding) || (skill.TakeoffPosition == Seat && !skill.SeatLanding) {
		return 0.1
	}
	return 0
}
func singleSomersaultTariff(skill *TrampolineSkill) float64 {
	tariff := 0
	if skill.Rotation > 3 {
		tariff++
		if skill.TotalTwist() == 0 {
			switch skill.Shape {
			case Straight, Pike:
				tariff++
			default:
			}
		}
	}
	tariff += skill.Rotation
	tariff += skill.TotalTwist()
	return float64(tariff) / 10
}
func doubleSomersaultTariff(skill *TrampolineSkill) float64 {
	tariff := 2
	if skill.Backward {
		tariff++
	}
	if skill.Shape == Straight || skill.Shape == Pike {
		tariff += 2
	}
	if skill.TotalTwist() > 4 {
		tariff += skill.TotalTwist() - 4
	}
	tariff += skill.Rotation
	tariff += skill.TotalTwist()
	return float64(tariff) / 10
}
func tripleSomersaultTariff(skill *TrampolineSkill) float64 {
	tariff := 4
	if skill.Backward {
		tariff += 2
	}
	if skill.Shape == Straight || skill.Shape == Pike {
		tariff += 3
	}
	if skill.TotalTwist() > 2 {
		tariff += (skill.TotalTwist() - 2) * 2
	}
	tariff += skill.Rotation
	tariff += skill.TotalTwist()
	return float64(tariff) / 10
}
func quadSomersaultTariff(skill *TrampolineSkill) float64 {
	tariff := 6
	if skill.Backward {
		tariff += 3
	}
	if skill.Shape == Straight || skill.Shape == Pike {
		tariff += 4
	}
	tariff += skill.Rotation
	tariff += skill.TotalTwist() * 3
	return float64(tariff) / 10
}

type BodyPosition int

const (
	Feet BodyPosition = iota
	Front
	Back
	Seat
	Invalid
)

var BodyPositionName = map[BodyPosition]string{
	Feet:    "Feet",
	Front:   "Front",
	Back:    "Back",
	Seat:    "Seat",
	Invalid: "Invalid",
}

func bodyPosition(angle int) BodyPosition {

	if val, ok := BodyPositionAngles[angle-(angle/4)*4]; ok {
		return val
	} else {
		return Invalid
	}
}

var BodyPositionAngles = map[int]BodyPosition{
	-3: Front,
	-1: Back,
	0:  Feet,
	1:  Front,
	3:  Back,
}

func (pos BodyPosition) String() string {
	return BodyPositionName[pos]
}
func (pos BodyPosition) MarshalJSON() ([]byte, error) {
	names := [...]string{"Feet", "Front", "Back", "Seat", "Invalid"}
	if pos < Feet || pos > Invalid {
		return json.Marshal("Invalid")
	}
	return json.Marshal(names[pos])

}
func (pos *BodyPosition) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch strings.ToLower(s) {
	case "feet":
		*pos = Feet
	case "front":
		*pos = Front
	case "back":
		*pos = Back
	case "seat":
		*pos = Seat
	default:
		*pos = Invalid
	}
	return nil
}

func (pos BodyPosition) Angle() int {
	switch pos {
	case Feet, Seat:
		return 0
	case Back:
		return 3
	case Front:
		return 1
	default:
		return -1
	}
}

type Shape int

const (
	Straight Shape = iota
	Tuck
	Pike
	Straddle
	InvalidShape
)

var ShapeName = map[Shape]string{
	Straight:     "Straight",
	Tuck:         "Tuck",
	Pike:         "Pike",
	Straddle:     "Straddle",
	InvalidShape: "Invalid Shape",
}

func (shape Shape) String() string {
	return ShapeName[shape]
}

func (shape Shape) MarshalJSON() ([]byte, error) {
	return json.Marshal(shape.String())
}
func (shape *Shape) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	for k, v := range ShapeName {
		if strings.EqualFold(s, v) {
			*shape = k
			return nil
		}
	}
	*shape = InvalidShape
	return nil
}

func BodyPositionFromString(s string) BodyPosition {
	switch strings.ToLower(s) {
	case "feet":
		return Feet
	case "front":
		return Front
	case "back":
		return Back
	case "seat":
		return Seat
	default:
		return Invalid
	}
}

func (skill *TrampolineSkill) Equal(b *TrampolineSkill) bool {
	if skill.TotalTwist() == b.TotalTwist() && skill.Rotation == b.Rotation && skill.Backward == b.Backward && skill.SeatLanding == b.SeatLanding && skill.TakeoffPosition == b.TakeoffPosition {

		if skill.Rotation == 0 && skill.TotalTwist() == 0 && skill.LandingPosition() != Seat && skill.TakeoffPosition != Seat {
			return skill.Shape == b.Shape
		}
		if skill.Rotation < 3 {
			return true
		}
		if skill.Rotation >= 3 && skill.Rotation < 6 {
			if skill.TotalTwist() < 2 {
				return skill.Shape == b.Shape
			}
			return true
		}
		if skill.Shape != b.Shape {
			return false
		}
		if !slices.Equal(skill.TwistDistribution, b.TwistDistribution) {
			return false
		}
		return true
	}
	return false

}

var CommonSkills = map[string]TrampolineSkill{
	"shapeJump":       {Name: "Shape Jump", SeatLanding: false, Shape: Tuck, Backward: false, Rotation: 0, TwistDistribution: []int{0}, TakeoffPosition: Feet},
	"halfTwist":       {Name: "Half Twist", SeatLanding: false, Shape: Straight, Backward: false, Rotation: 0, TwistDistribution: []int{1}, TakeoffPosition: Feet},
	"fullTwist":       {Name: "Full Twist", SeatLanding: false, Shape: Straight, Backward: false, Rotation: 0, TwistDistribution: []int{2}, TakeoffPosition: Feet},
	"seatDrop":        {Name: "Seat Drop", SeatLanding: true, Shape: Straight, Backward: false, Rotation: 0, TwistDistribution: []int{0}, TakeoffPosition: Feet},
	"seatToFeet":      {Name: "Seat To Feet", TakeoffPosition: Seat, Shape: Straight, Backward: false, SeatLanding: false, Rotation: 0, TwistDistribution: []int{0}},
	"frontToSeat":     {Name: "Front To Seat", Rotation: 4, TwistDistribution: []int{0}, TakeoffPosition: Feet, Backward: false, Shape: Tuck, SeatLanding: true},
	"backToSeat":      {Name: "Back To Seat", Rotation: 4, TwistDistribution: []int{0}, TakeoffPosition: Feet, Backward: true, Shape: Tuck, SeatLanding: true},
	"baraniToFront":   {Name: "Barani To Front", Rotation: 3, TwistDistribution: []int{1}, TakeoffPosition: Feet, Backward: false, Shape: Tuck, SeatLanding: false},
	"backDrop":        {Name: "Back Drop", Rotation: 1, TwistDistribution: []int{0}, TakeoffPosition: Feet, Backward: true, Shape: Straight, SeatLanding: false},
	"frontDrop":       {Name: "Front Drop", Rotation: 1, TwistDistribution: []int{0}, TakeoffPosition: Feet, Backward: false, Shape: Straight, SeatLanding: false},
	"backHalfToFeet":  {Name: "Back Half Twist To Feet", Rotation: 1, TwistDistribution: []int{1}, TakeoffPosition: Back, Backward: false, Shape: Straight, SeatLanding: false},
	"backToFeet":      {Name: "Back To Feet", Rotation: 1, TwistDistribution: []int{0}, TakeoffPosition: Back, Backward: false, Shape: Straight, SeatLanding: false},
	"frontToFeet":     {Name: "Front To Feet", Rotation: 1, TwistDistribution: []int{0}, TakeoffPosition: Front, Backward: true, Shape: Straight, SeatLanding: false},
	"front":           {Name: "Front", Rotation: 4, TwistDistribution: []int{0}, TakeoffPosition: Feet, Backward: false, Shape: Tuck, SeatLanding: false},
	"ballOut":         {Name: "Ball-Out", Rotation: 5, TwistDistribution: []int{0}, TakeoffPosition: Back, Backward: false, Shape: Tuck, SeatLanding: false},
	"baraniBallOut":   {Name: "Barani Ball-Out", Rotation: 5, TwistDistribution: []int{1}, TakeoffPosition: Back, Backward: false, Shape: Tuck, SeatLanding: false},
	"rudiBallOut":     {Name: "Rudi Ball-Out", Rotation: 5, TwistDistribution: []int{3}, TakeoffPosition: Back, Backward: false, Shape: Straight, SeatLanding: false},
	"crashDive":       {Name: "Crash Dive", Rotation: 3, TwistDistribution: []int{0}, TakeoffPosition: Feet, Shape: Straight, Backward: false, SeatLanding: false},
	"lazyBack":        {Name: "Lazy Back", Rotation: 3, TwistDistribution: []int{0}, TakeoffPosition: Feet, Backward: true, Shape: Straight, SeatLanding: false},
	"seatHalfToFeet":  {Name: "Seat Half Twist To Feet", Rotation: 0, TakeoffPosition: Seat, Shape: Straight, Backward: false, SeatLanding: false, TwistDistribution: []int{1}},
	"seatHalfToSeat":  {Name: "Seat Half Twist To Seat", Rotation: 0, TakeoffPosition: Seat, Shape: Straight, Backward: false, SeatLanding: true, TwistDistribution: []int{1}},
	"seatHalfToFront": {Name: "Seat Half Twist To Front", TakeoffPosition: Seat, Shape: Straight, SeatLanding: false, TwistDistribution: []int{1}, Backward: true, Rotation: 1},
	"barani":          {Name: "Barani", Rotation: 4, TwistDistribution: []int{1}, TakeoffPosition: Feet, Backward: false, Shape: Tuck, SeatLanding: false},
	"rudi":            {Name: "Rudi", Rotation: 4, TwistDistribution: []int{3}, TakeoffPosition: Feet, Backward: false, Shape: Straight, SeatLanding: false},
	"randi":           {Name: "Randi", Rotation: 4, TwistDistribution: []int{5}, TakeoffPosition: Feet, Backward: false, Shape: Straight, SeatLanding: false},
	"fullBack":        {Name: "Full Back", Rotation: 4, TwistDistribution: []int{2}, TakeoffPosition: Feet, Backward: true, Shape: Straight, SeatLanding: false},
	"doubleFullBack":  {Name: "Double Full Back", Rotation: 4, TwistDistribution: []int{4}, TakeoffPosition: Feet, Backward: true, Shape: Straight, SeatLanding: false},
	"backSomersault":  {Name: "Back", Rotation: 4, TwistDistribution: []int{0}, TakeoffPosition: Feet, Backward: true, Shape: Tuck, SeatLanding: false},
	"fullCody":        {Name: "Full Cody", Rotation: 5, TwistDistribution: []int{2}, TakeoffPosition: Front, Backward: true, Shape: Straight, SeatLanding: false},
	"cody":            {Name: "Cody", Rotation: 5, TwistDistribution: []int{0}, TakeoffPosition: Front, Backward: true, Shape: Tuck, SeatLanding: false},
	"doubleBack":      {Name: "Double Back", Rotation: 8, TwistDistribution: []int{0, 0}, TakeoffPosition: Feet, Backward: true, Shape: Tuck, SeatLanding: false},
	"tripleBack":      {Name: "Triple Back", Rotation: 12, TwistDistribution: []int{0, 0, 0}, TakeoffPosition: Feet, Backward: true, Shape: Tuck, SeatLanding: false},
	"halfOut":         {Name: "Half-Out", Rotation: 8, TwistDistribution: []int{0, 1}, TakeoffPosition: Feet, Backward: false, Shape: Tuck, SeatLanding: false},
	"halfhalf":        {Name: "Half Half", Rotation: 8, TwistDistribution: []int{1, 1}, TakeoffPosition: Feet, Backward: true, Shape: Tuck, SeatLanding: false},
	"trifHalfOut":     {Name: "Trif Half-Out", Rotation: 12, TwistDistribution: []int{0, 0, 1}, TakeoffPosition: Feet, Backward: false, Shape: Tuck, SeatLanding: false},
	"fullFull":        {Name: "Full Full", Rotation: 8, TwistDistribution: []int{2, 2}, TakeoffPosition: Feet, Backward: true, Shape: Straight, SeatLanding: false},
	"fullRudi":        {Name: "Full Rudi", Rotation: 8, TwistDistribution: []int{2, 3}, TakeoffPosition: Feet, Backward: false, Shape: Straight, SeatLanding: false},
	"miller":          {Name: "Miller", Rotation: 8, TwistDistribution: []int{3, 3}, TakeoffPosition: Feet, Backward: true, Shape: Straight, SeatLanding: false},
}

func GetCommonSkill(name string) (TrampolineSkill, bool) {
	skill, exists := CommonSkills[name]
	return skill, exists
}
func CalculatePhases(rotation int) int {
	absRotation := rotation
	if absRotation < 0 {
		absRotation = -absRotation
	}
	switch {
	case absRotation <= 6:
		return 1
	case absRotation <= 10:
		return 2
	case absRotation <= 14:
		return 3
	default: // 15-16
		return 4
	}
}

// NormalizePhases trims or zero-pads TwistDistribution to exactly one entry per
// twist phase for the skill's rotation. It always allocates a new slice, so it is
// safe on copies that share a backing array (e.g. values from CommonSkills).
func (skill *TrampolineSkill) NormalizePhases() {
	twists := make([]int, CalculatePhases(skill.Rotation))
	copy(twists, skill.TwistDistribution)
	skill.TwistDistribution = twists
}

// ShapeFromString parses a shape name case-insensitively, returning InvalidShape
// for anything unrecognised so that Validate rejects it.
func ShapeFromString(s string) Shape {
	for shapeEnum, name := range ShapeName {
		if strings.EqualFold(s, name) {
			return shapeEnum
		}
	}
	return InvalidShape
}

// MaxRotation is a quadruple somersault (1440°), the most the Code of Points
// scores (§17.1.1.5).
const MaxRotation = 16

// MaxCustomNameLength is the longest custom name allowed, in characters.
const MaxCustomNameLength = 60

// Validate reports whether the skill is one the engine can score: rotation within
// 0..MaxRotation quarters, one non-negative twist count per phase, a known
// take-off position and shape, and a custom name of at most MaxCustomNameLength
// characters.
func (skill *TrampolineSkill) Validate() error {
	if n := utf8.RuneCountInString(skill.CustomName); n > MaxCustomNameLength {
		return fmt.Errorf("custom name must be at most %d characters, got %d", MaxCustomNameLength, n)
	}
	if skill.Rotation < 0 || skill.Rotation > MaxRotation {
		return fmt.Errorf("rotation must be between 0 and %d quarter somersaults, got %d",
			MaxRotation, skill.Rotation)
	}
	requiredPhases := CalculatePhases(skill.Rotation)
	if len(skill.TwistDistribution) != requiredPhases {
		return fmt.Errorf("requires %d twist phases for %d/4 rotation",
			requiredPhases, skill.Rotation)
	}
	for i, twist := range skill.TwistDistribution {
		if twist < 0 {
			return fmt.Errorf("twist in phase %d must not be negative, got %d", i+1, twist)
		}
	}
	if skill.TakeoffPosition < Feet || skill.TakeoffPosition >= Invalid {
		return fmt.Errorf("invalid take-off position")
	}
	if skill.Shape < Straight || skill.Shape >= InvalidShape {
		return fmt.Errorf("invalid shape")
	}
	return nil
}

// IsStraightJump reports whether the skill is a plain straight jump: no rotation,
// no twist, straight shape, feet to feet. Inside a routine this is an intermediate
// jump that interrupts the exercise (CoP §15.1.3).
func (skill *TrampolineSkill) IsStraightJump() bool {
	return skill.Rotation == 0 && skill.TotalTwist() == 0 && skill.Shape == Straight &&
		skill.TakeoffPosition == Feet && !skill.SeatLanding
}

// ShapeIsRelevant reports whether the body shape (tuck/pike/straight, plus
// straddle for jumps) is part of the skill's identity. It is the single source
// of truth for "does shape matter here" used by naming and FIG notation.
//
// Shape is relevant for:
//   - basic jumps (no somersault, no twist): tuck/pike/straddle/straight are distinct skills;
//   - single somersaults performed with less than a full twist (the straight/pike
//     bonus of CoP §17.1.4 applies and tuck/pike/straight read as different skills);
//   - 1½ somersaults and above (CoP §17.1.5 / §14.5.3).
//
// Shape is NOT relevant for twisting jumps, sub-3/4 rotations (drops), and single
// somersaults carrying a full twist or more (e.g. Rudi, Full Back) — these are
// effectively straight and the shape is not distinguished.
func (skill *TrampolineSkill) ShapeIsRelevant() bool {
	totalTwist := skill.TotalTwist()
	switch {
	case skill.Rotation == 0 && totalTwist == 0:
		// Basic jumps, but not seat drops / seat take-offs.
		return skill.LandingPosition() != Seat && skill.TakeoffPosition != Seat
	case skill.Rotation == 0:
		// Twisting jumps (half twist, full twist, ...): straight only.
		return false
	case skill.Rotation < 3:
		// Drops and other sub-3/4 rotations: no shape distinction.
		return false
	case skill.Rotation < 6:
		// Single somersaults below 1½: shape matters under a full twist.
		return totalTwist < 2
	default:
		// 1½ somersaults and above.
		return true
	}
}

func (skill *TrampolineSkill) FIGNotation() string {
	// Shape mapping
	var shapeSymbol string
	switch skill.Shape {
	case Tuck:
		shapeSymbol = "o"
	case Pike:
		shapeSymbol = "<"
	case Straight:
		shapeSymbol = "/"
	case Straddle:
		shapeSymbol = "v"
	default:
		shapeSymbol = "?" // Handle unexpected shapes
	}

	// Special case for zero rotation and zero twist (basic jumps)
	if skill.Rotation == 0 && skill.TotalTwist() == 0 {
		// Only return shape for non-straight basic jumps
		if (skill.Shape == Tuck || skill.Shape == Pike || skill.Shape == Straddle) && skill.LandingPosition() != Seat && skill.TakeoffPosition != Seat {
			return fmt.Sprintf("(%s)", shapeSymbol)
		}
		return "" // Return empty for straight jump (no rotation, no twist, straight shape)
	}

	// Twist distribution mapping
	var twistParts []string
	expectedPhases := CalculatePhases(skill.Rotation)
	displayTwists := make([]int, expectedPhases)

	// Use TotalTwist for single-phase (rotation 0) skills with twist
	if expectedPhases == 1 && skill.Rotation == 0 && skill.TotalTwist() > 0 {
		displayTwists[0] = skill.TotalTwist()
	} else if skill.TwistDistribution != nil {
		// Copy provided twists, respecting expected phases
		copyCount := len(skill.TwistDistribution)
		if copyCount > expectedPhases {
			copyCount = expectedPhases
		}
		copy(displayTwists, skill.TwistDistribution[:copyCount])
	}
	// Note: displayTwists is already initialized with zeros if skill.TwistDistribution was nil or shorter

	// Generate twist parts string
	if len(displayTwists) == 0 {
		// Fallback if somehow displayTwists is empty
		twistParts = append(twistParts, "-")
	} else {
		for _, twistHalfTurns := range displayTwists {
			if twistHalfTurns == 0 {
				twistParts = append(twistParts, "-")
			} else {
				// Convert half-turns to string
				twistParts = append(twistParts, strconv.Itoa(twistHalfTurns))
			}
		}
	}
	twistString := strings.Join(twistParts, " ")

	// Determine if shape should be included based on FIG rules (single source of truth).
	includeShape := skill.ShapeIsRelevant()

	// Combine into final notation string
	rotationStr := strconv.Itoa(skill.Rotation)
	if includeShape {
		return fmt.Sprintf("(%s %s %s)", rotationStr, twistString, shapeSymbol)
	} else {
		return fmt.Sprintf("(%s %s)", rotationStr, twistString)
	}
}

// FindCommonSkillName returns the display name for a skill: a known common-skill
// name with the shape appended when shape is relevant (e.g. "Front Tuck"), the
// basic-jump name (e.g. "Tuck Jump"), or "Custom Skill" when nothing matches.
func FindCommonSkillName(parsedSkill TrampolineSkill) string {
	compareSkill := parsedSkill
	compareSkill.NormalizePhases()

	for _, commonSkill := range CommonSkills {
		tempCommon := commonSkill
		tempCommon.NormalizePhases()

		// Match on core parameters; shape is handled separately below.
		if compareSkill.Rotation == tempCommon.Rotation &&
			compareSkill.TakeoffPosition == tempCommon.TakeoffPosition &&
			compareSkill.Backward == tempCommon.Backward &&
			compareSkill.SeatLanding == tempCommon.SeatLanding &&
			slices.Equal(compareSkill.TwistDistribution, tempCommon.TwistDistribution) {

			baseName := tempCommon.Name
			inputShape := compareSkill.Shape

			// Basic jumps: the shape *is* the skill (Tuck/Pike/Straddle Jump, or Straight Jump).
			if compareSkill.Rotation == 0 && compareSkill.TotalTwist() == 0 &&
				compareSkill.LandingPosition() != Seat && compareSkill.TakeoffPosition != Seat {
				if baseName == "Shape Jump" && (inputShape == Tuck || inputShape == Pike || inputShape == Straddle) {
					return fmt.Sprintf("%s Jump", inputShape.String())
				}
				return "Straight Jump"
			}

			// Otherwise append the shape when it is relevant, omit it when it is not.
			if compareSkill.ShapeIsRelevant() {
				return fmt.Sprintf("%s %s", baseName, inputShape.String())
			}
			return baseName
		}
	}

	return "Custom Skill"
}

// SkillValidation is the per-skill outcome of validating a routine.
type SkillValidation struct {
	Skill             TrampolineSkill
	Landing           BodyPosition
	FIGNotation       string
	InvalidTransition bool
	InvalidLanding    bool
	IsDuplicate       bool
	IntermediateJump  bool
}

// RoutineValidation is the pure-domain result of validating a routine: per-skill
// outcomes, per-skill messages (parallel to Skills), the counted vs raw tariff
// totals, and the routine-level flags. It carries no view/transport concerns.
type RoutineValidation struct {
	Skills                []SkillValidation
	Messages              []string
	TotalTariff           float64
	RawTariff             float64
	HasDuplicates         bool
	HasInvalidTransitions bool
	HasInvalidLandings    bool
	HasIntermediateJumps  bool
	TenthSkillWarning     bool
	RoutineTooLong        bool
}

// RoutineLength is the number of elements in an exercise (CoP §4.1).
const RoutineLength = 10

// ValidateRoutine evaluates a routine: duplicate detection (a repeat's difficulty
// is not counted, §14.1), take-off from feet for the first skill (§12.3) and
// landing/take-off transition legality thereafter, invalid landings, intermediate
// straight jumps (§15.1.3), the "10th skill must land on feet" rule (§16.1), and
// only the first RoutineLength skills counting toward the total (§4.1, §16.5).
// Tariffs are (re)computed defensively, so the routine need not be pre-priced.
func ValidateRoutine(routine []TrampolineSkill) RoutineValidation {
	res := RoutineValidation{
		Skills:         make([]SkillValidation, len(routine)),
		Messages:       make([]string, len(routine)),
		RoutineTooLong: len(routine) > RoutineLength,
	}

	duplicateMap := make(map[int]bool)

	for i := range routine {
		s := routine[i]
		s.SetTariff() // idempotent; ensures the derived tariff is populated
		res.Skills[i].Skill = s
		landing := s.LandingPosition()
		res.Skills[i].Landing = landing
		res.Skills[i].FIGNotation = s.FIGNotation()

		res.RawTariff += s.Tariff

		var msgs []string
		isCurrentSkillDuplicate := false
		for j := 0; j < i; j++ {
			if res.Skills[i].Skill.Equal(&res.Skills[j].Skill) {
				isCurrentSkillDuplicate = true
				res.HasDuplicates = true
				if _, marked := duplicateMap[j]; !marked {
					res.Skills[j].IsDuplicate = true
					duplicateMap[j] = true
					if res.Messages[j] == "" {
						res.Messages[j] = "Duplicate (Counts Once)"
					} else {
						res.Messages[j] += " / Duplicate (Counts Once)"
					}
				}
				res.Skills[i].IsDuplicate = true
				msgs = append(msgs, "Duplicate")
				break
			}
		}

		if !isCurrentSkillDuplicate && i < RoutineLength {
			res.TotalTariff += s.Tariff
		}

		if i == 0 {
			// The exercise starts from preparation straight jumps, i.e. on feet.
			if s.TakeoffPosition != Feet {
				res.Skills[i].InvalidTransition = true
				res.HasInvalidTransitions = true
				msgs = append(msgs, "Must Start From Feet")
			}
		} else {
			prevLanding := res.Skills[i-1].Landing
			currentTakeoff := s.TakeoffPosition
			if prevLanding != Invalid && prevLanding != currentTakeoff {
				res.Skills[i].InvalidTransition = true
				res.HasInvalidTransitions = true
				if i < RoutineLength || !res.RoutineTooLong {
					msgs = append(msgs, fmt.Sprintf("Bad Transition: %s -> %s", prevLanding.String(), currentTakeoff.String()))
				}
			}
		}

		if s.IsStraightJump() {
			res.Skills[i].IntermediateJump = true
			res.HasIntermediateJumps = true
			msgs = append(msgs, "Straight Jump Interrupts Routine")
		}

		if landing == Invalid {
			res.Skills[i].InvalidLanding = true
			res.HasInvalidLandings = true
			if i < RoutineLength || !res.RoutineTooLong {
				msgs = append(msgs, "Invalid Landing")
			}
		}

		if i == RoutineLength-1 {
			if landing != Feet {
				res.TenthSkillWarning = true
				msgs = append(msgs, "10th Must Land Feet")
			}
		}

		if i >= RoutineLength {
			msgs = append(msgs, "Skill >10 (No Tariff)")
		}

		res.Messages[i] = strings.Join(msgs, " / ")
	}

	return res
}
