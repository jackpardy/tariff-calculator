package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"
)

// unsentShop is a competition on a clock, with a club whose members are at
// different stages: Gus was sent at one level and has changed it since, Hal
// saved an entry that was never sent, Zoe has none. Ann coaches them all.
type unsentShop struct {
	h                  http.Handler
	clock              *testClock
	admin, club, coach string
	gus, hal, zoe      string
	l4                 url.Values
}

// before and after are times before the deadline (Saturday 9 March 2030,
// 23:59) and after it.
var (
	beforeDeadline = inDublin(2030, time.February, 6, 12, 0)
	afterDeadline  = inDublin(2030, time.March, 10, 12, 0)
)

func newUnsentShop(t *testing.T, tweak ...func(url.Values)) *unsentShop {
	t.Helper()
	h, clock := competitionServerAt(t, inDublin(2030, time.February, 1, 12, 0))
	form := newCompetition()
	form["level"] = append(form["level"], "builtin-level:bucs-l4")
	for _, f := range tweak {
		f(form)
	}
	s := &unsentShop{h: h, clock: clock, admin: created(t, h, form),
		l4: url.Values{"level": {"BUCS L4"}, "ex1Option": {"builtin:bucs-l4-option-1"}, "ex2Option": {"builtin:bucs-l4-second"}, "ex2Skills": {voluntary}}}
	c := aClub(t, h, s.admin, "UCD", false, "Gus", "Hal", "Zoe")
	s.club, s.gus, s.hal, s.zoe = c.page, c.members["Gus"], c.members["Hal"], c.members["Zoe"]
	s.coach = pathIn(t, do(t, h, http.MethodPost, s.club+"/coaches", url.Values{"name": {"Ann"}}).Body.String(), "/clubs/coach/")
	redirected(t, h, formAction(t, memberCompetitionPage(t, h, s.gus), s.gus+"/competitions/"), bucsEntry(""))
	s.send(t, "all")
	redirected(t, h, formAction(t, memberCompetitionPage(t, h, s.hal), s.hal+"/competitions/"), bucsEntry(""))
	clock.set(beforeDeadline)
	redirected(t, h, formAction(t, memberCompetitionPage(t, h, s.gus), s.gus+"/competitions/"), s.l4)
	redirected(t, h, s.admin+"/timetable/plan", url.Values{})
	return s
}

// send posts the Send buttons: which is "changed" or "all". It returns the notice.
func (s *unsentShop) send(t *testing.T, which string) string {
	t.Helper()
	return noticeOf(t, redirected(t, s.h, formAction(t, clubEntriesPage(t, s.h, s.club), s.club+"/competitions/"), url.Values{"which": {which}}))
}

// said is whether a page says this in its words (not its HTML).
func said(page, text string) bool { return strings.Contains(unescape(page), text) }

// coachPage is the coach's list: Ann's page.
func (s *unsentShop) coachPage(t *testing.T) string {
	t.Helper()
	return do(t, s.h, http.MethodGet, s.coach, nil).Body.String()
}

const (
	gusLine = "You changed this to BUCS L4 on 6 Feb, but it wasn't sent before entries closed. The competition has BUCS L3."
	halLine = "You saved this on 1 Feb, but it wasn't sent before entries closed, so it isn't in the competition."
)

