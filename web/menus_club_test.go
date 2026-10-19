package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"tariffCalculator/store"
	"tariffCalculator/views"
)

// currentTab is the label of the menu's current section in a page.
func currentTab(t *testing.T, page string) string {
	t.Helper()
	m := regexp.MustCompile(`<li class="is-active"><a href="[^"]*" aria-current>([^<]*?)\s*</a>`).FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("no current section in the menu: %s", page)
	}
	return m[1]
}

// The comp sec's page is in sections, each a page of its own with the menu on
// it, and what each section's forms do brings them back to that section.
func TestClubSections(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	ucd := aClub(t, h, admin, "UCD", false, "A. Murphy")
	club, member := ucd.page, ucd.members["A. Murphy"]
	_ = admin

	// Competitions is the default: the competitions, and not the other parts.
	page := do(t, h, http.MethodGet, club, nil).Body.String()
	if got := currentTab(t, page); got != "Competitions" {
		t.Errorf("the default section is %q", got)
	}
	for _, want := range []string{"Student Open", `href="` + club + `/competitions/`, `href="` + club + `/members"`, `href="` + club + `/coaches"`, `href="` + club + `/settings"`} {
		if !strings.Contains(page, want) {
			t.Errorf("Competitions should have %q", want)
		}
	}
	for _, not := range []string{"Join link", "Add coach", "Replace the admin link"} {
		if strings.Contains(page, not) {
			t.Errorf("Competitions shouldn't have %q", not)
		}
	}

	// Members: the join link, the members, a coach to choose for each.
	do(t, h, http.MethodPost, club+"/coaches", url.Values{"name": {"Bob"}})
	page = do(t, h, http.MethodGet, club+"/members", nil).Body.String()
	if got := currentTab(t, page); got != "Members" {
		t.Errorf("the members section is %q", got)
	}
	for _, want := range []string{"Join link", "A. Murphy", "0 entries", ">Bob</option>", ">Remove</button>"} {
		if !strings.Contains(page, want) {
			t.Errorf("Members should have %q", want)
		}
	}
	if strings.Contains(page, "Student Open") || strings.Contains(page, "Add coach") {
		t.Error("Members is only the members")
	}
	assign := formAction(t, page, club+"/members/")
	if !strings.HasSuffix(assign, "/coach") {
		assign = regexp.MustCompile(`action="(` + regexp.QuoteMeta(club) + `/members/[^"]+/coach)"`).FindStringSubmatch(page)[1]
	}
	if got := redirected(t, h, assign, url.Values{"coach": {""}}); !strings.HasPrefix(got, club+"/members?notice=A.+Murphy+has+no+coach") || !strings.Contains(got, "&at=") || !strings.Contains(got, "#member-") {
		t.Errorf("choosing a member's coach goes back to their row on Members: %s", got)
	}
	newLink := regexp.MustCompile(`action="(` + regexp.QuoteMeta(club) + `/members/[^"]+/new-link)"`).FindStringSubmatch(page)[1]
	if rec := do(t, h, http.MethodPost, newLink, url.Values{"anyway": {"1"}}); rec.Code != http.StatusOK || currentTab(t, rec.Body.String()) != "Members" || !strings.Contains(rec.Body.String(), "A. Murphy&#39;s new link") {
		t.Errorf("a member's new link is shown on Members: %d", rec.Code)
	}
	remove := strings.TrimSuffix(newLink, "/new-link") + "/remove"
	if got := withoutQuery(redirected(t, h, remove, url.Values{})); got != club+"/members" {
		t.Errorf("removing a member goes back to Members: %s", got)
	}
	_ = member

	// Coaches: each coach, and adding one.
	page = do(t, h, http.MethodGet, club+"/coaches", nil).Body.String()
	if got := currentTab(t, page); got != "Coaches" {
		t.Errorf("the coaches section is %q", got)
	}
	for _, want := range []string{"Bob", "Add coach", "Signs off routines", "Every coach sees every member", "Add a qualification"} {
		if !strings.Contains(page, want) {
			t.Errorf("Coaches should have %q", want)
		}
	}
	if strings.Contains(page, "Join link") {
		t.Error("Coaches is only the coaches")
	}
	rec := do(t, h, http.MethodPost, club+"/coaches", url.Values{"name": {"Cat"}})
	if rec.Code != http.StatusOK || currentTab(t, rec.Body.String()) != "Coaches" || !strings.Contains(rec.Body.String(), "Cat&#39;s coach link") {
		t.Errorf("a new coach's link is shown on Coaches: %d", rec.Code)
	}
	if got := withoutQuery(redirected(t, h, club+"/coaches", url.Values{"name": {" "}})); got != club+"/coaches" {
		t.Errorf("a coach without a name goes back to Coaches: %s", got)
	}
	coachID := regexp.MustCompile(regexp.QuoteMeta(club) + `/coaches/([^/"]+)/signs-off`).FindStringSubmatch(do(t, h, http.MethodGet, club+"/coaches", nil).Body.String())[1]
	for _, post := range []struct {
		path string
		form url.Values
	}{
		{club + "/coaches-see-all", url.Values{"on": {"1"}}},
		{club + "/coaches/" + coachID + "/signs-off", url.Values{"on": {""}}},
		{club + "/coaches/" + coachID + "/remove", url.Values{}},
	} {
		if got := withoutQuery(redirected(t, h, post.path, post.form)); got != club+"/coaches" {
			t.Errorf("POST %s goes back to Coaches: %s", post.path, got)
		}
	}

	// Settings: the two forms, as a page of their own.
	page = do(t, h, http.MethodGet, club+"/settings", nil).Body.String()
	if got := currentTab(t, page); got != "Settings" {
		t.Errorf("the settings section is %q", got)
	}
	if !strings.Contains(page, "Replace the admin link") || !strings.Contains(page, "Delete the club") || strings.Contains(page, "Add coach") {
		t.Error("Settings is the replace and delete forms")
	}
	if got := redirected(t, h, club+"/delete", url.Values{}); !strings.HasPrefix(got, club+"/settings?notice=") {
		t.Errorf("deleting without confirming goes back to Settings: %s", got)
	}
	// Replacing the admin link asks first, on Settings; then the new one
	// is shown on the default page, where it's saved.
	if rec := do(t, h, http.MethodPost, club+"/replace-link", url.Values{}); rec.Code != http.StatusOK || currentTab(t, rec.Body.String()) != "Settings" ||
		!strings.Contains(rec.Body.String(), "Replace the admin link?") || !strings.Contains(rec.Body.String(), `href="`+club+`/settings">Cancel`) {
		t.Errorf("replacing the admin link asks first: %d", rec.Code)
	}
	if got := redirected(t, h, club+"/replace-link", url.Values{"anyway": {"1"}}); !strings.HasSuffix(got, "?new=replaced") || strings.Contains(got, "/settings") {
		t.Errorf("replacing the admin link shows the new one on the default page: %s", got)
	}
}

