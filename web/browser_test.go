package web

import (
	"context"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// Browser tests load pages in headless Chrome, so the JavaScript that builds a
// page's requests runs too: a page can render fine on the server and still be
// broken in a browser (the tariff sheet's hx-vals once was). They need Chrome
// or Chromium, which CI's Ubuntu runners have; without one they're skipped,
// except in CI (the CI environment variable set), where they fail.

// findChrome is the path to a Chrome or Chromium to run, or "".
func findChrome() string {
	if path := os.Getenv("CHROME_PATH"); path != "" {
		return path
	}
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "headless-shell"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	for _, path := range []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
	} {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// browser starts headless Chrome for a test, or skips the test without one
// (fails it, in CI).
func browser(t *testing.T) context.Context {
	t.Helper()
	chrome := findChrome()
	switch {
	case chrome == "" && os.Getenv("CI") != "":
		t.Fatal("no Chrome or Chromium to run browser tests in CI (set CHROME_PATH)")
	case chrome == "":
		t.Skip("no Chrome or Chromium to run browser tests (set CHROME_PATH)")
	}
	// Chrome can take longer than chromedp's 20 seconds to start on a busy CI
	// runner, which failed the tests now and then; give it a minute.
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(chrome), chromedp.NoSandbox, chromedp.WSURLReadTimeout(time.Minute))
	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancelCtx := chromedp.NewContext(alloc)
	ctx, cancelTimeout := context.WithTimeout(ctx, 60*time.Second)
	t.Cleanup(func() { cancelTimeout(); cancelCtx(); cancelAlloc() })
	// Start it on this context, so a step's own timeout ending doesn't close
	// the browser with it.
	if err := chromedp.Run(ctx); err != nil {
		t.Fatalf("starting Chrome: %v", err)
	}
	return ctx
}

// stepTimeout is how long one step of a browser test may take.
const stepTimeout = 15 * time.Second

// step runs a browser test's step with its own timeout, failing the test
// with the step's name if it doesn't finish.
func step(t *testing.T, ctx context.Context, name string, actions ...chromedp.Action) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, stepTimeout)
	defer cancel()
	if err := chromedp.Run(ctx, actions...); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}

// sheetRoutine is the routine saved in the browser for the sheet to show: a
// tuck back (0.5) and a tuck barani (0.6).
const sheetRoutine = `{"current":"r1","routines":[{"id":"r1","name":"Sheet test","skills":[
	{"rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck","backward":true},
	{"rotation":4,"twist_distribution":[1],"takeoff_position":"Feet","shape":"Tuck"}]}]}`

// The tariff sheet page loads the routine saved in the browser and shows it:
// the page's script must post the routine for the sheet to appear at all.
func TestTariffSheetLoadsInABrowser(t *testing.T) {
	ctx := browser(t)
	srv := httptest.NewServer(routes())
	defer srv.Close()

	// Save the routine for this origin, then load the sheet with it.
	step(t, ctx, "opening the sheet with a routine saved",
		chromedp.Navigate(srv.URL+"/tariff-sheet"),
		chromedp.Evaluate(`localStorage.setItem('trampolineRoutines', `+"`"+sheetRoutine+"`"+`)`, nil),
		chromedp.Reload(),
	)
	// The sheet should appear in a moment. If it doesn't, say what the page
	// shows instead (still loading, or why it couldn't build the sheet).
	wait, cancel := context.WithTimeout(ctx, 10*time.Second)
	err := chromedp.Run(wait, chromedp.WaitVisible(`#tariff-sheet tbody tr`, chromedp.ByQuery))
	cancel()
	if err != nil {
		var shown string
		read, cancel := context.WithTimeout(ctx, 5*time.Second)
		_ = chromedp.Run(read, chromedp.Text(`body`, &shown, chromedp.ByQuery))
		cancel()
		t.Fatalf("the tariff sheet didn't load: %v\npage: %.300s", err, strings.TrimSpace(shown))
	}

	var rows int
	var total, text string
	step(t, ctx, "reading the sheet",
		chromedp.Evaluate(`document.querySelectorAll('#tariff-sheet tbody tr').length`, &rows),
		chromedp.Text(`#tariff-sheet tfoot td.diff`, &total, chromedp.ByQuery),
		chromedp.Text(`#tariff-sheet`, &text, chromedp.ByQuery),
	)
	if rows != 10 {
		t.Errorf("the sheet has %d rows, want a full exercise of 10", rows)
	}
	if strings.TrimSpace(total) != "1.1" {
		t.Errorf("total difficulty %q, want 1.1 (tuck back 0.5 + tuck barani 0.6)", total)
	}
	for _, want := range []string{"Tuck Back", "Tuck Barani"} {
		if !strings.Contains(text, want) {
			t.Errorf("the sheet doesn't show %s", want)
		}
	}
}

