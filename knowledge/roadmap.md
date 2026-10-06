---
type: Roadmap
title: Roadmap
description: Where the project could go beyond the routine builder, covering tools for competition organisers and attendees and for people running or training in a club, with what already exists and what to build first.
tags: [roadmap, competitions, clubs, planning]
status: draft
generated: { by: claude-code/cli, at: 2026-10-05T16:00:00Z }
stale_after: 2027-04-01T00:00:00Z
sources:
  - id: tramponline
    resource: https://www.tramponline.org/
    title: TrampOnline (online entries and scoring)
  - id: tscore
    resource: https://tscore.co.uk/wp/
    title: TScore (competition organiser and scoring)
  - id: brentwood
    resource: https://brentwood-trampoline.org/competitions/organisation-of-trampoline-competitions/
    title: Brentwood Trampoline, organisation of trampoline competitions
  - id: cop
    resource: https://www.gymnastics.sport/publicdir/rules/files/en_1.1%20-%20TRA%20CoP%202025-2028.pdf
    title: FIG Trampoline Code of Points 2025–2028
    author: org:fig
  - id: club-software
    resource: https://joinit.com/blog/best-gymnastics-club-management-software
    title: Gymnastics club management software round-up (2026)
  - id: generators
    resource: https://vbeaulieu.com/routine-trampoline/
    title: Vincent Beaulieu's trampoline routine generator
---

# Why

The skill, tariff and requirements engine sits between two groups with needs
nothing serves well yet: **competition organisers and attendees**, and **people
running or training in a club**. The aim is to help with trampoline problems
generally, whoever has them. Everything stays phone-first, plainly worded, and
built on the one engine, so rules exist only in Go ([rendering](architecture/rendering.md)).

# What already exists

- **Entries and live scoring are covered.** TrampOnline handles online entries
  and device-based scoring and results for 300+ organisers since 2012, and
  partners with ISTO, NEUT, the Southern Universities league and Scottish
  Student Trampolining.[^tramponline] TScore handles setup, entries,
  timetabling, tablet scoring with time of flight and horizontal displacement,
  and results.[^tscore] **Complement them first.** Replacing them is a possible
  later direction, not the starting point.
- **Card checking is still done by hand.** Gymnasts hand in competition cards
  (elements and tariff) before the event. Difficulty judges check them before the
  competition starts, and some rule books penalise a late card (0.2 off the
  difficulty).[^cop] The engine
  already does exactly this check.
- **Timetabling rule of thumb:** British Gymnastics allows 4½–6½ minutes per
  competitor for two rounds plus warm-up. A panel is two trampolines with their
  officials.[^brentwood]
- **Club admin tools are plentiful** (bookings, payments, attendance), and so
  are some training tools (session diaries, rotation planners).[^club-software]
  Routine generators exist,[^generators] but none build a routine that meets a
  competition's requirements.

# Competitions: organisers and attendees

| # | Area | What it does | Builds on |
|---|---|---|---|
| 1 | **Card collection and checking** (first; [what each person sees](features/competition-entries.md)) | Members keep their entries with their club and change them freely; the club's competition secretary sends them to the competition (individuals can enter directly where allowed); the organiser and difficulty judges see every card already checked against its level, print them, mark them checked, and export CSV; optionally asks for video proof by link (an unlisted YouTube video, say) for chosen skills | requirements, levels, tariff sheet, sharing |
| 2 | Difficulty judge helper | Tap elements as they're performed, compared live with the submitted card: changes, interruptions, the new tariff | picker, search, validation |
| 3 | Timetable and flight planner | Entries per category, panels and minutes per competitor give flights, running orders and estimated times; printable marshal and chair-of-judges sheets | — |
| 4 | Officials rota | Judges, recorders, marshals and spotters per panel and flight, with clashes flagged | 3 |
| 5 | "My competition" page | For attendees: flight, panel, time, card status | 1, 3 |
| 6 | Results history | Import results CSVs (TrampOnline, TScore) for personal bests and progression, and for next year's level planning | — |
| 7 | Later, if wanted | Entries, scoring with time of flight and displacement, live results: the "replace" path | 1–6 |

# Clubs: coaches, gymnasts, committees

| # | Area | What it does | Builds on |
|---|---|---|---|
| 1 | Club competition secretary | Now part of card collection (competitions 1): members' entries gathered by the club and sent to each competition; still to add: export for TrampOnline entry | competitions 1 |
| 2 | Skill tracking | Coaches tick skills per gymnast (club link); each gymnast sees what's next and which levels they're ready for | catalog |
| 3 | Session planner | Trampolines, gymnasts and session time give a turn rotation with spotters and coach ratio, on a phone | — |
| 4 | Judge and coach practice | Tariff and FIG-notation quizzes, card-checking practice with planted errors | engine |
| 5 | Committee handover kit | Safety checks, incident log and policy templates (mostly documents) | — |
| 6 | Routine suggester (low priority) | From the skills a gymnast can do and a chosen level, suggests legal routines (landing to take-off, no repeats, requirements, caps), highest tariff first. Hard to do well: a legal, high-tariff routine isn't necessarily one a gymnast should compete | skills, requirements, 2 |

# Decisions so far (2026-10-05)

- Complement TrampOnline and TScore first, and keep replacement possible.
- Anything stored on the server uses **secret links, no accounts**: an admin
  link and a share or submit link, like a form. Personal data is kept to names
  and clubs, and a competition's entries are deleted 120 days after it
  (sooner if the organiser chooses). Accounts can come later.
- **Competition card collection is the first thing to build.** Its design goes
  in [ADR 0004](../docs/adr/0004-server-storage-secret-links.md) (server storage with secret links), amending ADR 0001's plan for
  accounts.
- **Hosting for storage.** The app runs on the self-hosted server at
  `tariff.pardy.ie` (Render's free service had no persistent disk). Its
  `/srv/data/tariff` volume, `DATA_DIR` and nightly SQLite backup are a change
  to the server repository; once applied, competition entries go live there
  ([deploy](operations/deploy.md#competition-storage)).
- **Priorities (2026-10-05):** card collection first. The routine suggester
  drops to the bottom, as it would be hard to do right. ISTO's levels won't
  be available for a while, so building them in waits; users can write them
  as their own requirements meanwhile.
- **Keep the name for now.** Competition and club tools stay on
  `tariff.pardy.ie` even though they go beyond tariffs; a broader name can
  come later.

See [open questions](open-questions.md) for what's waiting on documents and
decisions.

[^tramponline]: TrampOnline (online entries and scoring)
[^tscore]: TScore (competition organiser and scoring)
[^brentwood]: Brentwood Trampoline, organisation of trampoline competitions
[^cop]: FIG Trampoline Code of Points 2025–2028
[^club-software]: Gymnastics club management software round-up (2026)
[^generators]: Vincent Beaulieu's trampoline routine generator
