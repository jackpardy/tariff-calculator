package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// categoryTicked says which of Men and Women a page's entry form has ticked,
// and fails unless the form has the one choice.
func categoryTicked(t *testing.T, page string) string {
	t.Helper()
	radios := regexp.MustCompile(`<input type="radio" name="category" value="(Men|Women)"( checked)?`).FindAllStringSubmatch(page, -1)
	if len(radios) != 2 {
		t.Fatalf("one Men or Women choice, not %d radios", len(radios))
	}
	ticked := ""
	for _, r := range radios {
		if r[2] != "" {
			ticked += r[1]
		}
	}
	return ticked
}

func TestEditingKeepsMenOrWomen(t *testing.T) {
	h := competitionServer(t)
	form := newCompetition()
	form.Set("split", "all")
	admin := created(t, h, form)
	settings := do(t, h, http.MethodGet, admin+"/settings", nil).Body.String()
	enter := pathIn(t, settings, "/competitions/enter/")
	l3 := url.Values{"level": {"BUCS L3"}, "category": {"Women"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}}

	// An individual's own entry.
	individual := url.Values{"gymnast": {"Ann"}}
	for k, v := range l3 {
		individual[k] = v
	}
	own := withoutQuery(redirected(t, h, enter, individual))
	if got := categoryTicked(t, do(t, h, http.MethodGet, own, nil).Body.String()); got != "Women" {
		t.Errorf("the individual's entry has %q ticked", got)
	}

	// A club member's own entry, and the comp sec's edit of it.
	club := withoutQuery(redirected(t, h, "/clubs", url.Values{"name": {"UCD"}}))
	redirected(t, h, pathIn(t, settings, "/competitions/club/"), url.Values{"clubAdmin": {club}})
	join := pathIn(t, do(t, h, http.MethodGet, club+"/members", nil).Body.String(), "/clubs/join/")
	member := withoutQuery(redirected(t, h, join, url.Values{"name": {"Bea"}}))
	redirected(t, h, formAction(t, memberCompetitionPage(t, h, member), member+"/competitions/"), l3)
	if got := categoryTicked(t, memberCompetitionPage(t, h, member)); got != "Women" {
		t.Errorf("the member's entry has %q ticked", got)
	}
	edit := regexp.MustCompile(`href="(` + regexp.QuoteMeta(club) + `/members/[^"]+/competitions/[^"?]+)`).FindStringSubmatch(do(t, h, http.MethodGet, club, nil).Body.String())
	if edit == nil {
		t.Fatal("the comp sec's Change link")
	}
	page := do(t, h, http.MethodGet, edit[1], nil).Body.String()
	if got := categoryTicked(t, page); got != "Women" {
		t.Errorf("the comp sec's edit has %q ticked", got)
	}
	if strings.Contains(page, `class="field comp-entry-category" hidden`) {
		t.Error("the choice shows for a split level")
	}
}