// A competition's pages for the comp sec are in Competitions, each with the
// competition's tabs; the pages for a member's entry and offer are too, and
// saving them goes back to the competition's tab they came from.
func TestClubCompetitionsSectionPages(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	ucd := aClub(t, h, admin, "UCD", false, "A. Murphy")
	club, member := ucd.page, ucd.members["A. Murphy"]
	entry := bucsEntry("")
	redirected(t, h, formAction(t, memberCompetitionPage(t, h, member), member+"/competitions/"), entry)

	page := clubEntriesPage(t, h, club)
	compPage := regexp.MustCompile(`href="(` + regexp.QuoteMeta(club) + `/competitions/[^"/?]+)"`).FindStringSubmatch(do(t, h, http.MethodGet, club, nil).Body.String())[1]
	if currentTab(t, page) != "Competitions" || !strings.Contains(page, `href="`+compPage+`/offers"`) || strings.Contains(page, `href="`+compPage+`/fees"`) {
		t.Error("a competition's page is in Competitions, with its tabs (no Fees where it doesn't charge)")
	}
	if rec := do(t, h, http.MethodGet, compPage+"/fees", nil); rec.Code != http.StatusNotFound {
		t.Errorf("no Fees where it doesn't charge: %d", rec.Code)
	}
	edit := regexp.MustCompile(`href="(` + regexp.QuoteMeta(club) + `/members/[^"]+)"`).FindStringSubmatch(page)[1]
	editPage := do(t, h, http.MethodGet, edit, nil).Body.String()
	if currentTab(t, editPage) != "Competitions" || !strings.Contains(editPage, "← Student Open") {
		t.Error("changing a member's entry is in Competitions")
	}
	if got := withoutQuery(redirected(t, h, edit, entry)); got != compPage {
		t.Errorf("saving a member's entry goes back to the competition: %s", got)
	}
	send := formAction(t, page, club+"/competitions/")
	if got := withoutQuery(redirected(t, h, send, url.Values{"which": {"all"}})); got != compPage {
		t.Errorf("sending goes back to the competition: %s", got)
	}
	offer := withoutQuery(edit) + "/offer"
	if offerPage := do(t, h, http.MethodGet, offer, nil).Body.String(); currentTab(t, offerPage) != "Competitions" {
		t.Error("changing a member's offer is in Competitions")
	}
	if got := withoutQuery(redirected(t, h, offer, url.Values{"recorder": {"1"}})); got != compPage+"/offers" {
		t.Errorf("saving a member's offer goes back to Judging and helping: %s", got)
	}
}

