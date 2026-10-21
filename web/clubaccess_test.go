package web

import (
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"

	"tariffCalculator/store"
)

// Every route under a club's link goes through the gate and is in
// clubRoutes, so a new one can't be left out (ADR 0015); and the table
// names no route that isn't there.
func TestEveryClubRouteHasAPermission(t *testing.T) {
	s := newTestServer(t)
	if len(s.p.clubPatterns) == 0 {
		t.Fatal("no routes went through the club gate")
	}
	var keys []string
	for _, pattern := range s.p.clubPatterns {
		key, ok := clubRoute(pattern)
		if !ok {
			t.Errorf("%s went through the club gate, but isn't under a club link", pattern)
			continue
		}
		keys = append(keys, key)
		if _, ok := clubRoutes[key]; !ok {
			t.Errorf("%s isn't in clubRoutes: say what it needs", pattern)
		}
	}
	for key := range clubRoutes {
		if !slices.Contains(keys, key) {
			t.Errorf("clubRoutes names %q, which isn't a route", key)
		}
	}
}

// clubAllows are the routes each permission adds to seeing the club, as ADR
// 0015 Decision 2 and the owner's decisions say; seeClub's every role has,
// adminOnly's only the club admin.
var clubAllows = map[string][]string{
	seeClub: {"GET /", "GET /members", "GET /coaches", "GET /competitions/{id}", "GET /competitions/{id}/coaches",
		"GET /competitions/{id}/offers", "GET /competitions/{id}/timeline", "GET /competitions/{id}/calendar.ics",
		"GET /members/{member}/routines", "GET /members/{member}/skills", "POST /feedback"},
	store.ClubEnter: {"GET /competitions/{id}/enter-member", "GET /members/{member}/competitions/{id}", "POST /members/{member}/competitions/{id}",
		"POST /members/{member}/competitions/{id}/withdraw", "GET /competitions/{id}/add-offer", "GET /members/{member}/competitions/{id}/offer",
		"POST /members/{member}/competitions/{id}/offer", "GET /members/{member}/competitions/{id}/late", "POST /members/{member}/competitions/{id}/late"},
	store.ClubSend: {"POST /competitions/{id}/send", "POST /competitions/{id}/coaches"},
	store.ClubFees: {"GET /competitions/{id}/fees", "GET /competitions/{id}/invoice"},
	store.ClubMembers: {"POST /members/{member}/new-link", "POST /members/{member}/remove", "POST /members/{member}/coach",
		"POST /members/{member}/judging", "POST /members/{member}/judging/{q}/remove"},
	store.ClubCoaches: {"POST /coaches", "POST /coaches/from-member", "POST /coaches/{coach}/new-link", "POST /coaches/{coach}/remove",
		"POST /coaches/{coach}/signs-off", "POST /coaches/{coach}/qualifications", "POST /coaches/{coach}/qualifications/{q}/remove"},
	store.ClubCertificates: {"GET /coaches/{coach}/qualifications/{q}/certificate", "GET /members/{member}/judging/{q}"},
	store.ClubOrganiser: {"POST /competitions/{id}/organiser", "POST /competitions/{id}/organiser/{message}",
		"GET /competitions/{id}/notify", "POST /competitions/{id}/notify"},
	store.ClubSettings: {"GET /settings", "POST /coaches-see-all", "POST /skills-open"},
	adminOnly: {"POST /roles", "POST /roles/{role}", "POST /roles/{role}/remove", "POST /links", "POST /links/{link}/role",
		"POST /links/{link}/remove", "POST /ties/remove", "POST /replace-link", "POST /delete"},
}

