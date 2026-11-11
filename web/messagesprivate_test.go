package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// A message to the organiser shares the concerns table with helpers'
// flags, but is shown only to links that may resolve concerns: the
// organiser's and the everything links. The helper links see no text, reply,
// note or count of it, and can't answer it; the flags they raise stay seen.
func TestMessagesPrivate(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	ucd := aClub(t, h, admin, "UCD", true, "Ann", "Cian")
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	redirected(t, h, enter, bucsEntry("Ann Ryan"))

	co := madeLink(t, h, admin, "Mary", "everything")
	cards := madeLink(t, h, admin, "Difficulty judges", "cards")
	chair := madeLink(t, h, admin, "Chairs", "chair")
	timetable := madeLink(t, h, admin, "Timetable team", "timetable")
	screen := madeLink(t, h, admin, "Screen", "screen")
	helpers := map[string]string{"cards": cards, "chair": chair, "timetable": timetable}

	// A helper flags a concern: everyone but the screen link sees it.
	redirected(t, h, cards+"/concerns", url.Values{"text": {"FLAGGED-BY-HELPER"}})

	ann := withoutQuery(formAction(t, memberCompetitionPage(t, h, ucd.members["Ann"]), ucd.members["Ann"]+"/competitions/"))
	action, fields, _, _ := tellFormOf(t, do(t, h, http.MethodGet, ann, nil).Body.String())
	fields.Set("text", "PRIVATE-MESSAGE")
	redirected(t, h, action, fields)
	id := regexp.MustCompile(`id="concern-([^"]+)"`).FindStringSubmatch(do(t, h, http.MethodGet, admin, nil).Body.String())[1]

	// The organiser replies and resolves with a note of their own.
	redirected(t, h, admin+"/concerns/"+id+"/reply", url.Values{"reply": {"PRIVATE-REPLY"}})
	redirected(t, h, admin+"/concerns/"+id+"/resolve", url.Values{"note": {"PRIVATE-NOTE"}})

	// Each link may reply and resolve if it can resolve; the page it sees.
	for name, link := range map[string]string{"organiser": admin, "everything": co} {
		page := unescape(do(t, h, http.MethodGet, link, nil).Body.String())
		for _, s := range []string{"PRIVATE-MESSAGE", "PRIVATE-REPLY", "PRIVATE-NOTE", "FLAGGED-BY-HELPER"} {
			if !strings.Contains(page, s) {
				t.Errorf("%s sees %s", name, s)
			}
		}
	}
	// The co-organiser can answer a message too.
	if rec := do(t, h, http.MethodPost, co+"/concerns/"+id+"/reply", url.Values{"reply": {"CO-REPLY"}}); rec.Code != http.StatusSeeOther {
		t.Errorf("everything replies: %d", rec.Code)
	}

	for name, link := range helpers {
		page := unescape(do(t, h, http.MethodGet, link, nil).Body.String())
		for _, s := range []string{"PRIVATE-MESSAGE", "PRIVATE-REPLY", "PRIVATE-NOTE", "CO-REPLY", "Ann, a member of UCD"} {
			if strings.Contains(page, s) {
				t.Errorf("%s sees %s", name, s)
			}
		}
		if !strings.Contains(page, "FLAGGED-BY-HELPER") {
			t.Errorf("%s still sees the flagged concern", name)
		}
		// Only the one flag is counted.
		if !strings.Contains(page, "1 message or concern open") || strings.Contains(page, "Waiting for you") {
			t.Errorf("%s sees a message in the count", name)
		}
		for _, p := range []string{"/concerns/" + id + "/reply", "/concerns/" + id + "/received", "/concerns/" + id + "/resolve"} {
			if rec := do(t, h, http.MethodPost, link+p, url.Values{"reply": {"x"}}); rec.Code != http.StatusForbidden {
				t.Errorf("%s posts %s: %d", name, p, rec.Code)
			}
		}
		if rec := do(t, h, http.MethodGet, link+"/concerns/"+id, nil); rec.Code == http.StatusOK || strings.Contains(rec.Body.String(), "PRIVATE") {
			t.Errorf("%s gets the message's thread: %d", name, rec.Code)
		}
		// Nor does the history say more than that something was done.
		if rec := do(t, h, http.MethodGet, link+"/history", nil); strings.Contains(rec.Body.String(), "PRIVATE") {
			t.Errorf("%s: the history quotes it", name)
		}
	}
	// The venue screen link has no dashboard at all.
	if rec := do(t, h, http.MethodGet, screen, nil); rec.Code == http.StatusOK || strings.Contains(rec.Body.String(), "PRIVATE") {
		t.Errorf("screen link's dashboard: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, screen+"/concerns/"+id+"/reply", url.Values{"reply": {"x"}}); rec.Code != http.StatusForbidden {
		t.Errorf("screen link replies: %d", rec.Code)
	}
}
