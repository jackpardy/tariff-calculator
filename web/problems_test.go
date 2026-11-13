package web

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"tariffCalculator/store"
)

const problemToken = "AbCdEfGhIjKlMnOpQrStUv"

// problemServer is a test server with email on, the maintainer's address
// set, and a handler with routes that panic and fail, as the app's do: the
// recover around them, and the problems in the request's context.
func problemServer(t *testing.T) (*testServer, http.Handler) {
	t.Helper()
	was := log.Writer()
	log.SetOutput(&bytes.Buffer{})
	t.Cleanup(func() { log.SetOutput(was) })
	s := newTestServer(t, withMail(), atTime(inDublin(2030, 1, 10, 12, 0)))
	s.p.problems.to = "maintainer@example.com"
	mux := http.NewServeMux()
	mux.HandleFunc("GET /clubs/member/{token}", func(w http.ResponseWriter, r *http.Request) {
		panic("boom for " + r.URL.Path)
	})
	mux.HandleFunc("GET /competitions/admin/{token}/x", func(w http.ResponseWriter, r *http.Request) {
		failed(w, r, errors.New("the disk is on fire at /competitions/admin/"+problemToken+"/x, ask ann@example.com"))
	})
	mux.HandleFunc("GET /gone/{token}", func(w http.ResponseWriter, r *http.Request) {
		failed(w, r, fmt.Errorf("fetching: %w", context.Canceled))
	})
	mux.HandleFunc("GET /missing/{token}", func(w http.ResponseWriter, r *http.Request) {
		failed(w, r, store.ErrNotFound)
	})
	h := withRecover(mux)
	return s, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r.WithContext(withProblems(r.Context(), s.p.problems)))
	})
}

// told is the emails the maintainer has had, once the sender has sent them.
func told(s *testServer) []sentEmail {
	s.p.notify.send.flush()
	s.mail.mu.Lock()
	defer s.mail.mu.Unlock()
	return append([]sentEmail(nil), s.mail.sent...)
}

