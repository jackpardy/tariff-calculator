# ADR 0006 — One panel for a whole event

- **Status:** Proposed
- **Date:** 2026-10-08
- **Deciders:** jackpardy (solo maintainer)
- **Amends:** ADR 0005 Decisions 9 (the scheduler) and 12 (the officials
  rota), and its soft constraint "a level's flights on one panel", which
  becomes a must

## Context

ADR 0005 plans an event's flights one at a time. The scheduler places each
flight wherever it scores best, with a level's flights on one area only a
preference (a cost of 60 if one lands elsewhere). The rota then fills each
flight's panel on its own, with a small bonus for someone staying on the
same area within half an hour. So in the demo, BUCS L7 Men's five flights
run on Friday and Saturday, on Panels 1 and 2, with different judges for
each.

That isn't how a competition is judged. **Everyone on an event's panel
judges all of it**, so every gymnast in a ranking is scored by the same
people and the scores compare fairly. The recorder and marshal stay too.
Trying the tools with an ISTO organiser made this the biggest priority
(roadmap, 2026-10-08).

Flights are also numbered when an event is split into them, before they're
placed, so "flight 2 of 5" can run before flight 1.

## Decision

1. **The unit is a ranking.** An event's panel covers every flight whose
   gymnasts are ranked together: a level, or, where its men and women fly
   separately, each of them (BUCS L7 Men and BUCS L7 Women can have
   different panels). A paired synchro event (Synchro BUCS L6/L7) is one.
   This is the category the timetable already splits into flights.

2. **An event's flights run back to back, on one area, on one day.** The
   scheduler places an event's flights as one **run**: in order, each
   starting when the last ends (after the minutes between flights, as now),
   on a single area. This is a **must**, replacing ADR 0005's preference. A
   run never spans two days.
   - **Not across breaks, unless the organiser allows it.** By default an
     event finishes before blocked time on its area (lunch, say) or starts
     after it. The organiser can turn on **events can run across breaks**:
     then a run can pause for the break, its panel breaking too, and finish
     afterwards.

3. **The panel's breaks are the general warm-ups.** Each flight starts with
   its **general warm-up** (the minutes between flights, ADR 0005 Decision
   13), when its gymnasts warm up freely; the panel isn't needed then. The
   panel is needed from the **one touches** (the official warm-up, in
   running order, counted in the minutes per competitor) to the end of the
   flight. So a long event is judged with a break before each flight, and
   that's the break the panel gets: no others are added. The rota still
   keeps the panel for the whole run (a warm-up is too short to compete in
   or officiate elsewhere), and their duties, the officials rota and the
   panel timeline show each flight from its start, the start of its general
   warm-up, as now.

4. **The scheduler places runs, not flights.** The greedy start and its
   attempts (ADR 0005 Decision 9) work on runs: a run's length is its
   flights' lengths, and it goes where it fits whole, clear of its people's
   other turns and the rules. Flights still matter inside a run (warm-up,
   running order, rest), but a person is only ever in one flight of an
   event, so rest is between runs. An event that doesn't fit whole anywhere
   is **reported**, with the same fixes tried as now (another area, less time
   per competitor or between flights, bigger flights, rest only preferred,
   a cap on entries).

5. **The rota fills a panel once per event.** Each run is one job: its
   seats are filled by people free for the **whole run** (not competing,
   not officiating elsewhere, and kept by no rule from any part of it), and
   they hold those seats for all its flights. The rota's other goals stay
   (own-club judging shared fairly, the work spread, rest before
   competing), counted per run. Blocked time that needs officials is still
   staffed separately, never from a panel that's paused for it.
   - **Recorders and marshals can change, if the organiser allows it.** By
     default the whole panel stays, recorders and marshals included. The
     organiser can let **recorders and marshals change between flights**:
     then those seats are filled flight by flight, as now, while the judges
     stay for the run.

6. **Flights are numbered in the order they run.** Within an event, flight 1
   is the first to start. Numbers are worked out again whenever times are
   (after planning, a hand edit or a kept "what if").

7. **Rarely: split by routine.** Where an event is too big for one panel in
   the time there is, the organiser can choose, for that event, to **split
   it by routine** instead: every gymnast does their first routine on one
   area and their second on another. Each area's panel judges one routine
   for the whole event, so each routine's scores still compare fairly.
   - The two areas run as a pipeline: a flight does its first routine on the
     first area, then its second on the second, while the next flight starts
     its first routine.
   - Each area's panel stays for its routine's whole run, as in Decisions 2
     and 5.
   - A routine takes half the minutes per competitor, plus the minutes
     between flights on each area.
   - Synchro, with one routine, can't be split.
   - It's the organiser's choice, never the scheduler's. When an event
     doesn't fit, the report can suggest it, but doesn't do it.

8. **Hand edits and the "what ifs" keep runs whole.**
   - **Moving a flight** to another day or area moves its event's run.
     Within a run, the organiser can change the flights' order; moving
     gymnasts between flights stays as now.
   - **What if there's a delay** shifts a held-up area's runs, and moves a
     run to another area only whole, with its panel.
   - **What if an official has to leave** fills each of their seats for the
     rest of that event's run, not flight by flight.

## Consequences

**Positive**
- Every gymnast in a ranking is judged by the same panel, as competitions
  are run.
- Officials sit once per event instead of hopping between flights and
  panels, so the rota is easier to follow and print.
- Fewer, larger pieces make the scheduler's search smaller.

**Negative / risks**
- **Officials are tied up for longer.** A large event can run for most of
  a day, with only the general warm-ups as breaks, and its panel can't
  compete or officiate elsewhere meanwhile. That's how competitions are
  judged, but where competitors judge, as at ISTO, fewer people are free at
  any time, so more seats may go unfilled. The simulation shows how many,
  and the demo should be planned again to see it.
- **Less room to fit.** A run can't be broken up to fill gaps, and by
  default can't run across a break, so a day can finish later than it does
  now, or an event may not fit at all. Lunch given as a window, rather than
  a fixed time, gives the scheduler room to put it between runs. The report
  says what doesn't fit, with fixes, including allowing events across
  breaks.
- **Splitting by routine is new.** Today both of a gymnast's routines are in
  one flight; a split event's flights are on two areas, which the timetable,
  the sheets, the panel timeline and "My competition" all need to show.

## Alternatives considered

- **Keep it a preference, with a higher cost.** Simple, but a preference can
  still be broken when the day is tight, which is exactly when the organiser
  can least afford to fix it by hand.
- **Same judges, but flights anywhere.** The panel follows its event to each
  time and area. That keeps the scheduler's freedom, but ties the judges up
  across the gaps (all day, or two days, in the demo), so it's worse for the
  people who judge.
- **Two panels at once for a big event,** each taking some flights. That
  breaks the reason for the rule: some gymnasts would be scored by different
  judges. Splitting by routine keeps each routine's scores comparable.

## Migration

Each step is a separate, shippable branch:

1. This ADR.
2. **Runs:** the scheduler places each event's flights back to back on one
   area and day, not across breaks unless the organiser allows it; flights
   numbered in the order they run; hand moves and the
   delay "what if" move whole runs. Timetables planned before keep their
   flights until planned again, and the report flags events whose flights
   aren't together.
3. **One panel per run:** the rota fills each event's panel once; the
   leaving official "what if" fills seats for the rest of the run; the
   sheets, rota and panel timeline show it; the organiser's choice to let
   recorders and marshals change between flights.
4. **Split by routine:** the organiser's choice per event; the pipeline on
   two areas; shown on every timetable view.
