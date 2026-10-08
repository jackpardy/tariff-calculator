package main

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func TestCoachApprovalPages(t *testing.T) {
	h := competitionServer(t)
	form := newCompetition()
	form.Set("signoff", "1")
	admin := created(t, h, form)

	// The organiser asks for approved coaches, trampoline at Level 3.
	if dash := do(t, h, http.MethodGet, admin, nil).Body.String(); !strings.Contains(dash, "Coaches must be approved") {
		t.Fatal("the setting is offered where entries need sign-off")
	}
	if location := redirected(t, h, admin+"/approve-coaches", url.Values{"on": {"1"}, "level-trampoline": {"3"}}); !strings.Contains(location, "Only+coaches+you+approve") {
		t.Errorf("saved: %s", location)
	}
	dash := do(t, h, http.MethodGet, admin, nil).Body.String()
	if !strings.Contains(dash, `href="`+admin+`/coaches"`) || !strings.Contains(dash, `<option value="3" selected>Level 3</option>`) {
		t.Error("the dashboard links to the coaches, and keeps the level")
	}

	// A club with Ann (Level 3, X's coach) and Bob (who doesn't sign off).
	clubLink := pathIn(t, dash, "/competitions/club/")
	club := withoutQuery(redirected(t, h, "/clubs", url.Values{"name": {"UCD"}}))
	redirected(t, h, clubLink, url.Values{"clubAdmin": {club}})
	ann := pathIn(t, do(t, h, http.MethodPost, club+"/coaches", url.Values{"name": {"Ann"}}).Body.String(), "/clubs/coach/")
	do(t, h, http.MethodPost, club+"/coaches", url.Values{"name": {"Bob"}})
	page := do(t, h, http.MethodGet, club, nil).Body.String()
	ids := regexp.MustCompile(regexp.QuoteMeta(club)+`/coaches/([^/"]+)/signs-off`).FindAllStringSubmatch(page, -1)
	annID, bobID := ids[0][1], ids[1][1]
	redirected(t, h, club+"/coaches/"+bobID+"/signs-off", url.Values{"on": {""}})
	upload(t, h, club+"/coaches/"+annID+"/qualifications", map[string]string{"qualification": "bg-trampoline-3"}, "certificate", []byte("%PDF-1.4 Ann"))
	join := pathIn(t, page, "/clubs/join/")
	x := withoutQuery(redirected(t, h, join, url.Values{"name": {"X"}}))
	redirected(t, h, x+"/coach", url.Values{"coach": {annID}})
	entry := url.Values{"level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}}
	sendX := formAction(t, do(t, h, http.MethodGet, x, nil).Body.String(), x+"/competitions/")
	redirected(t, h, sendX, entry)

	// Not sent yet: Ann can't sign off.
	home := do(t, h, http.MethodGet, ann, nil).Body.String()
	if !strings.Contains(home, "your club hasn&#39;t sent you yet") && !strings.Contains(home, "your club hasn't sent you yet") {
		t.Error("Ann sees she hasn't been sent")
	}
	open := strings.ReplaceAll(regexp.MustCompile(`href="(` + regexp.QuoteMeta(ann) + `/members/[^"]+)"`).FindStringSubmatch(home)[1], "&amp;", "&")
	if location := redirected(t, h, open, url.Values{"signed": {"1"}}); !strings.Contains(location, "Nothing+was+changed") {
		t.Errorf("refused: %s", location)
	}

	// The club sends Ann (ticked, as X's coach); Bob isn't offered.
	page = do(t, h, http.MethodGet, club, nil).Body.String()
	comp := regexp.MustCompile(regexp.QuoteMeta(club) + `/competitions/([^/"]+)/coaches`).FindStringSubmatch(page)
	if comp == nil || !regexp.MustCompile(`value="`+annID+`" checked`).MatchString(page) || strings.Contains(page, `name="coach" value="`+bobID+`"`) {
		t.Fatal("the club's coaches to send: Ann ticked, Bob not offered")
	}
	redirected(t, h, club+"/competitions/"+comp[1]+"/coaches", url.Values{"coach": {annID}})
	if page := do(t, h, http.MethodGet, club, nil).Body.String(); !strings.Contains(page, "Waiting for the organiser") {
		t.Error("the club sees Ann waiting")
	}

	// The organiser sees Ann, her certificate and that she meets Level 3, and approves her.
	coaches := do(t, h, http.MethodGet, admin+"/coaches", nil).Body.String()
	cert := regexp.MustCompile(`href="(` + regexp.QuoteMeta(admin) + `/coaches/[^"]+/certificates/[^"]+)"`).FindStringSubmatch(coaches)
	if !strings.Contains(coaches, "UCD") || !strings.Contains(coaches, "British Gymnastics Level 3 Trampoline Coach") || !strings.Contains(coaches, "Trampoline Level 3 ✓") || cert == nil {
		t.Fatalf("the coaches page: %s", coaches)
	}
	if got := do(t, h, http.MethodGet, cert[1], nil); got.Body.String() != "%PDF-1.4 Ann" || got.Header().Get("Content-Type") != "application/pdf" {
		t.Error("the organiser opens the certificate")
	}
	if dash := do(t, h, http.MethodGet, admin, nil).Body.String(); !strings.Contains(dash, "1 waiting") {
		t.Error("the dashboard counts her waiting")
	}
	redirected(t, h, admin+"/coaches/"+annID, url.Values{"action": {"approve"}})

	// Approved, she signs off; sent, it counts.
	redirected(t, h, open, url.Values{"signed": {"1"}})
	send := regexp.MustCompile(`action="(` + regexp.QuoteMeta(club) + `/competitions/[^"]+/send)"`).FindStringSubmatch(do(t, h, http.MethodGet, club, nil).Body.String())[1]
	redirected(t, h, send, url.Values{"which": {"all"}})
	problems := func() string { // the entry, as the organiser sees it
		dash := do(t, h, http.MethodGet, admin, nil).Body.String()
		e := regexp.MustCompile(`href="(` + regexp.QuoteMeta(admin) + `/entries/[^"?]+)`).FindStringSubmatch(dash)
		if e == nil {
			t.Fatal("X's entry on the dashboard")
		}
		return do(t, h, http.MethodGet, e[1], nil).Body.String()
	}
	if p := problems(); strings.Contains(p, "Not signed off by a coach") || strings.Contains(p, "approved coach") {
		t.Error("Ann's sign-off counts")
	}

	// Withdrawn: it doesn't, and says why; Ann sees it.
	redirected(t, h, admin+"/coaches/"+annID, url.Values{"action": {"withdraw"}, "note": {"Certificate out of date"}})
	if p := problems(); !strings.Contains(p, "Signed off by Ann, who isn&#39;t an approved coach") && !strings.Contains(p, "Signed off by Ann, who isn't an approved coach") {
		t.Error("an unapproved coach's sign-off is a problem")
	}
	if home := do(t, h, http.MethodGet, ann, nil).Body.String(); !strings.Contains(home, "withdrew your approval") || !strings.Contains(home, "Certificate out of date") {
		t.Error("Ann sees her approval withdrawn, and why")
	}
	// Approved from now on: still not counting; approved with her sign-offs: it counts.
	coaches = do(t, h, http.MethodGet, admin+"/coaches", nil).Body.String()
	if !strings.Contains(coaches, "Approve, with their sign-offs before") || !strings.Contains(coaches, "Approve, sign-offs from now on") {
		t.Error("approving again asks about her sign-offs")
	}
	redirected(t, h, admin+"/coaches/"+annID, url.Values{"action": {"afresh"}})
	if p := problems(); !strings.Contains(p, "approved coach") {
		t.Error("only sign-offs from now on")
	}
	// Signing off again counts; withdrawn and approved again with her
	// sign-offs, it still counts.
	redirected(t, h, open, url.Values{"signed": {"1"}})
	redirected(t, h, send, url.Values{"which": {"all"}})
	redirected(t, h, admin+"/coaches/"+annID, url.Values{"action": {"withdraw"}})
	redirected(t, h, admin+"/coaches/"+annID, url.Values{"action": {"approve"}})
	if p := problems(); strings.Contains(p, "approved coach") || strings.Contains(p, "Not signed off") {
		t.Error("approved again with her sign-offs")
	}
}
