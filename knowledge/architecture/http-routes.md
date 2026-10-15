---
type: API Reference
title: HTTP routes
description: Every route the server exposes, what it takes and what it returns, along with the limits applied to all requests.
resource: https://github.com/jackpardy/tariff-calculator/blob/master/web/calculator.go
tags: [architecture, http, routes]
generated: { by: claude-code/cli, at: 2026-10-04T17:45:00Z }
---

# Routes

All routes are defined in `routesWithPages()` in `web/calculator.go`. Forms are
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

[Competition entries](../features/competition-entries.md) (`web/comppages.go`
and the files beside it, `web/clubpages.go`), reached at `/competitions`; the calculator doesn't link to
them, for now. `{token}` is a secret link (ADR 0004). These
pages send `Referrer-Policy: no-referrer`, `Cache-Control: no-store` and
`X-Robots-Tag: noindex`. A wrong or replaced link gets 404, and without storage
every page answers 503.

**One link, several roles** (ADR 0010): wherever `{token}` is a member's,
coach's, club admin, individual entry or admin link, it may also be
`{personal}~{id}`: a personal link (a member's, coach's or individual's, or
an entry link) opening another of the person's roles by its id (the member,
coach, individual or entry; the club, for its comp sec's pages; the
competition or helper link, for an admin link). Every route takes it, and it
works only while the role is the person's. A GET on a personal link's page
gets tabs for the person's roles.

A form's POST answers 303 to the page to go back to, with what happened as
`?notice=` (first in the query, before any `#part`): `seeOther` in
`web/helpers.go`, which `back` (the timetable) and `backToDay` don't use
yet.

