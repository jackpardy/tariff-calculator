---
type: Architecture
title: The shared look
description: The rules every competition and club page keeps to, so the app reads as one thing - one page width, buttons by role, one open arrow, one notice, ways back that say where, the scroll fade, contrast and tap sizes, chips without marks, empty states, not-found pages, print, asking first without browser pop-ups, and one wording for each state - and where each lives in the code.
resource: https://github.com/jackpardy/tariff-calculator/blob/master/views/look.templ
tags: [architecture, ui, look, css, accessibility]
generated: { by: claude-code/cli, at: 2026-10-19T12:00:00Z }
---

# Why

The look-and-feel audit of 2026-10-16 found the pages looked assembled from
parts: about 45 button combinations, main actions in three colours, four
arrows for "this opens", notices green on some pages and blue on others, a
menu that jumped between pages, grey bars at the edge of scrolling rows. The
owner likes the look and wants it **consistent**. These are the rules; the
pieces are in `views/look.templ`, `views/opens.templ`, `views/status.templ`,
`views/menu.templ` and the end of `static/css/styles.css` ("The shared look").

They cover the competition and club pages. The Routine Builder and the
Requirements page have their own section below, kept by whoever works on
them.

# Page

- **One width.** Every competition and club page has the same column: the
  section's width, up to 84rem (1,344 px), centred
  (`.section.comp-page > .container`). The menu and heading sit in the same
  place on every page. A page that needs more (the entries, the day's areas,
  the timelines) scrolls inside its own box rather than widening the page.
- **Heading.** The page's name as the `h1`, and under it what it's of, the
  competition's name: `@pageHead("Fees", p.Competition.Name)`. The entries page
  is "Entries", not the competition's name. A person's or club's own pages
  are headed with their name. Headings go down a level at a time: on a
  member's page the competition is `h2`, each discipline `h3`, its
  exercises `h4` (`cardUnder`, `EntryCard.Heading`).
- **Menu.** `@pageMenu(...)` under the heading (`views/menu.templ`). A
  section's first page has a name of its own, not the section's again
  (Entries: All entries; Timetable: Flights; On the day: Areas; Settings:
  Competition). Every page shows a current tab: a page that opens while its
  section doesn't list it (Coaches, before coaches are approved) is listed
  while it's open. A section's pages are a plain row with only the current
  one underlined.
- **The way back**, where a page has one, is above the heading:
  `@backLink(href, "Entries")`, an arrow and the name of the page it goes
  to, never "← Back" (`TestBackLinksSayWhere`). It goes where it says: a
  person's timeline and score sheets go back to My competition, where
  they're opened.
- **Nothing to show** is one grey line saying so and what brings something:
  `@emptyLine("No entries yet.")`.

# Buttons, by role

Every button on these pages has one of these looks
(`TestButtonsHaveARole`, `views/look_test.go`):

