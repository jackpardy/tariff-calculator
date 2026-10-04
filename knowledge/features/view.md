---
type: Feature
title: View screen
description: A full-screen, scroll-free view of a routine, or a level's pair of routines, with a Show menu to turn each kind of information on or off.
resource: https://github.com/jackpardy/tariff-calculator/blob/master/views/view.templ
tags: [feature, view, display, level]
generated: { by: claude-code/cli, at: 2026-10-04T23:30:00Z }
---

# What it's for

Seeing a routine at a glance: on a phone or tablet at the side of the
trampoline, or on a bigger screen in the gym. Everything fits on one screen
without scrolling, and nothing is shown that the coach hasn't asked for.

# Opening it

The **View** button in the [routine builder](routine-builder.md) opens
`/view?routine=<id>` for the current routine in a new tab. If the routine is
one exercise of a [level](../requirements/levels.md) and has a partner, both
routines show, in exercise order. A select in the bar switches to another
saved routine (pairs are listed as "Q1 + Q2"), and the choice is kept in the
URL. The view re-renders when the routines change in another tab, so it can
stay open beside the builder.

# Layout

`POST /view` (`handleView`) takes what `/routine` takes, plus `routineName`,
and renders `views.RoutineDisplay`: one column per routine. Every column has the
same number of rows, a full exercise (10) or more, so the pair lines up. The rows
share the column's height, and their text is sized with container query units
(`static/css/view.css`), so 10 or 20 elements fill the screen at any size. A
pair is side by side on a landscape screen and stacked on a portrait one. It
follows the device's light or dark mode.

Each column has the routine's name, its level and exercise, how many
requirements it meets, and its total difficulty ("no difficulty" when it isn't
scored). Each row has the number, the skill (the coach's label if it has one),
FIG notation, a star if it meets a requirement, its difficulty and a warning
mark. Elements that don't count are struck through, including a repeat of an
element whose difficulty carried over from the first exercise. Elements that
don't score when only some do are greyed.

# The Show menu

Each part can be turned off. The choices are kept in the browser
(`localStorage['viewShow:hide-<key>']`, `static/js/view.js`). They toggle
`hide-<key>` classes on `#view`, so they apply instantly.

| Option | Default |
|---|---|
| Skill names | on |
| FIG notation | on |
| Difficulty of each skill | on |
| Total difficulty | on |
| Level and exercise | on |
| The other exercise | on |
| Requirements met (and the stars) | off |
| Warnings | off |

**Full screen** uses the Fullscreen API where the browser has one. iPhone Safari
doesn't, so the button is hidden there. Adding the page to the home screen gets
close.
