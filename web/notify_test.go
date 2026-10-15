package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"tariffCalculator/store"
)

// sentEmail is an email the fake mailer was given.
type sentEmail struct {
	to, subject, body string
	headers           map[string]string
}

// fakeMailer keeps what it's sent. The sender's workers send at once, so
// sent is read only once they're done (the test kit flushes the sender).
type fakeMailer struct {
	mu   sync.Mutex
	sent []sentEmail
}

func (m *fakeMailer) send(to, subject, body string, headers map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, sentEmail{to, subject, body, headers})
	return nil
}

// notifyServer is the competition pages with email on, to a fake mailer.
func notifyServer(t *testing.T) (http.Handler, *competitionPages, *fakeMailer) {
	t.Helper()
	s := newTestServer(t, withMail())
	return s.h, s.p, s.mail
}

func TestNotifications(t *testing.T) {
	h, p, mail := notifyServer(t)
	admin := created(t, h, newCompetition())
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	own := withoutQuery(redirected(t, h, enter, bucsEntry("Ann Ryan")))
	if !strings.Contains(do(t, h, http.MethodGet, own, nil).Body.String(), own+"/notify") {
		t.Fatal("the entry links to hearing about changes")
	}

	// Asking: an email, the tick box and something to hear about.
	notify := own + "/notify"
	if rec := do(t, h, http.MethodPost, notify, url.Values{"email": {"ann@example.com"}, "topic": {"cards"}}); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "18 or over") {
		t.Errorf("the tick box is needed: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, notify, url.Values{"email": {"Ann <ann@example.com>"}, "adult": {"1"}, "topic": {"cards"}}); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("only a plain address: %d", rec.Code)
	}
	redirected(t, h, notify, url.Values{"email": {"ann@example.com"}, "adult": {"1"}, "topic": {"cards", "timetable"}})
	if len(mail.sent) != 1 || mail.sent[0].to != "ann@example.com" || !strings.Contains(mail.sent[0].body, "https://tariff.example/notify/confirm/") {
		t.Fatalf("a link to confirm it: %+v", mail.sent)
	}
	// It goes to any address typed in, so it carries no name anyone chose.
	if got := mail.sent[0]; got.subject != "Confirm your email" || strings.Contains(got.body, "Student Open") ||
		!strings.Contains(got.body, "a competition on tariff.example.") {
		t.Errorf("no competition's name: %+v", got)
	}
	if page := do(t, h, http.MethodGet, notify, nil).Body.String(); !strings.Contains(page, "Waiting for you to confirm it") {
		t.Error("waiting to be confirmed")
	}
	confirm := strings.TrimPrefix(regexp.MustCompile(`https://tariff\.example(/notify/confirm/\S+)`).FindString(mail.sent[0].body), "https://tariff.example")

	// Not confirmed: nothing waits or is sent.
	entry := regexp.MustCompile(`href="(` + regexp.QuoteMeta(admin) + `/entries/[^"]+)"`).FindStringSubmatch(do(t, h, http.MethodGet, admin, nil).Body.String())[1]
	redirected(t, h, entry+"/check", url.Values{"checked": {"1"}})
	if strings.Contains(do(t, h, http.MethodGet, admin, nil).Body.String(), "Notify now") {
		t.Error("nobody to tell yet")
	}
	redirected(t, h, entry+"/check", url.Values{"checked": {"0"}})

	// Confirmed (a button, so a link checker doesn't).
	if rec := do(t, h, http.MethodGet, confirm, nil); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Confirm ann@example.com") {
		t.Errorf("the confirm page: %d", rec.Code)
	}
	do(t, h, http.MethodPost, confirm, nil)
	if page := do(t, h, http.MethodGet, notify, nil).Body.String(); !strings.Contains(page, ">On<") {
		t.Error("confirmed")
	}

	// Checked and unchecked within the wait: nothing to tell.
	mail.sent = nil
	redirected(t, h, entry+"/check", url.Values{"checked": {"1"}})
	redirected(t, h, entry+"/check", url.Values{"checked": {"0"}})
	redirected(t, h, admin+"/notify-now", nil)
	tellDue(p)
	if len(mail.sent) != 0 {
		t.Errorf("a change put right tells no one: %+v", mail.sent)
	}

	// Checked with a note: told after the wait, or at once with Notify now.
	redirected(t, h, entry+"/check", url.Values{"checked": {"1"}, "note": {"Element 7 repeats 3"}})
	tellDue(p)
	if len(mail.sent) != 0 {
		t.Error("not before the wait")
	}
	if !strings.Contains(do(t, h, http.MethodGet, admin, nil).Body.String(), "Notify now") {
		t.Error("the organiser sees changes waiting")
	}
	redirected(t, h, admin+"/notify-now", nil)
	tellDue(p)
	if len(mail.sent) != 1 {
		t.Fatalf("one email: %+v", mail.sent)
	}
	got := mail.sent[0]
	if got.subject != "A change at Student Open" || !strings.Contains(got.body, "Ann Ryan\n- BUCS L3: card checked, with a note: Element 7 repeats 3") ||
		!strings.Contains(got.body, "https://tariff.example/competitions") || strings.Contains(got.body, own) || !strings.Contains(got.headers["List-Unsubscribe"], "/notify/off/") {
		t.Errorf("the email: %+v", got)
	}
	if strings.Contains(do(t, h, http.MethodGet, admin, nil).Body.String(), "Notify now") {
		t.Error("nothing waiting once told")
	}

	// Stopping from the email's link.
	off := strings.TrimPrefix(regexp.MustCompile(`https://tariff\.example(/notify/off/\S+)`).FindString(got.body), "https://tariff.example")
	do(t, h, http.MethodPost, off, url.Values{"List-Unsubscribe": {"One-Click"}})
	if page := do(t, h, http.MethodGet, notify, nil).Body.String(); strings.Contains(page, "ann@example.com") {
		t.Error("stopped")
	}
	if rec := do(t, h, http.MethodGet, off, nil); rec.Code != http.StatusNotFound {
		t.Errorf("the link is gone: %d", rec.Code)
	}
}