// The builder's Check against chooser opens on its button, finds
// requirements by a search, takes the first match on Enter, shows it on the
// button, checks the routine against it, and lists it under Used lately.
func TestCheckAgainstChooserInABrowser(t *testing.T) {
	ctx := browser(t)
	srv := httptest.NewServer(routes())
	defer srv.Close()

	var chosen, saved, label, recent string
	step(t, ctx, "choosing requirements by a search",
		chromedp.Navigate(srv.URL+"/"),
		chromedp.WaitReady(`.requirement-picker .chooser-button`, chromedp.ByQuery),
		chromedp.Click(`.requirement-picker .chooser-button`, chromedp.ByQuery),
		chromedp.WaitVisible(`.requirement-picker .chooser-search`, chromedp.ByQuery),
		chromedp.SendKeys(`.requirement-picker .chooser-search`, "bucs l7", chromedp.ByQuery),
		// Alpine shows and hides the options a moment after the search changes.
		chromedp.Poll(`!document.querySelector('.requirement-picker .chooser-option[data-ref="builtin:bucs-fig"]')?.offsetParent && !!document.querySelector('.requirement-picker .chooser-option[data-ref="builtin:bucs-l7-second"]')?.offsetParent`, nil, chromedp.WithPollingInterval(50*time.Millisecond)),
		chromedp.Evaluate(`[...document.querySelectorAll('.requirement-picker .chooser-option[data-ref]')].filter((o) => o.offsetParent).map((o) => o.dataset.ref).join()`, &chosen),
		chromedp.SendKeys(`.requirement-picker .chooser-search`, "\r", chromedp.ByQuery),
		chromedp.WaitNotVisible(`.requirement-picker .chooser-panel`, chromedp.ByQuery),
		chromedp.Text(`.requirement-picker .chooser-chosen`, &label, chromedp.ByQuery),
		chromedp.Evaluate(`JSON.parse(localStorage.getItem('trampolineRoutines')).routines[0].requirements`, &saved),
		chromedp.Evaluate(`JSON.parse(localStorage.getItem('recentRequirements')).check.join()`, &recent),
	)
	if chosen != "builtin:bucs-l7-second" {
		t.Errorf("searching bucs l7 should leave BUCS L7's voluntary alone, got %q", chosen)
	}
	if saved != "builtin:bucs-l7-second" || recent != saved {
		t.Errorf("Enter should check the routine against the match and remember it: saved %q, recent %q", saved, recent)
	}
	if !strings.Contains(label, "BUCS L7") {
		t.Errorf("the button should show what's chosen, got %q", label)
	}
}

// Requirements written beside the routine in the builder: Copy and change
// opens a copy of the built-in set chosen, each rule saying whether the
// routine meets it; Save keeps it as one of yours and checks the routine
// against it.
func TestWriteRequirementsInTheBuilderInABrowser(t *testing.T) {
	ctx := browser(t)
	srv := httptest.NewServer(routes())
	defer srv.Close()

	var chips, name, saved, setName string
	step(t, ctx, "copying and changing the requirements checked against",
		chromedp.Navigate(srv.URL+"/"),
		chromedp.Evaluate(`localStorage.setItem('trampolineRoutines', `+"`"+`{"current":"r1","routines":[{"id":"r1","name":"Test","requirements":"builtin:bucs-l7-second","skills":[
			{"rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck","backward":true}]}]}`+"`"+`)`, nil),
		chromedp.Reload(),
		chromedp.WaitReady(`.requirement-picker .chooser-button`, chromedp.ByQuery),
		chromedp.Evaluate(`[...document.querySelectorAll('.requirement-picker button')].find((b) => b.textContent.trim() === 'Copy and change').click()`, nil),
		chromedp.WaitVisible(`#requirements-writer-editor .rule-card`, chromedp.ByQuery),
		chromedp.Evaluate(`[...document.querySelectorAll('#requirements-writer-editor .rule-card-title .chip')].map((c) => c.textContent.trim()).join()`, &chips),
		chromedp.Value(`#requirements-writer-editor [name=name]`, &name, chromedp.ByQuery),
		chromedp.SetValue(`#requirements-writer-editor [name=name]`, "Our L7", chromedp.ByQuery),
		chromedp.Evaluate(`Alpine.$data(document.querySelector('.requirement-picker')).saveWriting()`, nil),
		chromedp.WaitNotVisible(`.requirements-writer`, chromedp.ByQuery),
		chromedp.Evaluate(`JSON.parse(localStorage.getItem('trampolineRoutines')).routines[0].requirements`, &saved),
		chromedp.Evaluate(`JSON.parse(localStorage.getItem('trampolineRequirementSets')).find((s) => s.id === JSON.parse(localStorage.getItem('trampolineRoutines')).routines[0].requirements)?.set.name || ''`, &setName),
	)
	if chips != "!Not met,!Not met,!Not met" { // one skill: too few, too easy, and not the elements allowed
		t.Errorf("each rule says whether the routine meets it: %q", chips)
	}
	if name != "Copy of BUCS L7 · second exercise" {
		t.Errorf("a copy, named so: %q", name)
	}
	if strings.HasPrefix(saved, "builtin:") || setName != "Our L7" {
		t.Errorf("saved as one of yours and checked against: %q %q", saved, setName)
	}
}

