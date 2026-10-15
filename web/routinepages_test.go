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
	_, _, member := aClubEntered(t, h)

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
	_, _, member := aClubEntered(t, h)
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
	_, club, member := aClubEntered(t, h)
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
	if !strings.Contains(page, "Straighter legs in the back") || !strings.Contains(page, "Note from Ann") {
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
	_, club, member := aClubEntered(t, h)
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
