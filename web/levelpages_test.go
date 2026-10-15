package web

import (
	"encoding/base64"
	"encoding/json"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"tariffCalculator/skills"
)

// skillJSON is a common skill as the browser posts it.
func skillJSON(t *testing.T, key string) map[string]any {
	t.Helper()
	data, err := json.Marshal(skills.CommonSkills[key])
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// levelIn is a level's progress as text, from its summary down to its items.
func levelIn(page, name string) string {
	m := regexp.MustCompile(`(?s)<details class="box comp-level-progress"><summary><strong>` + regexp.QuoteMeta(name) + `</strong>(.*?)</details>`).FindStringSubmatch(page)
	if m == nil {
		return ""
	}
	text := regexp.MustCompile(`<[^>]+>`).ReplaceAllString(m[1], " ")
	return strings.Join(strings.Fields(html.UnescapeString(text)), " ")
}

// A coach saves requirements from the Requirements page as a club level,
// with a club skill to track; each gymnast's Skills shows how far through it
// they are, from the skills marked and the items marked as a whole.
func TestClubLevels(t *testing.T) {
	h := competitionServer(t)
	club := aClub(t, h, created(t, h, newCompetition()), "UCD", false, "A. Murphy")
	member := club.members["A. Murphy"]
	ann := pathIn(t, do(t, h, http.MethodPost, club.page+"/coaches", url.Values{"name": {"Ann"}}).Body.String(), "/clubs/coach/")
	redirected(t, h, ann+"/club-skills", url.Values{"name": {"Kaboom drill"}})

	set := `{"format":1,"name":"Grade 1","rules":[
		{"type":"count","label":"A back somersault","match":{"rotation":{"min":4,"max":4},"direction":"backward"},"min":1},
		{"type":"includes","label":"Back drop, then back to feet","options":[[{"takeoff":["feet"],"landing":["back"]},{"takeoff":["back"],"landing":["feet"]}]]},
		{"type":"elements","min":10,"max":10}
	]}`
	location := redirected(t, h, ann+"/levels", url.Values{"set": {set}, "id": {"b1"}})
	if notice := noticeOf(t, location); notice != "Added Grade 1 to the club's levels. 1 rule about a single routine (its length, difficulty or the like) can't be tracked from skills, so it was left out." {
		t.Errorf("saving: %q", notice)
	}
	page := do(t, h, http.MethodGet, ann+"/levels", nil).Body.String()
	if got := currentTab(t, page); got != "Levels" {
		t.Errorf("the coach's Levels section is %q", got)
	}
	for _, want := range []string{"Grade 1", "A back somersault", "Back drop, then back to feet", "Kaboom drill", "Open in the Requirements page"} {
		if !strings.Contains(page, want) {
			t.Errorf("the Levels page should have %q", want)
		}
	}
	if strings.Contains(page, "Exactly 10 elements") {
		t.Error("the rule about a single routine is left out")
	}
	m := regexp.MustCompile(`href="/requirements#share=j([A-Za-z0-9_-]+)"`).FindStringSubmatch(page)
	if m == nil {
		t.Fatal("no link to edit the requirements")
	}
	if data, _ := base64.RawURLEncoding.DecodeString(m[1]); !strings.Contains(string(data), `"name":"Grade 1"`) || !strings.Contains(string(data), `"v":1`) {
		t.Errorf("the requirements to edit: %s", data)
	}
	kaboom := regexp.MustCompile(`name="item" value="([^"]+)"`).FindStringSubmatch(page)[1]
	items := regexp.MustCompile(`action="(` + regexp.QuoteMeta(ann) + `/levels/[^/"]+/items)"`).FindStringSubmatch(page)[1]
	redirected(t, h, items, url.Values{"item": {kaboom}})

	// Saving the same requirements again changes the level.
	if notice := noticeOf(t, redirected(t, h, ann+"/levels", url.Values{"set": {strings.Replace(set, `,
		{"type":"elements","min":10,"max":10}`, "", 1)}, "id": {"b1"}})); notice != "Saved Grade 1." {
		t.Errorf("saving again: %q", notice)
	}

	// Nothing marked: nothing done.
	page = do(t, h, http.MethodGet, member+"/skills", nil).Body.String()
	if got := levelIn(page, "Grade 1"); !strings.HasPrefix(got, "Gymnast 0 of 3 Coach 0 of 3") {
		t.Errorf("before: %q", got)
	}

	// A back somersault marked Can do meets the first item; the link is
	// marked as a whole, and a kept routine that does it says so.
	back, _ := json.Marshal(skillJSON(t, "backSomersault"))
	redirected(t, h, member+"/skills", url.Values{"skill": {string(back)}, "state": {"can"}})
	redirected(t, h, member+"/routines", keep(t, map[string]any{"id": "r1", "name": "Drops", "skills": []any{skillJSON(t, "backDrop"), skillJSON(t, "backToFeet")}}))
	page = do(t, h, http.MethodGet, member+"/skills", nil).Body.String()
	link := regexp.MustCompile(`name="item" value="(r:[0-9a-f]+)"`).FindStringSubmatch(page)
	if link == nil {
		t.Fatalf("the link is marked as a whole: %s", levelIn(page, "Grade 1"))
	}
	if got := levelIn(page, "Grade 1"); !strings.Contains(got, "Gymnast 1 of 3") || !strings.Contains(got, "A back somersault From the skills marked Gymnast · You ✓ Can do") || !strings.Contains(got, "Done in Drops") {
		t.Errorf("the derived item, and the routine doing the link: %q", got)
	}
	if notice := noticeOf(t, redirected(t, h, member+"/skills", url.Values{"item": {link[1]}, "state": {"learning"}})); notice != "Back drop, then back to feet: Learning." {
		t.Errorf("marking the link: %q", notice)
	}
	redirected(t, h, member+"/skills", url.Values{"club": {kaboom}, "state": {"can"}})

	// The coach marks the link Can do; both sides show, and the counts.
	gymnasts := do(t, h, http.MethodGet, ann+"/gymnasts", nil).Body.String()
	open := regexp.MustCompile(`href="(` + regexp.QuoteMeta(ann) + `/gymnasts/[^"]+)">A. Murphy</a>`).FindStringSubmatch(gymnasts)[1]
	redirected(t, h, open+"/skills", url.Values{"item": {link[1]}, "state": {"can"}})
	page = do(t, h, http.MethodGet, member+"/skills", nil).Body.String()
	got := levelIn(page, "Grade 1")
	if !strings.Contains(got, "Gymnast 2 of 3 Coach 1 of 3") || !strings.Contains(got, "Back drop, then back to feet Done in Drops Gymnast · You ◐ Learning") || !strings.Contains(got, "Coach · Ann ✓ Can do") {
		t.Errorf("after the coach marks the link: %q", got)
	}
	if rec := do(t, h, http.MethodPost, member+"/skills", url.Values{"item": {"r:nope"}, "state": {"can"}}); rec.Code != http.StatusNotFound {
		t.Errorf("an item no level has: %d", rec.Code)
	}

	// The comp sec sees it too; removing the level takes it off.
	members := do(t, h, http.MethodGet, club.page+"/members", nil).Body.String()
	skillsLink := regexp.MustCompile(`href="(` + regexp.QuoteMeta(club.page) + `/members/[^"]+/skills)"`).FindStringSubmatch(members)[1]
	if !strings.Contains(levelIn(do(t, h, http.MethodGet, skillsLink, nil).Body.String(), "Grade 1"), "Coach 1 of 3") {
		t.Error("the comp sec sees the level")
	}
	remove := regexp.MustCompile(`action="(` + regexp.QuoteMeta(ann) + `/levels/[^/"]+/remove)"`).FindStringSubmatch(do(t, h, http.MethodGet, ann+"/levels", nil).Body.String())[1]
	redirected(t, h, remove, nil)
	if strings.Contains(do(t, h, http.MethodGet, member+"/skills", nil).Body.String(), "Grade 1") {
		t.Error("removed")
	}
	if rec := do(t, h, http.MethodPost, ann+"/levels", url.Values{"set": {"nope"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("requirements that can't be read: %d", rec.Code)
	}
}

// A member who coaches too saves a club level by their member link (ADR
// 0010): their member page marks the link so the Requirements page offers it,
// and the server finds their coach's role; someone who coaches in two clubs
// is asked which, and a member who doesn't coach is told so.
func TestSavingLevelsAsAMemberWhoCoaches(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	ucd := aClub(t, h, admin, "UCD", false, "A. Murphy", "B. Kelly")
	murphy, kelly := ucd.members["A. Murphy"], ucd.members["B. Kelly"]
	id := regexp.MustCompile(`/members/([^/"]+)/new-link"`).FindStringSubmatch(do(t, h, http.MethodGet, ucd.page+"/members", nil).Body.String())[1]
	redirected(t, h, ucd.page+"/coaches/from-member", url.Values{"member": {id}})
	set := `{"format":1,"name":"Grade 1","rules":[{"type":"count","label":"A back somersault","match":{"rotation":{"min":4,"max":4},"direction":"backward"},"min":1}]}`

	// Only the member who coaches has their link marked for coaches' tools.
	if page := do(t, h, http.MethodGet, murphy, nil).Body.String(); !strings.Contains(page, "data-member-coach") {
		t.Error("A. Murphy coaches too: their link is marked")
	}
	if page := do(t, h, http.MethodGet, kelly, nil).Body.String(); strings.Contains(page, "data-member-coach") {
		t.Error("B. Kelly doesn't coach")
	}

	// Saving by the member link opens their coach's Levels, through it.
	location := redirected(t, h, "/clubs/levels/save", url.Values{"link": {"http://example.com" + murphy}, "set": {set}, "id": {"b1"}})
	coachVia := strings.Replace(murphy, "/clubs/member/", "/clubs/coach/", 1) + "~"
	if !strings.HasPrefix(location, coachVia) || !strings.HasSuffix(withoutQuery(location), "/levels") || !strings.HasPrefix(noticeOf(t, location), "Added Grade 1") {
		t.Errorf("saved through the member link: %s", location)
	}
	if !strings.Contains(do(t, h, http.MethodGet, withoutQuery(location), nil).Body.String(), "Grade 1") {
		t.Error("the level is the club's")
	}
	if rec := do(t, h, http.MethodPost, "/clubs/levels/save", url.Values{"link": {kelly}, "set": {set}}); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "not a coach here") {
		t.Errorf("a member who doesn't coach: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, "/clubs/levels/save", url.Values{"link": {ucd.page}, "set": {set}}); rec.Code != http.StatusBadRequest {
		t.Errorf("a club's admin link isn't anyone's own: %d", rec.Code)
	}

	// Coaching in a second club too, they're asked which.
	dcu := aClub(t, h, admin, "DCU", false)
	bob := pathIn(t, do(t, h, http.MethodPost, dcu.page+"/coaches", url.Values{"name": {"Bob"}}).Body.String(), "/clubs/coach/")
	redirected(t, h, murphy+"/links", url.Values{"link": {"http://example.com" + bob}})
	page := do(t, h, http.MethodPost, "/clubs/levels/save", url.Values{"link": {murphy}, "set": {set}, "id": {"b2"}}).Body.String()
	choice := regexp.MustCompile(`(?s)<input type="hidden" name="role" value="([^"]+)">\s*<button class="button is-fullwidth" type="submit">DCU</button>`).FindStringSubmatch(page)
	if !strings.Contains(page, "Which club?") || choice == nil {
		t.Fatalf("which club: %s", page)
	}
	location = redirected(t, h, "/clubs/levels/save", url.Values{"link": {murphy}, "set": {set}, "id": {"b2"}, "role": {choice[1]}})
	if !strings.Contains(do(t, h, http.MethodGet, bob+"/levels", nil).Body.String(), "Grade 1") {
		t.Errorf("saved to DCU's levels: %s", location)
	}
}
