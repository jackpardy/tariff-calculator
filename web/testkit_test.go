package web

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"

	"tariffCalculator/store"
)

// The shared test kit: a server with a clock and mailer of the test's
// choosing, the usual entry and club, and named readers for the pages the
// day runs on (the venue screen, On the day and My competition), so tests
// check a card or a row rather than a word anywhere on the page.

// testClock is a clock a test sets by hand.
type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *testClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

// set moves the clock to a time.
func (c *testClock) set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = t
}

// testServer is the app on a fresh store, with what a test may reach into.
type testServer struct {
	h     http.Handler
	p     *competitionPages
	st    *store.Store
	mail  *fakeMailer // nil unless withMail
	clock *testClock  // nil unless atTime
}

// serverOption changes a test server before it's used.
type serverOption func(*testServer)

// atTime runs the server (its pages, notifier and store) on a clock set
// to now, which the test can move.
func atTime(now time.Time) serverOption {
	return func(s *testServer) {
		s.clock = &testClock{t: now}
		s.p.now, s.p.notify.now = s.clock.now, s.clock.now
		s.st.SetClock(s.clock.now)
	}
}

// withMail turns email on, to a fake mailer.
func withMail() serverOption {
	return func(s *testServer) {
		s.mail = &fakeMailer{}
		s.p.notify.mail, s.p.notify.base = s.mail, "https://tariff.example"
	}
}

// newTestServer is the app with storage in a fresh directory.
func newTestServer(t *testing.T, opts ...serverOption) *testServer {
	t.Helper()
	st, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	s := &testServer{p: newCompetitionPages(st), st: st}
	for _, o := range opts {
		o(s)
	}
	s.h = routesWithPages(s.p)
	return s
}

// competitionServerAt is the app on a clock set to now.
func competitionServerAt(t *testing.T, now time.Time) (http.Handler, *testClock) {
	t.Helper()
	s := newTestServer(t, atTime(now))
	return s.h, s.clock
}

// inDublin is a time on the competitions' clock.
func inDublin(year int, month time.Month, day, hour, minute int) time.Time {
	return time.Date(year, month, day, hour, minute, 0, 0, local)
}

