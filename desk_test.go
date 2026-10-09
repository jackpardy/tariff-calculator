package main

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// deskBox is the heading of the messages on a recipient's page.
const deskBox = "Messages from the organisers"

// subscribeByEmail asks to hear at a notify page by email, and confirms it
// from the email sent.
func subscribeByEmail(t *testing.T, h http.Handler, mail *fakeMailer, notify, address string) {
	t.Helper()
	mail.sent = nil
	redirected(t, h, notify, url.Values{"email": {address}, "adult": {"1"}, "topic": {"cards"}})
	if len(mail.sent) != 1 {
		t.Fatalf("a link to confirm %s: %+v", address, mail.sent)
	}
	confirm := strings.TrimPrefix(regexp.MustCompile(`https://tariff\.example(/notify/confirm/\S+)`).FindString(mail.sent[0].body), "https://tariff.example")
	do(t, h, http.MethodPost, confirm, nil)
	mail.sent = nil
}

// aClubWithMembers makes a club entered in the competition, with a member
// for each name who has entered, and returns the club's page and the
// members' pages.
func aClubWithMembers(t *testing.T, h http.Handler, admin string, names ...string) (string, map[string]string) {
	t.Helper()
	clubLink := pathIn(t, do(t, h, http.MethodGet, admin, nil).Body.String(), "/competitions/club/")
	club := withoutQuery(redirected(t, h, "/clubs", url.Values{"name": {"UCD"}}))
	redirected(t, h, clubLink, url.Values{"clubAdmin": {club}})
	join := pathIn(t, do(t, h, http.MethodGet, club, nil).Body.String(), "/clubs/join/")
	entry := url.Values{"level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}}
	members := map[string]string{}
	for _, name := range names {
		m := withoutQuery(redirected(t, h, join, url.Values{"name": {name}}))
		redirected(t, h, formAction(t, do(t, h, http.MethodGet, m, nil).Body.String(), m+"/competitions/"), entry)
		members[name] = m
	}
	redirected(t, h, formAction(t, do(t, h, http.MethodGet, club, nil).Body.String(), club+"/competitions/"), url.Values{"which": {"all"}})
	return club, members
}

// notifyPageOf is the notify page a page links to, under a path prefix.
func notifyPageOf(t *testing.T, h http.Handler, page, prefix string) string {
	t.Helper()
	found := regexp.MustCompile(regexp.QuoteMeta(prefix) + `/competitions/[^/"]+/notify`).FindString(do(t, h, http.MethodGet, page, nil).Body.String())
	if found == "" {
		t.Fatalf("%s links to hearing about changes", page)
	}
	return found
}

// personOnDesk is a person's key in the desk's list of people to pick.
func personOnDesk(t *testing.T, page, name string) string {
	t.Helper()
	m := regexp.MustCompile(`name="person" value="([^"]+)"/?>\s*` + regexp.QuoteMeta(name) + `\s`).FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("%s isn't on the desk's list: %s", name, page)
	}
	return html2text(m[1])
}