| Role | Classes | For example |
| --- | --- | --- |
| The one main action of a form or page | `button is-primary` (`is-small` among small controls; `is-medium` for the day's big taps) | Save, Publish, Send, Sign off, Approve, Started, Next gymnast, Print / Save as PDF |
| Ordinary | `button is-small` (or `button`) | Print this level's cards, Invoice, Not yet, Hold for changes, a Change that goes to another page |
| Change: opens a form in place | `@changeLine(...)` / `@opens(...)`: `button is-small comp-opens-button` | Change, Add a coach, Record a payment |
| Remove or delete | `button is-small is-text has-text-danger` | Remove, Delete, Withdraw, Delete from your page |
| Delete a whole competition or club | `button is-small is-danger`, in a `comp-danger-zone` (red edge, red-outlined Delete), after typing "delete" (`typeToDelete`; the server checks the word too) | Delete the competition, Delete the club: set apart from every other remove, and never by accident (the owner, 2026-10-19) |
| Quiet, a link that's a button | `button is-small is-text` | Untick all, Send all again, Make the change anyway |
| Sets a state (On the day) | `button is-small is-state-done` / `is-state-stopped` | Here; Scratched, Missing |

- Teal is the main action's colour, everywhere: not green, blue or yellow.
  A form has one main action; a second choice beside it is ordinary.
- No button is full width on a laptop: a button is as wide as its words.
- Red is Bulma's darker red (`#cc0f35`, 5.7:1): red text, the outlined
  danger button and white on the filled one all read.

# Things that open

- **One arrow**, a solid triangle in the text's colour. A dropdown's row
  (`details > summary`) has it before, pointing right, turning down when
  open. A button that opens something has it after, pointing down, turning
  up (`.comp-opens-button`, which a `summary` can be too). A list to choose
  from (`.select`) has the same triangle.
- Anything that opens has the arrow; a box that looks like a button is a
  button (`summary class="button is-small comp-opens-button"`: Feedback,
  Notify, Print, Columns).
- A page's own settings are one line each, saying how they're set, with
  Change opening the form in place (`changeLine`; see
  [rendering](rendering.md#settings-and-forms-on-a-page)).

# Notices

What a change just did is `@notice(text)`: green, `role="status"`, with × to
put it away. `@stickyNotice(text)` stays in sight while the page goes to the
row changed. The day page's notices, written by its script, are the same
(`competitions.js`, `say`). Warnings and problems aren't notices: they're
amber chips (`StateChip(Attention, ...)`), as an entry's problems are, with
the list under the chip.

# States and roles

`StateChip` and `WhoChip` from `views/status.templ`, in the `--state-*` and
`--role-*` colours ([rendering](rendering.md#states-and-roles-one-palette)).
A state chip is its words and its colour, with no mark before them (the
owner's call, 2026-10-19: "! 1 problem" read as shouting; the other marks
went too, so every state is shown the same way). In a row with little room,
say only what differs: the Coach column of the entries is the coach's name,
then Signed off, Needs changes or Waiting; a coach not approved to sign off
has an asterisk instead, explained once under the table
(`signoffCell`, `unapprovedKey`). Chips never wrap inside a row of the
entries table.

# Scrolling sideways

No grey bars and no snapping. Where a row has more to scroll, its edge fades
(`has-more-left`, `has-more-right`, set by `static/js/tables.js` for
`.table-container`, `.tabs`, `.comp-tt-areas` and `.comp-tl-sheets`; the
timetable's row of days by `dayPick`).

- The fade eases away in 0.15 s while scrolling (`is-scrolling`) and back in
  over 0.6 s once still. The widths are registered with `@property` and a
  **px** initial value; a rem one is dropped and the fade snaps.
- Where the page has a side margin the fade sits in it: the scroller
  reaches into the margin by `--room` (0.75rem on a phone, 1.5rem, 3rem from
  1,024 px) and is padded back by as much, so at rest nothing moves. Inside
  a box, a column or a form that opened, `--room` is 0.
- A row of tabs fades by a white veil with a small ‹ or › in it, sticking
  to the row's edge, because a row can end between two tabs and leave a
  mask nothing to fade. A row of tabs opens with its current tab in view.

# Contrast and size

- Text reads at 4.5:1 or better on white: grey small text is `#6b6b6b`
  (Bulma's `#7a7a7a` was 4.29:1), red is `#cc0f35`. `TestTextContrast`
  (`static/contrast_test.go`) checks these and the palette.
- A chip is never under 12 px: `font-size: max(0.8em, 0.75rem)`.
- On a phone everything tapped is big enough: small buttons 36 px tall, tick
  boxes 20 px (24 px on the entries cards) in a 36 px row, a menu's rows
  40 px, the way back 36 px. On the day's buttons are 40 px everywhere.

# Not found

An address with nothing at it is a page saying so, with the way on
(`notFound`, `web/helpers.go`), sent like a link's pages (no referrer, not
kept, not indexed): never Go's bare "404 page not found". An address that
asks for a day the timetable hasn't (`?day=9` on On the day) or a sheet that
isn't one (`print?sheet=zzz`) says so rather than showing another. The
venue screen is the exception: a TV left on a day that isn't one shows the
default day.

# Print

Printed, a page is what it shows: the menu, role tabs, buttons, the way
back, Feedback, the list's search and the tick bar don't print, sticky
notices print in place, and the scroll fades are off. The printouts
(cards, sheets, timelines) have their own rules.

# The Routine Builder and the Requirements page

They keep their own look for now (builder C, 2026-10-19). Shared already:
the same open arrow on the builder's More (`comp-opens-button`), red at
`#cc0f35`, and **Add to routine** as wide as its words on a laptop.

## Lists' filters

How every list's search and filters look (built in `views/lists.templ` and
`listTools` in `static/js/competitions.js`; [how](rendering.md#lists-of-people-search-and-filters)):

- **Counts on each choice.** Every choice of a filter says how many rows it
  would leave, given the search and the other filters chosen: "UCD (12)".
  The first choice ("Every club") counts too. They change as the search and
  filters change, and match what the list would show.
- **Nobody left is greyed out.** A choice that would leave nobody is a
  disabled option, so it can't be chosen by mistake, unless it's the one
  chosen: that stays, with its "(0)", to be seen and taken off.
- **The filters chosen are chips** after "N of M" under the search, in the
  palette's link blue (`tag is-link is-light`, as the Filters button's
  number): "Club: UCD ×". A filter whose choices say what they are on their
  own (`ListFilter.Plain`) is the choice alone: "Not checked ×". The whole
  chip takes its filter off; Clear takes them all.
- The Filters button's number is the number of chips.
- Don't add a second way to show a list's filters; use `ListTools`.

# Asking first

No browser pop-ups (`confirm()`, `alert()`) on the competition and club
pages: they look like the browser, not the app, and cover what's being
read (`TestNoBrowserPopups`, `views/look_test.go`). A change asks the way
its weight calls for, and only once:

| How | When | For example |
| --- | --- | --- |
| **On a page of its own**, "Before you change it" (`askBefore` and `views.AskFirst`; the timetable's `askFirst` and `askDayMarks`, `views.ChangeCheck`) | It does a lot at once: changes what many people see, or loses work done by hand | The timetable's Plan again, Assign officials again, Discard changes, Unpublish; publishing that drops what was marked on the day; turning on coach approval; lowering a limit; a change by hand that breaks a rule |
| **On the page it was asked from**, a box at its top (`views.Ask`, `askBox`; `inPlace` in `web/changecheck.go`) | It changes one person or thing and can't be put back, or is made without what it usually needs | Removing a member, a coach, a helper's link, a payment, a club skill or level; deleting a kept routine; New link; Replace the admin link (the organiser's and the comp sec's); approving a coach with no qualifications |
| **Done, with Undo** in its notice (`views.Undo`, `undoNotice`) | It's put back exactly with one tap | Withdrawing a coach's approval (Undo approves them again, with their sign-offs before) |
| **Typed** "delete" (`typeToDelete`) | Deleting a whole competition or club | |

- The ask says what the change does and to whom, plainly, then the button
  that makes it (named for the change: "Remove Ann Ryan", "Plan again") and
  Cancel, which goes back to where it was asked from.
- The ask posts the change again, as it was posted, with `anyway=1`. The
  server checks: without `anyway=1` these changes are never made, whatever
  the page sent (`asked`). An ask isn't recorded in the history
  (`noteAsked`).
- An ask to take something away (`Ask.Remove`: a person, a link, a
  payment, a skill, a level, a routine) has a red edge and a red-outlined
  button, so it reads as removing (the owner, 2026-10-19); other asks'
  button is the main action's teal. Solid red stays for deleting a whole
  competition or club.
- A button that asks first can say so in its title ("You're asked first").
- Once the competition has begun, Plan again is refused, not asked about.

# The same words

One state, one wording, on every competition and club page; the colour is
the state's (`StateChip`). The short form is for lists, rows and chips; the
long form where the page is about that one thing and there's room to say
who.

**Entries**

| State | Short | Long form, where |
| --- | --- | --- |
| Saved by a member, not sent by the club | Not sent | "Saved, not sent yet": the member's own page |
| Sent, not checked yet | Sent | "Sent by your club" (a member's page); "Sent Monday 19 October, 14:56." (an individual's entry). Never "Entered" for this |
| Checked by the organiser | Checked | "Checked by the organiser": the entry's own page |
| Changed since it was sent | Changed since sent | "Changed since your club sent it": the member's page |
| On a waiting list | Waiting list, 3rd | |
| Held for changes | On hold | "On hold: the organiser asks for changes" / "On hold: changed, send it again" |
| Problems on its card | 1 problem, 2 problems | the problems listed under the chip |
| Removed / withdrawn | Removed / Withdrawn | "Removed by the organiser" |

**Sign-off**

| State | Wording |
| --- | --- |
| Waiting for a coach | Waiting for sign-off, in full. Only in the organiser's Entries Coach column, beside the coach's name, is it "Waiting" |
| Signed off | Signed off by Declan Ward: the chip, then "by" and the name in small text, a link where there's a page (`signoffLine`; an entry's card too) |
| The coach asks for changes | Needs changes |
| By a coach not approved | the name with an asterisk, explained under the table |

**Coaches' approval**: Approved, Waiting for approval, Not approved,
Approval withdrawn. No ✓ after the words: the chip's colour says it.

**Check-in** is a chip wherever it shows: Here, Not checked in, Scratched,
Missing (an official); "Not yet" only in the day's check-in rows, where
everything is check-in.

**Who and what they do**

- **Officiating** is judging, chairing, recording and marshalling: the comp
  sec's tab (not "Judging and helping"), a person's duties and the
  organiser's page of a person. What someone offers is their **Offer to
  officiate**. The people are **officials** (the organiser's Officials page).
- Panel roles are names, so capitalised standing alone (a chip, a column, a
  list after "·"): Chair, Judge, Recorder, Marshal; in a sentence they're
  lowercase ("plus a recorder and a marshal"). On a person's role chips it's
  **Chair**, not "Can chair".
- A coach named as someone's coach is "Coach: Declan Ward", the name a link
  to their page. A role chip (`WhoChip`) says who did something where it
  isn't otherwise said (added it, left a note, marked it); a sign-off says
  it in its words, so it's "by" and the name.
- The first of a repeated element counts: its note on a card is grey, not
  red. Only what's wrong is red.
- No space before a colon: "Resolved by Organiser: asked UCD to fix it".
