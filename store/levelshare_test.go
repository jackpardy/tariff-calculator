package store

import (
	"strings"
	"testing"
)

// A shared level's rules of club skills point at the club's own once added:
// by their place in the shared list, dropping any the club doesn't track,
// and saying which places a rule used.
func TestPlaceSharedItems(t *testing.T) {
	set := `{"format":1,"name":"G","rules":[{"type":"track","item":"shared:1"},{"type":"count","min":1,"match":{}},{"type":"track","item":"shared:0"},{"type":"track","item":"shared:7"}]}`
	got, used, err := placeSharedItems(set, []string{"", "kab"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `{"item":"kab","type":"track"}`) || strings.Count(got, `"type":"track"`) != 1 || !strings.Contains(got, `"name":"G"`) {
		t.Errorf("placed: %s", got)
	}
	if !used[1] || used[0] || len(used) != 1 {
		t.Errorf("used: %v", used)
	}
}
