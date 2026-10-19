package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// Asking before a link is replaced, and coming back to the row changed
// (audit 2026-10-16, clubs and people 6 and 7).
func TestNewLinkAsksFirst(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	ucd := aClub(t, h, admin, "UCD", false, "Ciarán Ó Briain")
	club := ucd.page
	declan := pathIn(t, do(t, h, http.MethodPost, club+"/coaches", url.Values{"name": {"Declan"}}).Body.String(), "/clubs/coach/")

	// A coach's new link: asked first, on Coaches, the old link still working.
	newLink := formAction(t, do(t, h, http.MethodGet, club+"/coaches", nil).Body.String(), club+"/coaches/")
	if !strings.HasSuffix(newLink, "/new-link") {
		t.Fatalf("the first coach form is New link: %s", newLink)
	}
	rec := do(t, h, http.MethodPost, newLink, url.Values{})
	page := rec.Body.String()
	if rec.Code != http.StatusOK || currentTab(t, page) != "Coaches" || !strings.Contains(page, "Make a new link for Declan?") ||
		!strings.Contains(page, "wherever it&#39;s saved") || !strings.Contains(page, `action="`+newLink+`"`) || !strings.Contains(page, `name="anyway" value="1"`) ||
		!strings.Contains(page, `#coach-`) || strings.Contains(page, "/clubs/coach/") {
		t.Fatalf("a coach's new link is asked about first: %d %s", rec.Code, page)
	}
	if do(t, h, http.MethodGet, declan, nil).Code != http.StatusOK {
		t.Error("asking leaves the coach's link working")
	}
	fresh := pathIn(t, do(t, h, http.MethodPost, newLink, url.Values{"anyway": {"1"}}).Body.String(), "/clubs/coach/")
	if do(t, h, http.MethodGet, declan, nil).Code != http.StatusNotFound || do(t, h, http.MethodGet, fresh, nil).Code != http.StatusOK {
		t.Error("saying yes replaces the coach's link")
	}

	// Choosing a member's coach comes back to their row, saying so.
	members := do(t, h, http.MethodGet, club+"/members", nil).Body.String()
	id := regexp.MustCompile(`/members/([^/"]+)/coach"`).FindStringSubmatch(members)[1]
	if !strings.Contains(members, `id="member-`+id+`" class="comp-target"`) {
		t.Error("each member's row has its own place on the page")
	}
	coachID := regexp.MustCompile(`<option value="([^"]+)">Declan</option>`).FindStringSubmatch(members)[1]
	got := redirected(t, h, club+"/members/"+id+"/coach", url.Values{"coach": {coachID}})
	if got != club+"/members?notice="+url.QueryEscape("Ciarán Ó Briain's coach is now Declan.")+"&at="+id+"#member-"+id {
		t.Errorf("back to the member's row with a notice: %s", got)
	}
	if page := do(t, h, http.MethodGet, strings.Split(got, "#")[0], nil).Body.String(); strings.Count(page, "Ciarán Ó Briain&#39;s coach is now Declan.") != 2 ||
		!strings.Contains(page, `is-size-7">Ciarán Ó Briain&#39;s coach is now Declan.`) {
		t.Errorf("the notice shows, at the top and on their row, where the page opens: %s", page)
	}
	if got := redirected(t, h, club+"/members/"+id+"/coach", url.Values{"coach": {""}}); !strings.Contains(got, url.QueryEscape("Ciarán Ó Briain has no coach chosen now.")) {
		t.Errorf("no coach: %s", got)
	}
}