// An individual's notifications are theirs, not the entry's they signed up
// on (ADR 0010): withdrawing that entry still leaves them hearing about the
// other.
func TestIndividualNotificationsOutliveAnEntry(t *testing.T) {
	h, p, mail := notifyServer(t)
	form := newCompetition()
	form.Set("tumbling", "Novice")
	admin := created(t, h, form)
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	own := withoutQuery(redirected(t, h, enter, bucsEntry("Ann Ryan")))
	redirected(t, h, enter, url.Values{"gymnast": {"Ann Ryan"}, "discipline": {"tumbling"}, "level": {"Novice"}, "as": {"http://example.com" + own}})
	redirected(t, h, own+"/notify", url.Values{"email": {"ann@example.com"}, "adult": {"1"}, "topic": {"cards"}})
	confirm := strings.TrimPrefix(regexp.MustCompile(`https://tariff\.example(/notify/confirm/\S+)`).FindString(mail.sent[0].body), "https://tariff.example")
	do(t, h, http.MethodPost, confirm, nil)

	if rec := do(t, h, http.MethodPost, own+"/withdraw", url.Values{"confirm": {"1"}}); rec.Code != http.StatusOK {
		t.Fatalf("withdrawn: %d", rec.Code)
	}
	entries := regexp.MustCompile(`href="(`+regexp.QuoteMeta(admin)+`/entries/[^"]+)"`).FindAllStringSubmatch(do(t, h, http.MethodGet, admin, nil).Body.String(), -1)
	if len(entries) != 1 {
		t.Fatalf("one entry left: %v", entries)
	}
	mail.sent = nil
	redirected(t, h, entries[0][1]+"/check", url.Values{"checked": {"1"}, "note": {"Element 3 repeats"}})
	redirected(t, h, admin+"/notify-now", nil)
	tellDue(p)
	if len(mail.sent) != 1 || !strings.Contains(mail.sent[0].body, "Novice: card checked, with a note: Element 3 repeats") {
		t.Errorf("Ann still hears about her tumbling entry: %+v", mail.sent)
	}
}

