package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// Where men and women rank apart, the entries page has a section for each
// ("BUCS L3 · Men"), its cards printing as a part of the level, and the rows
// lose the category tag their heading gives; a level not split keeps one
// section with the tags.
func TestEntriesSplitByCategory(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	redirected(t, h, admin+"/split", url.Values{"split": {"some"}, "splitLevel": {"BUCS L3"}})
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	for name, category := range map[string]string{"Ann": "Women", "Bob": "Men", "Cat": "Women"} {
		e := bucsEntry(name)
		e.Set("category", category)
		redirected(t, h, enter, e)
	}
	page := do(t, h, http.MethodGet, admin, nil).Body.String()
	men, women := strings.Index(page, `data-level="BUCS L3 · Men"`), strings.Index(page, `data-level="BUCS L3 · Women"`)
	if men < 0 || women < men {
		t.Fatalf("a section for men, then women: %d %d", men, women)
	}
	if !strings.Contains(page, `data-group-count="1"`) || !strings.Contains(page, `data-group-count="2"`) {
		t.Error("each counts its own")
	}
	if strings.Contains(page[women:], `<span class="tag is-light ml-1">Women</span>`) {
		t.Error("no category tag in a section by category")
	}
	if !strings.Contains(page, `/cards?level=BUCS+L3&amp;part=Women"`) || !strings.Contains(page, ">Print these cards") {
		t.Error("a section's cards are a part of its level")
	}
	if strings.Contains(page, `data-level="BUCS L3"`) {
		t.Error("no section for the level as a whole")
	}
}
