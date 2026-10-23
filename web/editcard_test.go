package web

import (
	"context"
	"fmt"
	"math"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// Changing a skill happens in its own card: Edit moves the one skill form
// out of Add a skill into the card, under its head, and Update or Cancel
// moves it back (placeEditor in static/js/app.js). Meanwhile the head shows
// the skill as it will be, with Update and Cancel on it (readDraft).

// tuckBacks is n tuck backs as a routine's skills, as JSON.
func tuckBacks(n int) string {
	skill := `{"name":"Tuck Back","rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck","backward":true}`
	return "[" + strings.TrimSuffix(strings.Repeat(skill+",", n), ",") + "]"
}

// cardRoutines are two routines of tuck backs saved in the browser, the
// second side by side with the first if beside.
func cardRoutines(n int, beside bool) string {
	b := ""
	if beside {
		b = `,"beside":"r2"`
	}
	return `{"current":"r1"` + b + `,"routines":[{"id":"r1","name":"First","skills":` + tuckBacks(n) + `},{"id":"r2","name":"Second","skills":` + tuckBacks(n) + `}]}`
}

// savedSkills is a saved routine's skills (0 the first), by a property of each, joined.
func savedSkills(routine int, prop string) string {
	return `JSON.parse(localStorage.getItem('trampolineRoutines')).routines[` + string(rune('0'+routine)) + `].skills.map((s) => s.` + prop + `).join()`
}

// poll waits for a condition in the page. Alpine shows and hides things on
// its next tick, so what it shows is polled for, not read straight away. If
// it times out, it says which condition, and where the focus and the skill
// form were then, so a failure on CI says what it was waiting for.
func poll(cond string) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		err := chromedp.Poll(cond, nil, chromedp.WithPollingInterval(50*time.Millisecond)).Do(ctx)
		if err == nil {
			return nil
		}
		var state string
		quick, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		_ = chromedp.Evaluate(`'focus ' + (document.activeElement?.outerHTML || '').slice(0, 120) + '; on the card: ' + (document.querySelector('#skill-form-wrapper .skill-card-name')?.textContent || '') + '; results: ' + document.querySelectorAll('#search-results .picker-skill').length + '; a mouse: ' + matchMedia('(hover: hover)').matches`, &state).Do(quick)
		return fmt.Errorf("waiting for %s: %w (%s)", cond, err, state)
	})
}

// openBuilder loads the builder at a size (a phone's, with mobile) with
// routines saved, the skill form loaded in Add a skill.
func openBuilder(url string, w, h int64, mobile bool, routines string) chromedp.Tasks {
	viewport := chromedp.EmulateViewport(w, h)
	if mobile {
		viewport = chromedp.EmulateViewport(w, h, chromedp.EmulateMobile)
	}
	return chromedp.Tasks{
		viewport,
		chromedp.Navigate(url + "/"),
		chromedp.Evaluate(`localStorage.clear(); localStorage.setItem('trampolineRoutines', `+"`"+routines+"`"+`)`, nil),
		chromedp.Reload(),
		chromedp.WaitVisible(`#routine-skills .card-toggle`, chromedp.ByQuery),
		poll(`!!document.querySelector('.builder-add #skill-editor .skill-card-name') && !!Sortable.get(document.getElementById('routine-skills'))`),
	}
}

// inCard is whether the skill form is in the card at index i of a column's list.
func inCard(list string, i int) string {
	return `!!document.querySelectorAll('#` + list + ` .routine-skill-container')[` + string(rune('0'+i)) + `]?.querySelector('#skill-form-wrapper')`
}

// formHome is whether the skill form is back in Add a skill, ready to add.
const formHome = `!!document.querySelector('.builder-add > #skill-form-wrapper #add-btn') && document.querySelector('.builder-add-waiting').offsetParent === null`

