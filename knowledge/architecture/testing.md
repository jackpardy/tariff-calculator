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
| `competitions/timetable_test.go`, `timetablepages_test.go` | The planner (even flights, categories kept on one panel, balanced panels, mixed flights for a split level, panels needed, edits, the soft club spread) and the timetable pages end to end: men and women, planning, times only, moving, late entries, publishing to gymnasts and clubs, and the sheets |
| `clubpages_test.go` | The club pages end to end: creating a club, entering it through a competition's club link, a member joining and entering, sending new and changed entries, "changed since sent", changing an entry on a member's behalf, a member's new link, withdrawing and sending all again, removing, deleting, and the rate limit |
| `setform_test.go`, `levelform_test.go` | Reading the requirements and level editors' forms back into JSON; the requirements page listing levels |

The Docker build runs `go test ./...` too, so an image can't be built from
failing code.

# Not covered

Apart from the tariff sheet loading (`browser_test.go`), the browser JavaScript
has no automated tests. UI changes were checked by hand in a desktop browser,
and some phone behaviour is still untested
([open questions](../open-questions.md)). More pages can be added to
`browser_test.go` the same way.
