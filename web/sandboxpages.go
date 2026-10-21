package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"tariffCalculator/demo"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// The sandbox (ADR 0016): the app run a second time, with SANDBOX=1, its
// own data and address, full of made-up people to try it on. Nothing is
// emailed or pushed, every page says it's the sandbox, and feedback stays
// on its own maintainer's page. A seed (demo.Sandbox) makes a copy: two
// competitions, one open for entries and one on the day; the start page
// (/sandbox) hands out its links by role. The maintainer resets it, which
// makes a new copy before deleting the old; the competition on the day
// starts its day again by itself once its three days are over.

// sandboxSize is the shared sandbox's size: ISTO's, about 25 clubs and 400
// gymnasts.
var sandboxSize = demo.Full

// sandboxState is the sandbox's work under way: one reset or restart at a
// time.
type sandboxState struct {
	mu      sync.Mutex
	busy    string    // what's under way, "" for nothing
	started time.Time // when it started
	failed  string    // why the last one failed, "" if it didn't
	size    demo.Size
}

// begin starts what, unless something is under way; it says whether it did.
func (s *sandboxState) begin(what string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy != "" {
		return false
	}
	s.busy, s.started = what, time.Now()
	return true
}

// end ends what's under way, keeping why it failed.
func (s *sandboxState) end(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.busy, s.failed = "", ""
	if err != nil {
		s.failed = err.Error()
	}
}

// makeSandbox makes the pages the sandbox's: nothing emailed or pushed
// (FEEDBACK_EMAIL too), whatever is set up.
func (p *competitionPages) makeSandbox() {
	p.sandbox = &sandboxState{size: sandboxSize}
	p.feedbackTo = ""
	if p.notify != nil {
		p.notify.sandbox, p.notify.mail = true, nil
	}
}

// sendingNotice says who a desk message is being sent to, by push and
// email; in the sandbox, that notifications don't work.
func (p *competitionPages) sendingNotice(pushes, emails int) string {
	if p.sandbox != nil {
		return views.NotificationsOff
	}
	return sendingNotice(pushes, emails)
}

func (p *competitionPages) registerSandbox(mux *http.ServeMux) {
	p.notePage("GET /sandbox")
	mux.HandleFunc("GET /sandbox", p.public(p.sandboxStart))
	mux.Handle("POST /site/{token}/sandbox/reset", p.maintainer(p.sandboxReset))
	mux.Handle("POST /site/{token}/sandbox/restart", p.maintainer(p.sandboxRestart))
}

// sandboxAt is the site at a time, for the sandbox's seed: a handler of its
// own, the sandbox's, over the store with its clock reading now.
func (p *competitionPages) sandboxAt(now func() time.Time) http.Handler {
	q := newCompetitionPages(p.st.Clocked(now))
	q.now = now
	q.makeSandbox()
	q.notify.now = now
	// The seed sends feedback, whatever the maintainer has chosen.
	q.siteFeedback.Store(&store.FeedbackSettings{})
	return routesWithPages(q)
}

// sandboxLinks are a copy's links, as its seed gave them.
func sandboxLinks(c store.SandboxCopy) (demo.Links, error) {
	var l demo.Links
	err := json.Unmarshal(c.Links, &l)
	return l, err
}