// A change saved but not sent before entries close isn't the entry: the
// pages show what the competition holds, and say what happened to the change,
// until entries reopen.
func TestChangeNotSentBeforeClose(t *testing.T) {
	s := newUnsentShop(t)

	// While entries are open it's a change to send.
	page := memberCompetitionPage(t, s.h, s.gus)
	if !said(page, "Changed since your club sent it") || said(page, "wasn't sent before") {
		t.Error("open: changed since sent, nothing lost yet")
	}
	if page := clubEntriesPage(t, s.h, s.club); !strings.Contains(page, "Changed since sent") || !strings.Contains(page, "2 entries to send") {
		t.Errorf("open: the club has two to send: %s", page)
	}

	s.clock.set(afterDeadline)
	page = memberCompetitionPage(t, s.h, s.gus)
	if !said(page, gusLine) {
		t.Errorf("the member's page says what happened: %s", page)
	}
	if said(page, "Changed since your club sent it") || !strings.Contains(page, "BUCS L3") {
		t.Error("the member's page headlines what the competition holds, not the change")
	}
	if strings.Contains(page, "name=\"level\"") {
		t.Error("no form to change it")
	}
	club := clubEntriesPage(t, s.h, s.club)
	if !said(club, "Gus changed this to BUCS L4 on 6 Feb, but it wasn't sent before entries closed. The competition has BUCS L3.") ||
		strings.Contains(club, "Changed since sent") {
		t.Errorf("the comp sec's row: %s", club)
	}
	if strings.Contains(club, "Send new and changed") || strings.Contains(club, "to send") {
		t.Errorf("nothing to send once closed: %s", club)
	}
	if home := do(t, s.h, http.MethodGet, s.club, nil).Body.String(); strings.Contains(home, "to send") {
		t.Errorf("the club's home page doesn't count it: %s", home)
	}
	coach := s.coachPage(t)
	if !said(coach, "Gus changed this to BUCS L4 on 6 Feb, but it wasn't sent before entries closed. The competition has BUCS L3.") {
		t.Errorf("the coach's list: %s", coach)
	}
	if rows := tableRows(t, coach); len(rows) < 1 || !strings.Contains(strings.Join(rows[0], " "), "BUCS L3") {
		t.Errorf("the coach's list has the held level: %v", rows)
	}

	// With late changes allowed, the page offers the link to ask; a refused one
	// changes nothing here.
	redirected(t, s.h, s.admin+"/late/settings", url.Values{"late-level": {"1"}, "fee-level": {"10"}})
	redirected(t, s.h, s.club+"/member-sends", url.Values{"lateChanges": {"1"}, "lateEntries": {"1"}}) // members ask themselves
	page = memberCompetitionPage(t, s.h, s.gus)
	ask := regexp.MustCompile(`href="([^"]+/late\?discipline=)"`).FindStringSubmatch(page)
	if !said(page, gusLine) || ask == nil {
		t.Fatalf("the line and the link to ask for a late change: %s", page)
	}
	if page := clubEntriesPage(t, s.h, s.club); !strings.Contains(page, "Ask for a late change") {
		t.Error("the club row has the link too")
	}
	redirected(t, s.h, ask[1], s.l4)
	late := do(t, s.h, http.MethodGet, s.admin+"/late", nil).Body.String()
	id := regexp.MustCompile(`/late/([^/"]+)/accept`).FindStringSubmatch(late)
	if id == nil {
		t.Fatalf("the organiser has the request: %s", late)
	}
	redirected(t, s.h, s.admin+"/late/"+id[1]+"/reject", url.Values{"reason": {"Too late"}})
	page = memberCompetitionPage(t, s.h, s.gus)
	if !said(page, gusLine) || said(page, "Changed since your club sent it") {
		t.Errorf("after a refused late change it still says the competition has BUCS L3: %s", page)
	}
	if club := clubEntriesPage(t, s.h, s.club); !said(club, "The competition has BUCS L3.") || strings.Contains(club, "Changed since sent") {
		t.Error("and so does the club row")
	}

	// Reopened, the unsent version is the entry again, and can be sent.
	s.clock.set(beforeDeadline)
	page = memberCompetitionPage(t, s.h, s.gus)
	if !said(page, "Changed since your club sent it") || said(page, "wasn't sent before") {
		t.Errorf("reopened: the change is the entry again: %s", page)
	}
	if n := s.send(t, "changed"); n != "Sent 2 entries to Student Open." {
		t.Errorf("reopened: both go: %q", n)
	}
}

