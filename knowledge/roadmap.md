---
type: Roadmap
title: Roadmap
description: Where the project stands and what is left before the dry run, before ISTO 2027's entries open and before the day itself, with what waits on others, what is not planned, the standing decisions and a one-line list of what is built.
tags: [roadmap, competitions, clubs, planning]
status: current
generated: { by: claude-code/cli, at: 2026-11-13T12:00:00Z }
stale_after: 2026-12-15T00:00:00Z
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
---

# Why and where it stands

The routine tools check a routine's tariff and requirements; the competition
tools now take a competition's entries, check every card, plan the timetable
and officials, and run the day, with clubs keeping their members, routines and
skills here. Scoring and results stay with TrampOnline,[^tramponline] which
these tools complement rather than replace. The aim is to run the Irish
Student Trampoline Open (ISTO) in April 2027 on them
([ADR 0005](../docs/adr/0005-multi-discipline-timetabling.md)). Everything is
phone first, plainly worded and built on one engine, so rules live only in Go
([rendering](architecture/rendering.md)).

# Dates

| When | What |
|---|---|
| Not fixed yet | The dry run (planned for late 2026; no date or pass mark set) |
| 2027-01-01 | BUCS and BG national rule sets due for review ([BUCS](requirements/bucs.md), [BG national](requirements/bg-national.md)) |
| Early 2027 | ISTO's entries open (exact date not confirmed) |
| 2027-02-01 | ISTO 2025 levels due for review: 2027's routines expected ([ISTO](requirements/isto.md)) |
| February 2027 | ISTO's timetable drafted |
| March 2027 | A full rehearsal of the day (date not confirmed) |
| Last two weeks before ISTO | Fixes only |
| Early April 2027 | ISTO (exact date not confirmed) |
| 120 days after its date | A competition's entries and everything about its people are deleted; its set-up is kept to copy (sooner if the organiser chooses) |
| 3 years after its date | A kept set-up is deleted (sooner if the organiser chooses) |
| 30 days after it's made | A rehearsal copy is deleted (once 30 days more; sooner if the organiser chooses) |

# Now: before the dry run

