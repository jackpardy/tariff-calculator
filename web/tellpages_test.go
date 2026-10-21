package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"tariffCalculator/store"
)

// tellFormOf is the Tell the organiser form on a page: where it posts, its
// hidden fields, and the entries it can be about (label: id; a comp sec's
// or coach's are by gymnast, under their event) and whole events (as
// "all of <event>": the event).
func tellFormOf(t *testing.T, page string) (action string, fields url.Values, about map[string]string, ok bool) {
	t.Helper()
	page = unescape(page)
	start := strings.Index(page, `class="comp-tell`)
	if start < 0 {
		return "", nil, nil, false
	}
	form := page[start:]
	form = form[:strings.Index(form, "</form>")]
	action = regexp.MustCompile(`action="([^"]+/organiser)"`).FindStringSubmatch(form)[1]
	fields = url.Values{}
	for _, m := range regexp.MustCompile(`type="hidden" name="([^"]+)" value="([^"]*)"`).FindAllStringSubmatch(form, -1) {
		fields.Set(m[1], m[2])
	}
	about = map[string]string{}
	for _, m := range regexp.MustCompile(`name="entry" value="([^"]+)">\s*([^<]+?)\s*</label>`).FindAllStringSubmatch(form, -1) {
		about[m[2]] = m[1]
	}
	for _, m := range regexp.MustCompile(`name="event" value="([^"]+)"`).FindAllStringSubmatch(form, -1) {
		about["all of "+m[1]] = m[1]
	}
	return action, fields, about, true
}

// messagesOn is the dashboard's messages and concerns, as the organiser
// sees them: each one's text and who sent it, newest first.
func messagesOn(t *testing.T, h http.Handler, admin string) [][2]string {
	t.Helper()
	page := unescape(do(t, h, http.MethodGet, admin, nil).Body.String())
	var out [][2]string
	for _, m := range regexp.MustCompile(`(?s)<div class="comp-concern[^"]*" id="concern-[^"]+">(?:<p class="mb-1">.*?</p>)?<p>(.*?)</p><p class="is-size-7 has-text-grey">(.*?) · `).FindAllStringSubmatch(page, -1) {
		text := regexp.MustCompile(`<[^>]+>`).ReplaceAllString(m[1], "")
		out = append(out, [2]string{strings.TrimSpace(text), m[2]})
	}
	return out
}