// A new club's page says where to find the join link, and keeps saving its
// admin link in the browser.
func TestNewClubPage(t *testing.T) {
	h := competitionServer(t)
	location := redirected(t, h, "/clubs", url.Values{"name": {"UCD"}})
	page := do(t, h, http.MethodGet, location, nil).Body.String()
	if !strings.Contains(page, "data-club-admin=") || !strings.Contains(page, `href="`+withoutQuery(location)+`/members"`) {
		t.Error("the new club's page saves its admin link and points to Members")
	}
}

// A member's pages are tabs: the competitions their club is in, and their
// details; and a competition has a page of its own.
func TestMemberTabs(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	ucd := aClub(t, h, admin, "UCD", false, "A. Murphy")
	club, member := ucd.page, ucd.members["A. Murphy"]
	do(t, h, http.MethodPost, club+"/coaches", url.Values{"name": {"Bob"}})

	// The default page lists the competitions, and saves their link.
	list := do(t, h, http.MethodGet, member+"?new=joined", nil).Body.String()
	if got := currentTab(t, list); got != "Competitions" {
		t.Errorf("the default tab is %q", got)
	}
	if !strings.Contains(list, "You've joined.") || !strings.Contains(list, `data-member-link="http://example.com`+member+`"`) {
		t.Error("the default page welcomes them and saves their link in the browser")
	}
	row := regexp.MustCompile(`<a class="box comp-member-row" href="(` + regexp.QuoteMeta(member) + `/competitions/[^"/]+)"><strong>Student Open</strong>`).FindStringSubmatch(list)
	if row == nil || !strings.Contains(list, "Not entered") || !strings.Contains(list, "Saturday 16 March 2030") {
		t.Fatalf("the list has the competition, its date and status, linking to its page: %s", list)
	}
	if strings.Contains(list, "Your coach") || strings.Contains(list, "Change your entry") || strings.Contains(list, "<details") {
		t.Error("the list is only the list")
	}

	// Their details: the link and the coach, and the link is saved from here too.
	details := do(t, h, http.MethodGet, member+"/details", nil).Body.String()
	if got := currentTab(t, details); got != "Your details" {
		t.Errorf("the details tab is %q", got)
	}
	if !strings.Contains(details, "Your personal link") || !strings.Contains(details, "Your coach") || !strings.Contains(details, ">Bob</option>") || !strings.Contains(details, "data-member-link=") {
		t.Error("the details have the link and the coach")
	}
	if got := redirected(t, h, member+"/coach", url.Values{"coach": {""}}); !strings.HasPrefix(got, member+"/details?notice=") {
		t.Errorf("choosing a coach goes back to Your details: %s", got)
	}

	// The competition's page: its disciplines are rows, collapsed.
	compPath := row[1]
	page := do(t, h, http.MethodGet, compPath, nil).Body.String()
	if got := currentTab(t, page); got != "Competitions" {
		t.Errorf("a competition's page is in %q", got)
	}
	if !strings.Contains(page, `href="`+member+`">← Competitions</a>`) || !strings.Contains(page, "Student Open") || !strings.Contains(page, "Entries close") {
		t.Error("the competition's page has a way back and its facts")
	}
	if !strings.Contains(page, `<details class="box comp-event-row"><summary><h3 class="comp-event-name">Trampoline</h3>`+chip(t, views.NotYet, "Not entered")+`</summary>`) {
		t.Errorf("an unentered discipline is a collapsed row saying so: %s", regexp.MustCompile(`<details class="box comp-event-row".{0,300}`).FindString(page))
	}

	// Entering it: problems come back open, and as posted; the save is shown open.
	save := formAction(t, page, member+"/competitions/")
	entry := url.Values{"level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}}
	rec := do(t, h, http.MethodPost, save, entry)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `<details class="box comp-event-row" open>`) || !strings.Contains(rec.Body.String(), "Choose a routine for the second exercise.") {
		t.Errorf("an entry with problems is shown open on the competition's page: %d", rec.Code)
	}
	entry.Set("ex2Skills", voluntary)
	location := redirected(t, h, save, entry)
	if !strings.HasPrefix(location, compPath+"?notice=") || !strings.Contains(location, "discipline=") {
		t.Errorf("saving goes back to the competition's page: %s", location)
	}
	page = do(t, h, http.MethodGet, location, nil).Body.String()
	if !strings.Contains(page, `<details class="box comp-event-row" open>`) || !strings.Contains(page, "Entry saved for Student Open") {
		t.Error("the notice's entry is shown open")
	}
	page = do(t, h, http.MethodGet, compPath, nil).Body.String()
	if !strings.Contains(page, `<details class="box comp-event-row"><summary><h3 class="comp-event-name">Trampoline</h3><span class="has-text-grey is-size-7">· BUCS L3</span>`+chip(t, views.Attention, "2 problems")+`</summary>`) ||
		!strings.Contains(page, "Saved, not sent yet") {
		t.Errorf("an entered discipline is a collapsed row with its level and status, opening to its card: %s", regexp.MustCompile(`<details class="box comp-event-row".{0,300}`).FindString(page))
	}
	if list := do(t, h, http.MethodGet, member, nil).Body.String(); !strings.Contains(list, "Saved, not sent yet") {
		t.Error("the list shows the entry's status")
	}

	// What else is posted from the competition's page comes back to it.
	withdraw := formAction(t, page, member+"/competitions/")
	if !strings.HasSuffix(withdraw, "/withdraw") {
		withdraw = member + "/competitions/" + strings.Split(strings.TrimPrefix(compPath, member+"/competitions/"), "/")[0] + "/withdraw"
	}
	if got := redirected(t, h, withdraw, url.Values{"confirm": {"1"}, "discipline": {""}}); !strings.HasPrefix(got, compPath+"?notice=") {
		t.Errorf("withdrawing goes back to the competition's page: %s", got)
	}
	if got := redirected(t, h, withdraw, url.Values{"discipline": {""}}); got != compPath {
		t.Errorf("not confirming goes back to the competition's page: %s", got)
	}
	offer := compPath + "/offer"
	if got := redirected(t, h, offer, url.Values{"recorder": {"1"}}); !strings.HasPrefix(got, compPath+"?notice=") {
		t.Errorf("saving an offer goes back to the competition's page: %s", got)
	}
	if !strings.Contains(do(t, h, http.MethodGet, compPath, nil).Body.String(), "recorder") {
		t.Error("the offer is on the competition's page")
	}

	// The My competition page goes back to the competition's page.
	day := dayAll(t, h, compPath+"/day")
	if !strings.Contains(day, `<a href="`+compPath+`">← Student Open: your entries</a>`) {
		t.Error("the day page's back link goes to the competition's page")
	}
}

