# ADR 0017 — Judges offer each role up to a level of its own

- **Status:** Accepted; built (2026-10-22)
- **Date:** 2026-10-22
- **Deciders:** jackpardy (solo maintainer)
- **Amends:** ADR 0005 Decisions 5 (officials' offers) and 13 (who can judge
  what)

## Context

A judge offered one "up to" level per discipline, and a "can chair" tick.
That level held for every judging seat: execution, difficulty, chair, HD and
synchronisation. So a judge who offered up to a lower level was kept off HD
at the top level, though HD (horizontal displacement) is measured the same
at every level. And someone who can judge difficulty at a level may not be
able to judge execution there, which one level couldn't say.

The rule "only levels below the one they compete at" added little: nobody
could judge the top level, and a judge could just offer up to the level below
their own. A word from the organiser does the same where it's wanted.

The timetable's seat dropdown listed every official for every seat. Choosing
someone who couldn't take it asked first, but didn't say why ("Aaron Brown
can't be hd judge at ISTO Elite-Pro").

## Decision

1. **HD and synchronisation at any level.** Offering to judge a discipline is
   offering its HD (trampoline, synchro) and synchronisation (synchro) seats,
   at every level.
2. **Execution, difficulty and chair each up to a level of their own**: no,
   any level, or up to a level (`JudgeOffer.UpTo`, by role). A new offer
   starts as execution and difficulty at any level, not chairing. Tumbling and
   DMT, with nothing measured, need at least one of them.
3. **Offers from before read as HD only.** Their one level and chair tick
   are dropped, not copied: only demo competitions have offers so far.
4. **The "below their own level" rule is gone.** Competitions with it take
   the default, anyone as they offer (migration 52). "Only people I mark
   qualified" stays, for every judging seat.
5. **A note to those offering.** The organiser can write one (up to 500
   characters) with the panels; it's shown above every offer form, e.g. who
   they're short of, or that they'd rather people didn't judge their own
   level.
6. **The seat dropdown lists first who can take the seat**, then the rest
   under "Can't take this seat", to choose anyway. Who can is by role and
   level only; clashes are still asked about when the change is made.
7. **The ask says why**: "Ann can't take the execution judge seat at FIG AG3:
   their offer: Judge Trampoline (execution up to BUCS L3, and HD)", or "they
   aren't marked qualified".

8. **Synchro is judged by trampoline's judges** (added 2026-10-22, at the
   owner's ask: few could chair synchro, which was offered on its own).
   There's no synchro row to offer; trampoline's, named "Trampoline and
   synchro" where the competition has synchro, takes in each synchro event
   by the hardest trampoline level it pairs: chairing up to ISTO Advanced is
   chairing "ISTO Intervanced/Advanced". Synchronisation, like HD, at any
   level. Offers from before for synchro alone count for nothing.

## Consequences

- The rota can put anyone who judges a discipline on its HD seats, so it
  keeps judges who can take execution or difficulty there for those seats
  where it can (a small cost on HD and synchronisation).
- An offer's words are longer: "Judge Trampoline (execution and difficulty up
  to BUCS L5, chair at any level, and HD)".
- The "Can judge" filter on Officials goes by the hardest level someone takes
  execution, difficulty or chair at; someone offering HD only shows under
  "any level".
