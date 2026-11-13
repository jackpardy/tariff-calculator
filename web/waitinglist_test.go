package web

import (
	"context"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"testing"
	"time"

	"tariffCalculator/store"
)

// A waiting list is worked out on read, so whatever frees a place moves the
// next entry in. Each of those opens the notification window, and the person
// moved in is told once, after the grace period, by the app's words for it.

// waitingKit is a competition limiting BUCS L3 to two, three entries made in
// order, and Cat (the last, so waiting) hearing about her card.
type waitingKit struct {
	t      *testing.T
	s      *testServer
	admin  string
	catSee string // Cat's page
}

// advance moves the clock on, a minute for each step.
func (k *waitingKit) advance(d time.Duration) { k.s.clock.set(k.s.clock.now().Add(d)) }

// idOf is the id of an entry, by its gymnast.
func (k *waitingKit) idOf(name string) string {
	k.t.Helper()
	c, err := k.s.st.CompetitionByAdmin(context.Background(), path.Base(k.admin))
	if err != nil {
		k.t.Fatal(err)
	}
	entries, err := k.s.st.Entries(context.Background(), c.ID)
	if err != nil {
		k.t.Fatal(err)
	}
	for _, e := range entries {
		if e.Entry.Gymnast == name {
			return e.ID
		}
	}
	k.t.Fatalf("no entry for %s", name)
	return ""
}

// subscribeTo has the emails for a page's changes (cards) go to an address,
// confirmed.
func subscribeTo(t *testing.T, s *testServer, notify, email string) {
	t.Helper()
	s.mail.sent = nil
	redirected(t, s.h, notify, url.Values{"email": {email}, "adult": {"1"}, "topic": {"cards"}})
	if len(s.mail.sent) != 1 {
		t.Fatalf("a confirmation for %s: %+v", email, s.mail.sent)
	}
	confirm := strings.TrimPrefix(regexp.MustCompile(`https://tariff\.example(/notify/confirm/\S+)`).FindString(s.mail.sent[0].body), "https://tariff.example")
	do(t, s.h, http.MethodPost, confirm, nil)
	s.mail.sent = nil
}

// emailsTo are the emails sent to an address.
func emailsTo(s *testServer, to string) []sentEmail {
	var out []sentEmail
	for _, m := range s.mail.sent {
		if m.to == to {
			out = append(out, m)
		}
	}
	return out
}

// toldAfterTheWait tells what's due after the grace period, and again, and
// returns the emails to an address: once, however many times it's tried.
func toldAfterTheWait(t *testing.T, s *testServer, to string) []sentEmail {
	t.Helper()
	tellDue(s.p)
	if got := emailsTo(s, to); len(got) != 0 {
		t.Errorf("not before the wait: %+v", got)
	}
	s.clock.set(s.clock.now().Add(notifyGrace + time.Minute))
	tellDue(s.p)
	tellDue(s.p)
	return emailsTo(s, to)
}

// individualsWaiting makes the competition, with three individuals' entries
// (so the last is on the waiting list) and Cat's and Bea's changes emailed.
func individualsWaiting(t *testing.T) (*waitingKit, map[string]string) {
	t.Helper()
	s := newTestServer(t, atTime(dayBefore), withMail())
	k := &waitingKit{t: t, s: s, admin: created(t, s.h, newCompetition())}
	enter := pathIn(t, do(t, s.h, http.MethodGet, k.admin+"/settings", nil).Body.String(), "/competitions/enter/")
	links := map[string]string{}
	for _, name := range []string{"Ann Ryan", "Bea Kelly", "Cat Doyle"} {
		links[name] = withoutQuery(redirected(t, s.h, enter, bucsEntry(name)))
		k.advance(time.Minute)
	}
	redirected(t, s.h, k.admin+"/limits", url.Values{"limit-0": {"2"}, "anyway": {"1"}})
	subscribeTo(t, s, links["Cat Doyle"]+"/notify", "cat@example.com")
	subscribeTo(t, s, links["Bea Kelly"]+"/notify", "bea@example.com")
	k.catSee = links["Cat Doyle"]
	return k, links
}

// inOffTheList is the line Cat is told when a place comes free.
const inOffTheList = "Cat Doyle\n- BUCS L3: in, off the waiting list"

