package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"tariffCalculator/store"
)

// clubHistoryHas says whether the club's history has a row by who whose
// words start so.
func clubHistoryHas(t *testing.T, h http.Handler, club, who, what string) bool {
	t.Helper()
	for _, r := range clubHistoryOn(t, h, club) {
		if r[0] == who && strings.HasPrefix(r[1], what) {
			return true
		}
	}
	return false
}

// dashboardHas says whether the organiser's entries list a gymnast.
func dashboardHas(t *testing.T, h http.Handler, admin, gymnast string) bool {
	t.Helper()
	return strings.Contains(do(t, h, http.MethodGet, admin, nil).Body.String(), ">"+gymnast+"<")
}

// Entries and changes (2026-11-11): off, as every club starts, a member's
// save waits for the comp sec; on, it goes straight to the competition, the
// club's history saying they sent it; and a competition taking only what
// clubs send overrides that, until turned off again.
func TestMemberSendsEntries(t *testing.T) {
	s := newUnsentShop(t)
	h := s.h
	save := func(member string, form url.Values) string {
		t.Helper()
		return noticeOf(t, redirected(t, h, formAction(t, memberCompetitionPage(t, h, member), member+"/competitions/"), form))
	}
	if page := unescape(do(t, h, http.MethodGet, s.club+"/settings", nil).Body.String()); !strings.Contains(page, "Members save their entries and requests; you send them") ||
		!strings.Contains(page, "Members can send their own entries") || !strings.Contains(page, "Members can ask to enter late") {
		t.Fatalf("the club's Settings, all off: %s", page)
	}

	// Off: Zoe's entry waits for the comp sec.
	if notice := save(s.zoe, bucsEntry("")); notice != "Entry saved for Student Open. Your club's competition secretary sends it to the competition." {
		t.Errorf("saved, not sent: %q", notice)
	}
	if dashboardHas(t, h, s.admin, "Zoe") || !strings.Contains(clubEntriesPage(t, h, s.club), "entries to send") {
		t.Error("Zoe waits for the comp sec")
	}

	// On: it goes now, and the club's history says Zoe sent it.
	redirected(t, h, s.club+"/member-sends", url.Values{"entries": {"1"}})
	if notice := save(s.zoe, bucsEntry("")); notice != "Entry saved and sent to Student Open." {
		t.Errorf("sent: %q", notice)
	}
	if !dashboardHas(t, h, s.admin, "Zoe") || !clubHistoryHas(t, h, s.club, "Zoe", "Sent their entry to Student Open") {
		t.Error("Zoe's entry reached the organiser, and the club's history says so")
	}
	// A change to one sent still waits, until changes are on too.
	save(s.zoe, s.l4)
	if !strings.Contains(clubEntriesPage(t, h, s.club), "Changed since sent") {
		t.Error("Zoe's change waits for the comp sec")
	}
	redirected(t, h, s.club+"/member-sends", url.Values{"entries": {"1"}, "changes": {"1"}})
	if notice := save(s.zoe, s.l4); notice != "Entry saved and sent to Student Open." {
		t.Errorf("the change sent: %q", notice)
	}

	// The organiser takes only what clubs send: Hal's waits, whatever the club says.
	redirected(t, h, s.admin+"/clubs-send", url.Values{"on": {"1"}})
	if !historyHas(t, h, s.admin, "Organiser", "Changed whether club members can send their own entries") {
		t.Error("the organiser's history")
	}
	before := len(clubHistoryOn(t, h, s.club))
	if notice := save(s.hal, bucsEntry("")); !strings.Contains(notice, "Your club's competition secretary sends it") {
		t.Errorf("Hal's waits for the club: %q", notice)
	}
	if dashboardHas(t, h, s.admin, "Hal") || len(clubHistoryOn(t, h, s.club)) != before {
		t.Error("nothing sent, nothing recorded")
	}
	if page := unescape(clubEntriesPage(t, h, s.club)); !strings.Contains(page, "Student Open only takes entries sent by the club, so members' entries and requests wait for you to send them, whatever your club's settings.") {
		t.Errorf("the club is told: %s", page)
	}
	if page := unescape(do(t, h, http.MethodGet, s.club+"/settings", nil).Body.String()); !strings.Contains(page, "Student Open only takes entries sent by the club") {
		t.Errorf("the club's Settings say which competitions override them: %s", page)
	}
	if !dashboardHas(t, h, s.admin, "Zoe") {
		t.Error("what Zoe sent stays")
	}
	// Off again: the club's settings say.
	redirected(t, h, s.admin+"/clubs-send", url.Values{"on": {"0"}})
	if notice := save(s.hal, bucsEntry("")); notice != "Entry saved and sent to Student Open." {
		t.Errorf("the club's settings apply again: %q", notice)
	}
}

