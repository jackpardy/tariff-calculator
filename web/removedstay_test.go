package web

import (
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// memberIDs are the ids of a club's members, by name, from its Members page.
func memberIDs(t *testing.T, h http.Handler, club string) map[string]string {
	t.Helper()
	page := do(t, h, http.MethodGet, club+"/members", nil).Body.String()
	out := map[string]string{}
	for _, m := range regexp.MustCompile(`id="member-([^"]+)"[^>]*>\s*<td>\s*([^<\s]+)`).FindAllStringSubmatch(page, -1) {
		out[m[2]] = m[1]
	}
	return out
}

// removedSetup is a competition approving coaches with UCD's X and Y
// entered, both coached and signed off by Ann (approved), and sent.
func removedSetup(t *testing.T, h http.Handler) (admin, club, ann, annID string, members map[string]string) {
	t.Helper()
	form := newCompetition()
	form.Set("signoff", "1")
	admin = created(t, h, form)
	redirected(t, h, admin+"/approve-coaches", url.Values{"on": {"1"}, "level-trampoline": {"3"}})
	c := aClub(t, h, admin, "UCD", false, "X", "Y")
	club, members = c.page, c.members
	ann = pathIn(t, do(t, h, http.MethodPost, club+"/coaches", url.Values{"name": {"Ann Byrne"}}).Body.String(), "/clubs/coach/")
	annID = regexp.MustCompile(regexp.QuoteMeta(club) + `/coaches/([^/"]+)/signs-off`).FindStringSubmatch(do(t, h, http.MethodGet, club+"/coaches", nil).Body.String())[1]
	upload(t, h, club+"/coaches/"+annID+"/qualifications", map[string]string{"qualification": "bg-trampoline-3"}, "certificate", []byte("%PDF-1.4 Ann"))
	for _, m := range []string{"X", "Y"} {
		redirected(t, h, members[m]+"/coach", url.Values{"coach": {annID}})
		redirected(t, h, formAction(t, memberCompetitionPage(t, h, members[m]), members[m]+"/competitions/"), bucsEntry(""))
	}
	comp := regexp.MustCompile(regexp.QuoteMeta(club) + `/competitions/([^/"]+)/coaches`).FindStringSubmatch(clubCompetitionTab(t, h, club, "/coaches"))
	redirected(t, h, club+"/competitions/"+comp[1]+"/coaches", url.Values{"coach": {annID}})
	redirected(t, h, admin+"/coaches/"+annID, url.Values{"action": {"approve"}})
	home := do(t, h, http.MethodGet, ann, nil).Body.String()
	opens := regexp.MustCompile(`href="(`+regexp.QuoteMeta(ann)+`/members/[^"]+)"`).FindAllStringSubmatch(home, -1)
	if len(opens) != 2 {
		t.Fatalf("Ann sees X and Y: %d", len(opens))
	}
	for _, open := range opens {
		redirected(t, h, html.UnescapeString(open[1]), url.Values{"signed": {"1"}})
	}
	redirected(t, h, formAction(t, clubEntriesPage(t, h, club), club+"/competitions/"), url.Values{"which": {"all"}})
	return admin, club, ann, annID, members
}

func TestRemovedStayVisible(t *testing.T) {
	h := competitionServer(t)
	admin, club, _, annID, members := removedSetup(t, h)
	ids := memberIDs(t, h, club)
	if len(ids) != 2 {
		t.Fatalf("members: %v", ids)
	}
	redirected(t, h, admin+"/timetable/plan", url.Values{})
	if tt := do(t, h, http.MethodGet, admin+"/timetable", nil).Body.String(); !regexp.MustCompile(`>X<`).MatchString(tt) {
		t.Fatal("X is planned")
	}

	// The club removes X: asked first, saying the entry stays with the organiser.
	rec := do(t, h, http.MethodPost, club+"/members/"+ids["X"]+"/remove", nil)
	if body := rec.Body.String(); !strings.Contains(body, "marked removed by the club") || strings.Contains(body, "the organiser charges") {
		t.Errorf("the ask says what happens, and costs nothing before the close: %s", body)
	}
	redirected(t, h, club+"/members/"+ids["X"]+"/remove", url.Values{"anyway": {"1"}})

	// The organiser's entries: listed, marked, not counted.
	dash := do(t, h, http.MethodGet, admin, nil).Body.String()
	if !strings.Contains(dash, "Removed by the club") || !strings.Contains(dash, `removed by the club</p>`) {
		t.Errorf("the entry is listed, marked, and counted apart")
	}
	if strings.Contains(dash, "after entries closed</a>") {
		t.Error("before the close, nothing needs attention")
	}
	if !historyHas(t, h, admin, "UCD (Club admin)", "X · BUCS L3: removed by the club") {
		t.Error("the history says so")
	}
	// Their page, from People.
	people := do(t, h, http.MethodGet, admin+"/people", nil).Body.String()
	person := regexp.MustCompile(`href="(` + regexp.QuoteMeta(admin) + `/people/[^"]+)">X</a>`).FindStringSubmatch(people)
	if person == nil || !strings.Contains(people, "BUCS L3 · removed by the club") {
		t.Fatalf("X is on People: %s", people)
	}
	if page := do(t, h, http.MethodGet, html.UnescapeString(person[1]), nil); page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "BUCS L3: removed by the club on") {
		t.Errorf("their page says what the club removed: %d", page.Code)
	}
	// Not printed, nor in the timetable's flights.
	if cards := do(t, h, http.MethodGet, admin+"/cards", nil).Body.String(); strings.Contains(cards, ">X<") {
		t.Error("not printed")
	}
	if tt := do(t, h, http.MethodGet, admin+"/timetable", nil).Body.String(); !strings.Contains(tt, "Y") || regexp.MustCompile(`>X<`).MatchString(tt) {
		t.Error("the timetable loses X, as it does a withdrawal")
	}

	// The club removes Ann: she stays on the Coaches page, her sign-off counts.
	redirected(t, h, club+"/coaches/"+annID+"/remove", url.Values{"anyway": {"1"}})
	coaches := do(t, h, http.MethodGet, admin+"/coaches", nil).Body.String()
	if !strings.Contains(coaches, "Ann Byrne") || !strings.Contains(coaches, "Removed by the club") || !strings.Contains(coaches, "Their certificates went") ||
		!strings.Contains(coaches, "British Gymnastics Level 3 Trampoline Coach") || !strings.Contains(coaches, "Ask for these to be signed off again") {
		t.Fatalf("Ann's row: %s", coaches)
	}
	if strings.Contains(do(t, h, http.MethodGet, admin, nil).Body.String(), "Not signed off by a coach") {
		t.Error("Y's sign-off still counts")
	}

	// A link that can't decide about coaches can't ask either.
	cards := madeLink(t, h, admin, "Judges", "cards")
	if rec := do(t, h, http.MethodPost, cards+"/coaches/"+annID+"/signoff-again", url.Values{"anyway": {"1"}}); rec.Code != http.StatusForbidden {
		t.Errorf("a cards link: %d", rec.Code)
	}

	// Asked first, naming Y's entry; then it waits again, with the reason.
	rec = do(t, h, http.MethodPost, admin+"/coaches/"+annID+"/signoff-again", nil)
	if body := html.UnescapeString(rec.Body.String()); !strings.Contains(body, "1 entry goes back to waiting for a coach") || !strings.Contains(body, "Y · BUCS L3") {
		t.Fatalf("the ask: %s", body)
	}
	to := redirected(t, h, admin+"/coaches/"+annID+"/signoff-again", url.Values{"anyway": {"1"}})
	if noticeOf(t, to) != "Asked for 1 entry signed off by Ann Byrne to be signed off again." {
		t.Errorf("notice: %s", to)
	}
	reason := "Signed off by Ann Byrne, who is no longer one of UCD's coaches: the organiser has asked for another coach to sign it off."
	dash = do(t, h, http.MethodGet, admin, nil).Body.String()
	y := regexp.MustCompile(`href="(` + regexp.QuoteMeta(admin) + `/entries/[^"?#]+)">Y</a>`).FindStringSubmatch(dash)
	if y == nil {
		t.Fatal("Y's entry on the dashboard")
	}
	if page := html.UnescapeString(do(t, h, http.MethodGet, y[1], nil).Body.String()); !strings.Contains(page, "Not signed off by a coach") || !strings.Contains(page, reason) {
		t.Error("the organiser sees it waiting, and why")
	}
	if page := html.UnescapeString(clubEntriesPage(t, h, club)); !strings.Contains(page, reason) {
		t.Errorf("the club sees why: %s", page)
	}
	if page := html.UnescapeString(memberCompetitionPage(t, h, members["Y"])); !strings.Contains(page, reason) {
		t.Error("Y sees why")
	}
	bob := pathIn(t, do(t, h, http.MethodPost, club+"/coaches", url.Values{"name": {"Bob"}}).Body.String(), "/clubs/coach/")
	if page := html.UnescapeString(do(t, h, http.MethodGet, bob, nil).Body.String()); !strings.Contains(page, reason) {
		t.Error("another coach sees why")
	}
	if !historyHas(t, h, admin, "Organiser", "Asked for the entries Ann Byrne signed off to be signed off again") {
		t.Error("the history says so")
	}
	if coaches := do(t, h, http.MethodGet, admin+"/coaches", nil).Body.String(); strings.Contains(coaches, "Ask for these to be signed off again") || !strings.Contains(coaches, "You asked for their entries to be signed off again") {
		t.Error("nothing left to ask")
	}
	if to := redirected(t, h, admin+"/coaches/"+annID+"/signoff-again", url.Values{"anyway": {"1"}}); !strings.Contains(noticeOf(t, to), "Nothing was changed") {
		t.Errorf("asking again is turned down: %s", to)
	}
}