// A panic and an unexpected 500 each email the maintainer once, with the
// route's pattern and without the path's token or the error's link.
func TestProblemsEmailed(t *testing.T) {
	s, h := problemServer(t)
	if rec := do(t, h, http.MethodGet, "/clubs/member/"+problemToken, nil); rec.Code != http.StatusInternalServerError {
		t.Fatalf("a panic: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, "/competitions/admin/"+problemToken+"/x", nil); rec.Code != http.StatusInternalServerError {
		t.Fatalf("an error: %d", rec.Code)
	}
	got := told(s)
	if len(got) != 2 {
		t.Fatalf("emails: %d, want a panic's and an error's", len(got))
	}
	for _, e := range got {
		if e.to != "maintainer@example.com" {
			t.Errorf("sent to %s", e.to)
		}
		for _, secret := range []string{problemToken, "ann@example.com"} {
			if strings.Contains(e.subject+e.body, secret) {
				t.Errorf("the email has %q: %s", secret, e.body)
			}
		}
	}
	p, e := got[0], got[1]
	if p.subject != "Problem: Panic in GET /clubs/member/{token}" {
		t.Errorf("the panic's subject: %q", p.subject)
	}
	for _, want := range []string{"Something broke on the live site.", "What: Panic: boom for /clubs/member/{token}", "Where: GET /clubs/member/{token} (a GET request)",
		"When: Thu 10 January 2030, 12:00:00", "How often: once so far", "App version: ", "Site: live", "First lines of the stack:", "problems_test.go"} {
		if !strings.Contains(p.body, want) {
			t.Errorf("the panic's email lacks %q:\n%s", want, p.body)
		}
	}
	if strings.Contains(p.body, "runtime/debug") {
		t.Errorf("the stack starts at the panic:\n%s", p.body)
	}
	for _, want := range []string{"What: Error: the disk is on fire at /competitions/admin/{token}/x, ask {email}", "Where: GET /competitions/admin/{token}/x"} {
		if !strings.Contains(e.body, want) {
			t.Errorf("the error's email lacks %q:\n%s", want, e.body)
		}
	}
	t.Logf("one email:\nTo: %s\nSubject: %s\n\n%s", p.to, p.subject, p.body)

	// A visitor who left, and an expected kind, tell no one.
	do(t, h, http.MethodGet, "/gone/"+problemToken, nil)
	do(t, h, http.MethodGet, "/missing/"+problemToken, nil)
	if n := len(told(s)); n != 2 {
		t.Errorf("emails now %d, want still 2", n)
	}
}

// The same problem is told once, counted for an hour, then told again with
// how many times it happened.
func TestProblemsGrouped(t *testing.T) {
	s, h := problemServer(t)
	for range 11 {
		do(t, h, http.MethodGet, "/clubs/member/"+problemToken, nil)
	}
	if n := len(told(s)); n != 1 {
		t.Fatalf("emails after 11 of the same: %d, want 1", n)
	}
	s.clock.set(s.clock.now().Add(59 * time.Minute))
	do(t, h, http.MethodGet, "/clubs/member/"+problemToken, nil)
	if n := len(told(s)); n != 1 {
		t.Fatalf("emails within the hour: %d, want 1", n)
	}
	s.clock.set(s.clock.now().Add(2 * time.Minute))
	do(t, h, http.MethodGet, "/clubs/member/"+problemToken, nil)
	got := told(s)
	if len(got) != 2 {
		t.Fatalf("emails after the hour: %d, want 2", len(got))
	}
	if !strings.Contains(got[1].body, "How often: 12 times since you were last told, at 12:00") {
		t.Errorf("the second email:\n%s", got[1].body)
	}
	// A different error on the same route is another problem.
	do(t, h, http.MethodGet, "/competitions/admin/"+problemToken+"/x", nil)
	if n := len(told(s)); n != 3 {
		t.Errorf("a different problem: %d emails, want 3", n)
	}
}

// At most 20 are emailed a day, then one saying there are more.
func TestProblemsDailyCap(t *testing.T) {
	s, _ := problemServer(t)
	for i := range 30 {
		s.p.problems.job(fmt.Sprintf("job %d", i), errors.New("failed"))
	}
	got := told(s)
	if len(got) != 21 {
		t.Fatalf("emails: %d, want 20 and one more", len(got))
	}
	if last := got[20]; last.subject != "Problem: more problems" || !strings.Contains(last.body, "See the log") {
		t.Errorf("the last: %+v", last)
	}
	// The next day starts again.
	s.clock.set(s.clock.now().Add(24 * time.Hour))
	s.p.problems.job("job 1", errors.New("failed"))
	if n := len(told(s)); n != 22 {
		t.Errorf("the next day: %d emails, want 22", n)
	}
	// Every one is on the page's list, to the last 20.
	if n := len(s.p.problems.items()); n != recentProblems {
		t.Errorf("listed: %d, want %d", n, recentProblems)
	}
}

// Without FEEDBACK_EMAIL nothing is sent, and nothing breaks; the problem is
// still listed.
func TestProblemsNoAddress(t *testing.T) {
	s, h := problemServer(t)
	s.p.problems.to = ""
	if rec := do(t, h, http.MethodGet, "/clubs/member/"+problemToken, nil); rec.Code != http.StatusInternalServerError {
		t.Fatalf("a panic: %d", rec.Code)
	}
	s.p.problems.job("the expiry job", errors.New("failed"))
	if n := len(told(s)); n != 0 {
		t.Errorf("emails: %d, want none", n)
	}
	if n := len(s.p.problems.items()); n != 2 {
		t.Errorf("listed: %d, want 2", n)
	}
}

// A background job's failure is emailed; the sandbox's are marked, and sent
// directly, as it sends no deliveries.
func TestProblemsJobsAndSandbox(t *testing.T) {
	s, _ := problemServer(t)
	s.p.problems.job("the notifier", errors.New("database is locked"))
	got := told(s)
	if len(got) != 1 || got[0].subject != "Problem: Background job in the notifier" || !strings.Contains(got[0].body, "Where: the notifier\n") {
		t.Fatalf("emails: %+v", got)
	}

	sent := make(emailCatcher, 2)
	s.p.makeSandbox()
	s.p.problems.direct = sent
	s.p.problems.job("the sandbox's daily restart", errors.New("no room"))
	select {
	case e := <-sent:
		if !strings.HasPrefix(e.subject, "[sandbox] Problem:") || !strings.Contains(e.body, "Something broke in the sandbox.") || !strings.Contains(e.body, "Site: sandbox") {
			t.Errorf("the sandbox's email: %+v", e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the sandbox sent nothing")
	}
}

// Text from an error is scrubbed of secret links and email addresses, and
// kept short.
func TestScrubProblem(t *testing.T) {
	got := scrubProblem("open /clubs/member/" + problemToken + "?x=1 for  ann@example.com\n" + "in /competitions/enter/" + problemToken)
	if strings.Contains(got, problemToken) || strings.Contains(got, "ann@example.com") || strings.Contains(got, "\n") {
		t.Errorf("scrubbed: %q", got)
	}
	if !strings.Contains(got, "/clubs/member/{token}") || !strings.Contains(got, "/competitions/enter/{token}") {
		t.Errorf("scrubbed: %q", got)
	}
	if long := scrubProblem(strings.Repeat("a", 1000)); len(long) > 310 {
		t.Errorf("not short: %d", len(long))
	}
}

// The maintainer's page lists the problems lately, newest first, each with
// its count, and says when emails are failing: that's logged and shown, not
// emailed.
func TestProblemsOnMaintainersPage(t *testing.T) {
	s, h := problemServer(t)
	site := maintainerLink(t, s)
	page := do(t, s.h, http.MethodGet, site, nil).Body.String()
	if !strings.Contains(page, "Problems lately (0)") || strings.Contains(page, "Emails are failing") {
		t.Error("before any: none, and the mail is fine")
	}
	for range 3 {
		do(t, h, http.MethodGet, "/clubs/member/"+problemToken, nil)
	}
	s.clock.set(s.clock.now().Add(time.Minute))
	do(t, h, http.MethodGet, "/competitions/admin/"+problemToken+"/x", nil)
	for range 11 {
		s.p.problems.mailFailed()
	}
	text := strings.Join(strings.Fields(textOf(parsePage(t, do(t, s.h, http.MethodGet, site, nil).Body.String()))), " ")
	for _, want := range []string{"Problems lately (2)", "Error: the disk is on fire at /competitions/admin/{token}/x, ask {email}", "Panic: boom for /clubs/member/{token}",
		"Emails are failing: 11 emails couldn't be sent in the last hour"} {
		if !strings.Contains(text, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
	if strings.Contains(text, problemToken) || strings.Contains(text, "ann@example.com") {
		t.Error("the page shows a secret")
	}
	if i, j := strings.Index(text, "Error: the disk"), strings.Index(text, "Panic: boom"); i > j {
		t.Error("newest first")
	}
	// A hour on, the mail's fine again.
	s.clock.set(s.clock.now().Add(2 * time.Hour))
	if strings.Contains(do(t, s.h, http.MethodGet, site, nil).Body.String(), "Emails are failing") {
		t.Error("emails failing is only the last hour's")
	}
	if n := len(told(s)); n != 2 {
		t.Errorf("failing mail was emailed about: %d emails", n)
	}
}

// A panic site is told from the stack: the first frame after the panic that
// isn't the runtime's.
func TestPanicSite(t *testing.T) {
	stack := "goroutine 7 [running]:\nruntime/debug.Stack()\n\t/go/src/runtime/debug/stack.go:26 +0x5e\n" +
		"tariffCalculator/web.withRecover.func1.1()\n\t/app/web/recover.go:28 +0x3c\n" +
		"panic({0x1, 0x2})\n\t/go/src/runtime/panic.go:785 +0x132\n" +
		"runtime.goPanicIndex(0x3, 0x2)\n\t/go/src/runtime/panic.go:115 +0x1f\n" +
		"tariffCalculator/web.(*competitionPages).thing(0xc000123, {0x1, 0x2})\n\t/app/web/thing.go:42 +0x99\n" +
		"net/http.HandlerFunc.ServeHTTP(0x1, {0x2, 0x3})\n\t/go/src/net/http/server.go:2220 +0x29\n"
	site, frames := panicSite(stack)
	if site != "tariffCalculator/web.(*competitionPages).thing web/thing.go:42" {
		t.Errorf("site: %q", site)
	}
	if !strings.HasPrefix(frames, "  tariffCalculator/web.(*competitionPages).thing (web/thing.go:42)\n  net/http.HandlerFunc.ServeHTTP") || strings.Contains(frames, "0x") {
		t.Errorf("frames: %q", frames)
	}
}

// The expiry job deletes the copies of the database made before migrating
// once they're 14 days old.
func TestExpiryDeletesOldCopies(t *testing.T) {
	s := newTestServer(t)
	dir := s.st.Dir()
	old := time.Now().AddDate(0, 0, -15)
	for name, at := range map[string]time.Time{"tariff-before-v1.db": old, "tariff-before-v2.db": time.Now()} {
		path := dir + "/" + name
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		os.Chtimes(path, at, at)
	}
	expireOnce(s.st, s.p.problems, time.Now())
	if _, err := os.Stat(dir + "/tariff-before-v1.db"); err == nil {
		t.Error("the old copy stays")
	}
	if _, err := os.Stat(dir + "/tariff-before-v2.db"); err != nil {
		t.Error("the new copy goes")
	}
}
