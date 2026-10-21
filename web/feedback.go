package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"tariffCalculator/store"
	"tariffCalculator/views"
)

// Feedback about the app (migration 47): a Feedback button on every page,
// the calculator's and every link's, for anyone to tell the maintainer what
// confused them. A link's page posts to its own link's feedback route, so
// it's saved with their role and the competition it's about; any other page
// posts to /feedback. It keeps the page as its route pattern (never the
// path, which holds the link) and details from the browser, and as the
// maintainer chooses (Full or Light) a picture of the screen and a copy of
// the page. The maintainer reads it on their own page (maintainerpages.go),
// and FEEDBACK_EMAIL, with email set up, sends each one to them too.

// pageNames are what the maintainer's page calls the pages people send
// feedback from, by route pattern; a page without one shows its pattern.
var pageNames = map[string]string{
	"/":                                    "Routine builder",
	"/compare":                             "Compare routines",
	"/view":                                "Viewing a routine",
	"/tariff-sheet":                        "Tariff sheet",
	"/requirements":                        "Requirements",
	"/competitions":                        "Competitions",
	"/competitions/new":                    "Making a competition",
	"/clubs/new":                           "Making a club",
	"/clubs/join/{token}":                  "Joining a club",
	adminPrefix:                            "Entries",
	adminPrefix + "/entries/{id}":          "An entry",
	adminPrefix + "/people":                "People",
	adminPrefix + "/people/{key}":          "A person",
	adminPrefix + "/late":                  "Late changes",
	adminPrefix + "/coaches":               "Coaches",
	adminPrefix + "/fees":                  "Fees",
	adminPrefix + "/timetable":             "Timetable",
	adminPrefix + "/officials":             "Officials",
	adminPrefix + "/timetable/simulate":    "Simulate",
	adminPrefix + "/timetable/delay":       "What if there's a delay",
	adminPrefix + "/timetable/leave":       "What if an official leaves",
	adminPrefix + "/day":                   "On the day",
	adminPrefix + "/desk":                  "Desk messages",
	adminPrefix + "/settings":              "Settings",
	adminPrefix + "/history":               "History",
	"/clubs/admin/{token}":                 "Club home",
	"/clubs/admin/{token}/members":         "Club members",
	"/clubs/admin/{token}/coaches":         "Club coaches",
	"/clubs/admin/{token}/settings":        "Club settings",
	"/clubs/member/{token}":                "Member home",
	"/clubs/member/{token}/routines":       "Their routines",
	"/clubs/coach/{token}":                 "Coach home",
	"/clubs/coach/{token}/gymnasts":        "A coach's gymnasts",
	"/competitions/club/{token}":           "Entering a club",
	"/competitions/enter/{token}":          "Entering on their own",
	"/competitions/entry/{token}":          "Their entry",
	"/competitions/entry/{token}/day":      "My competition",
	"/competitions/entry/{token}/late":     "A late change",
	"/competitions/entry/{token}/notify":   "Hearing about changes",
	"/competitions/gymnast/{token}":        "Gymnast home",
	"/competitions/partner/{token}":        "Confirming a synchro partner",
	"/competitions/signoff/{token}":        "Signing off an entry",
	"/competitions/signoff/{token}/late":   "Signing off a late change",
	"/competitions/entry/{token}/timeline": "Their timetable",
	"/clubs/admin/{token}/members/{member}/competitions/{id}":       "A member's entry",
	"/clubs/admin/{token}/members/{member}/competitions/{id}/offer": "A member's offer to help",
	"/clubs/admin/{token}/members/{member}/competitions/{id}/late":  "A late change",
	"/clubs/admin/{token}/competitions/{id}":                        "The club's entries",
	"/clubs/admin/{token}/competitions/{id}/notify":                 "Hearing about changes",
	"/clubs/admin/{token}/competitions/{id}/timeline":               "The club's timetable",
	"/clubs/member/{token}/competitions/{id}":                       "Their entries",
	"/clubs/member/{token}/competitions/{id}/day":                   "My competition",
	"/clubs/member/{token}/competitions/{id}/late":                  "A late change",
	"/clubs/member/{token}/competitions/{id}/notify":                "Hearing about changes",
	"/clubs/member/{token}/competitions/{id}/timeline":              "Their timetable",
	"/clubs/member/{token}/competitions/{id}/club-timeline":         "The club's timetable",
	"/clubs/coach/{token}/members/{member}/competitions/{id}":       "Signing off an entry",
	"/clubs/coach/{token}/competitions/{id}/notify":                 "Hearing about changes",
	"/clubs/coach/{token}/competitions/{id}/timeline":               "The club's timetable",
	"/competitions/gymnast/{token}/competitions/{id}":               "Their entries",
	"/competitions/gymnast/{token}/competitions/{id}/enter":         "Entering on their own",
}

