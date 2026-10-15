package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// A form from a timetable page opened before someone else changed the draft
// is refused, as its flights and seats by position may now be others; one
// from the page as it is now goes through.
func TestStaleTimetableForm(t *testing.T) {
	h, _ := competitionServerAt(t, inDublin(2030, 3, 1, 10, 0))
	admin := created(t, h, newCompetition())
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	for _, name := range []string{"Ann", "Bea", "Cal", "Dan"} {
		redirected(t, h, enter, bucsEntry(name))
	}
	tt := admin + "/timetable"
	redirected(t, h, tt+"/plan", url.Values{})
	versionIn := func(page string) string {
		t.Helper()
		m := regexp.MustCompile(`name="version" value="([0-9a-f]+)"`).FindAllStringSubmatch(page, -1)
		if len(m) == 0 {
			t.Fatal("the timetable's forms carry its version")
		}
		for _, v := range m {
			if v[1] != m[0][1] {
				t.Fatalf("every form carries the same version: %s and %s", m[0][1], v[1])
			}
		}
		return m[0][1]
	}
	opened := do(t, h, http.MethodGet, tt, nil).Body.String()
	old := versionIn(opened)
	entry := regexp.MustCompile(`<option value="([^"]+)">\d+\. `).FindStringSubmatch(opened)[1]

	// A helper takes a gymnast out, from the page as it was.
	redirected(t, h, tt+"/entry", url.Values{"entry": {entry}, "to": {""}, "version": {old}})
	now := do(t, h, http.MethodGet, tt, nil).Body.String()
	if versionIn(now) == old {
		t.Fatal("a change gives the draft a new version")
	}

	// The organiser's page, opened before that, is out of date: each of its
	// forms that names a flight, gymnast, seat or rule by position is refused.
	stale := []struct {
		path string
		form url.Values
	}{
		{"/flight", url.Values{"flight": {"0"}, "action": {"redraw"}}},
		{"/entry", url.Values{"entry": {entry}, "to": {"0"}}},
		{"/officials", url.Values{"action": {"seat"}, "flight": {"0"}, "seat": {"0"}, "person": {""}}},
		{"/setup/days", url.Values{"day-0-name": {"Sunday"}, "day-0-start": {"09:00"}, "day-0-end": {"18:00"}}},
		{"/setup/rules", url.Values{"remove": {"0"}}},
		{"/publish", url.Values{"action": {"publish"}}},
	}
	for _, s := range stale {
		s.form.Set("version", old)
		if to := redirected(t, h, tt+s.path, s.form); noticeOf(t, to) != "The timetable changed since you opened this page; nothing was changed." {
			t.Errorf("%s from the old page: %q", s.path, noticeOf(t, to))
		}
	}
	if after := do(t, h, http.MethodGet, tt, nil).Body.String(); versionIn(after) != versionIn(now) || strings.Contains(after, "Sunday") {
		t.Error("nothing was changed by the refused forms")
	}
	if strings.Contains(dayAll(t, h, admin+"/day"), "Ann") {
		t.Error("nothing was published")
	}

	// From the page as it is now, the same change goes through.
	if to := redirected(t, h, tt+"/flight", url.Values{"flight": {"0"}, "action": {"redraw"}, "version": {versionIn(now)}}); noticeOf(t, to) != "" {
		t.Errorf("a redraw from the page as it is: %q", noticeOf(t, to))
	}
	if to := redirected(t, h, tt+"/entry", url.Values{"entry": {entry}, "to": {"0"}, "version": {versionIn(do(t, h, http.MethodGet, tt, nil).Body.String())}}); noticeOf(t, to) != "" {
		t.Errorf("putting the gymnast back: %q", noticeOf(t, to))
	}
}
