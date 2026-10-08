# ADR 0009 — Helpers' links, the change history and concerns

- **Status:** Accepted; built 2026-10-08
- **Date:** 2026-10-08
- **Deciders:** jackpardy (solo maintainer)
- **Amends:** ADR 0004 Decision 3 (secret links: a competition has more
  than one admin link)

## Context

A competition has one admin link, and it can do everything: change
settings, remove entries, plan and publish the timetable, delete the
competition. Running ISTO takes more people than the organiser: difficulty
judges check cards while entries are open, a timetable team plans the day,
and on the day each panel's chair of judges needs the sheets. Today each of
them would need the admin link, so anyone could change anything, and
nothing says who did.

Asked on the roadmap (2026-10-08): more admin links each able to do less,
and a change history; chairs of judges as one of them; and a way for
chairs to flag concerns.

## Decision

1. **The organiser makes extra links, each of a kind.** Under Links and
   settings, the organiser names a link (who it's for, e.g. "Difficulty
   judges") and picks what it can do:
   - **Checking cards:** see every entry, print cards, the CSV; mark cards
     checked, with notes; review videos.
   - **Chairs of judges:** see every entry, and print the timetable's sheets
     (chair of judges, score sheets, marshal, panel timeline).
   - **Timetable and officials:** plan, change and publish the timetable
     and the officials (and Notify now), and see the entries.
   - **Everything but links and deleting:** a co-organiser; all but making
     or removing links, replacing the admin link and deleting the
     competition.
   Each is a secret link like the admin link (ADR 0004 Decision 3), kept as
   a hash, shown once when made; the organiser can remove it, and it stops
   working. Up to 20 a competition. The admin link stays the organiser's
   and can do everything.

2. **Every route says who may use it.** One table (`allowed`) maps each
   admin route to the kinds of link that can use it; every admin route goes
   through one gate that finds the link and refuses the rest ("This link
   can't do that"). Pages offer only what their link can do.

3. **Every change is recorded.** Each change made through any admin link
   is kept in the competition's history: when, which link (its name;
   "Organiser" for the admin link) and what, in words ("Marked a card
   checked: Ann Ryan · BUCS L3"). Refused changes aren't. The latest 2,000
   are kept, and all go with the competition. Anyone with an admin link can
   read it.

4. **Anyone with an admin link can flag a concern.** About an entry ("element
   7 isn't on the card") or the competition generally, for the organiser:
   the dashboard lists them, open ones first, with who and when. The
   organiser and co-organisers mark each resolved, with a note. Concerns
   are for the organisers alone; clubs and gymnasts don't see them.

## Consequences

- The organiser can hand out links without handing over the competition,
  and see who changed what.
- A lost helper's link can't be shown again: the organiser removes it and
  makes another.
- New admin routes must be added to `allowed`, or only the organiser and
  co-organisers can use them (the safe default).
- On-the-day tools (flights started and finished, check-in and scratches)
  will likely go to the chairs' links.

## Alternatives considered

- **Accounts with roles.** The proper long-term answer (ADR 0004 Decision
  9), but too much for now; links fit how the tools are already shared.
- **Per-panel chairs' links.** One link for all chairs is simpler; a link
  per panel can come with the on-the-day tools if needed.
