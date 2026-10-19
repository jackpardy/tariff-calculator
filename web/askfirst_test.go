package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// Turning on coach approval, where entries are signed off by coaches not
// approved, asks first, saying how many entries it leaves waiting; made
// anyway, the notice says so too.
func TestCoachApprovalAsksFirst(t *testing.T) {
	h := competitionServer(t)
	form := newCompetition()
	form.Set("signoff", "1")
	admin := created(t, h, form)
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	for _, name := range []string{"Mary", "Nia", "Orla"} {
		own := withoutQuery(redirected(t, h, enter, bucsEntry(name)))
		if name == "Orla" {
			continue // not signed off: she's waiting already
		}
		signoff := pathIn(t, do(t, h, http.MethodGet, own, nil).Body.String(), "/competitions/signoff/")
		redirected(t, h, signoff, url.Values{"coach": {"Ann"}, "signed": {"1"}})
	}

	ask := do(t, h, http.MethodPost, admin+"/approve-coaches", url.Values{"on": {"1"}})
	body := ask.Body.String()
	if ask.Code != http.StatusOK || !strings.Contains(body, "Turn on coach approval") || !strings.Contains(body, "This makes 2 entries wait for a coach&#39;s sign-off.") ||
		!strings.Contains(body, `<input type="hidden" name="on" value="1">`) || !strings.Contains(body, `name="anyway" value="1"`) || !strings.Contains(body, ">Turn it on</button>") {
		t.Fatalf("asks first, with the count: %d %s", ask.Code, body)
	}
	if strings.Contains(do(t, h, http.MethodGet, admin, nil).Body.String(), `href="`+admin+`/coaches"`) {
		t.Error("not turned on until asked")
	}
	to := redirected(t, h, admin+"/approve-coaches", url.Values{"on": {"1"}, "anyway": {"1"}})
	if notice := noticeOf(t, to); !strings.Contains(notice, "Only coaches you approve can sign off") || !strings.Contains(notice, "Waiting for a coach's sign-off until then: 2 entries.") {
		t.Errorf("turned on, saying how many wait: %q", notice)
	}
	// Turned off and on again, it asks again: the same two would wait.
	redirected(t, h, admin+"/approve-coaches", url.Values{})
	if rec := do(t, h, http.MethodPost, admin+"/approve-coaches", url.Values{"on": {"1"}}); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "This makes 2 entries wait") {
		t.Errorf("asks again: %d", rec.Code)
	}
	// Saving the levels while it's on asks nothing.
	redirected(t, h, admin+"/approve-coaches", url.Values{"on": {"1"}, "anyway": {"1"}})
	if to := redirected(t, h, admin+"/approve-coaches", url.Values{"on": {"1"}, "level-trampoline": {"3"}}); strings.Contains(to, "until+then") {
		t.Errorf("already on: %s", to)
	}
}
