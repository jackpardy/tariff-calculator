package main

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func TestCoachSignoff(t *testing.T) {
	h := competitionServer(t)
	form := newCompetition()
	form.Set("signoff", "1")
	admin := created(t, h, form)
	dash := do(t, h, http.MethodGet, admin, nil).Body.String()
	if !strings.Contains(dash, "Entries need a coach&#39;s sign-off.") && !strings.Contains(dash, "Entries need a coach's sign-off.") {
		t.Error("the competition says it needs sign-off")
	}
	clubLink := pathIn(t, dash, "/competitions/club/")
	club := withoutQuery(redirected(t, h, "/clubs", url.Values{"name": {"UCD"}}))
	redirected(t, h, clubLink, url.Values{"clubAdmin": {club}})

	// The comp sec adds two coaches; each link is shown once.
	rec := do(t, h, http.MethodPost, club+"/coaches", url.Values{"name": {"Ann"}})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Ann&#39;s coach link") {
		t.Fatalf("adding a coach: %d", rec.Code)
	}
	ann := pathIn(t, rec.Body.String(), "/clubs/coach/")
	bob := pathIn(t, do(t, h, http.MethodPost, club+"/coaches", url.Values{"name": {"Bob"}}).Body.String(), "/clubs/coach/")
	if location := redirected(t, h, club+"/coaches", url.Values{"name": {""}}); !strings.Contains(location, "needs+a+name") {
		t.Errorf("a coach needs a name: %s", location)
	}

	// Two members join: X chooses Bob, Y chooses nobody.
	join := pathIn(t, do(t, h, http.MethodGet, club, nil).Body.String(), "/clubs/join/")
	x := withoutQuery(redirected(t, h, join, url.Values{"name": {"X"}}))
	y := withoutQuery(redirected(t, h, join, url.Values{"name": {"Y"}}))
	page := do(t, h, http.MethodGet, x, nil).Body.String()
	if !strings.Contains(page, "Your coach") || !strings.Contains(page, ">Ann</option>") {
		t.Error("a member can choose their coach")
	}
	bobID := regexp.MustCompile(`<option value="([^"]+)">Bob</option>`).FindStringSubmatch(page)[1]
	redirected(t, h, x+"/coach", url.Values{"coach": {bobID}})
	if page := do(t, h, http.MethodGet, x, nil).Body.String(); !strings.Contains(page, `<option value="`+bobID+`" selected>Bob</option>`) {
		t.Error("X's coach is Bob")
	}
	entry := url.Values{"level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}}
	for _, m := range []string{x, y} {
		redirected(t, h, formAction(t, do(t, h, http.MethodGet, m, nil).Body.String(), m+"/competitions/"), entry)
	}
	if page := do(t, h, http.MethodGet, y, nil).Body.String(); !strings.Contains(page, "Waiting for a coach&#39;s sign-off") && !strings.Contains(page, "Waiting for a coach's sign-off") {
		t.Error("the member sees their entry waiting for sign-off")
	}

	// Ann sees only Y (no coach); Bob sees X and Y.
	if page := do(t, h, http.MethodGet, ann, nil).Body.String(); strings.Contains(page, ">X</a>") || !strings.Contains(page, ">Y</a>") || !strings.Contains(page, "1 to sign off") {
		t.Error("Ann sees members without a coach, not Bob's")
	}
	page = do(t, h, http.MethodGet, bob, nil).Body.String()
	if !strings.Contains(page, ">X</a>") || !strings.Contains(page, ">Y</a>") || !strings.Contains(page, "2 to sign off") {
		t.Error("Bob sees his member and those without a coach")
	}
	xEntry := regexp.MustCompile(`href="(` + regexp.QuoteMeta(bob) + `/members/[^"]+)">X</a>`).FindStringSubmatch(page)[1]
	if rec := do(t, h, http.MethodGet, strings.Replace(xEntry, bob, ann, 1), nil); rec.Code != http.StatusNotFound {
		t.Errorf("Ann can't open Bob's member's entry: %d", rec.Code)
	}
	if page := do(t, h, http.MethodGet, xEntry, nil).Body.String(); !strings.Contains(page, "Sign off") || !strings.Contains(page, "Second exercise") {
		t.Error("the coach sees the entry checked, with the sign-off form")
	}

	// The comp sec lets every coach see everyone.
	redirected(t, h, club+"/coaches-see-all", url.Values{"on": {"1"}})
	if page := do(t, h, http.MethodGet, ann, nil).Body.String(); !strings.Contains(page, ">X</a>") || !strings.Contains(page, "You see every member") {
		t.Error("with see-all, Ann sees X too")
	}

	// Bob signs off X; Y is sent without a sign-off and flagged.
	location := redirected(t, h, xEntry, url.Values{"signed": {"1"}, "note": {"Ready"}})
	if page := do(t, h, http.MethodGet, location, nil).Body.String(); !strings.Contains(page, "Signed off X&#39;s entry") && !strings.Contains(page, "Signed off X's entry") {
		t.Error("signed off")
	}
	if page := do(t, h, http.MethodGet, x, nil).Body.String(); !strings.Contains(page, "Signed off by Bob") || !strings.Contains(page, "Ready") {
		t.Error("the member sees who signed it off")
	}
	clubPage := do(t, h, http.MethodGet, club, nil).Body.String()
	if !strings.Contains(clubPage, "Signed off by Bob") || !strings.Contains(clubPage, "Waiting for a coach") {
		t.Error("the comp sec sees each entry's sign-off")
	}
	redirected(t, h, formAction(t, clubPage, club+"/competitions/"), url.Values{"which": {"all"}})
	dash = do(t, h, http.MethodGet, admin, nil).Body.String()
	if !strings.Contains(dash, "<th>Coach</th>") || !strings.Contains(dash, "✓ Bob") || !strings.Contains(dash, "not signed off") {
		t.Error("the organiser sees who signed off, and the entry without one flagged")
	}
	yEntry := regexp.MustCompile(`href="(` + regexp.QuoteMeta(admin) + `/entries/[^"]+)">Y</a>`).FindStringSubmatch(dash)[1]
	if page := do(t, h, http.MethodGet, yEntry, nil).Body.String(); !strings.Contains(page, "<li>Not signed off by a coach</li>") {
		t.Error("not signed off is one of the entry's problems")
	}
	csv := do(t, h, http.MethodGet, admin+"/entries.csv", nil).Body.String()
	if !strings.Contains(csv, ",Bob\n") {
		t.Errorf("the CSV says who signed off:\n%s", csv)
	}

	// The comp sec assigns Y to Ann from the club page's members list.
	clubPage = do(t, h, http.MethodGet, club, nil).Body.String()
	assign := regexp.MustCompile(`action="(` + regexp.QuoteMeta(club) + `/members/[^"]+/coach)"`).FindAllStringSubmatch(clubPage, -1)
	if len(assign) != 2 {
		t.Fatalf("each member has a coach choice on the club page: %d", len(assign))
	}
	annID := regexp.MustCompile(`<option value="([^"]+)">Ann</option>`).FindStringSubmatch(clubPage)[1]
	redirected(t, h, assign[1][1], url.Values{"coach": {annID}}) // members are by name: X, then Y
	if page := do(t, h, http.MethodGet, y, nil).Body.String(); !strings.Contains(page, `<option value="`+annID+`" selected>Ann</option>`) {
		t.Error("Y's page shows the coach the comp sec assigned")
	}
	redirected(t, h, club+"/coaches-see-all", url.Values{"on": {""}})
	if page := do(t, h, http.MethodGet, bob, nil).Body.String(); strings.Contains(page, ">Y</a>") {
		t.Error("once Y has Ann, Bob no longer sees Y")
	}

	// Printed cards name the coach: who signed off, or else the member's coach.
	cards := do(t, h, http.MethodGet, admin+"/cards", nil).Body.String()
	if !strings.Contains(cards, `aria-label="Coach" value="Bob"`) || !strings.Contains(cards, `aria-label="Coach" value="Ann"`) {
		t.Error("X's cards say Bob (signed off), Y's say Ann (assigned)")
	}

	// A change needs signing off again.
	entry.Set("ex1Option", "builtin:bucs-l3-option-2")
	redirected(t, h, formAction(t, do(t, h, http.MethodGet, x, nil).Body.String(), x+"/competitions/"), entry)
	if page := do(t, h, http.MethodGet, x, nil).Body.String(); strings.Contains(page, "Signed off by Bob") {
		t.Error("a changed entry isn't signed off any more")
	}

	// Removing a coach ends their link.
	page = do(t, h, http.MethodGet, club, nil).Body.String()
	remove := regexp.MustCompile(`action="(` + regexp.QuoteMeta(club) + `/coaches/[^"]+/remove)"`).FindStringSubmatch(page)[1]
	redirected(t, h, remove, url.Values{})
	if do(t, h, http.MethodGet, ann, nil).Code != http.StatusNotFound {
		t.Error("the removed coach's link stops working")
	}
}

