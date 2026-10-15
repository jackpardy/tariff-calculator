package web

import (
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"

	"tariffCalculator/store"
	"tariffCalculator/views"
)

// listsSetup is a competition that needs a coach's sign-off and approves
// coaches, with a club (UCD) of two members who have entered: Mo, whose
// coach is Ann, and Zoë D'Arcy, who has no coach and is a coach too. Ann
// (approved, with a qualification), Bob (doesn't sign off, no qualification)
// and Zoë are the club's coaches; Ann and Zoë are sent for approval.
type listsSetup struct {
	h                    http.Handler
	admin, club, annPage string
	annID, bobID, zoeID  string
}

const (
	annName = "Ann O'Neill / Jr?"
	zoeName = "Zoë D'Arcy"
)

func listsCompetition(t *testing.T) listsSetup {
	t.Helper()
	h := competitionServer(t)
	form := newCompetition()
	form.Set("signoff", "1")
	admin := created(t, h, form)
	redirected(t, h, admin+"/approve-coaches", url.Values{"on": {"1"}, "level-trampoline": {"3"}})
	clubLink := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/club/")
	club := withoutQuery(redirected(t, h, "/clubs", url.Values{"name": {"UCD"}}))
	redirected(t, h, clubLink, url.Values{"clubAdmin": {club}})
	annPage := pathIn(t, do(t, h, http.MethodPost, club+"/coaches", url.Values{"name": {annName}}).Body.String(), "/clubs/coach/")
	do(t, h, http.MethodPost, club+"/coaches", url.Values{"name": {"Bob"}})

	join := pathIn(t, do(t, h, http.MethodGet, club+"/members", nil).Body.String(), "/clubs/join/")
	entry := bucsEntry("")
	members := map[string]string{}
	for _, name := range []string{"Mo", zoeName} {
		m := withoutQuery(redirected(t, h, join, url.Values{"name": {name}}))
		redirected(t, h, formAction(t, memberCompetitionPage(t, h, m), m+"/competitions/"), entry)
		members[name] = m
	}
	zoeMember := regexp.MustCompile(`(?s)` + regexp.QuoteMeta(strings.ReplaceAll(zoeName, "'", "&#39;")) + `.*?name="member" value="([^"]+)"`).FindStringSubmatch(do(t, h, http.MethodGet, club+"/members", nil).Body.String())[1]
	redirected(t, h, club+"/coaches/from-member", url.Values{"member": {zoeMember}})

	ids := regexp.MustCompile(regexp.QuoteMeta(club)+`/coaches/([^/"]+)/signs-off`).FindAllStringSubmatch(do(t, h, http.MethodGet, club+"/coaches", nil).Body.String(), -1)
	s := listsSetup{h: h, admin: admin, club: club, annPage: annPage, annID: ids[0][1], bobID: ids[1][1], zoeID: ids[2][1]}
	redirected(t, h, club+"/coaches/"+s.bobID+"/signs-off", url.Values{"on": {""}})
	upload(t, h, club+"/coaches/"+s.annID+"/qualifications", map[string]string{"qualification": "bg-trampoline-3"}, "certificate", []byte("%PDF-1.4 Ann"))
	redirected(t, h, members["Mo"]+"/coach", url.Values{"coach": {s.annID}})
	redirected(t, h, formAction(t, clubEntriesPage(t, h, club), club+"/competitions/"), url.Values{"which": {"all"}})
	comp := regexp.MustCompile(regexp.QuoteMeta(club) + `/competitions/([^/"]+)/coaches`).FindStringSubmatch(clubCompetitionTab(t, h, club, "/coaches"))[1]
	redirected(t, h, club+"/competitions/"+comp+"/coaches", url.Values{"coach": {s.annID, s.zoeID}})
	redirected(t, h, admin+"/coaches/"+s.annID, url.Values{"action": {"approve"}})
	return s
}

// listTools says whether a page has the tools of the list with an id, with
// these filters (by key) and no others.
func hasListTools(page, id, noun string, keys ...string) bool {
	if !strings.Contains(page, `data-list-tools="`+id+`" data-noun="`+noun+`"`) {
		return false
	}
	got := regexp.MustCompile(`data-list-filter="([^"]+)"`).FindAllStringSubmatch(page, -1)
	var have []string
	for _, m := range got {
		have = append(have, m[1])
	}
	return slices.Equal(have, keys)
}