func TestNotificationsWithoutEmail(t *testing.T) {
	h := competitionServer(t) // no email configured
	admin := created(t, h, newCompetition())
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	own := withoutQuery(redirected(t, h, enter, bucsEntry("Ann Ryan")))
	if !strings.Contains(do(t, h, http.MethodGet, own, nil).Body.String(), own+"/notify") {
		t.Fatal("push needs nothing set up, so the link is there")
	}
	if page := do(t, h, http.MethodGet, own+"/notify", nil).Body.String(); !strings.Contains(page, "On this phone") || strings.Contains(page, `name="email"`) {
		t.Error("push, but no email")
	}
}

// A club's comp sec hears about every member in one email, and an address
// on two pages gets one email.
func TestNotifyingAClub(t *testing.T) {
	h, p, mail := notifyServer(t)
	admin := created(t, h, newCompetition())
	clubLink := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/club/")
	club := withoutQuery(redirected(t, h, "/clubs", url.Values{"name": {"UCD"}}))
	redirected(t, h, clubLink, url.Values{"clubAdmin": {club}})
	join := pathIn(t, do(t, h, http.MethodGet, club+"/members", nil).Body.String(), "/clubs/join/")
	entry := bucsEntry("")
	var ann string
	for _, name := range []string{"Ann", "Bea"} {
		m := withoutQuery(redirected(t, h, join, url.Values{"name": {name}}))
		redirected(t, h, formAction(t, memberCompetitionPage(t, h, m), m+"/competitions/"), entry)
		if ann == "" {
			ann = m
		}
	}
	redirected(t, h, formAction(t, clubEntriesPage(t, h, club), club+"/competitions/"), url.Values{"which": {"all"}})
	clubPage := clubEntriesPage(t, h, club)
	notify := regexp.MustCompile(regexp.QuoteMeta(club) + `/competitions/[^/"]+/notify`).FindString(clubPage)
	if notify == "" {
		t.Fatal("the club page links to hearing about changes")
	}
	annNotify := regexp.MustCompile(regexp.QuoteMeta(ann) + `/competitions/[^/"]+/notify`).FindString(memberCompetitionPage(t, h, ann))
	if annNotify == "" {
		t.Fatal("Ann's page links to hearing about changes")
	}
	var tokens []string
	for _, page := range []string{notify, annNotify} {
		mail.sent = nil
		redirected(t, h, page, url.Values{"email": {"ucd@example.com"}, "adult": {"1"}, "topic": {"timetable"}})
		confirm := strings.TrimPrefix(regexp.MustCompile(`https://tariff\.example(/notify/confirm/\S+)`).FindString(mail.sent[0].body), "https://tariff.example")
		do(t, h, http.MethodPost, confirm, nil)
		tokens = append(tokens, strings.TrimPrefix(confirm, "/notify/confirm/"))
	}

	// Publishing: one email, both members, both pages.
	mail.sent = nil
	tt := admin + "/timetable"
	redirected(t, h, tt+"/plan", url.Values{})
	redirected(t, h, tt+"/publish", url.Values{"action": {"publish"}})
	redirected(t, h, admin+"/notify-now", nil)
	tellDue(p)
	if len(mail.sent) != 1 {
		t.Fatalf("one email: %d", len(mail.sent))
	}
	body := mail.sent[0].body
	if !strings.Contains(body, "Ann\n- BUCS L3") || !strings.Contains(body, "Bea\n- BUCS L3") || !strings.Contains(body, ": warm-up ") ||
		strings.Contains(body, club) || strings.Contains(body, ann) {
		t.Errorf("both members, and no page links: %s", body)
	}
	if n := strings.Count(body, "/notify/off/"); n != 1 {
		t.Errorf("one off link, for both: %d", n)
	}

	off := strings.TrimPrefix(strings.Trim(mail.sent[0].headers["List-Unsubscribe"], "<>"), "https://tariff.example")

	// Publishing again with nothing changed tells no one.
	mail.sent = nil
	redirected(t, h, tt+"/publish", url.Values{"action": {"publish"}})
	redirected(t, h, admin+"/notify-now", nil)
	tellDue(p)
	if len(mail.sent) != 0 {
		t.Errorf("nothing changed: %+v", mail.sent)
	}

	// The one-click off link in the header stops both.
	do(t, h, http.MethodPost, off, url.Values{"List-Unsubscribe": {"One-Click"}})
	for _, token := range tokens {
		if _, err := p.st.SubscriptionByToken(context.Background(), token); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("stopped, both: %v", err)
		}
	}
}

