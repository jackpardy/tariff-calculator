---
type: Feature
title: Competition entries (planned)
description: What organisers, club competition secretaries and members will see when entries are collected for a competition. Organisers get an overview of every entry with its problems, a full view of any one entry, printed cards and an export.
tags: [feature, competitions, clubs, planned]
status: draft
generated: { by: claude-code/cli, at: 2026-10-05T16:00:00Z }
sources:
  - id: adr-0004
    resource: ../../docs/adr/0004-server-storage-secret-links.md
    title: ADR 0004 — Server storage with secret links, no accounts
    author: human:jackpardy
---

**Not built yet.** This page describes the target for card collection, the
first item on the [roadmap](../roadmap.md). Storage, links and clubs are
decided in ADR 0004.[^adr-0004]

# The organiser (competition admin link)

The tariff sheet is only one of the organiser's views: the printable one. The
organiser's first need is an **overview of every entry with its problems**.

## Dashboard

There is one table per level, which can be filtered by club or to "problems
only":

| Gymnast | Club | 1st exercise | 2nd exercise | Requirements | Problems | Sent | Checked |
|---|---|---|---|---|---|---|---|
| A. Murphy | UCD | Set 1 | 4.6 | 5/5 ✓ | – | 3 Mar, 21:04 | ✓ |
| B. Kelly | DCU | Set 2 | 5.9 | 4/5 ✗ | Repeat (element 7) | 4 Mar (changed since) | – |
| C. Ryan | individual | 2.1 | 6.2 | 9/10 ✗ | 10th lands on back | 5 Mar | – |

- Counts at the top: entries per level and club, how many have problems, how
  many aren't checked yet, and any late ones.
- Each club's rows show when the club last sent its entries.
- Every column comes from the same checks as the
  [routine builder](routine-builder.md): [routine validation](../domain/routine-validation.md),
  [requirements](../requirements/framework.md) and the
  [level](../requirements/levels.md) carry-over between exercises.

## One entry

Both exercises are shown side by side, as in the builder's Levels mode: the
elements, FIG notation, tariff, flags (repeats, transitions, interruptions,
carry-over) and each requirement with ✓ or ✗. Two buttons:

- **Mark checked**, with an optional note. The note is shown to the club's comp
  sec and the member.
- **Print card.**

The organiser never edits a routine. A problem goes back to the club as a note.
The club fixes it and sends it again.

## Printing and export

- The [tariff sheet](tariff-sheet.md) (competition card) with its details
  filled in from the entry: gymnast, club, level, competition and exercise.
- **Print all** for a level, a club, or the entries not checked yet: one card
  per page, ready for the judges' table.
- **CSV** for the scoring system (name, club, level, tariff per exercise), and
  a single PDF of all cards.

# The club's comp sec (club admin link)

The same table, but for the club's members only, by competition. It has
**Send** and **Re-send changed** buttons, shows the organiser's notes, lists
the members with their personal links (to send again), and lets the comp sec
edit an entry on a member's behalf.

# A member (personal link)

Each of their entries per competition: the level, the routine(s), the check
result, and whether it has been **sent** or **changed since sent**. They can
replace the level or routine at any time until the deadline.

# Built from

- `views.TariffSheet` (`views/sheet.templ`) for printed cards.
- The view screen and level panel (`views/view.templ`, `views/routine.templ`)
  for one entry.
- `requirements.Evaluate`, `requirements.RequiredElements` and the shared level
  check (ADR 0004 Decision 4) for the table's columns.

[^adr-0004]: ADR 0004 — Server storage with secret links, no accounts
