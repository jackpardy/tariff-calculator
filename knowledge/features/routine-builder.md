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
and **Requirements** as links. On a laptop or wider (1,024 px up) **Add a
skill** is on the left and **Your routine** on the right, so the routine
starts on the first screen; on a phone, and while two routines are side by
side on any screen (they need the width), Add a skill is above. The page uses
Bulma's container width (1,152 px at 1,216 px, 1,344 px at 1,408 px); the
Requirements and compare pages are the competition pages' 60rem.

# Adding a skill

The "Add a skill" card shows one skill with a live summary: its name, FIG
notation and tariff. There are three ways to set it:

- **Picker**: big buttons by category, with tuck, pike or straight where the
  shape matters ([skill catalog](../domain/skill-catalog.md)).
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

Each card's head is a button that opens its details (so it works from the
keyboard): the number and name, the FIG notation, a **!** while a problem is
folded away, and the tariff. **↑**, **↓** and **Edit** sit at the right, each
at least 40 px tall (36 px wide on a phone). **Remove** is in the opened card,
away from them, beside How it's judged; it doesn't ask first, but the message
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
