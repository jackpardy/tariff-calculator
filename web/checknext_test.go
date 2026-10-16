package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// Mark checked and next opens the next entry to check, in the same level
// first; after the last, the entries page says so. Video OK waits for a video.
func TestCheckAndNext(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	for _, name := range []string{"Ann", "Bea"} {
		redirected(t, h, enter, bucsEntry(name))
	}
	page := do(t, h, http.MethodGet, admin, nil).Body.String()
	ids := regexp.MustCompile(`href="`+regexp.QuoteMeta(admin)+`/entries/([^"#]+)"`).FindAllStringSubmatch(page, -1)
	if len(ids) < 2 {
		t.Fatalf("two entries: %s", page)
	}
	first, second := ids[0][1], ids[1][1]
	if first == second {
		second = ids[2][1]
	}
	to := redirected(t, h, admin+"/entries/"+first+"/check", url.Values{"checked": {"1"}, "next": {"1"}})
	if !strings.Contains(to, "/entries/"+second+"?notice=") && !strings.Contains(to, "/entries/"+second+"#") && !strings.Contains(to, "/entries/"+second) {
		t.Errorf("on to the next: %s", to)
	}
	if to := redirected(t, h, admin+"/entries/"+second+"/check", url.Values{"checked": {"1"}, "next": {"1"}}); !strings.Contains(to, "last+to+check") {
		t.Errorf("after the last: %s", to)
	}
	if to := redirected(t, h, admin+"/entries/"+first+"/video", url.Values{"review": {"ok"}}); !strings.Contains(to, "no+video+yet") {
		t.Errorf("no video to say OK to: %s", to)
	}
}
