package web

import (
	"encoding/json"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/chromedp/chromedp"

	"tariffCalculator/competitions"
)

// listRows are the search name and filter values of each row of the list
// with element id id on a page, by name.
func listRows(t *testing.T, page, id string) map[string]map[string][]string {
	t.Helper()
	i := strings.Index(page, `id="`+id+`"`)
	if i < 0 {
		t.Fatalf("no list %q on the page", id)
	}
	out := map[string]map[string][]string{}
	for _, m := range regexp.MustCompile(`data-list-row="" data-name="([^"]*)"(?: data-values="([^"]*)")?`).FindAllStringSubmatch(page[i:], -1) {
		values := map[string][]string{}
		if m[2] != "" {
			if err := json.Unmarshal([]byte(html.UnescapeString(m[2])), &values); err != nil {
				t.Fatalf("row %q's values: %v", m[1], err)
			}
		}
		out[html.UnescapeString(m[1])] = values
	}
	return out
}

func TestOfficialsListFilters(t *testing.T) {
	h := competitionServer(t)
	form := newCompetition()
	form.Set("tumbling", "Novice\nElite")
	admin := created(t, h, form)
	officials := admin + "/officials"
	if page := do(t, h, http.MethodGet, officials, nil).Body.String(); strings.Contains(page, "data-list-tools") {
		t.Error("no search on an empty list")
	}
	redirected(t, h, officials+"/add", url.Values{"name": {"Seán Ó Briain"}, "club": {"UCD"},
		"judge-trampoline": {"1"}, "execution-trampoline": {"BUCS L3"}, "difficulty-trampoline": {"BUCS L3"}, "chair-trampoline": {"BUCS L3"}, "judge-tumbling": {"1"}})
	redirected(t, h, officials+"/add", url.Values{"name": {"Ann O'Neill / Jr?"}, "recorder": {"1"}, "marshal": {"1"}})
	page := do(t, h, http.MethodGet, officials, nil).Body.String()

	for _, want := range []string{`data-list-tools="officials"`, `data-noun="people"`, `placeholder="Find a name"`, `data-list-filter="club"`,
		`data-list-filter="can"`, `data-list-filter="judges" data-at-least`, `data-list-filter="qualified"`, `<option value="-">No club</option>`,
		`<optgroup label="Trampoline"><option value="trampoline:0">Any trampoline level</option><option value="trampoline:1">BUCS L3</option><option value="trampoline:2">FIG AG3 (17–21)</option></optgroup>`,
		`<optgroup label="Tumbling"><option value="tumbling:0">Any tumbling level</option><option value="tumbling:1">Novice</option><option value="tumbling:2">Elite</option></optgroup>`} {
		if !strings.Contains(page, want) {
			t.Errorf("the tools have %s", want)
		}
	}
	if !regexp.MustCompile(`<section class="comp-list mt-5"><h2 class="title is-5 mb-3">People <span[^>]*>· 2</span></h2>`).MatchString(page) {
		t.Error("the list is under its heading and count")
	}

	rows := listRows(t, page, "officials")
	sean := rows["Seán Ó Briain"]
	if !slices.Equal(sean["club"], []string{"UCD"}) || !slices.Equal(sean["can"], []string{"judge", "chair"}) || !slices.Equal(sean["qualified"], []string{"no"}) {
		t.Errorf("Seán's values: %v", sean)
	}
	// BUCS L3 is the first trampoline level, easiest first; tumbling up to
	// no level is every level, of two.
	if !slices.Equal(sean["judges"], []string{"trampoline:1", "tumbling:2"}) {
		t.Errorf("what Seán judges: %v", sean["judges"])
	}
	ann := rows["Ann O'Neill / Jr?"]
	if !slices.Equal(ann["club"], []string{"-"}) || !slices.Equal(ann["can"], []string{"recorder", "marshal"}) || ann["judges"] != nil {
		t.Errorf("Ann's values: %v", ann)
	}

	// Marking someone qualified changes their value.
	id := regexp.MustCompile(regexp.QuoteMeta(officials) + `/([^/"]+)/qualified`).FindStringSubmatch(page)[1]
	redirected(t, h, officials+"/"+id+"/qualified", url.Values{"on": {"1"}})
	rows = listRows(t, do(t, h, http.MethodGet, officials, nil).Body.String(), "officials")
	qualified := 0
	for _, r := range rows {
		if slices.Equal(r["qualified"], []string{"yes"}) {
			qualified++
		}
	}
	if qualified != 1 {
		t.Errorf("one qualified, not %d", qualified)
	}
}