// anIndividual enters a gymnast on their own and returns their page.
func anIndividual(t *testing.T, h http.Handler, admin, name string) string {
	t.Helper()
	enter := pathIn(t, do(t, h, http.MethodGet, admin, nil).Body.String(), "/competitions/enter/")
	return withoutQuery(redirected(t, h, enter, url.Values{"gymnast": {name}, "level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}}))
}

// A message to a club reaches its comp sec and members who asked, by email
// at once, and shows on every page of the club, subscribed or not.
func TestDeskMessageToAClub(t *testing.T) {
	h, _, mail := notifyServer(t)
	admin := created(t, h, newCompetition())
	club, members := aClubWithMembers(t, h, admin, "Ann", "Bea")
	subscribeByEmail(t, h, mail, notifyPageOf(t, h, members["Ann"], members["Ann"]), "ann@example.com")
	subscribeByEmail(t, h, mail, notifyPageOf(t, h, club, club), "ucd@example.com")

	desk := admin + "/desk"
	page := do(t, h, http.MethodGet, desk, nil).Body.String()
	clubID := regexp.MustCompile(`<option value="([^"]+)">UCD</option>`).FindStringSubmatch(page)
	if clubID == nil || !strings.Contains(page, "Nothing sent yet") {
		t.Fatalf("the desk lists the club: %s", page)
	}
	if !strings.Contains(do(t, h, http.MethodGet, admin, nil).Body.String(), desk) {
		t.Error("the dashboard links to the desk")
	}

	// Nothing to say, or no one to say it to: nothing is sent.
	for _, form := range []url.Values{
		{"to": {"club"}, "club": {clubID[1]}, "text": {"  "}},
		{"to": {"club"}, "club": {"nonsense"}, "text": {"Hello"}},
		{"to": {"people"}, "text": {"Hello"}},
		{"to": {"officials"}, "text": {"Hello"}},
		{"to": {"flight"}, "flight": {"0|Panel 1|BUCS L3||0"}, "text": {"Hello"}},
		{"to": {"panel"}, "panel": {"0|Panel 1"}, "text": {"Hello"}},
		{"text": {"Hello"}},
		{"to": {"club"}, "club": {clubID[1]}, "text": {strings.Repeat("a", 501)}},
	} {
		if to := redirected(t, h, desk, form); !strings.Contains(to, "Nothing+was+sent") {
			t.Errorf("%v: %s", form, to)
		}
	}
	if len(mail.sent) != 0 {
		t.Fatalf("nothing was sent: %+v", mail.sent)
	}

	to := redirected(t, h, desk, url.Values{"to": {"club"}, "club": {clubID[1]}, "text": {"Warm-up is moved to hall 2"}})
	if !strings.Contains(to, "Sent+to+UCD") || !strings.Contains(to, "2+by+email") {
		t.Errorf("the notice: %s", to)
	}
	// At once, with no wait and no Notify now.
	if len(mail.sent) != 2 {
		t.Fatalf("both addresses are emailed at once: %+v", mail.sent)
	}
	got := map[string]sentEmail{}
	for _, e := range mail.sent {
		got[e.to] = e
	}
	ann := got["ann@example.com"]
	if ann.subject != "Message from the organisers of Student Open" || !strings.HasPrefix(ann.body, "Warm-up is moved to hall 2\n") ||
		!strings.Contains(ann.body, "https://tariff.example"+members["Ann"]) || !strings.Contains(ann.body, "/notify/off/") || !strings.Contains(ann.headers["List-Unsubscribe"], "/notify/off/") {
		t.Errorf("Ann's email: %+v", ann)
	}
	if !strings.Contains(got["ucd@example.com"].body, "https://tariff.example"+club) {
		t.Errorf("the comp sec's email: %+v", got["ucd@example.com"])
	}

	// Every member and the club see it on their pages, subscribed or not.
	for name, m := range members {
		day := regexp.MustCompile(`/competitions/[^/"]+/day`).FindString(do(t, h, http.MethodGet, m, nil).Body.String())
		for _, p := range []string{m, m + day} {
			if body := do(t, h, http.MethodGet, p, nil).Body.String(); !strings.Contains(body, deskBox) || !strings.Contains(body, "Warm-up is moved to hall 2") {
				t.Errorf("%s's page %s shows the message: %s", name, p, body)
			}
		}
	}
	if body := do(t, h, http.MethodGet, club, nil).Body.String(); !strings.Contains(body, deskBox) || !strings.Contains(body, "Warm-up is moved to hall 2") {
		t.Error("the club's page shows the message")
	}

	// The desk lists it, with how many were told, and the history has it.
	page = do(t, h, http.MethodGet, desk, nil).Body.String()
	if !strings.Contains(page, "Warm-up is moved to hall 2") || !strings.Contains(page, "to UCD") || !strings.Contains(page, "told 0 by push, 2 by email") {
		t.Errorf("the desk lists what was sent: %s", page)
	}
	if history := do(t, h, http.MethodGet, admin+"/history", nil).Body.String(); !strings.Contains(history, "Sent a message to UCD") {
		t.Errorf("the history: %s", history)
	}

	// A message to one member reaches them and the comp sec (once each), not the other member.
	mail.sent = nil
	redirected(t, h, desk, url.Values{"to": {"people"}, "person": {personOnDesk(t, page, "Ann")}, "text": {"Your coach is looking for you"}})
	if len(mail.sent) != 2 {
		t.Errorf("Ann and the comp sec: %+v", mail.sent)
	}
	if body := do(t, h, http.MethodGet, members["Bea"], nil).Body.String(); strings.Contains(body, "Your coach is looking for you") {
		t.Error("Bea isn't shown what is for Ann")
	}
	if body := do(t, h, http.MethodGet, club, nil).Body.String(); !strings.Contains(body, "Your coach is looking for you") {
		t.Error("the club is shown what is for its member")
	}
}

// Everyone in a flight is told by push, and sees it on their entry page.
func TestDeskMessageToAFlight(t *testing.T) {
	h, p, _ := notifyServer(t)
	service := &fakePushService{status: http.StatusCreated}
	p.notify.http = &http.Client{Transport: service}
	admin := created(t, h, newCompetition())
	ann, bea := anIndividual(t, h, admin, "Ann Ryan"), anIndividual(t, h, admin, "Bea Walsh")

	// Ann's phone.
	ua, _ := ecdh.P256().GenerateKey(rand.Reader)
	auth := make([]byte, 16)
	rand.Read(auth)
	redirected(t, h, ann+"/notify", url.Values{"action": {"push"}, "endpoint": {"https://fcm.googleapis.com/fcm/send/ann"},
		"p256dh": {base64.RawURLEncoding.EncodeToString(ua.PublicKey().Bytes())}, "auth": {base64.RawURLEncoding.EncodeToString(auth)}, "topic": {"duties"}})

	desk := admin + "/desk"
	if page := do(t, h, http.MethodGet, desk, nil).Body.String(); !strings.Contains(page, "publish the timetable first") {
		t.Error("flights wait for the timetable")
	}
	redirected(t, h, admin+"/timetable/plan", url.Values{})
	redirected(t, h, admin+"/timetable/publish", url.Values{"action": {"publish"}})
	page := do(t, h, http.MethodGet, desk, nil).Body.String()
	flight := regexp.MustCompile(`<option value="(\d+\|[^"]+)">`).FindStringSubmatch(page)
	if flight == nil || !strings.Contains(page, "<optgroup") {
		t.Fatalf("the published flights, by day and area: %s", page)
	}

	to := redirected(t, h, desk, url.Values{"to": {"flight"}, "flight": {html2text(flight[1])}, "text": {"Your flight starts in 10 minutes"}})
	if !strings.Contains(to, "1+by+push") {
		t.Errorf("the notice: %s", to)
	}
	if len(service.got) != 1 {
		t.Fatalf("one push, at once: %d", len(service.got))
	}
	var m pushMessage
	if err := json.Unmarshal(openPush(t, ua, auth, service.bodies[0]), &m); err != nil {
		t.Fatal(err)
	}
	if m.Title != "Student Open" || m.Body != "Your flight starts in 10 minutes" || m.URL != "https://tariff.example"+ann {
		t.Errorf("the push: %+v", m)
	}
	// Bea hasn't asked to hear, but is in the flight: she sees it.
	for _, own := range []string{ann, bea, ann + "/day", bea + "/day"} {
		if body := do(t, h, http.MethodGet, own, nil).Body.String(); !strings.Contains(body, deskBox) || !strings.Contains(body, "Your flight starts in 10 minutes") {
			t.Errorf("%s shows the message: %s", own, body)
		}
	}
	if page := do(t, h, http.MethodGet, desk, nil).Body.String(); !strings.Contains(page, "told 1 by push, 0 by email") {
		t.Errorf("the desk says how many were told: %s", page)
	}

	// A phone that's gone is dropped.
	service.status = http.StatusGone
	redirected(t, h, desk, url.Values{"to": {"flight"}, "flight": {html2text(flight[1])}, "text": {"Again"}})
	if page := do(t, h, http.MethodGet, ann+"/notify", nil).Body.String(); strings.Contains(page, "A phone or browser added") {
		t.Error("a gone phone is dropped")
	}
}

// Asking someone to come to the desk says so in the subject, and is in the history.
func TestDeskCome(t *testing.T) {
	h, _, mail := notifyServer(t)
	admin := created(t, h, newCompetition())
	ann, bea := anIndividual(t, h, admin, "Ann Ryan"), anIndividual(t, h, admin, "Bea Walsh")
	subscribeByEmail(t, h, mail, ann+"/notify", "ann@example.com")
	subscribeByEmail(t, h, mail, bea+"/notify", "bea@example.com")

	desk := admin + "/desk"
	page := do(t, h, http.MethodGet, desk, nil).Body.String()
	key := personOnDesk(t, page, "Ann Ryan")
	if key != "i:ann ryan" {
		t.Errorf("an individual is picked by name: %s", key)
	}
	redirected(t, h, desk, url.Values{"to": {"people"}, "person": {key}, "come": {"1"}})
	if len(mail.sent) != 1 || mail.sent[0].to != "ann@example.com" {
		t.Fatalf("only Ann: %+v", mail.sent)
	}
	e := mail.sent[0]
	if e.subject != "Please come to the organisers' desk · Student Open" || !strings.HasPrefix(e.body, "Please come to the organisers' desk.\n") {
		t.Errorf("the email: %+v", e)
	}
	mail.sent = nil
	redirected(t, h, desk, url.Values{"to": {"people"}, "person": {key}, "come": {"1"}, "text": {"Your coach is waiting"}})
	if len(mail.sent) != 1 || !strings.HasPrefix(mail.sent[0].body, "Please come to the organisers' desk. Your coach is waiting\n") {
		t.Errorf("the request, then the text: %+v", mail.sent)
	}

	if body := do(t, h, http.MethodGet, ann, nil).Body.String(); !strings.Contains(body, "Please come to the organisers&#39; desk. Your coach is waiting") {
		t.Errorf("Ann sees it: %s", body)
	}
	if body := do(t, h, http.MethodGet, bea, nil).Body.String(); strings.Contains(body, deskBox) {
		t.Error("Bea doesn't")
	}
	if history := do(t, h, http.MethodGet, admin+"/history", nil).Body.String(); !strings.Contains(history, "Called Ann Ryan to the desk") {
		t.Errorf("the history: %s", history)
	}

	// Several people are counted in the history.
	redirected(t, h, desk, url.Values{"to": {"people"}, "person": {key, personOnDesk(t, page, "Bea Walsh")}, "text": {"Both of you"}})
	if history := do(t, h, http.MethodGet, admin+"/history", nil).Body.String(); !strings.Contains(history, "Sent a message to 2 people") {
		t.Errorf("the history: %s", history)
	}
}

// Who can use the desk: chairs and timetable links, not cards links.
func TestDeskLinks(t *testing.T) {
	h, _, _ := notifyServer(t)
	admin := created(t, h, newCompetition())
	anIndividual(t, h, admin, "Ann Ryan")
	cards := madeLink(t, h, admin, "Difficulty judges", "cards")
	chair := madeLink(t, h, admin, "Chairs", "chair")
	table := madeLink(t, h, admin, "Timetable", "timetable")
	co := madeLink(t, h, admin, "Co-organiser", "everything")

	form := url.Values{"to": {"people"}, "person": {"i:ann ryan"}, "come": {"1"}}
	if rec := do(t, h, http.MethodGet, cards+"/desk", nil); rec.Code != http.StatusForbidden {
		t.Errorf("a cards link can't see the desk: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, cards+"/desk", form); rec.Code != http.StatusForbidden {
		t.Errorf("a cards link can't send: %d", rec.Code)
	}
	if page := do(t, h, http.MethodGet, cards, nil).Body.String(); strings.Contains(page, ">Desk messages<") {
		t.Error("a cards link isn't offered the desk")
	}
	for _, link := range []string{chair, table, co, admin} {
		if rec := do(t, h, http.MethodGet, link+"/desk", nil); rec.Code != http.StatusOK {
			t.Errorf("%s sees the desk: %d", link, rec.Code)
		}
		if page := do(t, h, http.MethodGet, link, nil).Body.String(); !strings.Contains(page, ">Desk messages<") {
			t.Errorf("%s is offered the desk", link)
		}
	}
	redirected(t, h, chair+"/desk", form)
	history := do(t, h, http.MethodGet, admin+"/history", nil).Body.String()
	if !strings.Contains(history, "Called Ann Ryan to the desk") || !strings.Contains(history, "Chairs") {
		t.Errorf("the chair is in the history: %s", history)
	}
	// A message that wasn't sent isn't in the history.
	redirected(t, h, table+"/desk", url.Values{"to": {"people"}, "text": {"Nobody"}})
	if strings.Contains(do(t, h, http.MethodGet, admin+"/history", nil).Body.String(), "Sent a message to") {
		t.Error("nothing sent, nothing recorded")
	}

	// The On the day page links to the desk once the timetable is published.
	redirected(t, h, admin+"/timetable/plan", url.Values{})
	redirected(t, h, admin+"/timetable/publish", url.Values{"action": {"publish"}})
	if page := do(t, h, http.MethodGet, chair+"/day", nil).Body.String(); !strings.Contains(page, `href="`+chair+`/desk"`) {
		t.Error("the day page links to the desk")
	}
	if page := do(t, h, http.MethodGet, cards+"/day", nil).Body.String(); strings.Contains(page, "/desk") {
		t.Error("a cards link has no desk")
	}
}

// A coach hears about the members they coach, and about their club.
func TestDeskToACoach(t *testing.T) {
	h, _, mail := notifyServer(t)
	admin := created(t, h, newCompetition())
	club, members := aClubWithMembers(t, h, admin, "Ann", "Bea")
	bob := pathIn(t, do(t, h, http.MethodPost, club+"/coaches", url.Values{"name": {"Bob"}}).Body.String(), "/clubs/coach/")
	bobID := regexp.MustCompile(`<option value="([^"]+)">Bob</option>`).FindStringSubmatch(do(t, h, http.MethodGet, members["Ann"], nil).Body.String())[1]
	redirected(t, h, members["Ann"]+"/coach", url.Values{"coach": {bobID}})
	subscribeByEmail(t, h, mail, notifyPageOf(t, h, bob, bob), "bob@example.com")

	desk := admin + "/desk"
	page := do(t, h, http.MethodGet, desk, nil).Body.String()
	redirected(t, h, desk, url.Values{"to": {"people"}, "person": {personOnDesk(t, page, "Bea")}, "text": {"For Bea"}})
	if len(mail.sent) != 0 {
		t.Errorf("Bea isn't Bob's: %+v", mail.sent)
	}
	if strings.Contains(do(t, h, http.MethodGet, bob, nil).Body.String(), "For Bea") {
		t.Error("Bob isn't shown what is for Bea")
	}
	redirected(t, h, desk, url.Values{"to": {"people"}, "person": {personOnDesk(t, page, "Ann")}, "text": {"For Ann"}})
	if len(mail.sent) != 1 || mail.sent[0].to != "bob@example.com" || !strings.Contains(mail.sent[0].body, "https://tariff.example"+bob) {
		t.Errorf("Ann is Bob's: %+v", mail.sent)
	}
	if body := do(t, h, http.MethodGet, bob, nil).Body.String(); !strings.Contains(body, deskBox) || !strings.Contains(body, "For Ann") {
		t.Errorf("Bob is shown what is for Ann: %s", body)
	}
	mail.sent = nil
	clubID := regexp.MustCompile(`<option value="([^"]+)">UCD</option>`).FindStringSubmatch(page)[1]
	redirected(t, h, desk, url.Values{"to": {"club"}, "club": {clubID}, "text": {"For the club"}})
	if len(mail.sent) != 1 || mail.sent[0].to != "bob@example.com" {
		t.Errorf("a club's coaches hear what is sent to it: %+v", mail.sent)
	}
}

// Officials, all or one panel's, are told, and see it on their page.
func TestDeskToOfficials(t *testing.T) {
	h, _, mail := notifyServer(t)
	form := newCompetition()
	form.Set("tumbling", "Novice")
	admin := created(t, h, form)
	officials := admin + "/officials"
	redirected(t, h, officials+"/settings", url.Values{
		"panel-trampoline-chair": {"1"}, "panel-trampoline-execution": {"2"}, "panel-trampoline-difficulty": {"0"}, "panel-trampoline-recorder": {"0"}, "panel-trampoline-marshal": {"0"},
		"panel-tumbling-chair": {"1"}, "panel-tumbling-execution": {"1"}, "panel-tumbling-difficulty": {"0"}, "panel-tumbling-recorder": {"0"}, "panel-tumbling-marshal": {"0"}})
	redirected(t, h, officials+"/add", url.Values{"name": {"Mary"}, "judge-trampoline": {"1"}, "chair-trampoline": {"1"}})
	redirected(t, h, officials+"/add", url.Values{"name": {"Tom"}, "judge-trampoline": {"1"}})
	eve := anIndividual(t, h, admin, "Eve")
	enter := pathIn(t, do(t, h, http.MethodGet, admin, nil).Body.String(), "/competitions/enter/")
	// Dara competes in tumbling and judges trampoline.
	dara := withoutQuery(redirected(t, h, enter, url.Values{"gymnast": {"Dara"}, "discipline": {"tumbling"}, "level": {"Novice"}}))
	redirected(t, h, dara+"/offer", url.Values{"judge-trampoline": {"1"}})
	subscribeByEmail(t, h, mail, dara+"/notify", "dara@example.com")
	subscribeByEmail(t, h, mail, eve+"/notify", "eve@example.com")

	desk := admin + "/desk"
	page := do(t, h, http.MethodGet, desk, nil).Body.String()
	if strings.Contains(page, `name="panel"`) {
		t.Error("no panels before the timetable")
	}
	for _, name := range []string{"Mary", "Tom", "Dara", "Eve"} {
		personOnDesk(t, page, name)
	}
	if !strings.Contains(page, "official") || !strings.Contains(page, "gymnast") {
		t.Error("each person says what they are")
	}

	tt := admin + "/timetable"
	redirected(t, h, tt+"/setup/rules", url.Values{"add": {"1"}, "kind": {"before"}, "must": {"1"}, "event": {"Tumbling Novice"}, "event2": {"BUCS L3"}})
	redirected(t, h, tt+"/plan", url.Values{})
	redirected(t, h, tt+"/publish", url.Values{"action": {"publish"}})

	// All officials: Dara is told; Eve, who only competes, is not.
	to := redirected(t, h, desk, url.Values{"to": {"officials"}, "text": {"Judges' briefing at 9"}})
	if !strings.Contains(to, "Sent+to+All+officials") || len(mail.sent) != 1 || mail.sent[0].to != "dara@example.com" {
		t.Errorf("all officials: %s %+v", to, mail.sent)
	}
	if body := do(t, h, http.MethodGet, dara, nil).Body.String(); !strings.Contains(body, "Judges&#39; briefing at 9") {
		t.Errorf("Dara sees it: %s", body)
	}
	if body := do(t, h, http.MethodGet, eve, nil).Body.String(); strings.Contains(body, "briefing") {
		t.Error("Eve isn't an official")
	}

	// One panel's officials, on a day.
	mail.sent = nil
	page = do(t, h, http.MethodGet, desk, nil).Body.String()
	panel := regexp.MustCompile(`<select name="panel"[^>]*>\s*<option value="([^"]+)">([^<]+)</option>`).FindStringSubmatch(page)
	if panel == nil {
		t.Fatalf("a panel with officials is offered: %s", page)
	}
	redirected(t, h, desk, url.Values{"to": {"panel"}, "panel": {html2text(panel[1])}, "text": {"Panel check"}, "come": {"1"}})
	if len(mail.sent) != 1 || mail.sent[0].to != "dara@example.com" || !strings.HasPrefix(mail.sent[0].subject, "Please come to the organisers' desk") {
		t.Errorf("Dara is on the panel: %+v", mail.sent)
	}
	if history := html2text(do(t, h, http.MethodGet, admin+"/history", nil).Body.String()); !strings.Contains(history, "Called "+html2text(panel[2])+" to the desk") {
		t.Errorf("the history names the panel: %s", history)
	}

	// A flight: its gymnasts, its officials, or both. Eve competes in BUCS
	// L3; Dara judges it.
	page = do(t, h, http.MethodGet, desk, nil).Body.String()
	flight := regexp.MustCompile(`<option value="([^"]+)">[^<]*BUCS L3[^<]*</option>`).FindStringSubmatch(page)
	if flight == nil {
		t.Fatalf("BUCS L3's flight is offered: %s", page)
	}
	key := html2text(flight[1])
	send := func(extra url.Values) (string, []string) {
		t.Helper()
		mail.sent = nil
		form := url.Values{"to": {"flight"}, "flight": {key}, "flightChoice": {"1"}, "text": {"Flight news"}}
		for k, v := range extra {
			form[k] = v
		}
		to := redirected(t, h, desk, form)
		var told []string
		for _, m := range mail.sent {
			told = append(told, m.to)
		}
		slices.Sort(told)
		return to, told
	}
	if to, told := send(url.Values{"flightOfficials": {"1"}}); !strings.Contains(to, "officials") || strings.Join(told, " ") != "dara@example.com" {
		t.Errorf("its officials: %s %v", to, told)
	}
	if _, told := send(url.Values{"flightGymnasts": {"1"}}); strings.Join(told, " ") != "eve@example.com" {
		t.Errorf("its gymnasts: %v", told)
	}
	if _, told := send(url.Values{"flightGymnasts": {"1"}, "flightOfficials": {"1"}}); strings.Join(told, " ") != "dara@example.com eve@example.com" {
		t.Errorf("both: %v", told)
	}
	if to, _ := send(nil); !strings.Contains(to, "Nothing+was+sent") {
		t.Errorf("neither ticked: %s", to)
	}
}