func TestChanges(t *testing.T) {
	was := notifyState{
		Entries: map[string]entryState{
			"e1": {Gymnasts: "Ann", Event: "BUCS L3", Keys: []string{"m:a"}, Flight: "BUCS L3 · flight 1 of 2", Area: "Panel 1", Time: "Saturday 09:40"},
			"e2": {Gymnasts: "Bea", Event: "BUCS L3", Keys: []string{"m:b"}, Flight: "BUCS L3 · flight 1 of 2", Area: "Panel 1", Time: "Saturday 09:40"},
		},
		Duties: map[string][]string{"m:a": {"Saturday 11:00–12:00 · Panel 2 · BUCS L4 · Recorder"}},
		Names:  map[string]string{"m:a": "Ann", "m:b": "Bea"},
	}
	now := notifyState{
		Entries: map[string]entryState{
			"e1": {Gymnasts: "Ann", Event: "BUCS L3", Keys: []string{"m:a"}, Flight: "BUCS L3 · flight 2 of 2", Area: "Panel 2", Time: "Saturday 10:20"},
			"e2": was.Entries["e2"], // only her place in the running order changed
		},
		Duties: map[string][]string{"m:a": {"Saturday 13:00–14:00 · Panel 2 · BUCS L4 · Recorder"}},
		Names:  was.Names,
	}
	got := changes(was, now)
	var lines []string
	for _, c := range got {
		lines = append(lines, c.Who+": "+c.Line)
	}
	want := []string{
		"Ann: BUCS L3 · flight 2 of 2: now warm-up Saturday 10:20, Panel 2 (was Saturday 09:40, Panel 1)",
		"Ann: officiating: Saturday 13:00–14:00 · Panel 2 · BUCS L4 · Recorder",
		"Ann: no longer officiating: Saturday 11:00–12:00 · Panel 2 · BUCS L4 · Recorder",
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Errorf("changes:\n%s", strings.Join(lines, "\n"))
	}
	if len(changes(now, now)) != 0 {
		t.Error("nothing changed")
	}
}

// A competition sends at most 50 confirmation emails a day, from however
// many addresses they're asked; then a plain notice. A club's name is never
// in one.
func TestConfirmationEmailsPerCompetition(t *testing.T) {
	h, p, mail := notifyServer(t)
	p.emailLimiter = newLimiter(1000, time.Hour) // as if from many places
	admin := created(t, h, newCompetition())
	clubLink := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/club/")
	club := withoutQuery(redirected(t, h, "/clubs", url.Values{"name": {"Click here to win"}}))
	redirected(t, h, clubLink, url.Values{"clubAdmin": {club}})
	notify := regexp.MustCompile(regexp.QuoteMeta(club) + `/competitions/[^/"]+/notify`).FindString(clubEntriesPage(t, h, club))
	if notify == "" {
		t.Fatal("the club page links to hearing about changes")
	}
	for i := range maxConfirmsPerDay {
		redirected(t, h, notify, url.Values{"email": {fmt.Sprintf("a%d@example.com", i)}, "adult": {"1"}, "topic": {"timetable"}})
	}
	if len(mail.sent) != maxConfirmsPerDay {
		t.Fatalf("sent: %d", len(mail.sent))
	}
	if strings.Contains(mail.sent[0].body, "Click here") {
		t.Errorf("no club's name: %s", mail.sent[0].body)
	}
	rec := do(t, h, http.MethodPost, notify, url.Values{"email": {"one@example.com"}, "adult": {"1"}, "topic": {"timetable"}})
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Too many emails asked for at this competition today") || len(mail.sent) != maxConfirmsPerDay {
		t.Errorf("the 51st is refused: %d, %d sent", rec.Code, len(mail.sent))
	}
}
