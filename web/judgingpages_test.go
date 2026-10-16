package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// A member adds a judging qualification beside their offer; the organiser,
// approving judges, sees it with its certificate, flags who has none, and
// the member sees once they're approved. An individual adds theirs on their
// entry.
func TestJudgingQualifications(t *testing.T) {
	h := competitionServer(t)
	form := newCompetition()
	form.Set("tumbling", "Novice")
	admin := created(t, h, form)
	officials := admin + "/officials"
	redirected(t, h, officials+"/settings", url.Values{"judge": {"qualified"},
		"panel-trampoline-chair": {"1"}, "panel-trampoline-execution": {"4"}, "panel-trampoline-difficulty": {"1"}, "panel-trampoline-recorder": {"1"}, "panel-trampoline-marshal": {"0"},
		"panel-tumbling-chair": {"1"}, "panel-tumbling-execution": {"4"}, "panel-tumbling-difficulty": {"1"}, "panel-tumbling-recorder": {"1"}, "panel-tumbling-marshal": {"0"}})
	dash := do(t, h, http.MethodGet, admin+"/settings", nil).Body.String()
	club := withoutQuery(redirected(t, h, "/clubs", url.Values{"name": {"UCD"}}))
	redirected(t, h, pathIn(t, dash, "/competitions/club/"), url.Values{"clubAdmin": {club}})
	a := withoutQuery(redirected(t, h, pathIn(t, do(t, h, http.MethodGet, club+"/members", nil).Body.String(), "/clubs/join/"), url.Values{"name": {"A"}}))

	page := memberCompetitionPage(t, h, a)
	if !strings.Contains(page, "Your judging qualifications") || !strings.Contains(page, "The organiser approves judges") || !strings.Contains(page, `action="`+a+`/judging"`) {
		t.Fatalf("the member's offer asks for their judging qualifications: %s", page)
	}
	back := regexp.MustCompile(`name="back" value="([^"]+)"`).FindStringSubmatch(page)[1]
	pdf := []byte("%PDF-1.4\nA judge")
	if rec := upload(t, h, a+"/judging", map[string]string{"qualification": "bg-trampoline-judge-2", "back": back}, "certificate", pdf); rec.Code != http.StatusSeeOther ||
		!strings.Contains(rec.Header().Get("Location"), "Added+British+Gymnastics+Level+2+Trampoline+Judge") || !strings.HasSuffix(rec.Header().Get("Location"), "#judging") {
		t.Fatalf("added: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	if rec := upload(t, h, a+"/judging", map[string]string{"qualification": "bg-trampoline-judge-2"}, "certificate", []byte("not a certificate")); !strings.Contains(rec.Header().Get("Location"), "Nothing+was+added") {
		t.Error("only a photo or PDF")
	}
	page = memberCompetitionPage(t, h, a)
	cert := regexp.MustCompile(`href="(` + regexp.QuoteMeta(a) + `/judging/[^"]+)"`).FindStringSubmatch(page)
	if cert == nil || !strings.Contains(page, "British Gymnastics Level 2 Trampoline Judge") {
		t.Fatalf("listed, with its certificate: %s", page)
	}
	if rec := do(t, h, http.MethodGet, cert[1], nil); rec.Body.String() != string(pdf) || rec.Header().Get("Content-Type") != "application/pdf" {
		t.Error("the member opens their certificate")
	}

	// Offering to judge trampoline and tumbling, sent: the organiser sees
	// the certificate, and that there's none for tumbling.
	offer := regexp.MustCompile(`action="(` + regexp.QuoteMeta(a) + `/competitions/[^"]+/offer)"`).FindStringSubmatch(page)[1]
	redirected(t, h, offer, url.Values{"judge-trampoline": {"1"}, "judge-tumbling": {"1"}})
	redirected(t, h, formAction(t, clubEntriesPage(t, h, club), club+"/competitions/"), url.Values{"which": {"all"}})
	page = do(t, h, http.MethodGet, officials, nil).Body.String()
	theirs := regexp.MustCompile(`href="(` + regexp.QuoteMeta(officials) + `/[^/"]+/judging/[^"]+)"`).FindStringSubmatch(page)
	if theirs == nil || !strings.Contains(page, "British Gymnastics Level 2 Trampoline Judge") || !strings.Contains(page, "No judging qualification for Tumbling") {
		t.Fatalf("the organiser sees it, and the gap: %s", page)
	}
	if rec := do(t, h, http.MethodGet, theirs[1], nil); rec.Body.String() != string(pdf) {
		t.Error("the organiser opens the certificate")
	}
	id := regexp.MustCompile(regexp.QuoteMeta(officials) + `/([^/"]+)/qualified`).FindStringSubmatch(page)[1]
	redirected(t, h, officials+"/"+id+"/qualified", url.Values{"on": {"1"}})
	if !strings.Contains(memberCompetitionPage(t, h, a), "Approved to judge by the organiser") {
		t.Error("the member sees they're approved")
	}

	// The comp sec adds one for them too, and removes it.
	edit := regexp.MustCompile(`href="(` + regexp.QuoteMeta(club) + `/members/[^"]+/offer)"`).FindStringSubmatch(clubCompetitionTab(t, h, club, "/offers"))[1]
	page = do(t, h, http.MethodGet, edit, nil).Body.String()
	add := regexp.MustCompile(`action="(` + regexp.QuoteMeta(club) + `/members/[^"]+/judging)"`).FindStringSubmatch(page)
	if add == nil || !strings.Contains(page, "A&#39;s judging qualifications") {
		t.Fatalf("the comp sec's view of their offer: %s", page)
	}
	upload(t, h, add[1], map[string]string{"qualification": "gi-tumbling-judge-1", "back": edit}, "certificate", pdf)
	page = do(t, h, http.MethodGet, edit, nil).Body.String()
	remove := regexp.MustCompile(`action="(`+regexp.QuoteMeta(add[1])+`/[^"]+/remove)"`).FindAllStringSubmatch(page, -1)
	if len(remove) != 2 || strings.Contains(do(t, h, http.MethodGet, officials, nil).Body.String(), "No judging qualification for Tumbling") {
		t.Fatalf("two now, tumbling covered: %v", remove)
	}
	if to := redirected(t, h, remove[1][1], url.Values{"back": {edit}}); !strings.HasPrefix(to, edit+"?") {
		t.Errorf("removed, back to the page: %s", to)
	}

	// An individual adds theirs on their entry.
	own := withoutQuery(redirected(t, h, pathIn(t, dash, "/competitions/enter/"), url.Values{"gymnast": {"I"}, "discipline": {"tumbling"}, "level": {"Novice"}}))
	redirected(t, h, own+"/offer", url.Values{"judge-tumbling": {"1"}})
	if rec := upload(t, h, own+"/judging", map[string]string{"qualification": "gi-tumbling-judge-2"}, "certificate", pdf); rec.Code != http.StatusSeeOther {
		t.Fatalf("the individual adds theirs: %d", rec.Code)
	}
	if page := do(t, h, http.MethodGet, own, nil).Body.String(); !strings.Contains(page, "Gymnastics Ireland Level 2 Tumbling Judge") || !strings.Contains(page, "deleted with the competition") {
		t.Error("on their entry")
	}
	if page := do(t, h, http.MethodGet, officials, nil).Body.String(); strings.Count(page, "/judging/") != 2 {
		t.Errorf("the organiser sees both, the member's and the individual's: %d", strings.Count(page, "/judging/"))
	}
}
