package web

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/mail"
	"net/textproto"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"tariffCalculator/store"
)

// gatedMailer sends nothing until it's opened, then takes a while over each
// email; addresses starting "fail" are refused for good, "busy" for now.
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
	switch {
	case strings.HasPrefix(to, "fail"):
		return &textproto.Error{Code: 550, Msg: "no such address"}
	case strings.HasPrefix(to, "busy"):
		return &textproto.Error{Code: 451, Msg: "try again later"}
	}
	return nil
}

func (m *gatedMailer) tries(to string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.got[to]
}

// openMailer is a gatedMailer already open.
func openMailer() *gatedMailer {
	m := newGatedMailer(0)
	close(m.open)
	return m
}

// pushCounter takes pushes, counting them by endpoint; an endpoint ending
// "gone" is gone, "busy" busy for now, "bad" refused.
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
	switch {
	case strings.HasSuffix(r.URL.Path, "gone"):
		status = http.StatusGone
	case strings.HasSuffix(r.URL.Path, "busy"):
		status = http.StatusServiceUnavailable
	case strings.HasSuffix(r.URL.Path, "bad"):
		status = http.StatusBadRequest
	}
	return &http.Response{StatusCode: status, Status: http.StatusText(status), Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}, Request: r}, nil
}

func (p *pushCounter) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, c := range p.got {
		n += c
	}
	return n
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

// deskCompetition is a competition with a club, UCD, whose one member is
// Ann: its admin path, its id and the club's.
func deskCompetition(t *testing.T, s *testServer) (admin, competitionID, clubID string) {
	t.Helper()
	admin = created(t, s.h, newCompetition())
	aClub(t, s.h, admin, "UCD", true, "Ann")
	clubID = regexp.MustCompile(`<option value="([^"]+)"[^>]*>UCD</option>`).FindStringSubmatch(do(t, s.h, http.MethodGet, admin+"/desk", nil).Body.String())[1]
	c, err := s.st.CompetitionByAdmin(context.Background(), strings.TrimPrefix(admin, "/competitions/admin/"))
	if err != nil {
		t.Fatal(err)
	}
	return admin, c.ID, clubID
}