// clubTestRole makes a role with these permissions (through the store) and
// a link with it (through the club admin's Settings), returning the link's
// path.
func clubTestRole(t *testing.T, s *testServer, club, role, person string, permissions ...string) string {
	t.Helper()
	c, err := s.st.ClubByAdmin(t.Context(), strings.TrimPrefix(club, "/clubs/admin/"))
	if err != nil {
		t.Fatal(err)
	}
	roles, err := s.st.ClubRoles(t.Context(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(roles, func(r store.ClubRole) bool { return r.Name == role })
	id := ""
	if i >= 0 {
		id = roles[i].ID
	} else {
		r, err := s.st.AddClubRole(t.Context(), c.ID, role, permissions)
		if err != nil {
			t.Fatal(err)
		}
		id = r.ID
	}
	rec := do(t, s.h, http.MethodPost, club+"/links", url.Values{"name": {person}, "role": {id}})
	if rec.Code != http.StatusOK {
		t.Fatalf("making %s's link: %d %s", person, rec.Code, rec.Header().Get("Location"))
	}
	return pathIn(t, rec.Body.String(), "/clubs/admin/")
}

// Each route under a club link is open to the club admin, and to a role only
// if it's seeing the club or the role has the route's permission; the rest
// say the link can't do that. The club admin's own routes are its alone.
func TestClubRoutesByPermission(t *testing.T) {
	s := newTestServer(t)
	admin := created(t, s.h, newCompetition())
	ucd := aClub(t, s.h, admin, "UCD", true, "Ann")
	compID := regexp.MustCompile(`/competitions/([^"/?]+)"`).FindStringSubmatch(do(t, s.h, http.MethodGet, ucd.page, nil).Body.String())[1]

	// Every route is in the ADR's list once.
	seen := map[string]string{}
	for perm, routes := range clubAllows {
		for _, route := range routes {
			if was, ok := seen[route]; ok {
				t.Errorf("%s is under %q and %q", route, was, perm)
			}
			seen[route] = perm
		}
	}
	for key, need := range clubRoutes {
		if seen[key] != need {
			t.Errorf("%s needs %q; the ADR says %q", key, need, seen[key])
		}
	}

	links := map[string]string{"none": clubTestRole(t, s, ucd.page, "Looker", "Lou")}
	for _, perm := range store.ClubPermissions {
		links[perm] = clubTestRole(t, s, ucd.page, "Only "+perm, "Person "+perm, perm)
	}
	request := func(link, key string) *http.Response {
		method, path, _ := strings.Cut(key, " ")
		path = strings.TrimSuffix(link+path, "/")
		path = strings.ReplaceAll(path, "{id}", compID)
		path = regexp.MustCompile(`\{[a-z]+\}`).ReplaceAllString(path, "x")
		var form url.Values
		if method == http.MethodPost {
			form = url.Values{}
		}
		return do(t, s.h, method, path, form).Result()
	}
	refused := func(res *http.Response) bool {
		if res.StatusCode != http.StatusForbidden {
			return false
		}
		var b strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := res.Body.Read(buf)
			b.Write(buf[:n])
			if err != nil {
				break
			}
		}
		return strings.Contains(unescape(b.String()), "This link can't do that. Ask your club admin.")
	}
	for who, link := range links {
		for key, need := range clubRoutes {
			want := need == seeClub || need == who
			if got := !refused(request(link, key)); got != want {
				t.Errorf("a role with %s: %s allowed %v, want %v", who, key, got, want)
			}
		}
	}
	// The club admin can use every route (last: some change things).
	for key := range clubRoutes {
		if refused(request(ucd.page, key)) {
			t.Errorf("the club admin is refused %s", key)
		}
	}
}

// A role's permissions change for its links at once; a role with links
// can't be removed until they're moved; a removed link stops at once.
func TestClubRoleChanges(t *testing.T) {
	s := newTestServer(t)
	admin := created(t, s.h, newCompetition())
	ucd := aClub(t, s.h, admin, "UCD", true, "Ann")
	club := ucd.page

	// Made on Settings: the role, then a link with it, shown once.
	if got := redirected(t, s.h, club+"/roles", url.Values{"name": {"Treasurer"}, "permission": {store.ClubFees}}); !strings.Contains(noticeOf(t, got), "Made the role Treasurer") {
		t.Errorf("made the role: %s", got)
	}
	if got := noticeOf(t, redirected(t, s.h, club+"/roles", url.Values{"name": {"treasurer"}})); !strings.Contains(got, "already a role called treasurer; nothing was changed") {
		t.Errorf("one role a name: %s", got)
	}
	settings := do(t, s.h, http.MethodGet, club+"/settings", nil).Body.String()
	role := regexp.MustCompile(`action="` + regexp.QuoteMeta(club) + `/roles/([^"/]+)"`).FindStringSubmatch(settings)
	if role == nil {
		t.Fatalf("the role on Settings: %s", settings)
	}
	rec := do(t, s.h, http.MethodPost, club+"/links", url.Values{"name": {"Mary"}, "role": {role[1]}})
	if rec.Code != http.StatusOK || !strings.Contains(unescape(rec.Body.String()), "Mary's link (Treasurer)") {
		t.Fatalf("the new link, shown once: %d", rec.Code)
	}
	mary := pathIn(t, rec.Body.String(), "/clubs/admin/")
	if got := noticeOf(t, redirected(t, s.h, club+"/links", url.Values{"name": {"Mary"}, "role": {role[1]}})); !strings.Contains(got, "already a link for Mary") {
		t.Errorf("a reload doesn't make another: %s", got)
	}
	settings = unescape(do(t, s.h, http.MethodGet, club+"/settings", nil).Body.String())
	if !strings.Contains(settings, "<strong>Mary</strong>") || strings.Contains(settings, mary) {
		t.Errorf("Settings lists Mary's link, never showing it again")
	}

	send := formAction(t, clubEntriesPage(t, s.h, club), club+"/competitions/")
	send = strings.Replace(send, club, mary, 1)
	if rec := do(t, s.h, http.MethodPost, send, url.Values{"which": {"all"}}); rec.Code != http.StatusForbidden {
		t.Errorf("a Treasurer can't send: %d", rec.Code)
	}
	redirected(t, s.h, club+"/roles/"+role[1], url.Values{"name": {"Treasurer"}, "permission": {store.ClubFees, store.ClubSend}})
	if rec := do(t, s.h, http.MethodPost, send, url.Values{"which": {"all"}}); rec.Code != http.StatusSeeOther {
		t.Errorf("once the role can send, Mary can, at once: %d", rec.Code)
	}

	// A role with links stays until they're moved.
	got := noticeOf(t, redirected(t, s.h, club+"/roles/"+role[1]+"/remove", url.Values{}))
	if got != "Treasurer still has 1 link: give them another role or remove them first; nothing was changed." {
		t.Errorf("a role with links stays: %s", got)
	}
	redirected(t, s.h, club+"/roles", url.Values{"name": {"Comp sec"}, "permission": {store.ClubEnter, store.ClubSend}})
	settings = do(t, s.h, http.MethodGet, club+"/settings", nil).Body.String()
	compSec := regexp.MustCompile(`<option value="([^"]+)"[^>]*>Comp sec</option>`).FindStringSubmatch(settings)
	link := regexp.MustCompile(`action="` + regexp.QuoteMeta(club) + `/links/([^"/]+)/role"`).FindStringSubmatch(settings)
	if compSec == nil || link == nil {
		t.Fatalf("Change role on Settings: %s", settings)
	}
	if got := noticeOf(t, redirected(t, s.h, club+"/links/"+link[1]+"/role", url.Values{"role": {compSec[1]}})); got != "Mary's link is Comp sec now." {
		t.Errorf("moved: %s", got)
	}
	if rec := do(t, s.h, http.MethodGet, clubCompetitionTabOf(t, s.h, club, mary, "/fees"), nil); rec.Code != http.StatusForbidden {
		t.Errorf("as Comp sec, Mary can't see the fees: %d", rec.Code)
	}
	if got := noticeOf(t, redirected(t, s.h, club+"/roles/"+role[1]+"/remove", url.Values{})); got != "Removed the role Treasurer." {
		t.Errorf("removed once it had no links: %s", got)
	}

	// Removing the link asks first, then stops it at once.
	if rec := do(t, s.h, http.MethodPost, club+"/links/"+link[1]+"/remove", url.Values{}); rec.Code != http.StatusOK || !strings.Contains(unescape(rec.Body.String()), "Remove Mary's link?") {
		t.Errorf("asks first: %d", rec.Code)
	}
	if do(t, s.h, http.MethodGet, mary, nil).Code != http.StatusOK {
		t.Error("asking changed nothing")
	}
	redirected(t, s.h, club+"/links/"+link[1]+"/remove", url.Values{"anyway": {"1"}})
	if rec := do(t, s.h, http.MethodGet, mary, nil); rec.Code != http.StatusNotFound {
		t.Errorf("a removed link stops at once: %d", rec.Code)
	}
}

// clubCompetitionTabOf is a tab of the club's first competition, under
// another of its links.
func clubCompetitionTabOf(t *testing.T, h http.Handler, club, link, tab string) string {
	t.Helper()
	m := regexp.MustCompile(`href="` + regexp.QuoteMeta(club) + `(/competitions/[^"/?]+)"`).FindStringSubmatch(do(t, h, http.MethodGet, club, nil).Body.String())
	if m == nil {
		t.Fatalf("no competition in %s's list", club)
	}
	return link + m[1] + tab
}

// Pages offer only what the link's role can do; the club admin's pages say
// it's for setting up.
func TestClubPagesShowWhatTheRoleCanDo(t *testing.T) {
	s := newTestServer(t)
	admin := created(t, s.h, newCompetition())
	ucd := aClub(t, s.h, admin, "UCD", true, "Ann")
	club := ucd.page
	do(t, s.h, http.MethodPost, club+"/coaches", url.Values{"name": {"Cara"}})
	looker := clubTestRole(t, s, club, "Looker", "Lou")
	everything := clubTestRole(t, s, club, "Everything", "Eve", store.ClubPermissions...)

	home := unescape(do(t, s.h, http.MethodGet, club, nil).Body.String())
	if !strings.Contains(home, "You're using the <strong>club admin</strong> link: it's for setting the club up.") || !strings.Contains(home, `href="`+club+`/settings#roles"`) {
		t.Error("the club admin's pages say it's for setting up, and where to make a link with a role")
	}
	page := unescape(do(t, s.h, http.MethodGet, looker, nil).Body.String())
	if !strings.Contains(page, "You're using the link for <strong>Lou (Looker)</strong>.") || strings.Contains(page, `href="`+looker+`/settings"`) {
		t.Error("a role's pages say whose link it is, with no Settings without Club settings")
	}
	if p := do(t, s.h, http.MethodGet, everything, nil).Body.String(); !strings.Contains(p, `href="`+everything+`/settings"`) {
		t.Error("Settings with Club settings")
	}

	for _, tc := range []struct {
		page string
		has  []string // with every permission, and not with none
	}{
		{"/members", []string{"Join link", ">Remove</button>", ">New link</button>", "Make a coach too", `name="coach"`}},
		{"/coaches", []string{"Add coach", ">New link</button>", "/signs-off\"", "Every coach sees every member"}},
	} {
		all := do(t, s.h, http.MethodGet, everything+tc.page, nil).Body.String()
		none := do(t, s.h, http.MethodGet, looker+tc.page, nil).Body.String()
		for _, want := range tc.has {
			if !strings.Contains(all, want) {
				t.Errorf("%s has %q with every permission", tc.page, want)
			}
			if strings.Contains(none, want) {
				t.Errorf("%s hasn't %q without", tc.page, want)
			}
		}
		if !strings.Contains(none, "Ann") && tc.page == "/members" || !strings.Contains(none, "Cara") && tc.page == "/coaches" {
			t.Errorf("%s still lists them", tc.page)
		}
	}
	all := unescape(do(t, s.h, http.MethodGet, clubCompetitionTabOf(t, s.h, club, everything, ""), nil).Body.String())
	none := unescape(do(t, s.h, http.MethodGet, clubCompetitionTabOf(t, s.h, club, looker, ""), nil).Body.String())
	for _, want := range []string{"Send new and changed", "Enter a member", ">Change</a>", "Tell the organiser"} {
		if !strings.Contains(all, want) {
			t.Errorf("the entries have %q with every permission", want)
		}
		if strings.Contains(none, want) {
			t.Errorf("the entries haven't %q without", want)
		}
	}
	if !strings.Contains(none, "All sent") || !strings.Contains(none, "Ann") {
		t.Error("without Send, where sending stands; and the entries")
	}

	// Settings: the roles, links and replacing the admin link are the club
	// admin's; the history, Club settings'.
	settings := do(t, s.h, http.MethodGet, everything+"/settings", nil).Body.String()
	for _, not := range []string{`id="roles"`, `id="links"`, "Replace the club admin link", "Delete the club"} {
		if strings.Contains(settings, not) {
			t.Errorf("a role's Settings hasn't %q", not)
		}
	}
	if !strings.Contains(settings, `id="history"`) || !strings.Contains(settings, "Members&#39; skills") {
		t.Error("a role with Club settings sees the history and the settings")
	}
	settings = do(t, s.h, http.MethodGet, club+"/settings", nil).Body.String()
	for _, want := range []string{`id="roles"`, `id="links"`, `id="history"`, "Replace the club admin link"} {
		if !strings.Contains(settings, want) {
			t.Errorf("the club admin's Settings has %q", want)
		}
	}
}

// What's done or sent through a link says who of the club: feedback,
// messages to the organiser and what's added to them, and the club's
// history. The club's messages are seen and answered by each link that can
// talk to the organiser.
func TestClubNamesWhoDidIt(t *testing.T) {
	s := newTestServer(t, atTime(inDublin(2030, 1, 10, 12, 0)))
	h, site := s.h, maintainerLink(t, s)
	admin := created(t, h, newCompetition())
	ucd := aClub(t, h, admin, "UCD", true, "Ann Ryan")
	club := ucd.page
	mary := clubTestRole(t, s, club, "Team manager", "Mary", store.ClubOrganiser, store.ClubEnter)
	lou := clubTestRole(t, s, club, "Looker", "Lou")

	// Feedback.
	if code, _ := sendFeedbackFrom(t, h, mary, "Where's the timetable?"); code != http.StatusOK {
		t.Fatalf("feedback: %d", code)
	}
	if items := feedbackItems(t, h, site); len(items) == 0 || items[0][0] != "Mary (Team manager), UCD" {
		t.Errorf("feedback says who of the club: %v", items)
	}

	// A message from the club admin, which Mary sees and adds to.
	page := clubCompetitionTabOf(t, h, club, club, "")
	action, fields, _, ok := tellFormOf(t, do(t, h, http.MethodGet, page, nil).Body.String())
	if !ok {
		t.Fatal("the club admin can tell the organiser")
	}
	fields.Set("text", "Is there parking?")
	redirected(t, h, action, fields)
	if got := messagesOn(t, h, admin); len(got) == 0 || got[0] != [2]string{"Is there parking?", "Club admin, UCD"} {
		t.Errorf("from the club admin: %v", got)
	}
	maryPage := clubCompetitionTabOf(t, h, club, mary, "")
	if sent := sentOn(t, h, maryPage); len(sent) != 1 || !strings.Contains(sent[0], "Club admin,") || !strings.Contains(sent[0], "Is there parking?") {
		t.Errorf("Mary sees the club's message, and who sent it: %v", sent)
	}
	add := regexp.MustCompile(`action="(` + regexp.QuoteMeta(maryPage) + `/organiser/[^"]+)"`).FindStringSubmatch(do(t, h, http.MethodGet, maryPage, nil).Body.String())
	if add == nil {
		t.Fatal("Mary can add to it")
	}
	redirected(t, h, add[1], url.Values{"text": {"Or a bus?"}})
	dashboard := unescape(do(t, h, http.MethodGet, admin, nil).Body.String())
	if !strings.Contains(dashboard, "Mary (Team manager), UCD added") {
		t.Error("the organiser sees who of the club added to it")
	}
	if sent := sentOn(t, h, clubCompetitionTabOf(t, h, club, club, "")); len(sent) != 1 || !strings.Contains(sent[0], "Mary (Team manager) added") {
		t.Errorf("the club admin sees who added it: %v", sent)
	}
	// Mary's own message.
	action, fields, _, _ = tellFormOf(t, do(t, h, http.MethodGet, maryPage, nil).Body.String())
	if !strings.Contains(unescape(do(t, h, http.MethodGet, maryPage, nil).Body.String()), "From <strong>Mary (Team manager), UCD</strong>") {
		t.Error("the form says who it's from")
	}
	fields.Set("text", "We have a late entry")
	redirected(t, h, action, fields)
	if got := messagesOn(t, h, admin); got[0] != [2]string{"We have a late entry", "Mary (Team manager), UCD"} {
		t.Errorf("from Mary: %v", got)
	}
	// A role without Talk to the organiser has no form, nor the club's messages.
	if _, _, _, ok := tellFormOf(t, do(t, h, http.MethodGet, clubCompetitionTabOf(t, h, club, lou, ""), nil).Body.String()); ok {
		t.Error("Lou can't tell the organiser")
	}

	// The history: who and what, latest first.
	withdraw := regexp.MustCompile(`href="` + regexp.QuoteMeta(mary) + `(/members/[^"?]+)\?discipline=([^"&]*)"`).FindStringSubmatch(do(t, h, http.MethodGet, maryPage, nil).Body.String())
	if withdraw == nil {
		t.Fatal("Mary can change Ann's entry")
	}
	redirected(t, h, mary+withdraw[1]+"/withdraw", url.Values{"discipline": {withdraw[2]}, "confirm": {"1"}})
	history := clubHistoryOn(t, h, club)
	want := [][2]string{
		{"Mary (Team manager)", "Withdrew Ann Ryan · BUCS L3"},
		{"Mary (Team manager)", "Sent the organiser of Student Open a message"},
		{"Mary (Team manager)", "Added to a message to the organiser of Student Open"},
		{"Club admin", "Sent the organiser of Student Open a message"},
	}
	if len(history) < len(want) || !slices.Equal(history[:len(want)], want) {
		t.Errorf("the history: %v", history)
	}
	if !slices.Contains(history, [2]string{"Club admin", "Made a link for Lou (Looker)"}) {
		t.Errorf("making links is in it: %v", history)
	}
}

// clubHistoryOn is the club's history on its Settings, as who and what,
// latest first.
func clubHistoryOn(t *testing.T, h http.Handler, club string) [][2]string {
	t.Helper()
	page := unescape(do(t, h, http.MethodGet, club+"/settings", nil).Body.String())
	var out [][2]string
	for _, m := range regexp.MustCompile(`<td data-label="Who">([^<]*)</td><td data-label="What">([^<]*)</td>`).FindAllStringSubmatch(page, -1) {
		out = append(out, [2]string{m[1], m[2]})
	}
	return out
}

// A person can add a club role's link to their own page: their link then
// opens the club as that role, with a tab named for it.
func TestClubRoleOnAPersonalLink(t *testing.T) {
	s := newTestServer(t)
	admin := created(t, s.h, newCompetition())
	ucd := aClub(t, s.h, admin, "UCD", false, "Mary")
	member := ucd.members["Mary"]
	treasurer := clubTestRole(t, s, ucd.page, "Treasurer", "Mary", store.ClubFees)
	if got := noticeOf(t, redirected(t, s.h, member+"/links", url.Values{"link": {"http://example.com" + treasurer}})); got != "Added: Treasurer at UCD. Use the tabs at the top to switch." {
		t.Errorf("added: %s", got)
	}
	home := do(t, s.h, http.MethodGet, member, nil).Body.String()
	tab := roleTabTo(t, home, "Treasurer")
	page := unescape(do(t, s.h, http.MethodGet, tab, nil).Body.String())
	if currentRole(t, page) != "Treasurer" || !strings.Contains(page, "You're using the link for <strong>Mary (Treasurer)</strong>.") {
		t.Errorf("her link opens the club as Treasurer: %s", tab)
	}
	if rec := do(t, s.h, http.MethodGet, tab+"/settings", nil); rec.Code != http.StatusForbidden {
		t.Errorf("as Treasurer: %d", rec.Code)
	}
}