// sandboxStart is the start page, "Try it as…": a card for each role, from
// the shared copy's links.
func (p *competitionPages) sandboxStart(w http.ResponseWriter, r *http.Request) {
	if p.sandbox == nil || p.st == nil {
		notFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	c, err := p.st.SharedCopy(r.Context())
	if errors.Is(err, store.ErrNotFound) {
		render(w, r, views.SandboxStart(views.SandboxPage{}))
		return
	}
	if err != nil {
		failed(w, r, err)
		return
	}
	l, err := sandboxLinks(c)
	if err != nil {
		failed(w, r, err)
		return
	}
	page := views.SandboxPage{Ready: true, Cards: sandboxCards(l)}
	if at, _, err := p.st.SandboxReset(r.Context()); err == nil && !at.IsZero() {
		page.Reset = at.In(local).Format("Mon 2 Jan, 15:04")
	}
	render(w, r, views.SandboxStart(page))
}

// sandboxCards are the start page's cards: who to try it as, what to try,
// and their links.
func sandboxCards(l demo.Links) []views.SandboxCard {
	link := func(label, href string) views.SandboxLink { return views.SandboxLink{Label: label, Href: href} }
	who := func(p demo.Person) string {
		if p.Club == "" {
			return p.Name
		}
		return p.Name + ", " + p.Club
	}
	cards := []views.SandboxCard{
		{Role: "Organiser", Try: "Entries, the timetable and On the day: plan the timetable of the competition open for entries; run the day of the other, which is under way.",
			Links: []views.SandboxLink{link("Entries open", l.Open.Organiser), link("On the day", l.Day.Organiser+"/day"), link("Its timetable", l.Day.Organiser+"/timetable")}},
		{Role: "A helper", Try: "Check the gymnasts' cards while entries are open, or chair a panel's judges on the day.",
			Links: []views.SandboxLink{link("Checking cards", l.CardChecker), link("A chair of judges", l.Chair)}},
		{Role: "A comp sec", Who: who(l.CompSec), Try: "Enter your club's members and send the club's entries; send the organiser a message.",
			Links: []views.SandboxLink{link("The club's page", l.CompSec.Link)}},
		{Role: "A coach", Who: who(l.Coach), Try: "Sign off your gymnasts' routines, and see the skills they're learning.",
			Links: []views.SandboxLink{link("Their page", l.Coach.Link)}},
		{Role: "A member", Who: who(l.Member), Try: "Enter the competition open for entries; see My competition on the day, and how you'd hear about changes.",
			Links: []views.SandboxLink{link("Their page", l.Member.Link), link("My competition", l.Member.Link+"/competitions/"+l.Day.ID+"/day"),
				link("Tell me about changes", l.Member.Link+"/competitions/"+l.Day.ID+"/notify")}},
		{Role: "A gymnast on their own", Who: who(l.Own), Try: "Enter without a club, and have a coach sign off the entry by link.",
			Links: []views.SandboxLink{link("Their page", l.Own.Link), link("Their coach's sign-off", l.OwnSignoff), link("Enter as someone new", l.Open.Enter)}},
		{Role: "The venue screen", Try: "What's on each area now and next, and the organisers' announcements, as the hall's screen shows them.",
			Links: []views.SandboxLink{link("The screen", l.Screen)}},
		// A club role's card (ADR 0015) goes here once club roles are built.
	}
	return cards
}

// sandboxReset resets the sandbox in the background: one at a time.
func (p *competitionPages) sandboxReset(w http.ResponseWriter, r *http.Request) {
	back := maintainerPath(r.PathValue("token"))
	if p.sandbox == nil {
		notFound(w, r)
		return
	}
	if !p.sandbox.begin("Resetting the sandbox") {
		seeOther(w, r, back, "Something is under way already: wait for it to finish.")
		return
	}
	go func() {
		took, err := p.resetSandbox(context.Background())
		if err != nil {
			log.Printf("Resetting the sandbox: %v", err)
		} else {
			log.Printf("Reset the sandbox in %s", took.Round(time.Second))
		}
		p.sandbox.end(err)
	}()
	seeOther(w, r, back, "Resetting the sandbox: it takes about half a minute. The old one works until the new one is ready.")
}

// sandboxRestart restarts the day of the competition on the day, now, in
// the background.
func (p *competitionPages) sandboxRestart(w http.ResponseWriter, r *http.Request) {
	back := maintainerPath(r.PathValue("token"))
	if p.sandbox == nil {
		notFound(w, r)
		return
	}
	if !p.sandbox.begin("Restarting the day") {
		seeOther(w, r, back, "Something is under way already: wait for it to finish.")
		return
	}
	go func() {
		_, err := p.restartDay(context.Background(), time.Now(), true)
		if err != nil {
			log.Printf("Restarting the sandbox's day: %v", err)
		}
		p.sandbox.end(err)
	}()
	seeOther(w, r, back, "Restarting the day: it takes a few seconds.")
}

// resetSandbox makes a new shared copy, then deletes the old ones, so the
// start page always has one that works. A copy whose seed failed partway is
// deleted at once. It returns how long it took.
func (p *competitionPages) resetSandbox(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	cp, seedErr := demo.Sandbox(p.sandboxAt, p.sandbox.size, start)
	id, err := p.saveCopy(ctx, cp, start)
	if seedErr != nil {
		if err == nil {
			err = p.st.DeleteCopy(ctx, id)
		}
		return 0, errors.Join(seedErr, err)
	}
	if err != nil {
		return 0, err
	}
	copies, err := p.st.SandboxCopies(ctx)
	if err != nil {
		return 0, err
	}
	for _, c := range copies {
		if c.ID != id && c.ExpiresAt.IsZero() {
			if err := p.st.DeleteCopy(ctx, c.ID); err != nil {
				return 0, err
			}
		}
	}
	took := time.Since(start)
	return took, p.st.SetSandboxReset(ctx, start, took)
}

// saveCopy keeps what a seed made as a copy, by the ids its links lead to.
func (p *competitionPages) saveCopy(ctx context.Context, cp demo.Copy, at time.Time) (string, error) {
	var comps, clubs []string
	for _, admin := range cp.Competitions {
		c, err := p.st.CompetitionByAdmin(ctx, strings.TrimPrefix(admin, adminPath("")))
		if err != nil {
			return "", fmt.Errorf("the competition at %s: %w", admin, err)
		}
		comps = append(comps, c.ID)
	}
	for _, admin := range cp.Clubs {
		c, err := p.st.ClubByAdmin(ctx, strings.TrimPrefix(admin, clubPath("")))
		if err != nil {
			return "", fmt.Errorf("a club: %w", err)
		}
		clubs = append(clubs, c.ID)
	}
	links, err := json.Marshal(cp.Links)
	if err != nil {
		return "", err
	}
	return p.st.SaveCopy(ctx, store.SandboxCopy{CreatedAt: at, Size: p.sandbox.size.Clubs, Links: links}, comps, clubs)
}

// sandboxDays is how many days the competition on the day has.
const sandboxDays = 3

// restartDay starts the shared copy's competition on the day again from
// day 1 today, and plays its morning again: when its days are over, or now
// if forced. It says whether it did.
func (p *competitionPages) restartDay(ctx context.Context, now time.Time, force bool) (bool, error) {
	cp, err := p.st.SharedCopy(ctx)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	l, err := sandboxLinks(cp)
	if err != nil {
		return false, err
	}
	c, err := p.st.CompetitionByAdmin(ctx, strings.TrimPrefix(l.Day.Organiser, adminPath("")))
	if err != nil {
		return false, err
	}
	first, err := c.Day()
	if err != nil {
		return false, err
	}
	y, m, d := now.In(local).Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	if !force && today.Before(first.AddDate(0, 0, sandboxDays)) {
		return false, nil
	}
	if err := p.st.RestartDay(ctx, c.ID, today); err != nil {
		return false, err
	}
	return true, demo.Morning(p.sandboxAt, l.Day.Organiser, now)
}

// rollDay restarts the sandbox's day once its days are over: when the app
// starts and every hour. It never returns: run it in a goroutine.
func (p *competitionPages) rollDay() {
	for {
		if p.sandbox.begin("Restarting the day") {
			did, err := p.restartDay(context.Background(), time.Now(), false)
			switch {
			case err != nil:
				log.Printf("Restarting the sandbox's day: %v", err)
			case did:
				log.Print("Restarted the sandbox's day")
			}
			p.sandbox.end(err)
		}
		time.Sleep(time.Hour)
	}
}

// sandboxPanel is the sandbox's part of the maintainer's page; nil outside
// the sandbox.
func (p *competitionPages) sandboxPanel(ctx context.Context, base string) *views.SandboxPanel {
	if p.sandbox == nil {
		return nil
	}
	panel := &views.SandboxPanel{Base: base, StartPage: "/sandbox"}
	if at, took, err := p.st.SandboxReset(ctx); err == nil && !at.IsZero() {
		panel.Reset = at.In(local).Format("Mon 2 Jan, 15:04")
		panel.Took = views.Count(int(took.Round(time.Second).Seconds()), "second", "seconds")
	}
	if cp, err := p.st.SharedCopy(ctx); err == nil {
		if l, err := sandboxLinks(cp); err == nil {
			if c, err := p.st.CompetitionByAdmin(ctx, strings.TrimPrefix(l.Day.Organiser, adminPath(""))); err == nil {
				if day, err := c.Day(); err == nil {
					panel.Day = day.Format("Mon 2 Jan")
				}
			}
		}
	}
	p.sandbox.mu.Lock()
	if p.sandbox.busy != "" {
		panel.Busy = p.sandbox.busy + ", since " + p.sandbox.started.In(local).Format("15:04")
	}
	panel.Failed = p.sandbox.failed
	p.sandbox.mu.Unlock()
	return panel
}

// earlierSandbox is the page for a link the sandbox no longer has: from
// before it was reset.
func earlierSandbox(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex")
	w.WriteHeader(http.StatusNotFound)
	render(w, r, views.EarlierSandbox())
}

// sandboxLink says whether a path is one of the links a sandbox copy hands
// out, under a club's or a competition's: one that leads nowhere is from
// before a reset.
func sandboxLink(path string) bool {
	return strings.HasPrefix(path, "/clubs/") || strings.HasPrefix(path, "/competitions/")
}
