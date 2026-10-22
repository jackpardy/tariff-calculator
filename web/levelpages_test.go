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

	nethtml "golang.org/x/net/html"

	"tariffCalculator/skills"

	"tariffCalculator/requirements"
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
	kaboom := regexp.MustCompile(`/club-skills/([^/"]+)/remove"`).FindStringSubmatch(do(t, h, http.MethodGet, ann+"/club-skills", nil).Body.String())[1]

	// The club's own skill is one of the level's rules, in its place.
	set := `{"format":1,"name":"Grade 1","rules":[
		{"type":"count","label":"A back somersault","match":{"rotation":{"min":4,"max":4},"direction":"backward"},"min":1},
		{"type":"track","item":"` + kaboom + `"},
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
	if a, k, b := strings.Index(page, "<li>A back somersault</li>"), strings.Index(page, "<li>Kaboom drill</li>"), strings.Index(page, "<li>Back drop, then back to feet</li>"); !(a >= 0 && a < k && k < b) {
		t.Errorf("the club's own skill in its place among the rules: %d %d %d", a, k, b)
	}
	m := regexp.MustCompile(`href="/requirements#share=j([A-Za-z0-9_-]+)"`).FindStringSubmatch(page)
	if m == nil {
		t.Fatal("no link to edit the requirements")
	}
	if data, _ := base64.RawURLEncoding.DecodeString(m[1]); !strings.Contains(string(data), `"name":"Grade 1"`) || !strings.Contains(string(data), `"v":1`) {
		t.Errorf("the requirements to edit: %s", data)
	}

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
	if got := levelIn(page, "Grade 1"); !(strings.Index(got, "A back somersault") < strings.Index(got, "Kaboom drill") && strings.Index(got, "Kaboom drill") < strings.Index(got, "Back drop, then back to feet")) {
		t.Errorf("the gymnast's Skills page lists them in order: %q", got)
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
	if got := levelIn(page, "Grade 1"); !strings.Contains(got, "Gymnast 1 of 3") || !strings.Contains(got, "A back somersault From the skills marked You Can do") || !strings.Contains(got, "Done in Drops") {
		t.Errorf("the derived item, and the routine doing the link: %q", got)
	}
	if notice := noticeOf(t, redirected(t, h, member+"/skills", url.Values{"item": {link[1]}, "state": {"learning"}})); notice != "Back drop, then back to feet: Learning." {
		t.Errorf("marking the link: %q", notice)
	}
	// Marking comes back to the skill's row, where it was marked.
	at := regexp.MustCompile(`id="(mark-level0-[0-9a-f]{8})"`).FindStringSubmatch(page)
	if at == nil {
		t.Fatalf("the level's rows have anchors: %s", levelIn(page, "Grade 1"))
	}
	if to := redirected(t, h, member+"/skills", url.Values{"club": {kaboom}, "state": {"can"}, "at": {at[1]}}); !strings.HasSuffix(to, "#"+at[1]) {
		t.Errorf("back to the row: %s", to)
	}
	if to := redirected(t, h, member+"/skills", url.Values{"club": {kaboom}, "state": {"can"}, "at": {"elsewhere"}}); strings.Contains(to, "#") {
		t.Errorf("only a row's anchor: %s", to)
	}

	// The coach marks the link Can do; both sides show, and the counts.
	gymnasts := do(t, h, http.MethodGet, ann+"/gymnasts", nil).Body.String()
	open := regexp.MustCompile(`href="(` + regexp.QuoteMeta(ann) + `/gymnasts/[^"]+)">A. Murphy</a>`).FindStringSubmatch(gymnasts)[1]
	redirected(t, h, open+"/skills", url.Values{"item": {link[1]}, "state": {"can"}})
	page = do(t, h, http.MethodGet, member+"/skills", nil).Body.String()
	got := levelIn(page, "Grade 1")
	if !strings.Contains(got, "Gymnast 2 of 3 Coach 1 of 3") || !strings.Contains(got, "Back drop, then back to feet Done in Drops You Learning") || !strings.Contains(got, "Coach · Ann Can do") {
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
	if page := do(t, h, http.MethodPost, remove, nil).Body.String(); !strings.Contains(page, "Remove Grade 1?") || !strings.Contains(page, `name="anyway" value="1"`) {
		t.Errorf("removing a level asks first: %s", page)
	}
	redirected(t, h, remove, url.Values{"anyway": {"1"}})
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
	redirected(t, h, murphy+"/links", url.Values{"link": {"http://example.com" + bob}, "sure": {"1"}})
	page := do(t, h, http.MethodPost, "/clubs/levels/save", url.Values{"link": {murphy}, "set": {set}, "id": {"b2"}}).Body.String()
	choice := regexp.MustCompile(`(?s)<input type="hidden" name="role" value="([^"]+)">\s*<button class="button" type="submit">DCU</button>`).FindStringSubmatch(page)
	if !strings.Contains(page, "Which club?") || choice == nil {
		t.Fatalf("which club: %s", page)
	}
	location = redirected(t, h, "/clubs/levels/save", url.Values{"link": {murphy}, "set": {set}, "id": {"b2"}, "role": {choice[1]}})
	if !strings.Contains(do(t, h, http.MethodGet, bob+"/levels", nil).Body.String(), "Grade 1") {
		t.Errorf("saved to DCU's levels: %s", location)
	}
}

