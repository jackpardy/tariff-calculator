package web

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"tariffCalculator/static"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// Knowing when it breaks: a panic, a page answered 500 for an error nobody
// expected, a background job that failed, or the store not opening each
// emails the maintainer (FEEDBACK_EMAIL; unset, nothing is sent), through
// the deliveries like their other emails. The same problem (the same route
// and error text, or the same panic site) is told once, then counted and
// told again no sooner than an hour later with how many times it happened;
// at most maxProblemEmails a day are sent, then one "more problems" email.
// An email holds the route's pattern, never its path, and error text with
// anything shaped like a secret link or an email address taken out: no
// name or link of anyone's. The last recentProblems are listed, from
// memory, on the maintainer's page. Emails failing can't be told by email:
// that is only logged, and shown on that page.

const (
	// problemEvery is how long a problem is counted before it's told again.
	problemEvery = time.Hour
	// maxProblemEmails is the most problems emailed in a day.
	maxProblemEmails = 20
	// recentProblems is how many the maintainer's page lists.
	recentProblems = 20
	// mailTroubleMin is how many emails failing within an hour is trouble
	// with the mail server, more than it.
	mailTroubleMin = 10
	// stackLines is how many frames of a panic's stack an email has.
	stackLines = 6
)

// problem is one kind of problem seen: what, where and how often.
type problem struct {
	key       string
	Kind      string // "Panic", "Error", "Background job" or "Start-up"
	What      string
	Where     string // the route's pattern, or the job
	Method    string // "" for a job
	At        time.Time
	Count     int    // times seen
	Stack     string // a panic's first frames
	told      time.Time
	untold    int // seen since last told
	firstSeen time.Time
}

// problems keeps the problems seen and tells the maintainer. Its methods are
// safe to call on nil, which does nothing.
type problems struct {
	to      string // FEEDBACK_EMAIL
	sandbox bool
	now     func() time.Time
	// queue keeps an email to send, with the other deliveries; nil, or an
	// error, sends it directly instead.
	queue  func(subject, body string) error
	direct mailer // used when it can't be queued: nil for none

	mu     sync.Mutex
	recent []*problem // newest first, at most recentProblems
	byKey  map[string]*problem
	day    string // the day sentToday is of
	sent   int    // emails sent today
	capped bool   // today's "more problems" email is sent
	// failedMails are when emails failed to send, in the last hour.
	failedMails []time.Time
	troubleSaid time.Time
}

func newProblems(now func() time.Time) *problems {
	return &problems{to: os.Getenv("FEEDBACK_EMAIL"), now: now, direct: mailerFromEnv(), byKey: map[string]*problem{}}
}

type problemsKey struct{}

// withProblems is ctx with the problems to report to.
func withProblems(ctx context.Context, p *problems) context.Context {
	return context.WithValue(ctx, problemsKey{}, p)
}

// problemsOf are the problems a request reports to; nil for none.
func problemsOf(ctx context.Context) *problems {
	p, _ := ctx.Value(problemsKey{}).(*problems)
	return p
}

