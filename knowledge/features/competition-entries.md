---
type: Feature
title: Competition entries (planned)
description: What organisers, club competition secretaries and members will see when entries are collected for a competition. Organisers get an overview of every entry with its problems, a full view of any one entry, printed cards, an export, and optional video proof by link.
tags: [feature, competitions, clubs, planned]
status: draft
generated: { by: claude-code/cli, at: 2026-10-05T16:30:00Z }
sources:
  - id: adr-0004
    resource: ../../docs/adr/0004-server-storage-secret-links.md
    title: ADR 0004 — Server storage with secret links, no accounts
    author: human:jackpardy
---

**Not built yet.** This page describes the target for card collection, the
first item on the [roadmap](../roadmap.md). Storage, links and clubs are
decided in ADR 0004.[^adr-0004] Built so far (steps 2 and 3 of the ADR's
migration, then 4): storage (`store`), checking (`competitions`), the pages for
creating a competition, the organiser's dashboard and each entry, individual
entry (`comppages.go`), and clubs: creating one, members joining and keeping
their entries, the comp sec's page, and sending (`clubpages.go`,
[routes](../architecture/http-routes.md#competition-pages)). `/competitions`
lists the links this browser has. Step 5 added marking entries checked with a
note, printing cards, the CSV, closing or reopening entries, and deleting
expired data; step 6 video proof (links, which skills need them, and the
organiser's review). They're on only where `DATA_DIR` is set
([competition storage](../operations/deploy.md#competition-storage)), and are
reached at `/competitions`: the calculator doesn't link to them, for now.

# The organiser (competition admin link)

The tariff sheet is only one of the organiser's views: the printable one. The
organiser's first need is an **overview of every entry with its problems**.

## Dashboard

There is one table per level, which can be filtered by club or to "problems
only":

| Gymnast | Club | 1st exercise | 2nd exercise | Requirements | Problems | Video | Sent | Checked |
|---|---|---|---|---|---|---|---|---|
| A. Murphy | UCD | Set 1 | 4.6 | 5/5 ✓ | – | not needed | 3 Mar, 21:04 | ✓ |
| B. Kelly | DCU | Set 2 | 5.9 | 4/5 ✗ | Repeat (element 7) | missing | 4 Mar (changed since) | – |
| C. Ryan | individual | 2.1 | 6.2 | 9/10 ✗ | 10th lands on back | provided | 5 Mar | – |

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

If the competition asks for video, the entry also lists the skills that need
it, the member's link for each exercise (opens in a new tab) with its note,
and **Video OK** or **Need more** (with a note back to the club).

The organiser never edits a routine. A problem goes back to the club as a note.
The club fixes it and sends it again.

An entry the club sent for a member who has since withdrawn it, or left the
club, is marked **Withdrawn** and isn't counted or printed. It goes when the
club sends everyone's entries again: the organiser only sees what the club
sends (ADR 0004 Decision 2).

## Printing and export

- The [tariff sheet](tariff-sheet.md) (competition card) with its details
  filled in from the entry: gymnast, club, level, competition and exercise.
- **Print all** for a level, a club, or the entries not checked yet: one card
  per page, ready for the judges' table.
- **CSV** for the scoring system (name, club, level, tariff per exercise). A
  single PDF of all cards is the cards page saved as PDF from the browser's
  print dialog.

## Video proof

When creating the competition, the organiser chooses whether video proof is
needed: **none**, **for some skills** (described like a requirement, e.g. "any
triple" or "tariff 1.5 or more") or **whole routine**. Videos are links (an
unlisted YouTube video, a Drive file shared with anyone with the link, Vimeo,
Dropbox or OneDrive), never uploaded, and never played inside the app (ADR
0004 Decision 10).[^adr-0004]

# The club's comp sec (club admin link)

The same table, but for the club's members only, by competition. It has
**Send** and **Re-send changed** buttons, shows the organiser's notes, lists
the members (with **New link** for one who lost theirs, which ends the old
one), shows which members are
still missing a required video, and lets the comp sec edit an entry on a
member's behalf.

# A member (personal link)

Each of their entries per competition: the level, the routine(s), the check
result, and whether it has been **sent** or **changed since sent**. Where the
competition asks for video, the entry says which skills need it ("Video needed
for: Triple Back") and has a field for a link per exercise, with a reminder to
make a YouTube video unlisted, not private. They can replace the level or
routine at any time until the deadline.

# Built from

- `views.TariffSheet` (`views/sheet.templ`) for printed cards.
- The view screen and level panel (`views/view.templ`, `views/routine.templ`)
  for one entry.
- `requirements.Evaluate`, `requirements.RequiredElements` and the shared level
  check (ADR 0004 Decision 4) for the table's columns.

[^adr-0004]: ADR 0004 — Server storage with secret links, no accounts