// pageName is what the maintainer's page calls a page.
func pageName(pattern string) string {
	if pattern == "" {
		return "Page not known"
	}
	if name, ok := pageNames[pattern]; ok {
		return name
	}
	return pattern
}

// calculatorPages are the calculator's pages with the button, by route
// pattern: they post to /feedback, as "Calculator".
var calculatorPages = []string{"/", "/compare", "/view", "/tariff-sheet", "/requirements"}

// feedbackRoots are the links feedback is posted under, each with the role
// it's saved with (an admin link's is the link's own).
var feedbackRoots = map[string]string{
	adminPrefix:                     "",
	"/clubs/admin/{token}":          "Club admin", // or the link's person and role (ADR 0015)
	"/competitions/club/{token}":    "Comp sec",
	"/clubs/member/{token}":         "Member",
	"/clubs/coach/{token}":          "Coach",
	"/competitions/enter/{token}":   "Individual",
	"/competitions/entry/{token}":   "Individual",
	"/competitions/gymnast/{token}": "Individual",
	"/competitions/partner/{token}": "Synchro partner",
	"/competitions/signoff/{token}": "An individual's coach",
}

// Feedback's limits: what's said, the name or email to reply to, and what
// comes with it.
const (
	maxFeedbackText     = 2000
	maxFeedbackReply    = 200
	maxFeedbackDetails  = 16 << 10
	maxFeedbackPicture  = 2 << 20
	maxFeedbackSnapshot = 3 << 20  // as sent, gzipped or not
	maxSnapshotHTML     = 12 << 20 // gunzipped
	// maxFeedbackBytes bounds a request sending feedback: the picture, the
	// copy of the page and the form around them.
	maxFeedbackBytes = maxFeedbackPicture + maxFeedbackSnapshot + 64<<10
	// maxFeedbackPerHour is how much feedback one address can send from the
	// pages without a link in an hour; a whole gym on one wifi shares one.
	maxFeedbackPerHour = 30
	// keepCaptures is how long pictures and copies of pages are kept.
	keepCaptures = 90 * 24 * time.Hour
)

// sendsFeedback says whether a request posts feedback, for its size limit.
func sendsFeedback(path string) bool {
	return path == "/feedback" || strings.HasSuffix(path, "/feedback")
}

func (p *competitionPages) registerFeedback(handle func(string, http.HandlerFunc)) {
	for root := range feedbackRoots {
		handle("POST "+root+"/feedback", p.sendFeedback)
	}
	handle("POST /feedback", p.sendFeedback)
}

// feedbackSettings are the maintainer's choices, read once and kept until
// they change them.
func (p *competitionPages) feedbackSettings() store.FeedbackSettings {
	if s := p.siteFeedback.Load(); s != nil {
		return *s
	}
	s, err := p.st.SiteFeedbackSettings(context.Background())
	if err != nil {
		return store.FeedbackSettings{} // on, Full: as they start
	}
	p.siteFeedback.Store(&s)
	return s
}

// withFeedback gives a page its Feedback button, unless the maintainer has
// turned it off: posting to its link's feedback route, or to /feedback for a
// page without a link. Only pages drawn with the app's layout show it.
func (p *competitionPages) withFeedback(r *http.Request) *http.Request {
	if r.Method != http.MethodGet || p.st == nil {
		return r
	}
	method, pattern, _ := strings.Cut(r.Pattern, " ")
	pattern = strings.TrimSuffix(pattern, "{$}") // the calculator's home, "/{$}", is "/"
	if method != http.MethodGet || !p.feedbackPage(pattern) {
		return r
	}
	s := p.feedbackSettings()
	if s.Off {
		return r
	}
	action := "/feedback"
	if root := feedbackRootOf(pattern); root != "" {
		parts := strings.SplitN(r.URL.Path, "/", 5) // "", the section, the kind of link, its token, the rest
		if len(parts) < 4 {
			return r
		}
		action = strings.Join(parts[:4], "/") + "/feedback"
	}
	q := r.URL.Query()
	q.Del("notice")
	back := r.URL.Path
	if len(q) > 0 {
		back += "?" + q.Encode()
	}
	f := &views.FeedbackForm{Action: action, Page: pattern, Back: back, Light: s.Light}
	return r.WithContext(views.WithFeedback(r.Context(), f))
}

