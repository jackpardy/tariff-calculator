package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// Once a member's change can't be sent, their coach signs off what the
// competition holds, with the change shown apart and not signable; the
// sign-off is recorded against the held entry, and a form made before that
// isn't taken for it.
func TestCoachSignsOffWhatIsHeld(t *testing.T) {
	s := newUnsentShop(t, func(f url.Values) { f.Set("signoff", "1") })
	open := func() (page, path string) {
		home := do(t, s.h, http.MethodGet, s.coach, nil).Body.String()
		m := regexp.MustCompile(`href="(` + regexp.QuoteMeta(s.coach) + `/members/[^"]+)"[^>]*>Gus<`).FindStringSubmatch(home)
		if m == nil {
			t.Fatalf("Ann sees Gus: %s", home)
		}
		path = strings.ReplaceAll(m[1], "&amp;", "&")
		return do(t, s.h, http.MethodGet, path, nil).Body.String(), path
	}

	// Before the close: as ever.
	page, path := open()
	if !strings.Contains(page, "BUCS L4") || strings.Contains(page, `name="held"`) || said(page, "wasn't sent") {
		t.Errorf("before close the page is as it was: %s", page)
	}

	redirected(t, s.h, s.admin+"/later-deadlines", url.Values{"signoffsDate": {"2030-03-16"}})
	redirected(t, s.h, s.admin+"/late/settings", url.Values{"late-level": {"1"}, "fee-level": {"10"}})
	s.clock.set(afterDeadline)
	page, path = open()
	if !said(page, "Gus changed this to BUCS L4 on 6 Feb, but it wasn't sent before entries closed. The competition has BUCS L3.") ||
		!strings.Contains(page, `name="held"`) || !said(page, "What Gus changed it to, not sent") || !said(page, "Gus can ask for a late change") {
		t.Fatalf("the held entry to sign, the change apart: %s", page)
	}
	if !strings.Contains(page, "· BUCS L3") {
		t.Errorf("the entry shown is BUCS L3: %s", page)
	}
	held := regexp.MustCompile(`name="held" value="([^"]+)"`).FindStringSubmatch(page)[1]

	// A form that doesn't carry what was shown isn't signed.
	loc := redirected(t, s.h, path, url.Values{"signed": {"1"}})
	if !said(noticeOf(t, loc), "This entry changed since you opened this page; nothing was changed.") {
		t.Errorf("stale form: %s", loc)
	}
	redirected(t, s.h, path, url.Values{"signed": {"1"}, "held": {"other@1"}})
	dash := do(t, s.h, http.MethodGet, s.admin, nil).Body.String()
	gus := regexp.MustCompile(`href="(` + regexp.QuoteMeta(s.admin) + `/entries/[^"]+)"[^>]*>Gus<`).FindStringSubmatch(dash)
	if gus == nil {
		t.Fatalf("Gus on the dashboard: %s", dash)
	}
	if signedOffBy(t, do(t, s.h, http.MethodGet, gus[1], nil).Body.String(), "Ann") {
		t.Error("nothing was signed")
	}

	redirected(t, s.h, path, url.Values{"signed": {"1"}, "held": {held}})
	entry := do(t, s.h, http.MethodGet, gus[1], nil).Body.String()
	if !signedOffBy(t, entry, "Ann") || !strings.Contains(entry, "BUCS L3") {
		t.Errorf("the organiser's entry is signed off at the held level: %s", entry)
	}
	if page, _ := open(); !said(page, "wasn't sent before entries closed") {
		t.Error("the change is still shown apart")
	}

	// Reopened, the member's version is the entry again, and isn't signed.
	s.clock.set(beforeDeadline)
	if page, _ := open(); strings.Contains(page, `name="held"`) || said(page, "wasn't sent") || !strings.Contains(page, "BUCS L4") {
		t.Errorf("reopened: the page is as it was: %s", page)
	}
}