// headShows is whether the card being changed shows on its head, until
// Update, the skill as it will be: its number and name, its tariff, and
// "not saved" or not; Update and Cancel there, in place of ↑, ↓, Edit and ✕.
func headShows(name, tariff string, unsaved bool) string {
	return `(() => { const card = document.querySelector('.routine-skill-container.editing'), head = card?.querySelector('.card-head'), draft = head?.querySelector('.card-draft');
		const shown = (el) => !!el && el.offsetParent !== null;
		return shown(draft) && !shown(head.querySelector('.card-saved')) && draft.querySelector('.skill-name').textContent.replace(/\s+/g, ' ').trim() === ` + "`" + name + "`" + `
			&& [...head.querySelectorAll('.card-tariff')].find(shown)?.textContent === '` + tariff + `' && shown(draft.querySelector('.card-unsaved')) === ` + fmt.Sprint(unsaved) + `
			&& shown(head.querySelector('#update-btn')) && shown(head.querySelector('#cancel-btn')) && !shown(head.querySelector('[data-edit]')); })()`
}

// readoutShown is whether the skill card shows in the card being changed:
// only as the readout of Build or adjust while that's open.
const readoutShown = `document.querySelector('#skill-form-wrapper .skill-readout')?.offsetParent != null`

// headTop is where a card's head is on the screen.
func headTop(list string, i int) string {
	return `document.querySelectorAll('#` + list + ` .routine-skill-container')[` + string(rune('0'+i)) + `].querySelector('.card-head').getBoundingClientRect().top`
}