func TestIndividualSignoff(t *testing.T) {
	h := competitionServer(t)
	form := newCompetition()
	form.Set("signoff", "1")
	admin := created(t, h, form)
	enter := pathIn(t, do(t, h, http.MethodGet, admin, nil).Body.String(), "/competitions/enter/")
	own := withoutQuery(redirected(t, h, enter, url.Values{"gymnast": {"I"}, "level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}}))
	page := do(t, h, http.MethodGet, own, nil).Body.String()
	if !strings.Contains(page, "Sign-off link for your coach") {
		t.Fatal("the individual gets a sign-off link to send")
	}
	signoff := pathIn(t, page, "/competitions/signoff/")
	if page := do(t, h, http.MethodGet, signoff, nil).Body.String(); !strings.Contains(page, "Your name (coach)") || !strings.Contains(page, "Second exercise") {
		t.Error("the coach sees the entry and gives their name")
	}
	if rec := do(t, h, http.MethodPost, signoff, url.Values{"signed": {"1"}}); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("the coach must give their name: %d", rec.Code)
	}
	redirected(t, h, signoff, url.Values{"signed": {"1"}, "coach": {"Coach C"}})
	if page := do(t, h, http.MethodGet, own, nil).Body.String(); !strings.Contains(page, "Signed off by Coach C") {
		t.Error("the individual sees it signed off")
	}
	if dash := do(t, h, http.MethodGet, admin, nil).Body.String(); !strings.Contains(dash, "✓ Coach C") {
		t.Error("the organiser sees it signed off")
	}
	if cards := do(t, h, http.MethodGet, admin+"/cards", nil).Body.String(); !strings.Contains(cards, `aria-label="Coach" value="Coach C"`) {
		t.Error("the individual's cards name the coach who signed off")
	}

	// Without the requirement, there's no sign-off link.
	redirected(t, h, admin+"/signoff", url.Values{"on": {"0"}})
	if page := do(t, h, http.MethodGet, own, nil).Body.String(); strings.Contains(page, "Sign-off link for your coach") {
		t.Error("no sign-off link when it isn't needed")
	}
}
