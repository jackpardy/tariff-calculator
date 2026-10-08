package main

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func TestLateChanges(t *testing.T) {
	h := competitionServer(t)
	form := newCompetition()
	form["level"] = append(form["level"], "builtin-level:bucs-l4")
	admin := created(t, h, form)
	dash := do(t, h, http.MethodGet, admin, nil).Body.String()
	clubLink, enter := pathIn(t, dash, "/competitions/club/"), pathIn(t, dash, "/competitions/enter/")
	l3 := url.Values{"level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}}
	l4 := url.Values{"level": {"BUCS L4"}, "ex1Option": {"builtin:bucs-l4-option-1"}, "ex2Option": {"builtin:bucs-l4-second"}, "ex2Skills": {voluntary}}
	// A club's member, Ann, and Ivy on her own, both BUCS L3; Bea in L4.
	club := withoutQuery(redirected(t, h, "/clubs", url.Values{"name": {"UCD"}}))
	redirected(t, h, clubLink, url.Values{"clubAdmin": {club}})
	join := pathIn(t, do(t, h, http.MethodGet, club, nil).Body.String(), "/clubs/join/")
	ann := withoutQuery(redirected(t, h, join, url.Values{"name": {"Ann"}}))
	redirected(t, h, formAction(t, do(t, h, http.MethodGet, ann, nil).Body.String(), ann+"/competitions/"), l3)
	redirected(t, h, formAction(t, do(t, h, http.MethodGet, club, nil).Body.String(), club+"/competitions/"), url.Values{"which": {"all"}})
	with := func(name string, v url.Values) url.Values {
		out := url.Values{"gymnast": {name}}
		for k, x := range v {
			out[k] = x
		}
		return out
	}
	ivy := withoutQuery(redirected(t, h, enter, with("Ivy", l3)))
	redirected(t, h, enter, with("Bea", l4))
	redirected(t, h, admin+"/timetable/plan", url.Values{})

	// While entries are open there's no asking.
	if strings.Contains(do(t, h, http.MethodGet, ivy, nil).Body.String(), "Ask for a late change") {
		t.Error("no late changes while entries are open")
	}
	redirected(t, h, admin+"/deadline", url.Values{"close": {"1"}})
	if strings.Contains(do(t, h, http.MethodGet, ivy, nil).Body.String(), "Ask for a late change") {
		t.Error("none allowed yet")
	}
	// Level changes allowed, at €10; routine changes not.
	redirected(t, h, admin+"/late/settings", url.Values{"late-level": {"1"}, "fee-level": {"10"}, "fee-routines": {"5"}})
	if !strings.Contains(do(t, h, http.MethodGet, ivy, nil).Body.String(), ivy+"/late") {
		t.Fatal("Ivy can ask")
	}
	if page := do(t, h, http.MethodGet, ivy+"/late", nil).Body.String(); !strings.Contains(page, "Level change (€10 when accepted)") && !strings.Contains(page, "Level change (€10 if accepted)") {
		t.Errorf("the page says what's allowed: %s", page)
	}
	if rec := do(t, h, http.MethodPost, ivy+"/late", l3); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "change it first") {
		t.Errorf("no change: %d", rec.Code)
	}
	redirected(t, h, ivy+"/late", with("Ivy", url.Values{"level": {"BUCS L4"}, "ex1Option": {"builtin:bucs-l4-option-1"}, "ex2Option": {"builtin:bucs-l4-second"}, "ex2Skills": {voluntary}, "note": {"Moved up by her coach"}}))
	if page := do(t, h, http.MethodGet, ivy, nil).Body.String(); !strings.Contains(page, "Level change to BUCS L4") || !strings.Contains(page, "waiting for the organiser") {
		t.Error("Ivy sees it's waiting")
	}

	// Ann, by her comp sec.
	clubPage := do(t, h, http.MethodGet, club, nil).Body.String()
	annLate := regexp.MustCompile(regexp.QuoteMeta(club) + `/members/[^"]+/late\?discipline=`).FindString(clubPage)
	if annLate == "" {
		t.Fatal("the comp sec can ask for Ann")
	}
	redirected(t, h, strings.ReplaceAll(annLate, "&amp;", "&"), l4)

	// The organiser sees both, with whether they fit.
	page := do(t, h, http.MethodGet, admin, nil).Body.String()
	if !strings.Contains(page, "2 to decide") {
		t.Error("the dashboard says there are late changes to decide")
	}
	late := do(t, h, http.MethodGet, admin+"/late", nil).Body.String()
	if !strings.Contains(late, "BUCS L3 → <strong>BUCS L4</strong>") || !strings.Contains(late, "Moved up by her coach") || !strings.Contains(late, "In the draft timetable: into BUCS L4") {
		t.Fatalf("the requests and checks: %s", late)
	}
	ids := regexp.MustCompile(`/late/([^/"]+)/accept`).FindAllStringSubmatch(late, -1)
	if len(ids) != 2 {
		t.Fatalf("two to decide: %d", len(ids))
	}
	// Reject one with a reason, accept the other.
	if loc := redirected(t, h, admin+"/late/"+ids[1][1]+"/reject", url.Values{}); !strings.Contains(loc, "Give+a+reason") {
		t.Error("a reason is needed")
	}
	redirected(t, h, admin+"/late/"+ids[1][1]+"/reject", url.Values{"reason": {"BUCS L4 is full on the day"}})
	loc := redirected(t, h, admin+"/late/"+ids[0][1]+"/accept", url.Values{})
	if !strings.Contains(loc, "draft+timetable+has+them+in+BUCS+L4") {
		t.Errorf("accepted into the draft timetable: %s", loc)
	}
	statuses := do(t, h, http.MethodGet, ivy, nil).Body.String() + do(t, h, http.MethodGet, club, nil).Body.String()
	if !strings.Contains(statuses, "accepted (€10, on the invoice)") || !strings.Contains(statuses, "not accepted. BUCS L4 is full on the day") {
		t.Errorf("each sees the decision: %s", statuses)
	}

	// The accepted one is in BUCS L4, with the fee charged once there are fees.
	redirected(t, h, admin+"/fees/settings", url.Values{"fee-": {"12"}})
	fees := do(t, h, http.MethodGet, admin+"/fees", nil).Body.String()
	if !strings.Contains(fees, "€22") {
		t.Errorf("€12 entry and €10 late change: %s", fees)
	}
	if hist := do(t, h, http.MethodGet, admin+"/history", nil).Body.String(); !strings.Contains(hist, "Accepted a late change") || !strings.Contains(hist, "Turned down a late change") {
		t.Error("the history")
	}
}