// A competition id that isn't the club's gives the same not-found as the other
// pages of the member's competition.
func TestMemberCompetitionPageNotFound(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	ucd := aClub(t, h, admin, "UCD", false, "A. Murphy")
	_, member := ucd.page, ucd.members["A. Murphy"]
	want := do(t, h, http.MethodGet, member+"/competitions/nope/day", nil)
	got := do(t, h, http.MethodGet, member+"/competitions/nope", nil)
	if got.Code != http.StatusNotFound || got.Code != want.Code || !strings.Contains(got.Body.String(), "Link not found") {
		t.Errorf("an unknown competition: %d, as the day page's %d", got.Code, want.Code)
	}
	if rec := do(t, h, http.MethodGet, "/clubs/member/nope/details", nil); rec.Code != http.StatusNotFound {
		t.Errorf("an unknown member's details: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, "/clubs/admin/nope/members", nil); rec.Code != http.StatusNotFound {
		t.Errorf("an unknown club's members: %d", rec.Code)
	}
}

// A discipline's status in a word or two, for its row, and its state chip.
func TestEntryBrief(t *testing.T) {
	sent := store.MemberEntry{SentAt: time.Now().Add(-time.Hour), UpdatedAt: time.Now().Add(-2 * time.Hour)}
	checked := sent
	checked.Checked = true
	changed := sent
	changed.UpdatedAt = time.Now()
	for _, c := range []struct {
		name     string
		e        store.MemberEntry
		removal  store.Entry
		removed  bool
		problems int
		place    int
		want     string
		state    views.State
	}{
		{"saved", store.MemberEntry{}, store.Entry{}, false, 0, 0, "Saved, not sent", views.NotYet},
		{"sent", sent, store.Entry{}, false, 0, 0, "Sent", views.InProgress},
		{"checked", checked, store.Entry{}, false, 0, 0, "Organiser checked", views.Done},
		{"changed", changed, store.Entry{}, false, 0, 0, "Changed, send again", views.Attention},
		{"one problem", checked, store.Entry{}, false, 1, 0, "1 problem", views.Attention},
		{"problems", checked, store.Entry{}, false, 2, 0, "2 problems", views.Attention},
		{"waiting", sent, store.Entry{}, false, 0, 3, "Waiting list, 3rd", views.InProgress},
		{"on hold", sent, store.Entry{Removal: store.Held}, true, 0, 0, "On hold", views.Attention},
		{"removed", sent, store.Entry{Removal: store.Removed}, true, 0, 0, "Removed", views.Stopped},
	} {
		if got, state := brief(memberState(c.e, c.removal, c.removed, c.place), c.problems, c.place, "Sent"); got != c.want || state != c.state {
			t.Errorf("%s: %q %s, want %q %s", c.name, got, state, c.want, c.state)
		}
	}
}

// Where an entry stands, in the comp sec's order, for every page: removed,
// held, not sent, waiting (unless changed since), changed, checked, sent.
func TestEntryStanding(t *testing.T) {
	for _, c := range []struct {
		name          string
		removal       string
		sent, changed bool
		place         int
		checked       bool
		want          standing
	}{
		{"removed", store.Removed, true, true, 2, true, entryRemoved},
		{"held", store.Held, true, true, 2, true, entryHeld},
		{"not sent", "", false, false, 0, false, entryNotSent},
		{"not sent, though on a list", "", false, false, 2, false, entryNotSent},
		{"waiting, though checked", "", true, false, 2, true, entryWaiting},
		{"changed, though waiting", "", true, true, 2, true, entryChanged},
		{"checked", "", true, false, 0, true, entryChecked},
		{"sent", "", true, false, 0, false, entrySent},
	} {
		if got := stateOf(c.removal, c.sent, c.changed, c.place, c.checked); got != c.want {
			t.Errorf("%s: %d, want %d", c.name, got, c.want)
		}
	}
	// A member's checked entry on a waiting list reads as waiting, as on
	// the comp sec's page: sent, not checked.
	sent := store.MemberEntry{SentAt: time.Now().Add(-time.Hour), UpdatedAt: time.Now().Add(-2 * time.Hour), Checked: true}
	if got := memberStatus(memberState(sent, store.Entry{}, false, 1), store.Entry{}, false); got != "Sent by your club" {
		t.Errorf("a member's checked entry on a waiting list: %q", got)
	}
	if got := memberStatus(memberState(sent, store.Entry{}, false, 0), store.Entry{}, false); got != "Sent by your club · checked by the organiser ✓" {
		t.Errorf("a member's checked entry: %q", got)
	}
	// An individual's: on a waiting list, and checked.
	in := store.Entry{CheckedAt: time.Now()}
	if got, _ := brief(individualState(in, 2), 0, 2, "Entered"); got != "Waiting list, 2nd" {
		t.Errorf("an individual on a waiting list: %q", got)
	}
	if got, _ := brief(individualState(in, 0), 0, 0, "Entered"); got != "Organiser checked" {
		t.Errorf("an individual checked: %q", got)
	}
}