// Edit opens the card into the skill form, the card above it not moving:
// a shape changed shows on the head, not saved, and Update saves it; the
// search finds another skill, Enter, Enter and Enter putting it on the head
// and updating, the head staying put; the skill card shows only while Build
// or adjust is open; Cancel and Escape put the head back and leave the
// skill as it was; Edit on another card moves the form. Meanwhile Add a
// skill says where the form is.
func TestEditInTheCardInABrowser(t *testing.T) {
	ctx := browser(t)
	srv := httptest.NewServer(routes())
	defer srv.Close()

	var before, after, above float64
	var waiting string
	var pickerShown bool
	step(t, ctx, "opening the fourth card, scrolled to it",
		openBuilder(srv.URL, 1300, 1100, false, cardRoutines(10, false)),
		chromedp.Evaluate(`document.querySelector('[data-edit="a:3"]').scrollIntoView({block: 'center'}); 0`, nil),
		chromedp.Sleep(100*time.Millisecond),
		chromedp.Evaluate(headTop("routine-skills", 3), &before),
		chromedp.Evaluate(headTop("routine-skills", 2), &above),
		chromedp.Click(`[data-edit="a:3"]`, chromedp.ByQuery),
		poll(inCard("routine-skills", 3)+` && !!document.getElementById('update-btn') && document.querySelector('.routine-skill-container.editing') !== null`),
		poll(headShows("4. Tuck Back", "0.5", false)+` && !(`+readoutShown+`)`),
		poll(`document.querySelector('.builder-add-waiting').offsetParent !== null && document.querySelector('.picker-browse').offsetParent === null`),
		chromedp.Sleep(300*time.Millisecond),
		chromedp.Evaluate(headTop("routine-skills", 3), &after),
		chromedp.Evaluate(`document.querySelector('.builder-add-waiting').textContent`, &waiting),
		chromedp.Evaluate(`document.querySelector('.picker-browse').offsetParent !== null`, &pickerShown),
	)
	if math.Abs(after-before) >= 1 {
		t.Errorf("the card moved from %v to %v as it opened", before, after)
	}
	if want := "Changing skill 4 in its card below: Update or Cancel there, then carry on adding."; waiting != want || pickerShown {
		t.Errorf("Add a skill says %q (picker shown %v), want %q and the picker hidden", waiting, pickerShown, want)
	}
	var aboveAfter float64
	step(t, ctx, "the card above stays put",
		chromedp.Evaluate(headTop("routine-skills", 2), &aboveAfter),
	)
	if math.Abs(aboveAfter-above) >= 1 {
		t.Errorf("the card above moved from %v to %v", above, aboveAfter)
	}

	var shaped float64
	step(t, ctx, "changing its shape to pike: the head shows it, not saved, and Update saves it",
		chromedp.Click(`#skill-form-wrapper .shape-segment:nth-child(2)`, chromedp.ByQuery),
		poll(headShows("4. Pike Back", "0.6", true)),
		chromedp.Evaluate(headTop("routine-skills", 3), &shaped),
		poll(savedSkills(0, "shape")+` === 'Tuck,Tuck,Tuck,Tuck,Tuck,Tuck,Tuck,Tuck,Tuck,Tuck'`),
		chromedp.Click(`#update-btn`, chromedp.ByQuery),
		poll(savedSkills(0, "shape")+` === 'Tuck,Tuck,Tuck,Pike,Tuck,Tuck,Tuck,Tuck,Tuck,Tuck'`),
		poll(formHome+` && document.activeElement?.dataset.edit === 'a:3'`),
	)
	if math.Abs(shaped-after) >= 1 {
		t.Errorf("the card's head moved from %v to %v as the shape was chosen", after, shaped)
	}

	var held, chosen float64
	step(t, ctx, "changing it to a rudi by the search: Enter, Enter, Enter",
		chromedp.Evaluate(headTop("routine-skills", 3), &before),
		chromedp.Click(`[data-edit="a:3"]`, chromedp.ByQuery),
		poll(inCard("routine-skills", 3)+` && `+headShows("4. Pike Back", "0.6", false)),
		// With a mouse the search has the focus already; a browser without one
		// (headless on CI) is treated as a touch screen, so it's clicked into.
		poll(`!matchMedia('(min-width: 1024px) and (hover: hover)').matches || document.activeElement?.id === 'skill-search'`),
		chromedp.Click(`#skill-search`, chromedp.ByQuery),
		chromedp.SendKeys(`#skill-search`, "rudi\r", chromedp.ByQuery),
		poll(`document.activeElement?.matches('#search-results .picker-skill') && document.activeElement.textContent.includes('Rudi')`),
		// The typing's own search comes back after Enter's: the focus stays on the first result.
		chromedp.Sleep(400*time.Millisecond),
		poll(`document.activeElement?.matches('#search-results .picker-skill')`),
		chromedp.KeyEvent("\r"),
		// The result goes straight on the card's head, the focus on to its Update.
		poll(headShows("4. Rudi", "0.8", true)+` && document.activeElement?.id === 'update-btn'`),
		chromedp.Evaluate(headTop("routine-skills", 3), &chosen),
		chromedp.KeyEvent("\r"),
		poll(savedSkills(0, "name")+`.split(',')[3] === 'Rudi'`),
		poll(formHome+` && document.activeElement?.dataset.edit === 'a:3'`),
		chromedp.Evaluate(headTop("routine-skills", 3), &held),
	)
	if math.Abs(held-before) >= 1 || math.Abs(chosen-before) >= 1 {
		t.Errorf("the card moved from %v to %v as the result was chosen, and %v once updated", before, chosen, held)
	}

	var shapes string
	// The card's head as saved: its own name, nothing of a change left on it.
	savedHead := func(i int, name string) string {
		return `(() => { const head = document.querySelectorAll('#routine-skills .routine-skill-container')[` + fmt.Sprint(i) + `].querySelector('.card-head');
			return head.querySelector('.card-saved')?.offsetParent != null && head.querySelector('.card-draft').offsetParent === null
				&& head.querySelector('.card-saved .skill-name').textContent.trim() === '` + name + `' && head.querySelector('[data-edit]').offsetParent != null; })()`
	}
	step(t, ctx, "Cancel and Escape leave the skill as it was, Build or adjust showing the skill card while it's open",
		chromedp.Click(`[data-edit="a:1"]`, chromedp.ByQuery),
		poll(inCard("routine-skills", 1)+` && !!document.getElementById('cancel-btn') && !(`+readoutShown+`)`),
		chromedp.Click(`#skill-builder > summary`, chromedp.ByQuery),
		// (Its toggle event, a moment later, notes it's open for the server.)
		poll(readoutShown+` && !document.querySelector('#skill-form-wrapper #update-btn') && document.getElementById('main-form').elements.builder_open.value === '1'`),
		chromedp.Click(`#skill-form-wrapper .shape-segment:nth-child(3)`, chromedp.ByQuery),
		poll(headShows("2. Straight Back", "0.6", true)+` && `+readoutShown),
		chromedp.Click(`#skill-builder > summary`, chromedp.ByQuery),
		poll(`!(`+readoutShown+`)`),
		chromedp.Click(`#cancel-btn`, chromedp.ByQuery),
		poll(formHome+` && document.activeElement?.dataset.edit === 'a:1' && `+savedHead(1, "2. Tuck Back")),
		chromedp.Click(`[data-edit="a:1"]`, chromedp.ByQuery),
		poll(inCard("routine-skills", 1)+` && !!document.getElementById('cancel-btn')`),
		chromedp.Click(`#skill-form-wrapper .shape-segment:nth-child(2)`, chromedp.ByQuery),
		poll(headShows("2. Pike Back", "0.6", true)),
		chromedp.Focus(`#update-btn`, chromedp.ByQuery),
		chromedp.KeyEvent("\x1b"),
		poll(formHome+` && document.activeElement?.dataset.edit === 'a:1' && `+savedHead(1, "2. Tuck Back")),
		chromedp.Evaluate(savedSkills(0, "shape"), &shapes),
	)
	if shapes != "Tuck,Tuck,Tuck,Straight,Tuck,Tuck,Tuck,Tuck,Tuck,Tuck" {
		t.Errorf("after Cancel and Escape the shapes are %s", shapes)
	}

	var marked int
	step(t, ctx, "Edit on another card moves the form there",
		chromedp.Click(`[data-edit="a:0"]`, chromedp.ByQuery),
		poll(inCard("routine-skills", 0)+` && !!document.getElementById('update-btn')`),
		chromedp.Click(`#skill-form-wrapper .shape-segment:nth-child(2)`, chromedp.ByQuery),
		poll(headShows("1. Pike Back", "0.6", true)),
		chromedp.Evaluate(`document.querySelector('[data-edit="a:4"]').scrollIntoView({block: 'center'}); 0`, nil),
		chromedp.Sleep(100*time.Millisecond),
		chromedp.Evaluate(headTop("routine-skills", 4), &before),
		chromedp.Click(`[data-edit="a:4"]`, chromedp.ByQuery),
		poll(inCard("routine-skills", 4)+` && `+headShows("5. Tuck Back", "0.5", false)+` && `+savedHead(0, "1. Tuck Back")),
		chromedp.Sleep(200*time.Millisecond),
		chromedp.Evaluate(headTop("routine-skills", 4), &after),
		chromedp.Evaluate(`document.querySelectorAll('.routine-skill-container.editing').length`, &marked),
		// A label shows on the head as it's typed.
		chromedp.Click(`#skill-form-wrapper .skill-card-label > summary`, chromedp.ByQuery),
		chromedp.SendKeys(`#custom-name`, "Opener", chromedp.ByQuery),
		poll(headShows("5. Opener · Tuck Back", "0.5", true)),
		chromedp.Click(`#cancel-btn`, chromedp.ByQuery),
		poll(formHome),
		chromedp.Evaluate(savedSkills(0, "shape"), &shapes),
	)
	if math.Abs(after-before) >= 1 || marked != 1 {
		t.Errorf("the fifth card moved from %v to %v as the form came to it; %d cards marked", before, after, marked)
	}
	if shapes != "Tuck,Tuck,Tuck,Straight,Tuck,Tuck,Tuck,Tuck,Tuck,Tuck" {
		t.Errorf("the first card's change should have been left: %s", shapes)
	}
}

