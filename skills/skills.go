package skills

import (
	"encoding/json"
	"strings"
)

type TrampolineSkill struct {
	Name              string
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
	return json.Marshal(pos.String())
}
func (pos *BodyPosition) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	for k, v := range BodyPositionName {
		if strings.EqualFold(s, v) {
			*pos = k
			return nil
		}
	}
	*pos = Invalid
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