func TestWaitingListMoveUpIsTold(t *testing.T) {
	t.Run("a gymnast withdraws", func(t *testing.T) {
		k, links := individualsWaiting(t)
		if p := do(t, k.s.h, http.MethodGet, k.catSee, nil).Body.String(); !strings.Contains(p, "On the waiting list for BUCS L3: 1st") {
			t.Fatal("Cat starts on the waiting list")
		}
		redirected(t, k.s.h, links["Bea Kelly"]+"/withdraw", url.Values{"confirm": {"1"}})
		got := toldAfterTheWait(t, k.s, "cat@example.com")
		if len(got) != 1 || !strings.Contains(got[0].body, inOffTheList) {
			t.Fatalf("Cat is told once: %+v", got)
		}
		if bea := emailsTo(k.s, "bea@example.com"); len(bea) != 0 {
			t.Errorf("Bea withdrew herself: %+v", bea)
		}
		page := do(t, k.s.h, http.MethodGet, k.catSee, nil).Body.String()
		when := dayBefore.In(local).Format("2 Jan")
		if strings.Contains(page, "On the waiting list for") || !strings.Contains(page, "Was on the waiting list; a place came free on "+when+".") {
			t.Errorf("Cat's page says she was on the waiting list: %s", page)
		}
		// A week on, it's no longer said.
		k.s.clock.set(dayBefore.AddDate(0, 0, 8))
		if page := do(t, k.s.h, http.MethodGet, k.catSee, nil).Body.String(); strings.Contains(page, "Was on the waiting list") {
			t.Error("the line goes after a week")
		}
	})

	t.Run("a gymnast puts it back and goes to the back", func(t *testing.T) {
		k, links := individualsWaiting(t)
		redirected(t, k.s.h, links["Bea Kelly"]+"/withdraw", url.Values{"confirm": {"1"}})
		k.advance(time.Minute)
		redirected(t, k.s.h, links["Bea Kelly"]+"/restore", url.Values{})
		got := toldAfterTheWait(t, k.s, "bea@example.com")
		if len(got) != 1 || !strings.Contains(got[0].body, "Bea Kelly\n- BUCS L3: on the waiting list (1st)") {
			t.Errorf("Bea, put back behind Cat, is told she's waiting: %+v", got)
		}
		if cat := emailsTo(k.s, "cat@example.com"); len(cat) != 1 || !strings.Contains(cat[0].body, inOffTheList) {
			t.Errorf("Cat is in: %+v", cat)
		}
	})

	t.Run("the organiser removes an entry", func(t *testing.T) {
		k, _ := individualsWaiting(t)
		redirected(t, k.s.h, k.admin+"/remove", url.Values{"action": {"remove"}, "entry": {k.idOf("Ann Ryan")}})
		got := toldAfterTheWait(t, k.s, "cat@example.com")
		if len(got) != 1 || !strings.Contains(got[0].body, inOffTheList) {
			t.Errorf("Cat is told once: %+v", got)
		}
	})

	t.Run("an entry removed and restored in the wait tells nobody", func(t *testing.T) {
		k, _ := individualsWaiting(t)
		ann := k.idOf("Ann Ryan")
		redirected(t, k.s.h, k.admin+"/remove", url.Values{"action": {"remove"}, "entry": {ann}})
		redirected(t, k.s.h, k.admin+"/entries/"+ann+"/restore", url.Values{})
		k.s.clock.set(k.s.clock.now().Add(notifyGrace + time.Minute))
		tellDue(k.s.p)
		if len(k.s.mail.sent) != 0 {
			t.Errorf("put right, so nothing to tell: %+v", k.s.mail.sent)
		}
	})

	t.Run("the organiser raises the limit", func(t *testing.T) {
		k, _ := individualsWaiting(t)
		redirected(t, k.s.h, k.admin+"/limits", url.Values{"limit-0": {"3"}, "anyway": {"1"}})
		got := toldAfterTheWait(t, k.s, "cat@example.com")
		if len(got) != 1 || !strings.Contains(got[0].body, inOffTheList) {
			t.Errorf("Cat is told once: %+v", got)
		}
	})

	t.Run("the organiser lowers it again, pushing someone onto the list", func(t *testing.T) {
		k, _ := individualsWaiting(t)
		redirected(t, k.s.h, k.admin+"/limits", url.Values{"limit-0": {"3"}, "anyway": {"1"}})
		toldAfterTheWait(t, k.s, "cat@example.com")
		k.s.mail.sent = nil
		redirected(t, k.s.h, k.admin+"/limits", url.Values{"limit-0": {"2"}, "anyway": {"1"}})
		got := toldAfterTheWait(t, k.s, "cat@example.com")
		if len(got) != 1 || !strings.Contains(got[0].body, "Cat Doyle\n- BUCS L3: on the waiting list (1st)") {
			t.Errorf("Cat is told she's waiting again: %+v", got)
		}
	})
}

// clubWaiting is a club of Ann, Bea and Cat entered and sent in turn, a
// minute apart, with BUCS L3 limited to two: Cat is waiting. Cat's page and
// the comp sec's hear about their cards.
func clubWaiting(t *testing.T) (*testServer, string, testClub, string, map[string]string) {
	t.Helper()
	s := newTestServer(t, atTime(dayBefore), withMail())
	admin := created(t, s.h, newCompetition())
	c := aClub(t, s.h, admin, "UCD", false, "Ann", "Bea", "Cat")
	for _, name := range []string{"Ann", "Bea", "Cat"} {
		redirected(t, s.h, formAction(t, memberCompetitionPage(t, s.h, c.members[name]), c.members[name]+"/competitions/"), bucsEntry(""))
		s.clock.set(s.clock.now().Add(time.Minute))
		redirected(t, s.h, formAction(t, clubEntriesPage(t, s.h, c.page), c.page+"/competitions/"), url.Values{"which": {"all"}})
		s.clock.set(s.clock.now().Add(time.Minute))
	}
	redirected(t, s.h, admin+"/limits", url.Values{"limit-0": {"2"}, "anyway": {"1"}})
	comp := compIDOf(t, s.h, c.page)
	subscribeTo(t, s, c.members["Cat"]+"/competitions/"+comp+"/notify", "cat@example.com")
	subscribeTo(t, s, c.page+"/competitions/"+comp+"/notify", "compsec@example.com")
	return s, admin, c, comp, memberIDs(t, s.h, c.page)
}

