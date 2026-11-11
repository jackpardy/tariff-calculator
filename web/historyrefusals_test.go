package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"tariffCalculator/store"
)

// A change that was turned down is not in the history, whatever its notice
// says: each refusal below leaves the count of its row as it was, and the
// same kind of change made properly adds one.
func TestHistoryLeavesOutRefusedChanges(t *testing.T) {
	ls := lateCompetition(t)
	h, admin := ls.s.h, ls.admin
	rows := func(prefix string) int {
		n := 0
		for _, row := range tableRows(t, do(t, h, http.MethodGet, admin+"/history", nil).Body.String()) {
			if len(row) == 3 && strings.HasPrefix(row[2], prefix) {
				n++
			}
		}
		return n
	}
	lateID := func(action string) string {
		t.Helper()
		m := regexp.MustCompile(`/late/([^/"]+)/` + action).FindStringSubmatch(do(t, h, http.MethodGet, admin+"/late", nil).Body.String())
		if m == nil {
			t.Fatalf("no late change to %s", action)
		}
		return m[1]
	}
	// try posts a change and checks the history's row for it: one more if
	// it was made, the same if it was turned down.
	try := func(name, path string, form url.Values, row string, made bool) {
		t.Helper()
		before := rows(row)
		rec := do(t, h, http.MethodPost, admin+path, form)
		if rec.Code >= 400 && made {
			t.Errorf("%s: %d", name, rec.Code)
		}
		want := before
		if made {
			want++
		}
		if got := rows(row); got != want {
			t.Errorf("%s: %q rows %d, want %d (answered %d %s)", name, row, got, want, rec.Code, rec.Header().Get("Location"))
		}
	}

	redirected(t, h, admin+"/fees/settings", url.Values{"fee-": {"12"}, "club": {"25"}})
	// Made properly: each adds its row.
	try("a link with a name", "/links", url.Values{"name": {"Zed"}, "kind": {"cards"}}, "Made a link for Zed", true)
	try("a payment", "/fees/payments", url.Values{"payer": {"club:" + strings.Split(strings.Split(do(t, h, http.MethodGet, admin+"/fees", nil).Body.String(), `name="payer" value="club:`)[1], `"`)[0]}, "amount": {"25"}}, "Recorded a payment", true)

	// Turned down: nothing is added.
	try("a link with no name", "/links", url.Values{"name": {" "}, "kind": {"cards"}}, "Made a link for", false)
	try("a link made twice", "/links", url.Values{"name": {"zed"}, "kind": {"cards"}}, "Made a link for", false)
	try("going live at a time gone by", "/live", url.Values{"opens": {"at"}, "liveDate": {"2000-01-01"}, "liveTime": {"10:00"}}, "Set when it goes live", false)
	try("going live after entries close", "/live", url.Values{"opens": {"at"}, "liveDate": {"2031-01-01"}, "liveTime": {"10:00"}}, "Set when it goes live", false)
	try("removing a link that's gone", "/links/gone/remove", url.Values{"anyway": {"1"}}, "Removed a link", false)
	try("removing a payment that's gone", "/fees/payments/gone/remove", url.Values{"anyway": {"1"}}, "Removed a payment", false)
	try("adding nobody to the officials", "/officials/add", url.Values{"name": {""}}, "Changed the officials", false)
	try("moving no one", "/timetable/entry", url.Values{}, "Moved a gymnast", false)
	try("timings that aren't numbers", "/timetable/setup/timings", url.Values{"rest": {"x"}}, "Changed the timetable's timings", false)

	// Two helpers act on one late change: the second is turned down, and
	// the history has the first alone.
	redirected(t, h, ls.ivy+"/late", l4Entry("Ivy"))
	id := lateID("accept")
	try("accepting a late change", "/late/"+id+"/accept", url.Values{}, "Accepted a late change", true)
	try("accepting it again", "/late/"+id+"/accept", url.Values{}, "Accepted a late change", false)
	if got := rows("Accepted a late change"); got != 1 {
		t.Errorf("a late change accepted by two helpers is in the history %d times", got)
	}

	// And turning one down needs a reason, and can't be done twice.
	redirected(t, h, ls.ivy+"/late", bucsEntry("Ivy"))
	id = lateID("reject")
	try("turning a late change down with no reason", "/late/"+id+"/reject", url.Values{"reason": {""}}, "Turned down a late change", false)
	try("turning a late change down", "/late/"+id+"/reject", url.Values{"reason": {"Too late"}}, "Turned down a late change", true)
	try("turning it down again", "/late/"+id+"/reject", url.Values{"reason": {"Too late"}}, "Turned down a late change", false)
	try("making it private", "/live", url.Values{"opens": {"private"}}, "Made it private", true)
}

