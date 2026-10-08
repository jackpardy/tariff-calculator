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
| `GET /judging` | the form's fields (query), optional `last=1` | The "How it's judged" pop-up ([execution judging](../domain/execution.md)); `last=1` adds the landing; 400 for a skill that can't land |
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
| `POST /competitions` | `name`, `date`, `deadlineDate`, `deadlineTime` (Irish and UK time), `individuals`, `opens` (`later`, the default, for private; `now`; or `at` with `liveDate` and `liveTime`), each built-in `level`, each of the browser's own levels as `custom` (`{level, sets}`) | 303 to the dashboard with `?new=created`; 422 with the problems; 429 after 5 an hour from one address |
| `GET /competitions/admin/{token}` | optional `club` (a club's name or `individual`), `problems=1`, `new` | The organiser's dashboard |
| `GET /competitions/admin/{token}/entries/{id}` | — | One entry, checked |
| `POST /competitions/admin/{token}/entries/{id}/check` | `checked` (`1`/`0`), `note` | Marks the entry checked or not, with a note the club or gymnast sees |
| `GET /competitions/admin/{token}/cards` | optional `club`, `problems=1`, `unchecked=1`, `level`, `entry` | Printable competition cards (the [tariff sheet](../features/tariff-sheet.md), filled in), one exercise per page |
| `GET /competitions/admin/{token}/entries.csv` | — | Every entry as CSV: gymnast, club, level, each exercise and its difficulty, problems, checked, note, sent |
| `POST /competitions/admin/{token}/deadline` | `close=1`, or `deadlineDate` and `deadlineTime` | Closes entries now, or changes when they close (by the end of the competition date) |
| `GET`, `POST /clubs/member/{token}/competitions/{id}/notify`, `/competitions/entry/{token}/notify`, `/clubs/admin/{token}/competitions/{id}/notify`, `/clubs/coach/{token}/competitions/{id}/notify` | `email`, `adult=1`, each `topic` (`timetable`, `duties`, `cards`), `action` (`save`, or `remove` with `remove`, the subscription) | Hearing about a competition's changes (ADR 0008): a member, an individual, the comp sec, a coach. Push: `action=push`, `endpoint`, `p256dh`, `auth`, each `topic` (from `static/js/push.js`) |
| `GET /clubs/member/{token}/competitions/{id}/score-sheets`, `/competitions/entry/{token}/score-sheets` | — | The published score sheets of the flights the person officiates |
| `GET /sw.js` | — | The service worker, for push notifications |
| `GET /manifest.webmanifest` | — | The web manifest, for adding a page to the home screen (no `start_url`) |
| `GET`, `POST /notify/confirm/{token}` | — | Confirms an email (a button, then done) |
| `GET`, `POST /notify/off/{token}` | — (or `List-Unsubscribe=One-Click`) | Stops an email |
| `POST /competitions/admin/{token}/notify-now` | — | Sends the changes waiting at once |
| `POST /competitions/admin/{token}/no-wait` | `on` (`1`/`0`) | No wait before telling changes on the competition's days |
| `POST /competitions/admin/{token}/live` | `opens`: `now`, `at` (with `liveDate` and `liveTime`, before the deadline) or `private` | Goes live now or then, or makes the competition private, pausing entries and keeping those made |
| `POST /competitions/admin/{token}/video` | `video` (`""`, `skills`, `routine`), `videoTriples`, `videoDoubles`, `videoTariff` | Changes what video proof the competition asks for (also taken by `POST /competitions`) |
| `POST /competitions/admin/{token}/entries/{id}/video` | `review` (`ok`, `more`, `""`), `note` | The organiser's review of an entry's videos |
| `POST /competitions/admin/{token}/individuals` | `on` (`1`/`0`) | Turns individual entry on or off |
| `POST /competitions/admin/{token}/replace-link` | — | 303 to the new admin link, `?new=replaced` |
| `POST /competitions/admin/{token}/links` | `name`, `kind` (`cards`, `chair`, `timetable`, `everything`) | Makes a helper's link (ADR 0009) and shows it once |
| `POST /competitions/admin/{token}/links/{id}/remove` | — | Stops a helper's link working |
| `GET /competitions/admin/{token}/history` | — | Every change, when and by which link |
| `POST /competitions/admin/{token}/concerns` | `text`, optional `entry` | Flags a concern for the organiser (any admin link) |
| `POST /competitions/admin/{token}/concerns/{id}/resolve` | `note` | Marks a concern resolved |

Every `/competitions/admin/{token}` route takes the admin link or a helper's
link of a kind that may use it (`allowed`, ADR 0009); others get 403.
| `POST /competitions/admin/{token}/delete` | `confirm=1` | Deletes the competition and its entries |
| `GET`, `POST /competitions/enter/{token}` | `gymnast`, `level`, `ex1Option`, `ex1Skills`, `ex2Option`, `ex2Skills`, and where video is asked for `ex1Video`, `ex1VideoNote`, `ex2Video`, `ex2VideoNote` | The individual entry form; entering redirects to the personal link |
| `GET`, `POST /competitions/entry/{token}` | as entering | An individual's own entry, and changing it until the deadline |
| `POST /competitions/entry/{token}/withdraw` | `confirm=1` | Withdraws and deletes the entry |
| `GET /competitions/entry/{token}/day` | — | My competition, for someone entering on their own (all their entries, matched by name) |
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
| `GET /clubs/member/{token}/competitions/{id}/day` | — | My competition: their events, cards, flights and times, duties, and the published timetable |
| `POST /clubs/member/{token}/competitions/{id}` | as entering, without `gymnast` | Saves the member's entry (their club sends it) |
| `POST /clubs/member/{token}/competitions/{id}/withdraw` | `confirm=1` | Withdraws it |
| `POST /clubs/member/{token}/coach`, `POST /clubs/admin/{token}/members/{member}/coach` | `coach` (an id, `""` for none) | A member's coach, chosen by them or the comp sec |
| `POST /clubs/admin/{token}/coaches` | `name` | Adds a coach: the club page with their coach link, shown once |
| `POST /clubs/admin/{token}/coaches/{coach}/new-link`, `/remove` | — | A coach's new link (shown once), or removing them |
| `POST /clubs/admin/{token}/coaches-see-all` | `on` (`1`/`""`) | Whether every coach sees every member |
| `GET /clubs/coach/{token}` | — | A coach's page: each competition's entries they see, and their sign-off |
| `GET`, `POST /clubs/coach/{token}/members/{member}/competitions/{id}` | `signed` (`1`/`0`), `note` | One entry, checked, to sign off or say not yet |
| `POST /competitions/admin/{token}/events` | `synchroLevel`, `synchroPairs` (synchro levels to run as one event, one event a line, joined by `+`), `tumbling`, `dmt` (one level a line) | Changes the synchro, tumbling and DMT events (also on `POST /competitions`); entries then take `discipline` and, for synchro, `partnerName` and `partnerClub` |
| `POST /competitions/admin/{token}/levels/move` | `move` (`<discipline>:<place>:<-1 or 1>`) | Moves a level one place easier or harder in its discipline's order (easiest first), which judging "up to" a level and "only levels below their own" go by |
| `GET`, `POST /competitions/partner/{token}` | `link` (the partner's own member page or entry link, or blank) | A synchro partner confirming the pair |
| `GET /competitions/admin/{token}/officials` | `notice` | The officials: who can judge and chair each discipline, the panels and judging rule, and everyone offered or added |
| `POST /competitions/admin/{token}/officials/settings` | `judge` (`""`, `below`, `qualified`), `panel-<discipline>-<role>` | Panels per discipline and who may judge what |
| `POST /competitions/admin/{token}/officials/add` | `name`, `club`, and an offer | Adds a judge or helper |
| `POST /competitions/admin/{token}/officials/{id}/qualified`, `/remove` | `on` | Marks someone qualified; removes someone the organiser added |
| `POST /clubs/member/{token}/competitions/{id}/offer`, `GET`/`POST /clubs/admin/{token}/members/{member}/competitions/{id}/offer`, `POST /competitions/entry/{token}/offer` | `judge-<discipline>`, `upto-<discipline>`, `chair-<discipline>`, `recorder`, `marshal` | What a member, the comp sec for a member, or an individual offers to judge and help with |
| `POST /competitions/admin/{token}/split` | `split` (`""`, `all`, `some`), `splitLevel` | Which levels rank men and women separately (also `split=all` on `POST /competitions`); entries then take `category` |
| `GET /competitions/admin/{token}/timetable` | `notice` | The timetable: setup, the report and fixes, each day's areas, unplaced entries |
| `POST /competitions/admin/{token}/timetable/setup/days` | `day-{i}-name`, `-start`, `-end`, `-area`; `add`, `remove` | Saves, adds or removes days |
| `POST /competitions/admin/{token}/timetable/setup/areas` | `area-{i}-name`, `-discipline`; `add`, `remove` | Saves, adds or removes areas (renames carry to days, blocks and rules) |
| `POST /competitions/admin/{token}/timetable/setup/timings` | `per-`, `between-`, `max-` per discipline; `rest`, `restMust`, `separate`, `separateLevel` | Saves timings, rest and which levels fly men and women apart |
| `POST /competitions/admin/{token}/timetable/setup/blocks` | `name`, `minutes`, `day`, `at` or `from`/`to`, `area`, and officials needed: `chair`, `judges`, `recorder`, `marshal`; or `remove` | Adds or removes blocked time |
| `POST /competitions/admin/{token}/timetable/setup/rules` | `kind` (`area`, `day`, `before`, `apart`; about officials: `role`, `off`, `hours`), `must`, `event`, `event2`, `area`, `day`, `person`, `role`, `from`, `to`; or `remove` | Adds or removes a rule |
| `POST /competitions/admin/{token}/timetable/plan` | | Plans afresh from the entries and setup |
| `POST /competitions/admin/{token}/timetable/flight` | `flight`, `action` (`redraw`, `move` with `to` = `day:area`) | Redraws or moves a flight |
| `POST /competitions/admin/{token}/timetable/entry` | `entry`, `to` (flight index, `""` to take out) | Moves or places a gymnast |
| `POST /competitions/admin/{token}/timetable/publish` | `on` (`1`/`0`) | Shows clubs and gymnasts their flight, or hides it |
| `POST /competitions/admin/{token}/timetable/officials` | `action` (`rota`, or `seat` with `flight` or `block`, `seat`, `person`) | Assigns every panel again, or gives one seat to someone (`""` empties it) |
| `GET /competitions/admin/{token}/timetable/print` | `sheet` (`marshal`, `judges`, `scores`, `rota`, `timeline` or `timeline-officials`) | Printable sheets, one area to a page, with each flight's panel; the recorders' score sheets, a flight to a landscape page; each person's duties; the panel timeline, every day's areas side by side; or the timeline with a column for each seat, a day's area to a sheet |
| `GET /competitions/admin/{token}/timetable/timeline.csv` | — | The panel timeline as CSV: a row per flight or block on each area, with its officials by role |
| `GET /competitions/admin/{token}/timetable/delay` | `day` (index), `from` ("10:30"), `minutes`, `area` (any number; none for all); easing: `breaks` (`""`, `move`, `shorten`, `through`), `shorten`, `between`, `quicker` (%), `maxFlight`, `overrun` | What a delay does to the planned timetable, without saving: flights shifted or moved, what no longer fits, clashes |
| `GET /competitions/admin/{token}/timetable/leave` | `person` (key), `day`, `from`, `forgood` (`1`), `prefer` (`""` easiest to fill, `fewest`) | How a leaving official's seats would be filled, without saving |
| `GET /competitions/admin/{token}/timetable/simulate` | `from` (a scenario to start from), `notice` | The simulation: a scenario's numbers, filled in from the competition so far, and the scenarios side by side |
| `POST /competitions/admin/{token}/timetable/simulate` | `name`, `entries-<i>` (by event), `gymnasts`, `clubs`, `judges-<discipline>`, `chairs-<discipline>`, `quota-judges-<discipline>`, `quota-per-<discipline>`, `quota-chairs-<discipline>` (judges each club brings per competitors), `alsoJudge` (percent of the clubs' judges who also judge other disciplines), `judgePeople`, `competing`, `helpers`; or `again` (a scenario's index) | Plans stand-ins with the venue setup and keeps the scenario with its result (the last 8), or runs one again with the current setup |
| `POST /competitions/admin/{token}/timetable/simulate/delete` | `remove` (a scenario's index) | Removes a scenario |
| `POST /competitions/admin/{token}/signoff` | `on` (`1`/`0`) | Whether entries need a coach's sign-off (also `signoff=1` on `POST /competitions`) |
| `GET`, `POST /competitions/signoff/{token}` | `coach` (their name), `signed`, `note` | An individual's coach signing off their entry |

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
