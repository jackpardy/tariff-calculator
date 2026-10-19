package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"tariffCalculator/catalog"
)

// A body-landing tile loads the version of it that takes off where the skill
// before it landed, saying so, or its usual skill where nothing lands like
// that from there.
func TestLandingTileFollowsThePreviousSkill(t *testing.T) {
	previous := []struct {
		landing, skill, note string
	}{
		{"feet", "", "From feet, at the start of the routine"},
		{"feet", `{"rotation":0,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck"}`, "From feet, after Tuck Jump"},
		{"seat", `{"rotation":0,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Straight","seat_landing":true}`, "From seat, after Seat Drop"},
		{"back", `{"rotation":1,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Straight","backward":true}`, "From back, after Back Drop"},
		{"front", `{"rotation":1,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Straight","custom_name":"Our drop"}`, "From front, after Our drop"},
	}
	want := map[string][]string{ // the card's name, by tile, after each of previous; "*" where it's the usual one, with no note
		"seatToFeet":     {"*Seat To Feet", "*Seat To Feet", "Seat To Feet", "Back To Feet", "Front To Feet"},
		"seatHalfToFeet": {"*Seat Half Twist To Feet", "*Seat Half Twist To Feet", "Seat Half Twist To Feet", "Back Half Twist To Feet", "Front Half Twist To Feet"},
		"seatDrop":       {"Seat Drop", "Seat Drop", "Seat To Seat", "*Seat Drop", "*Seat Drop"},
		"halfToSeat":     {"Half Twist To Seat", "Half Twist To Seat", "Seat Half Twist To Seat", "Back Half Twist To Seat", "Front Half Twist To Seat"},
		"backDrop":       {"Back Drop", "Back Drop", "Seat To Back", "Back To Back", "Front To Back"},
		"halfToBack":     {"Half Twist To Back", "Half Twist To Back", "Seat Half Twist To Back", "Back Half Twist To Back", "Front Half Twist To Back"},
		"catTwist":       {"Full Twist To Back", "Full Twist To Back", "Seat Full Twist To Back", "Back Full Twist To Back", "Front Full Twist To Back"},
		"corkscrew":      {"1½ Twist To Back", "1½ Twist To Back", "Seat 1½ Twist To Back", "Back 1½ Twist To Back", "Front 1½ Twist To Back"},
		"frontDrop":      {"Front Drop", "Front Drop", "Seat To Front", "Back To Front", "Front To Front"},
		"halfToFront":    {"Half Twist To Front", "Half Twist To Front", "Seat Half Twist To Front", "Back Half Twist To Front", "Front Half Twist To Front"},
	}
	for key, names := range want {
		if _, ok := catalog.LandingTile(key); !ok {
			t.Fatalf("%s isn't a tile", key)
		}
		for i, p := range previous {
			form := url.Values{"load": {"landing"}, "commonSkillKey": {key}, "editIndex": {"-1"}, "previous_skill": {p.skill}}
			rec := postForm(t, "/skill-inputs", form)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s after %s: status %d", key, p.landing, rec.Code)
			}
			html := rec.Body.String()
			name, usual := strings.CutPrefix(names[i], "*")
			if !strings.Contains(html, fmt.Sprintf(`<p class="skill-card-name">%s</p>`, name)) {
				t.Errorf("%s after %s %q: want %s", key, p.landing, p.skill, name)
			}
			if hasNote := strings.Contains(html, p.note); hasNote == usual {
				t.Errorf("%s after %s: note %q shown %v, want %v", key, p.landing, p.note, hasNote, !usual)
			}
			if usual && strings.Contains(html, "skill-card-follows") {
				t.Errorf("%s after %s: the usual skill has no note", key, p.landing)
			}
		}
	}

	// After a skill that can't land, or one that can't be read, it's the usual skill.
	for _, prev := range []string{`{"rotation":2,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Straight"}`, `not json`} {
		html := postForm(t, "/skill-inputs", url.Values{"load": {"landing"}, "commonSkillKey": {"halfToSeat"}, "editIndex": {"-1"}, "previous_skill": {prev}}).Body.String()
		if !strings.Contains(html, `<p class="skill-card-name">Half Twist To Seat</p>`) || strings.Contains(html, "skill-card-follows") {
			t.Errorf("after %s: want the usual Half Twist To Seat, no note", prev)
		}
	}
	// Anything but a tile loads nothing.
	if rec := postForm(t, "/skill-inputs", url.Values{"load": {"landing"}, "commonSkillKey": {"barani"}}); rec.Code != http.StatusNoContent {
		t.Errorf("a barani isn't a tile: status %d", rec.Code)
	}
}
