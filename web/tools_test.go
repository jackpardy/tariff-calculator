package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// The routine tools share the competition pages' look under a slim banner:
// the builder's names the app as its heading, and each leads to the other
// tool. The builder no longer has the big hero or the footer.
func TestRoutineToolsHaveTheBanner(t *testing.T) {
	for _, path := range []string{"/", "/requirements", "/compare"} {
		rec := httptest.NewRecorder()
		routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		page := rec.Body.String()
		if !strings.Contains(page, `<header class="tool-banner">`) || !strings.Contains(page, `href="/requirements"`) {
			t.Errorf("%s has no banner leading to the other tool", path)
		}
		for _, old := range []string{`class="hero`, `class="footer"`} {
			if strings.Contains(page, old) {
				t.Errorf("%s still has %s", path, old)
			}
		}
	}
	if page := getPage(t, "/"); !strings.Contains(page, `<h1 class="tool-banner-name">Trampoline Tariff Calculator</h1>`) {
		t.Errorf("the builder's banner should be its heading")
	}
	// The built-in requirements aren't carried on every load: a copy comes from the server.
	if page := getPage(t, "/requirements"); strings.Contains(page, `id="builtin-sets"`) || len(page) > 150_000 {
		t.Errorf("the requirements page carries its built-in sets again (%d bytes)", len(page))
	}
}

// A card's head is a button that opens it; ↑, ↓ and Edit are at the right,
// and Remove beside them, a red ✕, with the tariff shown to one place.
func TestRoutineCardControls(t *testing.T) {
	html := postRoutine(t, `[{"rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck","backward":true}]`).Body.String()
	for _, want := range []string{
		`<button type="button" class="card-toggle" x-on:click="toggleExpanded(0, &#39;a&#39;)"`,
		`<span class="card-tariff">0.5</span>`,
		`aria-label="Move element 1 up"`,
		`class="button is-danger is-outlined card-remove" type="button" title="Remove" aria-label="Remove element 1" x-on:click="removeSkill(0, &#39;a&#39;)">✕</button>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("the card is missing %s", want)
		}
	}
	head := html[strings.Index(html, `class="card-head"`):strings.Index(html, `class="card-details"`)]
	if !strings.Contains(head, "removeSkill") {
		t.Errorf("Remove should be on the card's head, beside Edit, not in the opened card")
	}
}

// Removing a skill says so with Undo, which puts it back where it was; no
// question first.
func TestRemoveAndUndoInABrowser(t *testing.T) {
	ctx := browser(t)
	srv := httptest.NewServer(routes())
	defer srv.Close()

	var after, undone, message string
	step(t, ctx, "removing a skill and undoing it",
		chromedp.Navigate(srv.URL+"/"),
		chromedp.Evaluate(`localStorage.setItem('trampolineRoutines', `+"`"+sheetRoutine+"`"+`)`, nil),
		chromedp.Reload(),
		chromedp.WaitVisible(`#routine-skills .card-remove`, chromedp.ByQuery),
		chromedp.Click(`#routine-skills .card-remove`, chromedp.ByQuery),
		chromedp.WaitVisible(`.tool-toast-undo`, chromedp.ByQuery),
		chromedp.Text(`.tool-toast span`, &message, chromedp.ByQuery),
		chromedp.Evaluate(`JSON.parse(localStorage.getItem('trampolineRoutines')).routines[0].skills.map((s) => s.rotation + '/' + s.twist_distribution).join()`, &after),
		chromedp.Click(`.tool-toast-undo`, chromedp.ByQuery),
		chromedp.Poll(`JSON.parse(localStorage.getItem('trampolineRoutines')).routines[0].skills.length === 2`, nil, chromedp.WithPollingInterval(50*time.Millisecond)),
		chromedp.Evaluate(`JSON.parse(localStorage.getItem('trampolineRoutines')).routines[0].skills.map((s) => s.rotation + '/' + s.twist_distribution).join()`, &undone),
	)
	if after != "4/1" || !strings.HasPrefix(message, "Removed ") {
		t.Errorf("Remove should take the first skill out and say so: %q, %q", after, message)
	}
	if undone != "4/0,4/1" {
		t.Errorf("Undo should put it back first: %q", undone)
	}
}

