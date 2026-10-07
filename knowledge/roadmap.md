---
type: Roadmap
title: Roadmap
description: Where the project could go beyond the routine builder, covering tools for competition organisers and attendees and for people running or training in a club, with what already exists and what to build first.
tags: [roadmap, competitions, clubs, planning]
status: draft
generated: { by: claude-code/cli, at: 2026-10-07T19:00:00Z }
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
| 1 | **Card collection and checking** (built, live at `/competitions` since 2026-10-06; [what each person sees](features/competition-entries.md)) | Members keep their entries with their club and change them freely; the club's competition secretary sends them to the competition (individuals can enter directly where allowed); the organiser and difficulty judges see every card already checked against its level, print them, mark them checked, and export CSV; optionally asks for video proof by link (an unlisted YouTube video, say) for chosen skills. Synchro (2026-10-07): levels paired into one event (e.g. L1/L2), a pair entering one routine (the level's voluntary), and a pair at the wrong level for their individual ones flagged; levels kept easiest first, which judging "up to" goes by | requirements, levels, tariff sheet, sharing |
| 3 | Timetable and flight planner (built: every step of [ADR 0005](../docs/adr/0005-multi-discipline-timetabling.md), 2026-10-07: [timetable](features/competition-entries.md#timetable-adr-0005-step-4), [simulation](features/competition-entries.md#simulation-adr-0005-step-6)) | Entries per category, panels and minutes per competitor give flights, running orders and estimated times over days and named areas, around blocked time and the organiser's rules; printable marshal and chair-of-judges sheets; a panel timeline at an even time scale, with or without each official's seat; simulation of entry numbers, judges and the judges each club must bring. On the day: [what if there's a delay](features/competition-entries.md#what-if-theres-a-delay) (shift or move flights, eased by moving or shortening breaks, shorter changeovers, quicker turns, bigger flights or running over) | — |
| 4 | Officials rota (built, ADR 0005 step 5: [officials rota](features/competition-entries.md#officials-rota-adr-0005-step-5)) | Judges, recorders and marshals per panel and flight (spotters aren't scheduled), drawn from competitors and the organiser's own judges, with clashes flagged and own-club judging balanced; HD and synchronisation judges where no machine measures. On the day: [what if an official has to leave](features/competition-entries.md#what-if-an-official-has-to-leave) (each seat filled by someone free or by moving others round, easiest to fill or fewest changes) | 3 |
| 5 | "My competition" page (built: [my competition](features/competition-entries.md#my-competition-roadmap-competitions-5)) | For attendees: flight, panel, time, card status | 1, 3 |
| 6 | Results history | Import results CSVs (TrampOnline, TScore) for personal bests and progression, and for next year's level planning | — |
| 7 | Later, if wanted | Entries, scoring with time of flight and displacement, live results: the "replace" path | 1–6 |
| 2 | Difficulty judge helper (low priority) | Tap elements as they're performed, compared live with the submitted card: changes, interruptions, the new tariff. Hard to make reliable enough to use on the day | picker, search, validation |

# Clubs: coaches, gymnasts, committees

| # | Area | What it does | Builds on |
|---|---|---|---|
| 1 | Club competition secretary | Now part of card collection (competitions 1): members' entries gathered by the club and sent to each competition; still to add: export for TrampOnline entry | competitions 1 |
| 2 | Skill tracking | Coaches tick skills per gymnast (club link); each gymnast sees what's next and which levels they're ready for | catalog |
| 3 | Session planner | Trampolines, gymnasts and session time give a turn rotation with spotters and coach ratio, on a phone | — |
| 4 | Judge and coach practice | Tariff and FIG-notation quizzes, card-checking practice with planted errors; execution practice could build on the "How it's judged" rules ([execution judging](domain/execution.md), built 2026-10-07) | engine |
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
- **Storage is live (2026-10-06).** The app runs on the self-hosted server at
  `tariff.pardy.ie` (Render's free service had no persistent disk), with
  competition entries in SQLite on its `/srv/data/tariff` volume, backed up
  nightly ([deploy](operations/deploy.md#competition-storage)). Card
  collection (competitions 1) is built and live at `/competitions`, with coach
  sign-off; the calculator doesn't link to it yet.
- **Priorities (2026-10-05):** card collection first. The routine suggester
  drops to the bottom, as it would be hard to do right. ISTO's levels won't
  be available for a while, so building them in waits; users can write them
  as their own requirements meanwhile.
- **The aim is ISTO (2026-10-06).** The competition tools work towards running
  the Irish Student Trampoline Open's day: trampoline, synchro, tumbling and
  DMT, with competitors judging and a venue that must finish on time
  ([ADR 0005](../docs/adr/0005-multi-discipline-timetabling.md), accepted).
- **Priorities (2026-10-06):** card collection is live. Next is the timetable
  and flight planner (competitions 3), then the officials rota (4). The
  difficulty judge helper (2) drops to the bottom: it would be hard to get
  right enough to be useful on the day, and there's easier value elsewhere.
- **Next for the timetable (from the demo, 2026-10-06; built 2026-10-07 as
  the [panel timeline](features/competition-entries.md#timetable-adr-0005-step-4)):**
  a timeline per panel as well as the rota per person. One view (printable, and as a CSV)
  with the days side by side (Friday, Saturday, Sunday), a column per area
  (Panels 1–3, Track 1, DMT 1) and time running down, showing what's on
  when: each flight's event, warm-up to finish, who is officiating it (chair,
  difficulty, HD and execution judges, recorder, marshal), and blocked time
  (lunch, awards, with any officials it needs). The grid at the bottom of
  the timetable page is close, but it's for editing: on screen only, days
  one under another, and each area a list of flights rather than a time
  scale, so areas don't line up by time.
- **Where things are (2026-10-07).** Every step of ADR 0005 is built and
  live: events across disciplines, officials and the rota, the timetable over
  days and areas, and simulation. Since then: the panel timeline (with and
  without officials), synchro as ISTO runs it (paired levels, one routine, a
  pair's level from their individual ones, synchronisation judges), clearer
  reports (when flights end and the time free after them; empty seats
  counted), and two on-the-day "what ifs", a delay and an official leaving,
  neither of which changes the timetable. The demo is a 25-club, 400-gymnast
  ISTO over three days. The fix for judging "up to" a BUCS level (levels
  were hardest first) went out the same day.
- **Keep the name for now.** Competition and club tools stay on
  `tariff.pardy.ie` even though they go beyond tariffs; a broader name can
  come later.

# Next steps (proposed, 2026-10-07)

Not decided yet: candidates, roughly in the order they'd help ISTO.

1. **Try it for real.** Run the demo on the live site and walk an ISTO
   organiser through it, from entries to the timetable and the on-the-day
   tools; test the competition pages on a phone (see
   [open questions](open-questions.md)). What they find should reorder the
   rest.
2. **Draft and published timetables, and applying a "what if"** (decided
   2026-10-07). Once a timetable is published, attendees see that copy and
   the organiser works on a **draft**: every change (moving a flight,
   redrawing an order, planning again, keeping a delay's or a leaving
   official's result) goes into the draft, and **Publish** makes it what
   attendees see. Today the published timetable is the one being edited, so
   every change goes live straight away. Also close the gaps the "what ifs"
   don't check yet: coaches in two places, the organiser's rules about
   events and people, officials on blocked time, and rest as a must.
3. **Set up in private, go live when ready** (decided 2026-10-07). A new
   competition starts **private**: the organiser sets up levels, events,
   officials and the timetable, and the club and individual entry links
   show the competition's name, date and when entries open, so they can be
   shared early, but take no entries. It goes **live** when the organiser
   presses Go live, or at a date and time they set. The organiser can make
   a live competition private again to pause entries, keeping those already
   made. Today a competition takes entries from the moment it's created
   until its deadline.
4. **Notifications, by opting in** (decided 2026-10-07). Members,
   individuals, club comp secs and coaches can each ask to be told when
   something affects them: a flight they're in moves (time, area or day, or
   its warm-up), their officiating duties change, or their card is checked
   or found to have a problem. A comp sec hears about the club's members, a
   coach about the gymnasts they coach. Timetable and duty changes go out
   when the organiser **publishes**, comparing the new timetable with the
   last one and telling only those whose part changed; running order
   changes alone don't notify. After a change goes live there's a **grace
   period** before anyone is told, and anything else published in it joins
   the same notification. Each one compares what was live before the first
   change with what's live at the end, so a change made and then undone
   tells no one. The organiser can skip the wait with **Notify now**, or
   turn the grace period off for the competition's days, when news can't
   wait. Notifications are **grouped per person told**: a club gets
   one email however many of its members changed, and a gymnast one however
   many of their events. Each **summarises the changes** (who, which event,
   was and now) and links to their page. Notified by **phone push** where it works
   (on iPhone, once the site is on the home screen) and by **email**, which
   clubs always have. Emails are a change to
   [ADR 0004](../docs/adr/0004-server-storage-secret-links.md), which keeps
   none: they'd be given only to be notified and deleted with the
   competition, with a tick box, "I'm 18 or over, or this is a parent's
   email". Push stores nothing personal, so anyone can use it (see
   [open questions](open-questions.md#notifications)).
5. **Link the calculator to the competition tools.** Nothing on the routine
   builder leads to `/competitions` yet.
6. **Synchro's last detail.** Warn the gymnast on their own page too, not
   only the organiser, when a pair is at the wrong level. ("The lower level"
   for a pair a level apart means the easier one, as built: confirmed
   2026-10-07.)
7. **Results history** (competitions 6): import TrampOnline and TScore
   results CSVs for personal bests, progression and next year's levels.
8. **Clubs:** the TrampOnline entry export (clubs 1), then skill tracking
   (clubs 2).
9. **Better defaults** from ISTO: minutes per competitor for tumbling and DMT,
   and the usual rest between turns (both guesses for now).

See [open questions](open-questions.md) for what's waiting on documents and
decisions.

[^tramponline]: TrampOnline (online entries and scoring)
[^tscore]: TScore (competition organiser and scoring)
[^brentwood]: Brentwood Trampoline, organisation of trampoline competitions
[^cop]: FIG Trampoline Code of Points 2025–2028
[^club-software]: Gymnastics club management software round-up (2026)
[^generators]: Vincent Beaulieu's trampoline routine generator