// A club level is shared by link with a coach of another club, who adds a
// copy to theirs, with the club skills it tracks.
func TestSharingClubLevels(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	ucd := aClub(t, h, admin, "UCD", false, "A. Murphy")
	ann := pathIn(t, do(t, h, http.MethodPost, ucd.page+"/coaches", url.Values{"name": {"Ann"}}).Body.String(), "/clubs/coach/")
	redirected(t, h, ann+"/club-skills", url.Values{"name": {"Kaboom drill"}, "note": {"On the crash mat"}})
	kaboom := regexp.MustCompile(`/club-skills/([^/"]+)/remove"`).FindStringSubmatch(do(t, h, http.MethodGet, ann+"/club-skills", nil).Body.String())[1]
	redirected(t, h, ann+"/levels", url.Values{"set": {`{"format":1,"name":"Grade 1","rules":[{"type":"track","item":"` + kaboom + `"},{"type":"count","label":"A back somersault","match":{"rotation":{"min":4,"max":4},"direction":"backward"},"min":1}]}`}, "id": {"b1"}})

	// The link carries the level and its club skill, in its fragment: the
	// rule points at the link's list of club skills, not at UCD's.
	page := do(t, h, http.MethodGet, ann+"/levels", nil).Body.String()
	m := regexp.MustCompile(`data-copy="http://example.com/clubs/levels/shared#share=j([A-Za-z0-9_-]+)"`).FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("no share link: %s", page)
	}
	data, err := base64.RawURLEncoding.DecodeString(m[1])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"name":"Kaboom drill","note":"On the crash mat"`) || !strings.Contains(string(data), `{"type":"track","item":"shared:0"}`) || strings.Contains(string(data), kaboom) || strings.Contains(string(data), "A. Murphy") {
		t.Errorf("the link carries the level and its club skill, and no one's name: %s", data)
	}
	if rec := do(t, h, http.MethodGet, "/clubs/levels/shared", nil); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "data-shared-level") {
		t.Errorf("the page the link opens: %d", rec.Code)
	}

	// Shown, then added to another club by its coach's link.
	preview := do(t, h, http.MethodPost, "/clubs/levels/shared", url.Values{"level": {string(data)}}).Body.String()
	for _, want := range []string{"Grade 1", "A back somersault", "Kaboom drill", "Add to my club"} {
		if !strings.Contains(preview, want) {
			t.Errorf("the shared level should show %q", want)
		}
	}
	dcu := aClub(t, h, admin, "DCU", false)
	bob := pathIn(t, do(t, h, http.MethodPost, dcu.page+"/coaches", url.Values{"name": {"Bob"}}).Body.String(), "/clubs/coach/")
	location := redirected(t, h, "/clubs/levels/import", url.Values{"level": {string(data)}, "link": {"http://example.com" + bob}})
	if withoutQuery(location) != bob+"/levels" || noticeOf(t, location) != "Added Grade 1 to your club's levels. It brought 1 club skill to track, now on Club skills." {
		t.Errorf("adding it: %s", location)
	}
	// DCU's copy has Kaboom drill first, as UCD's does: DCU's own, new.
	if page = do(t, h, http.MethodGet, bob+"/levels", nil).Body.String(); !strings.Contains(page, "Grade 1") || !(strings.Contains(page, "<li>Kaboom drill</li>") && strings.Index(page, "<li>Kaboom drill</li>") < strings.Index(page, "<li>A back somersault</li>")) {
		t.Errorf("DCU's copy, tracking Kaboom drill first: %s", page)
	}
	if !strings.Contains(do(t, h, http.MethodGet, bob+"/club-skills", nil).Body.String(), "On the crash mat") {
		t.Error("the club skill came with it")
	}

	// A member who coaches too adds it by their member link.
	id := regexp.MustCompile(`/members/([^/"]+)/new-link"`).FindStringSubmatch(do(t, h, http.MethodGet, ucd.page+"/members", nil).Body.String())[1]
	if rec := do(t, h, http.MethodPost, "/clubs/levels/import", url.Values{"level": {string(data)}, "link": {ucd.members["A. Murphy"]}}); rec.Code != http.StatusConflict {
		t.Errorf("a member who doesn't coach: %d", rec.Code)
	}
	redirected(t, h, ucd.page+"/coaches/from-member", url.Values{"member": {id}})
	location = redirected(t, h, "/clubs/levels/import", url.Values{"level": {string(data)}, "link": {ucd.members["A. Murphy"]}})
	if !strings.HasPrefix(location, strings.Replace(ucd.members["A. Murphy"], "/clubs/member/", "/clubs/coach/", 1)+"~") || !strings.HasPrefix(noticeOf(t, location), "Added Grade 1") {
		t.Errorf("added by a member who coaches too: %s", location)
	}

	// A link that isn't anyone's own, and a level that can't be read.
	if rec := do(t, h, http.MethodPost, "/clubs/levels/import", url.Values{"level": {string(data)}, "link": {ucd.page}}); rec.Code != http.StatusBadRequest {
		t.Errorf("a club's admin link: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, "/clubs/levels/shared", url.Values{"level": {"nope"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("a level that can't be read: %d", rec.Code)
	}
}

// A coach writes a level on the club's page: a new one, with the club's own
// named skills to pick first, saved straight to the club; changed in place;
// copied as a new one. Only the kinds of rule a level tracks are offered.
func TestClubLevelEditor(t *testing.T) {
	h := competitionServer(t)
	club := aClub(t, h, created(t, h, newCompetition()), "UCD", false, "A. Murphy")
	ann := pathIn(t, do(t, h, http.MethodPost, club.page+"/coaches", url.Values{"name": {"Ann"}}).Body.String(), "/clubs/coach/")
	// The club names a back tuck "Back tuck (club)".
	back := skillJSON(t, "backSomersault")
	back["custom_name"] = "Back tuck (club)"
	raw, _ := json.Marshal(back)
	redirected(t, h, ann+"/club-skills", url.Values{"skill": {string(raw)}, "name": {"Back tuck (club)"}})

	levels := do(t, h, http.MethodGet, ann+"/levels", nil).Body.String()
	if !strings.Contains(levels, `href="`+ann+`/levels/new"`) {
		t.Fatalf("the Levels page offers a new level: %s", levels)
	}
	page := do(t, h, http.MethodGet, ann+"/levels/new", nil).Body.String()
	form := submitted(func() *nethtml.Node {
		doc, err := nethtml.Parse(strings.NewReader(page))
		if err != nil {
			t.Fatal(err)
		}
		return doc
	}())
	if !strings.Contains(page, `<optgroup label="Your club&#39;s skills">`) || !strings.Contains(page, `value="club:Back tuck (club)"`) {
		t.Errorf("the club's own named skills come first: %s", page)
	}
	for _, kind := range []string{`value="include"`, `value="count"`, `value="then"`} {
		if !strings.Contains(page, kind) {
			t.Errorf("a level offers %s", kind)
		}
	}
	if strings.Contains(page, `value="difficulty"`) || strings.Contains(page, `value="elements"`) {
		t.Error("rules about a single routine aren't offered")
	}

	// A new level starts empty; Add a skill adds the club's back tuck as a
	// line of its own. Named, and saved.
	if strings.Contains(page, `class="rule-card"`) || !strings.Contains(page, `name="addpick"`) {
		t.Errorf("a new level starts empty, with Add a skill: %s", page)
	}
	form.Set("name", "Grade 1")
	form.Set("addpick", "club:Back tuck (club)")
	edited := do(t, h, http.MethodPost, ann+"/levels/editor", form)
	if edited.Code != http.StatusOK || !strings.Contains(edited.Body.String(), `data-described="[&#34;Back tuck (club)&#34;]"`) {
		t.Fatalf("picking the club's skill: %d %s", edited.Code, edited.Body.String())
	}
	// Saved as the editor now shows it, as the browser does.
	form = submitted(func() *nethtml.Node {
		doc, _ := nethtml.Parse(strings.NewReader(edited.Body.String()))
		return doc
	}())
	location := redirected(t, h, ann+"/levels/new", form)
	if notice := noticeOf(t, location); notice != "Grade 1 saved." {
		t.Errorf("saved: %q", notice)
	}
	levels = do(t, h, http.MethodGet, ann+"/levels", nil).Body.String()
	edit := regexp.MustCompile(`href="(` + regexp.QuoteMeta(ann) + `/levels/[^/"]+/edit)"`).FindStringSubmatch(levels)
	if !strings.Contains(levels, "Back tuck (club)") || edit == nil {
		t.Fatalf("the level, with Change: %s", levels)
	}

	// Change it: renamed, and a second rule; saved in place.
	page = do(t, h, http.MethodGet, edit[1], nil).Body.String()
	form = submitted(func() *nethtml.Node {
		doc, _ := nethtml.Parse(strings.NewReader(page))
		return doc
	}())
	form.Set("name", "Grade 1A")
	redirected(t, h, edit[1], form)
	levels = do(t, h, http.MethodGet, ann+"/levels", nil).Body.String()
	if !strings.Contains(levels, "Grade 1A") || strings.Contains(levels, ">Grade 1<") || strings.Count(levels, "/edit\"") != 1 {
		t.Errorf("changed in place: %s", levels)
	}

	// Copy it as a new level.
	copyLink := regexp.MustCompile(`href="(` + regexp.QuoteMeta(ann) + `/levels/new\?from=[^"]+)"`).FindStringSubmatch(levels)
	if copyLink == nil {
		t.Fatal("no Copy as a new level")
	}
	page = do(t, h, http.MethodGet, unescape(copyLink[1]), nil).Body.String()
	if !strings.Contains(page, `value="Copy of Grade 1A"`) || !strings.Contains(page, "Back tuck (club)") {
		t.Errorf("a copy starts from the level: %s", page)
	}

	// Or start from a competition's set routine: the level's first item,
	// saved as it is.
	set := regexp.MustCompile(`href="(` + regexp.QuoteMeta(ann) + `/levels/new\?set=builtin%3Abg-club-l1)"`).FindStringSubmatch(levels)
	if set == nil || !strings.Contains(levels, ">Start from a set routine</summary>") {
		t.Fatalf("the Levels page offers the set routines to start from: %s", levels)
	}
	page = do(t, h, http.MethodGet, unescape(set[1]), nil).Body.String()
	if !strings.Contains(page, `value="BG Club L1"`) || !strings.Contains(page, `<option value="sequence" selected`) {
		t.Fatalf("the editor starts with the set routine: %s", page)
	}
	form = submitted(func() *nethtml.Node {
		doc, _ := nethtml.Parse(strings.NewReader(page))
		return doc
	}())
	if notice := noticeOf(t, redirected(t, h, ann+"/levels/new", form)); notice != "BG Club L1 saved." {
		t.Errorf("saved: %q", notice)
	}
	if levels = do(t, h, http.MethodGet, ann+"/levels", nil).Body.String(); !strings.Contains(levels, "<h3 class=\"title is-6 mb-1\">BG Club L1</h3>") {
		t.Error("the level is the club's")
	}

	// One of the club's own skills, as a rule in its place: added, moved
	// first, saved; a club with none isn't offered the kind.
	if strings.Contains(page, `value="track"`) {
		t.Error("no club skills of its own: no rule of one")
	}
	redirected(t, h, ann+"/club-skills", url.Values{"name": {"Kaboom drill"}})
	page = do(t, h, http.MethodGet, ann+"/levels/new", nil).Body.String()
	form = submitted(func() *nethtml.Node {
		doc, _ := nethtml.Parse(strings.NewReader(page))
		return doc
	}())
	form.Set("name", "Grade 2")
	form.Set("addpick", "club:Back tuck (club)")
	form = submitted(func() *nethtml.Node {
		doc, _ := nethtml.Parse(strings.NewReader(do(t, h, http.MethodPost, ann+"/levels/editor", form).Body.String()))
		return doc
	}())
	form.Set("add", "track")
	added := do(t, h, http.MethodPost, ann+"/levels/editor", form).Body.String()
	if !strings.Contains(added, `<option value="track" selected`) || !strings.Contains(added, ">Kaboom drill</option>") || !strings.Contains(added, "2. Kaboom drill") {
		t.Fatalf("a rule of one of the club's own skills, the first chosen: %s", added)
	}
	form = submitted(func() *nethtml.Node {
		doc, _ := nethtml.Parse(strings.NewReader(added))
		return doc
	}())
	form.Set("action", "up:1")
	moved := do(t, h, http.MethodPost, ann+"/levels/editor", form).Body.String()
	form = submitted(func() *nethtml.Node {
		doc, _ := nethtml.Parse(strings.NewReader(moved))
		return doc
	}())
	if notice := noticeOf(t, redirected(t, h, ann+"/levels/new", form)); notice != "Grade 2 saved." {
		t.Fatalf("saved: %q", notice)
	}
	levels = do(t, h, http.MethodGet, ann+"/levels", nil).Body.String()
	grade2 := levels[strings.Index(levels, ">Grade 2</h3>"):]
	if k, b := strings.Index(grade2, "<li>Kaboom drill</li>"), strings.Index(grade2, "Back tuck (club)"); k < 0 || b < k {
		t.Errorf("Kaboom drill first, then the back tuck: %s", grade2)
	}

	// A level of independent skills: each picked from Add a skill is a line
	// of its own, a trampoline skill or one of the club's own, in any mix.
	page = do(t, h, http.MethodGet, ann+"/levels/new", nil).Body.String()
	for _, pick := range []string{"skill:Tuck Jump", "skill:Seat Drop", "own:" + regexp.MustCompile(`value="own:([^"]+)"`).FindStringSubmatch(page)[1], "kind:single"} {
		form = submitted(func() *nethtml.Node {
			doc, _ := nethtml.Parse(strings.NewReader(page))
			return doc
		}())
		form.Set("addpick", pick)
		page = do(t, h, http.MethodPost, ann+"/levels/editor", form).Body.String()
	}
	for i, want := range []string{"1. Tuck Jump", "2. Seat Drop", "3. Kaboom drill", "4. Any single somersault (¾ to 1¼)"} {
		if !strings.Contains(page, want) {
			t.Errorf("line %d should be %q: %s", i+1, want, page)
		}
	}
}

