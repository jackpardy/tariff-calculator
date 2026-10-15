package web

import (
	"context"
	"errors"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"tariffCalculator/store"
)

// Sending in the background (roadmap, ready for the dry run): a request
// works out who a message reaches and hands its deliveries, one per
// address, to the sender, which answers at once. A few workers send them,
// emails no faster than a cap, so a message to 300 people neither holds up
// the organiser's page nor times out part way. Desk messages and change
// notifications share the one sender. The queue is in memory: what's still
// queued when the app stops, after a bounded wait, isn't sent.

// sendWorkers is how many deliveries go at once.
const sendWorkers = 4

// defaultMailPerSecond is the most emails sent a second unless
// MAIL_PER_SECOND says otherwise: Amazon SES's rate once out of its sandbox.
const defaultMailPerSecond = 14

// mailRetryWait is how long a failed email waits before its one retry.
const mailRetryWait = 5 * time.Second

// email is an email to send.
type email struct {
	to, subject, body string
	headers           map[string]string
}

// delivery is one email or push to one address.
type delivery struct {
	about string // what a failure is logged as, e.g. "Desk: emailing about <competition id>"
	email *email // nil for a push
	sub   store.Subscription
	push  pushMessage
	drop  []store.Subscription // dropped if the push service says it's gone
	tally *tally               // nil when nobody's counting
	retry bool                 // this is an email's retry
}

// tally is how a desk message's deliveries are going.
type tally struct {
	competitionID, messageID string
	pushes, emails           int // to send
	pushed, emailed, failed  int // done
}

// toGo is how many are still to be sent.
func (t tally) toGo() int { return t.pushes + t.emails - t.pushed - t.emailed - t.failed }

// sender sends deliveries in the background.
type sender struct {
	n         *notifier
	perSecond float64       // emails a second, at most
	retryWait time.Duration // before a failed email's retry
	start     sync.Once

	mu       sync.Mutex
	changed  *sync.Cond // the queue, the work under way or the tallies changed
	queue    []delivery
	busy     int // being sent
	waiting  int // emails waiting to be retried
	stopped  bool
	tallies  map[string]*tally // by message id, while sending
	nextMail time.Time         // when the next email may go
}

func newSender(n *notifier) *sender {
	s := &sender{n: n, perSecond: mailPerSecond(), retryWait: mailRetryWait, tallies: map[string]*tally{}}
	s.changed = sync.NewCond(&s.mu)
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

// send queues deliveries, counting a desk message's on its tally, and
// returns at once.
func (s *sender) send(ds []delivery) {
	if len(ds) == 0 {
		return
	}
	s.start.Do(func() {
		for range sendWorkers {
			go s.work()
		}
	})
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		log.Printf("Sending: stopped, so %d not sent", len(ds))
		return
	}
	for _, d := range ds {
		if t := d.tally; t != nil {
			s.tallies[t.messageID] = t
		}
	}
	s.queue = append(s.queue, ds...)
	s.changed.Broadcast()
}

// work sends what's queued, one at a time, until the sender stops.
func (s *sender) work() {
	s.mu.Lock()
	for {
		for len(s.queue) == 0 && !s.stopped {
			s.changed.Wait()
		}
		if s.stopped {
			s.mu.Unlock()
			return
		}
		d := s.queue[0]
		s.queue[0] = delivery{}
		s.queue = s.queue[1:]
		s.busy++
		s.mu.Unlock()

		err := s.deliver(d)
		if d.email != nil && err != nil && !d.retry {
			// One retry, after a short wait, from the back of the queue.
			d.retry = true
			s.mu.Lock()
			s.waiting++
			s.mu.Unlock()
			time.AfterFunc(s.retryWait, func() {
				s.mu.Lock()
				defer s.mu.Unlock()
				s.waiting--
				if !s.stopped {
					s.queue = append(s.queue, d)
				}
				s.changed.Broadcast()
			})
		} else {
			if d.email != nil && err != nil {
				log.Printf("%s: %v", d.about, err)
			}
			s.count(d, err)
		}

		s.mu.Lock()
		s.busy--
		s.changed.Broadcast()
	}
}

// deliver sends one email or push. A push to a subscription that's gone
// drops it.
func (s *sender) deliver(d delivery) error {
	if d.email != nil {
		return s.mail(*d.email)
	}
	ctx := context.Background()
	err := s.n.push(ctx, d.sub, d.push)
	switch {
	case errors.Is(err, errGone):
		for _, sub := range d.drop {
			s.n.st.DropSubscription(ctx, sub.ID)
		}
	case err != nil:
		log.Printf("%s: %v", d.about, err)
	}
	return err
}

// mail sends an email when the cap allows.
func (s *sender) mail(e email) error {
	if s.n.mail == nil {
		return errors.New("email is off")
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
	return s.n.mail.send(e.to, e.subject, e.body, e.headers)
}

// count counts a delivery on its tally, and once the message's are all
// done keeps the counts with it.
func (s *sender) count(d delivery, err error) {
	t := d.tally
	if t == nil {
		return
	}
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return // its counts are kept already
	}
	switch {
	case err != nil:
		t.failed++
	case d.email != nil:
		t.emailed++
	default:
		t.pushed++
	}
	finished := t.toGo() == 0
	final := *t
	s.mu.Unlock()
	if !finished {
		return
	}
	s.record(final)
	s.mu.Lock()
	delete(s.tallies, t.messageID)
	s.mu.Unlock()
}

// record keeps a desk message's counts.
func (s *sender) record(t tally) {
	if err := s.n.st.SetDeskTold(context.Background(), t.competitionID, t.messageID, t.pushed, t.emailed, t.failed); err != nil {
		log.Printf("Desk: %s: %v", t.competitionID, err)
	}
}

// progress is how a desk message's sending is going, while it is.
func (s *sender) progress(messageID string) (tally, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tallies[messageID]
	if !ok {
		return tally{}, false
	}
	return *t, true
}

// flush waits until everything queued has been sent.
func (s *sender) flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for len(s.queue) > 0 || s.busy > 0 || s.waiting > 0 {
		s.changed.Wait()
	}
}

// stop waits up to wait for what's queued to be sent, then stops sending,
// keeping the counts of desk messages still sending, with what wasn't sent
// counted as failed. It returns how many were left unsent.
func (s *sender) stop(wait time.Duration) int {
	done := make(chan struct{})
	go func() {
		s.flush()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(wait):
	}
	s.mu.Lock()
	s.stopped = true
	left := len(s.queue) + s.busy + s.waiting
	s.queue = nil
	var unfinished []tally
	for _, t := range s.tallies {
		final := *t
		final.failed += final.toGo()
		unfinished = append(unfinished, final)
	}
	s.changed.Broadcast()
	s.mu.Unlock()
	for _, t := range unfinished {
		s.record(t)
	}
	return left
}