func TestRemovedAfterCloseFees(t *testing.T) {
	h, clock := competitionServerAt(t, dayBefore)
	admin := created(t, h, newCompetition())
	c := aClub(t, h, admin, "UCD", true, "X", "Y")
	redirected(t, h, admin+"/fees/settings", url.Values{"fee-": {"20"}, "charge_removed": {"1"}, "removed_fee": {"5"}})
	ids := memberIDs(t, h, c.page)

	// Before the close, removing costs nothing.
	if body := do(t, h, http.MethodPost, c.page+"/members/"+ids["Y"]+"/remove", nil).Body.String(); strings.Contains(body, "charges") {
		t.Error("before the close the ask names no charge")
	}
	redirected(t, h, c.page+"/members/"+ids["Y"]+"/remove", url.Values{"anyway": {"1"}})
	if a := feeAccounts(t, do(t, h, http.MethodGet, admin+"/fees", nil).Body.String())["UCD"]; a.Due != "€20" {
		t.Errorf("Y, removed before the close, isn't charged: %+v", a)
	}

	// After it, the ask says what it costs, and the invoice charges it.
	clock.set(onTheDay(8, 0))
	body := html.UnescapeString(do(t, h, http.MethodPost, c.page+"/members/"+ids["X"]+"/remove", nil).Body.String())
	if !strings.Contains(body, "Their entry at Student Open was sent and entries have closed: the organiser charges €25 for it.") {
		t.Errorf("the ask names the charge: %s", body)
	}
	redirected(t, h, c.page+"/members/"+ids["X"]+"/remove", url.Values{"anyway": {"1"}})
	fees := do(t, h, http.MethodGet, admin+"/fees", nil).Body.String()
	a := feeAccounts(t, fees)["UCD"]
	if a.Due != "€25" {
		t.Fatalf("UCD owes €25: %+v", a)
	}
	inv := invoiceOf(t, do(t, h, http.MethodGet, a.Invoice, nil).Body.String())
	want := []string{"X · BUCS L3 (removed after entries closed): €20", "Removed after entries closed: X · BUCS L3: €5"}
	if strings.Join(inv.Lines, "|") != strings.Join(want, "|") || inv.ToPay != "To pay €25" {
		t.Errorf("the invoice: %+v", inv)
	}
	if dash := do(t, h, http.MethodGet, admin, nil).Body.String(); !strings.Contains(dash, "entry removed by its club") {
		t.Error("the dashboard says one was removed after entries closed")
	}

	// Off, nothing is charged for it.
	redirected(t, h, admin+"/fees/settings", url.Values{"fee-": {"20"}})
	if a, ok := feeAccounts(t, do(t, h, http.MethodGet, admin+"/fees", nil).Body.String())["UCD"]; ok && a.Due != "€0" {
		t.Errorf("off: %+v", a)
	}
}

// Asking for an entry to be signed off again tells those who hear about
// its card.
func TestSignoffAgainTold(t *testing.T) {
	was := notifyState{Entries: map[string]entryState{"e": {Gymnasts: "Y", Event: "BUCS L3", Keys: []string{"m:y"}}}}
	now := notifyState{Entries: map[string]entryState{"e": {Gymnasts: "Y", Event: "BUCS L3", Keys: []string{"m:y"}, Again: "Signed off by Ann, who is no longer one of UCD's coaches: the organiser has asked for another coach to sign it off"}}}
	got := changes(was, now)
	if len(got) != 1 || got[0].Line != "BUCS L3: needs another coach's sign-off. Signed off by Ann, who is no longer one of UCD's coaches: the organiser has asked for another coach to sign it off" {
		t.Errorf("told: %+v", got)
	}
}
