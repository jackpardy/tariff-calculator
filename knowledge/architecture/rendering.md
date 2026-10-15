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
`summary.button`, the routine menu (`.routine-menu`), the inline "move" link
(`.comp-move`), or any summary given the class `no-arrow`. The tariff sheet and
the view page have their own stylesheets and menus, so the rule doesn't reach them.

# Lists of people: search and filters

Every list of people has the same search and filters (roadmap item 6), from
one set of pieces, so they look and work alike. The organiser's Officials
page's People list is the example to copy.

- **The section.** `@listSection(title, count, closed)` in `views/lists.templ`
  draws the heading and count as a dropdown, open unless `closed`. Put the
  list, and its "nobody yet" message, inside it.
- **The tools.** `@listTools(views.ListTools{ID, Noun, Total, Filters})`
  goes above the list, which gets `id={ ID }`. `ID` is unique on the page and
  also keys the saved search, so keep it stable. `Noun` is plural ("people",
  "members"). Leave the tools out when the list is empty.
- **The rows.** Each row (`<tr>`, `<li>`, `<label>`) spreads
  `{ row.List.Attrs()... }`, from a `views.ListRow{Name, Values}`. `Name` is
  what "Find a name" searches. `Values` holds the row's values for each
  filter key; a row can have several values for a key, or none. A row that
  spans two table rows is one `<tbody>` (a table can have several). Where
  rows sit in groups (a box per club), give each group `data-list-group`:
  it hides when none of its rows match.
- **The filters.** Build them in Go, in `web/lists.go`, so lists share
  them: `clubFilter` and `clubValue` (with "No club"), `canDoFilter` and
  `canDo`, `judgesFilter` and `judges` (discipline and level, `AtLeast`:
  "BUCS L6" matches anyone judging L6 or harder), and `qualifiedFilter` and
  `qualifiedValue`. Add a new filter there, next to its row values, as a
  `views.ListFilter{Key, Label, Any, Options}`. `Any` is the first choice,
  which matches everyone (for example "Every club"). Group long choices with
  `ListOption.Group`.
- **The script.** `listTools` in `static/js/competitions.js` does the rest
  on the phone:
  - it searches a name's letters only, ignoring case, accents, spaces and
    punctuation, as the server's `competitions.NameMatches` does, so
    "oneill" finds "O'Neill", "o briain" finds "Ó Briain" and "marykate"
    finds "Mary Kate";
  - it shows "12 of 323 people", a Clear button, and "Nobody matches";
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
  other list. It also keeps the page's address and every link marked
  `data-list-carry` (Print, CSV) in step, and Enter sends it to the server.
- Groups that are dropdowns (the levels) open while a search finds a match
  in them. They close again when the search is cleared, and are never
  remembered as opened.
- Extra controls that aren't filters (the sort) go in the tools' children.

Test a list with `listRows` in `web/lists_test.go`, which reads each row's
name and values from the page.

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
