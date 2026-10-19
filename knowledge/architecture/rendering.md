---
type: Architecture
title: Server-rendered UI
description: Every page and fragment is HTML rendered by the server from templ components; the browser posts the routine on each change and swaps in the result, and domain rules exist only in Go.
resource: https://github.com/jackpardy/tariff-calculator/blob/master/docs/adr/0002-server-rendered-frontend.md
tags: [architecture, rendering, templ, htmx, adr-0002]
generated: { by: claude-code/cli, at: 2026-10-04T17:45:00Z }
sources:
  - id: adr-0001
    resource: ../../docs/adr/0001-architecture.md
    title: ADR 0001 — Architecture for growing the tariff tool into a product
    author: human:jackpardy
  - id: adr-0002
    resource: ../../docs/adr/0002-server-rendered-frontend.md
    title: ADR 0002 — Server-rendered frontend with templ and HTMX
    author: human:jackpardy
---

# The rule

**The server renders all UI as HTML, and domain rules exist only in Go.**[^adr-0002]

ADR 0001 originally planned to run the engine in the browser as WebAssembly.[^adr-0001]
A review in October 2026 found that the app had drifted into a hybrid: a
~560-line Alpine component rendered JSON from the server, and landing, equality,
phase and shape rules had been copied into JavaScript. A missing JSON field
meant card highlighting silently never worked. Offline use was confirmed as
not needed, so ADR 0002 moved everything to server rendering.

# How a change flows

1. The user changes something (adds, edits, reorders, picks requirements or
   checks).
2. The page's Alpine component updates `localStorage` and posts the
   **whole routine** as JSON, plus the requirements and checks, using htmx.
3. The server parses and validates it, re-derives names and tariffs, runs
   [routine validation](../domain/routine-validation.md) and the
   [requirements](../requirements/framework.md), and renders the HTML.
4. htmx swaps the HTML in.

The server is **stateless**: each request carries everything it needs.

