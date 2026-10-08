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

## Helpers' links, history and concerns (ADR 0009)

Under Links and settings the organiser makes **links for helpers**, each
named (who it's for) and of a kind: **checking cards** (see the entries,
mark cards checked with notes, review videos), **chairs of judges** (see
the entries, print the chair of judges, score and marshal sheets and the
panel timeline), **timetable and officials** (plan, change and publish
them, Notify now, see the entries) and **everything but links and
deleting** (a co-organiser). Each is a secret link, kept as a hash and
shown once; **Remove** stops it. Up to 20. Every admin route goes through
one gate (`allowed` in `access.go`); pages show only what the link can do,
and say whose link it is.

**History** (`/history`) lists every change made through any admin link,
latest first: when, which link (its name, "Organiser" for the admin link)
and what ("Marked a card checked: Ann Ryan · BUCS L3"); the latest 2,000
are kept.

**Concerns:** anyone with an admin link can **flag a concern**, on an entry
or generally. The dashboard lists them (open first, with who and when), for
the organisers only; the organiser and co-organisers mark each
**Resolved**, with a note. The demo has helpers' links (printed by `demo`),
three cards checked and two concerns.

## Entry fees

**Fees** (the dashboard's button, for the organiser and co-organisers;
roadmap 2026-10-08) sets what's charged: euro or sterling, an amount per
entry for each discipline offered (a synchro pair is one entry), an
optional fee once per club, and how to pay (printed on every invoice).
Removed, held and withdrawn entries aren't charged (`Fees.Invoice`). The
page lists each club, and each individual's entry, with entries, due, paid
and balance ("Paid" once settled), and totals; the organiser records each
payment (amount and note, with who recorded it) and can remove one made by
mistake. Each has an **invoice** to print: a line per entry and the club
fee, payments, what's left and how to pay. A club's comp sec sees "Fees:
€49 · €25 paid · €24 to pay" on each competition, with the invoice; an
individual on their entry. No money goes through the tools. The demo
charges €12 an entry (€15 synchro, €10 tumbling and DMT) and €25 a club,
with some clubs paid and some part paid.

## Going live

A new competition starts **private** (roadmap 2026-10-07), unless the form
says to open entries now or at a set time. While it's private the organiser
sets it up; the club and individual links already show its name, date and
when entries open ("Entries open Monday 1 February 2027, 09:00", or
"Entries aren't open at the moment"), so they can be shared early, but
nothing can be entered, sent, changed, withdrawn or signed off
(`store.ErrNotOpen`). The dashboard's **Go live now** opens entries, or a
date and time opens them then (before the deadline); **Make private (pause
entries)**, under Links and settings, stops them again, keeping the entries
already made. Entries are open while the competition is live and before
its deadline (`Competition.Open`); the time is `live_at`, empty while
private. Competitions made before this went live when they were made.

## Dashboard

There is one table per level (event), each folding away under its heading,
which says how many are shown ("12 of 96"); a list at the top jumps to each.
Levels start folded, with **Expand all** and **Collapse all**; the browser
remembers which were left open, and a search or filter opens the levels it
finds people in. Finding entries (roadmap 2026-10-08), all in
the page's address so a view can be kept or shared:

- **search** (`q`): every word in the gymnast's or synchro partner's name,
  ignoring case, accents and punctuation (`competitions.NameMatches`);
- **filters**: club, coach (who signed off, else the member's coach; or
  none), checked, signed off, video (missing, to review, need more, OK),
  men or women, problems only;
- **sort**: by club (as stored), gymnast, latest sent or coach;
- **print and CSV what's shown**: the cards and the CSV take the same
  filters.

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

## Removing and holding entries

Each row of the dashboard has a tick box (and each level's header ticks the
whole table, so filtering by club takes a whole club). Under **Remove or
hold entries** the organiser chooses:

