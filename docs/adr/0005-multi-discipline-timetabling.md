# ADR 0005 — Timetabling multi-discipline competitions (ISTO)

- **Status:** Accepted
- **Date:** 2026-10-06; settings and defaults (Decisions 5, 6 and 13) added
  2026-10-07
- **Deciders:** jackpardy (solo maintainer)
- **Builds on:** ADR 0004 (competition entries, clubs, coaches) and the
  timetable (roadmap: competitions 3)
- **Amends:** the roadmap's officials rota (competitions 4), which becomes
  part of this
- **Amended by:** ADR 0006 (one panel for a whole event, proposed
  2026-10-08), for Decisions 9 and 12

## Context

The goal the competition tools are working towards is helping the Irish
Student Trampoline Open (ISTO) run its day. ISTO has more than individual
trampoline:

- **Disciplines:** individual trampoline, **synchro** (two gymnasts on two
  trampolines), **tumbling** and **double mini-trampoline (DMT)**.
- **Competitors judge and help.** Many gymnasts also judge other levels or
  disciplines, or help as marshals and recorders, when they aren't competing.
  Some judges come only to judge and belong to no club.
- **The venue has a strict end time.** That's the biggest constraint.
- **It runs over 2½–3 days**, each with its own hours.
- **People compete more than once:** in several disciplines, and in synchro
  as well as individually.
- **Not everything is a flight.** Lunch, awards, and ad hoc events that take
  entries on the day (a fun event, say) take up time on the areas too.

The timetable built for competitions 3 plans flights of one discipline on
panels, from entries that each belong to one gymnast. It can't see that the
same person is in a tumbling flight, a synchro pair and judging L4, so it can
put them in three places at once.

Constraints, in order of weight:

| Constraint | Kind |
|---|---|
| Everything finishes by the venue's end time | Hard: if it can't, say so and what would fix it |
| Nobody is in two places at once: competing twice, or competing while judging or helping | Hard |
| Each flight has the judges its discipline needs, able to judge it | Hard |
| Rest between a person's turns, with slack in case one runs long | Soft: as much as fits |
| A coach isn't coaching two gymnasts competing at once | Soft: where possible |
| Gymnasts judging their own club: unavoidable, but balanced across clubs | Soft |
| A level's flights on one panel; a club's gymnasts spread in a running order | Soft, as now |
| Blocked time (lunch, awards, ad hoc events) at its fixed time or within its window | Hard |
| The organiser's own rules: what happens where, who does what | Hard or soft: the organiser chooses for each |

## Decision

1. **Events: a discipline at a level.** A competition offers **events**:
   individual trampoline, synchro, tumbling or DMT, each at a level. Trampoline
   and synchro levels are the requirements framework's levels (synchro checked
   as trampoline routines, one routine per pair). Tumbling and DMT levels are
   named by the organiser and are **entry and timetable only**: their passes
   aren't checked until the engine learns their tariff (later, if wanted).

2. **A person can enter several events.** A member's entry becomes one per
   event, not one per competition (`member_entries` keyed by member,
   competition and event). Every entry names its gymnasts: one, or two for
   synchro.