The skill form works the same way. On each change the form posts to
`POST /skill-inputs` (search waits 200 ms after typing stops), and the server returns the editor with the right twist
boxes, shape visibility and straddle option. While the input is unscorable (for
example a rotation still being typed), it answers 204 so htmx leaves the field
alone. The [requirements editor](../requirements/framework.md#the-editor)
follows the same pattern.

# Dropdowns

Every `<details>` looks the same: its `<summary>` gets an arrow before the text,
pointing right when closed and down when open, from one rule in
`static/css/styles.css` (`details > summary::before`). It works whatever class
the summary carries (`label`, `title`, `is-size-7`, a `<strong>` inside), so a
new dropdown needs no extra markup or CSS. Don't set `cursor`, `list-style` or
the marker on a summary: the rule does. A summary that is deliberately
something else opts out by `content: none` on its `::before`, in the same block:
`summary.button`, the routine menu (`.routine-menu`), or any summary given the class `no-arrow`. The tariff sheet and
the view page have their own stylesheets and menus, so the rule doesn't reach them.
A button that opens something has the same triangle after its words,
pointing down (`.comp-opens-button`; [the shared look](ui.md#things-that-open)).

# Settings and forms on a page

A page is about what it lists: the late changes asked for, the fees owed, the
officials, the entries. Its own settings, and forms for doing something to
what it lists, stay out of the way until asked for, as **one line with a small
button** (`views/opens.templ`), not a card with a heading of its own that
opens and closes (the look the pages had when they were all on one long
dashboard):

- `changeLine(what, now, open)`: a setting, **what** in bold and how it's set
  **now**, with a **Change** button opening its form beneath, e.g. "**Late
  changes** · level change (€5) and routine change (no fee) can be asked for
  after the deadline [Change]". Late changes, What you charge, Panels and who
  may judge, the timetable's Days and areas, Timings and rest, Blocked time
  and Your rules, and a person's Judging and helping. The server works out
  `now` with the page, so the line always says what's saved.
- `opens(what, now, label, open)`: the same with any button, for a form that
  does something: Add someone, Remove or hold entries, Change the entry.
- A row's rare actions, in a list where every row would otherwise carry them
  (a club's member: Make a coach too, New link, Remove), go under
  `opens("", "", "More", false)` in the row's last cell, given the class
  `comp-row-more`: the button stays at the right and the actions open
  beneath it.

Settings are rows of a list: the name in bold with how it's set beneath in
grey, the button always in the same place at the right, and a hairline
between rows, so a page's settings line up however long each one's text is.
A button on its own (an action, with no text) has no row. The button says
Close while its form is open, and its chevron turns, so it looks like
something that opens; the form sits beneath with a light rule on its left. `open` shows it at first: when nothing is set yet (fees), the
timetable isn't planned, or a save came back with problems.

A list that is what the page is for has a plain heading with its count
("People · 325", `listSection`), not a dropdown: the waiting lists, Removed
and on hold, Scenarios and the desk's Sent too. Rows that are the content
(each flight, each discipline of an entry, each routine) still open and close
as dropdowns.

A page's several printouts go under one **Print** button, a small pop-up
menu (`details.comp-popup[data-menu]`, which `static/js/menus.js` closes on a
tap outside): `printMenu` for the timetable's sheets, `dashboardPrint` for
the entries page's cards (and a chair of judges' sheets). A menu whose
button sits at the left of the page (the entries page's Print, a person's
Timetables and calendars) adds `comp-popup-left`, so it opens to the right
and stays on screen.

# Lists of people: search and filters

Every list of people has the same search and filters (roadmap item 6), from
one set of pieces, so they look and work alike. The organiser's Officials
page's People list is the example to copy.

- **The section.** `@listSection(title, count)` in `views/lists.templ`
  draws the plain heading and count ("People · 325"); it doesn't open and
  close. Put the list, and its "nobody yet" message, inside it.
- **The tools.** `@listTools(views.ListTools{ID, Noun, Total, Filters})`
  goes above the list, which gets `id={ ID }`. `ID` is unique on the page and
  also keys the saved search, so keep it stable. `Noun` is plural ("people",
  "members"). Leave the tools out when the list is empty.
- **The rows.** Each row (`<tr>`, `<li>`, `<label>`) spreads
  `{ row.List.Attrs()... }`, from a `views.ListRow{Name, Values}`. `Name` is
  what "Find a name" searches. `Values` holds the row's values for each
  filter key; a row can have several values for a key, or none. A row that
  spans two table rows is one `<tbody>` (a table can have several). Where
  rows sit in groups (a box per club, an area's column, a flight), give each
  group `data-list-group`: it hides when none of its rows match. A group that
  is a dropdown (`<details>`: a dashboard level, a flight's check-in or panel)
  opens while a search finds a match in it, closes again when the search is
  cleared, and is never remembered as opened.
- **The filters.** Build them in Go, in `web/lists.go`, so lists share
  them: `clubFilter` and `clubValue` (with "No club"), `canDoFilter` and
  `canDo`, `judgesFilter` and `judges` (discipline and level, `AtLeast`:
  "BUCS L6" matches anyone judging L6 or harder), and `qualifiedFilter` and
  `qualifiedValue`. Add a new filter there, next to its row values, as a
  `views.ListFilter{Key, Label, Any, Options}`. `Any` is the first choice,
  which matches everyone (for example "Every club"). Group long choices with
  `ListOption.Group`. Mark a filter `Plain` when its choices say what they
  are on their own ("Has entries", "Not checked"): its chip is then the
  choice alone, not "Entries: Has entries". `hasFilter` and the yes/no
  filters are plain.
- **The script.** `listTools` in `static/js/competitions.js` does the rest
  on the phone:
  - it searches a name's letters only, ignoring case, accents, spaces and
    punctuation, as the server's `competitions.NameMatches` does, so
    "oneill" finds "O'Neill", "o briain" finds "Ó Briain" and "marykate"
    finds "Mary Kate";
  - it shows "12 of 323 people", a Clear button, and "Nobody matches";
  - it puts on each choice how many it would leave ("UCD (12)"), given the
    search and the other filters chosen, as they change, the rows still
    folded away (`lazyRows`) counted from their templates without putting
    them in the page; a choice that would leave nobody is greyed out
    (a disabled `<option>`), unless it's the one chosen, so it can be seen
    and taken off;
  - it lists the filters chosen after the count, each a chip ("Club: UCD ×")
    that takes it off; the Filters button's number is how many chips there
    are;
  - it reads the list's rows each time it filters, so a page that swaps in
    fresh content (On the day's in-place taps) keeps its search: `dayPage`
    runs the search again after each swap;
  - it ignores Enter in the search, so a list inside a form (the desk's
    "People I pick", which isn't a `listSection`) never sends it;
  - it keeps the search and filters for the tab, so a page that reloads after
    a change comes back filtered the same.

  The tools stay hidden without JavaScript, and the whole list shows. Don't
  add Alpine or any other script for a list.

**A list the server filters.** The dashboard's entries are filtered on the
server, because printing, the CSV and each level's count follow the filters.
`ListTools.Form` draws the same tools as a GET form: each filter is a field
named by its key, the search is `q`, and `ListFilter.Value`, `Search`, `Shown`
and `Clear` come from the server.
- A filter reloads the page.
- The search narrows the rows already on the page as it's typed, as on every
  other list. It also keeps the page's address, every link and form marked
  `data-list-carry` (Print, CSV, Remove or hold) and, in a list marked
  `data-list-carry-links`, an entry's link as it's opened, in step, and
  Enter sends it to the server.
- Groups that are dropdowns (the levels) open while a search finds a match,
  as on every list.
- Extra controls that aren't filters (the sort) go in the tools' children.
- Its chips are drawn by the server, each a link to the page without that
  filter (`ListFilter.Remove`), carrying the search as it's typed.
- Only the rows shown are in the page, so to count each choice the server
  also sends every row's values (`ListTools.All`), small, as JSON in a
  script element (`listAll`); the script counts from those, with the search
  as typed. An entry's values (`judged.filterValues`) are exactly what
  `judged.matches` reads, which `TestDashboardFilterValuesMatch` holds to.

Test a list with `listRows` in `web/lists_test.go`, which reads each row's
name and values from the page.

# Wide tables on a phone

A table too wide for a phone (Officials, Fees, and others as they're added)
takes the class `comp-stack`. Below 769px each row becomes a card: the first
cell is its title, and each other cell is a line labelled by its
`data-label` attribute ("Club: UCD"). The header row hides, empty cells
hide, and a cell's form sits on its label's line. Rows the list search hides
stay hidden. Give every cell after the first a `data-label` matching its
column's heading; leave it off a cell that's only actions.

# States and roles: one palette

What state something is in, and who did it, are shown with two chips from
`views/status.templ`, and with no other colour, so a colour means the same
thing on every page (the plan is in the
[cleanup audit](../audits/2026-10-cleanup.md#colour-and-status-added-15-october-2026)).

- **`@views.StateChip(state, label)`**: a filled chip with a short label, and
  no mark before it (dropped 2026-10-19: the words and the colour say it).
  Pick the state by what it means for the reader, not by the word:

  | State | Means | For example |
  | --- | --- | --- |
  | `NotYet` | not started, not yet | Not checked in, Saved not sent, On the waiting list |
  | `InProgress` | under way, or waiting on someone else | Sent, Waiting for sign-off, Learning |
  | `Done` | finished, fine | Checked, Signed off, Here, Paid, Can do |
  | `Attention` | someone should look | Has problems, Changed since sent, Waiting for you |
  | `Stopped` | out, ended or refused | Withdrawn, Removed, Scratched, Missing, Not approved |

- **`@views.WhoChip(role, name)`**: an outlined chip that always says the
  role (Gymnast, Coach, Club, Organiser) and the name if there is one: who
  marked, signed, checked or entered something, or whose a seat or entry is.
  Use it beside the state it explains, for example a Can do chip and a
  Coach chip together.

The colours are tokens in `static/css/styles.css` (`--state-*`, `--role-*`)
and `static/contrast_test.go` checks each reads at 4.5:1 or better. Don't
use Bulma's colour classes (`tag is-success`, `has-text-danger` and the rest)
to show a state or a role on a new page; the pages that still do move over
in turn (the audit's rollout). On the day, the dashboard, the club's pages,
the timetable's rota, Officials, a coach's entries, My competition, Tell me
about changes, every entry card's sign-off, late changes and an individual's
pages have moved
([what each shows](../features/competition-entries.md#states-and-roles-on-the-pages-one-palette-step-2)),
and `TestStatesUseChips` (`views/status_test.go`) fails if one of their
templates shows a state in Bulma's colours again: add a page to its list
once it moves over. Buttons have a look by role ([the shared look](ui.md#buttons-by-role)):
only a button that sets a state (On the day's Here, Scratched) takes that
state's colours. Notices after an action are `@notice` (green, with ×), and
the venue screen's area colours aren't states.

A chip placed after a name or another chip gets a small gap from the CSS.
After text on the same line, templ reads `@StateChip(...)` as more text, so
put the call on a line of its own (`Check-in ·`, then `@countChips(...)`).

# JavaScript's job

Only what must run in the browser: keeping `localStorage` in sync, expanding
and collapsing, toasts, drag to reorder, [sharing](../features/sharing.md)
(fragment encoding, share sheet), the tariff sheet's show/hide toggles, and the
[update notice](static-assets.md#update-notice). None of it computes a tariff,
landing or equality.

# Consequences

- Each interaction is a round trip. This is accepted because the engine answers
  in milliseconds and offline isn't needed.
- The whole UI is testable from Go: handler tests assert on rendered HTML
  ([testing](testing.md)).
- Template mistakes are compile errors.

All four migration steps in ADR 0002 were completed on 2026-10-03/04.

[^adr-0001]: ADR 0001 — Architecture for growing the tariff tool into a product
[^adr-0002]: ADR 0002 — Server-rendered frontend with templ and HTMX