// A club with no coach who signs off: entries say why they wait, and what
// to do (audit 2026-10-16, clubs and people 21).
func TestWhoSignsOff(t *testing.T) {
	h := competitionServer(t)
	form := newCompetition()
	form.Set("signoff", "1")
	admin := created(t, h, form)
	ucd := aClub(t, h, admin, "UCD", true, "Ciarán")
	club, member := ucd.page, ucd.members["Ciarán"]

	entries := clubEntriesPage(t, h, club)
	if !strings.Contains(entries, "no coach of the club signs off routines yet") || !strings.Contains(entries, `href="`+club+`/coaches">Add a coach or let one sign off, on Coaches →`) {
		t.Errorf("the comp sec is told no one can sign off, once: %s", entries)
	}
	if page := memberCompetitionPage(t, h, member); !strings.Contains(page, "No coach of the club signs off routines yet") || !strings.Contains(page, "Ask your comp sec") {
		t.Errorf("the member is told why it waits: %s", page)
	}

	// With a coach, the member is told who signs it off; the comp sec's rows say nothing more.
	do(t, h, http.MethodPost, club+"/coaches", url.Values{"name": {"Declan"}})
	if page := memberCompetitionPage(t, h, member); !strings.Contains(page, "Declan signs it off, on their coach page.") {
		t.Errorf("who signs it off: %s", page)
	}
	if entries := clubEntriesPage(t, h, club); strings.Contains(entries, "signs off routines yet") || strings.Contains(entries, "Declan signs it off") {
		t.Error("the comp sec's rows don't repeat who signs off")
	}

	// A coach who doesn't sign off, chosen by the member: no one can, and the comp sec sees it on the row.
	do(t, h, http.MethodPost, club+"/coaches", url.Values{"name": {"Ann"}})
	coaches := do(t, h, http.MethodGet, club+"/coaches", nil).Body.String()
	declanID := regexp.MustCompile(`/coaches/([^/"]+)/new-link"`).FindStringSubmatch(coaches[strings.Index(coaches, ">Declan"):])[1]
	redirected(t, h, club+"/coaches/"+declanID+"/signs-off", url.Values{"on": {"0"}})
	page := do(t, h, http.MethodGet, member+"/details", nil).Body.String()
	redirected(t, h, member+"/coach", url.Values{"coach": {regexp.MustCompile(`<option value="([^"]+)">Declan</option>`).FindStringSubmatch(page)[1]}})
	if page := memberCompetitionPage(t, h, member); !strings.Contains(page, "Your coach, Declan, doesn&#39;t sign off routines") {
		t.Errorf("their coach doesn't sign off: %s", page)
	}
	if entries := clubEntriesPage(t, h, club); !strings.Contains(entries, "Ciarán&#39;s coach, Declan, doesn&#39;t sign off routines") {
		t.Errorf("the comp sec sees it on the row: %s", entries)
	}
}

// No routine yet: the entry form opens the Routine Builder, which links back
// (audit 2026-10-16, clubs and people 8).
func TestNoRoutineYet(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	ucd := aClub(t, h, admin, "UCD", false, "Ciarán")
	club, member := ucd.page, ucd.members["Ciarán"]

	page := memberCompetitionPage(t, h, member)
	action := formAction(t, page, member+"/competitions/")
	builder := "/?back=" + url.QueryEscape(action+"?discipline=")
	if !strings.Contains(unescape(page), `href="`+builder+`"`) || !strings.Contains(page, "None kept on your page yet") {
		t.Fatalf("the member's form links to the Routine Builder, coming back: %s", page)
	}
	entry := bucsEntry("")
	entry.Del("ex2Skills")
	rec := do(t, h, http.MethodPost, action, entry)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "None to choose? Build one with the Routine Builder link under it.") ||
		!strings.Contains(unescape(rec.Body.String()), `href="`+builder+`"`) {
		t.Errorf("the problem says what to do: %d", rec.Code)
	}

	// The builder links back to the form.
	rec = do(t, h, http.MethodGet, builder, nil)
	if !strings.Contains(unescape(rec.Body.String()), `href="`+action+`?discipline="`) || !strings.Contains(rec.Body.String(), "go back to your entry") || rec.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Errorf("the builder links back: %s", rec.Body.String())
	}
	for _, elsewhere := range []string{"https://example.com/clubs/x", "//example.com/clubs/x", "/requirements", "/\\example.com"} {
		if strings.Contains(do(t, h, http.MethodGet, "/?back="+url.QueryEscape(elsewhere), nil).Body.String(), "builder-back") {
			t.Errorf("no way back to %s", elsewhere)
		}
	}
	if strings.Contains(do(t, h, http.MethodGet, "/", nil).Body.String(), "builder-back") {
		t.Error("the builder on its own has no way back")
	}

	// The comp sec's Enter a member has it too, back to their form.
	enter := regexp.MustCompile(`action="(` + regexp.QuoteMeta(club) + `/competitions/[^"]+/enter-member)"`).FindStringSubmatch(clubEntriesPage(t, h, club))[1]
	members := do(t, h, http.MethodGet, club+"/members", nil).Body.String()
	id := regexp.MustCompile(`/members/([^/"]+)/new-link"`).FindStringSubmatch(members)[1]
	edit := withoutQuery(do(t, h, http.MethodGet, enter+"?member="+id, nil).Header().Get("Location"))
	if page := do(t, h, http.MethodGet, edit, nil).Body.String(); !strings.Contains(unescape(page), `href="/?back=`+url.QueryEscape(edit+"?discipline=")+`"`) || !strings.Contains(page, "None kept on their page yet") {
		t.Errorf("the comp sec's form links to the Routine Builder: %s", page)
	}
}