// A message to a few hundred addresses answers at once, saying how many it's
// going to; then each address is told once, and the desk counts what went
// and what failed.
func TestDeskSendsInTheBackground(t *testing.T) {
	s := newTestServer(t, withMail())
	mail := newGatedMailer(2 * time.Millisecond)
	pushes := &pushCounter{got: map[string]int{}}
	s.p.notify.mail, s.p.notify.http = mail, &http.Client{Transport: pushes}
	s.p.notify.send.perSecond = 10000
	h := s.h
	admin, cID, clubID := deskCompetition(t, s)
	desk := admin + "/desk"

	var emails, endpoints []string
	for i := range 230 {
		emails = append(emails, fmt.Sprintf("parent%d@example.com", i))
	}
	emails = append(emails, "fail1@example.com", "fail2@example.com", "PARENT0@example.com") // the last is the first again
	for i := range 59 {
		endpoints = append(endpoints, fmt.Sprintf("https://fcm.googleapis.com/fcm/send/phone%d", i))
	}
	endpoints = append(endpoints, "https://fcm.googleapis.com/fcm/send/gone")
	subscribe(t, s.st, cID, clubID, store.ByEmail, emails)
	subscribe(t, s.st, cID, clubID, store.ByPush, endpoints)

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
	if mail.got["fail1@example.com"] != 1 || mail.got["PARENT0@example.com"] != 0 {
		t.Errorf("an address refused is tried once; one address is told once: %v", mail.got)
	}
	for _, e := range endpoints {
		if pushes.got[e] != 1 {
			t.Errorf("%s: pushed %d times", e, pushes.got[e])
		}
	}
	subs, _ := s.st.CompetitionSubscriptions(context.Background(), cID)
	for _, sub := range subs {
		if strings.HasSuffix(sub.Address, "gone") {
			t.Error("a push subscription that's gone is dropped")
		}
	}
	// The counts are kept with the message.
	if line := deskSentLine(t, do(t, h, http.MethodGet, desk, nil).Body.String(), "Warm-up is moved to hall 2"); !strings.HasSuffix(line, "to UCD · told 59 by push, 230 by email · 3 failed") {
		t.Errorf("the desk counts what went and what failed: %s", line)
	}
	sent, _ := s.st.DeskMessages(context.Background(), cID)
	if m := sent[0]; m.Pushed != 59 || m.Emailed != 230 || m.Failed != 3 || m.Sending != 0 {
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
		names = append(names, fmt.Sprintf("Gymnast %c", 'A'+i))
	}
	aClub(t, h, admin, "UCD", true, names...)
	clubID := regexp.MustCompile(`<option value="([^"]+)"[^>]*>UCD</option>`).FindStringSubmatch(do(t, h, http.MethodGet, admin+"/desk", nil).Body.String())[1]
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

// Changes due when the app stops are told by the next start, once: the
// notifier keeps them to send though the sender has stopped, and a fresh
// notifier and sender over the same store send them.
func TestChangesSurviveARestart(t *testing.T) {
	s := newTestServer(t, withMail())
	h := s.h
	admin := created(t, h, newCompetition())
	aClub(t, h, admin, "UCD", true, "Ann", "Bea")
	clubID := regexp.MustCompile(`<option value="([^"]+)"[^>]*>UCD</option>`).FindStringSubmatch(do(t, h, http.MethodGet, admin+"/desk", nil).Body.String())[1]
	c, _ := s.st.CompetitionByAdmin(context.Background(), strings.TrimPrefix(admin, "/competitions/admin/"))
	members, _ := s.st.Members(context.Background(), clubID)
	subscribe(t, s.st, c.ID, clubID, store.ByEmail, []string{"ann@example.com", "bea@example.com"}, members...)

	// The app is stopping: its sender takes nothing more.
	s.p.notify.send.stop(time.Second)
	redirected(t, h, admin+"/timetable/plan", url.Values{})
	redirected(t, h, admin+"/timetable/publish", url.Values{"action": {"publish"}})
	redirected(t, h, admin+"/notify-now", nil)
	s.p.notify.tell(context.Background())
	if len(s.mail.sent) != 0 {
		t.Fatalf("nothing sent by a stopped sender: %+v", s.mail.sent)
	}
	if page := do(t, h, http.MethodGet, admin, nil).Body.String(); !strings.Contains(page, "0 sent") || !strings.Contains(page, "still sending to 2") {
		t.Errorf("the dashboard says they're still to go: %s", page)
	}

	// The next start.
	next := newCompetitionPages(s.st)
	again := &fakeMailer{}
	next.notify.mail, next.notify.base = again, "https://tariff.example"
	t.Cleanup(func() { next.notify.send.stop(time.Second) })
	tellDue(next)
	if len(again.sent) != 2 {
		t.Fatalf("both told after the restart: %+v", again.sent)
	}
	// Nobody is told twice: nothing is due or kept.
	tellDue(next)
	s.p.notify.send.flush()
	if len(again.sent) != 2 || len(s.mail.sent) != 0 {
		t.Errorf("told once: %d and %d", len(again.sent), len(s.mail.sent))
	}
	if _, waiting, _ := s.st.NextDelivery(context.Background()); waiting {
		t.Error("nothing left to send")
	}
	if page := do(t, routesWithPages(next), http.MethodGet, admin, nil).Body.String(); !strings.Contains(page, "2 sent") || strings.Contains(page, "still sending") {
		t.Errorf("the dashboard counts them: %s", page)
	}
}

// A desk message's counts come from the store: still sending after a
// restart, then told by the next start.
func TestDeskCountsSurviveARestart(t *testing.T) {
	s := newTestServer(t, withMail())
	stuck := newGatedMailer(0)
	s.p.notify.mail = stuck
	admin, cID, clubID := deskCompetition(t, s)
	subscribe(t, s.st, cID, clubID, store.ByEmail, []string{"a@example.com", "b@example.com", "c@example.com", "d@example.com", "e@example.com", "f@example.com"})
	redirected(t, routesWithPages(s.p), admin+"/desk", url.Values{"to": {"club"}, "club": {clubID}, "text": {"Hello"}})
	start := time.Now()
	if left := s.p.notify.send.stop(100 * time.Millisecond); left == 0 {
		t.Error("the emails cut off are counted")
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("stopping waits only so long: %v", took)
	}

	next := newCompetitionPages(s.st)
	if line := deskSentLine(t, do(t, routesWithPages(next), http.MethodGet, admin+"/desk", nil).Body.String(), "Hello"); !strings.HasSuffix(line, "told 0 by push, 0 by email · still sending to 6") {
		t.Errorf("still sending after the restart: %s", line)
	}
	next.notify.mail = &fakeMailer{}
	t.Cleanup(func() { next.notify.send.stop(time.Second) })
	next.notify.send.flush()
	// The first run's stuck workers finish after: nothing is counted twice.
	close(stuck.open)
	time.Sleep(50 * time.Millisecond)
	if line := deskSentLine(t, do(t, routesWithPages(next), http.MethodGet, admin+"/desk", nil).Body.String(), "Hello"); !strings.HasSuffix(line, "told 0 by push, 6 by email") {
		t.Errorf("told by the next start: %s", line)
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
	if err := send.add(context.Background(), store.Round{}, ds); err != nil {
		t.Fatal(err)
	}
	send.flush()
	// The first at once, then one every 20ms: the 26th after 500ms.
	if took := time.Since(start); took < 450*time.Millisecond {
		t.Errorf("26 emails at 50 a second took %v", took)
	}
	if len(s.mail.sent) != 26 {
		t.Errorf("all sent: %d", len(s.mail.sent))
	}
}

// A failure that may pass is tried again, waiting longer each time, seven
// tries in all; one that won't is kept as failed at once. Either way the
// round counts it, with why.
func TestSendRetries(t *testing.T) {
	now := time.Date(2027, 3, 13, 9, 0, 0, 0, time.UTC)
	s := newTestServer(t, withMail(), atTime(now))
	mail := openMailer()
	pushes := &pushCounter{got: map[string]int{}}
	s.p.notify.mail, s.p.notify.http = mail, &http.Client{Transport: pushes}
	send := s.p.notify.send
	send.perSecond = 10000
	ctx := context.Background()
	_, cID, clubID := deskCompetition(t, s)
	subscribe(t, s.st, cID, clubID, store.ByPush, []string{"https://fcm.googleapis.com/fcm/send/busy", "https://fcm.googleapis.com/fcm/send/bad"})
	subs, _ := s.st.CompetitionSubscriptions(ctx, cID)
	ds := []delivery{
		{competition: cID, about: "test", who: "Busy", email: &email{to: "busy@example.com"}},
		{competition: cID, about: "test", who: "Fail", email: &email{to: "fail@example.com"}},
		{competition: cID, about: "test", who: "Fine", email: &email{to: "fine@example.com"}},
	}
	for _, sub := range subs {
		ds = append(ds, delivery{competition: cID, about: "test", who: "Phone", sub: sub, drop: []store.Subscription{sub}, push: pushMessage{Title: "T", Body: "B"}})
	}
	if err := send.add(ctx, store.Round{CompetitionID: cID, Kind: store.RoundChanges}, ds); err != nil {
		t.Fatal(err)
	}
	send.flush()
	if mail.tries("busy@example.com") != 1 || mail.tries("fail@example.com") != 1 || mail.tries("fine@example.com") != 1 {
		t.Errorf("each tried once: %v", mail.got)
	}
	for i, wait := range retryWaits {
		// Not before it's due.
		s.clock.set(now.Add(wait - time.Second))
		send.flush()
		if got := mail.tries("busy@example.com"); got != i+1 {
			t.Fatalf("try %d too soon: %d", i+2, got)
		}
		now = now.Add(wait)
		s.clock.set(now)
		send.flush()
		if got := mail.tries("busy@example.com"); got != i+2 {
			t.Fatalf("try %d: %d", i+2, got)
		}
	}
	// Seven tries over about an hour, then no more.
	s.clock.set(now.Add(24 * time.Hour))
	send.flush()
	if mail.tries("busy@example.com") != 7 || mail.tries("fail@example.com") != 1 {
		t.Errorf("tries: %v", mail.got)
	}
	if pushes.got["https://fcm.googleapis.com/fcm/send/busy"] != 7 || pushes.got["https://fcm.googleapis.com/fcm/send/bad"] != 1 {
		t.Errorf("pushes too: %v", pushes.got)
	}
	r, _, _ := s.st.LatestRound(ctx, cID, store.RoundChanges)
	if r.Emailed != 1 || r.Failed != 4 || r.Sending != 0 {
		t.Errorf("counted: %+v", r)
	}
	failed, _ := s.st.FailedDeliveries(ctx, r.ID)
	var why []string
	for _, d := range failed {
		why = append(why, d.Who+": "+d.Why)
	}
	want := "Busy: tried 7 times: the mail server was busy|Fail: the address was refused|Phone: tried 7 times: the phone's notification service was busy|Phone: the phone's notification service refused it"
	if strings.Join(why, "|") != want {
		t.Errorf("who and why:\n%s\nwant\n%s", strings.Join(why, "|"), want)
	}
	// A push refused isn't a phone gone: its subscription stays.
	if subs, _ := s.st.CompetitionSubscriptions(ctx, cID); len(subs) != 2 {
		t.Errorf("the subscriptions stay: %d", len(subs))
	}
}

// stallingServer accepts connections and never answers.
func stallingServer(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var conns []net.Conn
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, c)
			mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		l.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, c := range conns {
			c.Close()
		}
	})
	return l.Addr().String()
}