// public gives a page of the calculator its Feedback button.
func (p *competitionPages) public(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h(w, p.withFeedback(r))
	}
}

// feedbackPage says whether a route pattern is a page feedback can be sent
// from: one of the calculator's, or a page under the competition routes but
// a printout (which ones draw the button is up to their views: not the
// venue screen, nor a download).
func (p *competitionPages) feedbackPage(pattern string) bool {
	if slices.Contains(calculatorPages, pattern) {
		return true
	}
	return p.pages[pattern] && !printed(pattern)
}

// printed says whether a page is for printing (the cards, invoices, the
// timetable's sheets), which has no button.
func printed(pattern string) bool {
	for _, end := range []string{"/cards", "/invoice", "/timetable/print", "/score-sheets"} {
		if strings.HasSuffix(pattern, end) {
			return true
		}
	}
	return false
}

// feedbackRootOf is the link a page's pattern is under, "" for none.
func feedbackRootOf(pattern string) string {
	for root := range feedbackRoots {
		if pattern == root || strings.HasPrefix(pattern, root+"/") {
			return root
		}
	}
	return ""
}

// feedbackAbout marks the competition a page is about, kept with feedback
// sent from it.
func feedbackAbout(r *http.Request, c store.Competition) {
	if f := views.FeedbackFormOf(r.Context()); f != nil {
		f.Competition = c.ID
	}
}

// feedbackFrom is who feedback is from, by the link it's posted under (not
// ok, having answered, if the link isn't one), the competition it's about if
// the link can see it, and the link's token, never kept.
func (p *competitionPages) feedbackFrom(w http.ResponseWriter, r *http.Request) (role string, c *store.Competition, token string, ok bool) {
	ctx, id := r.Context(), r.FormValue("competition")
	_, pattern, _ := strings.Cut(r.Pattern, " ")
	root := strings.TrimSuffix(pattern, "/feedback")
	if root == "" {
		page := r.FormValue("page")
		if slices.Contains(calculatorPages, page) {
			return "Calculator", nil, "", true
		}
		return "Without a link", nil, "", true
	}
	role, token = feedbackRoots[root], r.PathValue("token")
	seen := func(found store.Competition, ok bool) (string, *store.Competition, string, bool) {
		if !ok {
			return role, nil, token, false
		}
		return role, &found, token, true
	}
	entered := func(clubID string) bool {
		in, err := p.st.ClubEntered(ctx, clubID, id)
		return err == nil && in
	}
	competition := func(see bool) (string, *store.Competition, string, bool) {
		if id == "" || !see {
			return role, nil, token, true
		}
		found, err := p.st.Competition(ctx, id)
		if err != nil {
			return role, nil, token, true
		}
		return role, &found, token, true
	}
	switch root {
	case adminPrefix:
		found, ok := p.admin(w, r)
		if l := linkOf(r); l.Kind == store.LinkOrganiser {
			role = "Organiser"
		} else {
			role = "Helper link: " + l.Name
			if !strings.EqualFold(l.Name, linkKinds[l.Kind]) {
				role += " (" + linkKinds[l.Kind] + ")"
			}
		}
		return seen(found, ok)
	case "/competitions/club/{token}":
		return seen(p.clubLinkCompetition(w, r))
	case "/competitions/enter/{token}":
		return seen(p.individual(w, r))
	case "/competitions/entry/{token}":
		_, found, ok := p.own(w, r)
		return seen(found, ok)
	case "/competitions/partner/{token}":
		_, found, ok := p.invite(w, r)
		return seen(found, ok)
	case "/competitions/signoff/{token}":
		_, found, ok := p.signoffEntry(w, r)
		return seen(found, ok)
	case "/clubs/admin/{token}":
		club, found := p.clubAdmin(w, r)
		if !found {
			return role, nil, token, false
		}
		// Who of the club, and which club: "Mary (Treasurer), UCD" (ADR 0015).
		role = clubLinkOf(r).Who() + ", " + club.Name
		return competition(id != "" && entered(club.ID))
	case "/clubs/member/{token}":
		m, found := p.member(w, r)
		if !found {
			return role, nil, token, false
		}
		return competition(id != "" && entered(m.ClubID))
	case "/clubs/coach/{token}":
		coach, _, found := p.coach(w, r)
		if !found {
			return role, nil, token, false
		}
		see := id != "" && entered(coach.ClubID)
		if id != "" && !see {
			// A synchro partner's coach sees the pair's entry from another club.
			entries, err := p.st.CoachEntries(ctx, coach)
			see = err == nil && slices.ContainsFunc(entries, func(e store.MemberEntry) bool { return e.CompetitionID == id })
		}
		return competition(see)
	case "/competitions/gymnast/{token}":
		_, entries, found := p.gymnast(w, r)
		if !found {
			return role, nil, token, false
		}
		return competition(slices.ContainsFunc(entries, func(e store.Entry) bool { return e.CompetitionID == id }))
	}
	failed(w, r, store.ErrNotFound)
	return role, nil, token, false
}

