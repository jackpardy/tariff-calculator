package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// Someone entering on their own in two disciplines pays as one: one row in
// Fees, both entries on it.
func TestFeesOnePersonOneAccount(t *testing.T) {
	h := competitionServer(t)
	form := newCompetition()
	form.Set("tumbling", "Novice")
	admin := created(t, h, form)
	redirected(t, h, admin+"/fees/settings", url.Values{"fee-": {"12"}, "fee-tumbling": {"10"}})
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	first := redirected(t, h, enter, bucsEntry("Róisín Brennan"))
	link := pathIn(t, do(t, h, http.MethodGet, first, nil).Body.String(), "/competitions/gymnast/")
	redirected(t, h, enter, url.Values{"gymnast": {"Róisín Brennan"}, "discipline": {"tumbling"}, "level": {"Novice"}, "as": {"http://example.com" + link + "/details"}})
	page := do(t, h, http.MethodGet, admin+"/fees", nil).Body.String()
	if n := strings.Count(page, "<td>Róisín Brennan (on their own)"); n != 1 {
		t.Fatalf("one row for her, not %d", n)
	}
	if got := feeAccounts(t, page)["Róisín Brennan (on their own)"]; got.Due != "€22" {
		t.Errorf("both entries on it: %+v", got)
	}
}
