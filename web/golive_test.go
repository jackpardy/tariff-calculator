package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestGoingLive(t *testing.T) {
	h := competitionServer(t)
	form := newCompetition()
	form.Del("opens") // private: the default
	admin := created(t, h, form)
	page := do(t, h, http.MethodGet, admin, nil).Body.String()
	if !strings.Contains(page, "<strong>Private.</strong>") || !strings.Contains(page, "Go live now") || !strings.Contains(page, "Entries aren't open at the moment.") {
		t.Error("a new competition starts private, with a way to go live")
	}
	enter := pathIn(t, page, "/competitions/enter/")
	ann := url.Values{"gymnast": {"Ann Ryan"}, "level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}}

	// The entry link shows the competition, and takes no entries.
	if got := do(t, h, http.MethodGet, enter, nil).Body.String(); !strings.Contains(got, "Student Open") || !strings.Contains(got, "Entries aren't open at the moment.") || strings.Contains(got, `name="gymnast"`) {
		t.Error("the entry link shows the competition, without the form")
	}
	if rec := do(t, h, http.MethodPost, enter, ann); rec.Code != http.StatusConflict {
		t.Errorf("entering a private competition: %d", rec.Code)
	}

	// Going live at a set time: still not open, and the links say when.
	redirected(t, h, admin+"/live", url.Values{"opens": {"at"}, "liveDate": {"2030-03-01"}, "liveTime": {"09:00"}})
	if got := do(t, h, http.MethodGet, enter, nil).Body.String(); !strings.Contains(got, "Entries open Friday 1 March 2030, 09:00") {
		t.Error("the entry link says when entries open")
	}
	if got := do(t, h, http.MethodGet, admin, nil).Body.String(); !strings.Contains(got, "It goes live Friday 1 March 2030, 09:00") || !strings.Contains(got, "stay private") {
		t.Error("the organiser sees when it goes live, and can call it off")
	}
	if rec := do(t, h, http.MethodPost, admin+"/live", url.Values{"opens": {"at"}, "liveDate": {"2030-03-10"}, "liveTime": {"09:00"}}); !strings.Contains(rec.Header().Get("Location"), url.QueryEscape("Entries must open before they close")) {
		t.Errorf("going live after the deadline: %s", rec.Header().Get("Location"))
	}

	// Live now: entries open.
	redirected(t, h, admin+"/live", url.Values{"opens": {"now"}})
	if rec := do(t, h, http.MethodPost, enter, ann); rec.Code != http.StatusSeeOther {
		t.Fatalf("entering once live: %d", rec.Code)
	}
	if got := do(t, h, http.MethodGet, admin, nil).Body.String(); strings.Contains(got, "Go live now") || !strings.Contains(got, "Make private (pause entries)") {
		t.Error("once live, the organiser can make it private again")
	}

	// Private again: entries paused, those made kept.
	redirected(t, h, admin+"/live", url.Values{"opens": {"private"}})
	if rec := do(t, h, http.MethodPost, enter, ann); rec.Code != http.StatusConflict {
		t.Errorf("entering while paused: %d", rec.Code)
	}
	if got := do(t, h, http.MethodGet, admin, nil).Body.String(); !strings.Contains(got, "Ann Ryan") {
		t.Error("entries made are kept")
	}
}

func TestCreatingGoesLiveLater(t *testing.T) {
	h := competitionServer(t)
	form := newCompetition()
	form.Set("opens", "at")
	form.Set("liveDate", "2020-01-01")
	form.Set("liveTime", "09:00")
	if rec := do(t, h, http.MethodPost, "/competitions", form); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "already passed") {
		t.Errorf("going live in the past: %d", rec.Code)
	}
	form.Set("liveDate", "2030-02-01")
	admin := created(t, h, form)
	if got := do(t, h, http.MethodGet, admin, nil).Body.String(); !strings.Contains(got, "Entries open Friday 1 February 2030, 09:00") {
		t.Error("goes live at the time set")
	}
}