// sendFeedback keeps feedback about the app (text, optional reply, the
// page's pattern, details and what was captured), emails it to
// FEEDBACK_EMAIL if set, and goes back to the page. The page's script sends
// it in the background (X-Feedback), and gets back what to say.
func (p *competitionPages) sendFeedback(w http.ResponseWriter, r *http.Request) {
	fetched := r.Header.Get("X-Feedback") != ""
	refuse := func(status int, why string) {
		if fetched {
			http.Error(w, why, status)
			return
		}
		message(w, r, status, "Feedback not sent", why)
	}
	if err := r.ParseMultipartForm(1 << 20); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		refuse(http.StatusRequestEntityTooLarge, "That's too much to send: try again without the picture.")
		return
	}
	settings := p.feedbackSettings()
	if settings.Off {
		refuse(http.StatusForbidden, "Feedback is turned off just now.")
		return
	}
	public := r.URL.Path == "/feedback"
	if public && !p.feedbackLimiter.allow(clientIP(r), p.now()) {
		refuse(http.StatusTooManyRequests, "That's a lot of feedback from here: please try again in an hour.")
		return
	}
	role, c, token, ok := p.feedbackFrom(w, r)
	if !ok {
		return
	}
	text := strings.TrimSpace(r.FormValue("text"))
	reply := strings.Join(strings.Fields(r.FormValue("reply")), " ") // one line: it may be an email header
	switch {
	case text == "":
		refuse(http.StatusUnprocessableEntity, "Say what happened, or what you think, then send it.")
		return
	case utf8.RuneCountInString(text) > maxFeedbackText:
		refuse(http.StatusUnprocessableEntity, fmt.Sprintf("That's too long to send: keep it under %d characters.", maxFeedbackText))
		return
	case utf8.RuneCountInString(reply) > maxFeedbackReply:
		refuse(http.StatusUnprocessableEntity, fmt.Sprintf("The name or email is too long: keep it under %d characters.", maxFeedbackReply))
		return
	}
	f := store.SiteFeedback{Text: text, ReplyTo: reply, Role: role, Page: r.FormValue("page")}
	if !p.feedbackPage(f.Page) {
		f.Page = "" // never anything but a known page's pattern
	}
	if c != nil {
		f.CompetitionID, f.Competition = c.ID, c.Name
	}
	details := p.feedbackDetails(r.FormValue("details"), token, settings.Light)
	capture := p.feedbackCapture(r, token, details, settings.Light)
	if b, err := json.Marshal(details); err == nil && string(b) != "{}" {
		f.Details = string(b)
	}
	f, err := p.st.AddSiteFeedback(r.Context(), f, capture)
	if errors.Is(err, store.ErrLimit) {
		refuse(http.StatusTooManyRequests, "The app has had a lot of feedback today: please try again tomorrow.")
		return
	}
	if err != nil {
		failed(w, r, err)
		return
	}
	p.emailFeedback(f, details)
	if fetched {
		fmt.Fprint(w, "Thanks: your feedback is sent.")
		return
	}
	http.Redirect(w, r, feedbackBack(r)+"#feedback-sent", http.StatusSeeOther)
}