func TestCoachApprovalsListFilters(t *testing.T) {
	s := listsCompetition(t)
	page := do(t, s.h, http.MethodGet, s.admin+"/coaches", nil).Body.String()
	if !hasListTools(page, "coaches", "coaches", "club", "status") {
		t.Errorf("the tools, with club and status: %s", page)
	}
	for _, want := range []string{`<option value="UCD">UCD</option>`, `<option value="approved">Approved</option><option value="waiting">Waiting</option><option value="refused">Not approved</option><option value="withdrawn">Approval withdrawn</option>`,
		`<option value="">Any status</option>`, `<option value="">Every club</option>`} {
		if !strings.Contains(page, want) {
			t.Errorf("the tools have %s", want)
		}
	}
	if !regexp.MustCompile(`<section class="comp-list mt-5"><h2 class="title is-5 mb-3">Coaches <span[^>]*>· 2</span></h2>`).MatchString(page) {
		t.Error("the list is under its heading and count")
	}
	if n := strings.Count(page, "data-list-group"); n != 1 {
		t.Errorf("one box for UCD is the one group, not %d", n)
	}
	rows := listRows(t, page, "coaches")
	if len(rows) != 2 || !slices.Equal(rows[annName]["status"], []string{"approved"}) || !slices.Equal(rows[annName]["club"], []string{"UCD"}) ||
		!slices.Equal(rows[zoeName]["status"], []string{"waiting"}) {
		t.Errorf("the coaches' values: %v", rows)
	}
	redirected(t, s.h, s.admin+"/coaches/"+s.annID, url.Values{"action": {"withdraw"}})
	rows = listRows(t, do(t, s.h, http.MethodGet, s.admin+"/coaches", nil).Body.String(), "coaches")
	if !slices.Equal(rows[annName]["status"], []string{"withdrawn"}) {
		t.Errorf("withdrawn: %v", rows[annName])
	}

	// Nothing sent: no tools.
	h := competitionServer(t)
	form := newCompetition()
	form.Set("signoff", "1")
	empty := do(t, h, http.MethodGet, created(t, h, form)+"/coaches", nil).Body.String()
	if strings.Contains(empty, "data-list-tools") || !strings.Contains(empty, "No club has sent its coaches yet.") {
		t.Error("no search on an empty list")
	}
}

func TestDeskPeopleListFilters(t *testing.T) {
	s := listsCompetition(t)
	anIndividual(t, s.h, s.admin, "Ivy")
	page := do(t, s.h, http.MethodGet, s.admin+"/desk", nil).Body.String()
	if !hasListTools(page, "desk-people", "people", "club") {
		t.Errorf("the tools, with club only: %s", page)
	}
	if strings.Contains(page, "x-data") || strings.Contains(page, "x-model") || strings.Contains(page, "x-show") {
		t.Error("no Alpine on the desk")
	}
	if !strings.Contains(page, `type="button" data-list-clear`) {
		t.Error("Clear doesn't submit the form")
	}
	if !strings.Contains(page, `id="desk-people" style="max-height: 14rem; overflow-y: auto"`) {
		t.Error("the list keeps its scrolling box")
	}
	if strings.Contains(page, `<summary class="title is-5 mb-3">People`) {
		t.Error("the desk's list is not a section of its own")
	}
	rows := listRows(t, page, "desk-people")
	if !slices.Equal(rows["Mo"]["club"], []string{"UCD"}) || !slices.Equal(rows[zoeName]["club"], []string{"UCD"}) || !slices.Equal(rows["Ivy"]["club"], []string{"-"}) {
		t.Errorf("the people's clubs: %v", rows)
	}
	if !regexp.MustCompile(`<label class="checkbox is-block" data-list-row="" data-name="Mo"[^>]*>\s*<input type="checkbox" name="person"`).MatchString(page) {
		t.Error("each person is a checkbox with the row's attributes")
	}
}

