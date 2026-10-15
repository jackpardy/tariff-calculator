package web

import (
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// skillIn is the skill a page's first mark form for a name posts, as JSON.
func skillIn(t *testing.T, page, name string) string {
	t.Helper()
	m := regexp.MustCompile(`(?s)<strong>` + regexp.QuoteMeta(name) + `</strong>.*?<input type="hidden" name="skill" value="([^"]+)"`).FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("no %s to mark", name)
	}
	return html.UnescapeString(m[1])
}

// sideIn is the role and state chips a page shows for a skill's mark on one
// side ("gymnast" or "coach"), as text, e.g. "Coach · Ann ✓ Can do".
func sideIn(page, name, side string) string {
	m := regexp.MustCompile(`(?s)<strong>` + regexp.QuoteMeta(name) + `</strong>.*?<div class="skill-sides mt-1">(.*?)</div>`).FindStringSubmatch(page)
	if m == nil {
		return ""
	}
	s := regexp.MustCompile(`(?s)<span class="skill-side"><span class="chip chip-who chip-who-` + side + `">(.*?)</span>\s*<span class="has-text-grey`).FindStringSubmatch(m[1])
	if s == nil {
		return ""
	}
	text := regexp.MustCompile(`<[^>]+>`).ReplaceAllString(s[1], " ")
	return strings.Join(strings.Fields(html.UnescapeString(text)), " ")
}

// A gymnast marks what they're learning, their coach marks it too, and both
// marks show, each saying who; the comp sec sees them, and, where the club
// shares skills, so do the other members.
func TestMarkingSkills(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	club := aClub(t, h, admin, "UCD", false, "A. Murphy", "B. Kelly")
	member, other := club.members["A. Murphy"], club.members["B. Kelly"]
	ann := pathIn(t, do(t, h, http.MethodPost, club.page+"/coaches", url.Values{"name": {"Ann"}}).Body.String(), "/clubs/coach/")

	page := do(t, h, http.MethodGet, member+"/skills?q=barani", nil).Body.String()
	if got := currentTab(t, page); got != "Skills" {
		t.Errorf("the Skills tab is %q", got)
	}
	for _, want := range []string{"Mark a skill", "Tuck Barani", "Singles"} {
		if !strings.Contains(page, want) {
			t.Errorf("the Skills tab should have %q", want)
		}
	}
	barani := skillIn(t, page, "Tuck Barani")
	location := redirected(t, h, member+"/skills?q=barani", url.Values{"skill": {barani}, "state": {"can"}})
	if noticeOf(t, location) != "Tuck Barani: Can do." || !strings.Contains(location, "q=barani") {
		t.Errorf("marking: %s", location)
	}
	page = do(t, h, http.MethodGet, member+"/skills", nil).Body.String()
	if got := sideIn(page, "Tuck Barani", "gymnast"); got != "Gymnast · You ✓ Can do" {
		t.Errorf("the gymnast's own mark: %q", got)
	}

	// The coach sees it to confirm, but says Learning: both marks show.
	gymnasts := do(t, h, http.MethodGet, ann+"/gymnasts", nil).Body.String()
	if !strings.Contains(gymnasts, "1 to confirm") {
		t.Errorf("the coach's list says there's one to confirm: %s", gymnasts)
	}
	open := regexp.MustCompile(`href="(` + regexp.QuoteMeta(ann) + `/gymnasts/[^"]+)">A. Murphy</a>`).FindStringSubmatch(gymnasts)[1]
	skills := do(t, h, http.MethodGet, open+"/skills", nil).Body.String()
	if !strings.Contains(skills, ">Confirm Can do<") || sideIn(skills, "Tuck Barani", "gymnast") != "Gymnast · A. Murphy ✓ Can do" {
		t.Fatalf("the coach's page for the gymnast's skills: %s", skills)
	}
	redirected(t, h, open+"/skills", url.Values{"skill": {barani}, "state": {"learning"}})
	page = do(t, h, http.MethodGet, member+"/skills", nil).Body.String()
	if g, c := sideIn(page, "Tuck Barani", "gymnast"), sideIn(page, "Tuck Barani", "coach"); g != "Gymnast · You ✓ Can do" || c != "Coach · Ann ◐ Learning" {
		t.Errorf("both marks show: %q and %q", g, c)
	}
	if !strings.Contains(page, "Marked differently") {
		t.Error("a skill the two mark differently is listed as such")
	}

	// The coach confirms it: both say Can do, and nothing's left to confirm.
	redirected(t, h, open+"/skills", url.Values{"skill": {barani}, "state": {"can"}})
	page = do(t, h, http.MethodGet, member+"/skills", nil).Body.String()
	if g, c := sideIn(page, "Tuck Barani", "gymnast"), sideIn(page, "Tuck Barani", "coach"); g != "Gymnast · You ✓ Can do" || c != "Coach · Ann ✓ Can do" || strings.Contains(page, "Marked differently") {
		t.Errorf("both say Can do: %q and %q", g, c)
	}
	if strings.Contains(do(t, h, http.MethodGet, ann+"/gymnasts", nil).Body.String(), "1 to confirm") {
		t.Error("nothing left to confirm")
	}

	// The gymnast takes theirs off; the coach's stays.
	redirected(t, h, member+"/skills", url.Values{"skill": {barani}, "state": {""}})
	page = do(t, h, http.MethodGet, member+"/skills", nil).Body.String()
	if sideIn(page, "Tuck Barani", "gymnast") != "" || sideIn(page, "Tuck Barani", "coach") != "Coach · Ann ✓ Can do" {
		t.Errorf("the coach's mark stays: %s", page)
	}

	// The comp sec sees it, to look at.
	members := do(t, h, http.MethodGet, club.page+"/members", nil).Body.String()
	link := regexp.MustCompile(`href="(` + regexp.QuoteMeta(club.page) + `/members/[^"]+/skills)">can do 1</a>`).FindStringSubmatch(members)
	if link == nil {
		t.Fatalf("Members links to the skills: %s", members)
	}
	page = do(t, h, http.MethodGet, link[1], nil).Body.String()
	if !strings.Contains(page, "Tuck Barani") || strings.Contains(page, "Mark a skill") {
		t.Errorf("the comp sec's view: %s", page)
	}

	// Other members see it only where the club shares skills.
	id := strings.TrimSuffix(strings.TrimPrefix(link[1], club.page+"/members/"), "/skills")
	if rec := do(t, h, http.MethodGet, other+"/skills/"+id, nil); rec.Code != http.StatusNotFound {
		t.Errorf("another member before the club shares skills: %d", rec.Code)
	}
	redirected(t, h, club.page+"/skills-open", url.Values{"on": {"1"}})
	page = do(t, h, http.MethodGet, other+"/skills", nil).Body.String()
	if !strings.Contains(page, `href="`+other+"/skills/"+id+`"`) || !strings.Contains(page, "and so does everyone in your club") {
		t.Errorf("another member's Skills tab lists the club: %s", page)
	}
	page = do(t, h, http.MethodGet, other+"/skills/"+id, nil).Body.String()
	if !strings.Contains(page, "Tuck Barani") || strings.Contains(page, "Mark a skill") {
		t.Errorf("another member sees A. Murphy's skills, to look at: %s", page)
	}

	if rec := do(t, h, http.MethodPost, member+"/skills", url.Values{"skill": {`{"rotation":99}`}, "state": {"can"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("an unreadable skill: %d", rec.Code)
	}
}
