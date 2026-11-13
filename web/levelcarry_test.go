package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"tariffCalculator/competitions"
)

// carryFixture is a competition with tumbling levels T1 to T3 and three
// people who say they can judge up to T2 (execution): a club's member, whose
// club has sent it, an individual on their own entry, and one the organiser
// added.
type carryFixture struct {
	h                      http.Handler
	admin, club, member    string
	own, officials, offers string
}

func newCarryFixture(t *testing.T) carryFixture {
	t.Helper()
	f := carryFixture{h: competitionServer(t)}
	form := newCompetition()
	form.Set("tumbling", "T1\nT2\nT3")
	form.Set("dmt", "Open")
	f.admin = created(t, f.h, form)
	f.officials = f.admin + "/officials"
	dash := do(t, f.h, http.MethodGet, f.admin+"/settings", nil).Body.String()
	upTo := func(level string) url.Values {
		return url.Values{"judge-tumbling": {"1"}, "execution-tumbling": {level}, "difficulty-tumbling": {"no"}, "chair-tumbling": {"no"}}
	}
	redirected(t, f.h, f.officials+"/add", withName(upTo("T2"), "Ann"))
	enter := pathIn(t, dash, "/competitions/enter/")
	f.own = withoutQuery(redirected(t, f.h, enter, url.Values{"gymnast": {"Fay"}, "level": {"FIG AG3 (17–21)"}, "ex1Skills": {voluntary}, "ex2Skills": {voluntary}}))
	redirected(t, f.h, f.own+"/offer", upTo("T2"))
	f.club = withoutQuery(redirected(t, f.h, "/clubs", url.Values{"name": {"UCD"}}))
	redirected(t, f.h, pathIn(t, dash, "/competitions/club/"), url.Values{"clubAdmin": {f.club}})
	join := pathIn(t, do(t, f.h, http.MethodGet, f.club+"/members", nil).Body.String(), "/clubs/join/")
	f.member = withoutQuery(redirected(t, f.h, join, url.Values{"name": {"Bea"}}))
	offer := regexp.MustCompile(`action="(` + regexp.QuoteMeta(f.member) + `/competitions/[^"]+/offer)"`).FindStringSubmatch(memberCompetitionPage(t, f.h, f.member))[1]
	redirected(t, f.h, offer, upTo("T2"))
	page := clubEntriesPage(t, f.h, f.club)
	redirected(t, f.h, formAction(t, page, f.club+"/competitions/"), url.Values{"which": {"changed"}})
	if page := clubEntriesPage(t, f.h, f.club); !strings.Contains(page, "All sent") {
		t.Fatal("the club's offer is sent")
	}
	return f
}

// withoutFragment is a location without its #fragment, which a browser keeps.
func withoutFragment(location string) string {
	l, _, _ := strings.Cut(location, "#")
	return l
}

func withName(v url.Values, name string) url.Values {
	v.Set("name", name)
	return v
}

// events posts the tumbling and DMT levels.
func (f carryFixture) events(t *testing.T, tumbling, dmt string) string {
	t.Helper()
	return redirected(t, f.h, f.admin+"/events", url.Values{"tumbling": {tumbling}, "dmt": {dmt}})
}

func (f carryFixture) page(t *testing.T, path string) string {
	t.Helper()
	return do(t, f.h, http.MethodGet, path, nil).Body.String()
}

// Renaming a level that offers name asks whether to carry them over: nothing
// is carried unless the organiser says, leaving them keeps the flag, and
// carrying changes every place the competition keeps offers.
func TestRenameCarriesLevels(t *testing.T) {
	f := newCarryFixture(t)
	gone := "a level any more"

	location := f.events(t, "T1\nTier 2\nT3", "Open")
	if !strings.Contains(location, "carry=1") {
		t.Fatalf("saving a rename that offers name goes to the ask: %s", location)
	}
	settings := f.page(t, withoutFragment(location))
	for _, want := range []string{"You renamed T2 to Tier 2", "3 judges said they can judge up to T2.", "Carry them over to Tier 2", "Leave them"} {
		if !strings.Contains(settings, want) {
			t.Errorf("the ask lacks %q", want)
		}
	}
	if m := regexp.MustCompile(`<input[^>]*name="carry"[^>]*>`).FindString(settings); strings.Contains(m, "checked") {
		t.Errorf("nothing ticked: %s", m)
	}

	// Left: the flag stays, on the Officials page and the judge's own page.
	officials := f.page(t, f.officials)
	if strings.Count(officials, gone) != 3 {
		t.Errorf("each of three still flagged: %d", strings.Count(officials, gone))
	}
	if !strings.Contains(officials, "Carry them over to Tier 2") {
		t.Error("the Officials page offers it later")
	}
	if !strings.Contains(f.page(t, f.own), gone) {
		t.Error("Fay's page says it")
	}

	// Carrying: every place, every person.
	carry := f.admin + "/events/carry"
	location = redirected(t, f.h, carry, url.Values{"carry": {"tumbling:T2"}, "back": {"officials"}})
	if n := noticeOf(t, location); n != "Carried 3 judges' levels from T2 to Tier 2." {
		t.Errorf("notice: %q", n)
	}
	officials = f.page(t, f.officials)
	if strings.Contains(officials, gone) || strings.Contains(officials, "Carry them over") {
		t.Error("nothing left to carry or flag")
	}
	if strings.Count(officials, "execution up to Tier 2") != 3 {
		t.Errorf("all three say up to Tier 2: %d", strings.Count(officials, "execution up to Tier 2"))
	}
	if page := f.page(t, f.own); strings.Contains(page, gone) || !strings.Contains(page, "up to Tier 2") {
		t.Error("Fay's page follows")
	}
	if page := f.page(t, f.admin+"/history"); !strings.Contains(page, "Carried 3 judges&#39; levels from T2 to Tier 2") {
		t.Error("the history says so")
	}
	// A second go finds nothing.
	if n := noticeOf(t, redirected(t, f.h, carry, url.Values{"carry": {"tumbling:T2"}})); !strings.HasPrefix(n, "Nothing was carried") {
		t.Errorf("nothing again: %q", n)
	}

	// The club's own copy follows, so sending again doesn't bring T2 back.
	page := clubEntriesPage(t, f.h, f.club)
	if !strings.Contains(page, "All sent") {
		t.Error("the carried offer and the sent one are the same")
	}
	redirected(t, f.h, formAction(t, page, f.club+"/competitions/"), url.Values{"which": {"all"}})
	if officials := f.page(t, f.officials); strings.Contains(officials, gone) || strings.Count(officials, "execution up to Tier 2") != 3 {
		t.Error("a club send keeps the new name")
	}
}