func TestClubMembersListFilters(t *testing.T) {
	s := listsCompetition(t)
	page := do(t, s.h, http.MethodGet, s.club+"/members", nil).Body.String()
	if !hasListTools(page, "members", "members", "coach", "entries", "coachtoo") {
		t.Errorf("the tools, with coach, entries and coach too: %s", page)
	}
	for _, want := range []string{`<option value="` + s.annID + `">Ann O&#39;Neill / Jr?</option>`, `<option value="` + s.bobID + `">Bob</option>`,
		`<option value="-">No coach</option>`, `<option value="yes">Has entries</option><option value="no">No entries</option>`,
		`<option value="yes">Coach too</option><option value="no">Not a coach</option>`} {
		if !strings.Contains(page, want) {
			t.Errorf("the tools have %s in %s", want, regexp.MustCompile(`(?s)data-list-tools.*?data-list-none`).FindString(page))
		}
	}
	if !regexp.MustCompile(`<section class="comp-list mt-5"><h2 class="title is-5 mb-3">Members <span[^>]*>· 2</span></h2>`).MatchString(page) {
		t.Error("the list is under its heading and count")
	}
	rows := listRows(t, page, "members")
	mo, zoe := rows["Mo"], rows[zoeName]
	if !slices.Equal(mo["coach"], []string{s.annID}) || !slices.Equal(mo["entries"], []string{"yes"}) || !slices.Equal(mo["coachtoo"], []string{"no"}) {
		t.Errorf("Mo: %v", mo)
	}
	if !slices.Equal(zoe["coach"], []string{"-"}) || !slices.Equal(zoe["coachtoo"], []string{"yes"}) {
		t.Errorf("Zoë: %v", zoe)
	}

	// A club with no members has no tools.
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	club := anEmptyClub(t, h, admin)
	if empty := do(t, h, http.MethodGet, club+"/members", nil).Body.String(); strings.Contains(empty, "data-list-tools") || !strings.Contains(empty, "No members yet.") {
		t.Error("no search on an empty list")
	}
}

func TestClubCoachesListFilters(t *testing.T) {
	s := listsCompetition(t)
	page := do(t, s.h, http.MethodGet, s.club+"/coaches", nil).Body.String()
	if !hasListTools(page, "coaches", "coaches", "signs", "quals") {
		t.Errorf("the tools, with signs off and qualifications: %s", page)
	}
	for _, want := range []string{`<option value="yes">Signs off routines</option><option value="no">Doesn&#39;t sign off</option>`,
		`<option value="yes">Has qualifications</option><option value="no">None</option>`} {
		if !strings.Contains(page, want) {
			t.Errorf("the tools have %s", want)
		}
	}
	if n := strings.Count(page, "<tbody data-list-row="); n != 3 {
		t.Errorf("each coach's two table rows are one tbody: %d", n)
	}
	if !strings.Contains(page, `action="`+s.club+`/coaches"`) || !strings.Contains(page, "Coaches sign off members' routines") {
		t.Errorf("the add-coach form and the help text are still there: %s", "x")
	}
	rows := listRows(t, page, "coaches")
	if !slices.Equal(rows[annName]["signs"], []string{"yes"}) || !slices.Equal(rows[annName]["quals"], []string{"yes"}) ||
		!slices.Equal(rows["Bob"]["signs"], []string{"no"}) || !slices.Equal(rows["Bob"]["quals"], []string{"no"}) || !slices.Equal(rows[zoeName]["signs"], []string{"yes"}) {
		t.Errorf("the coaches' values: %v", rows)
	}

	// A club with no coaches has no tools.
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	club := anEmptyClub(t, h, admin)
	if empty := do(t, h, http.MethodGet, club+"/coaches", nil).Body.String(); strings.Contains(empty, "data-list-tools") {
		t.Error("no search on an empty list")
	}
}