The dry run tries the core tools with about 20 people from 3 clubs: entries
first, then a half day on the tools. It has no date and no pass mark yet.
What is left is mostly the owner's, on the server (see
[the owner's, on the server](#the-owners-on-the-server)), and trying on real
phones what hasn't been: push notifications, pages working offline, drag to
reorder, printing the tariff sheet, the share sheet and QR scanning, the
keyboard with the skill card open, Levels mode's tabs on a narrow screen and
the view screen held either way
([open questions](open-questions.md#not-yet-tested-on-a-real-phone)).

# Next: before ISTO's entries open

1. **Notifications and emails kept across a restart**: a table of deliveries
   still to send, so a deploy doesn't drop them, and time limits on sending an
   email. From the November audit; not built.
2. **Feedback made safe for real entries**: built 2026-11-13: a capture no
   longer keeps other people's working links (`data-private` blurred and
   blanked, secret-link paths cleaned, on the server too), a copy can't load
   from elsewhere, feedback is deleted at 90 days with a Delete for each, and
   there's a limit per address and per link
   ([feedback](features/competition-entries.md#feedback-about-the-app)). Left
   to the owner: **switch capture from Full to Light** on the maintainer's page
   before real entries open
   ([deploy](operations/deploy.md#notifications)).
3. **Tell someone let in from a waiting list** when another withdraws. Not
   built ([limits and waiting lists](features/competition-entries.md#limits-and-waiting-lists)).
4. **Small audit items**: an entry let in over a limit stays let in after it
   moves level; the public individual-entry link has no rate limit; a second
   block with the same name on a day is dropped without a word; an event
   longer than a day is offered only a cap; no copy of the database before a
   migration; privacy notes behind the features; ADRs for site feedback and
   for messages; missing indexes.
5. **Know when it breaks**: an outside uptime check, an email on errors and
   panics, and a restore drill.
6. **A lost organiser or club admin link**: a command that issues a new one.
7. **Link the Routine Builder to the competition pages**: still nothing
   leads from it to `/competitions`.
8. **ISTO's 2027 routines**, swapped in when the document arrives, saying
   what happens to routines already checked against 2025's.
9. **Hand-off to scoring: to be decided.** Clubs used to enter by hand on
   TrampOnline and ISTO exported a CSV of the entries; TrampOnline has a bulk
   upload tool, but how it works isn't known yet (the owner, 2026-11-13).
   Waiting on a sample of that format.
10. **Timings learnt from the day**: the Warm-up over tap measures warm-up
    and competing apart and **Real times today** sets them beside Setup's
    ([flight times](features/competition-entries.md#flight-times-roadmap-15));
    better defaults for tumbling, DMT and rest between turns follow from the
    dry run ([open questions](open-questions.md)).

# Then: before the rehearsal and ISTO

- **The March rehearsal** on a [rehearsal copy](features/competition-entries.md#copying-a-competition)
  of the real competition with its entries (built 2026-11-13), at the venue if possible, a script standing in for the crowd, and
  15 minutes with the app switched off to practise the paper fallback.
- **A page for the day's runbook**: restart, restore, roll back, hold
  deploys.
- **A way to hold deploys** over a competition weekend.
- **For the weekend**: a 4G/5G router for the organiser, marshals and chairs
  (and a cable for the venue screen); everyone's times by push and email the
  night before, so their page is saved on their phone; the server scaled up
  (charged by the hour; keep the disk size so it can scale back; a minute or
  two offline, so the day before).
- **Nothing but fixes** for the last two weeks.

# After ISTO

- **Results history**: import TrampOnline and TScore results CSVs for
  personal bests, progression and next year's levels; for Elite and
  Elite-Pro, two voluntaries with the higher score ranking and both tariffs
  kept for ties.
- **Club tools not yet built**: a session planner (trampolines, gymnasts and
  time give a turn rotation with spotters, needing no storage), and judge and
  coach practice (tariff and notation quizzes, card checking with planted
  errors, execution building on [execution judging](domain/execution.md)).
- **A member moving club** taking their routines, marks and progress (for
  now they stay with the club).
- **Archive a competition** once it's over: kept, read only, out of the way
  (noted 2026-10-19; not designed).

# Waiting on others

- A past ISTO timetable with flight sizes and run times.
- TrampOnline's entry and bulk-upload formats.
- ISTO's score sheets.
- ISTO's 2027 routines.
- The coaching and judging qualification lists.
- ISTO 2027's venue, fees and closing dates.

See [open questions](open-questions.md) for the detail.

# The owner's, on the server

- **Email**: Amazon SES, with DKIM, SPF and DMARC
  ([steps](operations/deploy.md#notifications)); the owner said on
  2026-11-13 they will do it this weekend, 14 to 15 November 2026.
- **The stop timeout** set to 40 seconds, so requests under way finish on a
  deploy.
- **The access log keeps secret links for 90 days**: a log filter, needing
  the owner's go-ahead.
- **A load-test run on the server** ([load test](operations/deploy.md#load-test)).
- **The backup restore drill.**
- **An uptime check.**
- **Before sending anything out**: reset the sandbox and rebuild the guide
  PDFs.

# Not planned

- **Checking tumbling and DMT cards automatically**: likely not (the owner,
  2026-11-13); those entries are a level only and say so.
- **The difficulty judge helper** (tapping elements as performed against the
  card): too hard to make reliable on the day.
- **The routine suggester**: a legal, high-tariff routine isn't necessarily
  one a gymnast should compete.
- **The committee handover kit**: documents, not software.
- **Replacing TrampOnline's scoring**: complement it instead.
- **A coach per discipline, and synchro signed off by both coaches**: only if
  clubs ask.
- **Splitting an event by routine**: only if ISTO's timetable needs it
  ([ADR 0006](../docs/adr/0006-one-panel-per-event.md)).
- **The builder redesigns A to F**: overtaken by editing a skill in its card.

# Decisions that still hold

- **Secret links, no accounts**; personal data kept to names and clubs, and a
  competition's entries deleted 120 days after it
  ([ADR 0004](../docs/adr/0004-server-storage-secret-links.md)).
- **Complement TrampOnline for scoring**, keeping replacement possible.
- **Plain wording and phone first** everywhere ([the shared look](architecture/ui.md)).
- **One engine for rules**, in Go ([ADR 0003](../docs/adr/0003-requirements-framework.md)).
- **One panel for a whole event**, its flights back to back on one area
  ([ADR 0006](../docs/adr/0006-one-panel-per-event.md)).
- **Coaches aren't checked for being in two places** (2026-10-19: their
  lines flooded the list).
- **A gymnast may be in two synchro pairs** in one event, with different
  partners; never flagged.
- **A synchro pair does one routine** on the day.
- **Clubs kept 13 months after last real use**
  ([ADR 0011](../docs/adr/0011-club-routines-and-skill-tracking.md)).
- **Club roles from one club admin link**
  ([ADR 0015](../docs/adr/0015-club-roles.md)).
- **Keep the name**: competition and club tools stay on `tariff.pardy.ie`; a
  broader name can come later.

# Done

Dates are when built, where known.

## Routine tools

- [Changing a skill in its card](features/routine-builder.md#changing-a-skill-in-its-card), its head showing the change (2026-10-22, 2026-10-23).
- [Requirements written beside the routine](features/routine-builder.md#requirements-and-checks), each rule Met or Not met, and a Check against picker with search (2026-10-16).
- [The routine tools in the competition pages' look](architecture/ui.md#the-routine-builder-and-the-requirements-page) (2026-10-22).
- [Skill names and search](domain/skill-catalog.md): twisting singles to triples named, found by the names people type (2026-10-19).
- [Execution judging rules](domain/execution.md) (2026-10-07).

## Requirements and levels

- [A skill picked in the rule editor, and rule kinds named by what they mean](requirements/framework.md#the-editor) (2026-10-15).
- [Requirements and levels kept with the person](features/competition-entries.md#requirements-on-your-page-adr-0013) ([ADR 0013](../docs/adr/0013-requirements-with-the-person.md), 2026-10-15).
- [Club levels](features/competition-entries.md#club-levels-adr-0011) edited on the club page, starting from a set routine (2026-10-15, 2026-10-16).
- [Club skills](features/competition-entries.md#club-skills-adr-0011) and [sharing club levels](features/competition-entries.md#sharing-club-levels-adr-0011) (2026-10-15).

## Entries and clubs

- [Card collection and checking](features/competition-entries.md#dashboard), live at `/competitions` (2026-10-06).
- [Synchro events](features/competition-entries.md#events-and-synchro-adr-0005-step-2): paired levels, a pair at the wrong level flagged (2026-10-07).
- [Going live](features/competition-entries.md#going-live) (2026-10-08).
- [Approved coaches](features/competition-entries.md#coaches-coach-link) ([ADR 0007](../docs/adr/0007-approved-coaches.md), 2026-10-08).
- [Removing and holding entries](features/competition-entries.md#removing-and-holding-entries) (2026-10-08).
- Finding your way round the [dashboard](features/competition-entries.md#dashboard): levels folded, search, filters, sort (2026-10-08); synchro partners see the pair's entry (2026-10-08).
- [Helpers' links, history and concerns](features/competition-entries.md#helpers-links-history-and-concerns-adr-0009) ([ADR 0009](../docs/adr/0009-helper-links.md), 2026-10-08).
- [Video proof](features/competition-entries.md#video-proof) by link.
- [Menus and the Settings page](features/competition-entries.md#the-menu-and-the-settings-page-roadmap-2026-10-14) and [All people](features/competition-entries.md#all-people-roadmap-2026-10-14) (2026-10-14).
- [One link, several roles](features/competition-entries.md#one-link-several-roles-adr-0010) and [entering on your own](features/competition-entries.md#entering-on-your-own-one-page) ([ADR 0010](../docs/adr/0010-one-link-several-roles.md), 2026-10-14).
- [Search and filters on every list of people](features/competition-entries.md#searching-lists-of-people-roadmap-item-6) (2026-10-14; counts and chips 2026-10-19).
- [Routines on your page](features/competition-entries.md#routines-on-your-page-adr-0011) and [skills](features/competition-entries.md#skills-adr-0011) ([ADR 0011](../docs/adr/0011-club-routines-and-skill-tracking.md), 2026-10-15).
- [Club roles and links](features/competition-entries.md#club-roles-and-links-adr-0015) (2026-10-21).
- Joining with several people on one phone: the join page lists the phone's pages at the club, a known name asks first (2026-11-11).
- Who sends: four club settings for members sending their own, and the organiser taking only what clubs send ([late changes](features/competition-entries.md#late-changes), 2026-11-11).
- [Removed by the club](features/competition-entries.md#removed-by-the-club): removed members and coaches stay visible to the organiser, sign-offs can be asked for again, a pair whose partner has gone is flagged, a member can be taken out of one competition (entries, offer to judge, or both), and fees for removal after entries close (2026-11-13).
- [Copying a competition](features/competition-entries.md#copying-a-competition): its set-up for another, or with its entries as a private rehearsal copy that sends nothing and is deleted after 30 days; a finished competition's set-up kept for 3 years (2026-11-13).

## Fees, limits, late requests

- [Entry fees](features/competition-entries.md#entry-fees) (2026-10-08).
- [Limits and waiting lists](features/competition-entries.md#limits-and-waiting-lists) (2026-10-08).
- [Late changes](features/competition-entries.md#late-changes) (2026-10-08), [closing later than new entries](features/competition-entries.md#when-entries-changes-and-sign-offs-close) (2026-10-09).
- [Late requests](features/competition-entries.md#late-changes) to enter, with replies and the coach's sign-off showing (2026-11-11); an unsent change after closing shows what the competition holds (2026-11-11).

## Timetable and officials

- [Officials](features/competition-entries.md#officials-adr-0005-step-3), [the rota](features/competition-entries.md#officials-rota-adr-0005-step-5), [the timetable](features/competition-entries.md#timetable-adr-0005-step-4) and [simulation](features/competition-entries.md#simulation-adr-0005-step-6): every step of ADR 0005 (2026-10-07), the panel timeline (2026-10-07).
- One panel per event ([ADR 0006](../docs/adr/0006-one-panel-per-event.md), 2026-10-08); drafts and publishing (2026-10-08).
- [Personal and club timetables](features/competition-entries.md#personal-and-club-timetables) and [My competition](features/competition-entries.md#my-competition-roadmap-competitions-5) (2026-10-08).
- [What if there's a delay](features/competition-entries.md#what-if-theres-a-delay) and [what if an official has to leave](features/competition-entries.md#what-if-an-official-has-to-leave), with their missing checks (2026-10-16); an official leaving puts no one in two places (2026-11-11).
- Saying what a manual change breaks, with fixes (2026-10-16); re-planning keeps what's marked on the day (2026-10-19).
- Judges' qualifications ([ADR 0014](../docs/adr/0014-judges-qualifications.md), 2026-10-16) and levels by role ([ADR 0017](../docs/adr/0017-judge-levels-by-role.md), 2026-10-22); seat problems and who's free for a seat (2026-10-22).
- A judge's level that has gone: the offer takes that role at no level and says so (2026-11-11); renaming a level can carry judges' levels over, if the organiser chooses (2026-11-13).
- [Flight times of their own](features/competition-entries.md#flight-times-roadmap-15): warm-up and competing apart, an event's own, a flight's own (2026-11-11).
- Recorders' score sheets (2026-10-08).

## The day

- [Flights started and finished, check-in, scratches and panel check-in](features/competition-entries.md#on-the-day-flights-started-and-finished) (2026-10-09); check-in keeps the flight open (2026-10-14).
- Taps without signal and Correct the times (2026-10-15); kept across a restart (2026-11-11).
- The Warm-up over tap and Real times today (2026-11-11).
- [The venue screen](features/competition-entries.md#the-venue-screen) (2026-10-09), its changes and who's up (2026-10-14); a synchro pair does one routine (2026-11-11).
- [Offline pages](features/competition-entries.md#offline) and add to calendar (2026-10-09).
- [States and roles in one palette](features/competition-entries.md#states-and-roles-on-the-pages-one-palette-step-2) (2026-10-15).

## Notifications, feedback and messages

- [Notifications](features/competition-entries.md#notifications-adr-0008) by push and email ([ADR 0008](../docs/adr/0008-notifications.md), 2026-10-08); email waits on SES.
- [Messages from the organisers' desk](features/competition-entries.md#messages-from-the-organisers-desk) (2026-10-09).
- Sending in the background ([ADR 0012](../docs/adr/0012-sending-in-the-background.md), 2026-10-15).
- [Feedback about the app](features/competition-entries.md#feedback-about-the-app) (2026-10-15; split from messages 2026-10-20; competitions made 2026-10-22).
- [Messages to the organiser](features/competition-entries.md#messages-to-the-organiser) with threads (2026-10-21); shown only to the organiser's links (2026-11-11).

## Operations

- Storage live on the self-hosted server, backed up nightly ([competition storage](operations/deploy.md#competition-storage), 2026-10-06).
- Faster and smaller pages (2026-10-15, again 2026-10-19; [load test](operations/deploy.md#load-test), [compression](architecture/static-assets.md#compression)); lighter pages for phones (2026-10-19).
- The cleanup audit's fixes ([report](audits/2026-10-cleanup.md), 2026-10-15) and the full audit's (2026-10-19), including [one look](architecture/ui.md).
- [The sandbox](features/sandbox.md) ([ADR 0016](../docs/adr/0016-sandbox.md), live 2026-10-22).
- [CI building the image](operations/deploy.md#ci) the server loads (2026-10-23).
- The November audit's fixes: the vulnerability check, stopping and panics, uploads given 90 seconds, history leaving out refusals, requirements following the person (2026-11-11).
- [Browser tests made reliable](architecture/testing.md#browser-tests-how-to-wait) (2026-11-11).

[^tramponline]: TrampOnline (online entries and scoring)
