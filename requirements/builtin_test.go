package requirements

import "testing"

func TestBuiltinLevelIDsAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, l := range BuiltinLevels() {
		if seen[l.ID] {
			t.Errorf("two built-in levels share the ID %q", l.ID)
		}
		seen[l.ID] = true
	}
	if len(builtinLevelsByID) != len(BuiltinLevels()) {
		t.Errorf("%d levels by ID, %d levels", len(builtinLevelsByID), len(BuiltinLevels()))
	}
}
