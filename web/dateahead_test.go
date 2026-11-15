package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// A competition can't be made, or copied, with a first day more than two
// years ahead: its people would be kept for as long. A copy's default (a
// year on) is fine.
func TestDateTooFarAhead(t *testing.T) {
	s := newTestServer(t, atTime(dayBefore))
	s.p.yearsAhead = 2
	h := s.h
	form := newCompetition()
	form.Set("date", "2033-03-16")
	rec := do(t, h, http.MethodPost, "/competitions", form)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "The date is more than two years away: check the year.") {
		t.Errorf("a far date: %d", rec.Code)
	}
	admin := created(t, h, newCompetition())
	rec = do(t, h, http.MethodPost, admin+"/copy", url.Values{"as": {"setup"}, "name": {"Far"}, "date": {"2040-03-15"}})
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(unescape(rec.Body.String()), "more than two years away") {
		t.Errorf("a far copy: %d", rec.Code)
	}
	ask := do(t, h, http.MethodGet, admin+"/copy?as=setup", nil).Body.String()
	if !strings.Contains(ask, `value="2031-03-16"`) {
		t.Error("a copy's default date, a year on, is offered")
	}
	rec = do(t, h, http.MethodPost, admin+"/copy", url.Values{"as": {"setup"}, "name": {"Next"}, "date": {"2031-03-16"}})
	if rec.Code != http.StatusSeeOther {
		t.Errorf("the default copy: %d", rec.Code)
	}
}
