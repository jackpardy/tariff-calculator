package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// FIG notation in the skill search: each reading says where it takes off and
// lands; the first loads onto the card as it's typed, opening "Build or
// adjust" to finish one that isn't a named skill.
func TestNotationSearchLoadsTheCard(t *testing.T) {
	search := func(q string) string {
		t.Helper()
		rec := httptest.NewRecorder()
		routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/skill-search?q="+url.QueryEscape(q), nil))
		return rec.Body.String()
	}
	cody := search("5 0 /")
	if !strings.Contains(cody, ">Cody<") || !strings.Contains(cody, "Front → Feet") || strings.Count(cody, `hx-trigger="click, load"`) != 1 {
		t.Errorf("5 0 /: Cody among them, from the front, the first loaded: %s", cody)
	}
	if strings.Contains(cody[:strings.Index(cody, `hx-trigger="click, load"`)], "builder_open") {
		t.Error("a named skill doesn't open the builder")
	}
	custom := search("6 0 /")
	first := custom[:strings.Index(custom, `hx-trigger="click, load"`)]
	if !strings.Contains(custom, "Back → Front") || !strings.Contains(first, "builder_open") {
		t.Errorf("6 0 /: lands from the back, and opens the builder to finish it: %s", custom)
	}
	if name := search("barani"); strings.Contains(name, "load\"") && strings.Contains(name, `hx-trigger="click, load"`) {
		t.Error("a name search doesn't load anything by itself")
	}
}
