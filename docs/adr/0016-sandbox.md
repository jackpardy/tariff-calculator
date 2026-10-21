# ADR 0016 — A sandbox to try the app in, reset by the maintainer

- **Status:** Proposed (2026-10-21); not built. Decisions for the owner are
  listed at the end
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

## Decision (proposed)

1. **A separate instance, never the live site.** The same app, run as a
   second container with `SANDBOX=1`, its own `DATA_DIR` and its own address
   (e.g. `sandbox.tariff.pardy.ie`). In sandbox mode:
   - **email is off** whatever is set: people will type real addresses into
     "Tell me about changes", and a sandbox mustn't email them. Push stays on
     (they asked for it on their own phone);
   - **every page has a banner**: "Sandbox: made-up people. It's reset from
     time to time; nothing here is kept.";
   - feedback goes to the sandbox's own maintainer page (its own
     `maintainer-link`), apart from the live site's, in Full.

2. **What's in it: ISTO, in two phases.** A sandbox seed, from the demo's
   code, with ISTO's 2025 levels and events instead of BUCS, and two
   competitions, since one can't be in two phases at once:
   - **"ISTO (sandbox): entries open"**: closing three weeks after the reset,
     nothing published. To try entering, coaches signing off, the card
     checker, fees, limits and a waiting list, late changes.
   - **"ISTO (sandbox): on the day"**: dated the day of the reset, over three
     days, the timetable published and the morning under way: flights
     started and finished, check-ins, a scratch, a late arrival, desk
     messages, a notice on the venue screen. To try On the day, the desk,
     the venue screen, My competition and the what-ifs.

   Both have, at every stage, **messages to the organiser**: one just sent
   (Waiting for you), one received, one replied to and answered (waiting
   again), one resolved with a reply and a note, one resolved and opened
   again; helpers' concerns; and feedback on the sandbox's maintainer page.
   Smaller than the demo, so a reset takes seconds: about 6 clubs and 100
   gymnasts, with gymnasts on their own and synchro pairs across clubs.

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
   The site's home page in sandbox mode points to it. A sandbox page whose
   copy has gone (a link from before a reset) says "This was in an earlier
   sandbox" with a link to the start page, not a bare "Link not found".

6. **Reset is a button on the maintainer's page.** In sandbox mode,
   `/site/{token}` has **Reset the sandbox**: it makes a new copy in the
   background, then deletes the old one, so the start page always has a
   working copy and nobody sees it half-made. The page says when it was
   last reset and how long it took. `tariffCalculator sandbox-reset` does
   the same on the server, for a script if ever wanted. Nothing resets by
   itself.

7. **Later, not now: "Make me my own sandbox".** A button on the start page
   that seeds a lighter copy for that visitor, keeps its links in their
   browser and opens its start page (`/sandbox/{copy}`); each own copy
   expires after 14 days (`expires_at`, deleted with the rest of what
   expires), one a visitor an hour (the existing per-address limits), at
   most 50 at once. Decisions 3–5 are what make this a small step: the
   copy, the seed that returns links, and the start page from stored
   links are the same.

## How it's built

- `SANDBOX` read once in `main.go`; email off and the banner in sandbox
  mode; the banner in `competitionLayout` and the calculator's pages.
- Migration: `sandbox_copies`, `sandbox_items`; `Store.DeleteCopy` (in one
  transaction, through the same deletes as `DeleteExpired`).
- `demo`: ISTO levels and events for the sandbox; dates from `now`; the
  morning of the day under way (taps through the day's routes with the
  clock set); the message stages and concerns; `Sandbox` returning links.
- `web/sandboxpages.go`: `/sandbox`, the reset (background, one at a time),
  the gone-copy page; the button on the maintainer's page.
- Tests: a sandbox seed at its size makes both competitions in their
  phases and every message stage; a reset leaves no rows of the old copy
  and nothing outside it touched; email is never sent in sandbox mode; the
  start page lists the current copy's links; a link from an old copy
  leads to the start page.
- Docs: deploy (the second container), the feature page, routes, log,
  roadmap.
- Rough size: one PR, a little more than the feedback page (PR #151). The
  seed's day-under-way part is most of it.

**On the server, for the owner:** a second service in the Compose file
(same image, `SANDBOX=1`, its own `DATA_DIR` and `PUBLIC_URL`, no SMTP
settings); a Caddy site and DNS for the sandbox address; then
`maintainer-link` and one reset in it.

## Decisions for the owner

1. **Who gets it.** Recommended: people **not** in the dry run (ISTO's
   committee, other clubs and coaches) now, and dry-run participants only
   after the dry run, so it doesn't stand in for the walkthrough the dry
   run leaves out.
2. **Size.** About 6 clubs and 100 gymnasts (a reset in seconds), or the
   demo's 25 clubs and 400 (about half a minute, and the timetable looks
   like the real thing)? Recommended: the smaller one; the live demo stays
   the big one.
3. **"On the day" goes stale.** It's dated the day of the reset (over three
   days), so a week later it's over: the day's pages show it as past.
   Recommended: the start page says "On the day is set for 23 October;
   reset to bring it to today", and you reset before sending it to someone.
   (Making the sandbox's clock follow the competition would need a clock
   that differs per competition: not worth it.)
4. **Push in the sandbox:** on (recommended; it's their phone, they asked)
   or off like email.
5. **The address:** `sandbox.tariff.pardy.ie`, or another.
6. **When:** before the dry run (recommended, so the committee can try it
   while the dry run is being set up), after club roles (so it has them)
   or before them (and add them to the seed after).
