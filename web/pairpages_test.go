package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// A synchro pair from two clubs: once B confirms, B, B's coach and B's club
// see the entry A made, though B's club isn't entered.
func TestPartnerSeesThePair(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, eventsCompetition())
	clubLink := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/club/")
	ucd := withoutQuery(redirected(t, h, "/clubs", url.Values{"name": {"UCD"}}))
	redirected(t, h, clubLink, url.Values{"clubAdmin": {ucd}})
	a := withoutQuery(redirected(t, h, pathIn(t, do(t, h, http.MethodGet, ucd+"/members", nil).Body.String(), "/clubs/join/"), url.Values{"name": {"A"}}))
	dcu := withoutQuery(redirected(t, h, "/clubs", url.Values{"name": {"DCU"}}))
	dee := pathIn(t, do(t, h, http.MethodPost, dcu+"/coaches", url.Values{"name": {"Dee"}}).Body.String(), "/clubs/coach/")
	b := withoutQuery(redirected(t, h, pathIn(t, do(t, h, http.MethodGet, dcu+"/members", nil).Body.String(), "/clubs/join/"), url.Values{"name": {"B"}}))

	save := formAction(t, memberCompetitionPage(t, h, a), a+"/competitions/")
	redirected(t, h, save, url.Values{"discipline": {"synchro"}, "level": {"BUCS L3"}, "partnerName": {"B"}, "partnerClub": {"DCU"},
		"ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}})
	if strings.Contains(do(t, h, http.MethodGet, b, nil).Body.String(), "Synchro as a partner") {
		t.Error("nothing for B before confirming")
	}
	partner := pathIn(t, memberCompetitionPage(t, h, a), "/competitions/partner/")
	redirected(t, h, partner, url.Values{"link": {"http://example.com" + b}})

	for who, path := range map[string]string{"B": b, "B's coach": dee, "B's club": dcu + "/members"} {
		page := do(t, h, http.MethodGet, path, nil).Body.String()
		if !strings.Contains(page, "Synchro as a partner") || !strings.Contains(page, "Synchro BUCS L3 · A and B") || !strings.Contains(page, "Entered by A (UCD)") || !strings.Contains(page, "Not sent to the competition yet") {
			t.Errorf("%s sees the pair's entry", who)
		}
	}
	if strings.Contains(do(t, h, http.MethodGet, ucd, nil).Body.String(), "Synchro as a partner") {
		t.Error("UCD sees A's entry as its own already")
	}

	// Sent by UCD: B sees it's sent.
	redirected(t, h, formAction(t, clubEntriesPage(t, h, ucd), ucd+"/competitions/"), url.Values{"which": {"all"}})
	if page := do(t, h, http.MethodGet, b, nil).Body.String(); !strings.Contains(page, "Sent by your club") {
		t.Error("B sees it sent")
	}

	// Entered in the competition too, DCU sees it with its own entries.
	redirected(t, h, clubLink, url.Values{"clubAdmin": {dcu}})
	if page := clubEntriesPage(t, h, dcu); !strings.Contains(page, "Synchro as a partner") || !strings.Contains(page, "Synchro BUCS L3 · A and B") {
		t.Error("DCU's entries list the pair B is in")
	}
	if strings.Contains(clubEntriesPage(t, h, ucd), "Synchro as a partner") {
		t.Error("not UCD's, whose own entry it is")
	}
}