// Declining then carrying later from the Officials page; a second rename
// chains from the first, and the box lists several renames with a tick each.
func TestRenameCarriesLater(t *testing.T) {
	f := newCarryFixture(t)
	f.events(t, "T1\nTier 2\nT3", "Open")
	// Renamed again before carrying: the old name is still the one offers have.
	if loc := f.events(t, "T1\nTier II\nT3", "Open"); !strings.Contains(loc, "carry=1") {
		t.Errorf("asks again: %s", loc)
	}
	if page := f.page(t, f.officials); !strings.Contains(page, "You renamed T2 to Tier II") || strings.Contains(page, "Tier 2") {
		t.Error("T2 goes to the latest name")
	}
	// Two at once: T1 and T3, each named by one more judge.
	redirected(t, f.h, f.officials+"/add", withName(url.Values{"judge-tumbling": {"1"}, "execution-tumbling": {"T1"}, "difficulty-tumbling": {"T3"}, "chair-tumbling": {"no"}}, "Cal"))
	loc := f.events(t, "Level 1\nTier II\nLevel 3", "Open")
	page := f.page(t, withoutFragment(loc))
	for _, want := range []string{"You renamed 3 levels", "Tumbling: T1 to Level 1 (1 judge said up to T1)", "Tumbling: T3 to Level 3 (1 judge said up to T3)", "Tumbling: T2 to Tier II (3 judges said up to T2)", "Carry the ticked ones over"} {
		if !strings.Contains(page, want) {
			t.Errorf("the ask lacks %q", want)
		}
	}
	boxes := regexp.MustCompile(`<input[^>]*name="carry"[^>]*>`).FindAllString(page, -1)
	if len(boxes) != 3 {
		t.Errorf("a tick for each rename: %v", boxes)
	}
	for _, b := range boxes {
		if strings.Contains(b, "checked") {
			t.Errorf("nothing ticked: %s", b)
		}
	}
	// Tick only T1: the others stay flagged and offered.
	redirected(t, f.h, f.admin+"/events/carry", url.Values{"carry": {"tumbling:T1"}, "back": {"officials"}})
	officials := f.page(t, f.officials)
	if !strings.Contains(officials, "Tumbling: T3 to Level 3") || strings.Contains(officials, "T1 to Level 1") {
		t.Error("T1 is carried, T3 still offered")
	}
	if !strings.Contains(officials, "execution up to Level 1") {
		t.Errorf("Cal says up to Level 1: %s", officials)
	}
	// An organiser's helper link can't carry.
	if allowed("timetable", "POST "+adminPrefix+"/events/carry") {
		t.Error("a helper link can't carry")
	}
}

// A level taken away isn't offered; neither is a rename no offer names; and a
// swapped pair of levels isn't a rename.
func TestRenameNotOffered(t *testing.T) {
	f := newCarryFixture(t)
	if loc := f.events(t, "T1\nT3", "Open"); strings.Contains(loc, "carry") {
		t.Errorf("a removed level isn't carried: %s", loc)
	}
	if page := f.page(t, f.officials); strings.Contains(page, "Carry them over") {
		t.Error("no ask for a removed level")
	}
	f = newCarryFixture(t)
	if loc := f.events(t, "T1\nT2\nT3", "Open Elite"); strings.Contains(loc, "carry") {
		t.Errorf("a DMT level no offer names asks nothing: %s", loc)
	}
	if loc := f.events(t, "T3\nT2\nT1", "Open Elite"); strings.Contains(loc, "carry") {
		t.Errorf("a reorder isn't a rename: %s", loc)
	}
	if loc := f.events(t, "T1\nT2\nT3\nT4", "Open Elite"); strings.Contains(loc, "carry") {
		t.Errorf("an added level isn't a rename: %s", loc)
	}
}

func TestRenamedLevels(t *testing.T) {
	for _, c := range []struct {
		was, now []string
		want     [][2]string
	}{
		{[]string{"A", "B"}, []string{"A", "C"}, [][2]string{{"B", "C"}}},
		{[]string{"A", "B"}, []string{"B", "A"}, nil},
		{[]string{"A", "B", "C"}, []string{"X", "B", "Y"}, [][2]string{{"A", "X"}, {"C", "Y"}}},
		{[]string{"A", "B"}, []string{"A"}, nil},
		{[]string{"A", "B"}, []string{"A", "B", "C"}, nil},
		{[]string{"A", "B"}, []string{"A", "C", "D"}, nil},
		{[]string{"A"}, []string{"B"}, [][2]string{{"A", "B"}}},
	} {
		if got := competitions.RenamedLevels(c.was, c.now); len(got) != len(c.want) || (len(got) > 0 && got[0] != c.want[0]) || (len(got) > 1 && got[1] != c.want[1]) {
			t.Errorf("%v to %v: %v, want %v", c.was, c.now, got, c.want)
		}
	}
}