// Levels mode starts a level from one chooser (with its search), as Check
// against chooses requirements, not a search box and a list.
func TestNewLevelChooserInABrowser(t *testing.T) {
	ctx := browser(t)
	srv := httptest.NewServer(routes())
	defer srv.Close()

	var level string
	step(t, ctx, "starting a level by a search",
		chromedp.Navigate(srv.URL+"/"),
		chromedp.WaitReady(`.builder-mode`, chromedp.ByQuery),
		chromedp.Evaluate(`Alpine.$data(document.querySelector('.builder')).setMode('levels')`, nil),
		chromedp.WaitVisible(`.levels-bar .chooser-button`, chromedp.ByQuery),
		chromedp.Click(`.levels-bar .chooser-button`, chromedp.ByQuery),
		chromedp.WaitVisible(`.levels-bar .chooser-search`, chromedp.ByQuery),
		chromedp.SendKeys(`.levels-bar .chooser-search`, "bucs l7", chromedp.ByQuery),
		// Alpine shows and hides the options a moment after the search changes.
		chromedp.Poll(`document.querySelector('.levels-bar .chooser-search').value === 'bucs l7' && document.querySelector('.levels-bar .chooser-option:not([style*=none])')?.dataset.ref === 'builtin-level:bucs-l7'`, nil, chromedp.WithPollingInterval(50*time.Millisecond)),
		chromedp.SendKeys(`.levels-bar .chooser-search`, "\r", chromedp.ByQuery),
		chromedp.Poll(`!!JSON.parse(localStorage.getItem('trampolineLevelEntries') || 'null')?.entries?.length`, nil, chromedp.WithPollingInterval(50*time.Millisecond)),
		chromedp.Evaluate(`JSON.parse(localStorage.getItem('trampolineLevelEntries')).entries[0].level`, &level),
	)
	if level != "builtin-level:bucs-l7" {
		t.Errorf("Enter should start the level found: %q", level)
	}
}

// Deleting on the Requirements page asks on the row, not in a browser
// pop-up: Keep it leaves it, Delete deletes it.
func TestDeleteAsksOnTheRowInABrowser(t *testing.T) {
	ctx := browser(t)
	srv := httptest.NewServer(routes())
	defer srv.Close()

	var asked string
	var kept, left int
	step(t, ctx, "deleting requirements",
		chromedp.Navigate(srv.URL+"/requirements"),
		chromedp.Evaluate(`localStorage.setItem('trampolineRequirementSets', JSON.stringify([{id: 'set-1', set: {format: 1, name: 'Ours', rules: []}}])); window.confirm = () => { throw new Error('confirm() asked'); }`, nil),
		chromedp.Reload(),
		chromedp.WaitVisible(`.set-item .routine-menu summary`, chromedp.ByQuery),
		chromedp.Evaluate(`window.confirm = () => { throw new Error('confirm() asked'); }`, nil),
		chromedp.Click(`.set-item .routine-menu summary`, chromedp.ByQuery),
		chromedp.Evaluate(`[...document.querySelectorAll('.set-item .routine-menu-panel button')].find((b) => b.textContent.trim() === 'Delete').click()`, nil),
		chromedp.WaitVisible(`.set-ask`, chromedp.ByQuery),
		chromedp.Text(`.set-ask-text`, &asked, chromedp.ByQuery),
		chromedp.Evaluate(`[...document.querySelectorAll('.set-ask button')].find((b) => b.textContent.trim() === 'Keep it').click()`, nil),
		chromedp.WaitNotVisible(`.set-ask`, chromedp.ByQuery),
		chromedp.Evaluate(`JSON.parse(localStorage.getItem('trampolineRequirementSets')).length`, &kept),
		chromedp.Click(`.set-item .routine-menu summary`, chromedp.ByQuery),
		chromedp.Evaluate(`[...document.querySelectorAll('.set-item .routine-menu-panel button')].find((b) => b.textContent.trim() === 'Delete').click()`, nil),
		chromedp.WaitVisible(`.set-ask`, chromedp.ByQuery),
		chromedp.Evaluate(`[...document.querySelectorAll('.set-ask button')].find((b) => b.textContent.trim() === 'Delete').click()`, nil),
		chromedp.Poll(`JSON.parse(localStorage.getItem('trampolineRequirementSets')).length === 0`, nil, chromedp.WithPollingInterval(50*time.Millisecond)),
		chromedp.Evaluate(`JSON.parse(localStorage.getItem('trampolineRequirementSets')).length`, &left),
	)
	if !strings.HasPrefix(asked, "Delete Ours?") || kept != 1 || left != 0 {
		t.Errorf("asked %q; kept %d, then %d left", asked, kept, left)
	}
}