// Side by side, a card in the second routine opens in its own column, and
// Update changes that routine only.
func TestEditInTheSecondColumnInABrowser(t *testing.T) {
	ctx := browser(t)
	srv := httptest.NewServer(routes())
	defer srv.Close()

	var first, second, waiting string
	var fits bool
	step(t, ctx, "changing the second routine's third skill to pike",
		openBuilder(srv.URL, 1300, 760, false, cardRoutines(4, true)),
		poll(`!!document.querySelector('#routine-skills-b [data-edit="b:2"]')`),
		chromedp.Click(`[data-edit="b:2"]`, chromedp.ByQuery),
		poll(inCard("routine-skills-b", 2)+` && !!document.getElementById('update-btn')`),
		chromedp.Evaluate(`document.querySelector('.builder-add-waiting').textContent`, &waiting),
		chromedp.Evaluate(`(() => { const card = document.querySelectorAll('#routine-skills-b .routine-skill-container')[2].getBoundingClientRect(), form = document.getElementById('main-form').getBoundingClientRect(); return form.left >= card.left && form.right <= card.right; })()`, &fits),
		chromedp.Click(`#skill-form-wrapper .shape-segment:nth-child(2)`, chromedp.ByQuery),
		poll(headShows("3. Pike Back", "0.6", true)+` && !!document.querySelector('#routine-skills-b .editing #update-btn')`),
		chromedp.Click(`#update-btn`, chromedp.ByQuery),
		poll(savedSkills(1, "shape")+` === 'Tuck,Tuck,Pike,Tuck'`),
		poll(formHome+` && document.activeElement?.dataset.edit === 'b:2'`),
		chromedp.Evaluate(savedSkills(0, "shape"), &first),
		chromedp.Evaluate(savedSkills(1, "shape"), &second),
	)
	if want := "Changing skill 3 of Second in its card below: Update or Cancel there, then carry on adding."; waiting != want {
		t.Errorf("Add a skill says %q, want %q", waiting, want)
	}
	if !fits {
		t.Error("the form should fit the second column's card")
	}
	if first != "Tuck,Tuck,Tuck,Tuck" || second != "Tuck,Tuck,Pike,Tuck" {
		t.Errorf("the routines are %s and %s", first, second)
	}
}