// The same for a club's link: a change it was refused is not in the club's
// history.
func TestClubHistoryLeavesOutRefusedChanges(t *testing.T) {
	ls := lateCompetition(t)
	h, club := ls.s.h, ls.ucd.page
	c, err := ls.s.st.ClubByAdmin(t.Context(), strings.TrimPrefix(club, "/clubs/admin/"))
	if err != nil {
		t.Fatal(err)
	}
	members, err := ls.s.st.Members(t.Context(), c.ID)
	if err != nil || len(members) != 1 {
		t.Fatalf("members: %v %v", members, err)
	}
	ann := members[0]
	comp := regexp.MustCompile(`/competitions/([^/"?]+)`).FindStringSubmatch(clubEntriesPage(t, h, club))
	if comp == nil {
		t.Fatal("no competition")
	}
	rows := func(prefix string) int {
		n := 0
		for _, row := range clubHistoryOn(t, h, club) {
			if strings.HasPrefix(row[1], prefix) {
				n++
			}
		}
		return n
	}
	try := func(name, path string, form url.Values, row string, made bool) {
		t.Helper()
		before := rows(row)
		rec := do(t, h, http.MethodPost, club+path, form)
		if rec.Code >= 400 && made {
			t.Errorf("%s: %d", name, rec.Code)
		}
		want := before
		if made {
			want++
		}
		if got := rows(row); got != want {
			t.Errorf("%s: %q rows %d, want %d (answered %d %s)", name, row, got, want, rec.Code, rec.Header().Get("Location"))
		}
	}

	try("a coach with a name", "/coaches", url.Values{"name": {"Cal"}}, "Added a coach", true)
	try("a coach with no name", "/coaches", url.Values{"name": {""}}, "Added a coach", false)
	try("a coach with a name too long", "/coaches", url.Values{"name": {strings.Repeat("x", 300)}}, "Added a coach", false)
	try("a member made a coach", "/coaches/from-member", url.Values{"member": {ann.ID}}, "Made Ann a coach", true)
	try("a coach made again", "/coaches/from-member", url.Values{"member": {ann.ID}}, "Made Ann a coach", false)

	coaches, err := ls.s.st.Coaches(t.Context(), c.ID)
	if err != nil || len(coaches) == 0 {
		t.Fatalf("coaches: %v %v", coaches, err)
	}
	try("no coaches sent", "/competitions/"+comp[1]+"/coaches", url.Values{}, "Sent coaches", false)
	try("a coach sent", "/competitions/"+comp[1]+"/coaches", url.Values{"coach": {coaches[0].ID}}, "Sent coaches", true)

	try("who coaches see changed", "/coaches-see-all", url.Values{"on": {"1"}}, "Changed who coaches see", true)
	// Entries have closed: the answer is 4xx, which is never recorded.
	try("an entry changed after the deadline", "/members/"+ann.ID+"/competitions/"+comp[1], bucsEntry(""), "Changed an entry", false)

	try("a link with no role", "/links", url.Values{"name": {"Mary"}, "role": {"gone"}}, "Made a link for", false)
	try("removing a link that's gone", "/links/gone/remove", url.Values{"anyway": {"1"}}, "Removed the link for", false)
	try("removing a role that's gone", "/roles/gone/remove", url.Values{}, "Removed the role", false)
	try("removing a tie that's gone", "/ties/remove", url.Values{"id": {"gone"}}, "Stopped someone's own link", false)

	// A role's link is held to the same.
	mary := clubTestRole(t, ls.s, club, "Coach manager", "Mary", store.ClubCoaches)
	rec := do(t, h, http.MethodPost, mary+"/coaches", url.Values{"name": {""}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Mary adding no one: %d", rec.Code)
	}
	if got := rows("Added a coach"); got != 1 {
		t.Errorf("Mary's refused change is in the history: %d rows", got)
	}
}
