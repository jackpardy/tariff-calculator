package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"tariffCalculator/views"
)

// An offer to judge or help is something to send: the club page counts it,
// the Judging and helping tab and the member's page say whether it has gone,
// and the organiser sees it only once the club sends.
func TestOffersToSend(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	dash := do(t, h, http.MethodGet, admin+"/settings", nil).Body.String()
	officials := admin + "/officials"
	club := withoutQuery(redirected(t, h, "/clubs", url.Values{"name": {"UCD"}}))
	redirected(t, h, pathIn(t, dash, "/competitions/club/"), url.Values{"clubAdmin": {club}})
	join := pathIn(t, do(t, h, http.MethodGet, club+"/members", nil).Body.String(), "/clubs/join/")
	a := withoutQuery(redirected(t, h, join, url.Values{"name": {"A"}}))
	b := withoutQuery(redirected(t, h, join, url.Values{"name": {"B"}}))

	// The club sends an entry first.
	entry := url.Values{"level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}}
	redirected(t, h, formAction(t, memberCompetitionPage(t, h, a), a+"/competitions/"), entry)
	page := clubEntriesPage(t, h, club)
	location := redirected(t, h, formAction(t, page, club+"/competitions/"), url.Values{"which": {"changed"}})
	if n := noticeOf(t, location); n != "Sent 1 entry to Student Open." {
		t.Errorf("entry alone: %q", n)
	}
	if page := clubEntriesPage(t, h, club); !strings.Contains(page, "All sent") {
		t.Error("with nothing offered, all sent")
	}

	// Members save what they offer, after the entries went.
	offerPath := func(member string) string {
		return regexp.MustCompile(`action="(` + regexp.QuoteMeta(member) + `/competitions/[^"]+/offer)"`).FindStringSubmatch(memberCompetitionPage(t, h, member))[1]
	}
	judge := url.Values{"judge-trampoline": {"1"}, "execution-trampoline": {"BUCS L3"}, "difficulty-trampoline": {"BUCS L3"}, "chair-trampoline": {""}}
	redirected(t, h, offerPath(a), judge)
	redirected(t, h, offerPath(b), url.Values{"recorder": {"1"}})
	page = clubEntriesPage(t, h, club)
	if strings.Contains(page, "All sent") || !strings.Contains(page, "2 offers to send") || !strings.Contains(page, "Send new and changed (2)") {
		t.Error("two offers wait: the page says so and counts them")
	}
	if strings.Contains(page, "entries to send") || strings.Contains(page, "entry to send") {
		t.Error("no entries wait")
	}
	if home := do(t, h, http.MethodGet, club, nil).Body.String(); !strings.Contains(home, "2 to send") {
		t.Error("the club's home page counts them")
	}
	tab := clubCompetitionTab(t, h, club, "/offers")
	if strings.Count(tab, chip(t, views.NotYet, "Not sent")) != 2 {
		t.Error("the tab marks both offers Not sent")
	}
	if page := memberCompetitionPage(t, h, a); !strings.Contains(page, chip(t, views.NotYet, "Not sent")) || !strings.Contains(page, "Your club sends it with its entries.") {
		t.Error("the member sees their offer isn't sent")
	}
	if page := do(t, h, http.MethodGet, officials, nil).Body.String(); !strings.Contains(page, "Nobody yet.") {
		t.Error("the organiser sees nothing until the club sends")
	}

	// Send: the notice says offers, and the organiser has them.
	location = redirected(t, h, formAction(t, page, club+"/competitions/"), url.Values{"which": {"changed"}})
	if n := noticeOf(t, location); n != "Sent 2 offers to judge or help to Student Open." {
		t.Errorf("offers alone: %q", n)
	}
	if page := do(t, h, http.MethodGet, officials, nil).Body.String(); strings.Contains(page, "Nobody yet.") || !onList(t, page, "B") {
		t.Error("the organiser has the offers")
	}
	page = clubEntriesPage(t, h, club)
	if !strings.Contains(page, "All sent") || !strings.Contains(page, "Send new and changed (0)") {
		t.Error("all sent again")
	}
	if tab := clubCompetitionTab(t, h, club, "/offers"); strings.Count(tab, chip(t, views.InProgress, "Sent")) != 2 {
		t.Error("the tab marks both Sent")
	}
	if page := memberCompetitionPage(t, h, a); !strings.Contains(page, chip(t, views.InProgress, "Sent")) || strings.Contains(page, "Your club sends it with its entries.") {
		t.Error("the member sees their offer is sent")
	}

	// A changed offer, a withdrawn one and a changed entry go together.
	judge.Set("chair-trampoline", "BUCS L3")
	redirected(t, h, offerPath(a), judge)
	redirected(t, h, offerPath(b), url.Values{})
	entry.Set("ex1Option", "builtin:bucs-l3-option-2")
	redirected(t, h, formAction(t, memberCompetitionPage(t, h, a), a+"/competitions/"), entry)
	page = clubEntriesPage(t, h, club)
	if !strings.Contains(page, "1 entry to send") || !strings.Contains(page, "2 offers to send") || !strings.Contains(page, "Send new and changed (3)") {
		t.Error("the changed entry, the changed offer and the withdrawn one are counted")
	}
	tab = clubCompetitionTab(t, h, club, "/offers")
	if !strings.Contains(tab, chip(t, views.Attention, "Changed since sent")) || !strings.Contains(tab, "Withdrawn") {
		t.Error("the tab marks the changed and the withdrawn offers")
	}
	if page := memberCompetitionPage(t, h, a); !strings.Contains(page, chip(t, views.Attention, "Changed since sent")) {
		t.Error("the member sees their offer changed since sent")
	}
	if page := do(t, h, http.MethodGet, officials, nil).Body.String(); !onList(t, page, "B") {
		t.Error("the organiser still holds what was sent")
	}
	location = redirected(t, h, formAction(t, page, club+"/competitions/"), url.Values{"which": {"changed"}})
	if n := noticeOf(t, location); n != "Sent 1 entry and 2 offers to judge or help to Student Open." {
		t.Errorf("entries and offers: %q", n)
	}
	if page := do(t, h, http.MethodGet, officials, nil).Body.String(); onList(t, page, "B") || !onList(t, page, "A") {
		t.Error("the withdrawn offer is gone from the organiser's list")
	}
	if page := clubEntriesPage(t, h, club); !strings.Contains(page, "All sent") {
		t.Error("all sent at the end")
	}
}

// onList says whether someone has a row on the organiser's officials page.
func onList(t *testing.T, page, name string) bool {
	t.Helper()
	for _, row := range tableRows(t, page) {
		if len(row) > 0 && (row[0] == name || strings.HasPrefix(row[0], name+" ")) {
			return true
		}
	}
	return false
}
