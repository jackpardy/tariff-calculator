package web

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"tariffCalculator/store"
)

// keep is the routines field the browser posts to keep routines (RoutineStore.keepable).
func keep(t *testing.T, routines ...map[string]any) url.Values {
	t.Helper()
	data, err := json.Marshal(routines)
	if err != nil {
		t.Fatal(err)
	}
	return url.Values{"routines": {string(data)}}
}

// aRoutine is a routine as the browser keeps it, from voluntary.
func aRoutine(id, name string) map[string]any {
	var skills []any
	if err := json.Unmarshal([]byte(voluntary), &skills); err != nil {
		panic(err)
	}
	return map[string]any{"id": id, "name": name, "skills": skills, "requirements": "builtin:bucs-l3-second", "checks": map[string]any{"difficulty": true}}
}

// A member keeps routines on their page, renames and deletes them; each shows
// checked, and opens in the routine builder.
func TestMemberKeepsRoutines(t *testing.T) {
	h := competitionServer(t)
	member := aClub(t, h, created(t, h, newCompetition()), "UCD", false, "A. Murphy").members["A. Murphy"]

	page := do(t, h, http.MethodGet, member+"/routines", nil).Body.String()
	if got := currentTab(t, page); got != "Routines" {
		t.Errorf("the Routines tab is %q", got)
	}
	if !strings.Contains(page, "You haven&#39;t kept any routines here yet.") || !strings.Contains(page, `action="`+member+`/routines"`) {
		t.Errorf("an empty Routines tab, with adding from this browser: %s", page)
	}

	// Keeping two (one empty, one that can't be read): one is kept.
	location := redirected(t, h, member+"/routines", keep(t, aRoutine("b1", "Comp vol"), map[string]any{"id": "b2", "name": "Empty", "skills": []any{}},
		map[string]any{"id": "b3", "name": "Bad", "skills": []any{map[string]any{"rotation": 99}}}))
	if notice := noticeOf(t, location); notice != "Kept 1 routine on your page. 1 routine couldn't be read." {
		t.Errorf("notice: %q", notice)
	}
	page = do(t, h, http.MethodGet, member+"/routines", nil).Body.String()
	for _, want := range []string{"Comp vol", "2 skills", "BUCS L3 · second exercise", "Open in the Routine Builder", "Delete from your page"} {
		if !strings.Contains(page, want) {
			t.Errorf("the kept routine should show %q", want)
		}
	}

	// Its builder link is a share link with the routine and its requirements.
	m := regexp.MustCompile(`href="/#share=j([A-Za-z0-9_-]+)"`).FindStringSubmatch(page)
	if m == nil {
		t.Fatal("no builder link")
	}
	data, err := base64.RawURLEncoding.DecodeString(m[1])
	if err != nil {
		t.Fatal(err)
	}
	var share struct {
		V        int `json:"v"`
		Routines []struct {
			Name         string           `json:"name"`
			Skills       []map[string]any `json:"skills"`
			Requirements string           `json:"requirements"`
		} `json:"routines"`
	}
	if err := json.Unmarshal(data, &share); err != nil || share.V != 1 || len(share.Routines) != 1 {
		t.Fatalf("share = %s (%v)", data, err)
	}
	if r := share.Routines[0]; r.Name != "Comp vol" || len(r.Skills) != 2 || r.Requirements != "builtin:bucs-l3-second" || r.Skills[0]["tariff"] != nil {
		t.Errorf("shared routine = %+v", r)
	}

	// Keeping it again from the same browser changes it rather than adding one.
	redirected(t, h, member+"/routines", keep(t, aRoutine("b1", "Comp vol 2")))
	page = do(t, h, http.MethodGet, member+"/routines", nil).Body.String()
	if strings.Count(page, `class="box comp-routine-row"`) != 1 || !strings.Contains(page, "Comp vol 2") {
		t.Error("keeping it again should change it")
	}

	rename := formAction(t, page, member+"/routines/")
	rename = strings.TrimSuffix(rename, "/rename")
	if notice := noticeOf(t, redirected(t, h, rename+"/rename", url.Values{"name": {"Regionals"}})); notice != "Renamed." {
		t.Errorf("rename: %q", notice)
	}
	if notice := noticeOf(t, redirected(t, h, rename+"/rename", url.Values{"name": {" "}})); notice != "A routine needs a name." {
		t.Errorf("rename to nothing: %q", notice)
	}
	if !strings.Contains(do(t, h, http.MethodGet, member+"/routines", nil).Body.String(), "Regionals") {
		t.Error("renamed")
	}
	redirected(t, h, rename+"/delete", nil)
	if strings.Contains(do(t, h, http.MethodGet, member+"/routines", nil).Body.String(), "Regionals") {
		t.Error("deleted")
	}
	if rec := do(t, h, http.MethodPost, rename+"/delete", nil); rec.Code != http.StatusNotFound {
		t.Errorf("deleting it again: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, member+"/routines", url.Values{"routines": {"nope"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("unreadable routines: %d", rec.Code)
	}
}

// A browser's worth of routines fits, more than the usual request; past the
// most a member keeps, the rest are left out and the member told.
func TestKeepingManyRoutines(t *testing.T) {
	h := competitionServer(t)
	member := aClub(t, h, created(t, h, newCompetition()), "UCD", false, "A. Murphy").members["A. Murphy"]
	var many []map[string]any
	for i := range store.MaxRoutines + 2 {
		r := aRoutine(fmt.Sprint("b", i), fmt.Sprint("Routine ", i))
		r["requirements"] = strings.Repeat(" ", 2000) + "builtin:bucs-l3-second" // a big own set, say
		many = append(many, r)
	}
	form := keep(t, many...)
	if len(form.Encode()) <= maxRequestBytes {
		t.Fatalf("the test should post more than the usual limit: %d", len(form.Encode()))
	}
	notice := noticeOf(t, redirected(t, h, member+"/routines", form))
	if want := fmt.Sprintf("Kept %d routines on your page. 2 routines didn't fit: there's room for %d, so delete one to keep another.", store.MaxRoutines, store.MaxRoutines); notice != want {
		t.Errorf("notice: %q", notice)
	}
}

// A coach sees the routines of the gymnasts they see, and notes them; the
// member sees the note, and the comp sec the routines.
func TestCoachSeesRoutines(t *testing.T) {
	h := competitionServer(t)
	c := aClub(t, h, created(t, h, newCompetition()), "UCD", false, "A. Murphy")
	club, member := c.page, c.members["A. Murphy"]
	ann := pathIn(t, do(t, h, http.MethodPost, club+"/coaches", url.Values{"name": {"Ann"}}).Body.String(), "/clubs/coach/")
	bob := pathIn(t, do(t, h, http.MethodPost, club+"/coaches", url.Values{"name": {"Bob"}}).Body.String(), "/clubs/coach/")
	redirected(t, h, member+"/routines", keep(t, aRoutine("b1", "Comp vol")))

	// The coach's page has Entries and Gymnasts.
	if got := currentTab(t, do(t, h, http.MethodGet, ann, nil).Body.String()); got != "Entries" {
		t.Errorf("a coach's default section is %q", got)
	}
	page := do(t, h, http.MethodGet, ann+"/gymnasts", nil).Body.String()
	if got := currentTab(t, page); got != "Gymnasts" {
		t.Errorf("Gymnasts is %q", got)
	}
	open := regexp.MustCompile(`href="(` + regexp.QuoteMeta(ann) + `/gymnasts/[^"]+)">A. Murphy</a>`).FindStringSubmatch(page)
	if open == nil || !strings.Contains(page, "1 routine") {
		t.Fatalf("the coach should see A. Murphy with a routine: %s", page)
	}
	page = do(t, h, http.MethodGet, open[1], nil).Body.String()
	if !strings.Contains(page, "Comp vol") || !strings.Contains(page, "Save note") {
		t.Fatalf("the gymnast's page should have the routine and a note: %s", page)
	}
	note := formAction(t, page, open[1]+"/routines/")
	if location := redirected(t, h, note, url.Values{"note": {"Straighter legs in the back"}}); noticeOf(t, location) != "Note saved." {
		t.Errorf("noting: %s", location)
	}
	page = do(t, h, http.MethodGet, member+"/routines", nil).Body.String()
	if !strings.Contains(page, "Straighter legs in the back") || !strings.Contains(page, "left a note") || !strings.Contains(page, `<span class="chip-role">Coach</span> <span class="chip-name">· Ann</span>`) {
		t.Error("the member sees the coach's note")
	}

	// Once A. Murphy chooses Ann, Bob doesn't see them.
	details := do(t, h, http.MethodGet, member+"/details", nil).Body.String()
	annID := regexp.MustCompile(`<option value="([^"]+)">Ann</option>`).FindStringSubmatch(details)[1]
	redirected(t, h, member+"/coach", url.Values{"coach": {annID}})
	gymnast := strings.TrimPrefix(open[1], ann)
	if rec := do(t, h, http.MethodGet, bob+gymnast, nil); rec.Code != http.StatusNotFound {
		t.Errorf("another coach opens the gymnast: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, bob+strings.TrimPrefix(note, ann), url.Values{"note": {"x"}}); rec.Code != http.StatusNotFound {
		t.Errorf("another coach notes the routine: %d", rec.Code)
	}
	if strings.Contains(do(t, h, http.MethodGet, bob+"/gymnasts", nil).Body.String(), "A. Murphy") {
		t.Error("another coach lists the gymnast")
	}

	// The comp sec sees how many, and the routines.
	members := do(t, h, http.MethodGet, club+"/members", nil).Body.String()
	link := regexp.MustCompile(`href="(` + regexp.QuoteMeta(club) + `/members/[^"]+/routines)">1 routine</a>`).FindStringSubmatch(members)
	if link == nil {
		t.Fatalf("Members should link to the routines: %s", members)
	}
	page = do(t, h, http.MethodGet, link[1], nil).Body.String()
	if !strings.Contains(page, "Comp vol") || strings.Contains(page, "Delete from your page") || strings.Contains(page, "Save note") {
		t.Errorf("the comp sec sees the routine, to look at: %s", page)
	}
}

// Entering a competition offers the routines kept on the member's page, to
// the member and to the comp sec changing their entry.
func TestEntryOffersKeptRoutines(t *testing.T) {
	h := competitionServer(t)
	c := aClub(t, h, created(t, h, newCompetition()), "UCD", false, "A. Murphy")
	club, member := c.page, c.members["A. Murphy"]
	page := memberCompetitionPage(t, h, member)
	if strings.Contains(page, "Kept on your page") {
		t.Error("nothing kept yet")
	}
	redirected(t, h, member+"/routines", keep(t, aRoutine("b1", "Comp vol")))
	page = memberCompetitionPage(t, h, member)
	if !strings.Contains(page, `<optgroup label="Kept on your page" data-kept>`) || !strings.Contains(page, "Comp vol (2 skills)") {
		t.Errorf("the member's entry form should offer the kept routine: %s", page)
	}

	// The comp sec changing the member's entry gets them too.
	members := do(t, h, http.MethodGet, club+"/members", nil).Body.String()
	id := regexp.MustCompile(`/members/([^/"]+)/routines"`).FindStringSubmatch(members)[1]
	competition := regexp.MustCompile(`/competitions/([A-Za-z0-9_-]+)"`).FindStringSubmatch(do(t, h, http.MethodGet, member, nil).Body.String())[1]
	edit := do(t, h, http.MethodGet, club+"/members/"+id+"/competitions/"+competition+"?discipline=", nil)
	if edit.Code != http.StatusOK || !strings.Contains(edit.Body.String(), `<optgroup label="Kept on their page" data-kept>`) {
		t.Errorf("the comp sec's form should offer the member's kept routine: %d %s", edit.Code, edit.Body.String())
	}

	// Choosing it posts its skills, as choosing a browser's routine does.
	kept := regexp.MustCompile(`<option value="kept:0" data-skills="([^"]+)"`).FindStringSubmatch(page)
	if kept == nil {
		t.Fatal("no kept option")
	}
	var skills []map[string]any
	if err := json.Unmarshal([]byte(strings.ReplaceAll(kept[1], "&#34;", `"`)), &skills); err != nil || len(skills) != 2 {
		t.Errorf("the option's skills: %q (%v)", kept[1], err)
	}
}

// A coach, on a gymnast's page, sees which skills in each routine no coach
// has marked Can do, checks a routine against a level they choose, and adds
// routines of their own to the gymnast's page; the comp sec's Members and
// the coach's Gymnasts filter by where each stands on the club's levels.
func TestCoachRoutineTools(t *testing.T) {
	h := competitionServer(t)
	c := aClub(t, h, created(t, h, newCompetition()), "UCD", false, "A. Murphy", "B. Kelly")
	club, member, kelly := c.page, c.members["A. Murphy"], c.members["B. Kelly"]
	ann := pathIn(t, do(t, h, http.MethodPost, club+"/coaches", url.Values{"name": {"Ann"}}).Body.String(), "/clubs/coach/")
	bob := pathIn(t, do(t, h, http.MethodPost, club+"/coaches", url.Values{"name": {"Bob"}}).Body.String(), "/clubs/coach/")
	redirected(t, h, member+"/routines", keep(t, aRoutine("b1", "Comp vol")))
	redirected(t, h, ann+"/levels", url.Values{"set": {`{"format":1,"name":"Grade 1","rules":[{"type":"count","label":"A back somersault","match":{"rotation":{"min":4,"max":4},"direction":"backward"},"min":1}]}`}, "id": {"g1"}})
	gymnasts := do(t, h, http.MethodGet, ann+"/gymnasts", nil).Body.String()
	open := regexp.MustCompile(`href="(` + regexp.QuoteMeta(ann) + `/gymnasts/[^"]+)">A. Murphy</a>`).FindStringSubmatch(gymnasts)[1]

	// Neither skill is marked Can do by a coach; the gymnast's own mark
	// doesn't count, a coach's does.
	backTuck := `{"rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck","backward":true}`
	barani := `{"rotation":4,"twist_distribution":[1],"takeoff_position":"Feet","shape":"Tuck"}`
	if page := do(t, h, http.MethodGet, open, nil).Body.String(); !strings.Contains(page, "2 skills not marked Can do") || !strings.Contains(page, "Mark them on Skills") {
		t.Errorf("both skills to mark: %s", page)
	}
	redirected(t, h, member+"/skills", url.Values{"skill": {barani}, "state": {"can"}})
	redirected(t, h, open+"/skills", url.Values{"skill": {backTuck}, "state": {"can"}})
	page := do(t, h, http.MethodGet, open, nil).Body.String()
	marked := regexp.MustCompile(`No coach has marked these Can do: ([^.]*)\.`).FindStringSubmatch(page)
	if !strings.Contains(page, "1 skill not marked Can do") || marked == nil || !strings.Contains(marked[1], "Barani") {
		t.Errorf("only the barani is left to mark: %v", marked)
	}
	if strings.Contains(do(t, h, http.MethodGet, member+"/routines", nil).Body.String(), "not marked Can do") {
		t.Error("the gymnast isn't told: it's for the coach")
	}

	// Checked against the club's level, it opens checked against it.
	level := regexp.MustCompile(`<option value="(club:[^"]+)">Grade 1</option>`).FindStringSubmatch(page)
	routine := regexp.MustCompile(`name="routine" value="([^"]+)"`).FindStringSubmatch(page)
	if level == nil || routine == nil || !strings.Contains(page, `<optgroup label="Your club&#39;s levels">`) {
		t.Fatalf("the levels to check against: %s", page)
	}
	page = do(t, h, http.MethodGet, open+"?"+url.Values{"routine": {routine[1]}, "against": {level[1]}}.Encode(), nil).Body.String()
	if !strings.Contains(page, `open>`) || !strings.Contains(page, `<option value="`+level[1]+`" selected>`) || !strings.Contains(page, "Grade 1") {
		t.Errorf("checked against Grade 1: %s", page)
	}
	if page = do(t, h, http.MethodGet, open+"?routine="+routine[1]+"&against=builtin:nope", nil).Body.String(); strings.Contains(page, `open>`) {
		t.Error("a level that isn't there is ignored")
	}

	// The coach adds a routine; the member's own with the same browser id is
	// theirs, and adding it again changes only the coach's.
	location := redirected(t, h, open+"/routines", keep(t, aRoutine("c1", "Drill vol")))
	if noticeOf(t, location) != "Kept 1 routine on A. Murphy's page." {
		t.Errorf("adding: %s", location)
	}
	mine := do(t, h, http.MethodGet, member+"/routines", nil).Body.String()
	if !strings.Contains(mine, "Drill vol") || !strings.Contains(mine, "added it") || !strings.Contains(mine, "Delete from your page") {
		t.Errorf("the member sees the coach's routine, theirs to keep or delete: %s", mine)
	}
	redirected(t, h, member+"/routines", keep(t, aRoutine("c1", "My vol")))
	redirected(t, h, open+"/routines", keep(t, aRoutine("c1", "Drill vol 2")))
	mine = do(t, h, http.MethodGet, member+"/routines", nil).Body.String()
	if !strings.Contains(mine, "My vol") || !strings.Contains(mine, "Drill vol 2") || strings.Contains(mine, "Drill vol<") || strings.Count(mine, `class="box comp-routine-row"`) != 3 {
		t.Errorf("the member's routine and the coach's kept apart: %s", mine)
	}

	// Members and Gymnasts filter by level: A. Murphy's coach marked the back
	// tuck, so Grade 1 is completed; B. Kelly is working on it.
	redirected(t, h, kelly+"/skills", url.Values{"skill": {backTuck}, "state": {"learning"}})
	levelID := strings.TrimPrefix(level[1], "club:")
	for _, list := range []struct{ page, id string }{{club + "/members", "members"}, {ann + "/gymnasts", "gymnasts"}} {
		page := do(t, h, http.MethodGet, list.page, nil).Body.String()
		rows := listRows(t, page, list.id)
		if got := rows["A. Murphy"]["level"]; len(got) != 1 || got[0] != levelID+":done" {
			t.Errorf("%s: A. Murphy's level: %v", list.id, got)
		}
		if got := rows["B. Kelly"]["level"]; len(got) != 1 || got[0] != levelID+":working" {
			t.Errorf("%s: B. Kelly's level: %v", list.id, got)
		}
		if !strings.Contains(page, `<optgroup label="Grade 1">`) || !strings.Contains(page, "Grade 1: completed") {
			t.Errorf("%s: the Level filter", list.id)
		}
	}

	// A coach who doesn't see the gymnast can't add to their page.
	details := do(t, h, http.MethodGet, member+"/details", nil).Body.String()
	annID := regexp.MustCompile(`<option value="([^"]+)">Ann</option>`).FindStringSubmatch(details)[1]
	redirected(t, h, member+"/coach", url.Values{"coach": {annID}})
	if rec := do(t, h, http.MethodPost, bob+strings.TrimPrefix(open, ann)+"/routines", keep(t, aRoutine("d1", "Bob's"))); rec.Code != http.StatusNotFound {
		t.Errorf("another coach adding: %d", rec.Code)
	}
}
