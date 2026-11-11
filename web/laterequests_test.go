package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// lateIDs are the late requests on the organiser's page, by id, in order.
func lateIDs(t *testing.T, page string) []string {
	t.Helper()
	var out []string
	for _, m := range regexp.MustCompile(`/late/([^/"]+)/reject`).FindAllStringSubmatch(page, -1) {
		out = append(out, m[1])
	}
	return out
}

// A late change carries a thread too: the organiser replies, the gymnast
// sees it with the request and answers, and the organiser sees that.
func TestLateChangeThread(t *testing.T) {
	ls := lateCompetition(t)
	h, admin := ls.s.h, ls.admin
	redirected(t, h, ls.ivy+"/late", l4Entry("Ivy"))
	ids := lateIDs(t, do(t, h, http.MethodGet, admin+"/late", nil).Body.String())
	if len(ids) != 1 {
		t.Fatalf("Ivy's change reaches the organiser: %v", ids)
	}
	redirected(t, h, admin+"/late/"+ids[0]+"/reply", url.Values{"reply": {"Only if her coach signs it"}})
	page := do(t, h, http.MethodGet, ls.ivy+"/late", nil).Body.String()
	if !said(page, "Only if her coach signs it") || !strings.Contains(page, "Reply to the organiser") {
		t.Fatalf("Ivy sees the reply, and can answer: %s", page)
	}
	if loc := redirected(t, h, ls.ivy+"/late/reply", url.Values{"text": {""}}); !strings.Contains(loc, "nothing+was+sent") {
		t.Errorf("an empty answer: %s", loc)
	}
	redirected(t, h, ls.ivy+"/late/reply", url.Values{"text": {"She will"}})
	if !said(do(t, h, http.MethodGet, admin+"/late", nil).Body.String(), "She will") {
		t.Error("the organiser sees Ivy's answer")
	}
	redirected(t, h, admin+"/late/"+ids[0]+"/reject", url.Values{"reason": {"Too late"}})
	if loc := redirected(t, h, ls.ivy+"/late/reply", url.Values{"text": {"Please"}}); !strings.Contains(loc, "decided") {
		t.Errorf("decided, the thread is closed: %s", loc)
	}
	if !historyHas(t, h, admin, "Organiser", "Replied to a late change: Ivy · BUCS L4") {
		t.Error("the history")
	}
}

