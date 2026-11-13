package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/textproto"
	"os"
	"strconv"
	"sync"
	"time"

	"tariffCalculator/store"
)

// Sending in the background (ADR 0012): a request works out who a message
// reaches and keeps its deliveries, one per address, in the store
// (migration 58), then answers at once. A few workers take them from there
// and send them, emails no faster than a cap, so a message to 300 people
// neither holds up the organiser's page nor times out part way. A failure
// that may pass is tried again, waiting longer each time, for about an
// hour; one that won't is kept as failed at once. What's kept is sent after
// a restart: only what's being sent when the app stops may go twice.

// sendWorkers is how many deliveries go at once.
const sendWorkers = 4

// defaultMailPerSecond is the most emails sent a second unless
// MAIL_PER_SECOND says otherwise: Amazon SES's rate once out of its sandbox.
const defaultMailPerSecond = 14

// retryWaits are how long a delivery that failed for a reason that may
// pass waits before each try after the first: seven tries over about an
// hour, then it's kept as failed.
var retryWaits = []time.Duration{time.Minute, 2 * time.Minute, 5 * time.Minute, 10 * time.Minute, 15 * time.Minute, 30 * time.Minute}

// idleWait is the longest a worker waits before looking for deliveries due.
const idleWait = 30 * time.Second

// errMailOff is an email when email isn't set up.
var errMailOff = errors.New("email is off")

// delivery is one email or push to one address, before it's kept.
type delivery struct {
	// competition is the competition it's about, "" for none: nothing is
	// sent about a rehearsal copy.
	competition string
	about       string // what a failure is logged as, e.g. "Desk: emailing about <competition id>"
	who         string // whose subscription it is, in words, for the organiser
	email       *email // nil for a push
	sub         store.Subscription
	push        pushMessage
	drop        []store.Subscription // dropped if the push service says it's gone
}

// email is an email to send.
type email struct {
	to, subject, body string
	headers           map[string]string
}

// kept is a delivery as the store keeps it.
func (d delivery) kept() store.Delivery {
	k := store.Delivery{CompetitionID: d.competition, About: d.about, Who: d.who}
	if d.email != nil {
		k.Kind, k.Address, k.Subject, k.Body, k.Headers = store.DeliverEmail, d.email.to, d.email.subject, d.email.body, d.email.headers
		return k
	}
	payload, _ := json.Marshal(d.push)
	k.Kind, k.SubscriptionID, k.Address, k.Keys, k.Body = store.DeliverPush, d.sub.ID, d.sub.Address, d.sub.Keys, string(payload)
	for _, sub := range d.drop {
		k.Drop = append(k.Drop, sub.ID)
	}
	return k
}

// sender sends the deliveries kept, in the background.
type sender struct {
	n         *notifier
	perSecond float64         // emails a second, at most
	waits     []time.Duration // before each try after the first
	start     sync.Once

	mu       sync.Mutex
	wake     chan struct{} // closed when deliveries are kept or the sender stops
	busy     int           // workers sending, or looking for one to send
	stopped  bool
	nextMail time.Time // when the next email may go
	idle     *sync.Cond
}

func newSender(n *notifier) *sender {
	s := &sender{n: n, perSecond: mailPerSecond(), waits: retryWaits, wake: make(chan struct{})}
	s.idle = sync.NewCond(&s.mu)
	return s
}

// mailPerSecond is MAIL_PER_SECOND, or defaultMailPerSecond if it's unset
// or not a number above 0.
func mailPerSecond() float64 {
	if v, err := strconv.ParseFloat(os.Getenv("MAIL_PER_SECOND"), 64); err == nil && v > 0 {
		return v
	}
	return defaultMailPerSecond
}

// begin starts the workers, once, after putting back what was being sent
// when the app last stopped. The sandbox sends nothing, so has none.
func (s *sender) begin() {
	if s.n.sandbox {
		return
	}
	s.start.Do(func() {
		if err := s.n.st.ResumeDeliveries(context.Background()); err != nil {
			log.Printf("Sending: %v", err)
		}
		for range sendWorkers {
			go s.work()
		}
	})
}

// kick wakes the workers to look for deliveries due.
func (s *sender) kick() {
	s.begin()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.stopped {
		close(s.wake)
		s.wake = make(chan struct{})
	}
}

// add keeps deliveries to send, counted on a round if it has a kind, and
// wakes the workers. Nothing is kept in the sandbox, or about a rehearsal
// copy (notRehearsals): the switch is checked here, where they're kept.
func (s *sender) add(ctx context.Context, round store.Round, ds []delivery) error {
	kept := s.n.keep(ds)
	if len(kept) == 0 {
		return nil
	}
	if err := s.n.st.AddDeliveries(ctx, round, kept); err != nil {
		return err
	}
	s.kick()
	return nil
}

// keep are the deliveries to keep, as the store keeps them: none in the
// sandbox or about a rehearsal copy.
func (n *notifier) keep(ds []delivery) []store.Delivery {
	if n.sandbox {
		return nil
	}
	var out []store.Delivery
	for _, d := range n.notRehearsals(ds) {
		out = append(out, d.kept())
	}
	return out
}

// notRehearsals are the deliveries not about a rehearsal copy: it emails
// and pushes no one, whatever's asked.
func (n *notifier) notRehearsals(ds []delivery) []delivery {
	quiet := map[string]bool{}
	out := ds[:0:0]
	for _, d := range ds {
		if d.competition != "" {
			q, seen := quiet[d.competition]
			if !seen {
				c, err := n.st.Competition(context.Background(), d.competition)
				q = err == nil && c.Rehearsal()
				quiet[d.competition] = q
			}
			if q {
				continue
			}
		}
		out = append(out, d)
	}
	return out
}

