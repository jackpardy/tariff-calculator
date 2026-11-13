package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The individual link is public, so one address can make only so many
// entries through it in an hour, asking to enter late included.
func TestEnteringThroughTheIndividualLinkIsLimited(t *testing.T) {
	for _, late := range []bool{false, true} {
		name := "entering"
		if late {
			name = "asking to enter late"
		}
		t.Run(name, func(t *testing.T) {
			s := newTestServer(t, atTime(dayBefore))
			admin := created(t, s.h, newCompetition())
			enter := pathIn(t, do(t, s.h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
			if late {
				redirected(t, s.h, admin+"/late/settings", url.Values{"late-new": {"1"}})
				s.clock.set(afterDeadline) // new entries have closed
			}
			from := func(addr, gymnast string) *httptest.ResponseRecorder {
				req := httptest.NewRequest(http.MethodPost, enter, strings.NewReader(bucsEntry(gymnast).Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				req.RemoteAddr = addr
				rec := httptest.NewRecorder()
				s.h.ServeHTTP(rec, req)
				return rec
			}
			// Mistakes in the form don't use any up.
			for range 3 {
				if rec := from("203.0.113.9:1", ""); rec.Code != http.StatusUnprocessableEntity {
					t.Fatalf("an entry with no name: %d", rec.Code)
				}
			}
			for i := range maxIndividualEntriesPerHour {
				if rec := from("203.0.113.9:1", "Gymnast "+strconv.Itoa(i)); rec.Code != http.StatusSeeOther {
					t.Fatalf("entry %d: %d %s", i+1, rec.Code, rec.Body.String())
				}
			}
			rec := from("203.0.113.9:1", "One more")
			if rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), "This address has made many entries in the last hour") {
				t.Errorf("one address can make %d an hour: %d", maxIndividualEntriesPerHour, rec.Code)
			}
			if rec := from("203.0.113.10:1", "Someone else"); rec.Code != http.StatusSeeOther {
				t.Errorf("another address: %d", rec.Code)
			}
			s.clock.set(s.clock.now().Add(time.Hour + time.Minute))
			if rec := from("203.0.113.9:1", "Later"); rec.Code != http.StatusSeeOther {
				t.Errorf("an hour later: %d", rec.Code)
			}
		})
	}
}
