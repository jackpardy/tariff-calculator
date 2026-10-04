package requirements

import (
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	l, err := ParseLevel([]byte(`{"format":1,"name":"Student L3","first":{"options":["builtin:bucs-l3-option-1","set-abc"]},"second":{"options":["builtin:bucs-l3-second"]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := l.Exercise(1).Options; len(got) != 2 || got[1] != "set-abc" {
		t.Errorf("first exercise: %v", got)
	}
	if got := l.Exercise(2).Options; len(got) != 1 || got[0] != "builtin:bucs-l3-second" {
		t.Errorf("second exercise: %v", got)
	}

	same, err := ParseLevel([]byte(`{"format":1,"name":"Two voluntaries","first":{"options":["set-abc"]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := same.Exercise(2).Options; len(got) != 1 || got[0] != "set-abc" {
		t.Errorf("without a second exercise, both use the first's requirements: %v", got)
	}

	for _, tc := range []struct{ json, want string }{
		{`{"format":1,"name":"","first":{"options":["set-a"]}}`, "needs a name"},
		{`{"format":1,"name":"x","first":{"options":[]}}`, "first exercise needs requirements"},
		{`{"format":1,"name":"x","first":{"options":["set-a"]},"second":{"options":[]}}`, "second exercise needs requirements"},
		{`{"format":1,"name":"x","first":{"options":["set-a","set-a"]}}`, "twice"},
		{`{"format":1,"name":"x","first":{"options":["builtin:nope"]}}`, `"nope" don't exist`},
		{`{"format":1,"name":"x","first":{"options":["set-a"]},"secnd":{}}`, "unknown field"},
	} {
		if _, err := ParseLevel([]byte(tc.json)); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want %q", tc.json, err, tc.want)
		}
	}
}

func TestBuiltinLevels(t *testing.T) {
	if len(BuiltinLevels()) == 0 {
		t.Fatal("no built-in levels")
	}
	for _, b := range BuiltinLevels() {
		if err := b.Level.Validate(); err != nil {
			t.Errorf("%s: %v", b.ID, err)
		}
		if b.Level.Source == "" {
			t.Errorf("%s has no source", b.ID)
		}
		if got, ok := LookupBuiltinLevel(BuiltinLevelPrefix + b.ID); !ok || got.Name != b.Level.Name {
			t.Errorf("%s doesn't look up", b.ID)
		}
	}

	// A set-routine level: either set routine, then a voluntary.
	l3, _ := LookupBuiltinLevel("bucs-l3")
	for _, ref := range l3.Exercise(1).Options {
		if set, _ := LookupBuiltin(ref); !isSetRoutine(set) {
			t.Errorf("BUCS L3's first exercise option %s isn't a set routine", ref)
		}
	}
	if set, _ := LookupBuiltin(l3.Exercise(2).Options[0]); isSetRoutine(set) {
		t.Error("BUCS L3's second exercise should be a voluntary")
	}

	// Both exercises under the same rules.
	club, _ := LookupBuiltinLevel("bg-club-l1")
	if club.Second != nil || club.Exercise(2).Options[0] != "builtin:bg-club-l1" {
		t.Errorf("BG Club L1 is the set routine twice: %+v", club)
	}

	if ag3, _ := LookupBuiltinLevel("fig-ag3"); !ag3.ScoredOnce {
		t.Error("FIG AG3's first-exercise scoring elements score once")
	}

	// Every built-in set belongs to a level.
	used := map[string]bool{}
	for _, b := range BuiltinLevels() {
		for n := 1; n <= 2; n++ {
			for _, ref := range b.Level.Exercise(n).Options {
				used[strings.TrimPrefix(ref, BuiltinPrefix)] = true
			}
		}
	}
	for _, b := range Builtins() {
		if !used[b.ID] {
			t.Errorf("built-in set %s isn't in any level", b.ID)
		}
	}
}

func isSetRoutine(s Set) bool {
	_, ok := SetRoutine(s)
	return ok
}