// A mail server that accepts the connection and says nothing costs one try,
// and the other workers carry on.
func TestMailServerThatHangs(t *testing.T) {
	s := newTestServer(t, withMail())
	addr := stallingServer(t)
	pushes := &pushCounter{got: map[string]int{}}
	s.p.notify.mail = &smtpMailer{addr: addr, host: "127.0.0.1", from: "noreply@tariff.example", timeout: 300 * time.Millisecond}
	s.p.notify.http = &http.Client{Transport: pushes}
	admin, cID, clubID := deskCompetition(t, s)
	subscribe(t, s.st, cID, clubID, store.ByEmail, []string{"a@example.com", "b@example.com", "c@example.com", "d@example.com"})
	var endpoints []string
	for i := range 20 {
		endpoints = append(endpoints, fmt.Sprintf("https://fcm.googleapis.com/fcm/send/phone%d", i))
	}
	subscribe(t, s.st, cID, clubID, store.ByPush, endpoints)
	start := time.Now()
	redirected(t, routesWithPages(s.p), admin+"/desk", url.Values{"to": {"club"}, "club": {clubID}, "text": {"Hello"}})
	s.p.notify.send.flush()
	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("each email gives up after its time limit: %v", took)
	}
	if pushes.count() != 20 {
		t.Errorf("the pushes went: %d", pushes.count())
	}
	if line := deskSentLine(t, do(t, s.h, http.MethodGet, admin+"/desk", nil).Body.String(), "Hello"); !strings.HasSuffix(line, "told 20 by push, 0 by email · still sending to 4") {
		t.Errorf("the emails are tried again later: %s", line)
	}
	s.p.notify.send.stop(time.Second)
	s.st.SetClock(func() time.Time { return time.Now().Add(time.Hour) })
	d, ok, _ := s.st.TakeDelivery(context.Background())
	if !ok || d.Tries != 2 || d.Why != "the mail server didn't answer in time" {
		t.Errorf("one try spent, and why: %+v", d)
	}
}