func TestCoachEntriesListFilters(t *testing.T) {
	s := listsCompetition(t)
	page := do(t, s.h, http.MethodGet, s.annPage, nil).Body.String()
	if !hasListTools(page, "coach-entries", "entries", "level", "problems", "signoff") {
		t.Errorf("the tools, without a competition filter for one competition: %s", page)
	}
	for _, want := range []string{`<option value="BUCS L3">BUCS L3</option>`, `<option value="yes">Has problems</option><option value="no">No problems</option>`,
		`<option value="signed">Signed off</option><option value="not-yet">Not yet</option><option value="waiting">Waiting</option>`} {
		if !strings.Contains(page, want) {
			t.Errorf("the tools have %s", want)
		}
	}
	if strings.Contains(page, "<summary class=\"title is-5 mb-3\">") || strings.Count(page, `<div class="box" data-list-group>`) != 1 {
		t.Error("each competition is a group, and the page isn't a section")
	}
	rows := listRows(t, page, "coach-entries")
	if len(rows) != 2 || !slices.Equal(rows["Mo"]["signoff"], []string{"waiting"}) || !slices.Equal(rows["Mo"]["level"], []string{"BUCS L3"}) ||
		!slices.Equal(rows["Mo"]["problems"], []string{"yes"}) || len(rows["Mo"]["competition"]) != 1 {
		t.Errorf("the entries' values: %v", rows)
	}
	open := strings.ReplaceAll(regexp.MustCompile(`href="(` + regexp.QuoteMeta(s.annPage) + `/members/[^"]+)">Mo</a>`).FindStringSubmatch(page)[1], "&amp;", "&")
	redirected(t, s.h, open, url.Values{"signed": {"1"}})
	if rows := listRows(t, do(t, s.h, http.MethodGet, s.annPage, nil).Body.String(), "coach-entries"); !slices.Equal(rows["Mo"]["signoff"], []string{"signed"}) {
		t.Errorf("signed off: %v", rows["Mo"])
	}

	// A club with no competitions has no tools.
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	club := anEmptyClub(t, h, admin)
	cy := pathIn(t, do(t, h, http.MethodPost, club+"/coaches", url.Values{"name": {"Cy"}}).Body.String(), "/clubs/coach/")
	if page := do(t, h, http.MethodGet, cy, nil).Body.String(); strings.Contains(page, "data-list-tools") {
		t.Error("no search with no entries")
	}
}

// anEmptyClub is a club linked to the competition, with no members or coaches.
func anEmptyClub(t *testing.T, h http.Handler, admin string) string {
	t.Helper()
	clubLink := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/club/")
	club := withoutQuery(redirected(t, h, "/clubs", url.Values{"name": {"Empty"}}))
	redirected(t, h, clubLink, url.Values{"clubAdmin": {club}})
	return club
}

func TestCoachEntryFilters(t *testing.T) {
	two := []views.ListOption{{Value: "a", Label: "Spring"}, {Value: "b", Label: "Autumn"}}
	f := coachEntryFilters(two, []string{"BUCS L3", "BUCS L2", "BUCS L3"})
	var keys, levels []string
	for _, x := range f {
		keys = append(keys, x.Key)
	}
	for _, o := range f[1].Options {
		levels = append(levels, o.Value)
	}
	if !slices.Equal(keys, []string{"competition", "level", "problems", "signoff"}) || !slices.Equal(levels, []string{"BUCS L2", "BUCS L3"}) {
		t.Errorf("competition when there are two; each level once, by name: %v %v", keys, levels)
	}
	if f := coachEntryFilters(two[:1], nil); f[0].Key != "level" {
		t.Error("no competition filter for one competition")
	}
	for s, want := range map[views.SignoffView]string{{Signed: true, By: "Ann"}: "signed", {By: "Ann"}: "not-yet", {Required: true}: "waiting", {}: "waiting"} {
		if got := coachSignoffValue(s); got != want {
			t.Errorf("%+v is %s, not %s", s, got, want)
		}
	}
}

func TestPeopleDoAndCoachStatus(t *testing.T) {
	for roles, want := range map[string][]string{
		"gymnast judge chair": {"gymnast", "official"}, "coach": {"coach"}, "marshal coach": {"official", "coach"}, "gymnast coach": {"gymnast", "coach"}, "": nil,
	} {
		if got := peopleDo(strings.Fields(roles)); !slices.Equal(got, want) {
			t.Errorf("%q does %v, not %v", roles, got, want)
		}
	}
	for status, want := range map[string]string{store.CoachWaiting: "waiting", store.CoachApproved: "approved", store.CoachRefused: "refused", store.CoachWithdrawn: "withdrawn"} {
		if got := coachStatusValue(status); got != want {
			t.Errorf("%q is %s, not %s", status, got, want)
		}
	}
	f := memberCoachFilter([]views.CoachOption{{ID: "c1", Name: "Ann"}})
	if len(f.Options) != 2 || f.Options[1].Value != "-" || memberCoachValue("") != "-" || memberCoachValue("c1") != "c1" {
		t.Errorf("the coach filter ends with no coach: %+v", f.Options)
	}
}
