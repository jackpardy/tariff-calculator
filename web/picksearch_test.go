package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"
)

// The requirements editor's skill lists search as the skill search does:
// another name for a skill, or its FIG notation, finds its pick.
func TestPickSearch(t *testing.T) {
	ids := func(q string) []string {
		t.Helper()
		rec := httptest.NewRecorder()
		routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/requirements/pick-search?q="+url.QueryEscape(q), nil))
		var out []string
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("%s: %v %s", q, err, rec.Body)
		}
		return out
	}
	if got := ids("back s/s"); !slices.Contains(got, "skill:Back") {
		t.Errorf("back s/s: %v", got)
	}
	if got := ids("4 1 o"); !slices.Contains(got, "skill:Barani") {
		t.Errorf("4 1 o: %v", got)
	}
	if got := ids(""); len(got) != 0 {
		t.Errorf("nothing for nothing: %v", got)
	}
}