3. **Synchro pairs can be any two gymnasts.** One gymnast enters the pair and
   gets a **partner link** to send the other, who confirms from their own
   member or individual page (or the partner's comp sec confirms for them).
   Until confirmed, the partner is a name only: the entry shows "partner not
   confirmed", and the timetable can't check that partner's clashes.

4. **People are what the timetable schedules.** A competition's **people**
   are its gymnasts (members and individuals, each matched across their
   entries) and its **officials**. Constraints are checked per person.

5. **Officials.** When entering, a gymnast ticks what they can **judge** (for
   each discipline, up to which level) and whether they can **help** (marshal,
   recorder). The comp sec can correct it for their members. The organiser
   can **add people directly**, such as judges with no club, with what they
   can do. Each discipline has a **panel of officials**, which the organiser
   can change. The default is the FIG Code of Points 2025–2028's: a Chair of
   Judges Panel, 6 execution judges and 2 difficulty judges, 9 in all, for
   trampoline and synchro (§18.1; in synchro, execution judges 1, 3 and 5 watch
   trampoline 1 and 2, 4 and 6 trampoline 2), tumbling and DMT (§17.1 in
   their sections). The CJP covers time of flight, horizontal displacement and
   synchronisation. Trampoline adds 2 horizontal displacement (HD) judges
   where no machine measures it, a setting (none by default). Helpers (a
   recorder, a marshal) aren't in the Code; they
   default to one each per panel, also changeable. **Spotters aren't a panel
   position** and aren't scheduled.

6. **The venue, over several days.** The organiser describes the **areas**,
   each named, and as many of each discipline as the venue has: e.g. "Panel
   1", "Panel 2" and "Panel 3" for trampoline (which also run synchro), "Track
   A" for tumbling, "DMT 1" and "DMT 2". The competition runs over one or more
   **days** (ISTO: 2½–3), each with its own **start** and **strict end** time,
   and its own areas if they differ (a half day may use fewer). No person's
   turn spans the end of a day.

7. **Blocked time.** The organiser can block off time for anything that isn't
   a planned flight: **lunch**, **awards**, warm-up periods, or an **ad hoc
   event** that takes its entries on the day. Each block has a name, a length,
   the areas it takes (one, several, or the whole venue), and either a **fixed
   time** or a **window** the planner may place it in ("lunch, 45 minutes,
   somewhere between 12:00 and 14:00"). An ad hoc event can give an expected
   number of entries to size it, and the officials it needs, who are then
   treated like a flight's. Blocks count towards the end time, and the
   planner schedules flights around them.

8. **Organiser rules: soft or hard, the organiser's choice.** The organiser
   can say what happens where and who does what, and mark each rule **must**
   (hard: never broken; reported if it can't be met) or **prefer** (soft: a
   weight in the score). For example:
   - an event or level on a particular area ("high-level synchro on Panel 2");
   - a person in a particular role for an event ("Mary chairs Elite Women");
   - a person not judging, or not before or after a time ("Tom judges only
     after 12:00");
   - events in an order, or not at the same time as each other;
   - turning a built-in soft constraint into a hard one ("at least 20 minutes
     between a person's turns"), or a hard one's report off.
   Rules are the main way the organiser's knowledge of the day gets in; the
   planner works around them.

9. **The scheduler.** Planning builds flights per event (as now), then places
   flights on areas and assigns officials:
   - Hard constraints are never broken. A flight that can't be placed, or
     can't be staffed, is reported rather than forced.
   - Soft constraints are weighted. The planner starts from a greedy schedule
     (longest events first, earliest free area, people's clashes respected),
     then improves it by local search: moving flights between areas and
     times, swapping running orders and judges, scoring each schedule and
     keeping better ones, within a time limit. It's deterministic for a seed,
     so a plan can be reproduced.
   - The result says how well each soft constraint was met: e.g. "3 gymnasts
     have under 20 minutes between turns", "UCD judges its own gymnasts 4
     times, DCU 3".

10. **When it doesn't fit, say what would.** If the plan runs past the end
   time, the timetable shows by how much and which events are left over, and
   re-plans to show what each option would do: another area, fewer minutes
   per gymnast or between flights, larger flights, or a cap on an event's
   entries.

11. **Simulation.** Before entries exist (or alongside them), the organiser
   can **simulate**: numbers of entries per event, how many gymnasts enter two
   disciplines, and how many can judge. The app makes stand-in people and plans
   them with the real venue settings. It shows the finish time and what
   doesn't fit, without touching the real timetable. Several scenarios can be
   compared.

12. **The officials rota (competitions 4) is part of this.** Judges and
    helpers are assigned to flights by the scheduler, and printed on the
    marshal and chair of judges sheets.

13. **Every number is a setting, with a default.** Nothing about ISTO is
    built in; the organiser can change each of these, per discipline:
    - the **panel of officials** (Decision 5; default: the Code of Points);
    - **minutes per competitor**: default 5 for trampoline (both rounds, from
      British Gymnastics' 4½–6½), 5 per pair for synchro (2½ since 2026-10-07:
      a pair does one routine), and 2 for tumbling and DMT (two passes)
      until better figures are known;
    - **minutes between flights** (warm-up and changeover), default 10;
    - **who can judge what**: anyone up to the level they say (the default),
      only levels below the one they compete at, or only people the organiser
      marks as qualified;
    - the **rest** wanted between a person's turns, and whether it's a must or
      a prefer (Decision 8).

## Consequences

**Positive**
- The timetable can't put someone in two places at once, and keeps to the
  venue's hours or says plainly why it can't.
- Officials come from the people already entered, as at ISTO, plus the
  organiser's own judges.
- Simulation answers "will it fit?" months before entries open.

**Negative / risks**
- **Scheduling is hard.** Exact answers are out of reach at this size, so the
  planner gives a good plan, not the best one, and reports where it falls
  short. The organiser can still adjust by hand, with clashes flagged.
- **People must be matched across entries.** Members are matched by their
  member link; individuals by their entry; synchro partners by confirmation.
  An unconfirmed partner, or the same gymnast entering twice under different
  names, escapes the clash checks: the timetable flags names that look alike.
- **More to enter.** Judging abilities and partners add questions to the entry
  form. They're optional, and asked once per person per competition.
- **Tumbling and DMT aren't checked.** Their entries are a level only, until
  their tariff is built in.

## Alternatives considered

- **A separate timetable per discipline.** Simple, but it's exactly what puts
  one person in two places at once.
- **An off-the-shelf solver** (constraint programming). Better optimality, but
  a large dependency and hard to explain to an organiser; the local search is
  small, testable and good enough at hundreds of people.
- **Organiser-only officials.** Fewer questions for gymnasts, but at ISTO the
  judges are mostly the gymnasts, and the organiser would retype them all.

## Migration

Each step is a separate, shippable branch:

1. This ADR.
2. **Events and people:** events per competition (trampoline, synchro,
   tumbling, DMT), several entries per member, synchro pairs with partner
   links, tumbling and DMT entry-only.
3. **Officials:** judging and helping abilities on entries, the organiser
   adding people, panels of officials per discipline.
4. **Venue and scheduler:** areas, start and end; blocked time (lunch,
   awards, ad hoc events, fixed or within a window); the organiser's rules,
   each must or prefer; flights placed per person with clashes, rest and end
   time; the report of what doesn't fit and what would.
5. **Officials rota:** judges and helpers assigned to flights, own-club
   balance, coach clashes, printed on the sheets.
6. **Simulation:** scenarios of entry numbers, planned against the venue.
