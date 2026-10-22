---
type: API Reference
title: HTTP routes
description: Every route the server exposes, what it takes and what it returns, along with the limits applied to all requests.
resource: https://github.com/jackpardy/tariff-calculator/tree/master/web
tags: [architecture, http, routes]
generated: { by: claude-code/cli, at: 2026-10-04T17:45:00Z }
---

# Routes

The calculator's routes are defined in `routesWithPages()` in `web/calculator.go`;
the competition and club routes are registered by each page file in `web/`
(each has a `register…` function calling `handle`). Forms are
`application/x-www-form-urlencoded`. "Routine" means a JSON array of
[skills](../domain/skill-model.md#schema).

| Method and path | Input | Returns |
|---|---|---|
| `GET /` | optional `back` (an entry form's address, under `/clubs/` or `/competitions/`) | The calculator page ([routine builder](../features/routine-builder.md)); with `back`, from an entry form's **Open the Routine Builder →**, a **← go back to your entry** link to it (`no-referrer`, `no-store`) |
| `POST /skill-form` | optional `skill` (JSON) + `editIndex` | The "Add a skill" panel, for the default skill or the one being edited |
| `POST /skill-inputs` | the form's fields, or `load=common` + `commonSkillKey` (+ `shape`), or `load=landing` + `commonSkillKey` + `previous_skill` (a body-landing tile, taking off where that skill landed), or `load=skill` + `skill` | The re-rendered editor; 204 while the form holds something unscorable |
| `GET /skill-search` | `q` | Search results ([skill catalog](../domain/skill-catalog.md)) |
| `POST /calculate-skill` | the form's fields | The skill as JSON, named and priced, for the page to store |
| `GET /judging` | the form's fields (query), optional `last=1` | The "How it's judged" pop-up ([execution judging](../domain/execution.md)); `last=1` adds the landing; 400 for a skill that can't land |
| `POST /routine` | `routineData`, `requirementSet`, `checks`, `side` (`a`/`b`), optional `compareData` + `compareName`; for a [level](../requirements/levels.md)'s exercise, `level`, `exercise` and the other exercise's `pairData`, `pairSet`, `pairChecks`, `pairName` | The routine view: cards, flags, totals, requirement results, and the level panel |
| `POST /set-routine` | `requirementSet`, optional `routineData` | JSON `{name, skills, matches}` ([set routines](../requirements/set-routines.md)) |
| `POST /requirements/set-routine` | `routineData` | The set routine editor filled from a routine |
| `POST /qr` | `text` | An SVG QR code ([sharing](../features/sharing.md)) |
| `GET /compare`, `POST /compare` | `aData`, `aName`, `bData`, `bName` | The [compare page](../features/compare.md) and its comparison |
| `GET /requirements` | — | The requirements page |
| `POST /requirements/editor` | `set` (JSON), `copy` (a built-in's reference: a copy named "Copy of …"), or the editor's form fields + `action`/`add` | The re-rendered editor with problems listed. The Requirements page duplicates a built-in with `copy`, rather than carrying every built-in set in the page |
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

Any other `GET` address, anywhere, is **Page not found** (404): a page as
the app's look, with the way on, sent with the same three headers since an
address may hold a link's secret (`notFound`, `web/helpers.go`). An address
that only takes a `POST` still answers 405.

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
messages, Tell the organiser) are built in one place too, `competitionLinks` in the same file,
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

**Kept between requests** (`web/facts.go`). A competition's entries
(`entriesOf`, as `Store.Entries` has them), its `liveRota`, its checked
cards (`judgedOf`: the entries not waiting, or only the live ones) and its
published timetable as the pages show it (`publishedDays`, for everyone's
My competition) are worked out by the first page that needs them and shared
until something they're read from changes. The store counts those changes
for each competition (`Store.Writes(id)`, `store/changes.go`): every
statement and transaction runs on a connection watching each row it
changes (SQLite's pre-update hook), and once it's done the count moves for
the competition the row is part of. Only the tables the cache reads count
(`watched`: competitions, entries, member entries, officials, the club and
coach links to a competition, and members, coaches and individuals, who
may be in any competition, so a change to one of those counts for every
competition, unless it only touches columns the pages don't read, such as
when a club was last used). Check-ins, who's up, flight times, desk
messages, notices, feedback, push subscriptions, marks and other
competitions' changes leave a competition's facts as they are.
`TestWatchedTables` makes every new table go in `watched` or not, and
`TestWrites` lists each kind of change and whose count it moves. A page
that finds the count moved reads them again; one request does it while the
others wait. What the cache hands out is shared: read it, never change it.
Slices are copies, but the entries' own maps and pointers, `rotaFacts`'
maps and the timetable's days are not. Every web test checks, after each
request, that the cache still matches the store read afresh
(`checkingFacts` in `web/facts_test.go`), which also catches a change the
count missed. It counts only this process's changes: nothing else may
write to the database while the app runs. On the full demo (700 entries),
a gymnast's own page went from 65 ms and 88 MB allocated to 6 ms and 12 MB
(2026-10-15), and, as a check-in no longer drops the cache, to 2 ms and
3 MB just after one (2026-10-19), not 64 ms and 88 MB.

A timetable that doesn't fit lists changes that might make it (`fixesOf`,
`web/fixcache.go`), each a plan made again: they're kept for each
competition until its entries, events or setup change.

The database has at most four connections (`maxConns`, `store/store.go`),
kept open: SQLite writes one at a time anyway, and a connection for each
request in a burst made hundreds. No change holds a connection while it
asks for another. Opening a member's or coach's page reads without a
transaction, and marks their club used (once a day) only when it's due.

| Method and path | Input | Returns |
|---|---|---|
| `GET /competitions/new` | — | The form that creates a competition, and the competitions saved in this browser |
| `POST /competitions` | `name`, `date`, `deadlineDate`, `deadlineTime` (Irish and UK time), `individuals`, `opens` (`later`, the default, for private; `now`; or `at` with `liveDate` and `liveTime`), each built-in `level`, each of the browser's own levels as `custom` (`{level, sets}`) | 303 to the dashboard with `?new=created`; 422 with the problems; 429 after 5 an hour from one address |
| `GET /competitions/admin/{token}` | optional `club` (a club's name or `individual`), `problems=1`, `new` | The organiser's dashboard |
| `GET /competitions/admin/{token}/settings` | optional `notice`, `new=replaced` | The Settings page: links and every setting, in groups (organiser and "everything" links only). The settings forms below come back to it |
| `GET /competitions/admin/{token}/entries/{id}` | optional the dashboard's filters and `sort` (`club`, `problems`, `q`, `coach`, `checked`, `signedoff`, `video`, `category`), carried on | One entry, checked; **← Entries** goes back to the dashboard with those filters, at `#entry-<id>` |
| `POST /competitions/admin/{token}/entries/{id}/check` | posted `checked` (`1`/`0`), `note`, `next=1`; the dashboard's filters and `sort` in the query | Marks the entry checked or not, with a note the club or gymnast sees; with `next=1`, on to the next entry to check that the filters show, keeping them |
| `GET /competitions/admin/{token}/cards` | optional `club`, `problems=1`, `unchecked=1`, `level`, `entry` (and the entries page's other filters); `part` (a level's category), `all=1` | Printable competition cards (the [tariff sheet](../features/tariff-sheet.md), filled in), one exercise per page. More than 100 over more than one level, with no `level`, `entry` or `all`, lists the levels (a big one in parts by category) to print one at a time instead |
| `GET /competitions/admin/{token}/entries.csv` | — | Every entry as CSV: gymnast, club, level, each exercise and its difficulty, problems, checked, note, sent |
| `POST /competitions/admin/{token}/deadline` | `close=1`, or `deadlineDate` and `deadlineTime` | Closes entries now, or changes when they close (by the end of the competition date) |
| `GET`, `POST /clubs/member/{token}/competitions/{id}/notify`, `/competitions/entry/{token}/notify`, `/clubs/admin/{token}/competitions/{id}/notify`, `/clubs/coach/{token}/competitions/{id}/notify` | `email`, `adult=1`, each `topic` (`timetable`, `duties`, `cards`), `action` (`save`, or `remove` with `remove`, the subscription) | Hearing about a competition's changes (ADR 0008): a member, an individual, the comp sec, a coach. Push: `action=push`, `endpoint`, `p256dh`, `auth`, each `topic` (from `static/js/push.js`) |
| `GET /clubs/member/{token}/competitions/{id}/score-sheets`, `/competitions/entry/{token}/score-sheets` | — | The published score sheets of the flights the person officiates |
| `GET /clubs/member/{token}/competitions/{id}/calendar.ics`, `/competitions/entry/{token}/calendar.ics` | — | A person's published flights and duties as an iCalendar file to add or subscribe to; empty until published |
| `GET /clubs/member/{token}/competitions/{id}/club-calendar.ics`, `/clubs/admin/{token}/competitions/{id}/calendar.ics`, `/clubs/coach/{token}/competitions/{id}/calendar.ics` | — | The club's: every flight one of its gymnasts competes in, and every seat one of its people holds, named |
| `GET /sw.js` | — | The service worker, for push notifications and offline copies of pages (dropped after 30 days, or on a 404 or 410) |
| `GET /favicon.ico` | — | Nothing (204, cached a day): the site has no icon, so browsers stop asking |
| `GET /manifest.webmanifest` | — | The web manifest, for adding a page to the home screen (no `start_url`) |
| `GET`, `POST /notify/confirm/{token}` | — | Confirms an email (a button, then done) |
| `GET`, `POST /notify/off/{token}` | — (or `List-Unsubscribe=One-Click`) | Stops every email to that address for the competition |
| `POST /competitions/admin/{token}/notify-now` | — | Sends the changes waiting at once |
| `POST /competitions/admin/{token}/no-wait` | `on` (`1`/`0`) | No wait before telling changes on the competition's days |
| `POST /competitions/admin/{token}/live` | `opens`: `now`, `at` (with `liveDate` and `liveTime`, before the deadline) or `private`; `back=settings` to come back to Settings rather than the entries page | Goes live now or then, or makes the competition private, pausing entries and keeping those made |
| `POST /competitions/admin/{token}/video` | `video` (`""`, `skills`, `routine`), `videoTriples`, `videoDoubles`, `videoTariff` | Changes what video proof the competition asks for (also taken by `POST /competitions`) |
| `POST /competitions/admin/{token}/entries/{id}/video` | `review` (`ok`, `more`, `""`), `note` | The organiser's review of an entry's videos |
| `POST /competitions/admin/{token}/individuals` | `on` (`1`/`0`) | Turns individual entry on or off |
| `POST /competitions/admin/{token}/replace-link` | `anyway=1` | Without `anyway`, Settings asking first (200): the old link stops working at once for everyone who has it. With it, 303 to the new admin link's Settings page, `?new=replaced` |
| `GET`, `POST /clubs/member/{token}/competitions/{id}/late`, `/clubs/admin/{token}/members/{member}/competitions/{id}/late`, `/competitions/entry/{token}/late` | `discipline` (query), the entry form, `note` | Asking for a late change, once entries close |
| `GET /competitions/admin/{token}/late` | — | Late changes: what's allowed and each request, with whether it fits |
| `POST /competitions/admin/{token}/late/settings` | `late-<kind>=1`, `fee-<kind>` (`level`, `routines`) | Which late changes can be asked for, and their fees |
| `POST /competitions/admin/{token}/late/{id}/accept` | `note` | Applies a late change: the entry, the draft timetable, the fee |
| `POST /competitions/admin/{token}/late/{id}/reject` | `reason` | Turns a late change down |
| `POST /competitions/admin/{token}/later-deadlines` | `changesDate`, `changesTime`, `signoffsDate`, `signoffsTime` (empty: with the deadline) | When changes to entries, and coaches' sign-offs, close |
| `POST /competitions/admin/{token}/limits` | `limit-<n>` for each event, in the competition's order (empty: none), `anyway=1` | Sets each event's limit (waiting lists). Limits that would let entries in from a waiting list, or move entries in onto one, show **Before you change it** (200), naming them, unless `anyway=1` |
| `POST /competitions/admin/{token}/entries/{id}/let-in` | `on` (`1`/`0`) | Lets a waiting entry in over the limit, or back |
| `GET /competitions/admin/{token}/fees` | — | Entry fees: what's charged, and each club's and individual's account |
| `POST /competitions/admin/{token}/fees/settings` | `currency` (`GBP` or not), `fee-<discipline>`, `club`, `instructions` | Sets the fees |
| `POST /competitions/admin/{token}/fees/payments` | `payer` (`club:<id>` or `entry:<id>`), `amount`, `note` | Records a payment |
| `POST /competitions/admin/{token}/fees/payments/{id}/remove` | `anyway=1` | Removes a payment; without `anyway`, Fees asking first (200). One removed already says so |
| `GET /competitions/admin/{token}/fees/invoice` | `payer` | A payer's invoice, to print |
| `GET /clubs/admin/{token}/competitions/{id}/invoice`, `/competitions/entry/{token}/invoice` | — | The club's, or an individual's, invoice, to print; Back goes to the club's Fees tab |
| `POST /competitions/admin/{token}/links` | `name`, `kind` (`cards`, `chair`, `timetable`, `everything`) | Makes a helper's link (ADR 0009) and shows it once |
| `POST /competitions/admin/{token}/links/{id}/remove` | `anyway=1` | Stops a helper's link working; without `anyway`, Settings asking first (200) |
| `POST /competitions/admin/{token}/ties/remove` | `id` (the tie) | Stops someone's own link opening these pages through the admin link or a helper's (organiser only) |
| `GET /competitions/admin/{token}/history` | — | Every change, when and by which link |
| `POST /competitions/admin/{token}/concerns` | `text`, optional `entry` | Flags a concern for the organiser (any admin link) |
| `POST /competitions/admin/{token}/concerns/{id}/resolve` | `note` (only organisers see it), optional `reply` (a message's sender sees it) | Marks a concern or message resolved; a message's sender is told by push or email if they asked to hear |
| `POST /competitions/admin/{token}/concerns/{id}/received` | — | Tells a message's sender the organiser has it (once) |
| `POST /competitions/admin/{token}/concerns/{id}/reply` | `reply` (up to 1,000) | Adds the organiser's reply to a message's thread, which the sender sees and is told of; past 50 additions it's refused |
| `POST /clubs/member/{token}/competitions/{id}/organiser`, `/clubs/admin/{token}/competitions/{id}/organiser`, `/clubs/coach/{token}/competitions/{id}/organiser`, `/competitions/entry/{token}/organiser` | `text` (up to 1,000), `entry` (any number, theirs the competition has), `event` (any number; a comp sec's or coach's, events they have entries in), `back` (`day` for My competition, `gymnast` for an individual's page) | [A message to the organiser](../features/competition-entries.md#messages-to-the-organiser), named from the link (never the form), kept with the concerns, entries an event covers dropped; 303 back to the page with a notice. Refused with a notice while the organiser has messages off, when empty, or past 10 a sender in 24 hours; an entry not theirs, or an event they can't choose, is a 404 |
| `POST /clubs/member/{token}/competitions/{id}/organiser/{message}`, and the same under the other three | `text` (up to 1,000), `back` | Adds the sender's words to their own message's thread, opening it again if resolved; 303 back to it (`#message-{id}`); someone else's is a 404 |
| `POST /competitions/admin/{token}/messages` | `on` (`1`/`0`) | Lets people send the organiser messages, or stops them (organiser and everything links) |
| `POST /competitions/admin/{token}/feedback`, `/clubs/admin/{token}/feedback`, `/competitions/club/{token}/feedback`, `/clubs/member/{token}/feedback`, `/clubs/coach/{token}/feedback`, `/competitions/enter/{token}/feedback`, `/competitions/entry/{token}/feedback`, `/competitions/gymnast/{token}/feedback`, `/competitions/partner/{token}/feedback`, `/competitions/signoff/{token}/feedback`, and `/feedback` for pages without a link | Multipart (or a plain form without JavaScript): `text` (up to 2,000), optional `reply` (up to 200), `page` (the page's route pattern), `back`, `details` (JSON, up to 16 KB), `picture` (JPEG, PNG or WebP, up to 2 MB), `snapshot` (the page's HTML, gzipped or not, up to 3 MB; Full only), `with-picture` (`1`, Light); `competition` for the page's competition | Keeps [feedback about the app](../features/competition-entries.md#feedback-about-the-app) for the maintainer, with the link's role (any admin link but the venue screen's; "Calculator" or "Without a link" at `/feedback`, 30 an hour an address) and the competition if the link can see it, the copy cleaned (`cleanSnapshot`), and emails it to `FEEDBACK_EMAIL`; with `X-Feedback`, answers 200 with the thanks (or the reason it's refused: 403 when the button is off, 413, 422, 429 past 300 a day), otherwise redirects back to the page at `#feedback-sent` |
| `GET /site/{token}` | `all=1` for those dealt with too | The maintainer's page: the feedback about the app, newest first, with its picture, copy and details; the button and Full or Light as settings lines. Only the maintainer's link (`tariffCalculator maintainer-link`); anything else under `/site/` is a 404 |
| `POST /site/{token}/settings` | `button` (`on`, `off`) or `capture` (`full`, `light`) | Turns the Feedback button on or off everywhere, or chooses what goes with it |
| `POST /site/{token}/feedback/{id}/done` | `done` (`1` or `0`), `all` | Marks feedback dealt with, or not |
| `POST /site/{token}/captures/delete` | — | Deletes every picture and copy of a page kept; the words stay |
| `GET /site/{token}/feedback/{id}/picture`, `/site/{token}/feedback/{id}/page` | — | The picture of their screen; the copy of the page, cleaned, served `sandbox` with `default-src 'none'` (`snapshotPolicy`), framed only by the maintainer's page |
| `GET /sandbox` | — | The [sandbox](../features/sandbox.md)'s start page, "Try it as…": a card for each role with the shared copy's links; "being set up" before the first reset. Sandbox mode (`SANDBOX=1`) only; a 404 otherwise |
| `POST /site/{token}/sandbox/reset` | — | Resets the sandbox in the background (a new copy, then the old deleted), one thing at a time, and comes back to the maintainer's page saying so. Sandbox mode only |
| `POST /site/{token}/sandbox/restart` | — | Restarts the day of the competition on the day now, in the background: day 1 today, the day's marks cleared, the morning played again. Sandbox mode only |

Every `/competitions/admin/{token}` route takes the admin link or a helper's
link of a kind that may use it (`allowed`, ADR 0009); others get 403.
Likewise every `/clubs/admin/{token}` route (those under the shared roots
too) takes the club admin link, or a link with a role whose permissions
allow it (`clubRoutes` in `web/clubaccess.go`, ADR 0015); others get 403,
"This link can't do that. Ask your club admin."
| `POST /competitions/admin/{token}/delete` | `confirm` (the word `delete`, typed) | Deletes the competition and its entries; anything else typed deletes nothing and says so |
| `GET`, `POST /competitions/enter/{token}` | `gymnast`, `level`, `ex1Option`, `ex1Skills`, `ex2Option`, `ex2Skills`, and where video is asked for `ex1Video`, `ex1VideoNote`, `ex2Video`, `ex2VideoNote`; optional `as`, the link of the person's page (a member's, coach's or individual's, or an entry link) | The individual entry form; entering redirects to the entry, `/competitions/entry/{personal}~{entry id}?saved=1`, as that person or someone new; 422 for an `as` that isn't a page's link |
| `GET`, `POST /competitions/entry/{token}` | as entering; `saved`, `notice` carried on | An individual's own entry: GET redirects (303) to their page for the competition with `entry=<id>`, opening its row, carrying the query; POST changes it until the deadline, back here, or with problems shows their page with the row and form open (422). `{token}` an entry link from before or a personal link's way to the entry; an entry on no one's page shows on its own |
| `GET /competitions/gymnast/{token}` | optional `notice` | An individual's page: the competitions they entered. `{token}` their personal link or an entry link from before |
| `GET /competitions/gymnast/{token}/details` | optional `notice` | Their link and their other links |
| `GET /competitions/gymnast/{token}/competitions/{id}` | optional `entry` (the row to open), `saved=1`, `notice` | Their page for a competition: a row per discipline, each opening to all of that entry (its card, changing and withdrawing it, their coach, the sign-off link, fees, what they offer), its forms posting to the entry's own address; 404 for one they haven't entered |
| `GET`, `POST /competitions/gymnast/{token}/competitions/{id}/enter` | as entering, without `as` or `gymnast` | Enters another discipline of a competition they've entered, under the same link and their name, on their page (tabs for the disciplines not entered, links to those entered, back to the competition); a discipline entered already shows a link to that entry (422 when posted) |
| `POST /clubs/member/{token}/links`, `/clubs/coach/{token}/links`, `/competitions/gymnast/{token}/links` | `link`, `sure=1` | Adds another of the person's links to their page; back to their details with a notice. A personal link whose name (its first word) isn't one of theirs shows **Is this you?** first, which posts again with `sure=1` |
| `POST /clubs/member/{token}/links/remove`, `/clubs/coach/{token}/links/remove`, `/competitions/gymnast/{token}/links/remove` | `kind` (`member`, `coach`, `individual`, or a shared link's), `id` (the record, or the tie) | Takes a role off their page |
| `POST /competitions/entry/{token}/withdraw` | `confirm=1` | Withdraws the entry: kept, marked withdrawn (`store.Withdrew`), out of everything counted; their helping offer moves to another of their entries there, or goes with the last. Back to the entry with a notice |
| `POST /competitions/entry/{token}/restore` | — | Puts back an entry they withdrew, until entries close, at the back of any waiting list |
| `GET /competitions/entry/{token}/day` | `notice`, `timetable=all` | My competition, for someone entering on their own (all their entries, matched by name) |
| `POST /competitions/entry/{token}/here` | — | "I'm here for the rest": a scratched individual clears both scratch warnings for themselves, then goes back to My competition with a notice; only once the timetable is published |
| `GET /competitions` | — | The competitions, clubs and entries this browser has links to |
| `GET`, `POST /competitions/club/{token}` | `clubAdmin` (a club's admin link, or its token; or a club link whose role can enter entries) | A competition's club link: a comp sec enters their club, then lands on the club's page for the competition |
| `GET /clubs/new`, `POST /clubs` | `name` | Creates a club: 303 to its page with `?new=created`; 429 after 5 an hour from one address |
| `GET /clubs/admin/{token}` | optional `new`, `notice` | The comp sec's Competitions section (the default): a row for each competition the club is in (entries, to send, with problems, timetable published, messages), opening its page |
| `GET /clubs/admin/{token}/competitions/{id}` | optional `notice` | The comp sec's page for one competition, its **Entries** tab: where they stand, sending them, and the members' entries; 404 if the club isn't entered |
| `GET /clubs/admin/{token}/competitions/{id}/coaches`, `/offers`, `/fees` | optional `notice` | Its other tabs: the coaches to send for approval (404 unless it approves coaches), what members offer to judge and help with, and what the club owes with the invoice's lines (404 unless it charges) |
| `GET /clubs/admin/{token}/competitions/{id}/enter-member` | `member`, `discipline` | **Enter a member**: to the member's entry form for that discipline; back with "Choose a member" for one who isn't the club's |
| `GET /clubs/admin/{token}/competitions/{id}/add-offer` | `member` | **Add an offer**: to the member's offer form |
| `GET /clubs/admin/{token}/members`, `/coaches`, `/settings` | optional `notice`; on Members, `at` (the member the notice is about, whose row says it too); on Settings, `history=all` | The comp sec's other sections: the join link, members and synchro entries made with them; the coaches; Settings: the club admin's roles and links, members' skills, the club's history (the latest 50, or all kept), replacing the admin link and deleting the club. Each has the section menu |
| `POST /clubs/admin/{token}/competitions/{id}/send` | `which` (`changed` or `all`) | Sends new and changed entries, or all again (taking back withdrawn ones); back to the competition's Entries |
| `GET`, `POST /clubs/admin/{token}/members/{member}/competitions/{id}` | as entering, without `gymnast` | Enters or changes a member's entry on their behalf (a synchro partner chosen from the members, or typed); saving goes back to the competition's Entries |
| `POST /clubs/admin/{token}/members/{member}/competitions/{id}/withdraw` | `confirm=1`, `discipline` | Withdraws a member's entry, as they can; back to the competition's Entries |
| `POST /clubs/admin/{token}/members/{member}/new-link` | `anyway=1` | Without `anyway`, Members asking first (the old link stops working), its button posting again with it; with it, the page with the member's new link, shown once |
| `POST /clubs/admin/{token}/members/{member}/remove` | `anyway=1` | Removes the member and their entries; without `anyway`, Members asking first (200) |
| `POST /clubs/admin/{token}/replace-link`, `/delete` | `anyway=1` to replace, `confirm` (the word `delete`, typed) to delete | As for a competition, but replacing asks first on Settings without `anyway`. The club admin's only |
| `POST /clubs/admin/{token}/roles` | `name` (up to 40, one role a name), `permission` (repeated: `enter`, `send`, `fees`, `members`, `coaches`, `certificates`, `organiser`, `settings`) | [Club roles](../features/competition-entries.md#club-roles-and-links-adr-0015) (ADR 0015): makes a role; back to Settings `#links`. Refused with a notice for no name, a name taken, or past 10. The club admin's only |
| `POST /clubs/admin/{token}/roles/{role}` | `name`, `permission` (repeated) | Renames a role and sets its permissions, for every link with it at once; back to Settings `#roles` |
| `POST /clubs/admin/{token}/roles/{role}/remove` | — | Removes a role with no links; refused with a notice (how many links) while it has some |
| `POST /clubs/admin/{token}/links` | `name` (up to 60, one link a name), `role` (its id) | Makes a link for a person with a role: Settings (200) with the link, shown once (`/clubs/admin/{token}` like the admin link). Refused with a notice past 30, for no name, a name taken or no role |
| `POST /clubs/admin/{token}/links/{link}/role` | `role` (its id) | Gives a link another role, at once; back to Settings `#links` |
| `POST /clubs/admin/{token}/links/{link}/remove` | `anyway=1` | Without `anyway`, Settings asking first (200); with it, the link stops at once |
| `GET`, `POST /clubs/join/{token}` | `name` | Joining a club: 303 to the member's page with `?new=joined` |
| `GET /clubs/member/{token}` | optional `new`, `notice` | A member's Competitions tab (the default): a row for each competition the club is in, with its date and status |
| `GET /clubs/member/{token}/competitions/{id}` | optional `notice`, `discipline` (the row to open) | A member's page for one competition: facts, links, each discipline's entry as a row that opens to its card and forms, officiating, and the judging offer; 404 if the club isn't entered |
| `POST /clubs/member/{token}/judging`, `POST /clubs/admin/{token}/members/{member}/judging`, `POST /competitions/entry/{token}/judging` | `qualification` (a key of `competitions.JudgeQualifications`), `certificate` (a file), `back` | Adds a judging qualification with its certificate (ADR 0014): the member's, the comp sec's for a member, or an individual's; back to `back` at `#judging` with what happened. An upload, up to 3 MB |
| `POST <those>/{q}/remove` | `back` | Removes one, with its certificate |
| `GET <those>/{q}` | — | Its certificate, for the person (or the comp sec) |
| `GET /competitions/admin/{token}/officials/{id}/judging/{q}` | — | An official's certificate, for the organiser and the timetable link |
| `GET /clubs/member/{token}/details` | optional `notice` | A member's personal link, their coach and their other links |
| `GET /clubs/member/{token}/requirements`, `GET /clubs/coach/{token}/requirements`, `GET /competitions/gymnast/{token}/requirements` | optional `notice` | The person's Requirements tab (ADR 0013): their kept requirements and levels, Take off, and Add from this browser |
| `GET <personal link>/requirements.json` | — | The person's kept requirements as JSON, `{"sets": [...], "levels": [...]}`, each `{id, kind, data, described}`, for the Requirements page and the builder |
| `POST <personal link>/requirements` | `items` (JSON list of `{id, kind, data}`), optional `back` | Keeps them on the person's page by their id in the browser (changing one kept by that id); with `back`, to the Requirements tab with a notice, else JSON `{kept, added, refused}` |
| `POST <personal link>/requirements/{kind}/{source}/remove` | optional `back` | Takes one off the person's page |
| `GET /clubs/member/{token}/routines` | optional `notice` | A member's Routines tab (ADR 0011): their kept routines, each checked, with a link opening it in the routine builder (`/#share=j…`), and adding routines from this browser |
| `POST /clubs/member/{token}/routines` | `routines` (a JSON list of `{id, name, skills, requirements, checks}`, as `RoutineStore.keepable` gives) | Keeps routines on the member's page (the builder's **Save to my page**, or adding from this browser); one with the same browser `id` is changed. Up to 512 KB. Back to Routines saying how many |
| `POST /clubs/member/{token}/routines/{routine}/rename`, `/delete` | `name` to rename, `anyway=1` to delete | Renames or deletes a kept routine (without `anyway`, Routines asking first, 200); back to Routines |
| `GET /clubs/member/{token}/competitions/{id}/day` | `notice`, `timetable=all` (the whole timetable, not only their flights and panels) | My competition: their events, cards, flights and times, duties, and the published timetable; a box to say "I'm here for the rest" if they're scratched from something |
| `POST /clubs/member/{token}/competitions/{id}/here` | — | "I'm here for the rest": a scratched member clears both scratch warnings for themselves, then goes back to My competition with a notice; only once the timetable is published |
| `POST /clubs/member/{token}/competitions/{id}` | as entering, without `gymnast` | Saves the member's entry (their club sends it) |
| `POST /clubs/member/{token}/competitions/{id}/withdraw` | `confirm=1` | Withdraws it |
| `POST /clubs/member/{token}/coach`, `POST /clubs/admin/{token}/members/{member}/coach` | `coach` (an id, `""` for none) | A member's coach, chosen by them (back to Your details) or the comp sec (back to the member's row on Members, `?at=<member>#member-<id>`, saying who their coach is now) |
| `POST /clubs/admin/{token}/coaches` | `name` | Adds a coach: the club page with their coach link, shown once |
| `POST /clubs/admin/{token}/coaches/from-member` | `member` | Makes a member a coach too; their member link opens the coach's page. Back to Coaches |
| `POST /clubs/admin/{token}/ties/remove` | `id` (the tie) | Stops someone's own link opening the comp sec's pages. Back to Settings |
| `POST /clubs/admin/{token}/coaches/{coach}/new-link`, `/remove` | `anyway=1` | A coach's new link (shown once), or removing them; without `anyway`, Coaches asking first (200) |
| `POST /clubs/admin/{token}/coaches-see-all` | `on` (`1`/`""`) | Whether every coach sees every member |
| `GET /clubs/coach/{token}` | — | A coach's page: each competition's entries they see, and their sign-off |
| `GET /clubs/coach/{token}/details` | optional `notice` | A coach's Your details: their coach link and their other links; 404 through another of their links (a member's who coaches too), whose page has them |
| `GET`, `POST /clubs/coach/{token}/members/{member}/competitions/{id}` | `signed` (`1`/`0`), `note` | One entry, checked, to sign off or say not yet; after either, the next entry at the competition waiting for a sign-off opens, or the coach's page once there's none |
| `GET /clubs/coach/{token}/gymnasts` | optional `notice` | A coach's Gymnasts: the members they see, with how many routines each keeps |
| `GET /clubs/coach/{token}/gymnasts/{member}` | optional `notice`; `routine` and `against` (`club:<level id>` or `builtin:<id>`) | One of them, with their kept routines, each checked, a note on each, the skills in each no coach has marked Can do, and a choice of level to check each against: `routine` opens checked against `against`; their entries, each with its sign-off; 404 for a member the coach doesn't see |
| `POST /clubs/coach/{token}/gymnasts/{member}/routines` | `routines` (as `POST /clubs/member/{token}/routines`) | A coach adds routines to the gymnast's page, kept as the coach's: the same browser routine again changes only theirs. Back to the gymnast; 404 for a member the coach doesn't see |
| `POST /clubs/coach/{token}/gymnasts/{member}/routines/{routine}/note` | `note` (`""` clears it) | The coach's note on a routine, which the member sees with the coach's name |
| `GET /clubs/admin/{token}/members/{member}/routines` | — | The comp sec's view of a member's kept routines, from Members |
| `GET /clubs/member/{token}/skills` | optional `q` (a skill search), `notice` | A member's Skills tab (ADR 0011): what they can do and are learning, who marked each, and skills to mark: the search's, their routines', the catalog's |
| `POST /clubs/member/{token}/skills`, `POST /clubs/coach/{token}/gymnasts/{member}/skills` | `skill` (a skill as JSON), `club` (a club skill to track, by id) or `item` (a level's link or set routine, `r:…`), `state` (`learning`, `can`, or `""` to take it off); `q` in the query is kept; `at` (the skill row's anchor, `mark-<list>-<hash>`); for a coach, `back` (`routine-<id>`) | The member, or a coach who sees them, marks a skill; back to the skills page at the row marked (`#at`), or for a coach marking from a routine, to the gymnast's page at that routine, with a notice. 400 for a skill that can't be read |
| `GET /clubs/member/{token}/skills/{member}` | — | Another member's skills, to look at, where the club shares skills; otherwise 404 |
| `GET /clubs/coach/{token}/gymnasts/{member}/skills` | optional `q`, `notice` | A coach's page for a gymnast's skills, to mark and confirm |
| `GET /clubs/admin/{token}/members/{member}/skills` | — | The comp sec's view of a member's skills |
| `POST /clubs/admin/{token}/skills-open` | `on` (`1`/`""`) | Whether every member sees everyone's skills. Back to Settings |
| `GET /clubs/coach/{token}/club-skills` | optional `q` (a catalog search), `notice` | The club's own skills (ADR 0011), for its coaches: its names for catalog skills and anything else it tracks, with forms to add them |
| `POST /clubs/coach/{token}/club-skills` | `name`, and `skill` (a catalog skill as JSON) to name it, or `note` for something else to track | Adds one of the club's skills; naming a skill already named renames it. Back with a notice (a name already used, or the club full, says so) |
| `POST /clubs/coach/{token}/club-skills/{id}/remove` | `anyway=1` | Removes it, with its marks if it's something to track; without `anyway`, Club skills asking first (200); 404 for another club's |
| `GET /clubs/coach/{token}/levels` | optional `notice` | The club's levels (ADR 0011), in order: each one's requirements in words, its club skills to track, **Change** and **Copy as a new level** (the level editor), and a link to it on the Requirements page |
| `GET /clubs/coach/{token}/levels/new` | optional `from` (a level's id, to copy) | The level editor for a new level: the requirements editor with the kinds of rule a level tracks and the club's own named skills to pick |
| `POST /clubs/coach/{token}/levels/new` | the editor's form | Saves it as a new level, last; with problems, the editor again (422) |
| `GET /clubs/coach/{token}/levels/{id}/edit` | — | The level editor for one of the club's levels |
| `POST /clubs/coach/{token}/levels/{id}/edit` | the editor's form | Saves the level in place |
| `POST /clubs/coach/{token}/levels/editor` | the editor's form, `level` (its id, "" for a new one) in the query | Draws the level editor again as it changes (htmx, as `POST /requirements/editor`) |
| `POST /clubs/coach/{token}/levels` | `set` (requirements as JSON), `id` (the browser's id for them) | Saves requirements as a club level, from the Requirements page's **Save to club levels**; the same `id` changes its level. Rules about a single routine are left out, saying so. 400 for requirements that can't be read |
| `POST /clubs/levels/save` | `link` (a coach's link, or a personal link with a coach's role), `set`, `id`, `role` (which coach's role, when the link has several) | The Requirements page's **Save to club levels**, as `POST /clubs/coach/{token}/levels`, for any link that opens a coach's page, a member's who coaches too included (ADR 0010). Asks which club where the link has several coach's roles; 400 for a link that isn't someone's own, 409 for one with no coach's role |
| `POST /clubs/coach/{token}/levels/{id}/move`, `/remove` | `by` (`-1` or `1`) to move, `anyway=1` to remove | Moves a level earlier or later, or removes it (without `anyway`, Levels asking first, 200); 404 for another club's |
| `GET /clubs/levels/shared` | the level in the fragment, `#share=j…` | Where a shared club level's link opens: its script posts the fragment to be shown |
| `POST /clubs/levels/shared` | `level` (the shared level as JSON) | The shared level, to add to a coach's club; 400 if it can't be read |
| `POST /clubs/levels/import` | `level`, `link` (a coach's link, or a personal link with a coach's role), `role` (which coach's role, when the link has several) | Adds a copy of the shared level, with its club skills to track, to the coach's club, and opens their Levels. The coach is found from the link as for `POST /clubs/levels/save`, asking which club where there are several; 400 for a level that can't be read or a link that isn't someone's own, 409 for one with no coach's role |
| `POST /competitions/admin/{token}/events` | `synchroLevel`, `synchroPairs` (synchro levels to run as one event, one event a line, joined by `+`), `tumbling`, `dmt` (one level a line) | Changes the synchro, tumbling and DMT events (also on `POST /competitions`); entries then take `discipline` and, for synchro, `partnerName` and `partnerClub` |
| `POST /competitions/admin/{token}/levels/move` | `move` (`<discipline>:<place>:<-1 or 1>`) | Moves a level one place easier or harder in its discipline's order (easiest first), which judging "up to" a level goes by |
| `GET`, `POST /competitions/partner/{token}` | `link` (the partner's own member page or entry link, or blank) | A synchro partner confirming the pair |
| `GET /competitions/admin/{token}/people` | — | People: everyone at the competition once (gymnasts, partners, officials, coaches), with what they do. Any link that can see the entries page, not the venue screen link |
| `GET /competitions/admin/{token}/people/{key}` | `key`: a person key (`store/personkey.go`), path-escaped as one segment (`m:<id>`, `i:<id>` or `i:<name>`, `p:<entry>`, `o:<id>`, `c:<coach id>`, `c::<name>` for an individual's coach); `timetable=all` | One person's read-only page for the organiser: entries, officiating, coaching, and once published their check-in, desk messages and their flights and panels (the whole timetable with `timetable=all`). 404 for a key nobody has |
| `GET /competitions/admin/{token}/officials` | `notice` | The officials: who can judge and chair each discipline, the panels and judging rule, and everyone offered or added |
| `POST /competitions/admin/{token}/officials/settings` | `judge` (`""`, `qualified`), `note`, `panel-<discipline>-<role>` | Panels per discipline, who may judge what, and the note to those offering |
| `POST /competitions/admin/{token}/officials/add` | `name`, `club`, and an offer | Adds a judge or helper |
| `POST /competitions/admin/{token}/officials/{id}/qualified`, `/remove` | `on` | Marks someone qualified, back at their row (`#official-<id>`) with a notice; removes someone the organiser added |
| `POST /clubs/member/{token}/competitions/{id}/offer`, `GET`/`POST /clubs/admin/{token}/members/{member}/competitions/{id}/offer`, `POST /competitions/entry/{token}/offer` | `judge-<discipline>`, `execution-<discipline>`, `difficulty-<discipline>`, `chair-<discipline>` (each `no`, `""` for any level, or the highest level; left out: execution and difficulty at any level, not chairing), `recorder`, `marshal` | What a member, the comp sec for a member, or an individual offers to judge and help with (the comp sec back to the competition's Judging and helping) |
| `POST /competitions/admin/{token}/split` | `split` (`""`, `all`, `some`), `splitLevel` | Which levels rank men and women separately (also `split=all` on `POST /competitions`); entries then take `category` |
| `GET /competitions/admin/{token}/timetable/setup` | `notice` | The timetable's Setup page: days and areas, timings and rest, blocked time, your rules, and **Plan**. The setup forms below come back to it |
| `GET /competitions/admin/{token}/timetable` | `notice` | The timetable: whether it's published (Publish, Discard, Unpublish, and Print's sheets), what to look at in the flights and the officials, Find a name with the jumps to each day and area, each day's areas, unplaced entries; before planning, a pointer to Setup. Each form on it also posts `version`, the draft's version as the page showed it: the posts below that carry it are refused with "The timetable changed since you opened this page; nothing was changed." if the draft has changed since, and every change to the draft is read and saved in one transaction |
| `POST /competitions/admin/{token}/timetable/setup/days` | `day-{i}-name`, `-start`, `-end`, `-area`; `add`, `remove` | Saves, adds or removes days |
| `POST /competitions/admin/{token}/timetable/setup/areas` | `area-{i}-name`, `-discipline`; `add`, `remove` | Saves, adds or removes areas (renames carry to days, blocks and rules) |
| `POST /competitions/admin/{token}/timetable/setup/timings` | `per-`, `between-`, `max-` per discipline; `rest`, `restMust`, `separate`, `separateLevel` | Saves timings, rest and which levels fly men and women apart |
| `POST /competitions/admin/{token}/timetable/setup/blocks` | `name`, `minutes`, `day`, `at` or `from`/`to`, `area`, and officials needed: `chair`, `judges`, `recorder`, `marshal`; or `remove` | Adds or removes blocked time |
| `POST /competitions/admin/{token}/timetable/setup/rules` | `kind` (`area`, `day`, `before`, `apart`; about officials: `role`, `off`, `hours`), `must`, `event`, `event2`, `area`, `day`, `person`, `role`, `from`, `to`; or `remove` | Adds or removes a rule |
| `POST /competitions/admin/{token}/timetable/plan` | `anyway=1` | Plans afresh from the entries and setup, each flight keeping the ID of the one it was (`Schedule.KeepIDs`); refused once the day has begun (anything marked on a published flight). Planning again (the draft is planned) shows **Before you change it** (200) instead, unless `anyway=1` |
| `POST /competitions/admin/{token}/timetable/flight` | `flight` (index), `action` (`redraw`, `move` with `to` = `day:area`, `earlier`), `version`, `anyway=1` | Redraws or moves a flight; it keeps its ID. A move that would break something new shows **Before you change it** (200) instead, unless `anyway=1` |
| `POST /competitions/admin/{token}/timetable/entry` | `entry`, `to` (flight index, `""` to take out), `version`, `anyway=1` | Moves or places a gymnast; as for a flight, it asks first if that breaks something new |
| `POST /competitions/admin/{token}/timetable/publish` | `action` (`publish`, `unpublish`, `discard`; or `on` = `1`/`0`), `version`, `anyway=1` | Shows clubs and gymnasts their flight, or hides it; publishing a draft changed since the page was opened is refused; publishing what would drop marks on the day (flights run, panel check-ins, gymnasts checked in) asks first unless `anyway=1`. Unpublishing and discarding the changes always ask first, on **Before you change it** (200), unless `anyway=1` |
| `POST /competitions/admin/{token}/timetable/officials` | `action` (`rota`, or `seat` with `flight` or `block`, `seat`, `person`), `version`, `anyway=1` | Assigns every panel again (on a planned draft, asking first on **Before you change it** unless `anyway=1`), or gives one seat to someone (`""` empties it); giving a flight's seat asks first if that breaks something new |
| `GET /competitions/admin/{token}/timetable/print` | `sheet` (`marshal`, `judges`, `scores`, `rota`, `timeline` or `timeline-officials`; any other is a 404, **No such sheet**), `draft` (`1` for the draft) | Of what's published (the draft, marked, before publishing, or with `draft=1` from a link that can change the timetable). Printable sheets, one area to a page, with each flight's panel; the recorders' score sheets, a flight to a landscape page; each person's duties; the panel timeline, every day's areas side by side; or the timeline with a column for each seat, a day's area to a sheet |
| `GET /competitions/admin/{token}/timetable/timeline` | `officials` (`1` for a column for each seat), `draft` (`1`) | The panel timeline as a page of the Timetable menu, once planned: of what's published, as the print sheets |
| `GET /competitions/admin/{token}/timetable/timeline.csv` | `draft` (`1`) | The panel timeline as CSV: a row per flight or block on each area, with its officials by role; of what's published, as the print sheets (a draft's first line says DRAFT) |
| `GET /competitions/admin/{token}/timetable/delay` | `day` (index), `from` ("10:30"), `minutes`, `area` (any number; none for all); easing: `breaks` (`""`, `move`, `shorten`, `through`), `shorten`, `between`, `quicker` (%), `maxFlight`, `overrun` | What a delay does to the planned timetable, without saving: flights shifted or moved, what no longer fits, clashes. `POST` to the same address, with `version`, keeps it in the draft, worked out again from the draft as it is (refused if the draft has changed since the page was opened) |
| `GET /competitions/admin/{token}/day` | `day` (index; default today if it's a day of the timetable, else the first; one the timetable hasn't is a 404, **No such day**), `area` (an area's name: its own page), `notice` | On the day: without `area`, an overview card for each area (how late it's running, the flight on now, who's up, counts, the next tap); with it, that area's own page with its flights in full. The day's forms post `day`, and `area` from an area's page, and come back to the same page (chair and timetable links too) |
| `POST /competitions/admin/{token}/day/flight` | `flight` (key: the flight's ID, `competitions.FlightKey`), `what` (`start`, `finish`, `undo-finish`, `undo-start`), `day`; `tapped` and `sent` (optional, the phone's clock in ms, for a tap kept without signal) | Records a flight started or finished when tapped (now, or now less `sent` − `tapped`), or takes one back; the earliest Started and Finished win and a Finished before the start is refused; a mark that changes nothing, or a tap time in the future or over 12 hours old, is ignored with a notice (chair and timetable links too) |
| `POST /competitions/admin/{token}/day/times` | `flight` (key), `started` and `finished` (clock times on the flight's day, "10:05"; empty for not yet), `day` | Corrects when a flight started and finished; refused with a notice for a finish before the start or without one, a time that can't be read, or one later than now on or after the flight's day; in the history (chair and timetable links too) |
| `POST /competitions/admin/{token}/day/checkin` | `entry` (an entry in a published flight), `status` (`here`, `scratched`, or empty to clear), `day`; `tapped` and `sent` as for `/day/flight` | Marks a gymnast here or scratched, or clears it, at the time tapped; the latest tap wins, so one older than the last mark is ignored with a notice; scratched gymnasts are struck through on the printed sheets; a mark that changes nothing is ignored with a notice (chair and timetable links too) |
| `POST /competitions/admin/{token}/day/clear` | `person` (key of a scratched person), `kind` (`officiating` or `competing`), `day`; `tapped` and `sent` as for `/day/flight` | Clears a scratched person's warning of that kind ("still officiating" / "still competing") for the whole competition; refused with a 400 if the person isn't scratched or the kind is neither; clearing what is already cleared is ignored with a notice (chair and timetable links too) |
| `POST /competitions/admin/{token}/day/officials` | `person` (key of an official with a seat on the flight), `flight` (key: the flight's ID, `competitions.FlightKey`), `status` (`here`, `missing`, or empty to clear), `day`; `tapped` and `sent` as for `/day/flight` | Marks an official here or missing, at the time tapped (the latest tap wins, flight by flight) for the panel, on every flight of the flight's run (same event, area and day, back to back) where they hold a seat, or clears it; "here" clears their scratch warnings and ticks them on the printed chair of judges sheets; refused with a 400 if they hold no seat on that flight; a mark that changes nothing is ignored with a notice (chair and timetable links too) |
| `POST /competitions/admin/{token}/day/notify` | `person` (key), `flight` (key), `as` (`official` or `gymnast`), `them` (`1`), `club` (`1`), `note` (up to 300), `day` | Sends a desk message (in the background, like the desk's) saying where they should be (the official's seat or the gymnast's flight and warm-up), to them and/or their club's comp sec and coaches (not its members); refused with a 400 if they aren't on that flight; with no one ticked, or a club for someone with none, nothing is sent and a notice says so (chair and timetable links too) |
| `POST /competitions/admin/{token}/day/up` | `flight` (key: the flight's ID, `competitions.FlightKey`), `what` (`next` or `back`), `entry` and `round` (the gymnast and routine the page showed, empty before the first tap: a tap from anywhere else changes nothing), `day`; `tapped` and `sent` as for `/day/flight` | Who's up now: moves a running flight (started, not finished) on to the next gymnast's routine, or back one, skipping the scratched; a tap that changes nothing (Back at the first, Next after the last routine, a flight not running) is ignored with a notice, and Next never finishes the flight; refused with a 400 for a flight not in the published timetable or another `what`; not in the history (chair and timetable links too) |
| `GET /competitions/admin/{token}/screen` | `day` (index; default today if it's a day of the timetable, else the first), `area` (any number; none for all) | The venue screen: each area in its own colour, busy ones as cards (the flight on now and the next, with gymnasts and clubs, and the area's lateness), quiet ones as strips; announcements across the top. Its script fetches it again every 30 seconds and keeps the last good one when that fails. The only route a "venue screen" link can use; organiser, "everything", chair and timetable links can open it too |
| `POST /competitions/admin/{token}/desk/screen` | `text` (up to 160), `for` (minutes: `15`, `30`, `60`; `0` until taken down) | Puts an announcement across the top of the venue screen; nothing, with a notice, if there's no text (the links that can send desk messages) |
| `POST /competitions/admin/{token}/desk/screen/{id}/remove` | — | Takes an announcement down |
| `GET /competitions/admin/{token}/desk` | `notice` | Messages from the organisers' desk: the form (to a club, a flight, all officials, a panel's officials or picked people) and the messages sent, latest first (chair, timetable and "everything" links too) |
| `POST /competitions/admin/{token}/desk` | `to` (`club`, `flight`, `officials`, `panel`, `people`), `club` (id), `flight` (key: the flight's ID, `competitions.FlightKey`), `panel` (`day\|area`), `person` (keys, repeated), `text` (up to 500), `come` (`1`) | Hands the message to the background sender for push and email to those who asked to hear it, answering at once with how many it's sending to, and shows it on recipients' pages; nothing is sent, with a notice, if there is no text (unless `come`) or no one to send it to (chair, timetable and "everything" links too) |
| `GET /competitions/admin/{token}/timetable/leave` | `person` (key), `day`, `from`, `forgood` (`1`) | How a leaving official's seats would be filled, without saving. `POST` to the same address, with `version`, keeps it in the draft, as for a delay |
| `GET /competitions/admin/{token}/timetable/simulate` | `from` (a scenario to start from), `notice` | The simulation: a scenario's numbers, filled in from the competition so far, and the scenarios side by side |
| `POST /competitions/admin/{token}/timetable/simulate` | `name`, `entries-<i>` (by event), `gymnasts`, `clubs`, `judges-<discipline>`, `chairs-<discipline>`, `quota-judges-<discipline>`, `quota-per-<discipline>`, `quota-chairs-<discipline>` (judges each club brings per competitors), `alsoJudge` (percent of the clubs' judges who also judge other disciplines), `judgePeople`, `competing`, `helpers`; or `again` (a scenario's index) | Plans stand-ins with the venue setup and keeps the scenario with its result (the last 8), or runs one again with the current setup |
| `POST /competitions/admin/{token}/timetable/simulate/delete` | `remove` (a scenario's index) | Removes a scenario |
| `POST /competitions/admin/{token}/signoff` | `on` (`1`/`0`) | Whether entries need a coach's sign-off (also `signoff=1` on `POST /competitions`) |
| `GET`, `POST /competitions/signoff/{token}` | `coach` (their name), `signed`, `note` | An individual's coach signing off their entry |
| `GET`, `POST /competitions/signoff/{token}/late` | `coach` (their name), `note` | An individual's coach signing off their late change |
| `POST /competitions/entry/{token}/coach` | `name`, plus a `qualification` and its `certificate` (multipart) where the competition approves coaches | An individual names their coach, with the certificate the organiser checks |
| `GET /competitions/admin/{token}/coaches` | optional `notice`, `undo` (a coach's anchor, after a withdrawal) | Coaches (ADR 0007): each club's coaches sent for approval and individuals' coaches, with their qualifications and certificates, to approve or not |
| `POST /competitions/admin/{token}/approve-coaches` | `on` (`1`/`0`), `level-<discipline>`, `anyway=1` | Whether only approved coaches' sign-offs count. Turning it on where entries are signed off by coaches not approved shows **Before you change it** (200), with how many would wait, unless `anyway=1` |
| `POST /competitions/admin/{token}/coaches/{coach}` | `action` (`approve`, `afresh` for sign-offs from now on, `refuse`, `withdraw`), `note`, `anyway=1` | The organiser's decision on a club's coach, with a note the club sees; back at the coach (`#coach-<id>`). Approving a coach sent with no qualifications shows Coaches asking first (200) unless `anyway=1`. A withdrawal comes back with `undo=coach-<id>`, its notice offering **Undo** (approving again, with their sign-offs before) |
| `GET /competitions/admin/{token}/coaches/{coach}/certificates/{q}` | — | A club coach's certificate, as uploaded |
| `POST /competitions/admin/{token}/entry-coaches/{entry}` | `action` (as above), `note` | The organiser's decision on an individual's coach; back at the coach (`#named-<entry>`), a withdrawal with `undo=named-<entry>` |
| `GET /competitions/admin/{token}/entry-coaches/{entry}/certificate` | — | An individual's coach's certificate |
| `POST /competitions/admin/{token}/remove` | `entry` (repeated), `action` (`hold` to hold rather than remove), `note`, `to` (who the note is for: the club, the member or both) | Removes or holds the ticked entries, with a note; back to the dashboard with the filters and `sort` in its query |
| `POST /competitions/admin/{token}/entries/{id}/restore` | — | Puts a removed or held entry back |
| `POST /clubs/admin/{token}/competitions/{id}/coaches` | `coach` (ids, repeated) | Sends the ticked coaches to the competition for approval; back to its Coaches tab |
| `POST /clubs/admin/{token}/coaches/{coach}/qualifications` | `qualification` (its key), `certificate` (a file, multipart, up to 3 MB) | Adds a qualification with its certificate to a coach |
| `POST /clubs/admin/{token}/coaches/{coach}/qualifications/{q}/remove` | — | Removes a coach's qualification and its certificate |
| `GET /clubs/admin/{token}/coaches/{coach}/qualifications/{q}/certificate` | — | A coach's certificate, for the comp sec |
| `POST /clubs/admin/{token}/coaches/{coach}/signs-off` | `on` (`1`/`""`) | Whether the club lets that coach sign off routines |
| `GET /clubs/member/{token}/competitions/{id}/timeline`, `/competitions/entry/{token}/timeline` | — | A person's own timeline: their flights and duties in time order (once published) |
| `GET /clubs/member/{token}/competitions/{id}/club-timeline`, `/clubs/admin/{token}/competitions/{id}/timeline`, `/clubs/coach/{token}/competitions/{id}/timeline` | — | The club's timeline: every flight a club's gymnast competes in and every seat its people hold |
| `GET`, `POST /clubs/coach/{token}/late/{id}` | `note` | A club's coach signing off a member's late change |

`requirementSet` is either `builtin:<id>` or the custom requirements as JSON.
If the requirements can't be used, the problem is reported in the rendered
check rather than failing the request.

# Limits and hardening

- Request bodies are capped at **64 KB** (`http.MaxBytesReader`), as are headers,
  except the two certificate uploads, which take a 3 MB certificate and the
  form around it (ADR 0007).
  A full routine is a few kilobytes.
- Timeouts: read header 5 s, read 10 s, write 10 s, idle 60 s.
- Every incoming skill is normalised to its phase count and validated (rotation
  0–16, non-negative twists, known shape and positions). Errors are a
  `400` with the message alone, which the page shows to the user as the rest
  of a sentence ("Can't add skill: rotation must be…").
- Official names are always re-derived and never taken from input.
- Every response carries `X-App-Version` ([static assets](static-assets.md#update-notice)).
- Text responses are gzipped when the browser accepts it ([compression](static-assets.md#compression)).
- `PORT` sets the port (default 8080). `DATA_DIR` turns on competition
  storage there (SQLite); unset, it's off. With storage on, competitions 120
  days past their date and clubs unused for 13 months (ADR 0011) are deleted
  at start-up and every 6 hours.