// fakeSMTP is a mail server that answers as one should, refusing addresses
// starting "fail", and keeps each message.
func fakeSMTP(t *testing.T) (addr string, got chan string) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	got = make(chan string, 10)
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				tp := textproto.NewConn(c)
				tp.PrintfLine("220 fake")
				for {
					line, err := tp.ReadLine()
					if err != nil {
						return
					}
					switch cmd := strings.ToUpper(strings.Fields(line + " x")[0]); cmd {
					case "EHLO", "HELO":
						tp.PrintfLine("250 fake")
					case "RCPT":
						if strings.Contains(line, "fail") {
							tp.PrintfLine("550 no such address")
						} else {
							tp.PrintfLine("250 ok")
						}
					case "DATA":
						tp.PrintfLine("354 go on")
						b, _ := tp.ReadDotBytes()
						got <- string(b)
						tp.PrintfLine("250 ok")
					case "QUIT":
						tp.PrintfLine("221 bye")
						return
					default:
						tp.PrintfLine("250 ok")
					}
				}
			}()
		}
	}()
	return l.Addr().String(), got
}

// The mailer speaks SMTP: a message is sent whole, and an address refused
// is a failure that won't pass.
func TestSMTPMailer(t *testing.T) {
	addr, got := fakeSMTP(t)
	m := &smtpMailer{addr: addr, host: "127.0.0.1", from: "Competitions <noreply@tariff.example>", timeout: 2 * time.Second}
	if err := m.send("ann@example.com", "Hi", "Hello", nil); err != nil {
		t.Fatal(err)
	}
	if msg := <-got; !strings.Contains(msg, "Subject: Hi") || !strings.HasSuffix(msg, "\nHello\n") {
		t.Errorf("the message: %q", msg)
	}
	err := m.send("fail@example.com", "Hi", "Hello", nil)
	if again, why := failure(store.DeliverEmail, err); err == nil || again || why != "the address was refused" {
		t.Errorf("refused: %v %v %q", err, again, why)
	}
}

