package web

import (
	"context"
	"html"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"tariffCalculator/demo"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// inSandbox makes the test server the sandbox, its seed a few clubs.
func inSandbox() serverOption {
	return func(s *testServer) {
		s.p.makeSandbox()
		s.p.sandbox.size = demo.Small
	}
}

// sandboxCopy resets the sandbox and returns its copy's links, and its
// competitions: open for entries, and on the day.
func sandboxCopy(t *testing.T, s *testServer) (demo.Links, store.Competition, store.Competition) {
	t.Helper()
	ctx := context.Background()
	if _, err := s.p.resetSandbox(ctx); err != nil {
		t.Fatal(err)
	}
	cp, err := s.st.SharedCopy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	l, err := sandboxLinks(cp)
	if err != nil {
		t.Fatal(err)
	}
	byAdmin := func(admin string) store.Competition {
		c, err := s.st.CompetitionByAdmin(ctx, strings.TrimPrefix(admin, adminPath("")))
		if err != nil {
			t.Fatalf("%s: %v", admin, err)
		}
		return c
	}
	return l, byAdmin(l.Open.Organiser), byAdmin(l.Day.Organiser)
}

// The sandbox's seed makes both competitions in their phases, with
// messages to the organiser at every stage, and the morning of the day
// under way.
func TestSandboxSeed(t *testing.T) {
	s := newTestServer(t, inSandbox())
	ctx := context.Background()
	l, open, day := sandboxCopy(t, s)
	now := time.Now().In(local)

	if open.Name != demo.SandboxOpen || day.Name != demo.SandboxDay {
		t.Errorf("named %q and %q", open.Name, day.Name)
	}
	t.Run("open for entries", func(t *testing.T) {
		if open.Published != nil {
			t.Error("nothing is published")
		}
		if want := now.AddDate(0, 0, 21); open.Deadline.In(local).Format("2006-01-02") != want.Format("2006-01-02") || !open.Open(now) {
			t.Errorf("closes %v, three weeks from now, and is open", open.Deadline)
		}
		entries, err := s.st.Entries(ctx, open.ID)
		if err != nil {
			t.Fatal(err)
		}
		sent, unsigned := map[string]bool{}, 0
		for _, e := range entries {
			if e.ClubID != "" {
				sent[e.ClubID] = true
			}
			if e.Individual && e.SignedAt.IsZero() {
				unsigned++
			}
		}
		clubs, _ := s.st.CompetitionClubs(ctx, open.ID)
		if len(sent) == 0 || len(sent) == len(clubs) {
			t.Errorf("some clubs have sent their entries, not all: %d of %d", len(sent), len(clubs))
		}
		if unsigned == 0 {
			t.Error("a gymnast on their own waits for their coach's sign-off")
		}
	})
	t.Run("on the day", func(t *testing.T) {
		if day.Date != now.Format("2006-01-02") || day.Published == nil || len(day.Published.Setup.Days) != 3 {
			t.Fatalf("day 1 is today (%s) and three days are published", day.Date)
		}
		times, _ := s.st.FlightTimes(ctx, day.ID)
		started, finished := 0, 0
		for _, ft := range times {
			if ft.Started.In(local).Format("2006-01-02") != day.Date {
				t.Errorf("a flight started on another day: %v", ft.Started)
			}
			if !ft.Started.IsZero() {
				started++
			}
			if !ft.Finished.IsZero() {
				finished++
			}
		}
		if finished == 0 || started <= finished {
			t.Errorf("flights finished, and some under way: %d started, %d finished", started, finished)
		}
		checkins, _ := s.st.Checkins(ctx, day.ID)
		scratched := 0
		for _, c := range checkins {
			if c.Status == store.Scratched {
				scratched++
			}
		}
		if len(checkins) == 0 || scratched != 1 {
			t.Errorf("gymnasts checked in, one scratched: %d, %d", len(checkins), scratched)
		}
		if panels, _ := s.st.OfficialCheckins(ctx, day.ID); len(panels) == 0 {
			t.Error("panels checked in")
		}
		if up, _ := s.st.UpNow(ctx, day.ID); len(up) == 0 {
			t.Error("who's up, in a flight under way")
		}
		if desk, _ := s.st.DeskMessages(ctx, day.ID); len(desk) < 3 {
			t.Errorf("messages from the desk: %d", len(desk))
		}
		if notices, _ := s.st.ScreenNotices(ctx, day.ID); len(notices) == 0 {
			t.Error("a notice on the venue screen")
		}
	})
	for _, c := range []store.Competition{open, day} {
		t.Run("messages to the organiser of "+c.Name, func(t *testing.T) {
			concerns, err := s.st.Concerns(ctx, c.ID)
			if err != nil {
				t.Fatal(err)
			}
			stages := map[string]bool{}
			for _, k := range concerns {
				last := len(k.Thread) - 1
				switch {
				case !k.Message():
					stages["a helper's concern"] = true
				case k.Open() && k.Resolution != "" && last >= 0 && k.Thread[last].BySender:
					stages["resolved and opened again"] = true
				case k.Open() && !k.Acknowledged():
					stages["just sent"] = true
				case k.Open() && !k.AckAt.IsZero() && len(k.Thread) == 0:
					stages["received"] = true
				case k.Open() && last > 0 && k.Thread[last].BySender && !k.Thread[last-1].BySender:
					stages["replied to and answered"] = true
				case !k.Open() && k.Resolution != "" && last >= 0 && !k.Thread[last].BySender:
					stages["resolved with a reply and a note"] = true
				}
			}
			for _, want := range []string{"a helper's concern", "just sent", "received", "replied to and answered", "resolved with a reply and a note", "resolved and opened again"} {
				if !stages[want] {
					t.Errorf("no message %s", want)
				}
			}
		})
	}
	feedback, _ := s.st.AllSiteFeedback(ctx)
	if len(feedback) == 0 {
		t.Error("feedback on the sandbox's maintainer page")
	}

	// The start page lists the copy's links, and says notifications are off.
	page := do(t, s.h, http.MethodGet, "/sandbox", nil).Body.String()
	for _, want := range []string{l.Open.Organiser, l.Day.Organiser + "/day", l.CardChecker, l.Chair, l.CompSec.Link, l.Coach.Link, l.Member.Link,
		l.Own.Link, l.OwnSignoff, l.Screen, l.Open.Enter, html.EscapeString(views.NotificationsOff)} {
		if want == "" || !strings.Contains(page, want) {
			t.Errorf("the start page doesn't have %q", want)
		}
	}
	// Every link on it opens.
	for _, href := range []string{l.Open.Organiser, l.Day.Organiser + "/day", l.CardChecker, l.Chair, l.CompSec.Link, l.Coach.Link, l.Member.Link,
		l.Member.Link + "/competitions/" + l.Day.ID + "/day", l.Own.Link, l.OwnSignoff, l.Screen + "/screen", l.Open.Enter} {
		if rec := do(t, s.h, http.MethodGet, href, nil); rec.Code != http.StatusOK {
			t.Errorf("%s: %d", href, rec.Code)
		}
	}
	// The club's links with a role (ADR 0015), each on the start page and
	// opening the club as its role.
	if len(l.ClubRoles) != 2 {
		t.Fatalf("the club's links with a role: %+v", l.ClubRoles)
	}
	for _, r := range l.ClubRoles {
		if !strings.Contains(page, r.Link) {
			t.Errorf("the start page doesn't have %s's link", r.Role)
		}
		if body := do(t, s.h, http.MethodGet, r.Link, nil).Body.String(); !strings.Contains(body, html.EscapeString(r.Name+" ("+r.Role+")")) {
			t.Errorf("%s's link doesn't open the club as %s", r.Name, r.Role)
		}
	}
	// Members keep routines and mark skills: the start page's member has
	// both, and the club's Members links to them.
	members := do(t, s.h, http.MethodGet, l.CompSec.Link+"/members", nil).Body.String()
	// Their row, by the row's own name (data-name): a coach of the same name
	// is an option in the coach filter and other rows' dropdowns, so the name
	// on its own matched them now and then (the seed is random).
	row := ""
	named := `data-name="` + html.EscapeString(l.Member.Name) + `"`
	for _, tr := range strings.Split(members, "<tr ")[1:] {
		if head, _, _ := strings.Cut(tr, ">"); strings.Contains(head, named) {
			row = tr
			break
		}
	}
	for _, want := range []string{"/routines\"", "can do "} {
		if !strings.Contains(row, want) {
			t.Errorf("the club's Members doesn't link %s's %q: %q", l.Member.Name, want, row)
		}
	}
	if body := do(t, s.h, http.MethodGet, l.Member.Link+"/skills", nil).Body.String(); !strings.Contains(body, "Can do") {
		t.Error("the start page's member has marked no skills")
	}
}

// A reset makes a new copy and deletes the old, leaving nothing of it and
// touching nothing else; a link from the old copy leads to the start page.
func TestSandboxReset(t *testing.T) {
	s := newTestServer(t, inSandbox())
	ctx := context.Background()
	// Something made in the sandbox outside a copy: a visitor's own club.
	club := withoutQuery(redirected(t, s.h, "/clubs", url.Values{"name": {"Visitors"}}))
	old, _, _ := sandboxCopy(t, s)
	l, _, _ := sandboxCopy(t, s)
	if l.Open.Organiser == old.Open.Organiser {
		t.Fatal("the same links after a reset")
	}
	copies, _ := s.st.SandboxCopies(ctx)
	if len(copies) != 1 {
		t.Errorf("one copy kept: %d", len(copies))
	}
	// The old copy's competitions and clubs are gone.
	for _, admin := range []string{old.Open.Organiser, old.Day.Organiser} {
		if _, err := s.st.CompetitionByAdmin(ctx, strings.TrimPrefix(admin, adminPath(""))); err == nil {
			t.Errorf("the old copy's competition at %s is still there", admin)
		}
	}
	if _, err := s.st.ClubByAdmin(ctx, strings.TrimPrefix(old.CompSec.Link, clubPath(""))); err == nil {
		t.Error("the old copy's club is still there")
	}
	if rec := do(t, s.h, http.MethodGet, club, nil); rec.Code != http.StatusOK {
		t.Errorf("the visitor's club is untouched: %d", rec.Code)
	}
	for _, link := range []string{old.Open.Organiser, old.Member.Link, old.CompSec.Link} {
		rec := do(t, s.h, http.MethodGet, link, nil)
		if body := rec.Body.String(); rec.Code != http.StatusNotFound || !strings.Contains(body, "This was in an earlier sandbox") || !strings.Contains(body, `href="/sandbox"`) {
			t.Errorf("%s: %d, not the earlier sandbox's page", link, rec.Code)
		}
	}

	// The maintainer's page says when it was reset, with the buttons.
	token, err := s.st.NewMaintainerLink(ctx)
	if err != nil {
		t.Fatal(err)
	}
	page := do(t, s.h, http.MethodGet, "/site/"+token, nil).Body.String()
	for _, want := range []string{"Last reset", "Reset the sandbox", "Restart the day"} {
		if !strings.Contains(page, want) {
			t.Errorf("the maintainer's page doesn't say %q", want)
		}
	}
}

// Nothing is emailed or pushed in the sandbox, even with email set up and
// people asking to hear, and where it would be, it says so.
func TestSandboxSendsNothing(t *testing.T) {
	s := newTestServer(t, inSandbox())
	ctx := context.Background()
	l, _, day := sandboxCopy(t, s)
	// Email and push as if set up, and FEEDBACK_EMAIL.
	mail, pushes := &fakeMailer{}, &pushCounter{got: map[string]int{}}
	s.p.notify.mail, s.p.notify.http, s.p.feedbackTo = mail, &http.Client{Transport: pushes}, "maintainer@example.com"

	entries, _ := s.st.Entries(ctx, day.ID)
	var member store.Entry
	for _, e := range entries {
		if e.MemberID != "" {
			member = e
			break
		}
	}
	// People in their club asking to hear, by email and by push.
	subscribe(t, s.st, day.ID, member.ClubID, store.ByEmail, []string{"someone@example.com"})
	subscribe(t, s.st, day.ID, member.ClubID, store.ByPush, []string{"https://push.example/1"})

	// A desk message to their club.
	rec := do(t, s.h, http.MethodPost, l.Day.Organiser+"/desk", url.Values{"to": {"club"}, "club": {member.ClubID}, "text": {"Numbers at the desk"}})
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, url.QueryEscape(views.NotificationsOff)) {
		t.Errorf("the desk's notice: %s", loc)
	}
	// A reply to a message.
	concerns, _ := s.st.Concerns(ctx, day.ID)
	for _, k := range concerns {
		if k.Message() && k.Open() {
			rec := do(t, s.h, http.MethodPost, l.Day.Organiser+"/concerns/"+k.ID+"/reply", url.Values{"reply": {"Thanks"}})
			if loc := rec.Header().Get("Location"); !strings.Contains(loc, url.QueryEscape(views.NotificationsOff)) {
				t.Errorf("the reply's notice: %s", loc)
			}
			break
		}
	}
	// Changes, told once due.
	s.p.notify.now = func() time.Time { return time.Now().Add(time.Hour) }
	tellDue(s.p)
	// Feedback.
	do(t, s.h, http.MethodPost, "/feedback", url.Values{"page": {"/"}, "text": {"Hmm"}})
	// Tell me about changes: says so, and saves nothing.
	notify := l.Member.Link + "/competitions/" + l.Day.ID + "/notify"
	if page := do(t, s.h, http.MethodGet, notify, nil).Body.String(); !strings.Contains(page, html.EscapeString(views.NotificationsOff)) || strings.Contains(page, `name="email"`) {
		t.Error("Tell me about changes doesn't say notifications don't work, or offers email")
	}
	rec = do(t, s.h, http.MethodPost, notify, url.Values{"email": {"me@example.com"}, "topic": {store.AboutTimetable}, "adult": {"1"}})
	if loc := rec.Header().Get("Location"); rec.Code != http.StatusSeeOther || !strings.Contains(loc, url.QueryEscape(views.NotificationsOff)) {
		t.Errorf("asking to hear: %d %s", rec.Code, loc)
	}
	s.p.notify.send.flush()
	if len(mail.sent) > 0 || len(pushes.got) > 0 {
		t.Errorf("sent %d emails and pushed to %d phones", len(mail.sent), len(pushes.got))
	}
}

