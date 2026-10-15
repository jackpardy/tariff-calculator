---
type: Report
title: Cleanup audit, October 2026
description: The full audit of the app before the dry run (14–15 October 2026), what it found in each area, what was fixed, and what was left for the dry-run work or after it, with the measurements before and after.
tags: [audit, cleanup, quality]
generated: { by: claude-code/cli, at: 2026-10-15T16:00:00Z }
---

# Cleanup audit, October 2026

The review and cleanup stop agreed for after roadmap item 7. A read-only audit
of the whole app (about 45,000 lines, 176 routes, 34 page templates) found
about 130 things to fix, three of them security. All but those left for the
dry-run work or after it were fixed on `bulk/cleanup` on 15 October 2026. The
working copy of the report, with every finding, is a Claude Doc the owner
holds; this is the record kept with the code.

## How it was done

1. **Baseline**: tests, coverage, `go vet`, staticcheck, deadcode, govulncheck,
   and the main pages' sizes and times on the demo (709 entries).
2. **Audit**, read-only, in eight areas: structure and data model (A),
   duplication and dead code (B), correctness and security (C), performance
   (D), tests (E), pages at 375px and 1280px (F), docs (G), running it (H).
   Opus agents did A, B, C, E and F, a Sonnet agent G; D and H by hand.
3. **Triage**: the owner chose to fix everything, highest value first.
4. **Fixes** in waves of parallel branches, each merged into `bulk/cleanup`
   with the tests run after every merge: security and data, bugs users would
   hit, a test safety net, running it, identity, timetable integrity, page
   fixes, code tidying, the day and timetable pages, wide tables on a phone,
   docs.

## Before and after

| Measure | Before (3bbfc07) | After |
|---|---|---|
| Timetable page | 6.1 MB, 152 ms | 1.0 MB, 97 ms |
| On the day | 744 KB, 64 ms | 786 KB, 31 ms |
| A member's My competition | 111 KB, 85 ms | 112 KB, 67 ms |
| Dashboard | 527 KB, 49 ms | 683 KB, 63 ms (labels for phone cards, search data per row; compression will take it back) |
| Web tests | about 26 s | about 10 s |
| govulncheck | 40 reachable Go issues (Go 1.25.0) | none (Go 1.26.9) |
| staticcheck | 5 findings | none |
| Migrations | 32 | 36, checked on a copy of the live database |

## What was fixed, by area

- **Security** (C, A):
  - a link the comp sec makes or replaces opens only that club's records (ADR 0010 Decision 8);
  - certificates are limited to 3 MB, with 60 MB totals per club and per competition and a free-disk check;
  - no secret link is kept in subscriptions or written to logs;
  - one off link stops every email to the address for that competition;
  - confirmation emails carry no organiser-chosen names, 50 a day per competition;
  - phones drop offline copies after 30 days or once a link is gone.
- **Stored data** (A, C):
  - timetable edits read, change and save in one transaction, and a form opened before someone else's change is refused;
  - flights have stable ids, so flight times, check-ins and who's up follow a flight that's moved and republished (migration 34);
  - who's up is saved as gymnast and round;
  - one person across roles in the planner, Problems, delays and the rota;
  - coaches keyed by their record (35);
  - individuals' notifications belong to the person (36);
  - a newer database refuses an older app;
  - rows left behind by deletions are cleaned up.
- **Bugs users would hit** (F, C):
  - the comp sec's page shows each entry's own flight;
  - editing keeps Men or Women ticked;
  - taps on the day are safe to repeat on poor wifi;
  - partner confirmation starts on "Choose who you are";
  - contrast is at least 5.3:1;
  - lateness is shown by the clock and in hours;
  - the demo's waiting lists fill again.
- **Pages** (F):
  - names are sorted ignoring case and accents, and the browser's search matches the server's;
  - problems are written as plain sentences, and "Custom Skill" is gone;
  - search on the comp sec's Competitions, On the day and the Timetable, with jump links;
  - My competition is tidied up, and the dashboard's print buttons are merged;
  - 40px tap targets on the day;
  - wide tables become cards on a phone (dashboard, comp sec, Officials, Fees);
  - print sheets work on a phone;
  - wording made consistent.
- **Running it** (H):
  - Go 1.26.9;
  - govulncheck in CI;
  - `/healthz` fails when storage didn't open, and the deploy job checks it;
  - SIGTERM finishes requests and closes the database.
- **Tests** (E):
  - an end-to-end test of the dry run's path;
  - the day's tests run on a fixed clock;
  - specific assertions in place of page-wide word checks;
  - coverage of late changes, fees, the desk, delays and who's up;
  - Chrome tests time out per step;
  - a small demo seed by default;
  - a shared test kit.
- **Code** (B, A):
  - shared helpers in place of copied page-building: `whose`, `liveRota`, `competitionLinks`, one entry state, one `FlightGymnast`;
  - person keys in one file;
  - the who's-up rules in `competitions`;
  - dead code and CSS removed;
  - the largest files split.
- **Docs** (G):
  - the 24 missing routes;
  - ADR statuses;
  - the stack and features index;
  - a contents list for the features page.

## Left, and why

- **For the dry-run work**, as agreed:
  - keeping each competition's entries, levels and rota in memory rather than working them out on every load;
  - compressing pages;
  - sending desk messages and notifications in the background (today they send inside the request).
- **After the dry run:**
  - the marshal's flow as Start competing, then Done (buttons now read Started and Finished);
  - moving gymnasts around the running order on the day;
  - one error style for every handler;
  - the day and timetable pages going through `liveRota`;
  - a member who also enters on their own still counts as two people on scratch warnings, check-in, the desk and My competition (the planner and All people treat them as one).
- **On the server, for the owner:**
  - restore a nightly backup once to prove it works;
  - add an outside uptime check on `/healthz`.