// A level's line naming one skill can be marked from the line; a kind of
// skill is worked out from the skills marked.
func TestLevelLineOneSkill(t *testing.T) {
	one := func(id string) bool {
		p, ok := requirements.PickByID(id)
		if !ok {
			t.Fatalf("no pick %s", id)
		}
		m := p.Match.Clone()
		_, ok = oneSkill(requirements.Rule{Type: requirements.Count, Match: &m, Min: ptrFloat(1)})
		return ok
	}
	if !one("skill:Barani") || one("kind:back") {
		t.Error("a skill is marked from its line, a kind of skill isn't")
	}
}

func ptrFloat(v float64) *float64 { return &v }

// A coach's own set routines, kept on their page, are offered to start a
// level from, as competitions' are.
func TestLevelFromKeptSetRoutine(t *testing.T) {
	h := competitionServer(t)
	club := aClub(t, h, created(t, h, newCompetition()), "UCD", false, "A. Murphy")
	ann := pathIn(t, do(t, h, http.MethodPost, club.page+"/coaches", url.Values{"name": {"Ann"}}).Body.String(), "/clubs/coach/")
	set := `{"format":1,"name":"Club L1 set","rules":[{"type":"sequence","sequence":[{"label":"Tuck Jump","rotation":{"min":0,"max":0},"shapes":["tuck"]}]}]}`
	if rec := do(t, h, http.MethodPost, ann+"/requirements", url.Values{"items": {`[{"id":"set-l1","kind":"set","data":` + set + `}]`}}); rec.Code != http.StatusOK {
		t.Fatalf("keeping: %d %s", rec.Code, rec.Body)
	}
	levels := do(t, h, http.MethodGet, ann+"/levels", nil).Body.String()
	if !strings.Contains(levels, "Yours, kept on your page") || !strings.Contains(levels, `href="`+ann+`/levels/new?kept=set-l1"`) {
		t.Fatalf("the coach's own set routine to start from: %s", levels)
	}
	if page := do(t, h, http.MethodGet, ann+"/levels/new?kept=set-l1", nil).Body.String(); !strings.Contains(page, `value="Club L1 set"`) || !strings.Contains(page, "sequence") {
		t.Errorf("the level starts from it: %s", page)
	}
}
