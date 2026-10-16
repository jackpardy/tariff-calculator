# ADR 0014 — Judges' qualifications

- **Status:** Accepted; built 2026-10-16
- **Date:** 2026-10-16
- **Deciders:** jackpardy (solo maintainer)
- **Amends:** ADR 0004 Decision 6 (personal data: judges' certificates are
  kept, as coaches' are under ADR 0007); ADR 0005 Decision 5 (officials'
  offers)

## Context

A competition can already say that only people the organiser marks
**qualified** judge (`JudgeQualified`, ADR 0005), but the organiser ticks
them on trust: the offer says what someone will judge, not what they're
qualified to. The roadmap (2026-10-08) asks for judges' qualifications,
"approved like coaches'" (ADR 0007).

## Decision

1. **A built-in list**, as for coaches (ADR 0007 Decision 1): British
   Gymnastics' judging levels 1 to 4 and Gymnastics Ireland's 1 to 3, in
   trampoline (which covers synchro), tumbling and DMT, e.g. "British
   Gymnastics Level 2 Trampoline Judge" (`competitions.JudgeQualifications`).
   The names are from the bodies' pathways and still to be confirmed; FIG
   brevets can be added if an organiser asks.

2. **A judge gives theirs beside their offer.** Each with its certificate,
   a photo or PDF, by the same rules as a coach's (ADR 0007 Decision 9: 3 MB
   each, checked from its own bytes, four a person, none taken when the disk
   is nearly full):
   - **a club member's** are kept with them at the club, for every
     competition, added by them or by the comp sec for them, and count
     towards the club's 60 MB for certificates (with its coaches');
   - **an individual's** are kept with their entry (one set for the
     gymnast, whichever of their entries at the competition), counted
     towards the competition's 60 MB for its individuals' certificates, and
     deleted with it.

3. **The organiser sees them on Officials**, each official's qualifications
   with their certificates to open (the organiser's and timetable links),
   and, where the competition approves judges, which disciplines someone
   offers to judge with no qualification in.

4. **Approving is marking qualified**, as now: the tick on Officials. The
   member or individual sees whether the organiser approves judges, and once
   they're approved. Nothing else changes: the rota already uses only those
   marked qualified under that rule.

5. **Live, not sent.** The organiser sees a member's qualifications as they
   are now, not as the club sent them (unlike coaches, ADR 0007 Decision 5):
   a judge's offer already goes with each send, and changing a qualification
   after the organiser ticks them doesn't untick them.

## Consequences

- Organisers can see what a judge is qualified for before relying on them.
- More personal data: certificates name a person; the pages say who sees
  them and when they go.
- Not done: a lowest level asked per discipline (as for coaches), and
  unticking a judge whose qualifications change. Add them if organisers ask.