- **Remove**: the entry is taken out and can't be sent or changed again
  (a club's Send skips it; an individual's page can't change it) until the
  organiser **restores** it.
- **Hold, asking for changes**: taken out until the club or gymnast changes
  it and sends it again; it then shows "Changed and sent again: waiting for
  you", and **Accept back in** returns it.

Either way there's an optional reason, shown to the club, the gymnast or
both (an individual always sees theirs). Removed and held entries are left
out of `Store.Entries`, so out of every count, card, CSV, timetable, rota
and "My competition", and listed at the end of the dashboard under
**Removed and on hold**, to restore or accept. The club page and the
member's and individual's pages say what the organiser did ("Removed by
the organiser", "On hold: the organiser asks for changes", "On hold:
changed, send it again", "Changed and sent again: waiting for the
organiser") with the reason where it's for them.

## Video proof

When creating the competition, the organiser chooses whether video proof is
needed: **none**, **for some skills** (described like a requirement, e.g. "any
triple" or "tariff 1.5 or more") or **whole routine**. Videos are links (an
unlisted YouTube video, a Drive file shared with anyone with the link, Vimeo,
Dropbox or OneDrive), never uploaded, and never played inside the app (ADR
0004 Decision 10).[^adr-0004]

# Events and synchro (ADR 0005, step 2)

A competition offers **events**: a discipline at a level.

- **Trampoline** levels as before; **synchro** levels, where a pair does one
  routine, the level's voluntary (its second exercise, or its only one),
  checked like a trampoline routine; and **tumbling** and **DMT**
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
  needs a new confirmation. Once confirmed, the pair's entry shows under
  **Synchro as a partner** on the partner's page (member or individual),
  their coach's (unless they already see the entrant) and their club's comp
  sec's (unless the entrant is their own member), whether or not that club
  is entered (`PairEntries`): as checked, with its sign-off, sent or not,
  and once published, the flight. Only the entrant's side changes it. Both
  gymnasts' entries carry the pair-level warning the dashboard gives
  ("… compete individually, so as a pair they do BUCS L4 in synchro").
- **Paired synchro levels:** the organiser can pair synchro levels into one
  event, one a line ("BUCS L1 + BUCS L2" makes "Synchro BUCS L1/L2"),
  flighted, ranked, judged and timetabled as one. Each pair says which level
  they're doing ("BUCS L1/L2 (doing BUCS L1)", kept as the entry's
  `choice`), and their routine is checked against it. Pairings are set when
  creating the competition or under Links and settings; entries made before
  a level was paired need re-entering in its event.
- **A pair's level:** a pair does the level they both compete at
  individually, or the easier of two a level apart (by the competition's
  order of levels); more than a level apart, they usually can't pair. Once
  the partner has confirmed and both have individual trampoline entries, a
  synchro entry at another level is flagged on the dashboard, first among its
  problems ("A (BUCS L3) and B (BUCS L4) compete individually, so as a pair
  they do BUCS L4 in synchro, not BUCS L3").

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
  marshal, all changeable. Trampoline and synchro can add HD (horizontal
  displacement) judges where no machine measures it; none by default. Synchro starts
  with 2 synchronisation judges (the Code's §18.1 leaves synchronisation to
  a machine, which student competitions don't have), any of its judges able
  to take the seat; set 0 where a machine measures it.
- **Who may judge what:** anyone up to the level they say (the default), only
  levels below the one they compete at, or only people the organiser marks
  qualified. Both go by the **order of levels**, easiest first: built-in
  levels by their rank (BUCS lists its levels hardest first, so they're
  turned round), a coach's own levels after them, and tumbling and DMT's as
  typed. The organiser moves any level up or down under Links and settings;
  competitions from before the order (migration 8) are put in order when read
  until a level is moved. The page shows how many can judge and chair each discipline.
- The [officials rota](#officials-rota-adr-0005-step-5) puts them on panels
  around their own turns.

# Officials rota (ADR 0005, step 5)

Planning the timetable also fills each event's panel from the people who've
offered (roadmap: competitions 4): once for all of its flights, so everyone
in it is judged by the same people ([ADR 0006](../../docs/adr/0006-one-panel-per-event.md)).
A seat goes to someone free for the whole event. The organiser can let
**recorders change** or **marshals change** between an event's flights
(each on its own, both off by default, on the officials page); those seats
are then filled flight by flight.

- **Never in two places:** no one officiates while competing, or on two
  panels at once. Each seat goes only to someone who may take it: judges
  under the judging rule and up to their level, chairs who said they can
  chair, recorders and marshals who offered.
- **As far as it can:** clubs share judging their own gymnasts fairly (each
  time costs a club more than the last), the work is spread, a panel stays
  together on its area from one event to the next, helpers who could judge
  are kept for judging, and someone about to compete gets rest first.
- **Rules about people,** each a must or a prefer: someone in a role at an
  event ("Mary is Chair of judges at Elite Women"), someone not officiating
  (at an event or at all), someone officiating on a day only between two
  times. A must keeps that person for that seat, off any flight at the same
  time. Changing these rules asks you to **Assign officials again**, not to
  plan again.
- **The report:** seats no one could take, a seat for the whole event it's
  on (counted, with the panels they're on: "11, on 3 panels"), manual changes that break the rota
  (someone officiating while competing, in two places, or in a role they
  can't take), rules not kept, coaches needed on two areas at once, times
  each club judged its own, and the busiest officials.
- **Manually:** give any seat to anyone who's offered, or empty it; the change
  goes on the event's other flights the same person had that seat for.
  **Assign officials again** starts the rota afresh, keeping the flights.
- **Coaches:** planning keeps a coach's gymnasts off two areas at once where
  that can be done (by club and coach name), and reports where it can't.
- **Printed:** each flight's panel on the marshal and chair of judges sheets;
  the recorders' **score sheets** (asked 2026-10-08, as ISTO records scores
  online and on paper): a landscape page a flight, its panel named, a row
  per gymnast (or pair) in running order, and for each routine a column per
  execution judge, D, each HD and synchronisation judge the panel has,
  penalty and total, then the total and place, all left to fill in. The
  organiser prints them; an official on a flight's panel can also open that
  flight's sheet on their phone (**Score sheets for your panels**, beside
  their duties, once published). And the **officials rota**, each person's
  duties in time order, one line
  for an event ("Friday 09:30–15:15 · Panel 2 · BUCS L7 Men · all 5
  flights · Execution judge").
- **Published,** members and gymnasts entering on their own see their duties
  on their page.
- **Flights where judges are free:** planning prefers times when enough of
  an event's judges aren't competing to fill its panel, for each flight and
  for the whole event, counting both the flights being placed and those
  already beside them.

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
  from anyone who judges or helps. **Events can run across breaks** lets an
  event pause for blocked time on its area and finish afterwards; otherwise
  each event is all before a break or all after it (the default).
- **Rules,** each a **must** or a **prefer**: an event on an area, on a day,
  before another event, or apart from another (not at the same time).
- **Plan** splits each event into even flights, draws running orders (clubs
  spread where easy) and places each event's flights as one **run**, back to
  back on one area and day, so one panel can judge them all
  ([ADR 0006](../../docs/adr/0006-one-panel-per-event.md)): never a person
  in two places, every flight inside its day's hours and around the blocks,
  keeping the musts; then as many preferences as it can (rest, the
  organiser's prefers). Flights are numbered in the order they run. It tries
  many orders and keeps the best: events a must ties to a day or area
  first, then the longest; what didn't fit goes first next time; and a
  break with a window (lunch between 12:00 and 14:00) at different times in
  it.
  Where a person's next flight that day starts less than twice the rest after
  their last, they go early in the first flight's running order and late in
  the second's (also when a flight is redrawn).
  A person is a member (by their member link) or, for individuals entering
  several disciplines, by name; a confirmed synchro partner is the same
  person as their own entries.
- **The report:** for each day, when its flights end, when it finishes
  (blocked time such as awards included) against its end, and the time free
  after the last flight that isn't blocked off for the whole venue (room for
  more flights; the simulation shows it too),
  what didn't fit (flights and blocks), anyone resting less than asked, and
  rules broken, and names that look like one person entered twice (the same
  letters ignoring accents and punctuation, or one letter apart in a long
  name). When something doesn't fit, it tries changes one at a time (another
  area, half a minute less per competitor, fewer minutes between flights,
  bigger flights, rest only preferred, events running across breaks, or an
  event capped at the most entries that fit, a flight's worth fewer at a
  time) and says which would fit everything.
- **Adjusting:** **Move event** moves all of a flight's event to another day
  or area, after what's there (clear of breaks unless events can run across
  them); **Earlier** swaps a flight with the one before it in its event;
  redraw a flight's order, move a gymnast to another flight or take them
  out, and place entries made since planning. Changing the setup marks the
  plan out of date until **Plan** again; moves are rechecked and listed as
  problems: clashes, and an event whose flights aren't back to back on one
  area, or run either side of a break that isn't allowed (timetables
  planned before ADR 0006 show these until planned again).
- **Sheets:** marshal sheets (running orders to tick off) and chair of judges
  sheets (each gymnast's exercises and problems), one area to a page.
- **Panel timeline:** time running down at an even scale (a grid row a
  minute), so every area and day lines up by time. Two views:
  - **Panel timeline:** every day side by side, a column per area, each
    flight (warm-up to finish, how many gymnasts) and blocked time. Prints
    on A3 landscape, two pages for the demo's weekend.
  - **Timeline with officials:** a sheet for each day's area, with a column
    for the flight and one for each seat on its panel (Chair, D1, D2, HD1,
    E1…, Rec, Mar: as many of each as any of its flights has), a name to a
    cell ("—" for a seat no one could take). Each sheet runs over its own
    day's hours and prints a page each.

  Flights moved manually onto each other are outlined in red. Both download
  as CSV (`timeline.csv`): a row per flight or block on each area, in time
  order, with each role's officials and their clubs.
- **Publish** shows clubs (each member's row), members and gymnasts entering
  on their own their flight, area, day and warm-up time. The competition
  keeps two timetables (roadmap 2026-10-07): the organiser's **draft**
  (`timetable`), which every change goes into (planning, moves, seats, a
  kept "what if"), and the **published** copy (`published_timetable`),
  which every attendee page reads (placements, duties, "My competition",
  personal and club timetables). With changes not published, the page says
  so: **Publish changes** copies the draft over, **Discard changes** copies
  the published one back, **Unpublish** takes it down. Timetables published
  before this became their own published copy (migration 14).

## What if there's a delay

**What if there's a delay?**, linked from a planned timetable, takes a day,
the areas held up (none for all), from when and for how long, and shows what
follows without saving anything (`Schedule.Delayed`), unless the organiser
taps **Keep this in the draft timetable**, which makes it the draft, to
publish. A flight under way runs
that much later; the area's later flights shift back only as far as they must
(slack takes some of it), going round blocked time (a flight that wouldn't
finish before lunch waits for it to end, and says so). A shifted flight that
would run past the end of the day, or need someone (a gymnast or official) in
two places, moves to the earliest free slot on another area of its
discipline, no earlier than its published time and clear of the delay. An
event's flights stay together (ADR 0006): unless events can run across
breaks, the rest of an event's run waits for a break as one, and a flight
moves to another area with all of its event's flights still to come. No
flight comes earlier than it was: one already after a break stays where it
is when the flight before it works through the break. Flights
that have already run, or are under way when the delay starts, never move. The
page shows each day's flights end and free time before and after, what no
longer fits, who's needed in two places, short rest before and after, and
each flight that changes (was, now, why).

The organiser can **ease** it on the held-up areas, from the delay: breaks
(lunch and other blocked time) wait (the default), **move later** keeping
their length, **start late** by up to some minutes, or are **worked
through**, each on that area alone (the rest of the venue keeps its break);
**fewer minutes between flights**; **quicker turns** (a percentage less per
competitor); **bigger flights** (an event's flights next to each other merge
up to a size, saving a changeover); and **running past the end** of the day by
up to some minutes. The page says what the easing is and what the delay
alone would have done.

## What if an official has to leave

**What if an official has to leave?**, linked from a planned timetable,
takes who (anyone with a seat), the day and time they go, and whether for
the day or the rest of the competition, and fills each seat they'd have had
from then without saving anything (`Schedule.Left`), unless kept in the
draft as for a delay. Each seat is filled for
the rest of its event's flights they'd have had, by one person. A seat goes
to someone free for all of them (not competing or on another panel then, allowed the role at that event
and by the must rules about people), or is reached by moving others round, at
most three moves: someone on that panel up a role, or someone across from a
panel at the same time, their seat filled in turn. Two ways to choose:
**easiest to fill** (the default) ends the chain at the seat the most free
people could take (recorder or marshal before HD or execution, before chair),
fewer moves breaking ties; **fewest changes** takes the shortest chain. Each
seat shows the moves and who else was free for the last one; a seat no one
can reach is left empty. Coaches, the organiser's own rules about events,
and officials on blocked time aren't considered.

## Simulation (ADR 0005, step 6)

**Simulate**, linked from the timetable, tries numbers against the venue
setup before entries arrive, or alongside them (ADR 0005 Decision 11). The
form starts from the competition as it stands (or from a scenario, to
change it):

- **Entries per event** (a synchro entry is a pair) and **gymnasts in all**:
  fewer gymnasts than the entries' places means some enter several
  disciplines. Stand-ins fill the biggest discipline first, then the rest go
  to gymnasts already entered, in turn, never twice in one discipline.
- **Clubs** the gymnasts and judges come from, round in turn.
- **Officials:** people who can judge each discipline (any level) and how
  many of them can chair; **how many people** those judges are (someone
  judging trampoline and synchro counts once); how many of the judges also
  **compete** (spread across the gymnasts, those who don't chair first); and
  **recorders and marshals**.
- **Judges each club must bring**, per discipline: so many judges (so many
  of them able to chair) for every so many of the club's competitors in it,
  rounding up (9 at 1 per 8 is 2), a synchro pair counting as two. With any,
  the clubs' judges come from it and the judges above are only the
  organiser's own, with no club. Each discipline's quota is filled by its
  own judges, so a club brings their sum (4 trampoline and 2 tumbling judges
  is 6 people). A percent of the clubs' judges, spread through them all, can
  also judge every other discipline as well as filling their own quota (but
  chair only in their own). A competing judge
  is a gymnast of their own club where it has one.

Stand-ins are planned with the real setup, panels and rota (`PlanStaffed`,
then `Rota`), as the real timetable is, and the result kept with the
scenario: entries and gymnasts, each day's last flight and finish, what
doesn't fit (by event) with a change that would fit it, the judges clubs
brought, seats no one could take, and gymnasts with less rest than wanted. Scenarios sit side by side;
one planned with an earlier setup, panels or events says so until **Run
again**. The last 8 are kept, apart from the timetable (migration 9), so
planning never touches them. Coaches aren't simulated, and stand-in judges
judge any level, so a real day can be tighter.

# Coaches (coach link)

Where a competition requires it, each entry needs a coach's **sign-off**
(ADR 0004 Decision 11).[^adr-0004]

- The comp sec adds the club's coaches on the club page; each gets a coach
  link (shown once; **New link** replaces it). Each coach **signs off
  routines** or not (ADR 0007 Decision 3; new coaches do): one who doesn't
  sees their members' entries, checked, but has no sign-off buttons, and a
  sign-off from them is refused.
- **Qualifications** (ADR 0007 Decision 4): the comp sec gives a coach up to
  four, each from the built-in list (British Gymnastics levels 1–4 and
  Gymnastics Ireland levels 1–3, in trampoline, tumbling and DMT) with a
  certificate: a JPEG, PNG, WebP or PDF up to 10 MB, its type checked from
  its bytes. It's stored in the database with the coach, opened by the comp
  sec (served as its own type, `nosniff`, not cached), and deleted with the
  qualification, the coach or the club.
- **Approval** (ADR 0007 Decisions 2, 5–7): where entries need sign-off, the
  organiser can tick **Coaches must be approved** and set the lowest level
  accepted per discipline (default Level 2; synchro goes by trampoline's).
  Each competition that does lists, on the club page, the club's coaches who
  sign off, to tick and **Send** (members' coaches ticked at first), with
  the organiser's decision and note; changing a coach's qualifications
  shows "send again", and sending them again makes them wait again. The
  organiser's **Coaches** page lists them by club, with their certificates
  (opened only for coaches sent there), whether they meet each level, and
  **Approve**, **Not approved** or **Withdraw approval**, with a note;
  approving a withdrawn coach again asks whether their earlier sign-offs
  count, or only those from then (`counts_from`). A sign-off records which
  coach gave it (`signed_coach`); it counts only while that coach is
  approved, worked out as entries are read, so nothing is erased. An
  unapproved coach sees why on their page and can't sign off; the
  organiser's entry shows "Signed off by Ann, who isn't an approved coach".
  Sign-offs from before this kept only a name: where it's exactly one of
  the club's coaches, it's theirs (migration 18) and counts once they're
  approved; otherwise it doesn't count at a competition that approves
  coaches.
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
  who signs off under their name, without a coach page. Where the
  competition approves coaches (ADR 0007 Decision 8), the entry page first
  asks for **Your coach**: a name, a qualification from the list and a
  certificate, stored with the entry (deleted with it, or the competition).
  The organiser's Coaches page lists them under **Individuals**, to approve
  as clubs' coaches are. Until approved, the sign-off link says why and
  can't be used; once approved, the coach signs off as the named coach,
  and the sign-off counts while they stay approved. Naming another coach
  waits for approval again.
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

## Personal and club timetables

Once the timetable is published (roadmap 2026-10-08), the panel timeline
(days side by side, a column per area, time running down evenly) is also
shown focused (`timelineFocus`):

- **Your timetable** (a member's page, an individual's entry, "My
  competition"): the flights they compete in ("You compete") and the panels
  they sit on ("You: Execution judge") picked out in green.
- **Club timetable** (the comp sec's page, a coach's, a member's): every
  flight with one of the club's gymnasts, named, or one of its people
  officiating ("Officials: Ann (E2)"; members, and the organiser's judges
  from the club), picked out.

The rest is faint, or with **Only these**, left out (blocked time stays).
Each prints like the panel timeline; notes are in each cell's hover text
too.

# Notifications (ADR 0008)

Members (each competition on their page, and "My competition"),
individuals (their entry), comp secs (each competition on the club page)
and coaches (each competition on theirs) have **Tell me about changes**:
**On this phone** (push), and where the server can send email, an email
with the tick box "I'm 18 or over, or this is a parent's email"; each says
what about: **when or where** they (or their
gymnasts) compete, **officiating**, and **cards** (checked, a note from the
organiser, removed or on hold). A member and an individual hear about
themselves, a comp sec about every member of the club, a coach about the
members they coach. It's per competition, and deleted with it.

- An email is sent a link to confirm it first; nothing else is sent until
  it's tapped (a button, so link checkers don't). At most 10 confirmation
  emails an hour from one address.
- Before the organiser publishes or unpublishes the timetable, checks a
  card or changes its note, or removes, holds or restores an entry, what's
  live is kept as a **baseline** (unless changes are already waiting), due
  **10 minutes** later. The dashboard says when, with **Notify now**; **No
  wait on the competition's days** (Links and settings) makes them due at
  once on those days.
- Every half minute the server compares each due baseline with what's live
  (`changes`): each entry's flight, area and warm-up time, each person's
  duties, and each card. Running orders, and anything changed and put back,
  tell no one. Each address gets **one email** for all of it, grouped by
  gymnast ("Ann Ryan" then "- BUCS L5 Women · flight 2 of 3: now warm-up
  Saturday 10:20, Panel 2 (was Saturday 09:40, Panel 1)"), with links to
  their pages and to stop the emails (also as one-click `List-Unsubscribe`).

Email goes by SMTP when `SMTP_HOST` and `MAIL_FROM` are set
([deploy](../operations/deploy.md#notifications)); without them only push
is offered.

**Push** (ADR 0008 Decision 3) needs nothing set up. `static/js/push.js`
registers the service worker (`/sw.js`, the site's whole scope), asks to
show notifications and subscribes with the app's VAPID public key, made on
first use and kept in the `settings` table; the page keeps the browser's
endpoint and keys (up to 5 phones a person a competition) and lists them,
"This phone" for the one in hand, each with **Turn off**. Only push
services' addresses are taken (Google, Mozilla, Apple, Microsoft), so the
server posts nowhere else. A batch's push says the one change, or "3
changes for Ann Ryan and Bea Kelly. Tap to see them.", and opens their
page; it's encrypted for the browser (RFC 8291, `aes128gcm`) and signed
(RFC 8292, ES256), on the standard library. A push service answering 404
or 410 drops the phone. On an iPhone or iPad, push works only from the home
screen (iOS 16.4+): the page says to add it first (Share, Add to Home
Screen); the web manifest has no `start_url`, so the home screen opens the
person's own page.

# Built from

- `views.TariffSheet` (`views/sheet.templ`) for printed cards.
- The view screen and level panel (`views/view.templ`, `views/routine.templ`)
  for one entry.
- `requirements.Evaluate`, `requirements.RequiredElements` and the shared level
  check (ADR 0004 Decision 4) for the table's columns.

[^adr-0004]: ADR 0004 — Server storage with secret links, no accounts