// A member, a coach, a comp sec and an individual tell the organiser
// something from their pages for the competition: named from their link,
// about one of their entries or the competition, on the dashboard.
func TestTellOrganiser(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	ucd := aClub(t, h, admin, "UCD", true, "Ann", "Cian")
	ann := withoutQuery(formAction(t, memberCompetitionPage(t, h, ucd.members["Ann"]), ucd.members["Ann"]+"/competitions/"))
	coach := pathIn(t, do(t, h, http.MethodPost, ucd.page+"/coaches", url.Values{"name": {"Cara"}}).Body.String(), "/clubs/coach/")
	own := anIndividual(t, h, admin, "Bea Byrne")
	compSec := regexp.MustCompile(`href="(` + regexp.QuoteMeta(ucd.page) + `/competitions/[^"/?]+)"`).FindStringSubmatch(do(t, h, http.MethodGet, ucd.page, nil).Body.String())[1]
	bea := regexp.MustCompile(`action="(/competitions/entry/[^"]+)/organiser"`).FindStringSubmatch(do(t, h, http.MethodGet, own, nil).Body.String())
	gymnast := regexp.MustCompile(`href="(/competitions/gymnast/[^"/]+/competitions/[^"/?]+)"`).FindStringSubmatch(do(t, h, http.MethodGet, unescape(bea[1])+"/day", nil).Body.String())
	if bea == nil || gymnast == nil {
		t.Fatal("Bea's page has the form")
	}

	for _, tc := range []struct {
		from, page, sender string
		about              []string // the entries it can be about
		back               string   // the page it comes back to
	}{
		{"a member", ann, "Ann, a member of UCD", []string{"Ann · BUCS L3"}, ann},
		{"a member's My competition", ann + "/day", "Ann, a member of UCD", []string{"Ann · BUCS L3"}, ann + "/day"},
		{"a comp sec", compSec, "UCD's comp sec", []string{"Ann", "Cian", "all of BUCS L3"}, compSec},
		// No one has chosen a coach yet, so Cara sees them all.
		{"a coach", coach, "Cara, a coach at UCD", []string{"Ann", "Cian", "all of BUCS L3"}, coach},
		// An entry's link opens the individual's page for the competition.
		{"an individual", own, "Bea Byrne, entered on their own", []string{"Bea Byrne · BUCS L3"}, unescape(gymnast[1])},
		{"an individual's My competition", unescape(bea[1]) + "/day", "Bea Byrne, entered on their own", []string{"Bea Byrne · BUCS L3"}, unescape(bea[1]) + "/day"},
	} {
		page := do(t, h, http.MethodGet, tc.page, nil).Body.String()
		action, fields, about, ok := tellFormOf(t, page)
		if !ok {
			t.Errorf("%s has the form", tc.from)
			continue
		}
		if !strings.Contains(unescape(page), "From <strong>"+tc.sender+"</strong>, to the competition's organiser.") {
			t.Errorf("%s: the form says who it's from", tc.from)
		}
		if len(about) != len(tc.about) {
			t.Errorf("%s: about %v, want %v", tc.from, about, tc.about)
		}
		for _, a := range tc.about {
			if about[a] == "" {
				t.Errorf("%s: can be about %s: %v", tc.from, a, about)
			}
		}
		// A name typed in is never used: it's from the link.
		fields.Set("text", "From "+tc.from)
		fields.Set("who", "Someone else")
		to := redirected(t, h, action, fields)
		if withoutQuery(to) != tc.back || noticeOf(t, to) != "Sent to the organiser of Student Open, with your name." {
			t.Errorf("%s: back to %s, want %s", tc.from, to, tc.back)
		}
		if got := messagesOn(t, h, admin); len(got) == 0 || got[0] != [2]string{"From " + tc.from, tc.sender} {
			t.Errorf("%s: the dashboard's newest is %v", tc.from, got)
		}
		// The page says it was sent.
		if !strings.Contains(unescape(do(t, h, http.MethodGet, to, nil).Body.String()), "Sent to the organiser of Student Open, with your name.") {
			t.Errorf("%s: the page says it's sent", tc.from)
		}
	}

	// About one of their entries: the dashboard links to it.
	action, fields, about, _ := tellFormOf(t, do(t, h, http.MethodGet, ann, nil).Body.String())
	fields.Set("text", "My surname is missing")
	fields.Set("entry", about["Ann · BUCS L3"])
	redirected(t, h, action, fields)
	page := do(t, h, http.MethodGet, admin, nil).Body.String()
	if !regexp.MustCompile(`<a href="` + regexp.QuoteMeta(admin) + `/entries/[^"]+">Ann · BUCS L3</a>: My surname is missing`).MatchString(page) {
		t.Errorf("the organiser sees which entry: %s", page)
	}
	if !strings.Contains(page, "messages and concerns open") || !strings.Contains(page, "Messages and concerns") {
		t.Error("the dashboard names them messages and concerns")
	}
	// ...never someone else's.
	_, _, beas, _ := tellFormOf(t, do(t, h, http.MethodGet, own, nil).Body.String())
	fields.Set("entry", beas["Bea Byrne · BUCS L3"])
	if rec := do(t, h, http.MethodPost, action, fields); rec.Code != http.StatusNotFound {
		t.Errorf("about someone else's entry: %d", rec.Code)
	}
	// Nothing said, nothing sent.
	fields.Del("entry")
	fields.Set("text", "  ")
	if to := redirected(t, h, action, fields); noticeOf(t, to) != "Say what you want to tell the organiser; nothing was sent." {
		t.Errorf("empty: %s", to)
	}

	// Once Ann chooses Cara, another coach sees only Cian.
	dan := pathIn(t, do(t, h, http.MethodPost, ucd.page+"/coaches", url.Values{"name": {"Dan"}}).Body.String(), "/clubs/coach/")
	coachID := regexp.MustCompile(`/coaches/([^/"]+)/`).FindStringSubmatch(do(t, h, http.MethodGet, ucd.page+"/coaches", nil).Body.String())[1]
	redirected(t, h, ucd.members["Ann"]+"/coach", url.Values{"coach": {coachID}})
	if _, _, about, _ := tellFormOf(t, do(t, h, http.MethodGet, dan, nil).Body.String()); len(about) != 2 || about["Cian"] == "" {
		t.Errorf("Dan's gymnasts: %v", about)
	}

	// The organiser resolves a message, as a concern.
	id := regexp.MustCompile(`/concerns/([^/"]+)/resolve`).FindStringSubmatch(page)[1]
	if to := redirected(t, h, admin+"/concerns/"+id+"/resolve", url.Values{"note": {"Fixed"}}); noticeOf(t, to) != "Marked resolved. They see it on their page." {
		t.Errorf("resolved: %s", to)
	}

	// The Feedback tab says it isn't for the organiser.
	if !strings.Contains(unescape(do(t, h, http.MethodGet, ann, nil).Body.String()), "This doesn't go to the competition's organiser: for them, use <strong>Tell the organiser</strong> on this page.") {
		t.Error("the Feedback tab says it doesn't go to the organiser")
	}
	// The organiser's own pages have no form.
	if _, _, _, ok := tellFormOf(t, do(t, h, http.MethodGet, admin, nil).Body.String()); ok {
		t.Error("not on the organiser's pages")
	}
}