// Late changes and late entries: off, a member's request is saved for the
// comp sec, counted on the club's page and not the organiser's until sent;
// on, it goes at once; the organiser's override holds it whatever the club
// says. The comp sec answering a member's thread is named as the club's.
func TestMemberSendsLate(t *testing.T) {
	s := newUnsentShop(t)
	h := s.h
	s.clock.set(afterDeadline)
	redirected(t, h, s.admin+"/late/settings", url.Values{"late-new": {"1"}, "late-level": {"1"}})
	hal := memberCompetitionPage(t, h, s.hal)
	halLate := ""
	for _, m := range regexp.MustCompile(`action="([^"]+/late)"`).FindAllStringSubmatch(hal, -1) {
		halLate = m[1]
	}
	if halLate == "" {
		t.Fatalf("Hal can ask to enter late: %s", hal)
	}
	askHal := func() string {
		t.Helper()
		return noticeOf(t, redirected(t, h, halLate, url.Values{"discipline": {""}, "use": {"saved"}}))
	}
	pile := func() []string { return lateIDs(t, do(t, h, http.MethodGet, s.admin+"/late", nil).Body.String()) }

	// Off: saved for the comp sec.
	if notice := askHal(); notice != "Request to enter late saved. Your club's competition secretary sends it to the organiser." {
		t.Errorf("saved: %q", notice)
	}
	if page := unescape(do(t, h, http.MethodGet, halLate+"?discipline=", nil).Body.String()); !strings.Contains(page, "Not sent yet") || !strings.Contains(page, "Your club's competition secretary sends it to the organiser.") || strings.Contains(page, "Reply to the organiser") {
		t.Errorf("Hal sees it not sent yet, with nothing to answer: %s", page)
	}
	if loc := redirected(t, h, halLate+"/reply", url.Values{"discipline": {""}, "text": {"Hello"}}); !strings.Contains(loc, "hasn%27t+gone+to+the+organiser") {
		t.Errorf("no answering before it's sent: %s", loc)
	}
	if len(pile()) != 0 {
		t.Error("the organiser doesn't see it yet")
	}
	if page := clubEntriesPage(t, h, s.club); !strings.Contains(page, "1 late request to send") {
		t.Errorf("the club's page counts it: %s", page)
	}
	if notice := noticeOf(t, redirected(t, h, formAction(t, clubEntriesPage(t, h, s.club), s.club+"/competitions/"), url.Values{"which": {"changed"}})); notice != "Sent. 1 late request went to the organiser." {
		t.Errorf("the comp sec sends it: %q", notice)
	}
	ids := pile()
	if len(ids) != 1 {
		t.Fatalf("the organiser has it now: %v", ids)
	}
	// The comp sec answers on Hal's thread, as the club's competition secretary.
	redirected(t, h, s.admin+"/late/"+ids[0]+"/reply", url.Values{"reply": {"Which level?"}})
	m := regexp.MustCompile(`href="(` + regexp.QuoteMeta(s.club) + `/members/[^"]+/late\?discipline=)">Open the request`).FindStringSubmatch(clubEntriesPage(t, h, s.club))
	if m == nil {
		t.Fatal("the comp sec opens Hal's request")
	}
	clubLate := unescape(m[1])
	if loc := redirected(t, h, strings.Replace(clubLate, "/late?", "/late/reply?", 1), url.Values{"text": {"BUCS L3"}}); noticeOf(t, loc) != "Sent, for the organiser to see." {
		t.Errorf("the comp sec answered: %s", loc)
	}
	if page := do(t, h, http.MethodGet, s.admin+"/late", nil).Body.String(); !said(page, "UCD competition secretary replied") {
		t.Errorf("the organiser sees who of the club answered: %s", page)
	}

	// Late changes on: Gus's goes at once, and the club's history says so.
	redirected(t, h, s.club+"/member-sends", url.Values{"lateChanges": {"1"}})
	gusAsk := regexp.MustCompile(`href="([^"]+/late\?discipline=)"`).FindStringSubmatch(memberCompetitionPage(t, h, s.gus))
	if gusAsk == nil {
		t.Fatal("Gus can ask for a late change")
	}
	if notice := noticeOf(t, redirected(t, h, unescape(gusAsk[1]), s.l4)); notice != "Your late change has gone to the organiser." {
		t.Errorf("sent: %q", notice)
	}
	if len(pile()) != 2 || !clubHistoryHas(t, h, s.club, "Gus", "Asked for a late change") {
		t.Error("the organiser has it at once, and the club's history says Gus sent it")
	}

	// The override holds a late change whatever the club says.
	redirected(t, h, s.admin+"/clubs-send", url.Values{"on": {"1"}})
	if notice := noticeOf(t, redirected(t, h, unescape(gusAsk[1]), s.l4)); !strings.Contains(notice, "Your club's competition secretary sends it") {
		t.Errorf("held for the club: %q", notice)
	}
	if len(pile()) != 1 {
		t.Error("Gus's replaced request waits for the club, out of the pile")
	}
}

// The email telling of a reply to a late request quotes it as the request.
func TestYoursNamesALateRequest(t *testing.T) {
	if yours(store.Concern{Late: true}) != "Your request" || yours(store.Concern{}) != "Your message" {
		t.Error("a request is a request, a message a message")
	}
}
