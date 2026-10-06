---
type: Feature
title: Competition entries
description: What organisers, club competition secretaries, members and coaches see when entries are collected for a competition. Organisers get an overview of every entry with its problems, a full view of any one entry, printed cards, an export, optional video proof by link, and optional coach sign-off.
tags: [feature, competitions, clubs, coaches]
generated: { by: claude-code/cli, at: 2026-10-06T11:00:00Z }
sources:
  - id: adr-0004
    resource: ../../docs/adr/0004-server-storage-secret-links.md
    title: ADR 0004 — Server storage with secret links, no accounts
    author: human:jackpardy
---

**Live on tariff.pardy.ie since 2026-10-06**, at `/competitions`; the
calculator doesn't link to it yet. This is card collection, the first item on
the [roadmap](../roadmap.md). Storage, links, clubs, video proof and coach
sign-off are decided in ADR 0004.[^adr-0004]

It's built from storage (`store`), checking (`competitions`), and the pages:
competitions, the organiser's dashboard and each entry, and individual entry
(`comppages.go`); clubs, members and sending (`clubpages.go`); and coaches
(`coachpages.go`) ([routes](../architecture/http-routes.md#competition-pages)).
`/competitions` lists the links a browser has. The pages are on only where
`DATA_DIR` is set ([competition storage](../operations/deploy.md#competition-storage)).

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

# Events and synchro (ADR 0005, step 2)

A competition offers **events**: a discipline at a level.

- **Trampoline** levels as before; **synchro** levels, checked like
  trampoline routines (one routine for the pair); and **tumbling** and **DMT**
  levels the organiser names, which are a level only, not checked yet. The
  organiser picks them when creating the competition and can change synchro,
  tumbling and DMT under Links and settings.
- A gymnast can enter **several events**: a member's page has a section per
  discipline (enter, change, withdraw each on its own), the club page and
  dashboard list each entry under its event ("Synchro BUCS L3", "Tumbling
  Novice"), and an individual enters each discipline separately.
- **Synchro pairs** can be any two gymnasts. The entry names the partner and
  shows a **partner link** to send them; the partner confirms who they are
  with their own member page or entry link (or as having no other entry), so
  the timetable can later check their clashes. Naming a different partner
  needs a new confirmation.

# Officials (ADR 0005, step 3)

**Officials** on the dashboard lists everyone who can judge or help, and what
each discipline's panel needs.

- **Offers.** On their page, a member says what they can do at each
  competition: judge a discipline (up to a level, perhaps chairing), and help
  as a recorder or marshal. The comp sec can correct it, and it goes to the
  competition with every send. An individual offers on their entry.
- **People the organiser adds:** judges or helpers who aren't entering, with
  or without a club.
- **Panels:** each discipline's starts as the FIG Code of Points' (a Chair of
  Judges Panel, 6 execution and 2 difficulty judges), plus a recorder and a
  marshal, all changeable.
- **Who may judge what:** anyone up to the level they say (the default), only
  levels below the one they compete at, or only people the organiser marks
  qualified. The page shows how many can judge and chair each discipline.
- The [officials rota](#officials-rota-adr-0005-step-5) puts them on panels
  around their own turns.

# Officials rota (ADR 0005, step 5)

Planning the timetable also fills each flight's panel from the people who've
offered (roadmap: competitions 4).

- **Never in two places:** no one officiates while competing, or on two
  panels at once. Each seat goes only to someone who may take it: judges
  under the judging rule and up to their level, chairs who said they can
  chair, recorders and marshals who offered.
- **As far as it can:** clubs share judging their own gymnasts fairly (each
  time costs a club more than the last), the work is spread, a panel stays
  together on its area from one flight to the next, helpers who could judge
  are kept for judging, and someone about to compete gets rest first.
- **Rules about people,** each a must or a prefer: someone in a role at an
  event ("Mary is Chair of judges at Elite Women"), someone not officiating
  (at an event or at all), someone officiating on a day only between two
  times. A must keeps that person for that seat, off any flight at the same
  time. Changing these rules asks you to **Assign officials again**, not to
  plan again.
- **The report:** seats no one could take, hand changes that break the rota
  (someone officiating while competing, in two places, or in a role they
  can't take), rules not kept, coaches needed on two areas at once, times
  each club judged its own, and the busiest officials.
- **By hand:** give any seat to anyone who's offered, or empty it. **Assign
  officials again** starts the rota afresh, keeping the flights.
- **Coaches:** planning keeps a coach's gymnasts off two areas at once where
  that can be done (by club and coach name), and reports where it can't.
- **Printed:** each flight's panel on the marshal and chair of judges sheets,
  and the **officials rota**, each person's duties in time order.
- **Published,** members and gymnasts entering on their own see their duties
  on their page.
- **Flights where judges are free:** planning prefers times when enough of
  an event's judges aren't competing to fill its panel, counting both the
  flight being placed and those already beside it.

# Timetable (ADR 0005, step 4)

From the dashboard, **Timetable** plans the competition over the venue's days
(roadmap: competitions 3).

- **Days:** each with a name, start and strict end time, and the areas in use
  that day. A new competition starts with one day, 09:00–18:00.
- **Areas:** named panels, tracks and DMT beds, each for one discipline;
  synchro runs on trampoline areas too. A new competition starts with one area
  per discipline it offers ("Panel 1", "Track 1", "DMT 1").
- **Timings**, per discipline: minutes per competitor (5 for trampoline, 5 per
  synchro pair, 2 for tumbling and DMT), minutes between flights (10) and the
  largest flight. **Rest** between a person's flights (20 minutes), from the
  end of one to the start of the next one's warm-up, is a preference, or a
  must. Both rounds of an event are in one flight, so rest is only between
  events. **Men and women**: which levels ranked separately
  also fly separately.
- **Blocked time:** lunch, awards or an ad hoc event, on a day, at a fixed time
  or anywhere in a window, on chosen areas or all of them. Blocked time can
  need officials (a chair, judges, a recorder, a marshal): the rota staffs it
  from anyone who judges or helps.
- **Rules,** each a **must** or a **prefer**: an event on an area, on a day,
  before another event, or apart from another (not at the same time).
- **Plan** splits each event into even flights, draws running orders (clubs
  spread where easy) and places the flights: never a person in two places,
  every flight inside its day's hours and around the blocks, keeping the
  musts; then as many preferences as it can (rest, an event's flights on one
  area, the organiser's prefers). It tries many orders and keeps the best.
  Where a person's next flight that day starts less than twice the rest after
  their last, they go early in the first flight's running order and late in
  the second's (also when a flight is redrawn).
  A person is a member (by their member link) or, for individuals entering
  several disciplines, by name; a confirmed synchro partner is the same
  person as their own entries.
- **The report:** each day's finish against its end and the time to spare,
  what didn't fit (flights and blocks), anyone resting less than asked, and
  rules broken, and names that look like one person entered twice (the same
  letters ignoring accents and punctuation, or one letter apart in a long
  name). When something doesn't fit, it tries changes one at a time (another
  area, half a minute less per competitor, fewer minutes between flights,
  bigger flights, rest only preferred, or an event capped at the entries
  that fitted) and says which would fit everything.
- **Adjusting:** move a flight to another day or area, redraw its order, move
  a gymnast to another flight or take them out, and place entries made since
  planning. Changing the setup marks the plan out of date until **Plan**
  again; moves are rechecked for clashes and listed as problems.
- **Sheets:** marshal sheets (running orders to tick off) and chair of judges
  sheets (each gymnast's exercises and problems), one area to a page.
- **Publish** shows clubs (each member's row), members and gymnasts entering
  on their own their flight, area, day and warm-up time.

# Coaches (coach link)

Where a competition requires it, each entry needs a coach's **sign-off**
(ADR 0004 Decision 11).[^adr-0004]

- The comp sec adds the club's coaches on the club page; each gets a coach
  link (shown once; **New link** replaces it).
- A member chooses their coach on their own page, or the comp sec does on
  the club page. A coach sees their own members and every member without a
  coach; the club can let **every coach see every member**.
- The coach's page lists each competition's entries they see, with problems
  and sign-off, and how many are waiting. Opening one shows it checked, with
  **Sign off** and **Not yet**, and a note the member and comp sec see.
- A changed entry needs signing off again. The sign-off goes to the
  competition with the entry when the club sends it.
- Unsigned entries can still be sent: the organiser's dashboard has a Coach
  column, counts "Not signed off by a coach" as a problem, and the CSV says
  who signed off.
- An individual's entry page gives a **sign-off link** to send their coach,
  who signs off under their name, without a coach page.
- Printed cards fill in the **Coach** field: the coach who signed the entry
  off, or else the member's chosen or assigned coach.

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

# My competition (roadmap: competitions 5)

From a member's page (each competition) or an individual's entry, **Your
competition** opens one page for the day:

- Every event they're in that has reached the competition (a member's sent
  entries; an individual's entries, matched by name; synchro as either
  partner), with its card's status: checked, problems to sort out (listed),
  or not checked yet.
- Once the timetable is published: each flight, area, day and warm-up, where
  they are in the running order ("3rd of 12"), and about when each routine
  is, worked out from the flight's timings (a guide only).
- What they officiate, and the whole timetable, every day and area, with
  their name in bold and the flights they're in or on the panel of open and
  outlined.

# Built from

- `views.TariffSheet` (`views/sheet.templ`) for printed cards.
- The view screen and level panel (`views/view.templ`, `views/routine.templ`)
  for one entry.
- `requirements.Evaluate`, `requirements.RequiredElements` and the shared level
  check (ADR 0004 Decision 4) for the table's columns.

[^adr-0004]: ADR 0004 — Server storage with secret links, no accounts