// Side by side, only the second routine's differing skills are shaded, and a
// checkbox takes the shading away.
func TestSideBySideShadesTheSecondInABrowser(t *testing.T) {
	ctx := browser(t)
	srv := httptest.NewServer(routes())
	defer srv.Close()

	const two = `{"current":"r1","beside":"r2","routines":[` +
		`{"id":"r1","name":"First","skills":[{"rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck","backward":true},{"rotation":0,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck"}]},` +
		`{"id":"r2","name":"Second","skills":[{"rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck","backward":true},{"rotation":0,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Pike"}]}]}`
	const shaded = `(id) => [...document.querySelectorAll('#' + id + ' .routine-skill')].map((c) => getComputedStyle(c).backgroundColor === 'rgb(255, 246, 217)' ? 'S' : '-').join('')`
	var on, off, again string
	step(t, ctx, "two routines side by side, then the shading off and on",
		chromedp.EmulateViewport(1300, 900),
		chromedp.Navigate(srv.URL+"/"),
		chromedp.Evaluate(`localStorage.setItem('trampolineRoutines', `+"`"+two+"`"+`)`, nil),
		chromedp.Reload(),
		chromedp.WaitVisible(`#routine-view-b .routine-skill`, chromedp.ByQuery),
		chromedp.Sleep(200*time.Millisecond),
		chromedp.Evaluate(`(`+shaded+`)('routine-view') + '|' + (`+shaded+`)('routine-view-b')`, &on),
		chromedp.Click(`.compare-shade input`, chromedp.ByQuery),
		chromedp.Sleep(200*time.Millisecond),
		chromedp.Evaluate(`(`+shaded+`)('routine-view') + '|' + (`+shaded+`)('routine-view-b')`, &off),
		chromedp.Reload(),
		chromedp.WaitVisible(`#routine-view-b .routine-skill`, chromedp.ByQuery),
		chromedp.Sleep(200*time.Millisecond),
		chromedp.Evaluate(`(`+shaded+`)('routine-view') + '|' + (`+shaded+`)('routine-view-b')`, &again),
	)
	if on != "--|-S" {
		t.Errorf("only the second routine's second skill should be shaded: %s", on)
	}
	if off != "--|--" || again != "--|--" {
		t.Errorf("unticked, nothing is shaded, after a reload too: %s, then %s", off, again)
	}
}

// On a phone the Feedback tab at the right edge covers nothing: a card's ✕
// ends where the tab starts, the banner still reaches the edge, and nothing
// scrolls sideways.
func TestFeedbackTabClearOfThePageOnAPhoneInABrowser(t *testing.T) {
	ctx := browser(t)
	s := newTestServer(t) // the Feedback tab needs the storage it's kept in
	srv := httptest.NewServer(s.h)
	defer srv.Close()

	var got struct {
		Tab, Remove, Banner, Width float64
		Sideways                   bool
	}
	step(t, ctx, "the builder on a phone",
		chromedp.EmulateViewport(375, 812),
		chromedp.Navigate(srv.URL+"/"),
		chromedp.Evaluate(`localStorage.setItem('trampolineRoutines', `+"`"+sheetRoutine+"`"+`)`, nil),
		chromedp.Reload(),
		chromedp.WaitVisible(`#routine-skills .card-remove`, chromedp.ByQuery),
		chromedp.Evaluate(`({
			Tab: document.querySelector('.site-feedback-tab').getBoundingClientRect().left,
			Remove: Math.max(...[...document.querySelectorAll('#routine-skills .card-remove')].map((b) => b.getBoundingClientRect().right)),
			Banner: document.querySelector('.tool-banner').getBoundingClientRect().right,
			Width: innerWidth,
			Sideways: document.documentElement.scrollWidth > innerWidth,
		})`, &got),
	)
	if got.Remove > got.Tab {
		t.Errorf("the Feedback tab (from %v px) covers a card's ✕ (to %v px)", got.Tab, got.Remove)
	}
	if got.Banner != got.Width || got.Sideways {
		t.Errorf("the banner should reach the edge (%v of %v px), with nothing scrolling sideways (%v)", got.Banner, got.Width, got.Sideways)
	}
}
