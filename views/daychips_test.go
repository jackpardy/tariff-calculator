package views

import (
	"strings"
	"testing"
)

// A panel with an official missing isn't done: their seat needs filling.
func TestPanelMissingIsAttention(t *testing.T) {
	got := render(t, countChips(seatCount([]DaySeat{{Person: "a", State: "here"}, {Person: "b", State: "missing"}})))
	if !strings.Contains(got, "chip-state-"+string(Attention)) || strings.Contains(got, "chip-state-"+string(Done)) {
		t.Errorf("a missing marshal: %s", got)
	}
	if got := render(t, countChips(checkinCount([]DayGymnast{{State: "here"}, {State: "scratched"}}))); !strings.Contains(got, "chip-state-"+string(Done)) {
		t.Errorf("gymnasts here or scratched are done: %s", got)
	}
}
