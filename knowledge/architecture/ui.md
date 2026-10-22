---
type: Architecture
title: The look
description: The shared look rules for every page, so the app reads as one product.
tags: [architecture, ui, css, look]
---

# Routine tools

The Routine Builder, the Requirements page and the compare page are one family
with the competition pages (the look-and-feel pass, 2026-10-19). The tariff
sheet and the view screen keep looks of their own, for paper and a big screen,
but use the same words and the same main-action colour.

- **The banner.** A slim banner (`toolBanner` in `views/tool.templ`, about
  45 px; one line on a phone) names the app and links **Routine Builder** and
  **Requirements**, the page's own underlined (on a phone only the other one
  shows). On the builder the app's name is the page's `<h1>`; elsewhere it's a
  link home and the page has its own `h1.title.is-4`, as a competition page
  has. White on `#485fc7` (checked by `TestToolBannerContrast`). No hero and
  no footer.
- **Width.** The Requirements and compare pages are the competition pages'
  60rem (`.container.is-narrow-tool`). The builder keeps Bulma's container
  (1,152 px at a 1,300 px window), for two routines side by side.
- **Headings.** Sections are `h2.title.is-5` with no box around them, a list
  with its count in grey after a dot ("Your levels · 2") and its one button at
  the right (`.tool-list-head`), as `listSection` is. A working panel that
  opens (the requirements and level editors) stays a box.
- **Buttons.** Light (plain) buttons for everything but the one main action
  of a form (Add to routine, Save, Use in this level), which is
  `is-primary`. Links to other views of the same thing (View, Tariff sheet,
  Compare) are light buttons. Destructive actions are a menu's last item,
  after a rule, in red text, or `is-danger is-outlined` in an opened card.
- **Rows of saved things.** The same buttons in the same order on every row:
  the main one (Edit), Share, then **More** (`rowMore`, the builder's
  `routine-menu` pop-up) holding the rest, Delete last.
- **Asking.** Never `confirm()`. Something that can be undone is done at once
  with **Undo** on its message (removing a skill); something that can't asks
  on the page where it was pressed: on the row (`deleteAsk`), beside the Save
  pressed (`saveAnywayAsk`), or the builder's `askUser` dialog for whole
  routines.
- **States.** Requirements met are a state chip (`metChip`: done when all
  are, attention otherwise); a routine's problems are a list in the attention
  colours; a card's problem is a small **!** in them; invalid cards have a
  stopped-coloured edge, repeats an attention-coloured one. No Bulma
  `tag is-success`/`is-danger` or `has-text-*` states (`paletteOnly` in
  `views/status_test.go` covers these templates).
- **Words.** A routine's problems are said one way everywhere:
  `skills.ElementNote` on a card, `skills.RoutineNotes` for the list (the
  builder, the sheet, compare, the competition pages). Tariffs are one
  decimal place. A 400's message is the rest of a sentence, with no
  "Bad Request:" before it.
- **Tap targets.** Card buttons are at least 40 px tall (36 px wide on a
  phone); a card's head is a `<button>` with `aria-expanded`, its arrow the
  same triangle as a `<details>` summary's.
- **Opening things.** Choosing from a long list is a chooser
  (`views/chooser.templ`: a button showing the choice with a down arrow, a
  panel with a search): Check against, Start from a set, and Levels mode's
  New level. Not a search box beside a select.
