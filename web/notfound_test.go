package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// An address with nothing at it, a day the timetable hasn't or a sheet
// that isn't one is a page saying so, as the app's pages look, not Go's
// bare "404 page not found" or another day's or sheet's page.
func TestNothingThere(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	redirected(t, h, enter, bucsEntry("Ann"))
	redirected(t, h, admin+"/timetable/plan", url.Values{})
	redirected(t, h, admin+"/timetable/publish", url.Values{"action": {"publish"}})

	for _, path := range []string{"/nowhere", admin + "/nowhere", "/clubs/admin/x/y/z"} {
		rec := do(t, h, http.MethodGet, path, nil)
		if body := rec.Body.String(); rec.Code != http.StatusNotFound || !strings.Contains(body, "Page not found") || !strings.Contains(body, `href="/competitions"`) {
			t.Errorf("%s: %d %s", path, rec.Code, body)
		}
		if rec.Header().Get("Referrer-Policy") != "no-referrer" || rec.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("%s may hold a link's secret: %v", path, rec.Header())
		}
	}
	if rec := do(t, h, http.MethodGet, "/routine", nil); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("an address that only takes a POST: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, admin+"/day?day=9", nil); rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "No such day") {
		t.Errorf("a day the timetable hasn't: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, admin+"/day?day=0", nil); rec.Code != http.StatusOK {
		t.Errorf("a day it has: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, admin+"/timetable/print?sheet=zzz", nil); rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "No such sheet") {
		t.Errorf("a sheet that isn't one: %d", rec.Code)
	}
	for _, sheet := range []string{"", "marshal", "judges", "scores", "rota"} {
		if rec := do(t, h, http.MethodGet, admin+"/timetable/print?sheet="+sheet, nil); rec.Code != http.StatusOK {
			t.Errorf("sheet %q: %d", sheet, rec.Code)
		}
	}
}