// Every page says it's the sandbox, in the sandbox only.
func TestSandboxBanner(t *testing.T) {
	banner := "Sandbox: made-up people."
	sandbox, live := newTestServer(t, inSandbox()), newTestServer(t)
	for _, path := range []string{"/", "/compare", "/competitions", "/competitions/new", "/no-such-page"} {
		if body := do(t, sandbox.h, http.MethodGet, path, nil).Body.String(); !strings.Contains(body, banner) {
			t.Errorf("%s in the sandbox has no banner", path)
		}
		if body := do(t, live.h, http.MethodGet, path, nil).Body.String(); strings.Contains(body, banner) {
			t.Errorf("%s on the live site has the sandbox's banner", path)
		}
	}
	if rec := do(t, live.h, http.MethodGet, "/sandbox", nil); rec.Code != http.StatusNotFound {
		t.Errorf("the live site has no start page: %d", rec.Code)
	}
	if body := do(t, sandbox.h, http.MethodGet, "/sandbox", nil).Body.String(); !strings.Contains(body, "being set up") {
		t.Error("the start page before the first reset says it's being set up")
	}
}

// On the 4th day the day starts again: day 1 today, the same links, the
// morning played again and the messages kept.
func TestSandboxRestartsTheDay(t *testing.T) {
	s := newTestServer(t, inSandbox())
	ctx := context.Background()
	l, _, day := sandboxCopy(t, s)
	before, _ := s.st.Concerns(ctx, day.ID)
	now := time.Now()
	if did, err := s.p.restartDay(ctx, now.AddDate(0, 0, 2), false); err != nil || did {
		t.Errorf("on day 3 it doesn't: %v, %v", did, err)
	}
	// Three days on: the 4th day.
	later := now.AddDate(0, 0, 3)
	did, err := s.p.restartDay(ctx, later, false)
	if err != nil || !did {
		t.Fatalf("on the 4th day it does: %v, %v", did, err)
	}
	cp, _ := s.st.SharedCopy(ctx)
	after, _ := sandboxLinks(cp)
	if !reflect.DeepEqual(after, l) {
		t.Error("the links changed")
	}
	c, _ := s.st.Competition(ctx, day.ID)
	if want := later.In(local).Format("2006-01-02"); c.Date != want {
		t.Errorf("day 1 is %s, want %s", c.Date, want)
	}
	times, _ := s.st.FlightTimes(ctx, day.ID)
	if len(times) == 0 {
		t.Error("the morning is played again")
	}
	for _, ft := range times {
		if ft.Started.In(local).Format("2006-01-02") != c.Date {
			t.Errorf("a flight started on %v, not the new day 1", ft.Started)
		}
	}
	if kept, _ := s.st.Concerns(ctx, day.ID); len(kept) != len(before) {
		t.Errorf("messages kept: %d, were %d", len(kept), len(before))
	}
}

// The sandbox at ISTO's size is made in about half a minute: go test
// -short leaves it out.
func TestSandboxISTOSize(t *testing.T) {
	if testing.Short() {
		t.Skip("seeds ISTO's size")
	}
	s := newTestServer(t, inSandbox())
	s.p.sandbox.size = sandboxSize
	took, err := s.p.resetSandbox(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if took > time.Minute {
		t.Errorf("took %s", took)
	}
	t.Logf("reset in %s", took.Round(time.Second/10))
}
