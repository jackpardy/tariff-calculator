---
type: Practice
title: Testing and checks
description: What the Go tests cover (the 139 CoP examples, the engine, requirements, catalog and rendered HTML) and the checks to run before every commit.
resource: https://github.com/jackpardy/tariff-calculator/blob/master/web/main_test.go
tags: [testing, ci, practice]
generated: { by: claude-code/cli, at: 2026-10-05T14:20:00Z }
---

# Before every commit

```sh
go tool templ generate   # regenerate *_templ.go; CI fails if they're stale
gofmt -l .               # must print nothing
go vet ./...
go test ./...
```

Commit only if all of these pass. Work happens on a feature branch, and merging
to `master` deploys ([deploy](../operations/deploy.md)), so each merge is
approved by the maintainer first.

# What the tests cover

| File | Covers |
|---|---|
| `skills/cop_examples_test.go` | Every worked example in the Code of Points (139), so the [tariff](../domain/tariff.md) can't drift |
| `skills/skills_test.go` | Landing, notation, naming, equality (repeats), validation, interruptions, scored elements, JSON round-trip |
| `requirements/requirements_test.go` | Each rule type, matching, assignment, the built-ins loading and parsing, set routines |
| `requirements/level_test.go` | Parsing levels, and every built-in level referring to real requirements (and every built-in set belonging to a level) |
| `requirements/check_test.go` | Which checks apply to a routine, resolving references, and checking a level's two exercises together (carry-over and repeats) |
| `competitions/competitions_test.go` | Validating competitions and entries, and checking an entry as the builder does (set routines as prescribed, AG3 carry-over, a coach's own level) |
| `store/store_test.go` | Migrations, every kind of link (hashed, replaced, wrong), individual and club entries, sending and re-sending, deadlines, limits and deletion (competitions 120 days after, clubs 13 months unused), each on a fresh SQLite file |
| `store/migrations_test.go` | Refusing a database migrated further than the app knows, a table rebuild run with foreign keys off (and refused if a reference breaks), and migration 33 clearing notifications' page paths, and 36 giving individuals' notifications to the individual |
| `store/personkey_test.go`, `web/samepeople_test.go` | Person keys: each kind made and read back, an entry's coach keyed by record, every column holding keys covered by `renameKey` (a schema check), migration 35 moving saved coach keys, and one person across roles in the timetable: a member who coaches (not checked) or enters on their own warned against themselves and kept apart by the planner; a coach's gymnasts on two areas break nothing, and people in two places come first |
| `competitions/problems_test.go` | An entry's problems in one order (pair levels, card, sign-off), as the organiser's, club's and coach's pages count them |
| `store/certificates_test.go` | Certificates' limits: 3 MB each, 60 MB a club and a competition's individuals, none on a nearly full disk |
| `catalog/*_test.go` | Picker categories, shape options, search by name, alias and notation |
| `static/static_test.go` | Hashed URLs and caching headers |
| `web/main_test.go` | Handlers end to end, asserting on rendered HTML: flags, custom names, checks, sheet, compare, requirements, side by side, QR, sharing, oversize bodies; every template's `js:` `hx-vals` being an object literal (htmx wraps anything else in braces, which broke the sheet) |
| `web/tools_test.go` | The routine tools' look: the banner on the builder, Requirements and compare, no built-in sets carried in the Requirements page, a card's controls; and in Chrome, Remove with Undo, Levels mode's New level chooser, and Delete asked on a Requirements row (never `confirm()`) |
| `web/browser_test.go` | Pages loaded in headless Chrome (chromedp), so their JavaScript runs: the tariff sheet loads the routine saved in the browser and shows its elements and total. It needs Chrome or Chromium (CI's Ubuntu runners have it; `CHROME_PATH` points at one) and skips without, as in the Docker build, but fails without it when `CI` is set. Each step has its own 15-second timeout (`step`) and a failure names the step |
| `web/comppages_test.go` | The competition pages on a fresh store: storage off, creating (problems, the admin link, replacing it, deleting), the rate limit, and individual entry from form to dashboard, change and withdrawal |
| `web/comppages_test.go` (`TestOrganiserTools`) | Printing cards (filled in, filtered), the CSV (with formulas made harmless), marking checked with a note the gymnast sees, a change unchecking it, and closing and reopening entries |
| `web/comppages_test.go` (`TestVideoProof`), `competitions/video_test.go` | Video proof: the organiser's choice, which skills need video, links checked against known hosts, never embedded, missing, provided, "need more" and OK, and changing what's asked |
| `web/coachpages_test.go` | Coach sign-off end to end: coaches added and removed, members choosing a coach, who sees whom (and see-all), signing off, the organiser's flag and CSV, a change clearing it, and an individual's sign-off link |
| `competitions/timetable_test.go`, `competitions/schedule_test.go`, `web/timetablepages_test.go` | Flights (even splits, mixed flights for a split level, the soft club spread) and the scheduler: areas and days, a person never in two places, rest, the end time, blocks within a day's hours, must and prefer rules, the setup check, the report and the fixes it suggests (an entry cap among them), names that look alike, running orders bent
for short rest (early in the first flight, late in the next), and edits (moves, redraws, retiming, problems); the timetable pages end to end: setup, planning, moving, late entries, individuals matched by name across disciplines, publishing to gymnasts and clubs, and the sheets |
| `web/partnerpages_test.go` | Events end to end: synchro, tumbling and DMT on the dashboard and timetable, individuals choosing a discipline, partner links confirmed by an entry or a member link, a member with three entries, withdrawing one, and the comp sec editing a synchro entry; store tests cover entries per discipline and migration 6 keeping existing entries |
| `web/mypages_test.go` | My competition: linked from a member's page and an individual's entry, nothing sent yet, problems listed, not yet published, then flight, place in the order, routine times, the panel and the person picked out in the timetable |
| `competitions/rota_test.go`, `web/rotapages_test.go` | The officials rota: panels filled, no one in two places or judging while competing, only who may take each role, seats left short and reported, rules about people (must role, off, hours; broken by hand), own-club judging shared, flights placed where their judges are free, blocked time staffed; on the pages: planning fills panels, a rule and assigning again, setting a seat, the sheets and rota, and duties shown once published |
| `competitions/level_order_test.go`, `web/comppages_test.go` (`TestLevelOrderPages`), `store/store_test.go` (`TestLevelOrderMigration`) | The order of levels: built-ins easiest first (BUCS turned round), levels kept in place and new ones placed by rank, judging "up to" and "below" by it, moving a level on the dashboard, and older competitions put in order when read until one is moved |
| `competitions/simulate_test.go`, `web/simulatepages_test.go` | Simulation: stand-in entries, pairs, gymnasts in several disciplines (never twice in one), men and women, judges shared across disciplines and competing ones among the gymnasts; judges each club must bring per competitors in a discipline (rounding up, chairs among them, a club bringing the sum of its quotas, a percent of them also judging other disciplines, the form's judges then the organiser's own, competing judges from their own club); the plan's result (fits, seats, what doesn't fit and what would), the same seed planning the same, numbers refused; on the pages: filled in from the competition, a scenario run and shown, the real timetable untouched, changing one, out of date after the setup changes until run again, removing, and only the last 8 kept |
| `competitions/synchro_test.go`, `web/synchropages_test.go` | Paired synchro events: pairing levels by name (refused for levels not ticked or paired twice), the event's name ("BUCS L1/L2"), a pair choosing which level they do and being checked against it; a pair's level from their individual ones (the same, or the easier a level apart); on the pages: the pairing kept and shown, the entry form's choices, and a pair at the wrong level flagged on the dashboard until changed |
| `competitions/delay_test.go`, `web/delaypages_test.go` | A delay: a flight under way running late, slack absorbing some of it, later flights shifting round blocked time (and saying they waited), a flight past the day's end or clashing moving to another area's free slot (never into the delay), what can't fit and clashes reported; easing it (breaks moved, shortened or worked through on that area alone, fewer minutes between, quicker turns, flights merged, running over); the page: nothing before planning, the result, the timetable unchanged, and bad input refused |
| `competitions/leave_test.go`, `web/leavepages_test.go` | An official leaving: their seats from then (for the day or for good), filled by someone free or by moving others round (up the panel, or across from a panel at the same time), someone free before any move and the fewest moves after, the least busy first, someone competing not chosen, a seat no one can reach left empty; the page: nothing before planning, the result, bad input refused |
| `web/timelinepages_test.go` | The panel timelines: nothing before planning; a grid row a minute, days side by side with each day's other hours shaded; with officials, a sheet for each day's area over its own hours, a column for each seat and a name to a cell (an empty seat as "—"), blocked time across the area; flights moved onto each other flagged; and the CSV in time order with officials' clubs |
| `web/officialpages_test.go`, `competitions/officials_test.go` | Officials: Code of Points panels by default and changed, offers from members (corrected by the comp sec, sent with the club), individuals and the organiser, qualified marks, who may judge what, and the cover per discipline |
| `web/clubpages_test.go` | The club pages end to end: creating a club, entering it through a competition's club link, a member joining and entering, sending new and changed entries, "changed since sent", changing an entry on a member's behalf, a member's new link, withdrawing and sending all again, removing, deleting, and the rate limit |
| `web/demo_test.go`, `demo/routines_test.go` | The [demo competition](../operations/deploy.md#demo-competition) seeding a fresh store through the pages without a hitch: by default a small demo (`demo.Small`, three clubs, under two seconds), checking the clubs entered, every entry not waiting in a flight and most rota seats filled; `go test ./web -run TestDemoSeed -full` seeds the whole one (`demo.Full`, about 15 seconds) and also checks the timetable fits with HD judges seated. And its generated voluntaries passing each BUCS level |
| `web/endtoend_test.go` | The dry run's path in one test, stage by stage on a fixed clock: two clubs and an individual enter, coaches sign off, clubs send; plan and publish; the day (a late start, Next gymnast, a scratch, the finish); a desk message, a late change accepted and a payment. Each stage reads the venue screen, a member's My competition and the organiser's On the day |
| `web/daypages_test.go`, `web/screen_test.go`, `web/upnow_test.go`, `web/daygaps_test.go`, `web/latepages_test.go` | The day on a fixed clock in 2030: before the day, morning, mid-flight and evening; planned Now, on time, running late and early; `?area=`; late notes; taps (tumbling passes, everyone scratched); payments, desk refusals, notifying a judge's club, changes told at once on the day; late changes after planning and what the organiser is told |
| `web/setform_test.go`, `web/levelform_test.go` | Reading the requirements and level editors' forms back into JSON; the requirements page listing levels |

# Testing a migration

A migration test must keep passing when later migrations are added. Two
shapes do:

- **Run the one migration directly.** Set up the data it changes, then run
  just that migration's SQL (`migrations[n-1]`) and its Go step
  (`migrationSteps[n]`), if any, on the open store, and check the result.
  Nothing else runs.
- **Start from an older schema and open the store.** Set
  `migrations = all[:n-1]` (restoring `all` with `t.Cleanup`), open a store
  in a fresh directory, add rows in their old shape, then set
  `migrations = all` and open it again. Every migration from `n` on runs
  once, as it would on the server, so a later one just runs too.

Never wind a database back by deleting rows from its `migrations` table and
reopening it. That re-runs every migration from there on, not only the one
under test, and the first later migration that can't run twice (a
`CREATE TABLE`, an `ALTER TABLE … ADD COLUMN`) breaks the test. Never edit a
migration once it has shipped.

# The test kit

`web/testkit_test.go` is shared by the web tests:

- **A fixed clock.** `newTestServer(t, atTime(now))`, or `competitionServerAt(t, now)`, runs the pages, the notifier and the store on a `testClock` the test moves with `clock.set`, so anything that depends on the day (On the day, the venue screen, who's up, late notes, notifications) runs the same at any hour. The store reads it through `Store.SetClock`. `onTheDay(h, m)` and `dayBefore` (in `web/daypages_test.go`) are times on and before `newCompetition`'s date, 16 March 2030. `withMail()` adds a fake mailer. Tests that don't care about time use `competitionServer(t)`, on the real clock.
- **Sending waits for itself.** Emails and pushes go through a background sender (`web/sender.go`), so the test server's handler flushes it after each request, and `tellDue(p)` runs the notifier and waits the same way: a test reads `mail.sent` once the request has answered, never by sleeping. To see the page answer before the sending is done, use `routesWithPages(s.p)` directly. The fake mailer and push service take a lock, as the sender's workers call them at once.
- **The usual things to make.** `bucsEntry(name)` is the BUCS L3 entry form (an individual's with a name, a member's without); `aClub(t, h, admin, name, entered, members...)` is a club entered in a competition, its members joined, or entered and sent.
- **Named readers, not words on the page.** `screenCards`, `dayPageAreas`, `myCompetitionOf`, `feeAccounts`, `invoiceOf`, `deskPeople`, `historyHas` and `rowOf` parse a page (golang.org/x/net/html) into the cards, areas, flights and rows a test checks, so a check fails when the feature breaks rather than passing on a word found elsewhere. `unescape` undoes HTML escaping.

The Docker build runs `go test ./...` too, so an image can't be built from
failing code.

# Not covered

Apart from the tariff sheet loading (`web/browser_test.go`), the browser JavaScript
has no automated tests. UI changes were checked by hand in a desktop browser,
and some phone behaviour is still untested
([open questions](../open-questions.md)). More pages can be added to
`web/browser_test.go` the same way.