// On a phone the form fits the card, the page doesn't scroll sideways, the
// card stays where it was tapped, and Update works as on a laptop.
func TestEditInTheCardOnAPhoneInABrowser(t *testing.T) {
	ctx := browser(t)
	srv := httptest.NewServer(routes())
	defer srv.Close()

	var before, after float64
	var fits, sideways, headFit bool
	var shapes string
	// Update and Cancel, in place of ↑, ↓, Edit and ✕, fit the head beside
	// the tariff: all in the card, the buttons side by side, level with it.
	const headFits = `(() => { const head = document.querySelector('.routine-skill-container.editing .card-head'), box = head.getBoundingClientRect();
		const update = head.querySelector('#update-btn').getBoundingClientRect(), cancel = head.querySelector('#cancel-btn').getBoundingClientRect(), tariff = [...head.querySelectorAll('.card-tariff')].find((el) => el.offsetParent).getBoundingClientRect();
		return update.right <= cancel.left && Math.abs(update.top - cancel.top) < 1 && tariff.right <= update.left && cancel.right <= box.right + 0.5 && box.left >= 0 && box.right <= window.innerWidth; })()`
	step(t, ctx, "changing the fifth skill on a phone",
		openBuilder(srv.URL, 390, 844, true, cardRoutines(8, false)),
		chromedp.Evaluate(`document.querySelector('[data-edit="a:4"]').scrollIntoView({block: 'center'}); 0`, nil),
		chromedp.Sleep(100*time.Millisecond),
		chromedp.Evaluate(headTop("routine-skills", 4), &before),
		chromedp.Click(`[data-edit="a:4"]`, chromedp.ByQuery),
		poll(inCard("routine-skills", 4)+` && !!document.getElementById('update-btn')`),
		chromedp.Sleep(300*time.Millisecond),
		chromedp.Evaluate(headTop("routine-skills", 4), &after),
		chromedp.Evaluate(`(() => { const card = document.querySelectorAll('#routine-skills .routine-skill-container')[4].getBoundingClientRect(); return [...document.querySelectorAll('#main-form *')].every((el) => { const r = el.getBoundingClientRect(); return r.width === 0 || (r.left >= card.left - 0.5 && r.right <= card.right + 0.5); }); })()`, &fits),
		chromedp.Evaluate(`document.documentElement.scrollWidth > window.innerWidth`, &sideways),
		chromedp.Click(`#skill-form-wrapper .shape-segment:nth-child(3)`, chromedp.ByQuery),
		poll(headShows("5. Straight Back", "0.6", true)),
		chromedp.Evaluate(headFits, &headFit),
		chromedp.Click(`#update-btn`, chromedp.ByQuery),
		poll(formHome),
		chromedp.Evaluate(savedSkills(0, "shape"), &shapes),
	)
	if math.Abs(after-before) >= 1 {
		t.Errorf("the card moved from %v to %v as it opened", before, after)
	}
	if !fits || sideways {
		t.Errorf("the form should fit the card (%v), and the page not scroll sideways (%v)", fits, sideways)
	}
	if !headFit {
		t.Error("the head's name, tariff, Update and Cancel should fit on one row, in the card")
	}
	if shapes != "Tuck,Tuck,Tuck,Tuck,Straight,Tuck,Tuck,Tuck" {
		t.Errorf("the shapes are %s", shapes)
	}
}