// bucsEntry is the usual BUCS L3 entry form: the first set option and a
// voluntary. A name makes it an individual's (a member's entry has none).
func bucsEntry(name string) url.Values {
	v := url.Values{"level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}}
	if name != "" {
		v.Set("gymnast", name)
	}
	return v
}

// testClub is a club entered in a competition, and its members' pages by
// name.
type testClub struct {
	page    string
	members map[string]string
}

// aClub makes a club with this name and enters it in the competition, with
// a member for each name. With entered, each member enters BUCS L3 and the
// club sends them; otherwise they've only joined.
func aClub(t *testing.T, h http.Handler, admin, name string, entered bool, members ...string) testClub {
	t.Helper()
	clubLink := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/club/")
	c := testClub{page: withoutQuery(redirected(t, h, "/clubs", url.Values{"name": {name}})), members: map[string]string{}}
	redirected(t, h, clubLink, url.Values{"clubAdmin": {c.page}})
	join := pathIn(t, do(t, h, http.MethodGet, c.page+"/members", nil).Body.String(), "/clubs/join/")
	for _, m := range members {
		c.members[m] = withoutQuery(redirected(t, h, join, url.Values{"name": {m}}))
		if entered {
			redirected(t, h, formAction(t, memberCompetitionPage(t, h, c.members[m]), c.members[m]+"/competitions/"), bucsEntry(""))
		}
	}
	if entered && len(members) > 0 {
		redirected(t, h, formAction(t, do(t, h, http.MethodGet, c.page, nil).Body.String(), c.page+"/competitions/"), url.Values{"which": {"all"}})
	}
	return c
}

// unescape undoes HTML escaping, in text or an attribute.
func unescape(s string) string {
	return html.UnescapeString(s)
}

// Reading pages.

func parsePage(t *testing.T, page string) *html.Node {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func attrOf(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasClass(n *html.Node, class string) bool {
	return n.Type == html.ElementNode && strings.Contains(" "+attrOf(n, "class")+" ", " "+class+" ")
}

// byClass are the elements under n with a class, in order.
func byClass(n *html.Node, class string) []*html.Node {
	var out []*html.Node
	for d := range n.Descendants() {
		if hasClass(d, class) {
			out = append(out, d)
		}
	}
	return out
}

// firstByClass is the first element under n with a class, or nil.
func firstByClass(n *html.Node, class string) *html.Node {
	if all := byClass(n, class); len(all) > 0 {
		return all[0]
	}
	return nil
}

// byTag are the elements under n with a tag, in order.
func byTag(n *html.Node, tag string) []*html.Node {
	var out []*html.Node
	for d := range n.Descendants() {
		if d.Type == html.ElementNode && d.Data == tag {
			out = append(out, d)
		}
	}
	return out
}

// textOf is the text in n, with its spaces collapsed; "" for nil.
func textOf(n *html.Node) string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	for d := range n.Descendants() {
		if d.Type == html.TextNode {
			b.WriteString(d.Data + " ")
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// The venue screen.

// screenCard is an area on the venue screen: a card, or a strip when it's
// quiet.
type screenCard struct {
	Late      string
	Behind    bool
	Now       string // the flight or break on now, "—" for nothing
	NowTimes  string // e.g. "09:00–10:30"
	Planned   bool   // on by the plan only ("not started yet")
	Gymnasts  []string
	Scratched []string // those struck through
	Up        []string // who's on and next, "name · routine", once the marshal taps
	Next      string   // the next flight, "—" for none
	WarmUp    string   // the next flight's warm-up
	Finished  bool     // finished for the day
	Strip     string   // the strip's text, for a quiet area
}

// screenCards are the venue screen's areas, by name.
func screenCards(t *testing.T, body string) map[string]screenCard {
	t.Helper()
	doc := parsePage(t, body)
	out := map[string]screenCard{}
	for _, card := range byClass(doc, "screen-card") {
		var c screenCard
		name := textOf(firstByClass(card, "screen-area"))
		if late := firstByClass(card, "screen-late"); late != nil {
			c.Late, c.Behind = textOf(late), hasClass(late, "is-behind")
		}
		parts := byClass(card, "screen-part")
		if len(parts) != 2 {
			t.Fatalf("%s's card has %d parts", name, len(parts))
		}
		now, next := parts[0], parts[1]
		c.Now = textOf(firstByClass(now, "screen-flight"))
		if sub := firstByClass(now, "screen-sub"); sub != nil {
			times, planned := strings.CutSuffix(textOf(sub), " · not started yet")
			c.NowTimes, c.Planned = times, planned
		}
		for _, li := range byTag(now, "li") {
			if n := firstByClass(li, "screen-name"); n != nil {
				c.Gymnasts = append(c.Gymnasts, textOf(n))
				if hasClass(li, "comp-scratched") {
					c.Scratched = append(c.Scratched, textOf(n))
				}
			}
		}
		c.Up = screenUpIn(renderNode(now))
		label := textOf(firstByClass(next, "screen-label"))
		c.Finished = label == "Finished for the day"
		if !c.Finished {
			c.Next = textOf(firstByClass(next, "screen-flight"))
			c.WarmUp, _ = strings.CutPrefix(label, "Next · warm-up ")
			if c.WarmUp == "Next" {
				c.WarmUp = ""
			}
		}
		out[name] = c
	}
	for _, strip := range byClass(doc, "screen-strip") {
		name := textOf(firstByClass(strip, "screen-band"))
		out[name] = screenCard{Strip: strings.TrimSpace(strings.TrimPrefix(textOf(strip), name))}
	}
	return out
}

// renderNode is a node as HTML again.
func renderNode(n *html.Node) string {
	var b strings.Builder
	if err := html.Render(&b, n); err != nil {
		return ""
	}
	return b.String()
}

// On the day.

// dayArea is an area on the organiser's On the day page.
type dayArea struct {
	Status  string // "Nothing started yet", "On time", "Running 15 min late"…
	Since   string // e.g. "measured from BUCS L3 started at 09:20"
	Late    bool   // the warning tag
	Early   bool
	WhatIf  string // the link to the delay page, "" for none
	Flights []dayFlight
}

// dayFlight is a flight's box on On the day.
type dayFlight struct {
	Key, Name, Planned string // Key is what its forms post
	Started, Finished  string // "" for not yet
	Who                string
	Buttons            []string          // the start/finish/undo and tap buttons
	UpLabel            string            // "First up…", "On now", or the note that nobody is left
	Up, Routine        string            // who's on and their routine
	Last               bool              // the note to tap Finished
	Checkin            string            // e.g. "3 of 3 here"
	Gymnasts           map[string]string // name: "here", "scratched" or ""
	Order              []string          // the gymnasts in running order
}

// dayPageAreas are the On the day page's areas, by name.
func dayPageAreas(t *testing.T, page string) map[string]dayArea {
	t.Helper()
	doc := parsePage(t, page)
	out := map[string]dayArea{}
	root := byID(doc, "day-areas")
	if root == nil {
		return out
	}
	for _, col := range byClass(root, "column") {
		block := firstElementChild(col)
		if block == nil {
			continue
		}
		var a dayArea
		name := textOf(firstByClass(block, "title"))
		if tag := firstByClass(block, "tag"); tag != nil {
			a.Status, a.Late, a.Early = textOf(tag), hasClass(tag, "is-warning"), hasClass(tag, "is-info")
			if since := firstByClass(tag.Parent, "has-text-grey"); since != nil {
				a.Since = textOf(since)
			}
		}
		for _, link := range byTag(block, "a") {
			if textOf(link) == "What if: see the rest of the day" {
				a.WhatIf = unescape(attrOf(link, "href"))
			}
		}
		for _, box := range byClass(block, "comp-flight") {
			a.Flights = append(a.Flights, dayFlightOf(box))
		}
		out[name] = a
	}
	return out
}

func dayFlightOf(box *html.Node) dayFlight {
	f := dayFlight{Gymnasts: map[string]string{}}
	ps := byTag(box, "p")
	f.Name = textOf(ps[0])
	for _, in := range byTag(box, "input") {
		if attrOf(in, "name") == "flight" {
			f.Key = attrOf(in, "value")
			break
		}
	}
	f.Planned = strings.TrimPrefix(textOf(ps[1]), "Planned ")
	// "Started <strong>09:02</strong> · finished — · Organiser"
	label := ""
	for c := ps[2].FirstChild; c != nil; c = c.NextSibling {
		switch {
		case c.Type == html.TextNode && strings.Contains(c.Data, "Started"):
			label = "started"
		case c.Type == html.TextNode && strings.Contains(c.Data, "finished"):
			label = "finished"
		case c.Type == html.ElementNode && c.Data == "strong" && label == "started":
			f.Started = textOf(c)
		case c.Type == html.ElementNode && c.Data == "strong" && label == "finished":
			f.Finished = textOf(c)
		case c.Type == html.ElementNode && c.Data == "span":
			f.Who = strings.TrimPrefix(textOf(c), "· ")
		}
	}
	for _, b := range byTag(box, "button") {
		if attrOf(b, "name") == "what" {
			f.Buttons = append(f.Buttons, textOf(b))
		}
	}
	if up := firstByClass(box, "comp-up"); up != nil {
		ups := byTag(up, "p")
		f.UpLabel = textOf(ups[0])
		if len(ups) > 2 {
			// The name is the span after the running number.
			for _, s := range byTag(ups[1], "span") {
				if !hasClass(s, "comp-run-no") && !hasClass(s, "tag") {
					f.Up = textOf(s)
					break
				}
			}
			f.Routine = textOf(byTag(ups[2], "strong")[0])
		}
		f.Last = strings.Contains(textOf(up), "The flight's last routine")
	}
	for _, d := range byClass(box, "comp-checkin") {
		if attr := attrOf(d, "data-key"); !strings.HasPrefix(attr, "checkin:") {
			continue
		}
		f.Checkin = strings.TrimPrefix(textOf(firstChildTag(d, "summary")), "Check-in · ")
		for _, row := range byClass(d, "comp-checkin-row") {
			name := firstByClass(row, "comp-checkin-name")
			who := ""
			for _, s := range byTag(name, "span") {
				if !hasClass(s, "comp-run-no") && s.Parent == name {
					who = textOf(s)
					break
				}
			}
			state := ""
			for _, tag := range byClass(name, "tag") {
				state = strings.ToLower(textOf(tag))
			}
			f.Gymnasts[who] = state
			f.Order = append(f.Order, who)
		}
	}
	return f
}

func byID(n *html.Node, id string) *html.Node {
	for d := range n.Descendants() {
		if d.Type == html.ElementNode && attrOf(d, "id") == id {
			return d
		}
	}
	return nil
}

func firstElementChild(n *html.Node) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode {
			return c
		}
	}
	return nil
}

func firstChildTag(n *html.Node, tag string) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == tag {
			return c
		}
	}
	return nil
}

// My competition.

// myPage is a person's My competition page.
type myPage struct {
	Name     string   // the heading: the person's name on the organiser's page of them
	Roles    []string // on the organiser's page: "Gymnast", "Coach"…
	Coaching []string // on the organiser's page: who they coach, "Mo · BUCS L3"
	Lateness []string // the lines saying an area is running late or early
	Desk     []string // messages from the organisers, newest first
	Events   []myEvent
	Waiting  bool // the timetable isn't published yet
}

// myEvent is one of their events.
type myEvent struct {
	Event, Status string
	Gymnasts      string // e.g. "Sam & Pat"
	Flight        string // e.g. "BUCS L3 · Panel 1 · Saturday", "" before publishing
	WarmUp        string
	Problems      []string
}

func myCompetitionOf(t *testing.T, page string) myPage {
	t.Helper()
	doc := parsePage(t, page)
	var out myPage
	if h1 := byTag(doc, "h1"); len(h1) > 0 {
		out.Name = textOf(h1[0])
	}
	if tags := firstByClass(doc, "tags"); tags != nil {
		for _, tag := range byClass(tags, "tag") {
			out.Roles = append(out.Roles, textOf(tag))
		}
	}
	for _, h2 := range byTag(doc, "h2") {
		if textOf(h2) == "Coaching" {
			for _, li := range byTag(h2.Parent, "li") {
				out.Coaching = append(out.Coaching, textOf(li))
			}
		}
	}
	for _, n := range byClass(doc, "notification") {
		switch {
		case hasClass(n, "is-warning") && strings.Contains(textOf(n), " is running about "):
			out.Lateness = append(out.Lateness, textOf(n))
		case strings.HasPrefix(textOf(n), "The timetable isn't published yet"):
			out.Waiting = true
		}
	}
	if box := firstByClass(doc, "comp-desk-messages"); box != nil {
		for _, m := range byClass(box, "mb-2") {
			if m.Data == "div" {
				out.Desk = append(out.Desk, textOf(byTag(m, "p")[0]))
			}
		}
	}
	for _, box := range byClass(doc, "comp-my-event") {
		ps := byTag(box, "p")
		ev := myEvent{Event: textOf(byTag(ps[0], "strong")[0]), Gymnasts: textOf(ps[1]), Status: textOf(ps[2])}
		if fl := firstByClass(box, "comp-my-flight"); fl != nil {
			fps := byTag(fl, "p")
			ev.Flight = textOf(fps[0])
			ev.WarmUp = textOf(byTag(fps[1], "strong")[0])
		}
		for _, ul := range byTag(box, "ul") {
			for _, li := range byTag(ul, "li") {
				ev.Problems = append(ev.Problems, textOf(li))
			}
		}
		out.Events = append(out.Events, ev)
	}
	return out
}

// tableRows are the cells of each row in the page's table bodies.
func tableRows(t *testing.T, page string) [][]string {
	t.Helper()
	var out [][]string
	for _, body := range byTag(parsePage(t, page), "tbody") {
		for _, tr := range byTag(body, "tr") {
			var row []string
			for _, td := range byTag(tr, "td") {
				row = append(row, textOf(td))
			}
			out = append(out, row)
		}
	}
	return out
}

// historyHas says whether the history has a row by who, saying what.
func historyHas(t *testing.T, h http.Handler, admin, who, what string) bool {
	t.Helper()
	for _, row := range tableRows(t, do(t, h, http.MethodGet, admin+"/history", nil).Body.String()) {
		if len(row) == 3 && row[1] == who && strings.HasPrefix(row[2], what) {
			return true
		}
	}
	return false
}

// Fees.

// feeAccount is a club's or individual's row on the organiser's Fees page.
type feeAccount struct {
	Payer              string // what the Paid form posts
	Entries            string
	Due, Paid, Balance string // Balance is "Paid" once settled
	Payments           []string
	Invoice            string
}

// feeAccounts are the Fees page's rows, by club or individual.
func feeAccounts(t *testing.T, page string) map[string]feeAccount {
	t.Helper()
	out := map[string]feeAccount{}
	for _, body := range byTag(parsePage(t, page), "tbody") {
		for _, tr := range byTag(body, "tr") {
			tds := byTag(tr, "td")
			if len(tds) != 6 {
				continue
			}
			name := strings.TrimSpace(tds[0].FirstChild.Data)
			a := feeAccount{Entries: textOf(tds[1]), Due: textOf(tds[2]), Paid: textOf(tds[3]), Balance: textOf(tds[4])}
			for _, d := range byTag(tds[0], "div") {
				a.Payments = append(a.Payments, strings.TrimSuffix(textOf(d), " remove"))
			}
			for _, in := range byTag(tds[5], "input") {
				if attrOf(in, "name") == "payer" {
					a.Payer = attrOf(in, "value")
				}
			}
			if links := byTag(tds[5], "a"); len(links) > 0 {
				a.Invoice = unescape(attrOf(links[0], "href"))
			}
			out[name] = a
		}
	}
	return out
}

// invoice is an invoice's lines ("what: amount") and its last line, what's
// left to pay ("To pay €5" or "Paid in full €0").
type invoice struct {
	Lines []string
	ToPay string
}

func invoiceOf(t *testing.T, page string) invoice {
	t.Helper()
	var out invoice
	doc := parsePage(t, page)
	for _, body := range byTag(doc, "tbody") {
		for _, tr := range byTag(body, "tr") {
			if tds := byTag(tr, "td"); len(tds) == 2 {
				out.Lines = append(out.Lines, textOf(tds[0])+": "+textOf(tds[1]))
			}
		}
	}
	if foots := byTag(doc, "tfoot"); len(foots) > 0 {
		rows := byTag(foots[0], "tr")
		out.ToPay = textOf(rows[len(rows)-1])
	}
	return out
}

// deskPeople are the people the desk can message one by one, with what
// it says they are ("official", "gymnast"…), by name.
func deskPeople(t *testing.T, page string) map[string]string {
	t.Helper()
	out := map[string]string{}
	box := firstByClass(parsePage(t, page), "comp-desk-people")
	if box == nil {
		return out
	}
	for _, label := range byTag(box, "label") {
		name, detail := "", ""
		for c := label.FirstChild; c != nil; c = c.NextSibling {
			switch {
			case c.Type == html.TextNode && name == "":
				name = strings.TrimSpace(c.Data)
			case c.Type == html.ElementNode && c.Data == "span":
				detail = strings.TrimPrefix(textOf(c), "· ")
			}
		}
		out[name] = detail
	}
	return out
}

// rowOf is the cells of the first table row whose cells start with name,
// failing if there's none.
func rowOf(t *testing.T, page, name string) []string {
	t.Helper()
	for _, row := range tableRows(t, page) {
		if slices.ContainsFunc(row, func(c string) bool { return c == name || strings.HasPrefix(c, name+" ") }) {
			return row
		}
	}
	t.Fatalf("no row for %s", name)
	return nil
}

// tapUp taps a running flight's Next gymnast or Back (what) as the marshal
// would: with the position the day page shows for that flight, so a repeat
// isn't taken for a second tap. It returns where the tap redirected.
func tapUp(t *testing.T, h http.Handler, admin, flight, what string) string {
	t.Helper()
	page := do(t, h, http.MethodGet, admin+"/day?day=0", nil).Body.String()
	at := ""
	for _, form := range regexp.MustCompile(`(?s)<form[^>]*/day/up"[^>]*>.*?</form>`).FindAllString(page, -1) {
		if strings.Contains(form, `name="flight" value="`+html.EscapeString(flight)+`"`) {
			if m := regexp.MustCompile(`name="at" value="(\d*)"`).FindStringSubmatch(form); m != nil {
				at = m[1]
			}
			break
		}
	}
	return redirected(t, h, admin+"/day/up", url.Values{"flight": {flight}, "what": {what}, "day": {"0"}, "at": {at}})
}