// An entry never sent isn't in the competition once new entries close, and the
// pages say so instead of offering a Send that can't work.
func TestNeverSentAfterClose(t *testing.T) {
	s := newUnsentShop(t)
	s.clock.set(afterDeadline)

	page := memberCompetitionPage(t, s.h, s.hal)
	if !said(page, halLine) || said(page, "Saved, not sent yet") {
		t.Errorf("the member's page: %s", page)
	}
	if strings.Contains(page, "name=\"level\"") {
		t.Error("no form to change an entry that can't be sent")
	}
	club := clubEntriesPage(t, s.h, s.club)
	if !said(club, "Hal saved this on 1 Feb, but it wasn't sent before entries closed, so it isn't in the competition.") || !strings.Contains(club, "Not sent, not in the competition") {
		t.Errorf("the comp sec's row: %s", club)
	}
	if strings.Contains(club, "to send") || strings.Contains(club, ">Change</a>") {
		t.Errorf("nothing to send or change: %s", club)
	}
	if coach := s.coachPage(t); !said(coach, "Hal saved this on 1 Feb") {
		t.Errorf("the coach's list: %s", coach)
	}

	s.clock.set(beforeDeadline)
	if page := memberCompetitionPage(t, s.h, s.hal); !said(page, "Saved, not sent yet") || said(page, "isn't in the competition") {
		t.Errorf("reopened: sendable again: %s", page)
	}
	if page := clubEntriesPage(t, s.h, s.club); !strings.Contains(page, "2 entries to send") {
		t.Errorf("reopened: counted again: %s", page)
	}
}