// The organiser turns messages off on Settings: the forms go, and nothing
// more is taken; on again, they're back.
func TestTellOrganiserOff(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	ucd := aClub(t, h, admin, "UCD", true, "Ann")
	ann := withoutQuery(formAction(t, memberCompetitionPage(t, h, ucd.members["Ann"]), ucd.members["Ann"]+"/competitions/"))
	action, fields, _, ok := tellFormOf(t, do(t, h, http.MethodGet, ann, nil).Body.String())
	if !ok {
		t.Fatal("on at first")
	}
	if !strings.Contains(do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "members, coaches, comp secs and gymnasts can send you messages") {
		t.Error("Settings says it's on")
	}

	if to := redirected(t, h, admin+"/messages", url.Values{"on": {"0"}}); !strings.HasPrefix(to, admin+"/settings?notice=") || !strings.HasSuffix(to, "#messages") {
		t.Errorf("back to Settings: %s", to)
	}
	if !strings.Contains(do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "turned off") {
		t.Error("Settings says it's off")
	}
	for _, p := range []string{ann, ann + "/day", ucd.page} {
		if _, _, _, ok := tellFormOf(t, do(t, h, http.MethodGet, p, nil).Body.String()); ok {
			t.Errorf("%s has no form while it's off", p)
		}
	}
	fields.Set("text", "Still there?")
	if to := redirected(t, h, action, fields); noticeOf(t, to) != "The organiser isn't taking messages here just now; nothing was sent." {
		t.Errorf("sent while off: %s", to)
	}
	if got := messagesOn(t, h, admin); len(got) != 0 {
		t.Errorf("nothing is kept: %v", got)
	}

	redirected(t, h, admin+"/messages", url.Values{"on": {"1"}})
	if _, _, _, ok := tellFormOf(t, do(t, h, http.MethodGet, ann, nil).Body.String()); !ok {
		t.Error("back on")
	}
	history := do(t, h, http.MethodGet, admin+"/history", nil).Body.String()
	if !strings.Contains(history, "Changed whether people can send you messages") {
		t.Error("the history says so")
	}
	// Only the organiser's and everything links change it.
	cards := madeLink(t, h, admin, "Judges", "cards")
	if rec := do(t, h, http.MethodPost, cards+"/messages", url.Values{"on": {"0"}}); rec.Code != http.StatusForbidden {
		t.Errorf("a helper's link: %d", rec.Code)
	}
}