// Entering another discipline on your own: on your page, back to the
// competition, under your name, only what's not entered yet (audit
// 2026-10-16, clubs and people 11).
func TestEnterAnotherDiscipline(t *testing.T) {
	h := competitionServer(t)
	form := newCompetition()
	form.Set("tumbling", "Novice")
	form.Set("dmt", "Open")
	admin := created(t, h, form)
	enter := pathIn(t, dayAll(t, h, admin+"/settings"), "/competitions/enter/")
	first := withoutQuery(redirected(t, h, enter, bucsEntry("Róisín")))
	link := pathIn(t, dayAll(t, h, first), "/competitions/gymnast/")
	comp := regexp.MustCompile(`href="(` + regexp.QuoteMeta(link) + `/competitions/[^"]+)"`).FindStringSubmatch(dayAll(t, h, link))[1]

	page := dayAll(t, h, comp+"/enter")
	if cur, _ := menuCurrent(page); cur != "Competitions" {
		t.Errorf("on their own page, in Competitions: %s", page)
	}
	if !strings.Contains(page, `href="`+comp+`">← Student Open`) || !strings.Contains(page, "Enter another discipline") {
		t.Errorf("a way back to the competition: %s", page)
	}
	if strings.Contains(page, `name="gymnast"`) {
		t.Error("the name is theirs, not to change")
	}
	if !strings.Contains(page, `>Tumbling</a>`) || !strings.Contains(page, `>DMT</a>`) || strings.Contains(page, `>Trampoline</a>`) {
		t.Errorf("tabs for what's not entered yet: %s", page)
	}
	entryID := strings.SplitN(first, "~", 2)[1]
	if !strings.Contains(page, `href="`+comp+`?entry=`+entryID+`">Trampoline →`) {
		t.Errorf("what's entered links to it: %s", page)
	}

	// Trampoline again is refused, linking to the entry.
	rec := do(t, h, http.MethodPost, comp+"/enter", bucsEntry("Someone Else"))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "already entered Trampoline here.") ||
		!strings.Contains(rec.Body.String(), `href="`+comp+`?entry=`+entryID+`">Change that entry →`) {
		t.Errorf("the refusal links to the entry: %d %s", rec.Code, rec.Body.String())
	}
	// Tumbling goes in under their own name, whatever's posted.
	redirected(t, h, comp+"/enter", url.Values{"gymnast": {"Someone Else"}, "discipline": {"tumbling"}, "level": {"Novice"}})
	if page := dayAll(t, h, comp); strings.Contains(page, "Someone Else") {
		t.Error("entered under their own name")
	}
}
