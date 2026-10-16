package web

import (
	"testing"

	"tariffCalculator/requirements"
)

// A level from before club skills could be rules lists them after its
// rules: they become rules at the end, once each.
func TestWithClubItems(t *testing.T) {
	rules := []requirements.Rule{{Type: requirements.Track, Item: "a"}, {Type: requirements.Count}}
	got := withClubItems(rules, []string{"a", "b"})
	if len(got) != 3 || got[2].Type != requirements.Track || got[2].Item != "b" || got[0].Item != "a" {
		t.Errorf("got %+v", got)
	}
	if len(rules) != 2 {
		t.Error("the level's own rules are left as they were")
	}
}