// Each sender can send only so many a day; others still can.
func TestTellOrganiserLimit(t *testing.T) {
	s := newTestServer(t, atTime(inDublin(2030, 1, 10, 12, 0)))
	h := s.h
	admin := created(t, h, newCompetition())
	ucd := aClub(t, h, admin, "UCD", true, "Ann", "Cian")
	send := func(member, text string) string {
		t.Helper()
		page := withoutQuery(formAction(t, memberCompetitionPage(t, h, member), member+"/competitions/"))
		action, fields, _, _ := tellFormOf(t, do(t, h, http.MethodGet, page, nil).Body.String())
		fields.Set("text", text)
		return noticeOf(t, redirected(t, h, action, fields))
	}
	for i := range store.MaxMessagesPerDay {
		if got := send(ucd.members["Ann"], "Hello "+string(rune('a'+i))); !strings.HasPrefix(got, "Sent") {
			t.Fatalf("message %d: %s", i+1, got)
		}
	}
	if got := send(ucd.members["Ann"], "One more"); got != "You've sent the organiser a lot today, so this wasn't sent. Try again tomorrow, or speak to them." {
		t.Errorf("past the limit: %s", got)
	}
	if got := send(ucd.members["Cian"], "Me too"); !strings.HasPrefix(got, "Sent") {
		t.Errorf("another member still can: %s", got)
	}
	s.clock.set(inDublin(2030, 1, 11, 12, 1))
	if got := send(ucd.members["Ann"], "Next day"); !strings.HasPrefix(got, "Sent") {
		t.Errorf("the next day: %s", got)
	}
}

// sentOn is the "Your messages to the organiser" list on a sender's page:
// each message's status, its text, and the thread after it, as text.
func sentOn(t *testing.T, h http.Handler, page string) []string {
	t.Helper()
	body := unescape(do(t, h, http.MethodGet, page, nil).Body.String())
	var out []string
	for _, m := range regexp.MustCompile(`(?s)<div class="comp-tell-sent" id="message-[^"]+">(.*?)(?:<details|</div></div>)`).FindAllStringSubmatch(body, -1) {
		text := regexp.MustCompile(`<[^>]+>`).ReplaceAllString(m[1], " ")
		out = append(out, strings.Join(strings.Fields(text), " "))
	}
	return out
}

