---
type: Feature
title: Sandbox
description: A second instance of the app, sandbox.tariff.pardy.ie, full of made-up ISTO clubs and gymnasts in two competitions (one open for entries, one on the day with its morning under way), with a "Try it as…" start page, notifications off, a banner on every page, and the maintainer's buttons to reset it and restart its day.
tags: [feature, sandbox, competitions, demo]
generated: { by: claude-code/cli, at: 2026-10-21T21:30:00Z }
sources:
  - id: adr-0016
    resource: ../../docs/adr/0016-sandbox.md
    title: ADR 0016 — A sandbox to try the app in, reset by the maintainer
    author: human:jackpardy
---

# Summary

The sandbox is the same app run a second time with `SANDBOX=1`, its own
`DATA_DIR` and its own address, `sandbox.tariff.pardy.ie` ([ADR
0016](../../docs/adr/0016-sandbox.md); setting it up:
[deploy](../operations/deploy.md#sandbox-sandboxtariffpardyie)). **Live
since 2026-10-22** at <https://sandbox.tariff.pardy.ie/sandbox>. It's for
ISTO's organisers, and anyone else, to click everything, break things and send
messages with nothing real at stake, before the dry run. Their confusions
reach the maintainer through the Feedback tab, on the sandbox's own
maintainer's page.

# In sandbox mode

- **Nothing is emailed or pushed**, whatever is set up (`SMTP_HOST`,
  `FEEDBACK_EMAIL`, push): the sender drops everything (`sender.send`), the
  notifier has no mailer, and feedback isn't emailed. Where notifications are
  offered it says "Notifications don't work in the sandbox.": **Tell me about
  changes** (no email or push form; a post saves nothing), the desk's help
  and its notice after sending (and a Notify from On the day), and the notice
  after receiving, replying to or resolving a message.
- **Every page has a banner**, "Sandbox: made-up people. It's reset from time
  to time; nothing here is kept.", with a link to the start page: the
  competition and club pages, the calculator's pages, the venue screen and
  the printouts (on screen only). It comes from the request's context
  (`views.WithSandbox`), set for every request in sandbox mode.
- **Feedback** goes to the sandbox's maintainer page in Full (the default),
  never emailed.
- **A link that leads nowhere** under `/clubs/` or `/competitions/` says
  "This was in an earlier sandbox", with **Go to the start page**, rather
  than "Link not found" (the browser forgets it, as with a dead link).
- `/sandbox` is the start page; outside sandbox mode it's a 404.

# What's in it

A **copy** is what one seed makes (`demo.Sandbox(h, size, now)`), from the
demo's code and clubs: 25 student clubs with their coaches and about 400
members, 6 gymnasts on their own, synchro pairs across clubs, the
organiser's own judges, ISTO's 2025 levels (Novice to Elite-Pro entered;
Disability Levels 1–4 offered), synchro in ISTO's three levels (Novice/
Intermediate, Intervanced/Advanced, Elite/Elite-Pro), tumbling Levels 1–4 and
DMT Levels 1–6, men and women apart. The same clubs and members are in both
competitions:

- **"ISTO (sandbox): entries open"**: on five weeks from the reset, closing
  in three, nothing published. Most members have entered; coaches have signed
  off about half; every other club has sent its entries (not the first, the
  club admin's); the first gymnast on their own waits for their coach's
  sign-off; two events have limits and a waiting list; the venue is set up but
  not planned; fees are set, three paid.
- **"ISTO (sandbox): on the day"**: three days, Day 1 to Day 3, day 1 today;
  set up, entered, closed and published four weeks before (the seed runs with
  the clock set back). Its morning is under way (`demo.Morning`, tapped
  through the day pages with the clock set to each tap's time): the panels
  and gymnasts checked in from a quarter of an hour before each flight (a
  judge marked missing then here, a gymnast scratched), the flights started
  and finished until 11:15 (Panel 1 runs 10 minutes late, Panel 3 five), the
  one on then started, a few gymnasts in, with a gymnast checking in late on
  Panel 1, messages from the desk (to a club, a flight's panel, every
  official, and someone asked to come to the desk) and two notices on the
  venue screen.

Both share the clubs and members. About two members in three keep their
voluntary on their page (an Elite both), and about half of those have marked
its skills Can do and one or two of a harder level's Learning, for a coach to
confirm, so the club's Members, a coach's gymnasts and a member's own
Routines and Skills have something in them; the start page's member has both.
Voluntaries are drawn to suit each ISTO level (`style` in demo/routines.go),
so most entries' cards pass.

Both have **messages to the organiser** at every stage: just sent (Waiting
for you), received, replied to and answered (waiting again), resolved with a
reply and a note, and resolved then opened again by the sender; the demo's
member thread; helpers' concerns; and the demo's two pieces of feedback each,
on the maintainer's page.

Everything a seed makes belongs to its copy (migration 51:
`sandbox_copies`, `sandbox_items`): its competitions and clubs (by the
admin links the seed returns), the individuals entered in its competitions
and the feedback sent from them. `Store.DeleteCopy` deletes exactly those, and
anyone who entered only its competitions on their own, in one transaction,
through the same deletes as `DeleteExpired` (deleting the competitions and
clubs cascades to all they hold, club roles' tables included). The copy keeps
its links by role as JSON, for the start page; they are secret links stored
as they are, which a sandbox of made-up people can afford.

# The start page, "Try it as…"

`/sandbox`, from the shared copy's links: a card for each role with a line
of what to try and its links: the **organiser** (the competition open for
entries; On the day and the timetable of the other), **a helper** (checking
cards; a chair of judges), **a club admin** (the first club, which hasn't sent
its entries, and its Settings, for roles and links), **someone in a club,
with a role** (the first club's treasurer and team manager, ADR 0015),
**a coach**, **a member** (their page, My competition, Tell me
about changes), **a gymnast on their own** (their page, their coach's sign-off
link, and entering as someone new) and **the venue screen**. It says notifications don't
work, and when it was last reset; before the first reset, that it's being
set up. The calculator's home page points to it through the banner.

# Reset and the rolling day

On the maintainer's page (`/site/{token}`), in sandbox mode only, a
**Sandbox** box says when it was last reset and how long it took, and has
two buttons, one thing at a time (each greyed while one is under way):

- **Reset the sandbox** makes a new copy in the background (about 17
  seconds at ISTO's size on a laptop), then deletes the old copies, so the
  start page always has one that works. A seed that fails partway has what
  it made deleted at once, and the page says why. `tariffCalculator
  sandbox-reset` does the same on the server.
- **Restart the day**: `Store.RestartDay` moves the competition on the day's
  date (and its deadline, opening and closing times) so day 1 is today, and
  clears what was marked on the day (flights started and finished,
  check-ins and scratches, scratch warnings cleared, panels' check-ins,
  who's up, desk messages and screen notices), in one transaction; then the
  morning is played again. Links, entries, timetable and messages stay.

The app does that by itself once the competition's three days are over (on
its 4th day): when it starts and every hour (`rollDay`).

# Not yet

**Make me my own sandbox** (ADR 0016 Decision 7): a smaller copy for one
visitor, kept in their browser, expiring after 14 days. The copy, the seed
that returns its links and the start page from stored links are what it
would use; `expires_at` is there for it.

# Tests

`web/sandbox_test.go` (at a few clubs): the seed makes both competitions in
their phases and every message stage, the morning under way, and a start
page listing the copy's links, each opening; a reset leaves nothing of the
old copy and a visitor's club untouched, and an old link leads to the start
page; nothing is emailed or pushed with email set up and people subscribed;
the banner is on the sandbox's pages only; on the 4th day the day restarts
with the same links, day 1 that day, the morning played again and messages
kept. `TestSandboxISTOSize` seeds the ISTO size and wants it under a minute
(left out by `go test -short`). `store/sandbox_test.go`: the migration,
`DeleteCopy` touching nothing outside, `RestartDay` and the clocked store.