// An email's lines are never longer than SMTP allows, however long a
// paragraph, and names and subjects that aren't ASCII arrive as written.
func TestEmailMessage(t *testing.T) {
	body := strings.Repeat("Á long paragraph typed as one line. ", 42)[:1500] + "\nSecond line, Seán's\n"
	msg := emailMessage("Seán Ó Briain <noreply@tariff.example>", "ann@example.com", "Changes at Comórtas na hÉireann", body, map[string]string{"List-Unsubscribe": "<https://tariff.example/notify/off/x>"})
	for _, line := range strings.Split(string(msg), "\r\n") {
		if len(line) > 998 {
			t.Errorf("a line of %d bytes", len(line))
		}
	}
	if strings.Count(string(msg), "\n") != strings.Count(string(msg), "\r\n") {
		t.Error("every line ends CRLF")
	}
	m, err := mail.ReadMessage(bytes.NewReader(msg))
	if err != nil {
		t.Fatal(err)
	}
	from, err := mail.ParseAddress(m.Header.Get("From"))
	if err != nil || from.Name != "Seán Ó Briain" || from.Address != "noreply@tariff.example" {
		t.Errorf("from: %+v %v (%q)", from, err, m.Header.Get("From"))
	}
	subject, err := new(mime.WordDecoder).DecodeHeader(m.Header.Get("Subject"))
	if err != nil || subject != "Changes at Comórtas na hÉireann" {
		t.Errorf("subject: %q %v", subject, err)
	}
	if m.Header.Get("Content-Transfer-Encoding") != "quoted-printable" || !strings.HasSuffix(m.Header.Get("Message-ID"), "@tariff.example>") || m.Header.Get("List-Unsubscribe") == "" {
		t.Errorf("headers: %v", m.Header)
	}
	text, err := io.ReadAll(quotedprintable.NewReader(m.Body))
	if err != nil || string(text) != strings.ReplaceAll(body, "\n", "\r\n") {
		t.Errorf("the body back: %v\n%q", err, text)
	}
	// SMTP's own reading of it.
	if _, err := textproto.NewReader(bufio.NewReader(bytes.NewReader(msg))).ReadMIMEHeader(); err != nil {
		t.Error(err)
	}
}

