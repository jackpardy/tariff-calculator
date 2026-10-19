---
type: Feature
title: Routine builder
description: The calculator page, where users add skills through the picker, search or builder, keep several routines on the phone, check one against requirements, choose its checks, and show a second routine beside it.
resource: https://github.com/jackpardy/tariff-calculator/blob/master/views/page.templ
tags: [feature, routine, calculator, ui]
generated: { by: claude-code/cli, at: 2026-10-04T17:45:00Z }
---

# Adding a skill

The "Add a skill" card shows one skill with a live summary: its name, FIG
notation and tariff. There are three ways to set it:

- **Picker**: big buttons by category, with tuck, pike or straight where the
  shape matters ([skill catalog](../domain/skill-catalog.md)). Body landings
  start with the drops from the feet (fixed tiles); then, from a body landing,
  they're grouped by where they land (To feet, To seat, To back, To front), and a
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
by the server. A routine card's **Edit** button loads that skill back into the
card for editing.

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

The cards can be reordered by drag (SortableJS) or with the ↑/↓ buttons. Every change
posts the routine to `POST /routine`, which re-renders the cards, flags,
messages and total.

# Requirements and checks

A routine can be checked against built-in or custom
[requirements](../requirements/framework.md). The results show under the
routine, and the elements involved are highlighted. "Check against" is a
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
the top of the builder): pick a level to see its set routines as prescribed,
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
edited, and the cards mark where the two differ. On a phone the columns become
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
