# ADR 0016 — A sandbox to try the app in, reset by the maintainer

- **Status:** Accepted (2026-10-21, the owner's decisions at the end); built
  (2026-10-21), alongside club roles (ADR 0015), with the deviations in
  "As built" at the end. Decision 7 is not built
- **Date:** 2026-10-21
- **Deciders:** jackpardy (solo maintainer)

## Context

The live site has a demo competition the maintainer shows people. The owner
wants a **sandbox** to hand out before the dry run: a place where an
organiser, comp sec, coach or gymnast can click everything, break things and
send messages, with nothing real at stake, and where their confusions reach
the maintainer through the Feedback tab (in Full, with a picture and a copy
of the page).

What there is to build on:

- **The demo seed** (`demo.Seed`, `SeedSize` with a `Size`) makes a whole
  competition through the pages, as people would use them, and prints the
  links to open. It uses BUCS levels and fixed dates in February 2027.
- **ISTO's 2025 levels** (Novice to Elite-Pro, Disability 1–4) are built in.
- **The maintainer's page** (`/site/{token}`, `web/maintainerpages.go`)
  is where the maintainer already acts on the whole site.
- **Expiry** (`Store.DeleteExpired`): competitions go after their
  `delete_after`, clubs after 13 months unused.

The dry-run plan gives participants only their link and a task sheet, **no
walkthrough**, to see whether people manage on their own. Anyone who has used
a sandbox first has had one.

The owner chose (2026-10-21) **one shared sandbox, reset by a button**, not
nightly, built so that **"make me my own sandbox"** can follow later as a
small step.

## Decision

1. **A separate instance, never the live site.** The same app, run as a
   second container with `SANDBOX=1`, its own `DATA_DIR` and its own address
   (`sandbox.tariff.pardy.ie`). In sandbox mode:
   - **notifications are off**, email and push, whatever is set: people
     will type real addresses into "Tell me about changes", and a sandbox
     mustn't email them. Every place that offers them (Tell me about
     changes, the desk's and replies' "sending to") says "Notifications
     don't work in the sandbox";
   - **every page has a banner**: "Sandbox: made-up people. It's reset from
     time to time; nothing here is kept.";
   - feedback goes to the sandbox's own maintainer page (its own
     `maintainer-link`), apart from the live site's, in Full. It isn't
     emailed: no email leaves the sandbox, `FEEDBACK_EMAIL` included (the
     owner checks the sandbox's page, decided 2026-10-21).

2. **What's in it: ISTO, in two phases.** A sandbox seed, from the demo's
   code, with ISTO's 2025 levels and events instead of BUCS, and two
   competitions, since one can't be in two phases at once:
   - **"ISTO (sandbox): entries open"**: closing three weeks after the reset,
     nothing published. To try entering, coaches signing off, the card
     checker, fees, limits and a waiting list, late changes.
   - **"ISTO (sandbox): on the day"**: three days, day 1 today, the
     timetable published and the morning under way: flights started and
     finished, check-ins, a scratch, a late arrival, desk messages, a notice
     on the venue screen. To try On the day, the desk, the venue screen, My
     competition and the what-ifs. **It rolls:** on the 4th day it starts
     again from day 1 (see 6).

   Both have, at every stage, **messages to the organiser**: one just sent
   (Waiting for you), one received, one replied to and answered (waiting
   again), one resolved with a reply and a note, one resolved and opened
   again; helpers' concerns; and feedback on the sandbox's maintainer page.
   **ISTO-sized**, like the demo: about 25 clubs and 400 gymnasts, with
   gymnasts on their own and synchro pairs across clubs, so the entries and
   the timetable look like the real thing. A reset takes about half a
   minute (in the background, 6). A copy of your own (7) is smaller.

3. **Everything the seed makes belongs to a copy.** A table
   `sandbox_copies (id, created_at, expires_at, size, links JSON)` and
   `sandbox_items (copy_id, kind, id)` (competitions, clubs, individuals the
   copy made). Deleting a copy deletes exactly those, with everything they
   hold, so a reset can't touch anything else. `expires_at` is empty for
   the shared copy (it lasts until the next reset); it's what "make your
   own" will use (7).

4. **The seed is a function that returns its links.** `demo.Sandbox(h, size,
   now)` makes a copy and returns its links by role, using the same seeder
   as the demo (the demo, `tariffCalculator demo`, stays as it is for the
   live site and the load test). The links are kept with the copy, so the
   start page can show them.

5. **A start page, "Try it as…".** `/sandbox` (sandbox mode only), from the
   current copy's links: a card for each role with a line of what to try
   and its link:
   - **Organiser**: entries, the timetable and On the day;
   - **A helper**: checking cards; a chair of judges;
   - **A comp sec**: enter members and send the club's entries; messages
     to the organiser;
   - **A coach**: sign off routines; gymnasts' skills;
   - **A member**: enter, My competition, hearing about changes;
   - **A gymnast on their own**: enter, a coach's sign-off by link;
   - **The venue screen**;
   - and, once built, a club role (ADR 0015).
   The page also says notifications don't work in the sandbox. The site's
   home page in sandbox mode points to it. A sandbox page whose
   copy has gone (a link from before a reset) says "This was in an earlier
   sandbox" with a link to the start page, not a bare "Link not found".

6. **Reset is a button on the maintainer's page.** In sandbox mode,
   `/site/{token}` has **Reset the sandbox**: it makes a new copy in the
   background, then deletes the old one, so the start page always has a
   working copy and nobody sees it half-made. The page says when it was
   last reset and how long it took. `tariffCalculator sandbox-reset` does
   the same on the server, for a script if ever wanted.

   **The day rolls by itself.** Only the on-the-day competition: once its
   three days are over (on what would be its 4th day), the app **restarts
   the day**: it moves the competition's dates so day 1 is today, clears
   what was marked on the day (check-ins, flights started and finished,
   who's up, scratches, desk messages, screen notices, panels' check-ins)
   and replays the seeded morning, with the clock set. Its links, entries,
   timetable and messages to the organiser stay. The app checks when it
   starts and every hour. The maintainer's page has **Restart the day**
   too, to do it at once. Everything else resets only by the button.

7. **Later, not now: "Make me my own sandbox".** A button on the start page
   that seeds a lighter copy for that visitor, keeps its links in their
   browser and opens its start page (`/sandbox/{copy}`); each own copy
   expires after 14 days (`expires_at`, deleted with the rest of what
   expires), one a visitor an hour (the existing per-address limits), at
   most 50 at once, and **smaller** (a few clubs) so it's made in seconds.
   Decisions 3–5 are what make this a small step: the
   copy, the seed that returns links, and the start page from stored
   links are the same.

## How it's built

- `SANDBOX` read once in `main.go`; notifications off, "Notifications don't
  work in the sandbox" where they're offered, and the banner in
  `competitionLayout` and the calculator's pages.
- Migration: `sandbox_copies`, `sandbox_items`; `Store.DeleteCopy` (in one
  transaction, through the same deletes as `DeleteExpired`).
- `demo`: ISTO levels and events for the sandbox; dates from `now`; the
  morning of the day under way (taps through the day's routes with the
  clock set), as its own step so restarting the day replays it; the
  message stages and concerns; club roles and their links (ADR 0015);
  `Sandbox` returning links.
- `Store.RestartDay`: move a competition's dates and clear its day marks,
  in one transaction; the hourly check beside `DeleteExpired`.
- `web/sandboxpages.go`: `/sandbox`, the reset (background, one at a time),
  the gone-copy page; the button on the maintainer's page.
- Tests: a sandbox seed makes both competitions in their phases and every
  message stage; a reset leaves no rows of the old copy and nothing
  outside it touched; nothing is emailed or pushed in sandbox mode; the
  start page lists the current copy's links; a link from an old copy
  leads to the start page; on the 4th day the day restarts with the same
  links, day 1 today and the seeded morning, messages kept.
- Docs: deploy (the second container), the feature page, routes, log,
  roadmap.
- Rough size: one PR, a little more than the feedback page (PR #151). The
  seed's day-under-way part is most of it.

**On the server, for the owner:**
- a second service in the Compose file (same image, `SANDBOX=1`, its own
  `DATA_DIR`, `PUBLIC_URL=https://sandbox.tariff.pardy.ie`, no SMTP
  settings);
- **DNS:** a record for `sandbox.tariff.pardy.ie` pointing at the server
  (e.g. a CNAME to `tariff.pardy.ie`), unless a wildcard `*.pardy.ie`
  already does;
- **Caddy:** a site block for `sandbox.tariff.pardy.ie` passing to the
  sandbox container. Caddy gets its HTTPS certificate by itself once the
  DNS points at the server;
- then `maintainer-link` and one reset in it.

## The owner's decisions (2026-10-21)

1. **Who:** ISTO's organisers. Some may be in the dry run too; the owner
   doesn't see that as a problem.
2. **Size:** ISTO-sized (about 25 clubs, 400 gymnasts) for the shared
   sandbox; a copy of your own (7) smaller.
3. **On the day rolls:** three days, then back to day 1 on the 4th, by
   itself (Decision 6), rather than a button for visitors.
4. **Notifications off** (push too), with "Notifications don't work in the
   sandbox" said where they're offered.
5. **The address:** `sandbox.tariff.pardy.ie`.
6. **When:** alongside club roles (ADR 0015), before the dry run, with the
   sandbox seeding club roles.

## As built (2026-10-21)

Built as decided ([the feature](../../knowledge/features/sandbox.md)), but:

- **The seed's handler is a clock's.** `demo.Sandbox(h, size, now)` takes
  `h` as a `demo.Clocked`, the site's handler at a clock it moves, not one
  handler: the competition on the day is set up and entered four weeks
  before (it can't be entered after its deadline), and its morning is tapped
  at each tap's time. The web layer gives it a handler of its own over the
  store with its own clock (`Store.Clocked`), so the live app's clock never
  moves.
- **The morning is today's**, 09:00 to 11:15, whenever the reset or restart
  happens: before 09:00 its taps are ahead of the clock.
- **A copy holds the feedback its seed sent**, and anyone who entered only its
  competitions on their own, so a reset leaves neither behind; feedback
  people send stays.
- **"This was in an earlier sandbox"** is any link under `/clubs/` or
  `/competitions/` that leads nowhere, in sandbox mode: a link from an old
  copy can't be told from a mistyped one, and in the sandbox it nearly always
  is the former.
- **ISTO's levels**: Novice to Elite-Pro are entered, Disability Levels 1–4
  offered with nobody in them; the days are "Day 1" to "Day 3", as their
  dates move.
- **Club roles aren't seeded yet**, nor their card on the start page: they
  are built on another branch, and go in once both are merged (the start
  page and `demo.Links` say where). The sandbox's clubs are the demo
  seeder's, so what that branch adds to them comes too.
- **Migration 51** is the 50th on its own branch, until club roles' 50 is
  merged before it.
- **The last reset** (when, how long) is kept in `settings`.