// A push too long to send is trimmed to fit, and sent: its subscription
// stays.
func TestLongPushTrimmed(t *testing.T) {
	long := pushMessage{Title: "Competition", Body: strings.Repeat("🤸", 1000), URL: "https://tariff.example/competitions"}
	payload, err := fitPush(long)
	var back pushMessage
	if err != nil || len(payload) > maxPushPayload || json.Unmarshal(payload, &back) != nil ||
		!strings.HasSuffix(back.Body, "…") || !strings.HasPrefix(long.Body, strings.TrimSuffix(back.Body, "…")) || len(payload) < maxPushPayload-50 {
		t.Errorf("trimmed to fit: %d bytes, %v", len(payload), err)
	}
	if short, _ := fitPush(pushMessage{Body: "Hi"}); !strings.Contains(string(short), `"Hi"`) {
		t.Errorf("a short one as it is: %s", short)
	}

	s := newTestServer(t)
	pushes := &pushCounter{got: map[string]int{}}
	s.p.notify.http = &http.Client{Transport: pushes}
	_, cID, clubID := deskCompetition(t, s)
	subscribe(t, s.st, cID, clubID, store.ByPush, []string{"https://fcm.googleapis.com/fcm/send/phone"})
	subs, _ := s.st.CompetitionSubscriptions(context.Background(), cID)
	if err := s.p.notify.send.add(context.Background(), store.Round{}, []delivery{{competition: cID, about: "test", sub: subs[0], drop: subs, push: long}}); err != nil {
		t.Fatal(err)
	}
	s.p.notify.send.flush()
	if pushes.count() != 1 {
		t.Errorf("pushed: %v", pushes.got)
	}
	if subs, _ := s.st.CompetitionSubscriptions(context.Background(), cID); len(subs) != 1 {
		t.Error("the subscription stays")
	}
}

// The dashboard says how the latest changes told went, and who couldn't be
// told and why: never their address.
func TestDashboardSaysHowChangesWent(t *testing.T) {
	s := newTestServer(t, withMail())
	s.p.notify.mail = openMailer()
	h := s.h
	admin := created(t, h, newCompetition())
	aClub(t, h, admin, "UCD", true, "Ann Ryan", "Bea Kelly")
	clubID := regexp.MustCompile(`<option value="([^"]+)"[^>]*>UCD</option>`).FindStringSubmatch(do(t, h, http.MethodGet, admin+"/desk", nil).Body.String())[1]
	c, _ := s.st.CompetitionByAdmin(context.Background(), strings.TrimPrefix(admin, "/competitions/admin/"))
	members, _ := s.st.Members(context.Background(), clubID)
	subscribe(t, s.st, c.ID, clubID, store.ByEmail, []string{"ann@example.com", "fail@example.com"}, members...)
	if page := do(t, h, http.MethodGet, admin, nil).Body.String(); strings.Contains(page, "Your last changes") {
		t.Error("nothing to say before any are told")
	}
	redirected(t, h, admin+"/timetable/plan", url.Values{})
	redirected(t, h, admin+"/timetable/publish", url.Values{"action": {"publish"}})
	redirected(t, h, admin+"/notify-now", nil)
	tellDue(s.p)
	page := unescape(do(t, h, http.MethodGet, admin, nil).Body.String())
	told := regexp.MustCompile(`(?s)<div class="mb-3" id="told">(.*?)</div>`).FindStringSubmatch(page)
	if told == nil {
		t.Fatalf("no line on the dashboard: %s", page)
	}
	text := strings.Join(strings.Fields(regexp.MustCompile(`<[^>]+>`).ReplaceAllString(told[1], " ")), " ")
	name := members[1].Name
	if !strings.Contains(text, "1 sent") || !strings.Contains(text, "1 couldn't be sent") || !strings.Contains(text, name+", by email: the address was refused") || strings.Contains(text, "@") {
		t.Errorf("the line: %s", text)
	}
}

// When who to tell can't be read, the desk says nobody was pushed or
// emailed, not that nobody asked to hear.
func TestDeskSaysWhenTellingFailed(t *testing.T) {
	s := newTestServer(t, withMail())
	_, cID, _ := deskCompetition(t, s)
	c, _ := s.st.Competition(context.Background(), cID)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	pushes, emails, err := s.p.notify.tellDesk(ctx, c, store.DeskMessage{ID: "m", Text: "Hello"})
	if err == nil {
		t.Fatal("a failed read is an error")
	}
	if got := sendingNotice(pushes, emails, err); !strings.Contains(got, "failed") || strings.Contains(got, "asked to hear") {
		t.Errorf("the notice: %s", got)
	}
}
