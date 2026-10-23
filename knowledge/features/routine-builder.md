---
type: Feature
title: Routine builder
description: The calculator page, where users add skills through the picker, search or builder, keep several routines on the phone, check one against requirements, choose its checks, and show a second routine beside it.
resource: https://github.com/jackpardy/tariff-calculator/blob/master/views/page.templ
tags: [feature, routine, calculator, ui]
generated: { by: claude-code/cli, at: 2026-10-04T17:45:00Z }
---

# The page

The builder is one of the routine tools (with the Requirements and compare
pages), which share the competition pages' look: their text sizes, light
buttons with colour only for the main action, notices and state chips, under
a slim banner (`views/tool.templ`) naming the app, with **Routine Builder**
and **Requirements** as links. **Add a skill** is above **Your routine** at
every width, as before the look-and-feel pass (the owner kept that layout,
2026-10-22; the layout is being redesigned separately). The page uses
Bulma's container width (1,152 px at 1,216 px, 1,344 px at 1,408 px); the
Requirements and compare pages are the competition pages' 60rem.

# Adding a skill

The "Add a skill" card shows one skill with a live summary: its name, FIG
notation and tariff. There are three ways to set it:

- **Picker**: big buttons by category, with tuck, pike or straight where the
  shape matters ([skill catalog](../domain/skill-catalog.md)). Body landings
  start with the drops from the feet (fixed tiles); then one group, "From a
  body landing", by where they land (To Feet, ½ Twist To Feet, To Seat…), and a
  tile takes off from where the skill before it landed: the last skill of the
  routine Add puts it in, or the one before the skill being edited (feet at
  the start). The form posts that skill (`previous_skill`, from
  `previousSkill()` in `static/js/app.js`) and the server chooses the version,
  so after a Seat Drop "½ Twist To Seat" is a Seat Half Twist To Seat (Swivel
  Hips). The card says why ("From seat, after Seat Drop · also called Swivel Hips"). The set routine
  maker on the Requirements page posts its list's last skill the same way,
  though its compact form has only the search and the builder.
- **Search**: by name, alias or FIG notation.
- **Builder**: rotation, a twist box per phase, take-off, direction, seat
  landing and shape. The server decides which twist boxes exist and whether
  shape is shown, re-rendering the inputs on change
  ([rendering](../architecture/rendering.md)).

The user can also give the skill an optional **custom name** (at most 60
characters). It is shown alongside the official name, which is always derived
by the server.

## Changing a skill in its card