func TestClubFilter(t *testing.T) {
	f := clubFilter([]string{"ucc", "UCD", "", "UCD", "ATU Sligo", "Athlone", "Éire Óg"})
	var got []string
	for _, o := range f.Options {
		got = append(got, o.Value+"="+o.Label)
	}
	if want := []string{"Athlone=Athlone", "ATU Sligo=ATU Sligo", "Éire Óg=Éire Óg", "ucc=ucc", "UCD=UCD", "-=No club"}; !slices.Equal(got, want) {
		t.Errorf("clubs once each, ignoring case and accents in order, then no club: %v", got)
	}
	if f := clubFilter([]string{"UCD"}); len(f.Options) != 1 {
		t.Error("no \"No club\" when everyone has one")
	}
}

// A list's search in the browser matches names as the server's does: only
// their letters count, so "marykate" finds Mary Kate and "eabha" finds Éabha.
func TestListSearchInABrowser(t *testing.T) {
	ctx := browser(t)
	s := newTestServer(t, atTime(dayBefore))
	admin := created(t, s.h, newCompetition())
	club := aClub(t, s.h, admin, "UCD", false, "Mary Kate Byrne", "Éabha Ní Bhriain", "Tadhg Ryan")
	srv := httptest.NewServer(s.h)
	defer srv.Close()

	shown := func(search string) string {
		t.Helper()
		var names string
		step(t, ctx, "searching for "+search,
			chromedp.Evaluate(`{const s = document.querySelector('[data-list-search]'); s.value = `+strconv.Quote(search)+`; s.dispatchEvent(new Event('input', {bubbles: true}))}`, nil),
			chromedp.Evaluate(`[...document.querySelectorAll('#members [data-list-row]')].filter((r) => !r.hidden).map((r) => r.dataset.name).join(', ')`, &names),
		)
		return names
	}
	step(t, ctx, "opening the members", chromedp.Navigate(srv.URL+club.page+"/members"), chromedp.WaitVisible(`[data-list-search]`, chromedp.ByQuery))
	for _, c := range []struct{ search, want string }{
		{"marykate", "Mary Kate Byrne"},
		{"mary kate", "Mary Kate Byrne"},
		{"eabha", "Éabha Ní Bhriain"},
		{"ni bhriain", "Éabha Ní Bhriain"},
		{"", "Éabha Ní Bhriain, Mary Kate Byrne, Tadhg Ryan"}, // by name, ignoring accents
	} {
		if got := shown(c.search); got != c.want {
			t.Errorf("searching %q shows %q, want %q", c.search, got, c.want)
		}
	}
}

// An offer to judge tumbling or DMT with no role, as offers from before
// levels were by role read, gives no seat there: it isn't "Judge" under Can
// do, nor found under Can judge for that discipline.
func TestOfferWithNoSeat(t *testing.T) {
	c := competitions.Competition{Tumbling: []string{"Novice", "Elite"}, DMT: []string{"Novice"}}
	tumbling := competitions.Offer{Judge: map[string]competitions.JudgeOffer{competitions.Tumbling: {}, competitions.DMT: {}}, Recorder: true}
	if got := canDo(tumbling); !slices.Equal(got, []string{canRecorder}) {
		t.Errorf("can do %v", got)
	}
	if got := judges(c, tumbling); len(got) != 0 {
		t.Errorf("judges %v", got)
	}
	tumbling.Judge[competitions.Tumbling] = competitions.JudgeOffer{UpTo: map[string]string{competitions.RoleExecution: "Novice"}}
	if got := judges(c, tumbling); !slices.Equal(got, []string{offerKey(competitions.Tumbling) + ":1"}) || !slices.Contains(canDo(tumbling), canJudge) {
		t.Errorf("with a role: judges %v, can do %v", got, canDo(tumbling))
	}
}