// A card opened low on the screen is brought up, once, as far as its form
// needs and no further: Build or adjust in sight and the card's head (with
// Update) still on screen.
func TestEditBringsTheFormIntoViewInABrowser(t *testing.T) {
	ctx := browser(t)
	srv := httptest.NewServer(routes())
	defer srv.Close()

	var head, settled, later float64
	var updateSeen bool
	step(t, ctx, "opening a card at the foot of the window",
		openBuilder(srv.URL, 1300, 640, false, cardRoutines(6, false)),
		chromedp.Evaluate(`document.querySelector('[data-edit="a:2"]').scrollIntoView({block: 'end'}); 0`, nil),
		chromedp.Sleep(100*time.Millisecond),
		chromedp.Click(`[data-edit="a:2"]`, chromedp.ByQuery),
		poll(inCard("routine-skills", 2)+` && document.getElementById('skill-builder')?.getBoundingClientRect().bottom <= window.innerHeight`),
		chromedp.Sleep(600*time.Millisecond),
		chromedp.Evaluate(headTop("routine-skills", 2), &head),
		chromedp.Evaluate(`document.getElementById('skill-builder').getBoundingClientRect().bottom <= window.innerHeight && document.getElementById('update-btn').getBoundingClientRect().top >= 0`, &updateSeen),
		chromedp.Evaluate(`window.scrollY`, &settled),
		chromedp.Sleep(400*time.Millisecond),
		chromedp.Evaluate(`window.scrollY`, &later),
	)
	if head < 0 || !updateSeen || later != settled {
		t.Errorf("the card's head is at %v, the form and Update in sight %v; the page went on from %v to %v", head, updateSeen, settled, later)
	}
}

// In Levels mode, a voluntary's card is changed in its tab as in a routine:
// the head shows the change, and Update saves it to the routine doing it.
func TestEditInALevelInABrowser(t *testing.T) {
	ctx := browser(t)
	srv := httptest.NewServer(routes())
	defer srv.Close()

	entry := `{"current":"e1","entries":[{"id":"e1","name":"L3","level":"builtin-level:bucs-l3","exercises":[{},{"routine":"r1"}],"open":"2:builtin:bucs-l3-second"}]}`
	var shapes string
	step(t, ctx, "changing a voluntary's second skill to pike",
		chromedp.EmulateViewport(1300, 900),
		chromedp.Navigate(srv.URL+"/"),
		chromedp.Evaluate(`localStorage.clear(); localStorage.setItem('trampolineRoutines', `+"`"+cardRoutines(4, false)+"`"+`); localStorage.setItem('trampolineLevelEntries', `+"`"+entry+"`"+`); localStorage.setItem('builderMode', 'levels')`, nil),
		chromedp.Reload(),
		poll(`!!document.querySelector('#routine-skills [data-edit="a:1"]') && !!document.querySelector('.builder-add #skill-editor .skill-card-name')`),
		chromedp.Click(`[data-edit="a:1"]`, chromedp.ByQuery),
		poll(inCard("routine-skills", 1)+` && `+headShows("2. Tuck Back", "0.5", false)),
		chromedp.Click(`#skill-form-wrapper .shape-segment:nth-child(2)`, chromedp.ByQuery),
		poll(headShows("2. Pike Back", "0.6", true)),
		chromedp.Click(`#update-btn`, chromedp.ByQuery),
		poll(formHome+` && document.activeElement?.dataset.edit === 'a:1'`),
		chromedp.Evaluate(savedSkills(0, "shape"), &shapes),
	)
	if shapes != "Tuck,Pike,Tuck,Tuck" {
		t.Errorf("the voluntary's shapes are %s", shapes)
	}
}