func TestWaitingListMoveUpIsToldForClubs(t *testing.T) {
	// told checks Cat and her comp sec are each told once she's in, and her
	// page says so.
	told := func(t *testing.T, s *testServer, c testClub) {
		t.Helper()
		const line = "Cat\n- BUCS L3: in, off the waiting list"
		got := toldAfterTheWait(t, s, "cat@example.com")
		if len(got) != 1 || !strings.Contains(got[0].body, line) {
			t.Errorf("Cat is told once: %+v", got)
		}
		if sec := emailsTo(s, "compsec@example.com"); len(sec) != 1 || !strings.Contains(sec[0].body, line) {
			t.Errorf("the comp sec is told once: %+v", sec)
		}
		if page := memberCompetitionPage(t, s.h, c.members["Cat"]); !strings.Contains(page, "Was on the waiting list; a place came free on ") {
			t.Errorf("Cat's page: %s", page)
		}
	}

	t.Run("a member withdraws", func(t *testing.T) {
		s, _, c, comp, _ := clubWaiting(t)
		if page := memberCompetitionPage(t, s.h, c.members["Cat"]); !strings.Contains(page, "On the waiting list for BUCS L3: 1st") {
			t.Fatal("Cat starts on the waiting list")
		}
		redirected(t, s.h, c.members["Bea"]+"/competitions/"+comp+"/withdraw", url.Values{"confirm": {"1"}})
		told(t, s, c)
	})

	t.Run("the comp sec withdraws a member", func(t *testing.T) {
		s, _, c, comp, ids := clubWaiting(t)
		redirected(t, s.h, c.page+"/members/"+ids["Bea"]+"/competitions/"+comp+"/withdraw", url.Values{"confirm": {"1"}})
		told(t, s, c)
	})

	t.Run("the club takes a member out", func(t *testing.T) {
		s, _, c, comp, ids := clubWaiting(t)
		redirected(t, s.h, c.page+"/competitions/"+comp+"/take-out", url.Values{"member": {ids["Bea"]}, "what": {"entries"}, "anyway": {"1"}})
		told(t, s, c)
	})

	t.Run("the club removes a member", func(t *testing.T) {
		s, _, c, _, ids := clubWaiting(t)
		redirected(t, s.h, c.page+"/members/"+ids["Bea"]+"/remove", url.Values{"anyway": {"1"}})
		told(t, s, c)
	})

	t.Run("a withdrawal put right in the wait tells nobody", func(t *testing.T) {
		s, _, c, comp, _ := clubWaiting(t)
		bea := c.members["Bea"]
		redirected(t, s.h, bea+"/competitions/"+comp+"/withdraw", url.Values{"confirm": {"1"}})
		// Bea enters again and the club sends: she keeps her place.
		redirected(t, s.h, formAction(t, memberCompetitionPage(t, s.h, bea), bea+"/competitions/"), bucsEntry(""))
		redirected(t, s.h, formAction(t, clubEntriesPage(t, s.h, c.page), c.page+"/competitions/"), url.Values{"which": {"all"}})
		s.clock.set(s.clock.now().Add(notifyGrace + time.Minute))
		tellDue(s.p)
		if len(s.mail.sent) != 0 {
			t.Errorf("put right, so nothing to tell: %+v", s.mail.sent)
		}
	})
}

// A club's own withdrawal isn't told to its member as being "back in the
// competition" (their own doing), and an entry the club withdrew isn't
// "off the waiting list".
func TestWithdrawingIsNotMovingUp(t *testing.T) {
	was := notifyState{Entries: map[string]entryState{"e": {Gymnasts: "Cat", Event: "BUCS L3", Waiting: 1, Keys: []string{"k"}}}}
	now := notifyState{Entries: map[string]entryState{"e": {Gymnasts: "Cat", Event: "BUCS L3", Withdrawn: true, Keys: []string{"k"}}}}
	if got := changes(was, now); len(got) != 0 {
		t.Errorf("a waiting entry withdrawn isn't in: %+v", got)
	}
	was = notifyState{Entries: map[string]entryState{"e": {Gymnasts: "Cat", Event: "BUCS L3", Keys: []string{"k"}}}}
	now = notifyState{Entries: map[string]entryState{"e": {Gymnasts: "Cat", Event: "BUCS L3", Removal: store.Withdrew, Keys: []string{"k"}}}}
	if got := changes(was, now); len(got) != 0 {
		t.Errorf("withdrawn by the gymnast: nothing to tell: %+v", got)
	}
	was, now = now, was
	if got := changes(was, now); len(got) != 0 {
		t.Errorf("put back by the gymnast: nothing to tell: %+v", got)
	}
}
