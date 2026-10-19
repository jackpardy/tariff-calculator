package web

import (
	"encoding/json"
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
// side ("gymnast" or "coach"), as text, e.g. "Coach · Ann Can do".
func sideIn(page, name, side string) string {
	m := regexp.MustCompile(`(?s)<strong>` + regexp.QuoteMeta(name) + `</strong>.*?<div class="skill-sides mt-1">(.*?)</div>`).FindStringSubmatch(page)
	if m == nil {
		return ""
	}
	s := regexp.MustCompile(`(?s)<span class="skill-side"><span class="chip chip-who chip-who-` + side + `"[^>]*>(.*?)</span>\s*<span class="has-text-grey`).FindStringSubmatch(m[1])
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
	if got := sideIn(page, "Tuck Barani", "gymnast"); got != "You Can do" {
		t.Errorf("the gymnast's own mark: %q", got)
	}

	// The coach sees it to confirm, but says Learning: both marks show.
	gymnasts := do(t, h, http.MethodGet, ann+"/gymnasts", nil).Body.String()
	if !strings.Contains(gymnasts, "1 to confirm") {
		t.Errorf("the coach's list says there's one to confirm: %s", gymnasts)
	}
	open := regexp.MustCompile(`href="(` + regexp.QuoteMeta(ann) + `/gymnasts/[^"]+)">A. Murphy</a>`).FindStringSubmatch(gymnasts)[1]
	skills := do(t, h, http.MethodGet, open+"/skills", nil).Body.String()
	if !strings.Contains(skills, ">Confirm Can do<") || sideIn(skills, "Tuck Barani", "gymnast") != "Gymnast · A. Murphy Can do" {
		t.Fatalf("the coach's page for the gymnast's skills: %s", skills)
	}
	redirected(t, h, open+"/skills", url.Values{"skill": {barani}, "state": {"learning"}})
	page = do(t, h, http.MethodGet, member+"/skills", nil).Body.String()
	if g, c := sideIn(page, "Tuck Barani", "gymnast"), sideIn(page, "Tuck Barani", "coach"); g != "You Can do" || c != "Coach · Ann Learning" {
		t.Errorf("both marks show: %q and %q", g, c)
	}
	if !strings.Contains(page, "Marked differently") {
		t.Error("a skill the two mark differently is listed as such")
	}

	// The coach confirms it: both say Can do, and nothing's left to confirm.
	redirected(t, h, open+"/skills", url.Values{"skill": {barani}, "state": {"can"}})
	page = do(t, h, http.MethodGet, member+"/skills", nil).Body.String()
	if g, c := sideIn(page, "Tuck Barani", "gymnast"), sideIn(page, "Tuck Barani", "coach"); g != "You Can do" || c != "Coach · Ann Can do" || strings.Contains(page, "Marked differently") {
		t.Errorf("both say Can do: %q and %q", g, c)
	}
	if strings.Contains(do(t, h, http.MethodGet, ann+"/gymnasts", nil).Body.String(), "1 to confirm") {
		t.Error("nothing left to confirm")
	}

	// The gymnast takes theirs off; the coach's stays.
	redirected(t, h, member+"/skills", url.Values{"skill": {barani}, "state": {""}})
	page = do(t, h, http.MethodGet, member+"/skills", nil).Body.String()
	if sideIn(page, "Tuck Barani", "gymnast") != "" || sideIn(page, "Tuck Barani", "coach") != "Coach · Ann Can do" {
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

// Coaches keep the club's own skills: a name for a catalog skill, which then
// shows in its place on the club's pages, and something else to track, which
// members and coaches mark like any skill.
func TestClubSkills(t *testing.T) {
	h := competitionServer(t)
	club := aClub(t, h, created(t, h, newCompetition()), "UCD", false, "A. Murphy")
	member := club.members["A. Murphy"]
	ann := pathIn(t, do(t, h, http.MethodPost, club.page+"/coaches", url.Values{"name": {"Ann"}}).Body.String(), "/clubs/coach/")

	page := do(t, h, http.MethodGet, ann+"/club-skills?q=seat+half+twist+to+seat", nil).Body.String()
	if got := currentTab(t, page); got != "Club skills" {
		t.Errorf("the coach's Club skills section is %q", got)
	}
	m := regexp.MustCompile(`<input type="hidden" name="skill" value="([^"]+)"><p><strong>Seat Half Twist To Seat</strong>`).FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("no Seat Half Twist To Seat to name: %s", page)
	}
	swivel := html.UnescapeString(m[1])
	if notice := noticeOf(t, redirected(t, h, ann+"/club-skills", url.Values{"skill": {swivel}, "name": {"Swivel Hips"}})); notice != "Added Swivel Hips." {
		t.Errorf("naming a skill: %q", notice)
	}
	redirected(t, h, ann+"/club-skills", url.Values{"name": {"Kaboom drill"}, "note": {"On the crash mat"}})
	if notice := noticeOf(t, redirected(t, h, ann+"/club-skills", url.Values{"name": {"swivel hips"}})); !strings.Contains(notice, "already has a skill called") {
		t.Errorf("a name already used: %q", notice)
	}
	page = do(t, h, http.MethodGet, ann+"/club-skills", nil).Body.String()
	for _, want := range []string{"Swivel Hips", "Seat Half Twist To Seat", "Kaboom drill", "On the crash mat", "To track", `<span class="chip-role">Coach</span> <span class="chip-name">· <span class="chip-name-text">Ann</span></span>`} {
		if !strings.Contains(page, want) {
			t.Errorf("the club's skills should show %q", want)
		}
	}

	// The member marks both from Your club's skills.
	page = do(t, h, http.MethodGet, member+"/skills", nil).Body.String()
	if !strings.Contains(page, "Your club&#39;s skills") {
		t.Fatalf("Mark a skill offers the club's: %s", page)
	}
	kaboom := regexp.MustCompile(`<input type="hidden" name="club" value="([^"]+)"`).FindStringSubmatch(page)
	if kaboom == nil {
		t.Fatal("no club skill to mark")
	}
	redirected(t, h, member+"/skills", url.Values{"club": {kaboom[1]}, "state": {"learning"}})
	if notice := noticeOf(t, redirected(t, h, member+"/skills", url.Values{"skill": {swivel}, "state": {"can"}})); notice != "Swivel Hips: Can do." {
		t.Errorf("marking a named skill: %q", notice)
	}
	page = do(t, h, http.MethodGet, member+"/skills", nil).Body.String()
	if sideIn(page, "Kaboom drill", "gymnast") != "You Learning" || sideIn(page, "Swivel Hips", "gymnast") != "You Can do" {
		t.Errorf("both marked, under the club's names: %s", page)
	}
	if rec := do(t, h, http.MethodPost, member+"/skills", url.Values{"club": {"nope"}, "state": {"can"}}); rec.Code != http.StatusNotFound {
		t.Errorf("a club skill that isn't one: %d", rec.Code)
	}

	// A kept routine with the skill shows the club's name for it.
	var skill map[string]any
	if err := json.Unmarshal([]byte(swivel), &skill); err != nil {
		t.Fatal(err)
	}
	redirected(t, h, member+"/routines", keep(t, map[string]any{"id": "b1", "name": "Drops", "skills": []any{skill}}))
	if page = do(t, h, http.MethodGet, member+"/routines", nil).Body.String(); !strings.Contains(page, "Swivel Hips") {
		t.Errorf("the routine shows the club's name: %s", page)
	}

	// Removing something to track takes its marks with it.
	remove := formAction(t, do(t, h, http.MethodGet, ann+"/club-skills", nil).Body.String(), ann+"/club-skills/"+kaboom[1])
	other := aClub(t, h, created(t, h, newCompetition()), "DCU", false)
	bob := pathIn(t, do(t, h, http.MethodPost, other.page+"/coaches", url.Values{"name": {"Bob"}}).Body.String(), "/clubs/coach/")
	if rec := do(t, h, http.MethodPost, bob+strings.TrimPrefix(remove, ann), nil); rec.Code != http.StatusNotFound {
		t.Errorf("another club's coach removes it: %d", rec.Code)
	}
	if page := do(t, h, http.MethodPost, remove, nil).Body.String(); !strings.Contains(page, "marks on it go with it") || !strings.Contains(page, `name="anyway" value="1"`) {
		t.Errorf("removing a club skill asks first: %s", page)
	}
	redirected(t, h, remove, url.Values{"anyway": {"1"}})
	if page = do(t, h, http.MethodGet, member+"/skills", nil).Body.String(); strings.Contains(page, "Kaboom drill") {
		t.Error("its marks go with it")
	}
}