// A rule's skill list is searchable: typing finds the skill, and Enter picks
// it, as choosing from the list does.
func TestSkillListSearchInABrowser(t *testing.T) {
	ctx := browser(t)
	srv := httptest.NewServer(routes())
	defer srv.Close()

	var title string
	step(t, ctx, "searching a rule's skill list",
		chromedp.Navigate(srv.URL+"/"),
		chromedp.WaitReady(`.requirement-picker .chooser-button`, chromedp.ByQuery),
		chromedp.Evaluate(`Alpine.$data(document.querySelector('.requirement-picker')).writeRequirements('')`, nil),
		chromedp.WaitVisible(`#requirements-writer-editor #add-rule`, chromedp.ByQuery),
		chromedp.Sleep(300*time.Millisecond), // htmx sets the editor up just after it's swapped in
		chromedp.SetValue(`#requirements-writer-editor #add-rule`, "include", chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('#requirements-writer-editor #add-rule').dispatchEvent(new Event('change', {bubbles: true}))`, nil),
	)
	step(t, ctx, "a rule's skill list with its search",
		chromedp.WaitVisible(`#requirements-writer-editor .rule-card .select-search-input`, chromedp.ByQuery),
	)
	step(t, ctx, "typing a skill",
		chromedp.SendKeys(`#requirements-writer-editor .rule-card .select-search-input`, "barani", chromedp.ByQuery),
		chromedp.WaitVisible(`#requirements-writer-editor .select-search-option`, chromedp.ByQuery),
	)
	step(t, ctx, "picking it with Enter",
		chromedp.SendKeys(`#requirements-writer-editor .rule-card .select-search-input`, "\r", chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('#requirements-writer-editor .rule-card-title')?.textContent.toLowerCase().includes('barani')`, nil, chromedp.WithPollingInterval(50*time.Millisecond)),
		chromedp.Text(`#requirements-writer-editor .rule-card-title`, &title, chromedp.ByQuery),
	)
	if !strings.Contains(strings.ToLower(title), "barani") {
		t.Errorf("the barani is picked: %q", title)
	}
}

// Changing a field of the requirements editor redraws it without moving the
// page (the browser's scroll anchoring once threw it to the bottom).
func TestEditorKeepsItsPlaceInABrowser(t *testing.T) {
	ctx := browser(t)
	srv := httptest.NewServer(routes())
	defer srv.Close()

	var before, after float64
	step(t, ctx, "changing a field in the editor",
		chromedp.EmulateViewport(1000, 700),
		chromedp.Navigate(srv.URL+"/requirements"),
		chromedp.WaitReady(`[x-data]`, chromedp.ByQuery),
		chromedp.Sleep(300*time.Millisecond),
		chromedp.Evaluate(`Alpine.$data(document.querySelector('[x-data]')).newSet()`, nil),
		chromedp.WaitVisible(`#set-editor #add-rule`, chromedp.ByQuery),
		chromedp.Sleep(300*time.Millisecond),
		chromedp.Evaluate(`(() => { const a = document.getElementById('add-rule'); a.value = 'linked'; a.dispatchEvent(new Event('change', {bubbles: true})); })()`, nil),
		chromedp.WaitVisible(`#set-editor [name="r0.max"]`, chromedp.ByQuery),
		chromedp.Sleep(300*time.Millisecond),
		chromedp.Evaluate(`(() => { const f = document.querySelector('[name="r0.max"]'); f.scrollIntoView({block: 'center'}); return window.scrollY; })()`, &before),
		chromedp.Evaluate(`(() => { const f = document.querySelector('[name="r0.max"]'); f.focus(); f.value = '2'; f.dispatchEvent(new Event('change', {bubbles: true})); })()`, nil),
		chromedp.Poll(`document.querySelector('.rule-card-title')?.textContent.includes('At most 2')`, nil, chromedp.WithPollingInterval(50*time.Millisecond)),
		chromedp.Sleep(200*time.Millisecond),
		chromedp.Evaluate(`window.scrollY`, &after),
	)
	if before == 0 || after != before {
		t.Errorf("the page moved from %v to %v", before, after)
	}
}

// In a level's editor, requirements are written for an exercise right
// there: saved into the level and chosen for it, kept out of Your
// requirements unless ticked.
func TestWriteRequirementsInALevelInABrowser(t *testing.T) {
	ctx := browser(t)
	srv := httptest.NewServer(routes())
	defer srv.Close()

	var chosen, listed bool
	var name string
	var inLevel bool
	step(t, ctx, "writing requirements for a level's exercise",
		chromedp.Navigate(srv.URL+"/requirements"),
		chromedp.WaitReady(`[x-data]`, chromedp.ByQuery),
		chromedp.Sleep(300*time.Millisecond),
		chromedp.Evaluate(`Alpine.$data(document.querySelector('[x-data]')).newLevel()`, nil),
		chromedp.WaitVisible(`#level-editor input[name=structure][value=voluntary]`, chromedp.ByQuery),
		chromedp.Sleep(300*time.Millisecond),
		chromedp.Evaluate(`(() => {
			document.getElementById('level-name').value = 'Club open';
			const v = document.querySelector('#level-editor input[name=structure][value=voluntary]');
			v.checked = true;
			v.dispatchEvent(new Event('change', {bubbles: true}));
		})()`, nil),
		chromedp.Poll(`!!document.querySelector('#level-editor [name="first.0"]')`, nil, chromedp.WithPollingInterval(50*time.Millisecond)),
		chromedp.Sleep(300*time.Millisecond),
		chromedp.Evaluate(`[...document.querySelectorAll('#level-editor button')].find((b) => b.textContent.includes('Write new')).click()`, nil),
		chromedp.Poll(`!!document.querySelector('#level-write-area #set-editor')`, nil, chromedp.WithPollingInterval(50*time.Millisecond)),
		chromedp.Evaluate(`document.querySelector('#level-write-area [name=name]').value`, &name),
		chromedp.Evaluate(`Alpine.$data(document.querySelector('[x-data]')).saveForLevel()`, nil),
		chromedp.Poll(`!Alpine.$data(document.querySelector('[x-data]')).levelWriting`, nil, chromedp.WithPollingInterval(50*time.Millisecond)),
		chromedp.Poll(`(() => { const s = JSON.parse(localStorage.getItem('trampolineRequirementSets') || '[]')[0]; return !!s && document.querySelector('#level-editor [name="first.0"]')?.value === s.id; })()`, nil, chromedp.WithPollingInterval(50*time.Millisecond)),
		chromedp.Evaluate(`(() => { const s = JSON.parse(localStorage.getItem('trampolineRequirementSets'))[0]; return document.querySelector('#level-editor [name="first.0"]').value === s.id; })()`, &chosen),
		chromedp.Evaluate(`JSON.parse(localStorage.getItem('trampolineRequirementSets'))[0].inLevel === true`, &inLevel),
		chromedp.Evaluate(`Alpine.$data(document.querySelector('[x-data]')).requirementSets().length > 0`, &listed),
	)
	if name != "Club open · both exercises" {
		t.Errorf("named after the level: %q", name)
	}
	if !chosen || !inLevel || listed {
		t.Errorf("chosen for the exercise %v, the level's own %v, in Your requirements %v", chosen, inLevel, listed)
	}
}

// A set routine is built on the requirements page, skill by skill, with the
// builder's search and skill card, and saved with the rest.
func TestBuildASetRoutineOnTheRequirementsPageInABrowser(t *testing.T) {
	ctx := browser(t)
	srv := httptest.NewServer(routes())
	defer srv.Close()

	var tabs int
	var label, saved string
	// Steps of their own, each with its own time: CI's runners are slower.
	step(t, ctx, "opening + New set routine",
		chromedp.Navigate(srv.URL+"/requirements"),
		chromedp.WaitReady(`[x-data]`, chromedp.ByQuery),
		chromedp.Poll(`!!window.Alpine && !!Alpine.$data(document.querySelector('[x-data]'))`, nil, chromedp.WithPollingInterval(50*time.Millisecond)),
		chromedp.Evaluate(`Alpine.$data(document.querySelector('[x-data]')).makingSetRoutine = true`, nil),
	)
	step(t, ctx, "the skill form loading",
		chromedp.Poll(`!!document.getElementById('skill-search') && !!document.getElementById('add-btn')`, nil, chromedp.WithPollingInterval(50*time.Millisecond)),
		chromedp.Evaluate(`document.querySelectorAll('.set-routine-maker .picker-tab').length`, &tabs),
		// textContent: innerText is "" for a moment while the form shows.
		chromedp.Evaluate(`document.getElementById('add-btn').textContent.trim()`, &label),
	)
	// The search too is wired up just after the form is shown: it searches
	// again until there are results.
	step(t, ctx, "finding Seat Drop",
		chromedp.Evaluate(`Alpine.$data(document.querySelector('.set-routine-maker')).name = 'Club L1'`, nil),
		chromedp.Poll(`(() => {
			if (document.querySelector('#search-results .picker-skill')) { return true; }
			// The search only runs when the text changes: each try differs by a
			// space it ignores.
			const i = document.getElementById('skill-search');
			i.value = i.value === 'seat drop' ? 'seat drop ' : 'seat drop';
			i.dispatchEvent(new Event('input', {bubbles: true}));
			return false;
		})()`, nil, chromedp.WithPollingInterval(500*time.Millisecond)),
	)
	// htmx wires a result up just after it's shown: a click before then does
	// nothing, so it clicks again until the card has it.
	step(t, ctx, "loading it onto the card",
		chromedp.Poll(`(() => {
			if (document.querySelector('.skill-card-name')?.textContent.includes('Seat Drop')) { return true; }
			document.querySelector('#search-results .picker-skill')?.click();
			return false;
		})()`, nil, chromedp.WithPollingInterval(300*time.Millisecond)),
	)
	step(t, ctx, "adding it",
		chromedp.Evaluate(`Alpine.$data(document.querySelector('.set-routine-maker')).addFromForm()`, nil),
		chromedp.Poll(`Alpine.$data(document.querySelector('.set-routine-maker')).skills.length === 1`, nil, chromedp.WithPollingInterval(50*time.Millisecond)),
	)
	step(t, ctx, "saving the set routine",
		chromedp.Evaluate(`Alpine.$data(document.querySelector('.set-routine-maker')).save()`, nil),
		chromedp.Poll(`(JSON.parse(localStorage.getItem('trampolineRequirementSets') || '[]')).length === 1`, nil, chromedp.WithPollingInterval(50*time.Millisecond)),
		chromedp.Evaluate(`(() => { const s = JSON.parse(localStorage.getItem('trampolineRequirementSets'))[0].set; return s.name + ': ' + s.rules.find((r) => r.type === 'sequence').sequence.map((m) => m.label).join(', '); })()`, &saved),
	)
	if tabs != 0 || !strings.Contains(label, "Add to the set routine") {
		t.Errorf("a cut-down form, adding to the set routine: %d tabs, %q", tabs, label)
	}
	if saved != "Club L1: Seat Drop" {
		t.Errorf("saved: %q", saved)
	}
}

// A card is dragged into a new order by its head, which is a button (it
// opens the card), so the drag mustn't ignore it as it does ↑, ↓ and Edit.
func TestCardsDragByTheirHeadInABrowser(t *testing.T) {
	ctx := browser(t)
	srv := httptest.NewServer(routes())
	defer srv.Close()

	const routine = `{"current":"r1","routines":[{"id":"r1","name":"Test","skills":[{"rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck","backward":true},{"rotation":0,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck"}]}]}`
	var got struct{ Head, Up bool }
	step(t, ctx, "opening the builder with two skills",
		chromedp.Navigate(srv.URL+"/"),
		chromedp.Evaluate(`localStorage.setItem('trampolineRoutines', `+"`"+routine+"`"+`)`, nil),
		chromedp.Reload(),
		chromedp.WaitReady(`#routine-skills .card-toggle`, chromedp.ByQuery),
		chromedp.Poll(`!!Sortable.get(document.getElementById('routine-skills'))`, nil, chromedp.WithPollingInterval(50*time.Millisecond)),
		chromedp.Evaluate(`(() => {
			const list = document.getElementById('routine-skills');
			const filter = Sortable.get(list).options.filter;
			const card = list.querySelector('.routine-skill-container');
			const ignored = (el) => !!Sortable.utils.closest(el, filter, list);
			return {Head: !ignored(card.querySelector('.card-toggle')), Up: ignored(card.querySelector('button:not(.card-toggle)'))};
		})()`, &got),
	)
	if !got.Head {
		t.Error("a drag from the card's head is ignored, so cards can't be dragged by their name")
	}
	if !got.Up {
		t.Error("a drag from ↑, ↓ or Edit should be ignored, so those stay clickable")
	}
}
