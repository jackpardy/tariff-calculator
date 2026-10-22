package web

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// madeRows are the maintainer's page's competitions made, newest first:
// name, day and when made; and the line above them.
func madeRows(t *testing.T, h http.Handler, site string) (string, [][3]string) {
	t.Helper()
	rec := do(t, h, http.MethodGet, site, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("the maintainer's page: %d", rec.Code)
	}
	box := firstByClass(parsePage(t, rec.Body.String()), "comp-made")
	if box == nil {
		t.Fatal("the maintainer's page lists the competitions made")
	}
	var rows [][3]string
	for _, tr := range byTag(box, "tr") {
		cells := byTag(tr, "td")
		if len(cells) == 3 {
			rows = append(rows, [3]string{textOf(cells[0]), textOf(cells[1]), textOf(cells[2])})
		}
	}
	return strings.Join(strings.Fields(textOf(firstByClass(box, "comp-opens-now"))), " "), rows
}

// A competition made is emailed to FEEDBACK_EMAIL: its name, its day, when
// and on which site, never its links.
func TestCompetitionMadeEmailed(t *testing.T) {
	s := newTestServer(t, atTime(inDublin(2030, 1, 10, 12, 0)))
	sent := make(emailCatcher, 1)
	s.p.notify.mail, s.p.notify.base, s.p.feedbackTo = sent, "https://tariff.example", "maintainer@example.com"
	admin := created(t, s.h, newCompetition())
	select {
	case e := <-sent:
		if e.to != "maintainer@example.com" || e.subject != "Competition made: Student Open" {
			t.Errorf("the email: %+v", e)
		}
		for _, want := range []string{"Someone made a competition on the live site.", "Competition: Student Open", "Its day: Sat 16 Mar 2030",
			"Made: Thu 10 January 2030, 12:00", "Site: tariff.example"} {
			if !strings.Contains(e.body, want) {
				t.Errorf("the email says %q: %s", want, e.body)
			}
		}
		if strings.Contains(e.body, strings.TrimPrefix(admin, adminPath(""))) {
			t.Error("the email has no link")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no email")
	}

	// Without FEEDBACK_EMAIL, nothing.
	s.p.feedbackTo = ""
	created(t, s.h, newCompetition())
	select {
	case e := <-sent:
		t.Errorf("emailed without FEEDBACK_EMAIL: %+v", e)
	default:
	}
}

// The maintainer's page lists the competitions made, newest first, saying
// it's the live site.
func TestCompetitionsMadeListed(t *testing.T) {
	s := newTestServer(t, atTime(inDublin(2030, 1, 10, 12, 0)))
	site := maintainerLink(t, s)
	if line, rows := madeRows(t, s.h, site); line != "Competitions made on the live site none yet" || len(rows) != 0 {
		t.Errorf("before any: %q %v", line, rows)
	}
	first := created(t, s.h, newCompetition())
	s.clock.set(inDublin(2030, 1, 11, 9, 30))
	form := newCompetition()
	form.Set("name", "Spring Open")
	form.Set("date", "2030-04-06")
	created(t, s.h, form)

	line, rows := madeRows(t, s.h, site)
	if want := "Competitions made on the live site 2 competitions; the newest, Spring Open, made Fri 11 Jan, 09:30"; line != want {
		t.Errorf("the line: %q, want %q", line, want)
	}
	want := [][3]string{{"Spring Open", "Sat 6 Apr 2030", "Fri 11 Jan, 09:30"}, {"Student Open", "Sat 16 Mar 2030", "Thu 10 Jan, 12:00"}}
	if len(rows) != len(want) || rows[0] != want[0] || rows[1] != want[1] {
		t.Errorf("the competitions made: %v, want %v", rows, want)
	}
	if page := do(t, s.h, http.MethodGet, site, nil).Body.String(); strings.Contains(page, strings.TrimPrefix(first, adminPath(""))) {
		t.Error("the maintainer's page has no competition's link")
	}
}

// In the sandbox, the maintainer's page lists what people made there, not
// the seed's competitions, and says it's the sandbox; nothing is emailed.
func TestSandboxCompetitionsMade(t *testing.T) {
	s := newTestServer(t, inSandbox())
	sandboxCopy(t, s)
	mail := &fakeMailer{}
	s.p.notify.mail, s.p.feedbackTo = mail, "maintainer@example.com"
	site := maintainerLink(t, s)
	if line, rows := madeRows(t, s.h, site); line != "Competitions made in the sandbox none yet" || len(rows) != 0 {
		t.Errorf("the seed's are left out: %q %v", line, rows)
	}
	created(t, s.h, newCompetition())
	if _, rows := madeRows(t, s.h, site); len(rows) != 1 || rows[0][0] != "Student Open" {
		t.Errorf("the one made: %v", rows)
	}
	s.p.notify.send.flush()
	if len(mail.sent) > 0 {
		t.Errorf("the sandbox emailed: %+v", mail.sent)
	}
}