// emailLike is anything shaped like an email address.
var emailLike = regexp.MustCompile(`[A-Za-z0-9._%+'-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)

// scrubProblem is text from an error or a panic, safe to tell: tokens of
// secret links and email addresses taken out, on one line, no longer than
// 300 characters.
func scrubProblem(s string) string {
	s = hideTokens(s)
	s = emailLike.ReplaceAllString(s, "{email}")
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 300 {
		s = string(r[:300]) + "..."
	}
	return s
}

// panicked reports a handler's panic: v, with stack (debug.Stack's), on r's
// route.
func (p *problems) panicked(r *http.Request, v any, stack []byte) {
	if p == nil {
		return
	}
	site, frames := panicSite(string(stack))
	p.report(&problem{key: "panic|" + site, Kind: "Panic", What: scrubProblem(fmt.Sprint(v)), Where: r.Pattern, Method: r.Method, Stack: frames})
}

// unexpected reports a request answered 500 for an error nobody expected.
func (p *problems) unexpected(r *http.Request, err error) {
	if p == nil {
		return
	}
	what := scrubProblem(err.Error())
	p.report(&problem{key: "error|" + r.Pattern + "|" + what, Kind: "Error", What: what, Where: r.Pattern, Method: r.Method})
}

// job reports a background job's failure; job is e.g. "the expiry job".
func (p *problems) job(job string, err error) {
	if p == nil || err == nil {
		return
	}
	what := scrubProblem(err.Error())
	p.report(&problem{key: "job|" + job + "|" + what, Kind: "Background job", What: what, Where: job})
}

// startup reports a store that didn't open, directly, waiting up to a few
// seconds for it to go: nothing else is up to send it.
func (p *problems) startup(err error) {
	what := scrubProblem(err.Error())
	if e := p.record(&problem{key: "startup|" + what, Kind: "Start-up", What: what, Where: "opening the database"}); e != nil {
		p.deliver(*e, true)
	}
}

// ReportStartup emails FEEDBACK_EMAIL, if set, with email set up, that the
// competition storage didn't open, when the app starts: the store is what's
// down, so it's sent directly, not through the deliveries. sandbox says
// which instance it is.
func ReportStartup(err error, sandbox bool) {
	p := newProblems(time.Now)
	p.sandbox = sandbox
	p.startup(err)
}

// report records a problem and, if it's time, tells the maintainer.
func (p *problems) report(pr *problem) {
	if e := p.record(pr); e != nil {
		p.deliver(*e, false)
	}
}

// record notes pr as seen, and returns the email to send about it, nil if
// none is due.
func (p *problems) record(pr *problem) *email {
	now := p.now()
	p.mu.Lock()
	defer p.mu.Unlock()
	g := p.byKey[pr.key]
	if g == nil {
		pr.At, pr.firstSeen = now, now
		g = pr
		p.byKey[pr.key] = g
	} else {
		g.At = now
	}
	g.Count++
	g.untold++
	// Newest first, and not more than are listed.
	p.recent = append([]*problem{g}, withoutProblem(p.recent, g)...)
	if len(p.recent) > recentProblems {
		p.recent = p.recent[:recentProblems]
	}
	// What's counted for the hour is kept apart from what's listed, and
	// forgotten once the hour is over, so a flood of different errors
	// doesn't fill the memory.
	if len(p.byKey) > 200 {
		for k, o := range p.byKey {
			if now.Sub(o.At) >= problemEvery {
				delete(p.byKey, k)
			}
		}
	}
	if p.to == "" {
		return nil
	}
	if !g.told.IsZero() && now.Sub(g.told) < problemEvery {
		return nil
	}
	if day := now.In(local).Format("2006-01-02"); day != p.day {
		p.day, p.sent, p.capped = day, 0, false
	}
	if p.sent >= maxProblemEmails {
		if p.capped {
			return nil
		}
		p.capped = true
		return &email{to: p.to, subject: p.subject("more problems"), body: p.site() + ": there have been more problems today than are emailed (" +
			fmt.Sprint(maxProblemEmails) + "). See the log, and the list on your feedback page under Problems lately.\n"}
	}
	p.sent++
	e := p.email(g, now)
	g.told, g.untold = now, 0
	return &e
}

func withoutProblem(list []*problem, g *problem) []*problem {
	var out []*problem
	for _, o := range list {
		if o != g {
			out = append(out, o)
		}
	}
	return out
}

// site says which instance this is, for an email's first line.
func (p *problems) site() string {
	if p.sandbox {
		return "Something broke in the sandbox"
	}
	return "Something broke on the live site"
}

func (p *problems) subject(what string) string {
	if p.sandbox {
		return "[sandbox] Problem: " + what
	}
	return "Problem: " + what
}

// email is the email telling of g, as it stands at now. Called with p.mu held.
func (p *problems) email(g *problem, now time.Time) email {
	var b strings.Builder
	b.WriteString(p.site() + ".\n\n")
	fmt.Fprintf(&b, "What: %s: %s\n", g.Kind, g.What)
	if g.Method != "" {
		fmt.Fprintf(&b, "Where: %s (a %s request)\n", g.Where, g.Method)
	} else {
		fmt.Fprintf(&b, "Where: %s\n", g.Where)
	}
	fmt.Fprintf(&b, "When: %s\n", now.In(local).Format("Mon 2 January 2006, 15:04:05"))
	if g.told.IsZero() && g.untold <= 1 {
		b.WriteString("How often: once so far; you won't hear of it again for an hour\n")
	} else if g.told.IsZero() {
		fmt.Fprintf(&b, "How often: %d times so far\n", g.untold)
	} else {
		fmt.Fprintf(&b, "How often: %d times since you were last told, at %s\n", g.untold, g.told.In(local).Format("15:04"))
	}
	fmt.Fprintf(&b, "App version: %s\n", static.Version)
	if p.sandbox {
		b.WriteString("Site: sandbox\n")
	} else {
		b.WriteString("Site: live\n")
	}
	if g.Stack != "" {
		b.WriteString("\nFirst lines of the stack:\n" + g.Stack)
	}
	b.WriteString("\nThe last " + fmt.Sprint(recentProblems) + " problems are on your feedback page, under Problems lately. The log has the rest.\n")
	return email{to: p.to, subject: p.subject(g.Kind + " in " + g.Where), body: b.String()}
}

// deliver sends e: kept with the deliveries if it can be, else directly,
// without holding anything up unless wait (then for up to 10 seconds).
func (p *problems) deliver(e email, wait bool) {
	if p.queue != nil {
		err := p.queue(e.subject, e.body)
		if err == nil {
			return
		}
		if !errors.Is(err, errMailOff) {
			log.Printf("Emailing about a problem: %v", err)
		}
	}
	if p.direct == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := p.direct.send(e.to, e.subject, e.body, nil); err != nil {
			log.Printf("Emailing about a problem: %v", err)
		}
	}()
	if wait {
		select {
		case <-done:
		case <-time.After(10 * time.Second):
		}
	}
}

// mailFailed notes an email that couldn't be sent. Trouble with the mail
// server, more than mailTroubleMin in an hour, is logged (once an hour) and
// shown on the maintainer's page: it can't be emailed.
func (p *problems) mailFailed() {
	if p == nil {
		return
	}
	now := p.now()
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failedMails = append(recentTimes(p.failedMails, now), now)
	if len(p.failedMails) > mailTroubleMin && now.Sub(p.troubleSaid) >= problemEvery {
		p.troubleSaid = now
		log.Printf("EMAILS ARE FAILING: %d in the last hour; check the mail server", len(p.failedMails))
	}
}

// mailTrouble is how many emails have failed in the last hour, if that's
// trouble with the mail server; else 0.
func (p *problems) mailTrouble() int {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failedMails = recentTimes(p.failedMails, p.now())
	if len(p.failedMails) > mailTroubleMin {
		return len(p.failedMails)
	}
	return 0
}

// recentTimes are the times in ts within the hour before now.
func recentTimes(ts []time.Time, now time.Time) []time.Time {
	var out []time.Time
	for _, t := range ts {
		if now.Sub(t) < time.Hour {
			out = append(out, t)
		}
	}
	return out
}

// items are the problems lately for the maintainer's page, newest first.
func (p *problems) items() []views.ProblemItem {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]views.ProblemItem, len(p.recent))
	for i, g := range p.recent {
		out[i] = views.ProblemItem{At: g.At.In(local).Format("Mon 2 Jan, 15:04"), Kind: g.Kind, What: g.What, Where: g.Where, Method: g.Method, Count: g.Count, Stack: g.Stack}
	}
	return out
}

// panicSite finds where a panic came from in a stack (debug.Stack's): the
// first frame after panic( that isn't the runtime's, as "function file:line",
// and that frame with the next few, tidied: no arguments, no offsets, and
// the file's folder and name only.
func panicSite(stack string) (site, frames string) {
	lines := strings.Split(stack, "\n")
	start := -1
	for i, l := range lines {
		if strings.HasPrefix(l, "panic(") {
			start = i + 2
			break
		}
	}
	if start < 0 {
		return "unknown", ""
	}
	var b strings.Builder
	n := 0
	for i := start; i+1 < len(lines) && n < stackLines; i += 2 {
		fn, file := lines[i], strings.TrimSpace(lines[i+1])
		if n == 0 && strings.HasPrefix(fn, "runtime.") {
			continue
		}
		if cut := strings.LastIndex(fn, "("); cut > 0 {
			fn = fn[:cut]
		}
		file, _, _ = strings.Cut(file, " +0x")
		if parts := strings.Split(file, "/"); len(parts) > 2 {
			file = strings.Join(parts[len(parts)-2:], "/")
		}
		if n == 0 {
			site = fn + " " + file
		}
		fmt.Fprintf(&b, "  %s (%s)\n", fn, file)
		n++
	}
	if site == "" {
		return "unknown", ""
	}
	return site, b.String()
}

// queueProblem keeps an email to the maintainer with the other deliveries.
func (p *competitionPages) queueProblem(subject, body string) error {
	if p.notify.mail == nil {
		return errMailOff
	}
	return p.notify.send.add(context.Background(), store.Round{}, []delivery{{about: "Emailing about a problem", who: "The maintainer",
		email: &email{to: p.problems.to, subject: subject, body: body}}})
}
