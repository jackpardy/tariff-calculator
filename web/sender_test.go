package web

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"tariffCalculator/store"
)

// gatedMailer sends nothing until it's opened, then takes a while over each
// email, and fails for addresses starting "fail".
type gatedMailer struct {
	open chan struct{}
	each time.Duration
	mu   sync.Mutex
	got  map[string]int // tries, by address
}

func newGatedMailer(each time.Duration) *gatedMailer {
	return &gatedMailer{open: make(chan struct{}), each: each, got: map[string]int{}}
}

func (m *gatedMailer) send(to, subject, body string, headers map[string]string) error {
	<-m.open
	time.Sleep(m.each)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.got[to]++
	if strings.HasPrefix(to, "fail") {
		return errors.New("refused")
	}
	return nil
}

// pushCounter takes pushes, counting them by endpoint; an endpoint ending
// "gone" is gone.
type pushCounter struct {
	mu  sync.Mutex
	got map[string]int
}

func (p *pushCounter) RoundTrip(r *http.Request) (*http.Response, error) {
	io.Copy(io.Discard, r.Body)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.got[r.URL.String()]++
	status := http.StatusCreated
	if strings.HasSuffix(r.URL.Path, "gone") {
		status = http.StatusGone
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}, Request: r}, nil
}

// subscribe adds subscriptions straight to the store, each a member's: by
// email (confirmed) or by push, to every topic. members are who they're
// for; without them, each address is a new member of the club.
func subscribe(t *testing.T, st *store.Store, competitionID, clubID, channel string, addresses []string, members ...store.Member) {
	t.Helper()
	ctx := context.Background()
	ua, _ := ecdh.P256().GenerateKey(rand.Reader)
	auth := make([]byte, 16)
	rand.Read(auth)
	keys, _ := json.Marshal(pushKeys{P256dh: base64.RawURLEncoding.EncodeToString(ua.PublicKey().Bytes()), Auth: base64.RawURLEncoding.EncodeToString(auth)})
	for i, a := range addresses {
		var m store.Member
		if i < len(members) {
			m = members[i]
		} else {
			var err error
			if m, _, err = st.Join(ctx, clubID, fmt.Sprintf("Gymnast %d", i)); err != nil {
				t.Fatal(err)
			}
		}
		sub := store.Subscription{CompetitionID: competitionID, Kind: store.NotifyMember, OwnerID: m.ID, Channel: channel, Address: a, Topics: store.Topics}
		if channel == store.ByPush {
			sub.Keys = string(keys)
		}
		_, token, err := st.Subscribe(ctx, sub)
		if err != nil {
			t.Fatal(err)
		}
		if channel == store.ByEmail {
			if err := st.ConfirmSubscription(ctx, token); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// deskSentLine is the line under a sent message on the desk page saying
// how it went.
func deskSentLine(t *testing.T, page, text string) string {
	t.Helper()
	m := regexp.MustCompile(regexp.QuoteMeta(text) + `\s*</p>\s*<p class="is-size-7 has-text-grey">([^<]*)</p>`).FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("no message %q on the desk: %s", text, page)
	}
	return strings.Join(strings.Fields(unescape(m[1])), " ")
}

// A message to a few hundred addresses answers at once, saying how many it's
// going to; then each address is told once, and the desk counts what went
// and what failed.
func TestDeskSendsInTheBackground(t *testing.T) {
	s := newTestServer(t, withMail())
	mail := newGatedMailer(2 * time.Millisecond)
	pushes := &pushCounter{got: map[string]int{}}
	s.p.notify.mail, s.p.notify.http = mail, &http.Client{Transport: pushes}
	s.p.notify.send.perSecond, s.p.notify.send.retryWait = 10000, time.Millisecond
	h := s.h
	admin := created(t, h, newCompetition())
	aClub(t, h, admin, "UCD", true, "Ann")
	desk := admin + "/desk"
	clubID := regexp.MustCompile(`<option value="([^"]+)">UCD</option>`).FindStringSubmatch(do(t, h, http.MethodGet, desk, nil).Body.String())[1]
	c, err := s.st.CompetitionByAdmin(context.Background(), strings.TrimPrefix(admin, "/competitions/admin/"))
	if err != nil {
		t.Fatal(err)
	}

	var emails, endpoints []string
	for i := range 230 {
		emails = append(emails, fmt.Sprintf("parent%d@example.com", i))
	}
	emails = append(emails, "fail1@example.com", "fail2@example.com", "PARENT0@example.com") // the last is the first again
	for i := range 59 {
		endpoints = append(endpoints, fmt.Sprintf("https://fcm.googleapis.com/fcm/send/phone%d", i))
	}
	endpoints = append(endpoints, "https://fcm.googleapis.com/fcm/send/gone")
	subscribe(t, s.st, c.ID, clubID, store.ByEmail, emails)
	subscribe(t, s.st, c.ID, clubID, store.ByPush, endpoints)

	// Without the test kit's wait: the request answers before a single
	// email can go.
	quick := routesWithPages(s.p)
	start := time.Now()
	to, _ := url.QueryUnescape(redirected(t, quick, desk, url.Values{"to": {"club"}, "club": {clubID}, "text": {"Warm-up is moved to hall 2"}}))
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("the request answers at once: %v", took)
	}
	if !strings.Contains(to, "Sent to UCD. Sending to 292 people: 232 by email, 60 by push.") {
		t.Errorf("the notice says how many: %s", to)
	}
	if line := deskSentLine(t, do(t, quick, http.MethodGet, desk, nil).Body.String(), "Warm-up is moved to hall 2"); !strings.Contains(line, "still sending to") {
		t.Errorf("the desk says it's still sending: %s", line)
	}

	close(mail.open)
	s.p.notify.send.flush()
	for _, a := range emails[:230] {
		if mail.got[a] != 1 {
			t.Errorf("%s: emailed %d times", a, mail.got[a])
		}
	}
	if mail.got["fail1@example.com"] != 2 || mail.got["PARENT0@example.com"] != 0 {
		t.Errorf("a failure is tried once more; one address is told once: %v", mail.got)
	}
	for _, e := range endpoints {
		if pushes.got[e] != 1 {
			t.Errorf("%s: pushed %d times", e, pushes.got[e])
		}
	}
	subs, _ := s.st.CompetitionSubscriptions(context.Background(), c.ID)
	for _, sub := range subs {
		if strings.HasSuffix(sub.Address, "gone") {
			t.Error("a push subscription that's gone is dropped")
		}
	}
	// The counts are kept with the message.
	if line := deskSentLine(t, do(t, h, http.MethodGet, desk, nil).Body.String(), "Warm-up is moved to hall 2"); !strings.HasSuffix(line, "to UCD · told 59 by push, 230 by email · 3 failed") {
		t.Errorf("the desk counts what went and what failed: %s", line)
	}
	sent, _ := s.st.DeskMessages(context.Background(), c.ID)
	if m := sent[0]; m.Pushed != 59 || m.Emailed != 230 || m.Failed != 3 {
		t.Errorf("kept: %+v", m)
	}
}

// Change notifications go through the sender too: each address once.
func TestChangesSentInTheBackground(t *testing.T) {
	s := newTestServer(t, withMail())
	h := s.h
	admin := created(t, h, newCompetition())
	var names []string
	for i := range 12 {
		names = append(names, fmt.Sprintf("Gymnast %d", i))
	}
	aClub(t, h, admin, "UCD", true, names...)
	clubID := regexp.MustCompile(`<option value="([^"]+)">UCD</option>`).FindStringSubmatch(do(t, h, http.MethodGet, admin+"/desk", nil).Body.String())[1]
	c, err := s.st.CompetitionByAdmin(context.Background(), strings.TrimPrefix(admin, "/competitions/admin/"))
	if err != nil {
		t.Fatal(err)
	}
	members, err := s.st.Members(context.Background(), clubID)
	if err != nil || len(members) != 12 {
		t.Fatalf("the members: %v %+v", err, members)
	}
	// The first two share an address.
	emails := []string{"family@example.com", "Family@example.com"}
	for i := range 10 {
		emails = append(emails, fmt.Sprintf("parent%d@example.com", i))
	}
	subscribe(t, s.st, c.ID, clubID, store.ByEmail, emails, members...)

	s.mail.sent = nil
	redirected(t, h, admin+"/timetable/plan", url.Values{})
	redirected(t, h, admin+"/timetable/publish", url.Values{"action": {"publish"}})
	redirected(t, h, admin+"/notify-now", nil)
	tellDue(s.p)
	got := map[string][]string{}
	for _, e := range s.mail.sent {
		got[strings.ToLower(e.to)] = append(got[strings.ToLower(e.to)], e.body)
	}
	if len(got) != 11 || len(s.mail.sent) != 11 {
		t.Errorf("11 addresses, once each: %d emails to %d", len(s.mail.sent), len(got))
	}
	for i, m := range members[2:] {
		if b := got[fmt.Sprintf("parent%d@example.com", i)]; len(b) != 1 || !strings.Contains(b[0], m.Name+"\n- BUCS L3") {
			t.Errorf("%s's parent: %v", m.Name, b)
		}
	}
	if b := got["family@example.com"]; len(b) != 1 || !strings.Contains(b[0], members[0].Name+"\n- BUCS L3") || !strings.Contains(b[0], members[1].Name+"\n- BUCS L3") {
		t.Errorf("one email for both: %v", b)
	}
}

// Emails go no faster than the cap.
func TestSendRateCap(t *testing.T) {
	s := newTestServer(t, withMail())
	send := s.p.notify.send
	send.perSecond = 50
	var ds []delivery
	for i := range 26 {
		ds = append(ds, delivery{about: "test", email: &email{to: fmt.Sprintf("p%d@example.com", i), subject: "Hi", body: "Hello"}})
	}
	start := time.Now()
	send.send(ds)
	send.flush()
	// The first at once, then one every 20ms: the 26th after 500ms.
	if took := time.Since(start); took < 450*time.Millisecond {
		t.Errorf("26 emails at 50 a second took %v", took)
	}
	if len(s.mail.sent) != 26 {
		t.Errorf("all sent: %d", len(s.mail.sent))
	}
}

// Stopping waits for what's queued, up to a limit, and keeps a desk
// message's counts with what wasn't sent as failed.
func TestSendStop(t *testing.T) {
	s := newTestServer(t, withMail())
	mail := newGatedMailer(5 * time.Millisecond)
	s.p.notify.mail = mail
	send := s.p.notify.send
	send.perSecond = 10000
	var ds []delivery
	for i := range 12 {
		ds = append(ds, delivery{about: "test", email: &email{to: fmt.Sprintf("p%d@example.com", i)}})
	}
	send.send(ds)
	close(mail.open)
	if left := send.stop(5 * time.Second); left != 0 || len(mail.got) != 12 {
		t.Errorf("waits for them all: %d left, %d sent", left, len(mail.got))
	}

	// A mailer that never answers: stopping gives up after the wait.
	s = newTestServer(t, withMail())
	stuck := newGatedMailer(0)
	t.Cleanup(func() { close(stuck.open) })
	s.p.notify.mail = stuck
	send = s.p.notify.send
	admin := created(t, s.h, newCompetition())
	aClub(t, s.h, admin, "UCD", true, "Ann")
	clubID := regexp.MustCompile(`<option value="([^"]+)">UCD</option>`).FindStringSubmatch(do(t, s.h, http.MethodGet, admin+"/desk", nil).Body.String())[1]
	c, _ := s.st.CompetitionByAdmin(context.Background(), strings.TrimPrefix(admin, "/competitions/admin/"))
	subscribe(t, s.st, c.ID, clubID, store.ByEmail, []string{"a@example.com", "b@example.com", "c@example.com", "d@example.com", "e@example.com", "f@example.com"})
	redirected(t, routesWithPages(s.p), admin+"/desk", url.Values{"to": {"club"}, "club": {clubID}, "text": {"Hello"}})
	start := time.Now()
	if left := send.stop(100 * time.Millisecond); left != 6 {
		t.Errorf("left unsent: %d", left)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("stopping waits only so long: %v", took)
	}
	sent, _ := s.st.DeskMessages(context.Background(), c.ID)
	if m := sent[0]; m.Emailed != 0 || m.Failed != 6 {
		t.Errorf("what wasn't sent is kept as failed: %+v", m)
	}
}