// With sign-off first, a late change waits for a coach, whose sign-off
// goes onto the entry when it's accepted.
func TestLateChangeSignoff(t *testing.T) {
	h := competitionServer(t)
	form := newCompetition()
	form.Set("signoff", "1")
	form["level"] = append(form["level"], "builtin-level:bucs-l4")
	admin := created(t, h, form)
	dash := do(t, h, http.MethodGet, admin, nil).Body.String()
	clubLink, enter := pathIn(t, dash, "/competitions/club/"), pathIn(t, dash, "/competitions/enter/")
	l3 := url.Values{"level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}}
	l4 := url.Values{"level": {"BUCS L4"}, "ex1Option": {"builtin:bucs-l4-option-1"}, "ex2Option": {"builtin:bucs-l4-second"}, "ex2Skills": {voluntary}}
	club := withoutQuery(redirected(t, h, "/clubs", url.Values{"name": {"UCD"}}))
	redirected(t, h, clubLink, url.Values{"clubAdmin": {club}})
	ann := pathIn(t, do(t, h, http.MethodPost, club+"/coaches", url.Values{"name": {"Ann"}}).Body.String(), "/clubs/coach/")
	join := pathIn(t, do(t, h, http.MethodGet, club, nil).Body.String(), "/clubs/join/")
	x := withoutQuery(redirected(t, h, join, url.Values{"name": {"X"}}))
	redirected(t, h, formAction(t, do(t, h, http.MethodGet, x, nil).Body.String(), x+"/competitions/"), l3)
	redirected(t, h, formAction(t, do(t, h, http.MethodGet, club, nil).Body.String(), club+"/competitions/"), url.Values{"which": {"all"}})
	ind := url.Values{"gymnast": {"Ivy"}}
	for k, v := range l3 {
		ind[k] = v
	}
	ivy := withoutQuery(redirected(t, h, enter, ind))
	signoff := pathIn(t, do(t, h, http.MethodGet, ivy, nil).Body.String(), "/competitions/signoff/")

	redirected(t, h, admin+"/deadline", url.Values{"close": {"1"}})
	redirected(t, h, admin+"/late/settings", url.Values{"late-level": {"1"}, "signoffFirst": {"1"}})

	// X asks; it waits for a coach.
	ask := regexp.MustCompile(regexp.QuoteMeta(x) + `/competitions/[^/"]+/late\?discipline=`).FindString(do(t, h, http.MethodGet, x, nil).Body.String())
	if ask == "" {
		t.Fatal("X can ask")
	}
	if loc := redirected(t, h, strings.ReplaceAll(ask, "&amp;", "&"), l4); !strings.Contains(loc, "gone+to+your+coach") {
		t.Errorf("it goes to the coach first: %s", loc)
	}
	late := do(t, h, http.MethodGet, admin+"/late", nil).Body.String()
	if !strings.Contains(late, "waiting for a coach to sign it off") || strings.Contains(late, "/accept") {
		t.Fatalf("the organiser can't accept it yet: %s", late)
	}

	// Ann signs it off from her page.
	home := do(t, h, http.MethodGet, ann, nil).Body.String()
	link := regexp.MustCompile(regexp.QuoteMeta(ann) + `/late/[^"]+`).FindString(home)
	if link == "" {
		t.Fatal("Ann's page lists it")
	}
	if page := do(t, h, http.MethodGet, link, nil).Body.String(); !strings.Contains(page, "Level change to BUCS L4") {
		t.Error("Ann sees the change")
	}
	redirected(t, h, link, url.Values{"note": {"Seen it in training"}})

	// Ivy asks; her coach signs it off through her sign-off link.
	redirected(t, h, ivy+"/late", func() url.Values {
		v := url.Values{"gymnast": {"Ivy"}}
		for k, x := range l4 {
			v[k] = x
		}
		return v
	}())
	if page := do(t, h, http.MethodGet, signoff, nil).Body.String(); !strings.Contains(page, "Late changes to sign off") {
		t.Fatal("Ivy's coach sees it on her sign-off link")
	}
	if rec := do(t, h, http.MethodPost, signoff+"/late", url.Values{}); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("the coach gives their name: %d", rec.Code)
	}
	redirected(t, h, signoff+"/late", url.Values{"coach": {"Coach C"}})

	// Both reach the organiser, signed off; accepted, the entries are signed off.
	late = do(t, h, http.MethodGet, admin+"/late", nil).Body.String()
	if !strings.Contains(late, "Signed off by Ann.") || !strings.Contains(late, "Signed off by Coach C.") {
		t.Fatalf("the checks say who signed off: %s", late)
	}
	for _, m := range regexp.MustCompile(`/late/([^/"]+)/accept`).FindAllStringSubmatch(late, -1) {
		redirected(t, h, admin+"/late/"+m[1]+"/accept", url.Values{})
	}
	page := do(t, h, http.MethodGet, admin, nil).Body.String()
	if strings.Contains(page, "Not signed off by a coach") {
		t.Errorf("the changed entries are signed off: %s", page)
	}
	if !strings.Contains(do(t, h, http.MethodGet, x, nil).Body.String(), "Ann") {
		t.Error("X's entry shows Ann's sign-off")
	}
}