// work sends what's due, one at a time, until the sender stops.
func (s *sender) work() {
	ctx := context.Background()
	for {
		s.mu.Lock()
		if s.stopped {
			s.mu.Unlock()
			return
		}
		wake := s.wake
		s.busy++
		s.mu.Unlock()

		d, ok, err := s.n.st.TakeDelivery(ctx)
		if err != nil {
			log.Printf("Sending: %v", err)
			s.n.problems.job("the sender", err)
		}
		if ok {
			s.deliver(ctx, d)
		}

		s.mu.Lock()
		s.busy--
		s.idle.Broadcast()
		s.mu.Unlock()
		if ok {
			continue
		}
		wait := idleWait
		if at, ok, err := s.n.st.NextDelivery(ctx); err == nil && ok {
			wait = min(max(at.Sub(s.n.now()), 0), idleWait)
		}
		timer := time.NewTimer(wait)
		select {
		case <-wake:
		case <-timer.C:
		}
		timer.Stop()
	}
}

// deliver sends one email or push, deleting it once sent; one that fails is
// tried again later or kept as failed. A push to a subscription that's gone
// drops it.
func (s *sender) deliver(ctx context.Context, d store.Delivery) {
	var err error
	if d.Kind == store.DeliverEmail {
		err = s.mail(d)
	} else {
		var m pushMessage
		if err = json.Unmarshal([]byte(d.Body), &m); err == nil {
			err = s.n.push(ctx, store.Subscription{ID: d.SubscriptionID, Address: d.Address, Keys: d.Keys}, m)
		}
	}
	if err == nil {
		if err := s.n.st.Delivered(ctx, d); err != nil {
			log.Printf("Sending: %v", err)
			s.n.problems.job("the sender", err)
		}
		return
	}
	if errors.Is(err, errGone) {
		for _, id := range d.Drop {
			s.n.st.DropSubscription(ctx, id)
		}
	}
	again, why := failure(d.Kind, err)
	var retry time.Time
	if again && d.Tries <= len(s.waits) {
		retry = s.n.now().Add(s.waits[d.Tries-1])
	} else if again {
		why = fmt.Sprintf("tried %d times: %s", d.Tries, why)
	}
	log.Printf("%s (try %d): %v", d.About, d.Tries, err)
	if d.Kind == store.DeliverEmail {
		s.n.problems.mailFailed()
	}
	if err := s.n.st.DeliveryFailed(ctx, d, why, retry); err != nil {
		log.Printf("Sending: %v", err)
		s.n.problems.job("the sender", err)
	}
}

// failure says whether a failed delivery is worth trying again, and why it
// failed, in plain words for the organiser.
func failure(kind string, err error) (again bool, why string) {
	var smtpErr *textproto.Error
	var pushErr pushStatusError
	var netErr net.Error
	switch {
	case errors.Is(err, errGone):
		return false, "their phone no longer takes notifications from this site"
	case errors.Is(err, errPushTooLong):
		return false, "it was too long for a notification"
	case errors.Is(err, errMailOff):
		return false, "email isn't set up on this site"
	case errors.As(err, &smtpErr) && smtpErr.Code >= 500:
		return false, "the address was refused"
	case errors.As(err, &smtpErr):
		return true, "the mail server was busy"
	case errors.As(err, &pushErr) && (pushErr.code == 429 || pushErr.code >= 500):
		return true, "the phone's notification service was busy"
	case errors.As(err, &pushErr):
		return false, "the phone's notification service refused it"
	case errors.As(err, &netErr) && netErr.Timeout() && kind == store.DeliverEmail:
		return true, "the mail server didn't answer in time"
	case errors.As(err, &netErr) && netErr.Timeout():
		return true, "the phone's notification service didn't answer in time"
	case kind == store.DeliverEmail:
		return true, "the mail server couldn't be reached"
	}
	return true, "the phone's notification service couldn't be reached"
}

// mail sends an email when the cap allows.
func (s *sender) mail(d store.Delivery) error {
	if s.n.mail == nil {
		return errMailOff
	}
	s.mu.Lock()
	now := time.Now()
	at := now
	if s.nextMail.After(now) {
		at = s.nextMail
	}
	s.nextMail = at.Add(time.Duration(float64(time.Second) / s.perSecond))
	s.mu.Unlock()
	time.Sleep(at.Sub(now))
	return s.n.mail.send(d.Address, d.Subject, d.Body, d.Headers)
}

// flush wakes the workers and waits until nothing is due or being sent:
// what's waiting to be tried again later isn't waited for.
func (s *sender) flush() {
	// Nothing due and nothing being sent: no need to wake the workers.
	s.mu.Lock()
	busy := s.busy
	s.mu.Unlock()
	if n, err := s.n.st.DeliveriesDue(context.Background()); busy == 0 && (err != nil || n == 0) {
		return
	}
	s.kick()
	for {
		s.mu.Lock()
		busy, stopped := s.busy, s.stopped
		s.mu.Unlock()
		if stopped {
			return
		}
		if busy == 0 {
			n, err := s.n.st.DeliveriesDue(context.Background())
			if err != nil || n == 0 {
				return
			}
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// stop stops taking deliveries and waits up to wait for those being sent
// to finish. It returns how many were still being sent: they're sent again
// when the app next starts, as is everything not yet sent.
func (s *sender) stop(wait time.Duration) int {
	s.mu.Lock()
	if !s.stopped {
		s.stopped = true
		close(s.wake)
	}
	s.mu.Unlock()
	done := make(chan struct{})
	go func() {
		s.mu.Lock()
		for s.busy > 0 {
			s.idle.Wait()
		}
		s.mu.Unlock()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(wait):
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.busy
}