The pages for someone at a competition (My competition, "I'm here", score
sheets, timelines, calendars, notifications and late changes) sit under a
member's, an individual's entry, a comp sec's or a coach's link. One
resolver, `whose` in `web/whose.go`, works out from the link who is asking
and which competition, and each feature has one handler for all of them.
The links on a person's or club's page for a competition (timetables and
calendars, Tell me about changes, duties and score sheets, the desk's
messages) are built in one place too, `competitionLinks` in the same file,
as a `views.MyLinks` each page embeds.
A person's or club's timetable and calendar pick out the same flights by
one rule (`web/ourflights.go`): `inSlot` for a person (the entries they're
in, a partner's too, and their seats), and `clubPeople` with `ourClub.inSlot`
for a club (its entries, and seats held by its members or by the
organiser's officials from it).

Pages that read a competition's people (calendars, timelines, score sheets,
My competition, the desk, late changes, delays, leaving, simulation, the
rota) get them through one helper, `liveRota` in `web/rotapages.go`: the live
entries, the officials as the rota needs them, who's the same person, each
entry's people and their names, keyed by person or as one person each.

| Method and path | Input | Returns |
|---|---|---|
| `GET /competitions/new` | — | The form that creates a competition, and the competitions saved in this browser |
| `POST /competitions` | `name`, `date`, `deadlineDate`, `deadlineTime` (Irish and UK time), `individuals`, `opens` (`later`, the default, for private; `now`; or `at` with `liveDate` and `liveTime`), each built-in `level`, each of the browser's own levels as `custom` (`{level, sets}`) | 303 to the dashboard with `?new=created`; 422 with the problems; 429 after 5 an hour from one address |
| `GET /competitions/admin/{token}` | optional `club` (a club's name or `individual`), `problems=1`, `new` | The organiser's dashboard |
| `GET /competitions/admin/{token}/settings` | optional `notice`, `new=replaced` | The Settings page: links and every setting, in groups (organiser and "everything" links only). The settings forms below come back to it |
| `GET /competitions/admin/{token}/entries/{id}` | — | One entry, checked |
| `POST /competitions/admin/{token}/entries/{id}/check` | `checked` (`1`/`0`), `note` | Marks the entry checked or not, with a note the club or gymnast sees |
| `GET /competitions/admin/{token}/cards` | optional `club`, `problems=1`, `unchecked=1`, `level`, `entry` | Printable competition cards (the [tariff sheet](../features/tariff-sheet.md), filled in), one exercise per page |
| `GET /competitions/admin/{token}/entries.csv` | — | Every entry as CSV: gymnast, club, level, each exercise and its difficulty, problems, checked, note, sent |
| `POST /competitions/admin/{token}/deadline` | `close=1`, or `deadlineDate` and `deadlineTime` | Closes entries now, or changes when they close (by the end of the competition date) |
| `GET`, `POST /clubs/member/{token}/competitions/{id}/notify`, `/competitions/entry/{token}/notify`, `/clubs/admin/{token}/competitions/{id}/notify`, `/clubs/coach/{token}/competitions/{id}/notify` | `email`, `adult=1`, each `topic` (`timetable`, `duties`, `cards`), `action` (`save`, or `remove` with `remove`, the subscription) | Hearing about a competition's changes (ADR 0008): a member, an individual, the comp sec, a coach. Push: `action=push`, `endpoint`, `p256dh`, `auth`, each `topic` (from `static/js/push.js`) |
| `GET /clubs/member/{token}/competitions/{id}/score-sheets`, `/competitions/entry/{token}/score-sheets` | — | The published score sheets of the flights the person officiates |
| `GET /clubs/member/{token}/competitions/{id}/calendar.ics`, `/competitions/entry/{token}/calendar.ics` | — | A person's published flights and duties as an iCalendar file to add or subscribe to; empty until published |
| `GET /clubs/member/{token}/competitions/{id}/club-calendar.ics`, `/clubs/admin/{token}/competitions/{id}/calendar.ics`, `/clubs/coach/{token}/competitions/{id}/calendar.ics` | — | The club's: every flight one of its gymnasts competes in, and every seat one of its people holds, named |
| `GET /sw.js` | — | The service worker, for push notifications and offline copies of pages (dropped after 30 days, or on a 404 or 410) |
| `GET /manifest.webmanifest` | — | The web manifest, for adding a page to the home screen (no `start_url`) |
| `GET`, `POST /notify/confirm/{token}` | — | Confirms an email (a button, then done) |
| `GET`, `POST /notify/off/{token}` | — (or `List-Unsubscribe=One-Click`) | Stops every email to that address for the competition |
| `POST /competitions/admin/{token}/notify-now` | — | Sends the changes waiting at once |
| `POST /competitions/admin/{token}/no-wait` | `on` (`1`/`0`) | No wait before telling changes on the competition's days |
| `POST /competitions/admin/{token}/live` | `opens`: `now`, `at` (with `liveDate` and `liveTime`, before the deadline) or `private`; `back=settings` to come back to Settings rather than the entries page | Goes live now or then, or makes the competition private, pausing entries and keeping those made |
| `POST /competitions/admin/{token}/video` | `video` (`""`, `skills`, `routine`), `videoTriples`, `videoDoubles`, `videoTariff` | Changes what video proof the competition asks for (also taken by `POST /competitions`) |
| `POST /competitions/admin/{token}/entries/{id}/video` | `review` (`ok`, `more`, `""`), `note` | The organiser's review of an entry's videos |
| `POST /competitions/admin/{token}/individuals` | `on` (`1`/`0`) | Turns individual entry on or off |
| `POST /competitions/admin/{token}/replace-link` | — | 303 to the new admin link's Settings page, `?new=replaced` |
| `GET`, `POST /clubs/member/{token}/competitions/{id}/late`, `/clubs/admin/{token}/members/{member}/competitions/{id}/late`, `/competitions/entry/{token}/late` | `discipline` (query), the entry form, `note` | Asking for a late change, once entries close |
| `GET /competitions/admin/{token}/late` | — | Late changes: what's allowed and each request, with whether it fits |
| `POST /competitions/admin/{token}/late/settings` | `late-<kind>=1`, `fee-<kind>` (`level`, `routines`) | Which late changes can be asked for, and their fees |
| `POST /competitions/admin/{token}/late/{id}/accept` | `note` | Applies a late change: the entry, the draft timetable, the fee |
| `POST /competitions/admin/{token}/late/{id}/reject` | `reason` | Turns a late change down |
| `POST /competitions/admin/{token}/later-deadlines` | `changesDate`, `changesTime`, `signoffsDate`, `signoffsTime` (empty: with the deadline) | When changes to entries, and coaches' sign-offs, close |
| `POST /competitions/admin/{token}/limits` | `limit-<n>` for each event, in the competition's order (empty: none) | Sets each event's limit (waiting lists) |
| `POST /competitions/admin/{token}/entries/{id}/let-in` | `on` (`1`/`0`) | Lets a waiting entry in over the limit, or back |
| `GET /competitions/admin/{token}/fees` | — | Entry fees: what's charged, and each club's and individual's account |
| `POST /competitions/admin/{token}/fees/settings` | `currency` (`GBP` or not), `fee-<discipline>`, `club`, `instructions` | Sets the fees |
| `POST /competitions/admin/{token}/fees/payments` | `payer` (`club:<id>` or `entry:<id>`), `amount`, `note` | Records a payment |
| `POST /competitions/admin/{token}/fees/payments/{id}/remove` | — | Removes a payment |
| `GET /competitions/admin/{token}/fees/invoice` | `payer` | A payer's invoice, to print |
| `GET /clubs/admin/{token}/competitions/{id}/invoice`, `/competitions/entry/{token}/invoice` | — | The club's, or an individual's, invoice |
| `POST /competitions/admin/{token}/links` | `name`, `kind` (`cards`, `chair`, `timetable`, `everything`) | Makes a helper's link (ADR 0009) and shows it once |
| `POST /competitions/admin/{token}/links/{id}/remove` | — | Stops a helper's link working |
| `POST /competitions/admin/{token}/ties/remove` | `id` (the tie) | Stops someone's own link opening these pages through the admin link or a helper's (organiser only) |
| `GET /competitions/admin/{token}/history` | — | Every change, when and by which link |
| `POST /competitions/admin/{token}/concerns` | `text`, optional `entry` | Flags a concern for the organiser (any admin link) |
| `POST /competitions/admin/{token}/concerns/{id}/resolve` | `note` | Marks a concern resolved |

Every `/competitions/admin/{token}` route takes the admin link or a helper's
link of a kind that may use it (`allowed`, ADR 0009); others get 403.
| `POST /competitions/admin/{token}/delete` | `confirm=1` | Deletes the competition and its entries |
| `GET`, `POST /competitions/enter/{token}` | `gymnast`, `level`, `ex1Option`, `ex1Skills`, `ex2Option`, `ex2Skills`, and where video is asked for `ex1Video`, `ex1VideoNote`, `ex2Video`, `ex2VideoNote`; optional `as`, the link of the person's page (a member's, coach's or individual's, or an entry link) | The individual entry form; entering redirects to the entry, `/competitions/entry/{personal}~{entry id}?saved=1`, as that person or someone new; 422 for an `as` that isn't a page's link |
| `GET`, `POST /competitions/entry/{token}` | as entering | An individual's own entry, on their page, and changing it until the deadline; `{token}` an entry link from before or a personal link's way to the entry |
| `GET /competitions/gymnast/{token}` | optional `notice` | An individual's page: the competitions they entered. `{token}` their personal link or an entry link from before |
| `GET /competitions/gymnast/{token}/details` | optional `notice` | Their link and their other links |
| `GET /competitions/gymnast/{token}/competitions/{id}` | — | Their page for a competition: a row per discipline; 404 for one they haven't entered |
| `GET`, `POST /competitions/gymnast/{token}/competitions/{id}/enter` | as entering, without `as` | Enters another discipline of a competition they've entered, under the same link |
| `POST /clubs/member/{token}/links`, `/clubs/coach/{token}/links`, `/competitions/gymnast/{token}/links` | `link` | Adds another of the person's links to their page; back to their details (the coach's page) with a notice |
| `POST /clubs/member/{token}/links/remove`, `/clubs/coach/{token}/links/remove`, `/competitions/gymnast/{token}/links/remove` | `kind` (`member`, `coach`, `individual`, or a shared link's), `id` (the record, or the tie) | Takes a role off their page |
| `POST /competitions/entry/{token}/withdraw` | `confirm=1` | Withdraws and deletes the entry |
| `GET /competitions/entry/{token}/day` | `notice` | My competition, for someone entering on their own (all their entries, matched by name) |
| `POST /competitions/entry/{token}/here` | — | "I'm here for the rest": a scratched individual clears both scratch warnings for themselves, then goes back to My competition with a notice; only once the timetable is published |
| `GET /competitions` | — | The competitions, clubs and entries this browser has links to |
| `GET`, `POST /competitions/club/{token}` | `clubAdmin` (a club's admin link, or its token) | A competition's club link: a comp sec enters their club |
| `GET /clubs/new`, `POST /clubs` | `name` | Creates a club: 303 to its page with `?new=created`; 429 after 5 an hour from one address |
| `GET /clubs/admin/{token}` | optional `new`, `notice` | The comp sec's Competitions section (the default): each competition the club is in, with its members' entries |
| `GET /clubs/admin/{token}/members`, `/coaches`, `/settings` | optional `notice` | The comp sec's other sections: the join link, members and synchro entries made with them; the coaches; replacing the admin link and deleting the club. Each has the section menu |
| `POST /clubs/admin/{token}/competitions/{id}/send` | `which` (`changed` or `all`) | Sends new and changed entries, or all again (taking back withdrawn ones) |
| `GET`, `POST /clubs/admin/{token}/members/{member}/competitions/{id}` | as entering, without `gymnast` | Changes a member's entry on their behalf |
| `POST /clubs/admin/{token}/members/{member}/new-link` | — | The page with the member's new link, shown once |
| `POST /clubs/admin/{token}/members/{member}/remove` | — | Removes the member and their entries |
| `POST /clubs/admin/{token}/replace-link`, `/delete` | `confirm=1` to delete | As for a competition |
| `GET`, `POST /clubs/join/{token}` | `name` | Joining a club: 303 to the member's page with `?new=joined` |
| `GET /clubs/member/{token}` | optional `new`, `notice` | A member's Competitions tab (the default): a row for each competition the club is in, with its date and status |
| `GET /clubs/member/{token}/competitions/{id}` | optional `notice`, `discipline` (the row to open) | A member's page for one competition: facts, links, each discipline's entry as a row that opens to its card and forms, officiating, and the judging offer; 404 if the club isn't entered |
| `GET /clubs/member/{token}/details` | optional `notice` | A member's personal link, their coach and their other links |
| `GET /clubs/member/{token}/competitions/{id}/day` | `notice` | My competition: their events, cards, flights and times, duties, and the published timetable; a box to say "I'm here for the rest" if they're scratched from something |
| `POST /clubs/member/{token}/competitions/{id}/here` | — | "I'm here for the rest": a scratched member clears both scratch warnings for themselves, then goes back to My competition with a notice; only once the timetable is published |
| `POST /clubs/member/{token}/competitions/{id}` | as entering, without `gymnast` | Saves the member's entry (their club sends it) |
| `POST /clubs/member/{token}/competitions/{id}/withdraw` | `confirm=1` | Withdraws it |
| `POST /clubs/member/{token}/coach`, `POST /clubs/admin/{token}/members/{member}/coach` | `coach` (an id, `""` for none) | A member's coach, chosen by them or the comp sec |
| `POST /clubs/admin/{token}/coaches` | `name` | Adds a coach: the club page with their coach link, shown once |
| `POST /clubs/admin/{token}/coaches/from-member` | `member` | Makes a member a coach too; their member link opens the coach's page. Back to Coaches |
| `POST /clubs/admin/{token}/ties/remove` | `id` (the tie) | Stops someone's own link opening the comp sec's pages. Back to Settings |
| `POST /clubs/admin/{token}/coaches/{coach}/new-link`, `/remove` | — | A coach's new link (shown once), or removing them |
| `POST /clubs/admin/{token}/coaches-see-all` | `on` (`1`/`""`) | Whether every coach sees every member |
| `GET /clubs/coach/{token}` | — | A coach's page: each competition's entries they see, and their sign-off |
| `GET`, `POST /clubs/coach/{token}/members/{member}/competitions/{id}` | `signed` (`1`/`0`), `note` | One entry, checked, to sign off or say not yet |
| `POST /competitions/admin/{token}/events` | `synchroLevel`, `synchroPairs` (synchro levels to run as one event, one event a line, joined by `+`), `tumbling`, `dmt` (one level a line) | Changes the synchro, tumbling and DMT events (also on `POST /competitions`); entries then take `discipline` and, for synchro, `partnerName` and `partnerClub` |
| `POST /competitions/admin/{token}/levels/move` | `move` (`<discipline>:<place>:<-1 or 1>`) | Moves a level one place easier or harder in its discipline's order (easiest first), which judging "up to" a level and "only levels below their own" go by |
| `GET`, `POST /competitions/partner/{token}` | `link` (the partner's own member page or entry link, or blank) | A synchro partner confirming the pair |
| `GET /competitions/admin/{token}/people` | — | People: everyone at the competition once (gymnasts, partners, officials, coaches), with what they do. Any link that can see the entries page, not the venue screen link |
| `GET /competitions/admin/{token}/people/{key}` | `key`: a person key (`store/personkey.go`), path-escaped as one segment (`m:<id>`, `i:<id>` or `i:<name>`, `p:<entry>`, `o:<id>`, `c:<coach id>`, `c::<name>` for an individual's coach) | One person's read-only page for the organiser: entries, officiating, coaching, and once published their check-in and desk messages. 404 for a key nobody has |
| `GET /competitions/admin/{token}/officials` | `notice` | The officials: who can judge and chair each discipline, the panels and judging rule, and everyone offered or added |
| `POST /competitions/admin/{token}/officials/settings` | `judge` (`""`, `below`, `qualified`), `panel-<discipline>-<role>` | Panels per discipline and who may judge what |
| `POST /competitions/admin/{token}/officials/add` | `name`, `club`, and an offer | Adds a judge or helper |
| `POST /competitions/admin/{token}/officials/{id}/qualified`, `/remove` | `on` | Marks someone qualified; removes someone the organiser added |
| `POST /clubs/member/{token}/competitions/{id}/offer`, `GET`/`POST /clubs/admin/{token}/members/{member}/competitions/{id}/offer`, `POST /competitions/entry/{token}/offer` | `judge-<discipline>`, `upto-<discipline>`, `chair-<discipline>`, `recorder`, `marshal` | What a member, the comp sec for a member, or an individual offers to judge and help with |
| `POST /competitions/admin/{token}/split` | `split` (`""`, `all`, `some`), `splitLevel` | Which levels rank men and women separately (also `split=all` on `POST /competitions`); entries then take `category` |
| `GET /competitions/admin/{token}/timetable` | `notice` | The timetable: setup, the report and fixes, each day's areas, unplaced entries. Each form on it also posts `version`, the draft's version as the page showed it: the posts below that carry it are refused with "The timetable changed since you opened this page; nothing was changed." if the draft has changed since, and every change to the draft is read and saved in one transaction |
| `POST /competitions/admin/{token}/timetable/setup/days` | `day-{i}-name`, `-start`, `-end`, `-area`; `add`, `remove` | Saves, adds or removes days |
| `POST /competitions/admin/{token}/timetable/setup/areas` | `area-{i}-name`, `-discipline`; `add`, `remove` | Saves, adds or removes areas (renames carry to days, blocks and rules) |
| `POST /competitions/admin/{token}/timetable/setup/timings` | `per-`, `between-`, `max-` per discipline; `rest`, `restMust`, `separate`, `separateLevel` | Saves timings, rest and which levels fly men and women apart |
| `POST /competitions/admin/{token}/timetable/setup/blocks` | `name`, `minutes`, `day`, `at` or `from`/`to`, `area`, and officials needed: `chair`, `judges`, `recorder`, `marshal`; or `remove` | Adds or removes blocked time |
| `POST /competitions/admin/{token}/timetable/setup/rules` | `kind` (`area`, `day`, `before`, `apart`; about officials: `role`, `off`, `hours`), `must`, `event`, `event2`, `area`, `day`, `person`, `role`, `from`, `to`; or `remove` | Adds or removes a rule |
| `POST /competitions/admin/{token}/timetable/plan` | | Plans afresh from the entries and setup |
| `POST /competitions/admin/{token}/timetable/flight` | `flight` (index), `action` (`redraw`, `move` with `to` = `day:area`, `earlier`), `version` | Redraws or moves a flight; it keeps its ID |
| `POST /competitions/admin/{token}/timetable/entry` | `entry`, `to` (flight index, `""` to take out), `version` | Moves or places a gymnast |
| `POST /competitions/admin/{token}/timetable/publish` | `action` (`publish`, `unpublish`, `discard`; or `on` = `1`/`0`), `version` | Shows clubs and gymnasts their flight, or hides it; publishing a draft changed since the page was opened is refused |
| `POST /competitions/admin/{token}/timetable/officials` | `action` (`rota`, or `seat` with `flight` or `block`, `seat`, `person`), `version` | Assigns every panel again, or gives one seat to someone (`""` empties it) |
| `GET /competitions/admin/{token}/timetable/print` | `sheet` (`marshal`, `judges`, `scores`, `rota`, `timeline` or `timeline-officials`) | Printable sheets, one area to a page, with each flight's panel; the recorders' score sheets, a flight to a landscape page; each person's duties; the panel timeline, every day's areas side by side; or the timeline with a column for each seat, a day's area to a sheet |
| `GET /competitions/admin/{token}/timetable/timeline.csv` | — | The panel timeline as CSV: a row per flight or block on each area, with its officials by role |
| `GET /competitions/admin/{token}/timetable/delay` | `day` (index), `from` ("10:30"), `minutes`, `area` (any number; none for all); easing: `breaks` (`""`, `move`, `shorten`, `through`), `shorten`, `between`, `quicker` (%), `maxFlight`, `overrun` | What a delay does to the planned timetable, without saving: flights shifted or moved, what no longer fits, clashes. `POST` to the same address, with `version`, keeps it in the draft, worked out again from the draft as it is (refused if the draft has changed since the page was opened) |
| `GET /competitions/admin/{token}/day` | `day` (index; default today if it's a day of the timetable, else the first), `notice` | On the day: the published timetable's flights by area, planned against actual times, and how late each area is running (chair and timetable links too) |
| `POST /competitions/admin/{token}/day/flight` | `flight` (key: the flight's ID, `competitions.FlightKey`), `what` (`start`, `finish`, `undo`), `day` | Records a flight started or finished now, or takes the latest back; a mark that changes nothing is ignored with a notice (chair and timetable links too) |
| `POST /competitions/admin/{token}/day/checkin` | `entry` (an entry in a published flight), `status` (`here`, `scratched`, or empty to clear), `day` | Marks a gymnast here or scratched, or clears it; scratched gymnasts are struck through on the printed sheets; a mark that changes nothing is ignored with a notice (chair and timetable links too) |
| `POST /competitions/admin/{token}/day/clear` | `person` (key of a scratched person), `kind` (`officiating` or `competing`), `day` | Clears a scratched person's warning of that kind ("still officiating" / "still competing") for the whole competition; refused with a 400 if the person isn't scratched or the kind is neither; clearing what is already cleared is ignored with a notice (chair and timetable links too) |
| `POST /competitions/admin/{token}/day/officials` | `person` (key of an official with a seat on the flight), `flight` (key: the flight's ID, `competitions.FlightKey`), `status` (`here`, `missing`, or empty to clear), `day` | Marks an official here or missing for the panel, on every flight of the flight's run (same event, area and day, back to back) where they hold a seat, or clears it; "here" clears their scratch warnings and ticks them on the printed chair of judges sheets; refused with a 400 if they hold no seat on that flight; a mark that changes nothing is ignored with a notice (chair and timetable links too) |
| `POST /competitions/admin/{token}/day/notify` | `person` (key), `flight` (key), `as` (`official` or `gymnast`), `them` (`1`), `club` (`1`), `note` (up to 300), `day` | Sends a desk message at once saying where they should be (the official's seat or the gymnast's flight and warm-up), to them and/or their club's comp sec and coaches (not its members); refused with a 400 if they aren't on that flight; with no one ticked, or a club for someone with none, nothing is sent and a notice says so (chair and timetable links too) |
| `POST /competitions/admin/{token}/day/up` | `flight` (key: the flight's ID, `competitions.FlightKey`), `what` (`next` or `back`), `entry` and `round` (the gymnast and routine the page showed, empty before the first tap: a tap from anywhere else changes nothing), `day` | Who's up now: moves a running flight (started, not finished) on to the next gymnast's routine, or back one, skipping the scratched; a tap that changes nothing (Back at the first, Next after the last routine, a flight not running) is ignored with a notice, and Next never finishes the flight; refused with a 400 for a flight not in the published timetable or another `what`; not in the history (chair and timetable links too) |
| `GET /competitions/admin/{token}/screen` | `day` (index; default today if it's a day of the timetable, else the first), `area` (any number; none for all) | The venue screen: each area in its own colour, busy ones as cards (the flight on now and the next, with gymnasts and clubs, and the area's lateness), quiet ones as strips; announcements across the top. Its script fetches it again every 30 seconds and keeps the last good one when that fails. The only route a "venue screen" link can use; organiser, "everything", chair and timetable links can open it too |
| `POST /competitions/admin/{token}/desk/screen` | `text` (up to 160), `for` (minutes: `15`, `30`, `60`; `0` until taken down) | Puts an announcement across the top of the venue screen; nothing, with a notice, if there's no text (the links that can send desk messages) |
| `POST /competitions/admin/{token}/desk/screen/{id}/remove` | — | Takes an announcement down |
| `GET /competitions/admin/{token}/desk` | `notice` | Messages from the organisers' desk: the form (to a club, a flight, all officials, a panel's officials or picked people) and the messages sent, latest first (chair, timetable and "everything" links too) |
| `POST /competitions/admin/{token}/desk` | `to` (`club`, `flight`, `officials`, `panel`, `people`), `club` (id), `flight` (key: the flight's ID, `competitions.FlightKey`), `panel` (`day\|area`), `person` (keys, repeated), `text` (up to 500), `come` (`1`) | Sends the message at once by push and email to those who asked to hear it, and shows it on recipients' pages; nothing is sent, with a notice, if there is no text (unless `come`) or no one to send it to (chair, timetable and "everything" links too) |
| `GET /competitions/admin/{token}/timetable/leave` | `person` (key), `day`, `from`, `forgood` (`1`), `prefer` (`""` easiest to fill, `fewest`) | How a leaving official's seats would be filled, without saving. `POST` to the same address, with `version`, keeps it in the draft, as for a delay |
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