A routine card's **Edit** opens that card to change the skill in place
(2026-10-22): the one skill form moves out of Add a skill into the card,
under its head, as wide as the card, loaded with the skill (`placeEditor` in
`static/js/app.js`; `#editor-park` holds it while the routine is redrawn).
The card's own head shows the skill as it will be, live (2026-10-23, the
owner's ask): its number, the new name (or label, with the name after it),
notation and tariff, read off the form as the server last drew it
(`readDraft`), tinted, with "not saved" once it differs from the saved
skill; **Update** and **Cancel** sit on the head in place of ↑, ↓, Edit and
✕ (the two versions of the head swap by the card's `drafting` class, so they
never show together). Under the head come the **shapes** (where the shape
matters, with their tariff differences) and **Add a label** (`editFields`,
sent back out of band with each change), then **Change to another skill**
(a search; a result goes straight on the head, keeping the label), then
**Build or adjust this skill** (open only for a skill without a name, as
that's how it was made). The skill card shows only while Build or adjust is
open, as the readout of what's being built, without buttons of its own. The
picker's tabs are for adding and stay hidden; the card's details (the skill
as saved) wait until it's done. Update saves it and puts the focus back on
the card's Edit; Cancel, Escape (in the form or on Update/Cancel) or Edit
again puts the head back and leaves the skill as it was; Edit on another
card moves the form there, the first left as it was. Enter in the search
puts the focus on its first result, Enter again puts that on the head with
the focus on Update, and a third Enter updates. While a card is being changed, Add a skill
says so in one line ("Changing skill 7 in its card below: Update or Cancel
there, then carry on adding.", with "of" the routine when there are two
columns) and comes back as it was after. It works the same in either column
side by side, in Levels mode and on a phone.

Nothing moves under the finger: as Add a skill shrinks (and grows back), the
card being changed is held where it is on the screen, the page scrolling by
as much (`holdPlace`, `keepHeld`; the browser's scroll anchoring is off
meanwhile). Where the page can't scroll that far, Add a skill keeps the room
until the form comes back, or room is made under the builder and taken away
once it's scrolled out of sight. Once open, a form that runs off the screen
is brought up, once and gently (not with reduced motion), only as far as
fits with the card's head still in sight.

**How it's judged**, on the "Add a skill" card and in each routine card's
details, opens a pop-up with the execution deductions that can apply to that
skill and what each means, worked out from the skill's attributes
([execution judging](../domain/execution.md)). On the routine's last element
(the 10th, or the last of a shorter routine) it adds the landing. htmx adds
the pop-up to the end of the page (`GET /judging`); `static/js/judging.js`
closes it with ✕, the background or Escape. The compare page's cards have it
too.

**Add to** chooses where Add puts the skill, from every saved routine; it
starts on the routine on screen and goes back to it when the routine on screen
changes (in Levels mode, the voluntary's routine, so a coach can look at a set
and add to the voluntary). Adding to a routine that isn't on screen saves it
there and says so. On a phone the card is compact (Add to above, then the
shapes and Add on one row); on wider screens it's laid out in full: the shapes
under the name, the label, Add to and a full-width Add button.

# Saved routines

Routines are kept in the browser, in `localStorage['trampolineRoutines']` as
`{current, routines: [{id, name, skills, requirements?, checks?}]}`
(`static/js/routines.js`). The single routine that older versions saved under
`trampolineRoutine` becomes "Routine 1". Each routine stores canonical
[skill JSON](../domain/skill-model.md#schema), never rendered HTML. Names and
tariffs are re-derived by the server on every render, so routines saved by
older versions show corrected names.

Each card's head is a button that opens its details (so it works from the
keyboard): the number and name, the FIG notation, a **!** while a problem is
folded away, and the tariff. **↑**, **↓**, **Edit** and a red **✕** (Remove)
sit at the right, 34 px tall (36 px on a phone). Remove is in sight, not in
the opened card, and a ✕ at every width (the owner, 2026-10-22); it doesn't
ask first, but the message
saying so has **Undo** (for 6 seconds), which puts the skill back where it was,
in its own routine even if another is now on screen. The cards can be
reordered by drag (SortableJS) or with ↑/↓. Every change posts the routine to
`POST /routine`, which re-renders the cards, flags, messages and total.

Every tariff is shown to one decimal place, as the Code of Points' values
are: on the cards, in the details, the total ("Total tariff 5.8") and the
line saying what everything would add up to ("Counting every element: 6.5").
A card's message, and the list of the routine's problems under the total,
are in the same words as the tariff sheet and the competition pages
(`skills.ElementNote`, `skills.RoutineNotes`), e.g. "Element 10 repeats an
earlier element, so its difficulty doesn't count again."; problems are in the
attention state's colours. Routine lists name each routine "Ciara voluntary ·
10 skills" (`RoutineStore.label`), so a name like "Copy (2)" doesn't read
"(2) (10)".

# Requirements and checks

A routine can be checked against built-in or custom
[requirements](../requirements/framework.md). How many are met shows straight
under Check against, as a state chip ("1 of 7 met", done when all are), with
**See which** jumping to the list under the total, where each says ✓ or ✗;
the elements involved are highlighted. "Check against" is a
button showing what's chosen; it opens a panel under it with a search
(by organisation, level or name, ignoring case and accents; Enter takes the
first match), **Yours** (those kept on your page first, then this browser's),
**Used lately** (the last four built-in ones chosen, kept in the browser as
`recentRequirements`), then each organisation, closed until opened and all
open while searching (`views/chooser.templ`). It lists each
level's voluntary requirements by group, named after the level ("BUCS L7"), or
by exercise when its two voluntaries differ ("BUCS L1 · first exercise",
"BUCS L1 · second exercise"); requirements shared by levels are listed
once, by their own name ("BG Regional L4 13+ · first exercise").
Requirements can be written beside the routine (roadmap's requirements
workflow, step 4): **+ Write your own requirements** at the foot of Check
against's panel, **Copy and change** for a built-in set chosen, or
**Change** for one of yours, opens the requirements editor in a panel under
Check against (`views.requirementsWriter`). The editor posts the routine
with it (`routineData`), so each rule's card says **Met** or **Not met**
with what was found, and the routine's own checks below use the draft while
it can be read; changing the routine re-posts the editor. **Save and check
against these** saves the set as the Requirements page does (in the browser,
and on the person's page if it's kept there) and checks the routine against
it; **Cancel** goes back to what it was checked against.
Set routines aren't requirements to check against: **Start from a set**
beside "+ New", the same kind of chooser, uses one as a starting point
([set routines](../requirements/set-routines.md)). Routines checked against a
set routine by an earlier version stop being checked.

[Levels](../requirements/levels.md) have their own mode (Routines | Levels at
the top of the builder): pick a level under **+ New level**, a chooser like
Check against's (search, Yours, then each organisation's; Enter takes the
first match), to see its set routines as prescribed,
and link a routine to its voluntary (new, already built, or started from a
set). Side by side works for a level's tabs too.

## Checks

Each routine has **checks**: whether difficulty is scored, whether repeats are
flagged, and how many elements score. They start from the requirements'
[scoring settings](../requirements/framework.md#scoring), and a coach can
change them for this routine. They are stored as
`checks: {difficulty, repeats, scored}` and become `ValidateOptions`
([routine validation](../domain/routine-validation.md#options)). When only
some elements score, the coach can tick which ones.

# Side by side

A second saved routine can be shown beside the current one. Both columns can be
edited. The second routine's cards that differ from the first's at the same
number are shaded (only the second's: shading the same row in both said
nothing more, the owner, 2026-10-22), unless **Shade skills that differ from
the first**, beside Side by side with, is unticked, for when two routines are
just up together; that's kept in the browser (`builderShadeDiffs`). On a phone the columns become
tabs. The routine side by side is remembered with the routines (`beside` in
`trampolineRoutines`), so coming back from the view screen, a reload, or
Levels mode and back keeps it. The standalone [compare page](compare.md) does
a read-only version of the same.

# Other pages

From the builder: the [view screen](view.md), the
[tariff sheet](tariff-sheet.md), the [compare page](compare.md), the
requirements page and [sharing](sharing.md).

When a deploy happens while the page is open, a bar offers to refresh
([static assets](../architecture/static-assets.md)).
