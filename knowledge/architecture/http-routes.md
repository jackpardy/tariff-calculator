---
type: API Reference
title: HTTP routes
description: Every route the server exposes, what it takes and what it returns, along with the limits applied to all requests.
resource: https://github.com/jackpardy/tariff-calculator/blob/master/main.go
tags: [architecture, http, routes]
generated: { by: claude-code/cli, at: 2026-10-04T17:45:00Z }
---

# Routes

All routes are defined in `routes()` in `main.go`. Forms are
`application/x-www-form-urlencoded`. "Routine" means a JSON array of
[skills](../domain/skill-model.md#schema).

| Method and path | Input | Returns |
|---|---|---|
| `GET /` | — | The calculator page ([routine builder](../features/routine-builder.md)) |
| `POST /skill-form` | optional `skill` (JSON) + `editIndex` | The "Add a skill" panel, for the default skill or the one being edited |
| `POST /skill-inputs` | the form's fields, or `load=common` + `commonSkillKey` (+ `shape`), or `load=skill` + `skill` | The re-rendered editor; 204 while the form holds something unscorable |
| `GET /skill-search` | `q` | Search results ([skill catalog](../domain/skill-catalog.md)) |
| `POST /calculate-skill` | the form's fields | The skill as JSON, named and priced, for the page to store |
| `POST /routine` | `routineData`, `requirementSet`, `checks`, `side` (`a`/`b`), optional `compareData` + `compareName`; for a [level](../requirements/levels.md)'s exercise, `level`, `exercise` and the other exercise's `pairData`, `pairSet`, `pairChecks`, `pairName` | The routine view: cards, flags, totals, requirement results, and the level panel |
| `POST /set-routine` | `requirementSet`, optional `routineData` | JSON `{name, skills, matches}` ([set routines](../requirements/set-routines.md)) |
| `POST /qr` | `text` | An SVG QR code ([sharing](../features/sharing.md)) |
| `GET /compare`, `POST /compare` | `aData`, `aName`, `bData`, `bName` | The [compare page](../features/compare.md) and its comparison |
| `GET /requirements` | — | The requirements page |
| `POST /requirements/editor` | `set` (JSON), or the editor's form fields + `action`/`add` | The re-rendered editor with problems listed |
| `POST /requirements/level-editor` | `level` (JSON) and `custom`, or the editor's own fields and `action` | The [level](../requirements/levels.md) editor |
| `GET /tariff-sheet`, `POST /tariff-sheet` | as `/routine` | The [tariff sheet](../features/tariff-sheet.md) page and card |
| `GET /view`, `POST /view` | as `/routine`, plus `routineName`, `optionRef`, `pairOptionRef`, `tabNames`, `optionSets` | The [view screen](../features/view.md) page, and a routine or level pair to fill it |
| `GET /static/...` | — | [Static assets](static-assets.md) |

## Competition pages

[Competition entries](../features/competition-entries.md) (`comppages.go`,
`clubpages.go`), reached at `/competitions`; the calculator doesn't link to
them, for now. `{token}` is a secret link (ADR 0004). These
pages send `Referrer-Policy: no-referrer`, `Cache-Control: no-store` and
`X-Robots-Tag: noindex`. A wrong or replaced link gets 404, and without storage
every page answers 503.

| Method and path | Input | Returns |
|---|---|---|
| `GET /competitions/new` | — | The form that creates a competition, and the competitions saved in this browser |
| `POST /competitions` | `name`, `date`, `deadlineDate`, `deadlineTime` (Irish and UK time), `individuals`, each built-in `level`, each of the browser's own levels as `custom` (`{level, sets}`) | 303 to the dashboard with `?new=created`; 422 with the problems; 429 after 5 an hour from one address |
| `GET /competitions/admin/{token}` | optional `club` (a club's name or `individual`), `problems=1`, `new` | The organiser's dashboard |
| `GET /competitions/admin/{token}/entries/{id}` | — | One entry, checked |
| `POST /competitions/admin/{token}/entries/{id}/check` | `checked` (`1`/`0`), `note` | Marks the entry checked or not, with a note the club or gymnast sees |
| `GET /competitions/admin/{token}/cards` | optional `club`, `problems=1`, `unchecked=1`, `level`, `entry` | Printable competition cards (the [tariff sheet](../features/tariff-sheet.md), filled in), one exercise per page |
| `GET /competitions/admin/{token}/entries.csv` | — | Every entry as CSV: gymnast, club, level, each exercise and its difficulty, problems, checked, note, sent |
| `POST /competitions/admin/{token}/deadline` | `close=1`, or `deadlineDate` and `deadlineTime` | Closes entries now, or changes when they close (by the end of the competition date) |
| `POST /competitions/admin/{token}/video` | `video` (`""`, `skills`, `routine`), `videoTriples`, `videoDoubles`, `videoTariff` | Changes what video proof the competition asks for (also taken by `POST /competitions`) |
| `POST /competitions/admin/{token}/entries/{id}/video` | `review` (`ok`, `more`, `""`), `note` | The organiser's review of an entry's videos |
| `POST /competitions/admin/{token}/individuals` | `on` (`1`/`0`) | Turns individual entry on or off |
| `POST /competitions/admin/{token}/replace-link` | — | 303 to the new admin link, `?new=replaced` |
| `POST /competitions/admin/{token}/delete` | `confirm=1` | Deletes the competition and its entries |
| `GET`, `POST /competitions/enter/{token}` | `gymnast`, `level`, `ex1Option`, `ex1Skills`, `ex2Option`, `ex2Skills`, and where video is asked for `ex1Video`, `ex1VideoNote`, `ex2Video`, `ex2VideoNote` | The individual entry form; entering redirects to the personal link |
| `GET`, `POST /competitions/entry/{token}` | as entering | An individual's own entry, and changing it until the deadline |
| `POST /competitions/entry/{token}/withdraw` | `confirm=1` | Withdraws and deletes the entry |
| `GET /competitions` | — | The competitions, clubs and entries this browser has links to |
| `GET`, `POST /competitions/club/{token}` | `clubAdmin` (a club's admin link, or its token) | A competition's club link: a comp sec enters their club |
| `GET /clubs/new`, `POST /clubs` | `name` | Creates a club: 303 to its page with `?new=created`; 429 after 5 an hour from one address |
| `GET /clubs/admin/{token}` | optional `new`, `notice` | The comp sec's page: join link, each competition's entries, members |
| `POST /clubs/admin/{token}/competitions/{id}/send` | `which` (`changed` or `all`) | Sends new and changed entries, or all again (taking back withdrawn ones) |
| `GET`, `POST /clubs/admin/{token}/members/{member}/competitions/{id}` | as entering, without `gymnast` | Changes a member's entry on their behalf |
| `POST /clubs/admin/{token}/members/{member}/new-link` | — | The page with the member's new link, shown once |
| `POST /clubs/admin/{token}/members/{member}/remove` | — | Removes the member and their entries |
| `POST /clubs/admin/{token}/replace-link`, `/delete` | `confirm=1` to delete | As for a competition |
| `GET`, `POST /clubs/join/{token}` | `name` | Joining a club: 303 to the member's page with `?new=joined` |
| `GET /clubs/member/{token}` | — | A member's page: their entry for each competition the club is in |
| `POST /clubs/member/{token}/competitions/{id}` | as entering, without `gymnast` | Saves the member's entry (their club sends it) |
| `POST /clubs/member/{token}/competitions/{id}/withdraw` | `confirm=1` | Withdraws it |

`requirementSet` is either `builtin:<id>` or the custom requirements as JSON.
If the requirements can't be used, the problem is reported in the rendered
check rather than failing the request.

# Limits and hardening

- Request bodies are capped at **64 KB** (`http.MaxBytesReader`), as are headers.
  A full routine is a few kilobytes.
- Timeouts: read header 5 s, read 10 s, write 10 s, idle 60 s.
- Every incoming skill is normalised to its phase count and validated (rotation
  0–16, non-negative twists, known shape and positions). Errors are
  `400 Bad Request: <message>`, which the page shows to the user.
- Official names are always re-derived and never taken from input.
- Every response carries `X-App-Version` ([static assets](static-assets.md#update-notice)).
- `PORT` sets the port (default 8080). `DATA_DIR` turns on competition
  storage there (SQLite); unset, it's off. With storage on, competitions 120
  days past their date and clubs unused for 120 days are deleted at start-up
  and every 6 hours.
