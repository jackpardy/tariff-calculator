package web

import (
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// compIDOf is the id of a club's first competition.
func compIDOf(t *testing.T, h http.Handler, club string) string {
	t.Helper()
	return regexp.MustCompile(regexp.QuoteMeta(club) + `/competitions/([^"/?]+)"`).FindStringSubmatch(do(t, h, http.MethodGet, club, nil).Body.String())[1]
}

func TestTakeOutOfACompetition(t *testing.T) {
	h, clock := competitionServerAt(t, dayBefore)
	admin := created(t, h, newCompetition())
	c := aClub(t, h, admin, "UCD", true, "X", "Y")
	comp := compIDOf(t, h, c.page)
	ids := memberIDs(t, h, c.page)
	for _, m := range []string{"X", "Y"} {
		redirected(t, h, c.page+"/members/"+ids[m]+"/competitions/"+comp+"/offer", url.Values{"marshal": {"1"}})
	}
	redirected(t, h, formAction(t, clubEntriesPage(t, h, c.page), c.page+"/competitions/"), url.Values{"which": {"all"}})
	redirected(t, h, admin+"/fees/settings", url.Values{"fee-": {"20"}, "charge_removed": {"1"}, "removed_fee": {"5"}})
	takeOut := c.page + "/competitions/" + comp + "/take-out"
	if page := clubEntriesPage(t, h, c.page); !strings.Contains(page, "Take out of this competition") || !strings.Contains(page, `action="`+takeOut+`"`) {
		t.Fatal("the club's page offers taking a member out")
	}

	// Their offer only: asked first; then the organiser sees it taken back.
	body := html.UnescapeString(do(t, h, http.MethodPost, takeOut, url.Values{"member": {ids["Y"]}, "what": {"offer"}}).Body.String())
	body = body[strings.Index(body, `id="ask"`):][:900]
	if !strings.Contains(body, "Take Y out of Student Open?") || !strings.Contains(body, "Their offer to judge or help comes out") || strings.Contains(body, "Their entries") {
		t.Errorf("the ask: %s", body)
	}
	to := redirected(t, h, takeOut, url.Values{"member": {ids["Y"]}, "what": {"offer"}, "anyway": {"1"}})
	if noticeOf(t, to) != "Took Y's offer to judge or help out of Student Open." {
		t.Errorf("notice: %s", to)
	}
	officials := do(t, h, http.MethodGet, admin+"/officials", nil).Body.String()
	if !strings.Contains(officials, "Offers taken back by their club") || !strings.Contains(officials, "Offer taken back by the club") {
		t.Error("the Officials page lists Y, taken back")
	}
	if people := do(t, h, http.MethodGet, admin+"/people", nil).Body.String(); !strings.Contains(people, "offer taken back by the club") {
		t.Error("People says so on Y's row")
	}
	if dash := do(t, h, http.MethodGet, admin, nil).Body.String(); strings.Contains(dash, "Removed by the club") {
		t.Error("Y's entry is still in")
	}
	if !historyHas(t, h, admin, "UCD (Club admin)", "Y: offer to judge or help taken back by the club") {
		t.Error("the organiser's history")
	}

	// After entries close (changes still open), both: the ask says what it costs.
	redirected(t, h, admin+"/later-deadlines", url.Values{"changesDate": {"2030-03-16"}, "changesTime": {"09:00"}})
	clock.set(inDublin(2030, 3, 12, 10, 0))
	body = html.UnescapeString(do(t, h, http.MethodPost, takeOut, url.Values{"member": {ids["X"]}, "what": {"both"}}).Body.String())
	if !strings.Contains(body, "Their entry at Student Open was sent and entries have closed: the organiser charges €25 for it.") || !strings.Contains(body, "Take out both") {
		t.Errorf("the ask after the close: %s", body)
	}
	redirected(t, h, takeOut, url.Values{"member": {ids["X"]}, "what": {"both"}, "anyway": {"1"}})
	if dash := do(t, h, http.MethodGet, admin, nil).Body.String(); !strings.Contains(dash, "Removed by the club") || !strings.Contains(dash, "entry removed by its club") {
		t.Error("X's entry is removed by the club, after the close")
	}
	fees := feeAccounts(t, do(t, h, http.MethodGet, admin+"/fees", nil).Body.String())["UCD"]
	inv := invoiceOf(t, do(t, h, http.MethodGet, fees.Invoice, nil).Body.String())
	if !strings.Contains(strings.Join(inv.Lines, "|"), "X · BUCS L3 (removed after entries closed): €20|Removed after entries closed: X · BUCS L3: €5") {
		t.Errorf("the invoice: %+v", inv)
	}

	// Once changes close, the club can't: the page says so, and a post is turned down.
	clock.set(inDublin(2030, 3, 16, 10, 0))
	if page := clubEntriesPage(t, h, c.page); strings.Contains(page, "Take out of this competition") && !strings.Contains(page, "can't take anyone out") {
		t.Error("closed: no control")
	}
	if to := redirected(t, h, takeOut, url.Values{"member": {ids["Y"]}, "what": {"entries"}, "anyway": {"1"}}); !strings.Contains(noticeOf(t, to), "Nothing was changed") {
		t.Errorf("turned down: %s", to)
	}
}

func TestRemovingTakesOffersToo(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	c := aClub(t, h, admin, "UCD", true, "X")
	comp := compIDOf(t, h, c.page)
	ids := memberIDs(t, h, c.page)
	redirected(t, h, c.page+"/members/"+ids["X"]+"/competitions/"+comp+"/offer", url.Values{"marshal": {"1"}})
	redirected(t, h, formAction(t, clubEntriesPage(t, h, c.page), c.page+"/competitions/"), url.Values{"which": {"all"}})
	redirected(t, h, c.page+"/members/"+ids["X"]+"/remove", url.Values{"anyway": {"1"}})
	people := do(t, h, http.MethodGet, admin+"/people", nil).Body.String()
	if n := strings.Count(people, ">X</a>"); n != 1 || !strings.Contains(people, "BUCS L3 · removed by the club · offer taken back by the club") {
		t.Errorf("one row for X, saying both (%d): %s", n, people)
	}
	if officials := do(t, h, http.MethodGet, admin+"/officials", nil).Body.String(); !strings.Contains(officials, "Offers taken back by their club") {
		t.Error("the Officials page lists X's offer, taken back")
	}
}

// A pair's entry whose partner the partner's club removes is flagged for
// the organiser, the entrant and their club, and counts as a problem.
func TestPartnerRemovedFlag(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, eventsCompetition())
	clubLink := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/club/")
	ucd := withoutQuery(redirected(t, h, "/clubs", url.Values{"name": {"UCD"}}))
	redirected(t, h, clubLink, url.Values{"clubAdmin": {ucd}})
	a := withoutQuery(redirected(t, h, pathIn(t, do(t, h, http.MethodGet, ucd+"/members", nil).Body.String(), "/clubs/join/"), url.Values{"name": {"A"}}))
	dcu := withoutQuery(redirected(t, h, "/clubs", url.Values{"name": {"DCU"}}))
	b := withoutQuery(redirected(t, h, pathIn(t, do(t, h, http.MethodGet, dcu+"/members", nil).Body.String(), "/clubs/join/"), url.Values{"name": {"Adam Brown"}}))
	save := formAction(t, memberCompetitionPage(t, h, a), a+"/competitions/")
	redirected(t, h, save, url.Values{"discipline": {"synchro"}, "level": {"BUCS L3"}, "partnerName": {"Adam Brown"}, "partnerClub": {"DCU"},
		"ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}})
	redirected(t, h, pathIn(t, memberCompetitionPage(t, h, a), "/competitions/partner/"), url.Values{"link": {"http://example.com" + b}})
	redirected(t, h, formAction(t, clubEntriesPage(t, h, ucd), ucd+"/competitions/"), url.Values{"which": {"all"}})

	redirected(t, h, dcu+"/members/"+memberIDs(t, h, dcu)["Adam"]+"/remove", url.Values{"anyway": {"1"}})
	line := "Adam Brown was removed by DCU on "
	dash := html.UnescapeString(do(t, h, http.MethodGet, admin, nil).Body.String())
	if !strings.Contains(dash, line) || !strings.Contains(dash, "1 pair may need a new partner") {
		t.Errorf("the organiser's entries and needs-attention line")
	}
	if problems := html.UnescapeString(do(t, h, http.MethodGet, admin+"?problems=1", nil).Body.String()); !strings.Contains(problems, "this pair may need a new partner") {
		t.Error("it counts as a problem, so the filter finds it")
	}
	e := regexp.MustCompile(`<li><a href="(` + regexp.QuoteMeta(admin) + `/entries/[^"]+)">A & Adam Brown`).FindStringSubmatch(dash)
	if e == nil {
		t.Fatal("the pair's line links to it")
	}
	if page := html.UnescapeString(do(t, h, http.MethodGet, e[1], nil).Body.String()); !strings.Contains(page, line) {
		t.Error("the entry's page")
	}
	them := "Your partner Adam Brown is no longer a member of DCU: tell the organiser, or change your partner"
	if page := html.UnescapeString(memberCompetitionPage(t, h, a)); !strings.Contains(page, them) {
		t.Error("the entrant's page")
	}
	if page := html.UnescapeString(clubEntriesPage(t, h, ucd)); !strings.Contains(page, them) {
		t.Error("their club's row")
	}
	if !strings.Contains(do(t, h, http.MethodGet, admin+"/history", nil).Body.String(), "flagged: it may need a new partner") {
		t.Error("the history")
	}
}
