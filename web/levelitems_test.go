package web

import (
	"net/url"
	"strings"
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

// "No repeats" isn't a rule to add: "Elements may be repeated", unticked,
// already says so. Requirements that have one (the built-in ones) keep it.
func TestNoRepeatsOnlyWhereItIs(t *testing.T) {
	fresh := postForm(t, "/requirements/editor", url.Values{"set": {`{"format":1,"name":"x","rules":[]}`}}).Body.String()
	if strings.Contains(fresh, `value="different"`) || !strings.Contains(fresh, "a repeated element is flagged and scores no difficulty") {
		t.Errorf("not offered, and the tick box says what it does: %s", fresh)
	}
	kept := postForm(t, "/requirements/editor", url.Values{"set": {`{"format":1,"name":"x","rules":[{"type":"different"}]}`}}).Body.String()
	if !strings.Contains(kept, `<option value="different" selected`) {
		t.Errorf("kept where it is: %s", kept)
	}
	both := postForm(t, "/requirements/editor", url.Values{"set": {`{"format":1,"name":"x","repeats_allowed":true,"rules":[{"type":"different"}]}`}}).Body.String()
	if !strings.Contains(both, "These can&#39;t all be met") && !strings.Contains(both, "These can't all be met") {
		t.Errorf("both ways flagged: %s", both)
	}
}