// feedbackBack is the page to go back to without JavaScript: one of the same
// link's under a link, any page of the app's otherwise.
func feedbackBack(r *http.Request) string {
	back, _, _ := strings.Cut(r.FormValue("back"), "#")
	if r.URL.Path == "/feedback" {
		if !strings.HasPrefix(back, "/") || strings.HasPrefix(back, "//") || strings.Contains(back, `\`) {
			return "/"
		}
		return back
	}
	link := strings.TrimSuffix(r.URL.Path, "/feedback")
	if back != link && !strings.HasPrefix(back, link+"/") && !strings.HasPrefix(back, link+"?") {
		return link
	}
	return back
}

// feedbackDetails are what the page says about the browser it's in, at the
// time feedback is sent: kept to what's known, each part bounded, never a
// link (the sender's own is taken out, and pages are their patterns).
type feedbackDetails struct {
	Viewport  []int    `json:"viewport,omitempty"`  // the window's width and height
	Screen    []int    `json:"screen,omitempty"`    // the screen's
	Ratio     float64  `json:"ratio,omitempty"`     // device pixels to a CSS pixel
	Browser   string   `json:"browser,omitempty"`   // its user agent
	Language  string   `json:"language,omitempty"`  // e.g. "en-IE"
	Online    *bool    `json:"online,omitempty"`    // whether it thought it had signal
	Installed bool     `json:"installed,omitempty"` // opened from the home screen
	Version   string   `json:"version,omitempty"`   // the app's version the page came from
	PageTime  string   `json:"pageTime,omitempty"`  // when the page was made (a page kept offline is older)
	Scroll    []int    `json:"scroll,omitempty"`    // how far across and down they'd scrolled
	Height    int      `json:"height,omitempty"`    // the page's height
	Queued    int      `json:"queued,omitempty"`    // taps kept to send when there's signal
	Errors    []string `json:"errors,omitempty"`    // the page's script errors and failed requests
	Recent    []string `json:"recent,omitempty"`    // the pages opened before, in that tab
	Open      []string `json:"open,omitempty"`      // the dropdowns open (Full only)
	NoPicture string   `json:"noPicture,omitempty"` // why a picture couldn't be taken
}

// feedbackDetails reads the details a page sent.
func (p *competitionPages) feedbackDetails(raw, token string, light bool) feedbackDetails {
	var d feedbackDetails
	if raw == "" || len(raw) > maxFeedbackDetails {
		return d
	}
	if token != "" {
		raw = strings.ReplaceAll(raw, token, "{token}")
	}
	if json.Unmarshal([]byte(raw), &d) != nil {
		return feedbackDetails{}
	}
	clip := func(s string, n int) string {
		s = strings.ToValidUTF8(strings.TrimSpace(s), "")
		if r := []rune(s); len(r) > n {
			s = string(r[:n]) + "…"
		}
		return s
	}
	clips := func(list []string, n, each int, keep func(string) bool) []string {
		var out []string
		for _, s := range list {
			if s = clip(s, each); s != "" && (keep == nil || keep(s)) {
				out = append(out, s)
			}
		}
		if len(out) > n {
			out = out[len(out)-n:]
		}
		return out
	}
	ints := func(v []int) []int {
		if len(v) != 2 || v[0] < 0 || v[1] < 0 || v[0] > 1e6 || v[1] > 1e6 {
			return nil
		}
		return v
	}
	d.Viewport, d.Screen, d.Scroll = ints(d.Viewport), ints(d.Screen), ints(d.Scroll)
	if d.Ratio < 0 || d.Ratio > 10 {
		d.Ratio = 0
	}
	if d.Height < 0 || d.Height > 1e7 {
		d.Height = 0
	}
	if d.Queued < 0 || d.Queued > 1e4 {
		d.Queued = 0
	}
	d.Browser, d.Language, d.Version, d.PageTime = clip(d.Browser, 300), clip(d.Language, 40), clip(d.Version, 40), clip(d.PageTime, 40)
	d.NoPicture = clip(d.NoPicture, 200)
	d.Errors = clips(d.Errors, 10, 300, nil)
	d.Recent = clips(d.Recent, 6, 200, p.feedbackPage)
	d.Open = clips(d.Open, 10, 100, nil)
	if light {
		d.Open = nil
	}
	return d
}

// feedbackCapture reads what was captured with feedback: a picture of the
// screen (Full always, Light when they ticked for it) and, in Full, a copy
// of the page, cleaned to show safely (cleanSnapshot). A part that isn't
// what it should be is left off, noted in the details, never refusing the
// words.
func (p *competitionPages) feedbackCapture(r *http.Request, token string, d feedbackDetails, light bool) store.Capture {
	var c store.Capture
	if light && r.FormValue("with-picture") != "1" {
		return c
	}
	if b := formFile(r, "picture", maxFeedbackPicture); b != nil {
		if kind := http.DetectContentType(b); kind == "image/jpeg" || kind == "image/png" || kind == "image/webp" {
			c.Picture, c.PictureType = b, kind
		}
	}
	if light {
		return c
	}
	if b := formFile(r, "snapshot", maxFeedbackSnapshot); b != nil {
		scroll := 0
		if len(d.Scroll) == 2 {
			scroll = d.Scroll[1]
		}
		if snapshot, err := cleanSnapshot(b, token, scroll); err == nil {
			c.Snapshot = snapshot
		}
	}
	return c
}

// formFile is a file posted with a form, nil if there's none or it's over
// max bytes.
func formFile(r *http.Request, name string, max int64) []byte {
	if r.MultipartForm == nil {
		return nil
	}
	files := r.MultipartForm.File[name]
	if len(files) == 0 {
		return nil
	}
	return readPart(files[0], max)
}

func readPart(h *multipart.FileHeader, max int64) []byte {
	if h.Size > max {
		return nil
	}
	f, err := h.Open()
	if err != nil {
		return nil
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil || int64(len(b)) > max || len(b) == 0 {
		return nil
	}
	return b
}

// emailFeedback emails feedback to FEEDBACK_EMAIL, when it's set and email
// is, without waiting for it.
func (p *competitionPages) emailFeedback(f store.SiteFeedback, d feedbackDetails) {
	if p.feedbackTo == "" || p.notify == nil || p.notify.mail == nil {
		return
	}
	subject, body, headers := feedbackEmail(f, d)
	// The background sender, with the desk's messages and the
	// notifications: the request doesn't wait for it, and it keeps to the
	// cap on emails a second.
	p.notify.send.send([]delivery{{about: "Emailing feedback about the app",
		email: &email{to: p.feedbackTo, subject: subject, body: body, headers: headers}}})
}

// feedbackEmail is the email of a piece of feedback: what was said, who by
// (their role), on which page and when, replying to them if they gave an
// email. Never a link: they're secret.
func feedbackEmail(f store.SiteFeedback, d feedbackDetails) (subject, body string, headers map[string]string) {
	page := pageName(f.Page)
	if f.Page != "" && page != f.Page {
		page += " (" + f.Page + ")"
	}
	var b bytes.Buffer
	b.WriteString(f.Text + "\n\n")
	if f.Competition != "" {
		b.WriteString("Competition: " + f.Competition + "\n")
	}
	b.WriteString("From: " + f.Role + "\n")
	b.WriteString("Page: " + page + "\n")
	b.WriteString("At: " + f.At.In(local).Format("Mon 2 January 2006, 15:04") + "\n")
	if f.ReplyTo != "" {
		b.WriteString("Reply to: " + f.ReplyTo + "\n")
		if a, ok := emailAddress(f.ReplyTo); ok {
			headers = map[string]string{"Reply-To": a}
		}
	}
	if line := detailsLine(d); line != "" {
		b.WriteString("Browser: " + line + "\n")
	}
	switch {
	case f.Picture && f.Snapshot:
		b.WriteString("\nA picture of the screen and a copy of the page are on your feedback page.\n")
	case f.Picture:
		b.WriteString("\nA picture of the screen is on your feedback page.\n")
	}
	return "Feedback: " + pageName(f.Page), b.String(), headers
}

// detailsLine sums up the details in a line: the window's size, signal,
// opened from the home screen, taps waiting and errors.
func detailsLine(d feedbackDetails) string {
	var parts []string
	if len(d.Viewport) == 2 {
		parts = append(parts, fmt.Sprintf("%d × %d window", d.Viewport[0], d.Viewport[1]))
	}
	if d.Online != nil && !*d.Online {
		parts = append(parts, "offline")
	}
	if d.Installed {
		parts = append(parts, "opened from the home screen")
	}
	if d.Queued > 0 {
		parts = append(parts, views.Count(d.Queued, "tap", "taps")+" waiting for signal")
	}
	if len(d.Errors) > 0 {
		parts = append(parts, views.Count(len(d.Errors), "error", "errors"))
	}
	return strings.Join(parts, ", ")
}
