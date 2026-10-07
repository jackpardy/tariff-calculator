---
type: Practice
title: Testing and checks
description: What the Go tests cover (the 139 CoP examples, the engine, requirements, catalog and rendered HTML) and the checks to run before every commit.
resource: https://github.com/jackpardy/tariff-calculator/blob/master/main_test.go
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
| `store/store_test.go` | Migrations, every kind of link (hashed, replaced, wrong), individual and club entries, sending and re-sending, deadlines, limits and deletion after 120 days, each on a fresh SQLite file |
| `catalog/*_test.go` | Picker categories, shape options, search by name, alias and notation |
| `static/static_test.go` | Hashed URLs and caching headers |
| `main_test.go` | Handlers end to end, asserting on rendered HTML: flags, custom names, checks, sheet, compare, requirements, side by side, QR, sharing, oversize bodies; every template's `js:` `hx-vals` being an object literal (htmx wraps anything else in braces, which broke the sheet) |
| `browser_test.go` | Pages loaded in headless Chrome (chromedp), so their JavaScript runs: the tariff sheet loads the routine saved in the browser and shows its elements and total. It needs Chrome or Chromium (CI's Ubuntu runners have it; `CHROME_PATH` points at one) and skips without, as in the Docker build |
| `comppages_test.go` | The competition pages on a fresh store: storage off, creating (problems, the admin link, replacing it, deleting), the rate limit, and individual entry from form to dashboard, change and withdrawal |
| `comppages_test.go` (`TestOrganiserTools`) | Printing cards (filled in, filtered), the CSV (with formulas made harmless), marking checked with a note the gymnast sees, a change unchecking it, and closing and reopening entries |
| `comppages_test.go` (`TestVideoProof`), `competitions/video_test.go` | Video proof: the organiser's choice, which skills need video, links checked against known hosts, never embedded, missing, provided, "need more" and OK, and changing what's asked |
| `coachpages_test.go` | Coach sign-off end to end: coaches added and removed, members choosing a coach, who sees whom (and see-all), signing off, the organiser's flag and CSV, a change clearing it, and an individual's sign-off link |
| `competitions/timetable_test.go`, `competitions/schedule_test.go`, `timetablepages_test.go` | Flights (even splits, mixed flights for a split level, the soft club spread) and the scheduler: areas and days, a person never in two places, rest, the end time, blocks within a day's hours, must and prefer rules, the setup check, the report and the fixes it suggests (an entry cap among them), names that look alike, running orders bent
for short rest (early in the first flight, late in the next), and edits (moves, redraws, retiming, problems); the timetable pages end to end: setup, planning, moving, late entries, individuals matched by name across disciplines, publishing to gymnasts and clubs, and the sheets |
| `partnerpages_test.go` | Events end to end: synchro, tumbling and DMT on the dashboard and timetable, individuals choosing a discipline, partner links confirmed by an entry or a member link, a member with three entries, withdrawing one, and the comp sec editing a synchro entry; store tests cover entries per discipline and migration 6 keeping existing entries |
| `mypages_test.go` | My competition: linked from a member's page and an individual's entry, nothing sent yet, problems listed, not yet published, then flight, place in the order, routine times, the panel and the person picked out in the timetable |
| `competitions/rota_test.go`, `rotapages_test.go` | The officials rota: panels filled, no one in two places or judging while competing, only who may take each role, seats left short and reported, rules about people (must role, off, hours; broken by hand), own-club judging shared, coach clashes, flights placed where their judges are free, blocked time staffed; on the pages: planning fills panels, a rule and assigning again, setting a seat, the sheets and rota, and duties shown once published |
| `competitions/level_order_test.go`, `comppages_test.go` (`TestLevelOrderPages`), `store/store_test.go` (`TestLevelOrderMigration`) | The order of levels: built-ins easiest first (BUCS turned round), levels kept in place and new ones placed by rank, judging "up to" and "below" by it, moving a level on the dashboard, and older competitions put in order when read until one is moved |
| `competitions/simulate_test.go`, `simulatepages_test.go` | Simulation: stand-in entries, pairs, gymnasts in several disciplines (never twice in one), men and women, judges shared across disciplines and competing ones among the gymnasts; judges each club must bring per competitors in a discipline (rounding up, chairs among them, a club bringing the sum of its quotas, a percent of them also judging other disciplines, the form's judges then the organiser's own, competing judges from their own club); the plan's result (fits, seats, what doesn't fit and what would), the same seed planning the same, numbers refused; on the pages: filled in from the competition, a scenario run and shown, the real timetable untouched, changing one, out of date after the setup changes until run again, removing, and only the last 8 kept |
| `timelinepages_test.go` | The panel timeline: nothing before planning, flights and blocks with their officials by role (an empty seat as "—"), lunch on each area, rows that line up (every column filled once per row), flights moved onto each other sharing a cell, and the CSV in time order with officials' clubs |
| `officialpages_test.go`, `competitions/officials_test.go` | Officials: Code of Points panels by default and changed, offers from members (corrected by the comp sec, sent with the club), individuals and the organiser, qualified marks, who may judge what, and the cover per discipline |
| `clubpages_test.go` | The club pages end to end: creating a club, entering it through a competition's club link, a member joining and entering, sending new and changed entries, "changed since sent", changing an entry on a member's behalf, a member's new link, withdrawing and sending all again, removing, deleting, and the rate limit |
| `demo_test.go`, `demo/routines_test.go` | The [demo competition](../operations/deploy.md#demo-competition) seeding a fresh store through the pages without a hitch, its timetable fitting with HD judges seated; and its generated voluntaries passing each BUCS level |
| `setform_test.go`, `levelform_test.go` | Reading the requirements and level editors' forms back into JSON; the requirements page listing levels |

The Docker build runs `go test ./...` too, so an image can't be built from
failing code.

# Not covered

Apart from the tariff sheet loading (`browser_test.go`), the browser JavaScript
has no automated tests. UI changes were checked by hand in a desktop browser,
and some phone behaviour is still untested
([open questions](../open-questions.md)). More pages can be added to
`browser_test.go` the same way.
