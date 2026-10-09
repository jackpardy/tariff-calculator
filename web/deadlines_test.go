package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func TestLaterDeadlines(t *testing.T) {
	h := competitionServer(t)
	form := newCompetition()
	form.Set("signoff", "1")
	admin := created(t, h, form)
	dash := do(t, h, http.MethodGet, admin, nil).Body.String()
	clubLink, enter := pathIn(t, dash, "/competitions/club/"), pathIn(t, dash, "/competitions/enter/")
	l3 := url.Values{"level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}}
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

	// Changes and sign-offs close a week after the deadline, on the competition date.
	if loc := redirected(t, h, admin+"/later-deadlines", url.Values{"changesDate": {"2030-03-17"}}); !strings.Contains(loc, "nothing+was+changed") {
		t.Errorf("not after the competition date: %s", loc)
	}
	redirected(t, h, admin+"/later-deadlines", url.Values{"changesDate": {"2030-03-16"}, "changesTime": {"09:00"}, "signoffsDate": {"2030-03-16"}})
	redirected(t, h, admin+"/deadline", url.Values{"close": {"1"}})

	// New entries closed; Ivy can still change hers, and the page says until when.
	if page := do(t, h, http.MethodGet, enter, nil).Body.String(); strings.Contains(page, `name="gymnast"`) {
		t.Error("no new entries")
	}
	page := do(t, h, http.MethodGet, ivy, nil).Body.String()
	if !strings.Contains(page, "Change the entry") || !strings.Contains(page, "Changes to entries until Saturday 16 March 2030, 09:00") {
		t.Errorf("Ivy can change hers until changes close: %s", page)
	}

	// Ann signs X off after the deadline: it reaches the competition without sending again.
	home := do(t, h, http.MethodGet, ann, nil).Body.String()
	open := regexp.MustCompile(`href="(` + regexp.QuoteMeta(ann) + `/members/[^"]+)"`).FindStringSubmatch(home)
	if open == nil {
		t.Fatal("Ann sees X's entry")
	}
	redirected(t, h, strings.ReplaceAll(open[1], "&amp;", "&"), url.Values{"signed": {"1"}})
	dash = do(t, h, http.MethodGet, admin, nil).Body.String()
	xEntry := regexp.MustCompile(`href="(` + regexp.QuoteMeta(admin) + `/entries/[^"]+)"[^>]*>X<`).FindStringSubmatch(dash)
	if xEntry == nil {
		t.Fatal("X's entry on the dashboard")
	}
	if page := do(t, h, http.MethodGet, xEntry[1], nil).Body.String(); !strings.Contains(page, "Signed off by Ann") {
		t.Errorf("X is signed off at the competition: %s", page)
	}
}
