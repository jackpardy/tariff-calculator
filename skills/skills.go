package skills

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

type TrampolineSkill struct {
	Name              string       `json:"name"`
	Rotation          int          `json:"rotation"`           // 1/4 of a rotation/90 degrees
	TwistDistribution []int        `json:"twist_distribution"` // 1/2 of a twist/180 degrees per rotation
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
		if skill.Rotation == 0 && skill.TotalTwist() == 0 {
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
	"tuck":            {Name: "Tuck Jump", SeatLanding: false, Shape: Tuck, Backward: false, Rotation: 0, TwistDistribution: []int{0}, TakeoffPosition: Feet},
	"pike":            {Name: "Pike Jump", SeatLanding: false, Shape: Pike, Backward: false, Rotation: 0, TwistDistribution: []int{0}, TakeoffPosition: Feet},
	"straddle":        {Name: "Straddle Jump", SeatLanding: false, Shape: Straddle, Backward: false, Rotation: 0, TwistDistribution: []int{0}, TakeoffPosition: Feet},
	"halfTwist":       {Name: "Half Twist", SeatLanding: false, Shape: Straight, Backward: false, Rotation: 0, TwistDistribution: []int{1}, TakeoffPosition: Feet},
	"fullTwist":       {Name: "Full Twist", SeatLanding: false, Shape: Straight, Backward: false, Rotation: 0, TwistDistribution: []int{2}, TakeoffPosition: Feet},
	"seatDrop":        {Name: "Seat Drop", SeatLanding: true, Shape: Straight, Backward: false, Rotation: 0, TwistDistribution: []int{0}, TakeoffPosition: Feet},
	"seatToFeet":      {Name: "Seat To Feet", TakeoffPosition: Seat, Shape: Straight, Backward: false, SeatLanding: false, Rotation: 0, TwistDistribution: []int{0}},
	"tuckFrontToSeat": {Name: "Tuck Front To Seat", Rotation: 4, TwistDistribution: []int{0}, TakeoffPosition: Feet, Backward: false, Shape: Tuck, SeatLanding: true},
	"tuckBackToSeat":  {Name: "Tuck Back To Seat", Rotation: 4, TwistDistribution: []int{0}, TakeoffPosition: Feet, Backward: true, Shape: Tuck, SeatLanding: true},
	"baraniToFront":   {Name: "Barani To Front", Rotation: 3, TwistDistribution: []int{1}, TakeoffPosition: Feet, Backward: false, Shape: Tuck, SeatLanding: false},
	"backDrop":        {Name: "Back Drop", Rotation: 1, TwistDistribution: []int{0}, TakeoffPosition: Feet, Backward: true, Shape: Straight, SeatLanding: false},
	"frontDrop":       {Name: "Front Drop", Rotation: 1, TwistDistribution: []int{0}, TakeoffPosition: Feet, Backward: false, Shape: Straight, SeatLanding: false},
	"backHalfToFeet":  {Name: "Back Half Twist To Feet", Rotation: 1, TwistDistribution: []int{1}, TakeoffPosition: Back, Backward: false, Shape: Straight, SeatLanding: false},
	"backToFeet":      {Name: "Back To Feet", Rotation: 1, TwistDistribution: []int{0}, TakeoffPosition: Back, Backward: false, Shape: Straight, SeatLanding: false},
	"frontToFeet":     {Name: "Front To Feet", Rotation: 1, TwistDistribution: []int{0}, TakeoffPosition: Front, Backward: true, Shape: Straight, SeatLanding: false},
	"tuckFront":       {Name: "Tuck Front", Rotation: 4, TwistDistribution: []int{0}, TakeoffPosition: Feet, Backward: false, Shape: Tuck, SeatLanding: false},
	"pikeFront":       {Name: "Pike Front", Rotation: 4, TwistDistribution: []int{0}, TakeoffPosition: Feet, Backward: false, Shape: Pike, SeatLanding: false},
	"straightFront":   {Name: "Straight Front", Rotation: 4, TwistDistribution: []int{0}, TakeoffPosition: Feet, Backward: false, Shape: Straight, SeatLanding: false},
	"ballOut":         {Name: "Ball-Out", Rotation: 5, TwistDistribution: []int{0}, TakeoffPosition: Back, Backward: false, Shape: Tuck, SeatLanding: false},
	"baraniBallOut":   {Name: "Barani Ball-Out", Rotation: 5, TwistDistribution: []int{1}, TakeoffPosition: Back, Backward: false, Shape: Tuck, SeatLanding: false},
	"rudiBallOut":     {Name: "Rudi Ball-Out", Rotation: 5, TwistDistribution: []int{3}, TakeoffPosition: Back, Backward: false, Shape: Straight, SeatLanding: false},
	"crashDive":       {Name: "Crash Dive", Rotation: 3, TwistDistribution: []int{0}, TakeoffPosition: Feet, Shape: Straight, Backward: false, SeatLanding: false},
	"lazyBack":        {Name: "Lazy Back", Rotation: 3, TwistDistribution: []int{0}, TakeoffPosition: Feet, Backward: true, Shape: Straight, SeatLanding: false},
	"seatHalfToFeet":  {Name: "Seat Half Twist To Feet", Rotation: 0, TakeoffPosition: Seat, Shape: Straight, Backward: false, SeatLanding: false, TwistDistribution: []int{1}},
	"seatHalfToSeat":  {Name: "Seat Half Twist To Seat", Rotation: 0, TakeoffPosition: Seat, Shape: Straight, Backward: false, SeatLanding: true, TwistDistribution: []int{1}},
	"seatHalfToFront": {Name: "Seat Half Twist To Front", TakeoffPosition: Seat, Shape: Straight, SeatLanding: false, TwistDistribution: []int{1}, Backward: true, Rotation: 1},
	"tuckBarani":      {Name: "Tuck Barani", Rotation: 4, TwistDistribution: []int{1}, TakeoffPosition: Feet, Backward: false, Shape: Tuck, SeatLanding: false},
	"pikeBarani":      {Name: "Pike Barani", Rotation: 4, TwistDistribution: []int{1}, TakeoffPosition: Feet, Backward: false, Shape: Pike, SeatLanding: false},
	"straightBack":    {Name: "Straight Back", Rotation: 4, TwistDistribution: []int{0}, TakeoffPosition: Feet, Backward: true, Shape: Straight, SeatLanding: false},
	"straightBarani":  {Name: "Straight Barani", Rotation: 4, TwistDistribution: []int{1}, TakeoffPosition: Feet, Backward: false, Shape: Straight, SeatLanding: false},
	"rudi":            {Name: "Rudi", Rotation: 4, TwistDistribution: []int{3}, TakeoffPosition: Feet, Backward: false, Shape: Straight, SeatLanding: false},
	"randi":           {Name: "Randi", Rotation: 4, TwistDistribution: []int{5}, TakeoffPosition: Feet, Backward: false, Shape: Straight, SeatLanding: false},
	"fullBack":        {Name: "Full Back", Rotation: 4, TwistDistribution: []int{2}, TakeoffPosition: Feet, Backward: true, Shape: Straight, SeatLanding: false},
	"doubleFullBack":  {Name: "Double Full Back", Rotation: 4, TwistDistribution: []int{4}, TakeoffPosition: Feet, Backward: true, Shape: Straight, SeatLanding: false},
	"tuckBack":        {Name: "Tuck Back", Rotation: 4, TwistDistribution: []int{0}, TakeoffPosition: Feet, Backward: true, Shape: Tuck, SeatLanding: false},
	"pikeBack":        {Name: "Pike Back", Rotation: 4, TwistDistribution: []int{0}, TakeoffPosition: Feet, Backward: true, Shape: Pike, SeatLanding: false},
	"fullCody":        {Name: "Full Cody", Rotation: 5, TwistDistribution: []int{2}, TakeoffPosition: Front, Backward: true, Shape: Straight, SeatLanding: false},
	"cody":            {Name: "Cody", Rotation: 5, TwistDistribution: []int{0}, TakeoffPosition: Front, Backward: true, Shape: Tuck, SeatLanding: false},
	"doubleBackTuck":  {Name: "Double Back", Rotation: 8, TwistDistribution: []int{0, 0}, TakeoffPosition: Feet, Backward: true, Shape: Tuck, SeatLanding: false},
	"tripleBackTuck":  {Name: "Triple Back", Rotation: 12, TwistDistribution: []int{0, 0, 0}, TakeoffPosition: Feet, Backward: true, Shape: Tuck, SeatLanding: false},
	"halfOut":         {Name: "Half-Out", Rotation: 8, TwistDistribution: []int{0, 1}, TakeoffPosition: Feet, Backward: false, Shape: Tuck, SeatLanding: false},
	"halfOutPike":     {Name: "Pike Half-Out", Rotation: 8, TwistDistribution: []int{0, 1}, TakeoffPosition: Feet, Backward: false, Shape: Pike, SeatLanding: false},
	"halfhalf":        {Name: "Half Half", Rotation: 8, TwistDistribution: []int{1, 1}, TakeoffPosition: Feet, Backward: true, Shape: Tuck, SeatLanding: false},
	"halfhalfPike":    {Name: "Pike Half Half", Rotation: 8, TwistDistribution: []int{1, 1}, TakeoffPosition: Feet, Backward: true, Shape: Pike, SeatLanding: false},
	"trifHalfOut":     {Name: "Trif Half-Out", Rotation: 12, TwistDistribution: []int{0, 0, 1}, TakeoffPosition: Feet, Backward: false, Shape: Tuck, SeatLanding: false},
	"fullFull":        {Name: "Full Full", Rotation: 8, TwistDistribution: []int{2, 2}, TakeoffPosition: Feet, Backward: true, Shape: Straight, SeatLanding: false},
	"fullRudi":        {Name: "Full Rudi", Rotation: 8, TwistDistribution: []int{2, 3}, TakeoffPosition: Feet, Backward: false, Shape: Straight, SeatLanding: false},
	"miller":          {Name: "Miller", Rotation: 8, TwistDistribution: []int{3, 3}, TakeoffPosition: Feet, Backward: true, Shape: Straight, SeatLanding: false},
}

func GetCommonSkill(name string) (TrampolineSkill, bool) {
	skill, exists := CommonSkills[name]
	return skill, exists
}
func calculatePhases(rotation int) int {
	switch {
	case rotation <= 6:
		return 1
	case rotation <= 10:
		return 2
	case rotation <= 14:
		return 3
	default: // 15-16
		return 4
	}
}
func (skill *TrampolineSkill) Validate() error {
	requiredPhases := calculatePhases(skill.Rotation)
	if len(skill.TwistDistribution) != requiredPhases {
		return fmt.Errorf("requires %d twist phases for %d/4 rotation",
			requiredPhases, skill.Rotation)
	}
	return nil
}