// With changes closing later than new entries, a brand-new entry is refused on
// the member's and the comp sec's forms, while a change to one the competition
// holds still goes.
func TestNewEntryRefusedAfterClose(t *testing.T) {
	s := newUnsentShop(t)
	redirected(t, s.h, s.admin+"/later-deadlines", url.Values{"changesDate": {"2030-03-16"}, "changesTime": {"09:00"}})
	// Late entries turned off (they're allowed unless they are).
	redirected(t, s.h, s.admin+"/late/settings", url.Values{})
	// The forms as they were while entries were open.
	zoeSave := formAction(t, memberCompetitionPage(t, s.h, s.zoe), s.zoe+"/competitions/")
	compPage := clubEntriesPage(t, s.h, s.club)
	enterMember := regexp.MustCompile(`action="([^"]+/enter-member)"`).FindStringSubmatch(compPage)[1]
	member := regexp.MustCompile(`<option value="([^"]+)"[^>]*>Zoe</option>`).FindStringSubmatch(compPage)[1]
	zoeForm := withoutQuery(do(t, s.h, http.MethodGet, enterMember+"?member="+member+"&discipline=", nil).Header().Get("Location"))
	halSave := formAction(t, memberCompetitionPage(t, s.h, s.hal), s.hal+"/competitions/")

	s.clock.set(afterDeadline)
	closedWords := "The deadline for this competition has passed, so entries can't be changed."

	// Zoe, who has no entry: no form on her page, and a post is refused.
	if page := memberCompetitionPage(t, s.h, s.zoe); strings.Contains(page, "name=\"level\"") {
		t.Errorf("no form for a new entry: %s", page)
	}
	if rec := do(t, s.h, http.MethodPost, zoeSave, bucsEntry("")); rec.Code != http.StatusConflict || !said(rec.Body.String(), closedWords) {
		t.Errorf("member's new entry: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, s.h, http.MethodGet, zoeForm, nil); rec.Code != http.StatusConflict {
		t.Errorf("the comp sec's form for a new entry: %d", rec.Code)
	}
	if rec := do(t, s.h, http.MethodPost, zoeForm, bucsEntry("")); rec.Code != http.StatusConflict || !said(rec.Body.String(), closedWords) {
		t.Errorf("the comp sec's new entry: %d %s", rec.Code, rec.Body.String())
	}
	if page := clubEntriesPage(t, s.h, s.club); strings.Contains(page, "enter-member") {
		t.Error("the comp sec's Enter a member isn't offered")
	}
	// Hal's entry was never sent: it can't be changed either.
	if rec := do(t, s.h, http.MethodPost, halSave, bucsEntry("")); rec.Code != http.StatusConflict {
		t.Errorf("an entry never sent: %d", rec.Code)
	}

	// Gus's is held by the competition: he can still change it, and it goes.
	page := memberCompetitionPage(t, s.h, s.gus)
	if !strings.Contains(page, "Change your entry") {
		t.Fatalf("Gus can change his: %s", page)
	}
	redirected(t, s.h, formAction(t, page, s.gus+"/competitions/"), bucsEntry(""))
	if page := clubEntriesPage(t, s.h, s.club); !strings.Contains(page, "1 entry to send") {
		t.Errorf("Gus's change is the one thing to send: %s", page)
	}
}

// The notice after sending, and the count of what's to send, are what Send did:
// an entry left out because new entries have closed, or the organiser removed
// it, is said so, not counted as sent.
func TestSendNoticeSaysWhatWasLeftOut(t *testing.T) {
	h, clock := competitionServerAt(t, inDublin(2030, time.February, 1, 12, 0))
	form := newCompetition()
	form["level"] = append(form["level"], "builtin-level:bucs-l4")
	admin := created(t, h, form)
	c := aClub(t, h, admin, "UCD", false, "Gus", "Hal", "Kit")
	for _, name := range []string{"Gus", "Kit"} {
		redirected(t, h, formAction(t, memberCompetitionPage(t, h, c.members[name]), c.members[name]+"/competitions/"), bucsEntry(""))
	}
	send := func(which string) string {
		return noticeOf(t, redirected(t, h, formAction(t, clubEntriesPage(t, h, c.page), c.page+"/competitions/"), url.Values{"which": {which}}))
	}
	send("all")
	redirected(t, h, formAction(t, memberCompetitionPage(t, h, c.members["Hal"]), c.members["Hal"]+"/competitions/"), bucsEntry(""))
	clock.set(beforeDeadline)
	l4 := url.Values{"level": {"BUCS L4"}, "ex1Option": {"builtin:bucs-l4-option-1"}, "ex2Option": {"builtin:bucs-l4-second"}, "ex2Skills": {voluntary}}
	redirected(t, h, formAction(t, memberCompetitionPage(t, h, c.members["Gus"]), c.members["Gus"]+"/competitions/"), l4)

	// The organiser removes Kit; new entries close, changes stay open later.
	var kit string
	for _, m := range regexp.MustCompile(`name="entry" value="([^"]+)" form="remove-form" aria-label="Tick ([^"]+)"`).FindAllStringSubmatch(do(t, h, http.MethodGet, admin, nil).Body.String(), -1) {
		if m[2] == "Kit" {
			kit = m[1]
		}
	}
	redirected(t, h, admin+"/remove", url.Values{"entry": {kit}, "action": {"remove"}, "note": {"Not paid"}, "to": {"club"}})
	redirected(t, h, admin+"/later-deadlines", url.Values{"changesDate": {"2030-03-16"}, "changesTime": {"09:00"}})
	clock.set(afterDeadline)

	page := clubEntriesPage(t, h, c.page)
	if !strings.Contains(page, "1 entry to send") || !strings.Contains(page, "Send new and changed (1)") {
		t.Errorf("only Gus's change would go: %s", page)
	}
	if n := send("changed"); n != "Sent 1 entry to Student Open. 1 wasn't sent: entries have closed for new entries." {
		t.Errorf("changed: %q", n)
	}
	if page := clubEntriesPage(t, h, c.page); strings.Contains(page, "to send") || !strings.Contains(page, "All sent") {
		t.Errorf("nothing more to send: %s", page)
	}
	if n := send("all"); n != "Sent 1 entry to Student Open. 1 wasn't sent: entries have closed for new entries. 1 was removed by the organiser, so it wasn't sent." {
		t.Errorf("all again: %q", n)
	}
}
