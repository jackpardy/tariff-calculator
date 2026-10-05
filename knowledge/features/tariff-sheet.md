---
type: Feature
title: Tariff sheet
description: A printable competition card for the current routine, showing elements in order with FIG notation, difficulty, required-element ticks and a judge column, plus optional detail fields.
resource: https://github.com/jackpardy/tariff-calculator/blob/master/views/sheet.templ
tags: [feature, print, tariff-sheet, competition-card]
generated: { by: claude-code/cli, at: 2026-10-05T15:00:00Z }
---

# What it shows

`GET /tariff-sheet` is a plain page (a link, so reload works and it isn't
blocked as a pop-up). On load it posts the current routine, with its
requirements and checks, to `POST /tariff-sheet`, which renders the card (CoP
§6.1–6.2):

- One row per element, padded to at least ten. Each row has No., Element (Name
  and FIG), Difficulty, **Req. \*** and Judge.
- Only elements that score have a difficulty value. Other rows say why they
  add nothing (repeat, interrupted, past the tenth, not scored, or repeating
  an element whose difficulty carried over from the first exercise of a
  [level](../requirements/levels.md#carry-over)).
- **Req. \*** is ticked for elements that satisfy a requirement
  (`requirements.RequiredElements`). These are the special requirements a
  first exercise must star.
- Fill-in detail fields at the top, typed on the page before printing and never
  stored.
- Warnings about the routine show on screen only, not in print.

# Options

"Show on sheet" toggles skill names, the Req. \* column, the Judge column and
each detail field. Each one hides part of the sheet both on screen and in print,
and the choices are remembered in this browser (`static/js/sheet.js`,
`localStorage['sheetShow:*']`).

The toolbar with Print and "Show on sheet" stays at the top of the screen as
the sheet scrolls, so the options are always in reach on a phone.

Printing uses print CSS (`static/css/sheet.css`) and the browser's own print or
"Save as PDF". Server-generated PDFs (Gotenberg, [ADR 0001](../../docs/adr/0001-architecture.md) §7)
are deferred. Printing from a real phone hasn't been tested yet
([open questions](../open-questions.md)).

Related: [routine validation](../domain/routine-validation.md),
[requirements](../requirements/framework.md).