// Late entries (late requests, 2026-11-11): once new entries close, a member
// asks with their saved entry in one tap, the comp sec for a member with the
// form, and a gymnast on their own as they'd enter. Each reaches the
// organiser at once with its sign-off; the organiser replies and the asker
// answers (a helper link sees neither); a coach signs it off; accepted, it's
// an entry, with its fee; turned down, they see why.
func TestLateEntries(t *testing.T) {
	s := newUnsentShop(t, func(f url.Values) { f.Set("signoff", "1") })
	h := s.h
	redirected(t, h, s.club+"/coaches-see-all", url.Values{"on": {"1"}})
	enter := pathIn(t, do(t, h, http.MethodGet, s.admin+"/settings", nil).Body.String(), "/competitions/enter/")
	redirected(t, h, s.admin+"/late/settings", url.Values{"late-new": {"1"}, "fee-new": {"7"}, "signoffFirst": {"1"}})
	redirected(t, h, s.club+"/member-sends", url.Values{"lateChanges": {"1"}, "lateEntries": {"1"}}) // members ask themselves
	cards := madeLink(t, h, s.admin, "Cara", "cards")
	s.clock.set(afterDeadline)

	// Hal's entry was never sent: one tap asks with it.
	hal := memberCompetitionPage(t, h, s.hal)
	if !said(hal, halLine) || !strings.Contains(hal, "Ask to enter late") {
		t.Fatalf("Hal can ask to enter late: %s", hal)
	}
	halLate := formAction(t, hal, s.hal+"/competitions/")
	for _, m := range regexp.MustCompile(`action="([^"]+/late)"`).FindAllStringSubmatch(hal, -1) {
		halLate = m[1]
	}
	if notice := noticeOf(t, redirected(t, h, halLate, url.Values{"discipline": {""}, "use": {"saved"}})); notice != "Your request to enter late has gone to the organiser. A coach needs to sign it off before the organiser can accept it." {
		t.Errorf("asked: %q", notice)
	}
	if page := unescape(memberCompetitionPage(t, h, s.hal)); !strings.Contains(page, "Asked to enter late") || !strings.Contains(page, "Waiting for sign-off") {
		t.Errorf("Hal sees it asked, waiting for sign-off: %s", page)
	}

	// The organiser sees it at once, not signed off, and can't accept it yet.
	late := do(t, h, http.MethodGet, s.admin+"/late", nil).Body.String()
	ids := lateIDs(t, late)
	if len(ids) != 1 || !strings.Contains(late, "Late entry") || !strings.Contains(late, "Waiting for sign-off") || strings.Contains(late, "/accept") {
		t.Fatalf("Hal's request, waiting for sign-off: %s", late)
	}
	halID := ids[0]
	if strings.Contains(do(t, h, http.MethodGet, s.admin, nil).Body.String(), ">Hal<") {
		t.Error("Hal isn't among the entries yet")
	}
	if loc := redirected(t, h, s.admin+"/late/"+halID+"/accept", url.Values{}); !strings.Contains(loc, "can%27t+be+accepted") {
		t.Errorf("accepting before sign-off is refused: %s", loc)
	}

	// A reply, and Hal's answer.
	if notice := noticeOf(t, redirected(t, h, s.admin+"/late/"+halID+"/reply", url.Values{"reply": {"Can be entered if signed off by Friday"}})); !strings.HasPrefix(notice, "Reply sent.") {
		t.Errorf("replied: %q", notice)
	}
	page := do(t, h, http.MethodGet, halLate+"?discipline=", nil).Body.String()
	if !said(page, "Can be entered if signed off by Friday") || !strings.Contains(page, "The organiser replied") {
		t.Fatalf("Hal sees the reply with his request: %s", page)
	}
	redirected(t, h, halLate+"/reply", url.Values{"discipline": {""}, "text": {"Ann will sign it tomorrow"}})
	if !said(do(t, h, http.MethodGet, s.admin+"/late", nil).Body.String(), "Ann will sign it tomorrow") {
		t.Error("the organiser sees Hal's answer")
	}
	// The comp sec sees it too, and can add to it.
	club := clubEntriesPage(t, h, s.club)
	clubLate := regexp.MustCompile(regexp.QuoteMeta(s.club) + `/members/[^"]+/late\?discipline=`).FindString(club)
	if clubLate == "" || !strings.Contains(club, "Asked to enter late") {
		t.Fatalf("the comp sec sees Hal's request: %s", club)
	}
	if page := do(t, h, http.MethodGet, unescape(clubLate), nil).Body.String(); !said(page, "Ann will sign it tomorrow") || !said(page, "Hal replied") {
		t.Errorf("the comp sec sees the thread, with who said what: %s", page)
	}
	// A cards link sees none of it, and can't post to it.
	if strings.Contains(do(t, h, http.MethodGet, cards, nil).Body.String(), "signed off by Friday") {
		t.Error("the cards link doesn't see the thread")
	}
	for _, path := range []string{"/late", "/late/" + halID + "/reply"} {
		method := http.MethodGet
		if strings.HasSuffix(path, "/reply") {
			method = http.MethodPost
		}
		if rec := do(t, h, method, cards+path, url.Values{"reply": {"x"}}); rec.Code != http.StatusForbidden {
			t.Errorf("the cards link at %s: %d", path, rec.Code)
		}
	}

	// The coach signs it off; the organiser sees it and accepts it.
	link := regexp.MustCompile(regexp.QuoteMeta(s.coach) + `/late/[^"]+`).FindString(s.coachPage(t))
	if link == "" {
		t.Fatalf("the coach lists it to sign off: %s", s.coachPage(t))
	}
	redirected(t, h, link, url.Values{})
	late = do(t, h, http.MethodGet, s.admin+"/late", nil).Body.String()
	if !strings.Contains(late, "by Ann") || !strings.Contains(late, "/late/"+halID+"/accept") {
		t.Fatalf("signed off by Ann, it can be accepted: %s", late)
	}
	if notice := noticeOf(t, redirected(t, h, s.admin+"/late/"+halID+"/accept", url.Values{})); !strings.HasPrefix(notice, "Accepted: Hal is in BUCS L3.") {
		t.Errorf("accepted: %q", notice)
	}
	if page := unescape(memberCompetitionPage(t, h, s.hal)); !strings.Contains(page, "accepted (€7, on the invoice)") || strings.Contains(page, "Asked to enter late") {
		t.Errorf("Hal's entry is in, with its fee: %s", page)
	}
	if !strings.Contains(do(t, h, http.MethodGet, s.admin, nil).Body.String(), ">Hal<") {
		t.Error("Hal is among the entries")
	}

	// Zoe, by the comp sec, with the form; turned down.
	compPage := clubEntriesPage(t, h, s.club)
	enterMember := regexp.MustCompile(`action="([^"]+/enter-member)"`).FindStringSubmatch(compPage)
	member := regexp.MustCompile(`<option value="([^"]+)"[^>]*>Zoe</option>`).FindStringSubmatch(compPage)
	if enterMember == nil || member == nil || !strings.Contains(compPage, "Ask to enter a member late") {
		t.Fatalf("the comp sec can ask to enter a member late: %s", compPage)
	}
	zoe := do(t, h, http.MethodGet, enterMember[1]+"?member="+member[1]+"&discipline=", nil).Header().Get("Location")
	if !strings.Contains(zoe, "/late?") {
		t.Fatalf("to the late request's form: %s", zoe)
	}
	if page := do(t, h, http.MethodGet, zoe, nil).Body.String(); !strings.Contains(page, "Ask to enter late") {
		t.Fatalf("the form: %s", page)
	}
	redirected(t, h, zoe, bucsEntry(""))
	zoeID := ""
	for _, id := range lateIDs(t, do(t, h, http.MethodGet, s.admin+"/late", nil).Body.String()) {
		if id != halID {
			zoeID = id
		}
	}
	if zoeID == "" {
		t.Fatal("Zoe's request reaches the organiser")
	}
	redirected(t, h, s.admin+"/late/"+zoeID+"/reject", url.Values{"reason": {"BUCS L3 is full"}})
	if page := unescape(memberCompetitionPage(t, h, s.zoe)); !strings.Contains(page, "turned down. BUCS L3 is full") || !strings.Contains(page, "Turned down") {
		t.Errorf("Zoe sees why: %s", page)
	}
	// A turn-down ends that request only: asked again at another level, it's
	// a new request, which says what was turned down before.
	l4 := url.Values{"level": {"BUCS L4"}, "ex1Option": {"builtin:bucs-l4-option-1"}, "ex2Option": {"builtin:bucs-l4-second"}, "ex2Skills": {voluntary}}
	if notice := noticeOf(t, redirected(t, h, zoe, l4)); !strings.HasPrefix(notice, "Your request to enter late has gone to the organiser.") {
		t.Errorf("asked again, at BUCS L4: %q", notice)
	}
	late = unescape(do(t, h, http.MethodGet, s.admin+"/late", nil).Body.String())
	if !strings.Contains(late, "Asked before for BUCS L3, turned down 10 Mar: BUCS L3 is full") || !strings.Contains(late, "turned down. BUCS L3 is full") {
		t.Errorf("the new request says what was turned down before, which stays turned down: %s", late)
	}
	if page := do(t, h, http.MethodGet, zoe, nil).Body.String(); said(page, "BUCS L3 is full") {
		t.Errorf("the new request has a thread of its own, and Zoe sees it waiting: %s", page)
	}

	// Ivy, on her own, asks as she'd enter.
	if page := do(t, h, http.MethodGet, enter, nil).Body.String(); !strings.Contains(page, "Ask to enter late") {
		t.Fatalf("the entry link asks to enter late: %s", page)
	}
	ivy := redirected(t, h, enter, bucsEntry("Ivy"))
	if page := unescape(do(t, h, http.MethodGet, ivy, nil).Body.String()); !strings.Contains(page, "You asked to enter late") || !strings.Contains(page, "Waiting for sign-off") {
		t.Errorf("Ivy sees her request: %s", page)
	}
	ids = lateIDs(t, do(t, h, http.MethodGet, s.admin+"/late", nil).Body.String())
	if len(ids) != 2 {
		t.Fatalf("Zoe's second and Ivy's to decide now: %d", len(ids))
	}
	// Ivy turned down, asks again at another level from her request's page.
	redirected(t, h, s.admin+"/late/"+ids[1]+"/reject", url.Values{"reason": {"No room"}})
	ivyLate := withoutQuery(ivy) + "/late"
	if page := do(t, h, http.MethodGet, ivyLate, nil).Body.String(); !strings.Contains(page, "Ask again") {
		t.Fatalf("Ivy can ask again: %s", page)
	}
	ivyL4 := url.Values{"gymnast": {"Ivy"}}
	for k, v := range l4 {
		ivyL4[k] = v
	}
	if notice := noticeOf(t, redirected(t, h, ivyLate, ivyL4)); !strings.HasPrefix(notice, "Your request to enter late has gone to the organiser.") {
		t.Errorf("Ivy asked again: %q", notice)
	}
	if late := unescape(do(t, h, http.MethodGet, s.admin+"/late", nil).Body.String()); !strings.Contains(late, "Asked before for BUCS L3, turned down 10 Mar: No room") {
		t.Errorf("Ivy's new request says what was turned down: %s", late)
	}

	// The history: what was done, not what was refused.
	if !historyHas(t, s.h, s.admin, "Organiser", "Accepted a late entry: Hal · BUCS L3") || !historyHas(t, s.h, s.admin, "Organiser", "Turned down a late entry: Zoe") ||
		!historyHas(t, s.h, s.admin, "Organiser", "Replied to a late entry: Hal") {
		t.Error("the organiser's history")
	}
	refusedAccepts := 0
	for _, row := range tableRows(t, do(t, h, http.MethodGet, s.admin+"/history", nil).Body.String()) {
		if len(row) == 3 && strings.HasPrefix(row[2], "Accepted") {
			refusedAccepts++
		}
	}
	if refusedAccepts != 1 {
		t.Errorf("the refused accept isn't recorded: %d accepts", refusedAccepts)
	}
	rows := clubHistoryOn(t, h, s.club)
	found := false
	for _, r := range rows {
		found = found || strings.HasPrefix(r[1], "Asked to enter Zoe")
	}
	if !found {
		t.Errorf("the club's history: %v", rows)
	}

	// Turned off, nothing can be asked to enter late.
	redirected(t, h, s.admin+"/late/settings", url.Values{})
	if page := do(t, h, http.MethodGet, enter, nil).Body.String(); strings.Contains(page, "Ask to enter late") {
		t.Error("turned off: the entry link doesn't ask")
	}
	if rec := do(t, h, http.MethodPost, enter, bucsEntry("Max")); rec.Code < 400 {
		t.Errorf("turned off: entering is refused: %d", rec.Code)
	}
}