// A comp sec's message is about whole events and entries, an individual's
// about each of their entries; the organiser says it's received, replies,
// and resolves it with a reply and a note of their own; the sender sees
// each, adds to it (a thread, opening it again), and hears of it by email.
func TestMessageThreads(t *testing.T) {
	s := newTestServer(t, withMail())
	h, mail := s.h, s.mail
	form := newCompetition()
	form.Set("tumbling", "Novice")
	admin := created(t, h, form)
	ucd := aClub(t, h, admin, "UCD", true, "Ann", "Cian")
	compSec := regexp.MustCompile(`href="(` + regexp.QuoteMeta(ucd.page) + `/competitions/[^"/?]+)"`).FindStringSubmatch(do(t, h, http.MethodGet, ucd.page, nil).Body.String())[1]

	// An individual with two entries can pick both.
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	own := withoutQuery(redirected(t, h, enter, bucsEntry("Bea Byrne")))
	redirected(t, h, enter, url.Values{"gymnast": {"Bea Byrne"}, "discipline": {"tumbling"}, "level": {"Novice"}, "as": {"http://example.com" + own}})
	bea := own + "/day"
	action, fields, about, _ := tellFormOf(t, do(t, h, http.MethodGet, bea, nil).Body.String())
	if len(about) != 2 || about["all of BUCS L3"] != "" {
		t.Fatalf("Bea's two entries, no whole events: %v", about)
	}
	for _, id := range about {
		fields.Add("entry", id)
	}
	fields.Set("text", "Both my routines are the wrong way round")
	redirected(t, h, action, fields)
	if got := messagesOn(t, h, admin); len(got) != 1 || !strings.Contains(got[0][0], "Bea Byrne · BUCS L3") || !strings.Contains(got[0][0], "Bea Byrne · Tumbling Novice") {
		t.Errorf("about both: %v", got)
	}
	// ...but not a whole event.
	fields.Set("event", "BUCS L3")
	if rec := do(t, h, http.MethodPost, action, fields); rec.Code != http.StatusNotFound {
		t.Errorf("an individual's whole event: %d", rec.Code)
	}

	// A comp sec: the whole of BUCS L3, which covers Ann's entry.
	action, fields, about, _ = tellFormOf(t, do(t, h, http.MethodGet, compSec, nil).Body.String())
	fields.Set("event", about["all of BUCS L3"])
	fields.Set("entry", about["Ann"])
	fields.Set("text", "What time do our L3s warm up?")
	redirected(t, h, action, fields)
	got := messagesOn(t, h, admin)
	if got[0][0] != "all of UCD's BUCS L3: What time do our L3s warm up?" || got[0][1] != "UCD's comp sec" {
		t.Errorf("about the whole event, not the entry it covers: %v", got[0])
	}
	if sent := sentOn(t, h, compSec); len(sent) != 1 || !strings.HasPrefix(sent[0], "Sent") || !strings.Contains(sent[0], "about all of BUCS L3") {
		t.Errorf("the comp sec sees it, sent: %v", sent)
	}
	// An event they've no entries in is a page that isn't there.
	fields.Set("event", "Tumbling Novice")
	if rec := do(t, h, http.MethodPost, action, fields); rec.Code != http.StatusNotFound {
		t.Errorf("an event without their entries: %d", rec.Code)
	}

	// Ann hears by email.
	ann := withoutQuery(formAction(t, memberCompetitionPage(t, h, ucd.members["Ann"]), ucd.members["Ann"]+"/competitions/"))
	redirected(t, h, ann+"/notify", url.Values{"email": {"ann@example.com"}, "adult": {"1"}, "topic": {"cards"}})
	confirm := strings.TrimPrefix(regexp.MustCompile(`https://tariff\.example(/notify/confirm/\S+)`).FindString(mail.sent[len(mail.sent)-1].body), "https://tariff.example")
	do(t, h, http.MethodPost, confirm, nil)
	action, fields, _, _ = tellFormOf(t, do(t, h, http.MethodGet, ann, nil).Body.String())
	fields.Set("text", "My surname is spelled wrong")
	redirected(t, h, action, fields)
	dash := do(t, h, http.MethodGet, admin, nil).Body.String()
	id := regexp.MustCompile(`id="concern-([^"]+)"`).FindStringSubmatch(dash)[1]
	if !strings.Contains(dash, "Waiting for you") {
		t.Error("a new message waits for the organiser")
	}

	// Received.
	mail.sent = nil
	if to := redirected(t, h, admin+"/concerns/"+id+"/received", nil); noticeOf(t, to) != "Marked received. They see it on their page, and are told by push or email." {
		t.Errorf("received: %s", noticeOf(t, to))
	}
	if sent := sentOn(t, h, ann); !strings.HasPrefix(sent[0], "Received") {
		t.Errorf("Ann sees it's received: %v", sent)
	}
	if len(mail.sent) != 1 || mail.sent[0].to != "ann@example.com" || mail.sent[0].subject != "The organiser of Student Open has your message" ||
		!strings.Contains(mail.sent[0].body, "> My surname is spelled wrong") {
		t.Errorf("Ann is told: %+v", mail.sent)
	}
	if to := redirected(t, h, admin+"/concerns/"+id+"/received", nil); noticeOf(t, to) != "That was marked received already; nothing was changed." {
		t.Errorf("received twice: %s", noticeOf(t, to))
	}

	// A reply, then Ann adds to it.
	mail.sent = nil
	redirected(t, h, admin+"/concerns/"+id+"/reply", url.Values{"reply": {"How should it be spelled?"}})
	if len(mail.sent) != 1 || mail.sent[0].subject != "Reply from the organiser of Student Open" || !strings.Contains(mail.sent[0].body, "How should it be spelled?") {
		t.Errorf("Ann is sent the reply: %+v", mail.sent)
	}
	add := regexp.MustCompile(`action="(` + regexp.QuoteMeta(ann) + `/organiser/[^"]+)"`).FindStringSubmatch(do(t, h, http.MethodGet, ann, nil).Body.String())
	if add == nil {
		t.Fatal("Ann can add to it")
	}
	if to := redirected(t, h, add[1], url.Values{"text": {"Ann Ó Riain"}}); noticeOf(t, to) != "Added, for the organiser to see." || !strings.HasSuffix(to, "#message-"+id) {
		t.Errorf("added: %s", to)
	}
	if sent := sentOn(t, h, ann); !strings.Contains(sent[0], "The organiser replied") || !strings.Contains(sent[0], "How should it be spelled?") ||
		!strings.Contains(sent[0], "You added") || !strings.Contains(sent[0], "Ann Ó Riain") {
		t.Errorf("Ann sees the thread: %v", sent)
	}
	dash = unescape(do(t, h, http.MethodGet, admin, nil).Body.String())
	if !strings.Contains(dash, "They added") || !strings.Contains(dash, "Ann Ó Riain") {
		t.Error("the organiser sees what Ann added")
	}
	// Not someone else's.
	cian := withoutQuery(formAction(t, memberCompetitionPage(t, h, ucd.members["Cian"]), ucd.members["Cian"]+"/competitions/"))
	if rec := do(t, h, http.MethodPost, cian+"/organiser/"+id, url.Values{"text": {"Me too"}}); rec.Code != http.StatusNotFound {
		t.Errorf("adding to someone else's: %d", rec.Code)
	}

	// Resolved with a reply Ann sees and a note she doesn't.
	mail.sent = nil
	redirected(t, h, admin+"/concerns/"+id+"/resolve", url.Values{"reply": {"Fixed on the timetable"}, "note": {"Changed in Entries"}})
	sent := sentOn(t, h, ann)
	if !strings.HasPrefix(sent[0], "Resolved") || !strings.Contains(sent[0], "Fixed on the timetable") || strings.Contains(unescape(do(t, h, http.MethodGet, ann, nil).Body.String()), "Changed in Entries") {
		t.Errorf("Ann sees it resolved, with the reply, not the note: %v", sent)
	}
	if len(mail.sent) != 1 || !strings.Contains(mail.sent[0].body, "replied, and marked your message resolved") {
		t.Errorf("Ann is told: %+v", mail.sent)
	}
	if !strings.Contains(unescape(do(t, h, http.MethodGet, admin, nil).Body.String()), "your note: Changed in Entries") {
		t.Error("the organiser sees their note")
	}
	if !historyHas(t, h, admin, "Organiser", "Replied to a message and resolved it") || !historyHas(t, h, admin, "Organiser", "Said a message was received") {
		t.Error("the history says so")
	}

	// More from Ann opens it again.
	redirected(t, h, add[1], url.Values{"text": {"Still wrong on my card"}})
	if sent := sentOn(t, h, ann); !strings.HasPrefix(sent[0], "Received") {
		t.Errorf("open again, still received: %v", sent)
	}
	if !strings.Contains(do(t, h, http.MethodGet, admin, nil).Body.String(), "Waiting for you") {
		t.Error("it waits for the organiser again")
	}

	// With messages off, Ann still sees hers, without adding.
	redirected(t, h, admin+"/messages", url.Values{"on": {"0"}})
	page := do(t, h, http.MethodGet, ann, nil).Body.String()
	if sent := sentOn(t, h, ann); len(sent) != 1 || strings.Contains(page, `/organiser/`+id+`"`) {
		t.Errorf("hers, without adding: %v", sent)
	}
}
