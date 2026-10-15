---
type: Feature
title: Competition entries
description: What organisers, club competition secretaries, members, coaches and gymnasts entering on their own see, from entering a competition to the day itself — entries and their problems, fees, limits, late changes, officials, the timetable and rota, helpers' links, notifications, one link for several roles, and on the day check-in, who's up, desk messages and the venue screen.
tags: [feature, competitions, clubs, coaches]
generated: { by: claude-code/cli, at: 2026-10-06T11:00:00Z }
sources:
  - id: adr-0004
    resource: ../../docs/adr/0004-server-storage-secret-links.md
    title: ADR 0004 — Server storage with secret links, no accounts
    author: human:jackpardy
---

**Live on tariff.pardy.ie since 2026-10-06**, at `/competitions`; the
calculator doesn't link to it yet. This is card collection, the first item on
the [roadmap](../roadmap.md). Storage, links, clubs, video proof and coach
sign-off are decided in ADR 0004.[^adr-0004]

It's built from storage (`store`), checking (`competitions`), and the pages:
the routes, an entry's card and the entry form (`comppages.go`); creating a
competition and its settings (`compsettings.go`); the organiser's dashboard
and each entry (`dashboardpages.go`); individual entry and sign-off
(`individualpages.go`); clubs, members and sending (`clubpages.go`); and
coaches (`coachpages.go`), with shared helpers in `helpers.go`
([routes](../architecture/http-routes.md#competition-pages)). Their views
are `views/competitions.templ` (the layout and shared parts),
`dashboard.templ`, `entry.templ`, `cards.templ` and `partner.templ`.
`/competitions` lists the links a browser has. The pages are on only where
`DATA_DIR` is set ([competition storage](../operations/deploy.md#competition-storage)).

**Contents**

- [The organiser (competition admin link)](#the-organiser-competition-admin-link)
    - [The menu and the Settings page (roadmap 2026-10-14)](#the-menu-and-the-settings-page-roadmap-2026-10-14)
    - [All people (roadmap 2026-10-14)](#all-people-roadmap-2026-10-14)
    - [Helpers' links, history and concerns (ADR 0009)](#helpers-links-history-and-concerns-adr-0009)
    - [Feedback about the app (ready for the dry run)](#feedback-about-the-app-ready-for-the-dry-run)
    - [When entries, changes and sign-offs close](#when-entries-changes-and-sign-offs-close)
    - [Late changes](#late-changes)
    - [Limits and waiting lists](#limits-and-waiting-lists)
    - [Entry fees](#entry-fees)
    - [Going live](#going-live)
    - [Dashboard](#dashboard)
    - [One entry](#one-entry)
    - [Printing and export](#printing-and-export)
    - [Removing and holding entries](#removing-and-holding-entries)
    - [Video proof](#video-proof)
- [Events and synchro (ADR 0005, step 2)](#events-and-synchro-adr-0005-step-2)
- [Searching lists of people (roadmap item 6)](#searching-lists-of-people-roadmap-item-6)
- [Officials (ADR 0005, step 3)](#officials-adr-0005-step-3)
- [Officials rota (ADR 0005, step 5)](#officials-rota-adr-0005-step-5)
- [Timetable (ADR 0005, step 4)](#timetable-adr-0005-step-4)
    - [What if there's a delay](#what-if-theres-a-delay)
    - [What if an official has to leave](#what-if-an-official-has-to-leave)
    - [On the day: flights started and finished](#on-the-day-flights-started-and-finished)
    - [The venue screen](#the-venue-screen)
    - [Messages from the organisers' desk](#messages-from-the-organisers-desk)
    - [Simulation (ADR 0005, step 6)](#simulation-adr-0005-step-6)
- [Coaches (coach link)](#coaches-coach-link)
- [The club's comp sec (club admin link)](#the-clubs-comp-sec-club-admin-link)
- [A member (personal link)](#a-member-personal-link)
- [One link, several roles (ADR 0010)](#one-link-several-roles-adr-0010)
    - [Entering on your own: one page](#entering-on-your-own-one-page)
- [My competition (roadmap: competitions 5)](#my-competition-roadmap-competitions-5)
    - [Personal and club timetables](#personal-and-club-timetables)
- [Notifications (ADR 0008)](#notifications-adr-0008)
    - [Offline](#offline)
- [Built from](#built-from)

# The organiser (competition admin link)

The tariff sheet is only one of the organiser's views: the printable one. The
organiser's first need is an **overview of every entry with its problems**.

## The menu and the Settings page (roadmap 2026-10-14)

Every page of the organiser's section has a **menu** under its title (the
shared `pageMenu` in `views/menu.templ`, built by `orgMenu` in
`web/menus.go`): **Entries** (Entries, People, Late changes, Coaches when coaches are
approved, Fees), **Timetable** (Timetable, Officials, Simulate, and once the
timetable is planned the two "what if" pages), **On the day** (once the
timetable is published: On the day, Desk messages, Venue screen) and
**Settings** (Settings, History, and Feedback for links that may read it). Late changes, Coaches and Feedback show a count of
what waits; the menu works that out on every page. It shows only what the
admin link may open (ADR 0009): a cards link sees Entries (with People) and History; a
chair of judges adds the day's pages, and finds the printed sheets as buttons
on the entries page; the timetable team adds Timetable and Officials. The
menu is left off printouts, the CSV, invoices, cards and the venue screen. The
bar scrolls sideways on a phone.

**Settings** (`GET /competitions/admin/{token}/settings`, in `web/settingspages.go`)
holds what used to fold away on the entries page, in groups with a list of
jump links at the top: Links (helpers' links for the organiser, the club
link, the link for entering on their own), Entries and deadlines, Limits and waiting
lists, Events and levels (events, order of levels, men and women), Sign-off
and coaches, Video, Going live, Telling people about changes (when people
can ask to hear), Entering on their own, Feedback, and Admin link and deleting (the
organiser only). A co-organiser's link opens it without the links box,
replacing the admin link or deleting. Each form posts to its own route, which
comes back to the Settings page with a notice; the history records them as
before. The entries page keeps the **Go live now** box while the competition
is private.

## All people (roadmap 2026-10-14)

**People** (under Entries in the menu; `GET /competitions/admin/{token}/people`,
`web/peoplepages.go`) lists everyone at the competition once, by name, under a
count ("441 people"). A person is one **person key**, whatever they do: a
member is `m:<member id>` whether they compete, judge or both; a gymnast
entering on their own is `i:<id>` for the person, kept on each entry (entries
from before [one link a person](#one-link-several-roles-adr-0010) keep
`i:<lower-cased name>`); a synchro partner nobody has
linked is `p:<entry id>`; an official the organiser added is `o:<official
id>`; a club's coach is `c:<coach id>`, the coach who signed the entry off
or else the member's coach, so renaming them or a sign-off typed
differently is still them (keys saved under the old `c:<club id>:<name>`
moved at migration 34); a coach an individual named, who has no record, is
`c::<lower-cased name>`. Keys are made and read only in
`store/personkey.go`. The list is built from the live entries (not withdrawn, not
on the waiting list), the officials and the coaches (those entries name, those a
club sent and those an individual named), so a coach with no entries yet is
there too. Keys of one person (a member who is a coach too, or who also
enters on their own) are one row and one page, with all their roles
([one link, several roles](#one-link-several-roles-adr-0010)); the page is at
the first key found, and another of their keys redirects to it.

Each row has the name (a link), the club ("On their own" for someone entering
without a club, nothing for an official with no club) and **what they do**: the
events they compete in ("Trampoline BUCS L7, Synchro BUCS L6/L7"), what they
offered ("judge, can chair, recorder, marshal") and "coaches 6" (the gymnasts
they coach). The list can be searched by name and filtered by club and by what
they do (gymnast, official, coach; judging, chairing, recording or marshalling
makes an official), from the same pieces as the Officials list
([lists of people](../architecture/rendering.md#lists-of-people-search-and-filters)).
Its rows carry the shared row attributes, with the roles from `dirPerson.roles`
as the "does" values; the list is `id="people"`.

Each name opens `GET /competitions/admin/{token}/people/{key}`. The key is
escaped as one path segment (`url.PathEscape`, which leaves the colons and
escapes a slash, a question mark, a percent sign and a hash in a name); Go's
mux matches the escaped path and gives the handler the key unescaped. An
unknown key is a 404 ("Person not found"). The page is read-only and is the
person's own **My competition** page (`views.MyCompetition`) built by the
same code (`dayOf` in `web/mypages.go`, which the member's page and this one
both call), with the parts that are only for the person left out (notify, the
calendar, "I'm here for the rest", the score sheets link) and these added:

- **Who they are**: name, club, and a tag for each role.
- **Entries**: each event with its card status and a link to the entry, who
  coaches it (linking to the coach's page), and once the timetable is
  published their flight, place, rough times and check-in (here, scratched or
  not marked yet).
- **Officiating**: what they offered and who sent it, which events they may
  judge under the judging rule (and can chair), whether they're marked
  qualified where the rule asks, their seats (the published timetable's, or
  the draft's once planned) and whether they're here for their panels.
- **Coaching**: the entries they coach, each gymnast linking to their page,
  and where the competition approves coaches, how the coach stands.
- Scratch warnings (a scratched gymnast still down to officiate or compete),
  and the desk messages that reached them (to them, to their club, and for a
  coach, to the club's coaches and the gymnasts they coach).
- The timetable with them picked out, for anyone competing or on a panel.

Names link there from the dashboard's entry tables (a small "person" link
beside the name, and "partner" for a pair, since the name already opens the
entry), the Officials page's People table (the name) and the Coaches page (the
name). Any admin link that can see the dashboard can open the list and the
pages (`allowed` in `web/access.go`) except the venue screen link.

## Helpers' links, history and concerns (ADR 0009)

On the Settings page the organiser makes **links for helpers**, each
named (who it's for) and of a kind: **checking cards** (see the entries,
mark cards checked with notes, review videos), **chairs of judges** (see
the entries, print the chair of judges, score and marshal sheets and the
panel timeline), **timetable and officials** (plan, change and publish
them, Notify now, see the entries), **venue screen** (see
[the venue screen](#the-venue-screen); it can open that page and nothing
else, not even the dashboard or flagging a concern) and **everything but
links and deleting** (a co-organiser). Each is a secret link, kept as a hash and
shown once; **Remove** stops it. Up to 20. Every admin route goes through
one gate (`allowed` in `access.go`); pages show only what the link can do,
and say whose link it is.

**History** (`/history`) lists every change made through any admin link,
latest first: when, which link (its name, "Organiser" for the admin link)
and what ("Marked a card checked: Ann Ryan · BUCS L3"); the latest 2,000
are kept.

**Concerns:** anyone with an admin link can **flag a concern**, on an entry
or generally. The dashboard lists them (open first, with who and when), for
the organisers only; the organiser and co-organisers mark each
**Resolved**, with a note. The demo has helpers' links (printed by `demo`),
three cards checked and two concerns.

## Feedback about the app (ready for the dry run)

For the dry run, everyone using a competition's pages can tell the people
who make the app what confused them, at the moment it does. A small
**Feedback** dropdown sits at the foot of every page someone uses for one
competition: the organiser's and helpers' pages, the comp sec's pages for a
member's entry, offers, late changes, notifications and the club's
timetable, the club link's entering page, a member's and coach's pages for
the competition, an individual's entry pages and My competition, entering on
their own, the synchro partner's page and the individual's coach's sign-off.
Not the venue screen, printouts, cards, downloads, or pages about several
competitions (a club's, member's, coach's or individual's home page, and a
coach's late change). It opens a short form: what happened or what they
think (required, up to 2,000 characters) and, optionally, a name or email
for a reply (up to 200). `static/js/feedback.js` sends it without leaving
the page and says "Thanks: your feedback is sent." there; without
JavaScript the page comes back at `#feedback-sent`, which shows the thanks.

What's kept (migration 40, the `feedback` table, in `store/feedback.go`):
the competition, the words, the reply-to, the role (Organiser, "Helper
link: Chairs (chairs of judges)", Comp sec, Member, Coach, Individual,
Synchro partner or An individual's coach, from the link it's posted under,
never from the form), the page as its **route pattern**
(`/clubs/member/{token}/competitions/{id}/day`, only one of those with the
button; never the path, which holds the secret link) and the time. No
token is kept. A competition takes 200 in any 24 hours; empty or overlong
feedback is refused with a plain reason.

How it works (`web/feedback.go`): `feedbackPages` lists the pages with the
button by pattern. For those, the request gets an empty
`views.FeedbackForm`; the loader that finds the page's competition
(`gate` for the admin pages, `enteredCompetition`, `own`, `individual`,
`clubLinkCompetition`, `invite`, `signoffEntry`, `gymnastEntered`,
`coachSees`) fills in the competition and whether it shows the button, and
the layout (`competitionLayout`) draws it. So no page loads the competition
twice for it. The form posts to its link's own `/feedback` route; for a
club's, member's, coach's or individual's link the posted competition must
be one they're entered in.

The organiser turns it off or on in **Settings** under Feedback ("Show a
Feedback button on every page"); it's on for new competitions and, from the
migration, for those already made. The **Feedback** page (in the Settings
section of the menu, with a count of those not dealt with) lists it newest
first: when, role, page, the words and the reply-to, with a **Dealt with**
tick. Like resolving concerns, reading it needs the organiser's or a
co-organiser's link; anyone with an admin link but the venue screen's can
send it. Sending, marking and turning it on or off are in the history.

With `FEEDBACK_EMAIL` set and email set up ([deploy](../operations/deploy.md#notifications)),
each is emailed there too, with a `Reply-To` when the reply-to is an email
address, sent in the background (`emailFeedback`, one call to move onto the
background sender). The demo has two, from a member's My competition and the
chairs of judges.

## When entries, changes and sign-offs close

Three times (roadmap 2026-10-08), set on the Settings page: **entries
close** (the deadline: no new entries after it), **changes to entries
close** (changing, withdrawing or sending again entries already made,
offers to officiate) and **coaches' sign-off closes** (signing off, naming
a coach for approval). The last two can be later than the deadline, by the
end of the competition date; empty, they close with it
(`Competition.ChangesClose`, `SignoffsClose`; `store.open` checks each
change against its own). After the deadline a club's **Send** sends only
entries the competition already has. A coach's sign-off now goes straight
onto the competition's copy too, where it's the entry the coach signed, so
it doesn't need sending again. The pages say "New entries closed …
Changes to entries until … Coaches' sign-off until …". Late changes open
once changes close.

## Late changes

Once changes to entries close (the competition live), clubs and
individuals can ask for the changes the organiser allows (roadmap
2026-10-08). On **Late changes** (under Entries in the menu, which counts
those to decide) the organiser ticks which kinds can be asked for, each
with a fee charged only if accepted: a **level change** (a different
level, with its routines) or a **routine change** (new routines at the
same level). A member ("Ask for a late change" under each event), their
comp sec (on the club page's row) or an individual (on their entry) fills
in the entry form as it should be, with a note; the entry stays as it is
until decided, and a new request replaces one still waiting.

**Coach sign-off.** Where entries need a coach's sign-off, the organiser
can tick **A coach must sign off a late change before it comes to you**:
the request then waits for a coach (a member's coach, on their coach page
under "Late changes to sign off"; an individual's, through the sign-off
link), who sees the new card and signs it off, approved where the
competition approves coaches; then it reaches the organiser. Without it, a
coach can still sign off a request while it waits. Accepting carries the
request's sign-off onto the entry (both copies); one not signed off leaves
the entry needing sign-off. The checks say which.

Each waiting request shows the organiser whether it can be fitted in
(`lateChecks`): the new card's problems; whether the new level is full,
and their place on its waiting list if so; and in the draft timetable,
the flight of the new level they'd join (the one with fewest entries, at
the end of its running order; `Schedule.PlaceChange`), how much later it
finishes, how many later flights move, and anything it would break (a
clash, a day running over). **Accept** (with an optional note) replaces
the entry on the competition's copy and the club's, needing checking and
signing off again; a level change joins the new level's waiting list if
it's full, or moves into that flight in the draft timetable, to publish;
and the fee goes on the club's or individual's invoice. **Not accepted**
needs a reason, which whoever asked sees. Both show on their pages, and
notifications (cards) tell them.

## Limits and waiting lists

On the Settings page, **Limits and waiting lists** sets how many
entries each event takes (empty: no limit), showing how many have entered
(roadmap 2026-10-08). Once an event is full, later entries wait, in the
order they first came in (`entered_at`: kept when an entry is sent or
changed again, reset if its level changes, so switching from a quiet
level doesn't jump the queue). `Store.Entries` marks each entry's place
(`Waiting`, from `markWaiting`); withdrawn entries take no place. A waiting
entry isn't counted, charged, printed, exported or timetabled (`live`,
`splitWaiting`). When one is withdrawn or removed, or the limit is raised,
the next moves up by itself; **Let in** takes one in over the limit
without moving anyone else. The dashboard shows "88 of 90 places" beside a
limited event and a **Waiting lists** box; the comp sec sees "Sent ·
waiting list, 3rd", and a member or individual "On the waiting list for
BUCS L1: 3rd". Notifications (cards) say when an entry goes on or comes
off a waiting list, when the organiser's change does it.

Every page says where an entry stands in the same order (`stateOf` in
`web/entrystate.go`), the comp sec's: removed, on hold, not sent, on a
waiting list (unless changed since), changed since sent, checked, sent.
Each page words it for its reader. So a checked entry on a waiting list
reads as waiting everywhere: a member's status line says "Sent by your
club", not "checked by the organiser". The demo limits
BUCS L1 to 12 and DMT Advanced to 17.

## Entry fees

**Fees** (under Entries in the menu, for the organiser and co-organisers;
roadmap 2026-10-08) sets what's charged: euro or sterling, an amount per
entry for each discipline offered (a synchro pair is one entry), an
optional fee once per club, and how to pay (printed on every invoice).
Removed, held and withdrawn entries aren't charged (`Fees.Invoice`). The
page lists each club, and each individual's entry, with entries, due, paid
and balance ("Paid" once settled), and totals; the organiser records each
payment (amount and note, with who recorded it) and can remove one made by
mistake. Each has an **invoice** to print: a line per entry and the club
fee, payments, what's left and how to pay. A club's comp sec sees "Fees:
€49 · €25 paid · €24 to pay" on each competition, with the invoice; an
individual on their entry. No money goes through the tools. The demo
charges €12 an entry (€15 synchro, €10 tumbling and DMT) and €25 a club,
with some clubs paid and some part paid.

## Going live

A new competition starts **private** (roadmap 2026-10-07), unless the form
says to open entries now or at a set time. While it's private the organiser
sets it up; the club and individual links already show its name, date and
when entries open ("Entries open Monday 1 February 2027, 09:00", or
"Entries aren't open at the moment"), so they can be shared early, but
nothing can be entered, sent, changed, withdrawn or signed off
(`store.ErrNotOpen`). The dashboard's **Go live now** opens entries, or a
date and time opens them then (before the deadline); **Make private (pause
entries)**, on the Settings page, stops them again, keeping the entries
already made. Entries are open while the competition is live and before
its deadline (`Competition.Open`); the time is `live_at`, empty while
private. Competitions made before this went live when they were made.

## Dashboard

There is one table per level (event), each folding away under its heading,
which says how many are shown ("12 of 96"); a list at the top jumps to each.
Levels start folded, with **Expand all** and **Collapse all**; the browser
remembers which were left open, and a search or filter opens the levels it
finds people in. Finding entries (roadmap 2026-10-08), all in
the page's address so a view can be kept or shared:

- **search** (`q`): every word in the gymnast's or synchro partner's name,
  ignoring case, accents and punctuation (`competitions.NameMatches`);
- **filters**: club, coach (who signed off, else the member's coach; or
  none), checked, signed off, video (missing, to review, need more, OK),
  men or women, problems only;
- **sort**: by club (as stored), gymnast, latest sent or coach;
- **print and CSV what's shown**: one set of buttons (Print cards, Print
  cards not checked, Download CSV) above the levels; Print cards and the CSV
  take the same filters, and each open level has **Print this level's
  cards**.

| Gymnast | Club | 1st exercise | 2nd exercise | Requirements | Problems | Video | Sent | Checked |
|---|---|---|---|---|---|---|---|---|
| A. Murphy | UCD | Set 1 | 4.6 | 5/5 ✓ | – | not needed | 3 Mar, 21:04 | ✓ |
| B. Kelly | DCU | Set 2 | 5.9 | 4/5 ✗ | Repeat (element 7) | missing | 4 Mar (changed since) | – |
| C. Ryan | On their own | 2.1 | 6.2 | 9/10 ✗ | Element 10 must land on feet. | provided | 5 Mar | – |

- Counts at the top: entries per level and club, how many have problems, how
  many aren't checked yet, and any late ones.
- Each club's rows show when the club last sent its entries.
- Every column comes from the same checks as the
  [routine builder](routine-builder.md): [routine validation](../domain/routine-validation.md),
  [requirements](../requirements/framework.md) and the
  [level](../requirements/levels.md) carry-over between exercises.

## States and roles on the pages (one palette, step 2)

The pages the dry run leans on show each state with `views.StateChip` and
who did something, or whose it is, with `views.WhoChip`
([one palette](../architecture/rendering.md#states-and-roles-one-palette)),
and no Bulma colour for a state (buttons keep theirs):

| Page | States | Roles |
|---|---|---|
| On the day | each area's lateness (amber late, green on time or early, grey not started or not measured); each flight not started, under way, running late (under way in an area running late) or finished; each gymnast not yet, Here or Scratched, and each official not yet, Here or Missing; the Check-in and Panel headings count who's here (green once everyone is here or out) and out; a panel's seat no one is in, "Needs someone" | |
| Dashboard | Checked; problems (amber, with how many); video missing, provided, OK or need more; sign-off signed off, waiting, needs changes or coach not approved; Withdrawn; and under Removed and on hold, Removed, On hold, and changed and sent again | the coach who signed off; an individual's entry, "Gymnast · on their own" |
| One entry | Checked; the video review | |
| The comp sec's entries | not sent grey, sent or on a waiting list blue, changed since sent or on hold amber, checked green, withdrawn or removed red (sent again after a hold, blue); problems and missing video amber; the sign-off | the coach who signed off |
| A member's or individual's disciplines, and their competitions list | the same states in a word or two; problems; timetable published; a message from the organisers | |
| The rota (timetable) and Officials | a seat with no one; every seat filled; everything fits; a gymnast moved since | who fills a seat or offered: a gymnast if they compete, else the organiser's own (added on the Officials page) or a club's judge (`seatRole`) |

On the day, a panel now lists its empty seats too, once anyone has a seat on
it (`panelOf`); the counts are worked out on the page from the rows
(`checkinCount`, `seatCount`). `views/status_test.go` fails if these
templates show a state in Bulma's colours again.

## One entry

Both exercises are shown side by side, as in the builder's Levels mode: the
elements, FIG notation, tariff, flags (repeats, transitions, interruptions,
carry-over) and each requirement with ✓ or ✗. Two buttons follow.

An element's name is the catalogue's, or the coach's own words; one the
catalogue has no name for is left blank, as its FIG notation beside it says
what it is (never "Custom Skill"). A * marks an element that meets one of the
requirements, explained under the table. A set routine performed as written
names each element in the set's own words ("Seat landing"), as its Set
routine line does. The validation's flags read as sentences
(`skills.ElementNotes`): "Element 10 must land on feet.", "Element 4 takes
off from front, but element 3 lands on back.", on the card's rows and in the
problem lists gymnasts, comp secs, coaches and the organiser see. The buttons:

- **Mark checked**, with an optional note. The note is shown to the club's comp
  sec and the member.
- **Print card.**

If the competition asks for video, the entry also lists the skills that need
it, the member's link for each exercise (opens in a new tab) with its note,
and **Video OK** or **Need more** (with a note back to the club).

The organiser never edits a routine. A problem goes back to the club as a note.
The club fixes it and sends it again.

An entry the club sent for a member who has since withdrawn it, or left the
club, is marked **Withdrawn** and isn't counted or printed. It goes when the
club sends everyone's entries again: the organiser only sees what the club
sends (ADR 0004 Decision 2).

## Printing and export

- The [tariff sheet](tariff-sheet.md) (competition card) with its details
  filled in from the entry: gymnast, club, level, competition and exercise.
- **Print all** for a level, a club, or the entries not checked yet: one card
  per page, ready for the judges' table.
- **CSV** for the scoring system (name, club, level, tariff per exercise). A
  single PDF of all cards is the cards page saved as PDF from the browser's
  print dialog.

## Removing and holding entries

Each row of the dashboard has a tick box (and each level's header ticks the
whole table, so filtering by club takes a whole club). Under **Remove or
hold entries** the organiser chooses:

- **Remove**: the entry is taken out and can't be sent or changed again
  (a club's Send skips it; an individual's page can't change it) until the
  organiser **restores** it.
- **Hold, asking for changes**: taken out until the club or gymnast changes
  it and sends it again; it then shows "Changed and sent again: waiting for
  you", and **Accept back in** returns it.

Either way there's an optional reason, shown to the club, the gymnast or
both (an individual always sees theirs). Removed and held entries are left
out of `Store.Entries`, so out of every count, card, CSV, timetable, rota
and "My competition", and listed at the end of the dashboard under
**Removed and on hold**, to restore or accept. The club page and the
member's and individual's pages say what the organiser did ("Removed by
the organiser", "On hold: the organiser asks for changes", "On hold:
changed, send it again", "Changed and sent again: waiting for the
organiser") with the reason where it's for them.

## Video proof

When creating the competition, the organiser chooses whether video proof is
needed: **none**, **for some skills** (described like a requirement, e.g. "any
triple" or "tariff 1.5 or more") or **whole routine**. Videos are links (an
unlisted YouTube video, a Drive file shared with anyone with the link, Vimeo,
Dropbox or OneDrive), never uploaded, and never played inside the app (ADR
0004 Decision 10).[^adr-0004]

# Events and synchro (ADR 0005, step 2)

A competition offers **events**: a discipline at a level.

- **Trampoline** levels as before; **synchro** levels, where a pair does one
  routine, the level's voluntary (its second exercise, or its only one),
  checked like a trampoline routine; and **tumbling** and **DMT**
  levels the organiser names, which are a level only, not checked yet. The
  organiser picks them when creating the competition and can change synchro,
  tumbling and DMT on the Settings page.
- A gymnast can enter **several events**: a member's competition page has a row per
  discipline (enter, change, withdraw each on its own), the club page and
  dashboard list each entry under its event ("Synchro BUCS L3", "Tumbling
  Novice"), and an individual enters each discipline separately.
- **Synchro pairs** can be any two gymnasts. The entry names the partner and
  shows a **partner link** to send them; the partner confirms who they are
  with their own member page or entry link (or as having no other entry), so
  the timetable can later check their clashes. Where links are saved in their
  browser, the page offers them, starting on "Choose who you are" and leaving
  out the synchro entry itself (saved under the pair's names, which the page
  gives as `data-saved-self`). Naming a different partner
  needs a new confirmation. Once confirmed, the pair's entry shows under
  **Synchro as a partner** on the partner's page (member or individual),
  their coach's (unless they already see the entrant) and their club's comp
  sec's (unless the entrant is their own member), whether or not that club
  is entered (`PairEntries`): as checked, with its sign-off, sent or not,
  and once published, the flight. Only the entrant's side changes it. Both
  gymnasts' entries carry the pair-level warning the dashboard gives
  ("… compete individually, so as a pair they do BUCS L4 in synchro").
- **Paired synchro levels:** the organiser can pair synchro levels into one
  event, one a line ("BUCS L1 + BUCS L2" makes "Synchro BUCS L1/L2"),
  flighted, ranked, judged and timetabled as one. Each pair says which level
  they're doing ("BUCS L1/L2 (doing BUCS L1)", kept as the entry's
  `choice`), and their routine is checked against it. Pairings are set when
  creating the competition or on the Settings page; entries made before
  a level was paired need re-entering in its event.
- **A pair's level:** a pair does the level they both compete at
  individually, or the easier of two a level apart (by the competition's
  order of levels); more than a level apart, they usually can't pair. Once
  the partner has confirmed and both have individual trampoline entries, a
  synchro entry at another level is flagged on the dashboard, first among its
  problems ("A (BUCS L3) and B (BUCS L4) compete individually, so as a pair
  they do BUCS L4 in synchro, not BUCS L3").

# Searching lists of people (roadmap item 6)

Every list of people has the same search and filters, from one set of shared
pieces ([how](../architecture/rendering.md#lists-of-people-search-and-filters)).
"Find a name" looks at a name's letters only, ignoring case, accents, spaces
and punctuation, the same in the browser as on the server, so "marykate"
finds Mary Kate and "oneill" finds O'Néill; the count says how many are
shown, and Clear shows everyone again. The lists are the dashboard's
entries, All people, Officials, the organiser's Coaches, the desk's People I
pick, the club's Members, Coaches and each competition's entries, a
coach's own entries, and the names on the timetable and On the day. On the dashboard the filters work on the server, so its
one set of Print cards and Download CSV buttons follows them (a level's own
**Print this level's cards** button is inside the open level), and the search
also narrows the entries as it's typed and opens the levels it finds them in.

Names are listed in the order people expect, ignoring case and accents
(`competitions.CompareNames`): Aberdeen before ATU Sligo, DCU before DkIT,
Éabha before Tadhg. The store sorts members, coaches, officials, clubs and
entries this way after reading them, and the pages' own sorts (club filters,
the dashboard's sort, people) use the same compare.

# Officials (ADR 0005, step 3)

**Officials** (under Timetable in the menu) lists everyone who can judge or help, and what
each discipline's panel needs. The People list can be searched by name and
filtered by club, what they can do (judge, chair, recorder, marshal), the
discipline and level they can judge (a level shows everyone who judges it or
a harder one), and qualified or not. The search and filters stay while the tab
is open, so ticking someone qualified keeps the list as it was
([lists of people](../architecture/rendering.md#lists-of-people-search-and-filters)).

- **Offers.** On their page, a member says what they can do at each
  competition: judge a discipline (up to a level, perhaps chairing), and help
  as a recorder or marshal. The comp sec can correct it, and it goes to the
  competition with every send. An individual offers on their entry.
- **People the organiser adds:** judges or helpers who aren't entering, with
  or without a club.
- **Panels:** each discipline's starts as the FIG Code of Points' (a Chair of
  Judges Panel, 6 execution and 2 difficulty judges), plus a recorder and a
  marshal, all changeable. Trampoline and synchro can add HD (horizontal
  displacement) judges where no machine measures it; none by default. Synchro starts
  with 2 synchronisation judges (the Code's §18.1 leaves synchronisation to
  a machine, which student competitions don't have), any of its judges able
  to take the seat; set 0 where a machine measures it.
- **Who may judge what:** anyone up to the level they say (the default), only
  levels below the one they compete at, or only people the organiser marks
  qualified. Both go by the **order of levels**, easiest first: built-in
  levels by their rank (BUCS lists its levels hardest first, so they're
  turned round), a coach's own levels after them, and tumbling and DMT's as
  typed. The organiser moves any level up or down on the Settings page;
  competitions from before the order (migration 8) are put in order when read
  until a level is moved. The page shows how many can judge and chair each discipline.
- The [officials rota](#officials-rota-adr-0005-step-5) puts them on panels
  around their own turns.

# Officials rota (ADR 0005, step 5)

Planning the timetable also fills each event's panel from the people who've
offered (roadmap: competitions 4): once for all of its flights, so everyone
in it is judged by the same people ([ADR 0006](../../docs/adr/0006-one-panel-per-event.md)).
A seat goes to someone free for the whole event. The organiser can let
**recorders change** or **marshals change** between an event's flights
(each on its own, both off by default, on the officials page); those seats
are then filled flight by flight.

- **Never in two places:** no one officiates while competing, or on two
  panels at once. Each seat goes only to someone who may take it: judges
  under the judging rule and up to their level, chairs who said they can
  chair, recorders and marshals who offered.
- **As far as it can:** clubs share judging their own gymnasts fairly (each
  time costs a club more than the last), the work is spread, a panel stays
  together on its area from one event to the next, helpers who could judge
  are kept for judging, and someone about to compete gets rest first.
- **Rules about people,** each a must or a prefer: someone in a role at an
  event ("Mary is Chair of judges at Elite Women"), someone not officiating
  (at an event or at all), someone officiating on a day only between two
  times. A must keeps that person for that seat, off any flight at the same
  time. Changing these rules asks you to **Assign officials again**, not to
  plan again.
- **The report:** seats no one could take, a seat for the whole event it's
  on (counted, with the panels they're on: "11, on 3 panels"), manual changes that break the rota
  (someone officiating while competing, in two places, or in a role they
  can't take), rules not kept, coaches needed on two areas at once, times
  each club judged its own, and the busiest officials.
- **Manually:** give any seat to anyone who's offered, or empty it; the change
  goes on the event's other flights the same person had that seat for.
  **Assign officials again** starts the rota afresh, keeping the flights.
- **Coaches:** planning keeps a coach's gymnasts off two areas at once where
  that can be done (by club and coach name), and reports where it can't.
- **Printed:** each flight's panel on the marshal and chair of judges sheets;
  the recorders' **score sheets** (asked 2026-10-08, as ISTO records scores
  online and on paper): a landscape page a flight, its panel named, a row
  per gymnast (or pair) in running order, and for each routine a column per
  execution judge, D, each HD and synchronisation judge the panel has,
  penalty and total, then the total and place, all left to fill in. The
  organiser prints them; an official on a flight's panel can also open that
  flight's sheet on their phone (**Score sheets for your panels**, beside
  their duties, once published), which goes back to My competition, for a
  member and someone entered on their own alike. And the **officials rota**, each person's
  duties in time order, one line
  for an event ("Friday 09:30–15:15 · Panel 2 · BUCS L7 Men · all 5
  flights · Execution judge").
- **Published,** members and gymnasts entering on their own see their duties
  on their page.
- **Flights where judges are free:** planning prefers times when enough of
  an event's judges aren't competing to fill its panel, for each flight and
  for the whole event, counting both the flights being placed and those
  already beside them.

# Timetable (ADR 0005, step 4)

**Timetable**, in the menu, plans the competition over the venue's days
(roadmap: competitions 3).

- **Days:** each with a name, start and strict end time, and the areas in use
  that day. A new competition starts with one day, 09:00–18:00.
- **Areas:** named panels, tracks and DMT beds, each for one discipline;
  synchro runs on trampoline areas too. A new competition starts with one area
  per discipline it offers ("Panel 1", "Track 1", "DMT 1").
- **Timings**, per discipline: minutes per competitor (5 for trampoline, 5 per
  synchro pair, 2 for tumbling and DMT), minutes between flights (10) and the
  largest flight. **Rest** between a person's flights (20 minutes), from the
  end of one to the start of the next one's warm-up, is a preference, or a
  must. Both rounds of an event are in one flight, so rest is only between
  events. **Men and women**: which levels ranked separately
  also fly separately.
- **Blocked time:** lunch, awards or an ad hoc event, on a day, at a fixed time
  or anywhere in a window, on chosen areas or all of them. Blocked time can
  need officials (a chair, judges, a recorder, a marshal): the rota staffs it
  from anyone who judges or helps. **Events can run across breaks** lets an
  event pause for blocked time on its area and finish afterwards; otherwise
  each event is all before a break or all after it (the default).
- **Rules,** each a **must** or a **prefer**: an event on an area, on a day,
  before another event, or apart from another (not at the same time).
- **Plan** splits each event into even flights, draws running orders (clubs
  spread where easy) and places each event's flights as one **run**, back to
  back on one area and day, so one panel can judge them all
  ([ADR 0006](../../docs/adr/0006-one-panel-per-event.md)): never a person
  in two places, every flight inside its day's hours and around the blocks,
  keeping the musts; then as many preferences as it can (rest, the
  organiser's prefers). Flights are numbered in the order they run. It tries
  many orders and keeps the best: events a must ties to a day or area
  first, then the longest; what didn't fit goes first next time; and a
  break with a window (lunch between 12:00 and 14:00) at different times in
  it.
  Where a person's next flight that day starts less than twice the rest after
  their last, they go early in the first flight's running order and late in
  the second's (also when a flight is redrawn).
  A person is a member (by their member link) or, for individuals entering
  several disciplines, the person they entered as (ADR 0010); a confirmed
  synchro partner is the same person as their own entries. Records joined
  as one person ([one link, several roles](#one-link-several-roles-adr-0010))
  are one person here too: a member who also enters on their own, or also
  coaches, gets rest and clash warnings against themselves, and isn't put
  on a panel while they compete under another of their keys. Planning
  avoids someone coaching while they compete (as costly as a broken
  prefer), and the report lists it ("Ann coaches in L2 while competing in
  L1"). The keys saved in seats and rules stay as they were; only the
  working out sees them as one (`samePeople` in `web/personkeys.go`).
- **The report:** for each day, when its flights end, when it finishes
  (blocked time such as awards included) against its end, and the time free
  after the last flight that isn't blocked off for the whole venue (room for
  more flights; the simulation shows it too),
  what didn't fit (flights and blocks), anyone resting less than asked, and
  rules broken, and names that look like one person entered twice (the same
  letters ignoring accents and punctuation, or one letter apart in a long
  name). When something doesn't fit, it tries changes one at a time (another
  area, half a minute less per competitor, fewer minutes between flights,
  bigger flights, rest only preferred, events running across breaks, or an
  event capped at the most entries that fit, a flight's worth fewer at a
  time) and says which would fit everything.
- **Adjusting:** **Move event** moves all of a flight's event to another day
  or area, after what's there (clear of breaks unless events can run across
  them); **Earlier** swaps a flight with the one before it in its event;
  redraw a flight's order, move a gymnast to another flight or take them
  out, and place entries made since planning. On a flight's card these are
  buttons at least 40px tall: **Move** beside each gymnast (its flights
  filled in by `static/js/fill.js` when it opens), **Redraw order** and
  **Earlier**, and a dropdown labelled **Move to** ("Move the event to") with
  **Move** or **Move event**. At the top of the timetable, **Jump to** links
  to each day and its areas and to **Find a name**, which searches the
  running orders, the gymnasts not in a flight and the seated officials,
  hiding days, areas, flights and blocked time with nobody it finds and
  opening the Panel lists with someone. Changing the setup marks the
  plan out of date until **Plan** again; moves are rechecked and listed as
  problems: clashes, and an event whose flights aren't back to back on one
  area, or run either side of a break that isn't allowed (timetables
  planned before ADR 0006 show these until planned again).
- **Sheets:** marshal sheets (running orders to tick off) and chair of judges
  sheets (each gymnast's exercises and problems), one area to a page. On a
  phone a sheet's table (and the rota's) scrolls sideways in its own box,
  and the toolbar wraps, so the page doesn't and **Print / Save as PDF**
  stays on screen; printed, nothing changes.
- **Panel timeline:** time running down at an even scale (a grid row a
  minute), so every area and day lines up by time. Two views:
  - **Panel timeline:** every day side by side, a column per area, each
    flight (warm-up to finish, how many gymnasts) and blocked time. Prints
    on A3 landscape, two pages for the demo's weekend.
  - **Timeline with officials:** a sheet for each day's area, with a column
    for the flight and one for each seat on its panel (Chair, D1, D2, HD1,
    E1…, Rec, Mar: as many of each as any of its flights has), a name to a
    cell ("—" for a seat no one could take). Each sheet runs over its own
    day's hours and prints a page each.

  Flights moved manually onto each other are outlined in red. Both download
  as CSV (`timeline.csv`): a row per flight or block on each area, in time
  order, with each role's officials and their clubs.
- **Publish** shows clubs (each member's row), members and gymnasts entering
  on their own their flight, area, day and warm-up time. The competition
  keeps two timetables (roadmap 2026-10-07): the organiser's **draft**
  (`timetable`), which every change goes into (planning, moves, seats, a
  kept "what if"), and the **published** copy (`published_timetable`),
  which every attendee page reads (placements, duties, "My competition",
  personal and club timetables). With changes not published, the page says
  so: **Publish changes** copies the draft over, **Discard changes** copies
  the published one back, **Unpublish** takes it down. Timetables published
  before this became their own published copy (migration 14).
- **Changes at once** (built 2026-10-15): every change to the draft (setup,
  plan, a move, a redraw, a seat, a rota rule, a kept "what if", an accepted
  late change) reads, changes and saves it in one transaction
  (`store.UpdateTimetable`), so the organiser, a timetable helper and an
  accepted late change build on each other rather than one quietly undoing
  another. The timetable page's forms, and the delay and leave pages'
  **Keep** buttons, also post the draft's version (`version`:
  `Schedule.Version`, a hash of the draft as the page showed it). Forms name
  flights, gymnasts' flights, seats, days, areas, blocks and rules by their
  place in the draft, so one from a page opened before someone else changed
  the draft is refused with "The timetable changed since you opened this
  page; nothing was changed.", as is **Publish changes** (it would publish
  what nobody has looked at). A form without a version (not from these
  pages) isn't checked.
- **Flight IDs** (built 2026-10-15): each flight has an ID of its own
  (`ScheduledFlight.ID`, saved as `id`), given when it's planned (from the
  plan's seed) and kept by every change: moving its event, Earlier,
  redrawing, renumbering, a delay merging another into it. A flight made any
  other way gets a new one when the draft is saved (`Schedule.Identify`).
  Planning again makes new flights, with new IDs. What happens to a flight on
  the day is kept by its ID (`competitions.FlightKey`), so it follows the
  flight when the organiser moves or renumbers it and publishes again.

## What if there's a delay

**What if there's a delay?**, linked from a planned timetable, takes a day,
the areas held up (none for all), from when and for how long, and shows what
follows without saving anything (`Schedule.Delayed`), unless the organiser
taps **Keep this in the draft timetable**, which makes it the draft, to
publish. A flight under way runs
that much later; the area's later flights shift back only as far as they must
(slack takes some of it), going round blocked time (a flight that wouldn't
finish before lunch waits for it to end, and says so). A shifted flight that
would run past the end of the day, or need someone (a gymnast or official) in
two places, moves to the earliest free slot on another area of its
discipline, no earlier than its published time and clear of the delay. An
event's flights stay together (ADR 0006): unless events can run across
breaks, the rest of an event's run waits for a break as one, and a flight
moves to another area with all of its event's flights still to come. No
flight comes earlier than it was: one already after a break stays where it
is when the flight before it works through the break. Flights
that have already run, or are under way when the delay starts, never move. The
page shows each day's flights end and free time before and after, what no
longer fits, who's needed in two places, short rest before and after, and
each flight that changes (was, now, why).

The organiser can **ease** it on the held-up areas, from the delay: breaks
(lunch and other blocked time) wait (the default), **move later** keeping
their length, **start late** by up to some minutes, or are **worked
through**, each on that area alone (the rest of the venue keeps its break);
**fewer minutes between flights**; **quicker turns** (a percentage less per
competitor); **bigger flights** (an event's flights next to each other merge
up to a size, saving a changeover); and **running past the end** of the day by
up to some minutes. The page says what the easing is and what the delay
alone would have done.

## What if an official has to leave

**What if an official has to leave?**, linked from a planned timetable,
takes who (anyone with a seat), the day and time they go, and whether for
the day or the rest of the competition, and fills each seat they'd have had
from then without saving anything (`Schedule.Left`), unless kept in the
draft as for a delay. Each seat is filled for
the rest of its event's flights they'd have had, by one person. A seat goes
to someone free for all of them (not competing or on another panel then, allowed the role at that event
and by the must rules about people), or is reached by moving others round, at
most three moves: someone on that panel up a role, or someone across from a
panel at the same time, their seat filled in turn. Two ways to choose:
**easiest to fill** (the default) ends the chain at the seat the most free
people could take (recorder or marshal before HD or execution, before chair),
fewer moves breaking ties; **fewest changes** takes the shortest chain. Each
seat shows the moves and who else was free for the last one; a seat no one
can reach is left empty. Coaches, the organiser's own rules about events,
and officials on blocked time aren't considered.

## On the day: flights started and finished

Once the timetable is published, **On the day** (a button on the entries
page, and `/day` under the admin link) is where the marshal or chair of
judges records what really happened. It shows one day at a time (today's if
today is a day of the timetable, else the first; tabs switch), as an
**overview** of the areas with flights that day: a card for each, with how
it's running, the flight it's on (the first not finished) with who's up and
its check-in and panel counts, the next flight, the one tap that comes next
(Started; Next gymnast; or Finished once the last routine is on, or no one
is left), and **Open**, its own page (`/day?day=N&area=<name>`). An area's
**own page** has that flight in full, in the order a marshal uses it:
Started at the top, then who's up, Check-in and Panel as open sections,
Finished and Undo at the bottom and Correct the times under them; then its
later flights and those finished, each closed, opening to the same. Every
form on an area's page posts the area too, and comes back to that page.
**Find a name** on the overview searches everyone in each area's flights,
shown only when found, each linking to their flight on its area's page; on
an area's page it searches its check-in rows. Each flight shows the planned time (warm-up start to end), when it actually started and
finished, and who tapped it. **Started** (a flight not yet started),
**Finished** (started, not finished) and **Undo** (takes back the finish, else
the start: it posts `undo-finish` or `undo-start`, so the same tap sent twice
takes back only the one, and the repeat quietly changes nothing) post to
`/day/flight`, which stores the time it was tapped (now, or earlier for a
tap kept without signal: see below) (`flight_times`,
one row per published flight, keyed by the flight's ID; see
`competitions.FlightKey`, and `PlaceKey` for a flight without one). A tap
that changes nothing (someone else got there first) is ignored and says so.
It always works from the **published** timetable, so planning again or
moving flights in the draft doesn't lose the times, and as a flight keeps its
ID, neither does republishing it moved to another area or day, or
renumbered: its start, its panel's check-ins and who's on go with it.
Migration 34 gave timetables saved before then IDs (a draft flight taking
its published flight's, by place or else by event and gymnasts) and moved
the day's rows from each flight's place to its ID. Organiser, "everything", chair-of-judges and timetable links
can use it, and each tap is in the history ("Marked a flight started: BUCS
L3 · flight 1 of 2 (Panel 1)").

**How late an area is** (`competitions.Lateness`): on that day and area, take
the flight with the latest recorded event (finishing beats starting for the
same flight), and compare the actual time with the planned one, the flight's
end if it finished and its start if it only started. Late is positive, early
negative, and an area with nothing recorded says "Nothing started yet".
Times marked on another date than the day's (trying the page before the
competition, say) don't count: the area says "Not measured". The
area's header reads "Running 15 min late", "On time" or "5 min early", with
what it's measured from.

Where an area is late, **What if: see the rest of the day** (for links that can
use the timetable) opens the [delay](#what-if-theres-a-delay) page with the
day, area, the time of the measured event and the minutes late filled in; it
shows what holding up the area by that long does to its later flights. Nothing is saved from the day page.

Attendees see it too: on **My competition** (the same today-only rule), a line
at the top for each area their flights are on that is running 5 or more minutes
late or early, such as "Panel 2 is running about 15 min late (as of 10:40)",
in hours and minutes from an hour as on the day page ("1 h 5 min early").

**The page stays where it is** (built 2026-10-14). Every button on the day
page (flight Started/Finished/Undo, check-in, panel, Notify, Still
officiating/competing) is still a plain form that posts and redirects (303)
back to `/day?day=N`, so it works without JavaScript. The page marks its
parts with ids and keys so the browser can keep its place: `#day-notice` and
`#day-warnings` (always there, though they may be empty), `#day-areas` (which
carries `data-day-page`, the marker for the script), `area-…` on each area's
block and `flight-…` on each flight's box (ids made from the name or the
flight key by `idPart`, which turns every byte that isn't a letter, a number
or "-" into `_` and two hex characters, so different keys never share an id),
and a `data-key` on every `<details>` (`checkin:<flight key>`,
`panel:<flight key>`, `notify:<as>:<flight key>:<person>`). Without
JavaScript, each handler's redirect ends in a `#fragment` for the flight's box
(the warnings, for Still officiating/competing), so the browser lands back on
it; the notice stays in the query, and its block is sticky at the top of the
screen so it's seen from there. With JavaScript (`dayPage` in
`static/js/competitions.js`), a tap on a form inside the areas or the warnings
is sent in the background as a plain urlencoded form
(`new URLSearchParams(new FormData(form, submitter))`, so the tapped button's
name and value go; the history, which reads `r.Form`, would not see a
multipart body). The page it is redirected to is parsed, and the notice,
warnings and areas are swapped for the fresh ones, with the `<details>` that
were open still open (a Notify list closes once sent), the flight kept at the
same place on the screen, and the tapped button disabled meanwhile (taps go
through the phone's queue instead: see taps without signal, below); taps are
sent one at a time. If the request fails or the answer isn't the day page, the
form is submitted the usual way (with the tapped button added back as a hidden
input) and the server's redirect brings the person back to the flight. So a
form can reach the server twice (the first request went through, but its
answer was lost), and every day tap is safe to repeat: check-in, panel and
Started/Finished set a state, Undo says which it takes back, Next gymnast and
Back post the gymnast and routine shown, and Notify posts a one-off id. Which
lists are open is also kept in `sessionStorage` (per path and `?day`), so a
reload keeps them. The service worker only handles GET navigations, so these
posts go straight to the network.

**Taps without signal** (built 2026-10-15, ready for the dry run). The
venue's wifi drops, so the day page's taps (the forms marked `data-tap`:
Started, Finished, Undo, Next gymnast and Back, a gymnast's or official's
check-in, Still officiating/competing) are first kept on the phone, in
`localStorage` (`static/js/taps.js`), with the phone's clock when tapped.
The page shows each at once as done but waiting ("Started at 10:02: waiting
to send"), with the buttons as they'll be once it's sent: Started waiting
offers Finished, and shows who's up, which is on the page hidden for every
flight not yet started so the marshal can go on tapping **Next gymnast**;
the phone works out who's next from the flight's check-in rows (which carry
each gymnast's entry, running number, club, routine names and state),
everyone's 1st routine then everyone's 2nd, skipping the scratched, as the
server does. While any wait, "Waiting to send: 2 taps" shows at the top. They
are sent one at a time, oldest first: straight away, on the browser's
`online` event, when the page loads and every half minute; one phone's tabs
take turns (`navigator.locks`). Each goes as the form it came from, plus
`tapped` and `sent` (the phone's clock, in milliseconds). Without a signal
the rest wait; a tap the server answers is dropped, whatever the answer (done,
done already, or refused, with the notice saying why); one the server keeps
failing on (a 500) is given up after 5 tries so it can't hold up the rest.
When none are left the page shows the server's state (the last answer's page,
or a fresh copy if the taps came from another day's page), with every notice.
Without storage on the phone, a tap is sent at once as before.

**The tap's time.** The server (`tapAt` in `web/taps.go`) takes the phone's
`sent` minus `tapped` from its own clock, so a phone clock that's wrong
doesn't matter, and records the tap at that time rather than when it arrived:
a flight's start or finish, a check-in's time, who's up. A tap with no
`tapped` (no JavaScript) is now, as before, and one sent as it's made is
within milliseconds of it. Refused, with a notice and nothing changed: a time
in the future (`sent` before `tapped`: the phone's clock went back), one more
than 12 hours old, or one that can't be read. Taps before the competition's
day aren't refused, so the page can be tried out (lateness leaves out times on
another date anyway). The history says when a tap that waited a minute or
more was made: "Marked a flight started: BUCS L3 (Panel 1) (tapped at 09:50)".

**When taps clash.** Two phones can each keep a tap for the same thing:
- **Started**: the earliest wins. A Started earlier than the one recorded
  replaces it (if it's not after the finish); a later one changes nothing and
  says someone has already done it, as a second tap always has.
- **Finished**: the earliest wins too, and one before the flight's start is
  refused ("Finished for BUCS L3 wasn't recorded: it's earlier than the start
  (09:45)"); one for a flight not started is refused.
- **Undo**: as before, it takes back what it names if it's there.
- **Here, Scratched, Clear** (gymnasts) and **Here, Missing, Clear**
  (officials, per flight of the run): the latest tap wins. One older than the
  last mark changes nothing and says so ("they were marked again later, at
  10:00"). A Clear removes the mark and its time, so an older tap arriving
  after it still applies.
- **Next gymnast and Back**: as before, by the entry and round the page
  showed, so a tap from a routine the flight has moved on from (sent twice,
  or after another phone moved it on) quietly changes nothing; a chain kept
  on one phone goes through in order. If the order changed meanwhile (someone
  scratched), the taps after that point change nothing and the page shows
  where the flight is.
- **Still officiating/competing**: once is enough; a repeat says so.

A replayed tap that no longer applies is never an error page: the server
redirects to the day with a notice, and the queue moves on.

**Correct the times** (built 2026-10-15). Under each flight, for when the
paper sheet was used or a tap was wrong, the organiser and the links that can
mark flights (everything, chair, timetable) can set when it started and
finished as clock times on its day (`POST /day/times`; Finished empty for not
finished, both empty to clear, which also clears who's up as Undo does).
Refused with a notice: a finish before the start, a finish without a start,
a time that isn't a clock time, and, once the flight's day has come, a time
later than now. The times replace those recorded (`who` becomes the link's
name), and the history says who: "Corrected a flight's times: BUCS L3 ·
flight 1 of 2 (Panel 1): started 09:10, finished 09:40". Everything reads the
same `flight_times`, so lateness, the venue screen, the delay "what if" and
the lines on attendees' pages follow the times put right.

**Finding someone, and the layout** (2026-10-15). Under the day tabs, **Jump
to** links to each area, and **Find a name** (the shared list search) looks
through every check-in row, gymnasts' and officials': a flight's box and an
area's column hide while nothing in them matches, and a check-in or Panel
list with someone it finds opens, closing again when the search is cleared.
A tap that updates the page in place keeps the search and the lists it
opened (`dayPage` carries their `data-list-opened` mark onto the fresh copy
and runs the search again); lists opened only by the search aren't kept for
a reload, as lists opened by hand are. The page uses the screen's width with
at most three areas side by side (two on a tablet, one on a phone), so a name
and its Here and Scratched share a line on a laptop. Every button, dropdown,
day tab and link on the areas and warnings is at least 40px tall, and Notify
is a box on its own line, apart from Here and Scratched. The **Venue screen**
and **Desk messages** buttons at the top are named as in the menu (the
screen's opens on the day shown); the organiser is told to make a Venue
screen link under Settings, and a link that can't make links that the
organiser makes it.

**Who's up now** (roadmap item 7, built 2026-10-14): on a flight marked
started and not finished, the box shows who's on now in large type (name,
club for a club's gymnast, and the routine), with a big **Next gymnast** and a
small **Back**. Before the first tap it shows who's first up; the marshal taps
Next gymnast as each gymnast starts. The order is the published running
order's: everyone's 1st routine in order, then everyone's 2nd in the same
order, synchro pairs and tumbling and DMT passes alike. The rules (the order,
who's skipped, what's next and what's before) are `competitions.UpOrder`
(`competitions/upnow.go`); where the marshal has got is a
`competitions.Routine`, the gymnast on (by entry) and which routine (round 1
or 2), not a place in the order, so adding someone to the flight or taking
someone out while it runs leaves the marshal on the same gymnast. It is
stored per flight (by its ID) in `up_now` (migrations 32 and 34,
`store/upnow.go`: `MoveUp`, `ClearUp`, `UpNow`), with who tapped and when.
The form posts the gymnast and routine it showed (`entry` and `round`, empty
before the first tap), and `MoveUp` moves only from there, read and written
in one transaction: the same tap sent twice, or a tap from a page out of
date, is `ErrUpMoved` and quietly changes nothing (the page comes back
showing where the flight is). Should the gymnast on be moved out of the
flight, the page shows nobody on, and Next gymnast goes to the first in
their round who competes.
Scratched gymnasts, and entries no longer in the competition, are skipped both
ways (`upOrderOf` in `web/screenup.go` says who competes); one scratched after
being reached is shown struck through until the next tap. The routine is named
from the level (`competitions.RoutineNames`): "Set" for an exercise whose options are all set routines and
"Voluntary" for one whose options are all requirements, when the two
exercises differ; otherwise "1st routine" and "2nd routine" (two voluntaries,
mixed options, synchro, or a level that can't be read), and "1st pass" and
"2nd pass" for tumbling and DMT. Next gymnast at the last routine changes
nothing and says "That was the last routine in … Tap Finished once it's
done."; the box also says it's the flight's last routine. It never finishes
the flight. Back at the first gymnast (or before the first tap), and either tap
on a flight that isn't running, are ignored with a notice. **Undo** that takes
back the start clears where the marshal had got (`ClearUp`), so starting again starts from
the first; undoing a finish keeps it. Taps post to `/day/up` (organiser,
"everything", chair and timetable links), update in place like the other day
buttons, and aren't put in the history (one per routine would bury the rest).

**Check-in and scratches** (built 2026-10-09): under each flight on the day
page, a folded **Check-in · 8 of 10 here, 1 scratched** lists the flight's
gymnasts in running order (entries since withdrawn are left out), each with
their state and the buttons that would change it: **Here**, **Scratched** and
**Clear**. They post to `/day/checkin` (`entry`, `status` = `here`,
`scratched` or empty to clear, `day`), which stores one row per entry in
`checkins` (status, when, who; no row means not marked yet). The entry has to
be in a flight of the published timetable; a tap that changes nothing is
ignored with a notice, and each real change is in the history ("Checked in:
Ann Ryan (BUCS L3)", "Scratched: …", "Cleared check-in: …"). Organiser,
"everything", chair-of-judges and timetable links can use it.

A scratched gymnast is struck through, with "(scratched)", on the printed
marshal, chair of judges and score sheets, and their event on My competition
starts its status with "Scratched". They are not taken out of the running
order or the rota: the sheets still show where they would have been. If a
scratched gymnast also holds a seat on a flight that day (matched by person
key, so a scratched entry counts for the person), the top of the day page
lists them under "Ann Ryan is scratched" with **But officiates:** and their
seats on that day's flights ("Panel 2 · BUCS L4 · Execution judge"), and, for
links that can use the timetable, **What if they leave** opens the
[leave](#what-if-an-official-has-to-leave) tool for them and that day.

**Scratch warnings: officiating, competing and clearing** (built 2026-10-09).
`scratchWarnings` (`checkinpages.go`) flags each person with any entry
scratched, for the day shown, with up to two lists under their name:
**officiating** (their seats on that day's flights, as above) and
**competing** (their *other* entries, not scratched, that are in a flight that
day: "Tumbling Novice · flight 1 of 2 · Track 1 · 14:00", the flight's name,
area and warm-up time). A person with neither list isn't shown. Each list has a
button, **Still officiating** and **Still competing**, that posts to
`/day/clear` (`person` key, `kind` = `officiating` or `competing`, `day`). The
person must be scratched in this competition and the timetable published, or
it's refused with a 400; it stores a row in `scratch_clears` (migration 27: one
per competition, person and kind, with who and when), so the warning goes for
the whole competition, not just that day, and the history says "Cleared Ann
Ryan's scratch warning (officiating)". Clearing what is already cleared is
ignored with a notice and not recorded. Organiser, "everything", chair and
timetable links can clear.

Two things clear both kinds without the desk doing anything. **Checking in any
other entry** of the person ("here") clears them, computed from the check-ins
rather than stored, so taking that check-in back (Clear) brings the warnings
back. And the person can say so themselves: on **My competition**, if one of
their entries is scratched and they have officiating seats or other entries
(any day) whose warning isn't cleared, a box says "You're scratched from
Tumbling Novice. If you're still here for your other events, tell the
organisers:" with an **I'm here for the rest** button. It posts to
`/clubs/member/{token}/competitions/{id}/here` or
`/competitions/entry/{token}/here` (their own secret links, so no admin gate),
clears both kinds for that person as themselves, and goes back to My
competition with "Thanks: the organisers can see you're here." It only works
once the timetable is published, and only for someone scratched. Self-clears
aren't in the history, but the person drops off the day page's list.

**Panel check-in** (built 2026-10-09). Beside each flight's gymnasts' check-in, a
folded **Panel · 4 of 6 here, 1 missing** lists the seats that have someone in
them (empty seats are left out): the official's name, their role, their state
(**Here** or **Missing**, with who marked it) and the buttons that would change
it, **Here**, **Missing** and **Clear**. They post to `/day/officials`
(`person` key, `flight` key, `status` = `here`, `missing` or empty to clear,
`day`), which stores one row per person and flight in `official_checkins`
(migration 28: status, who, when; no row means not marked). The person must
hold a seat on that flight of the published timetable, or it's a 400; a tap
that changes nothing is ignored with a notice. Organiser, "everything", chair
and timetable links can use it, and the history says "Checked in official: Mary
(BUCS L3, Panel 1)", "Marked an official missing: …" or "Cleared an official's
check-in: …".

**Runs.** One panel judges an event's whole run, so a check-in is for the run,
not the flight tapped: `runOf` (`panelcheckin.go`) takes the flights of the
same event on the same area and day that are back to back (the same grouping
the rota uses for a run's judges), and the status is stored for every flight of
the run where the person holds a seat (seats that change between flights, a
recorder say, are only marked where they hold them). Each flight's list reads
its own rows, so marking Mary here on flight 1 of 2 shows her here on flight 2
too, and Clear on either takes both back.

**Missing → what if.** A missing official has **What if they leave** (for links
that can use the timetable), which opens the [leave](#what-if-an-official-has-to-leave)
tool for them and that day, as for a scratched official. **Here** on any flight
also counts as "here somewhere" for the scratch warnings: a scratched person
who is checked in for their panel has turned up, so both of their warnings go,
computed rather than stored (so Clear brings them back). **Missing** doesn't
clear anything. On the printed chair of judges sheets (`sheet=judges`) the
panel line puts " ✓" after the name of each official who is checked in for that
flight; the other sheets don't.

**Notify** (built 2026-10-09). Next to each official who isn't checked in or is
missing, and each gymnast who isn't checked in (not one already marked here or
scratched), a folded **Notify** has two tick boxes, **Them** (ticked) and **Their
club** (only for someone who belongs to a club: a club member, or an entry that
came from a club; organiser-added officials and individuals have none), an
optional extra line (up to 300 characters) and **Send**. It posts to
`/day/notify` (`person` key, `flight` key, `as` = `official` or `gymnast`,
`them`, `club`, `note`, `day`, and `once`, a one-off id made each time the form
is shown) and sends a [desk message](#messages-from-the-organisers-desk)
at once, as the link that tapped it. The text is built for them:
"Mary should be at Panel 1 now to officiate BUCS L3 · flight 1 of 2 (Execution
judge)." for an official, or "Ann should be at Panel 1 now to compete in BUCS
L3 · flight 1 of 2, warm-up 09:14." for a gymnast (a synchro pair is named
together and both are told), with the extra line after it. The person has to be
on that flight (a seat, or an entry in it) or it's a 400; with neither box
ticked, or **Their club** only for someone with no club, nothing is sent and the
notice says so. **Their club** reaches only the club's comp sec and coaches, not
its members: the message's `staff` (below) holds the club, where `clubs` would
reach everyone. The page comes back with "Sent to Ann and their club. Sending to 3
people: 3 by email, 0 by push." (sent in the background, like any desk
message) and the history says "Notified Ann (and their club): should
be at Panel 1" (or "Notified Ann's club: …" for the club alone). The same form
(the same `once`) sent again within 10 minutes is a repeat: nothing is sent or
recorded, and the page comes back without a notice. The ids sent are kept in
memory (`sentForms` in `web/panelcheckin.go`), so a restart forgets them; one
that wasn't sent (the store failed, or the message limit) is forgotten at once,
so trying again works. Organiser, "everything", chair and timetable links can
use it.

## The venue screen

Built 2026-10-09, reworked 2026-10-14 for a TV across a hall (roadmap item
7). `GET /competitions/admin/{token}/screen?day=N` (default: today's index
by `todayIndex`, else the first day; `area=` any number of times for a screen
showing only those areas) is a page for a screen at the venue, from the
published timetable (`screenpages.go`, `views/screen.templ`,
`static/js/screen.js`). With no published timetable it says so. It has no
navigation: the competition name (and the day's name when there are several)
and a clock at the top, then any announcements, then the areas.

**Layout.** Each area has its own colour, by its place in the setup, the
same on every day and every screen (eight colours, taken in turn). An area
with nothing on and nothing starting within 30 minutes (or nothing more
today) is a slim strip along the bottom in its colour: "Next: … · warm-up
14:00" or "Finished for the day". The rest are cards that share the screen:
one row of up to four, else two rows (seven busy areas, as at ISTO, are two
rows of four). If every area is quiet they all keep their cards. Text scales
with the screen's width, and fewer cards get bigger text. In two rows, Now
and Next sit side by side in each card. The script then shrinks every card's
text alike until nothing in any card overflows, down to a size still readable
across a hall; past that it shows fewer names and counts the rest in "+N
more", so no name is ever cut off unseen.

Each card has:

- **Its band:** the area's name and, when `Lateness` measures one, "Running
  15 min late" (hours and minutes from an hour: "Running 1 h 5 min late"),
  "On time" or "5 min early". The lateness is information only: every time
  on the screen is the timetable's, so a late or mistaken Started tap never
  moves a warm-up time.
- **Now:** chosen by `nowOn`, a pure function of the schedule, the recorded
  times and a moment. In order: the flight on that area marked started and
  not finished (the latest started, if several); else a flight that hasn't
  been marked at all and whose time has come, labelled "not started yet".
  With nothing marked on the area that day, that's the flight whose time
  window contains now (the plan, exactly). Once the area's times are being
  marked, it's the earliest unmarked flight whose time has come, however
  late, so an area running late never goes blank while it has flights left.
  Else the name of a block covering now (a block with no areas is on all of
  them); else "—". Planned windows and blocks only count on the day shown;
  times marked on another date don't count. Under it, the flight's gymnasts
  in running order with their clubs (none for those entering on their own),
  scratched ones struck through.
- **On now** and **Up next**, only for a flight marked started and only
  once the marshal has tapped **Next gymnast** for it: who's on (name, club,
  routine) in large type and the next two to go, skipping the scratched and
  running on from the 1st routines into the 2nd (`screenUp` in
  `web/screenup.go`, `views/screenup.templ`). They take the place of the
  flight's list of names. Before the first tap, or if the gymnast reached
  has since been scratched, the card shows the list instead: the screen
  doesn't guess.
- **Next:** the following flight that hasn't been started or finished, with
  its warm-up time from the timetable and its gymnasts and clubs; with
  nothing on, the first that starts after now, or the first unmarked one once
  the area's times are being marked. "Finished for the day" when there's
  nothing more.

**Running numbers.** Every gymnast has a running number: their place in the
published flight (`No` on the views), shown before their name on the venue
screen, in On now and Up next, in the On the day check-in and the marshal's
box, on the member's published timetable and on the printed marshal and score
sheets. It's their place in the flight as published, so it never changes when
someone ahead of them is scratched or withdrawn: "5" means the same gymnast on
every page.

**Keeping it current.** The script fetches the page every 30 seconds and
swaps in the new content. If a fetch fails (the venue's wifi drops), the last
good screen stays up, and after 75 seconds a black line says "Not updated
since 10:42: check the screen's connection". The clock ticks every second
from the server's time, in Irish time, whatever the TV's own clock says. A
new version of the app reloads the page once. Without JavaScript, the page
reloads itself every 30 seconds.

**Announcements.** On the Desk messages page, **On the venue screen** puts
a line across the top of every venue screen ("Lunch extended by 15 minutes",
up to 160 characters) for 15 or 30 minutes, an hour, or until someone takes
it down. Phones aren't told. The page lists what's up, by whom and until
when, each with **Take down**. They show even before the timetable is
published, latest first. The history records each one put up and taken
down. The links that can send desk messages can do this too. A competition
keeps its latest 50 announcements, shown or not (`store.MaxScreenNotices`;
`store/screennotices.go`, `web/screennotices.go`).

A new kind of link, **venue screen** (`store.LinkScreen`), can `GET` this
route and nothing else. Organiser, "everything", chair and timetable links
can open it too, and their On the day page has a **Venue screen** link at the
top. The organiser makes the link on the Settings page.
## Messages from the organisers' desk

Built 2026-10-09. **Desk messages** (a button on the entries page, a link at
the top of the On the day page, and `/desk` under the admin link) lets the
organiser, a co-organiser ("everything"), a chair of judges or a timetable
link send a message straight away, with no grace wait, or ask people to come to the
organisers' desk. Cards links can't use it.

**To** is one of: a **club** (a select of the clubs entered: its comp sec,
coaches and members); **a flight** (a select of the published flights,
grouped by day and area, with **Its gymnasts** and **Its officials** to
tick, either or both: the gymnasts in it, with a synchro pair's partner,
and the people holding a seat on its panel); **all officials**; **one panel's officials** on a day (the people
holding a seat on that area's flights that day); or **people I pick** (a
list of every gymnast and official, searchable by name and filterable by club with
the shared search and filters, `id="desk-people"`; it sits inside the form, so
Enter in the search doesn't send the message, Clear is a plain button, and a
person hidden by the search stays ticked and is still sent;
[lists of people](../architecture/rendering.md#lists-of-people-search-and-filters)). The **Message** is up to 500
characters, and **Ask them to come to the organisers' desk** starts it with
"Please come to the organisers' desk." (the message can then be empty). The
form is resolved to person keys (members `m:<id>`, individuals by person
`i:...`, organiser-added officials `o:<id>`) and, for a club, the club's
id; nothing is sent if there is no one to send it to.

**Who is told** (`notifier.tellDesk`, in `desk.go`): every confirmed
subscription (push, and confirmed emails; any topic, since it is urgent) that
covers someone it was sent to: a member's own; an individual's, if their key
is in it; the comp sec's, if the message is for their club or any of its
members, or its **staff**; a coach's, if it is for their club or its staff, or
any member who chose them. A message's **staff** is a list of club ids whose
comp sec and coaches (every coach of the club, whoever they coach) hear it,
without the club's members (the On the day [Notify](#on-the-day-flights-started-and-finished)
button's **Their club**; the desk form itself doesn't use it).
Each address is told once, however many pages it was given on. Email has the
subject "Message from the organisers of *Competition*" (or "Please come to the
organisers' desk · *Competition*"), the text, how to find their page (open
it from the link they saved; Competitions lists the links saved in that
browser) and the off link; push has the competition's name as the title,
the text as the body and Competitions as the link. A push service that says the phone is gone
drops it. A mailer that is off simply sends none.

**Sent in the background** (built 2026-10-15): the request works out who is
told, one delivery per address, hands them to the sender (`web/sender.go`)
and answers at once: "Sent to UCD. Sending to 214 people: 180 by email, 34
by push. Everyone sees it on their own page." (or "No one it reaches has
asked to hear by push or email."). Four deliveries go at once, emails no
faster than `MAIL_PER_SECOND` (default 14, Amazon SES's production rate;
[deploy](../operations/deploy.md#notifications)); pushes aren't capped.
A failed email is tried once more after 5 seconds, then counted as failed
and logged (the competition's id and the error, never the address's token or
a page path). Change notifications go through the same sender. Under each
message the desk page shows "told 34 by push, 178 by email", then "· still
sending to 2" while it's going and "· 2 failed" for those that couldn't be
told (a gone phone counts as failed); reloading the page updates it. While
sending, the counts are in memory; once a message's deliveries are all done,
they're kept with it (`pushed`, `emailed` and `failed`, migration 41). The
queue is in memory too: when the app stops it waits up to 20 seconds for
what's queued, then logs how many were left unsent and keeps those as
failed. The confirmation email when someone asks to hear by email is still
sent in their request, outside the cap, so a failure can say so on the
form.

**On recipients' pages** (up to the latest 10, newest first, with when it was
sent and, within the last day, how long ago, e.g. "Sat 27 Feb, 10:42 · 12
min ago", as there's no read status; in a "Messages from the organisers"
box): My competition, the member's
competition section, the individual's entry page, the club page's competition
box and the coach page's competition box. A message shows on a page if its
people include that person (on a club or coach page: any of the club's
members, or the coach's members) or its clubs include their club, so people
who haven't asked to be notified still see it when they look. A message to a
club's staff shows only on that club's page and its coaches' pages
(`DeskMessage.Reaches` takes people, clubs and staff clubs), not on its
members' pages.

Messages are kept in `desk_messages` (the text, whether it asks them to come,
the audience in words such as "UCD" or "3 people", its person keys and club
ids as JSON, who sent it, when, and how many addresses were pushed and
emailed; the club ids of its staff as JSON, added in migration 29; how many
failed, added in migration 41); a competition keeps up to 500. The page lists them, latest first.
Each is in the history: "Sent a message to UCD", "Called Panel 2's officials,
Saturday to the desk".

## Simulation (ADR 0005, step 6)

**Simulate**, linked from the timetable, tries numbers against the venue
setup before entries arrive, or alongside them (ADR 0005 Decision 11). The
form starts from the competition as it stands (or from a scenario, to
change it):

- **Entries per event** (a synchro entry is a pair) and **gymnasts in all**:
  fewer gymnasts than the entries' places means some enter several
  disciplines. Stand-ins fill the biggest discipline first, then the rest go
  to gymnasts already entered, in turn, never twice in one discipline.
- **Clubs** the gymnasts and judges come from, round in turn.
- **Officials:** people who can judge each discipline (any level) and how
  many of them can chair; **how many people** those judges are (someone
  judging trampoline and synchro counts once); how many of the judges also
  **compete** (spread across the gymnasts, those who don't chair first); and
  **recorders and marshals**.
- **Judges each club must bring**, per discipline: so many judges (so many
  of them able to chair) for every so many of the club's competitors in it,
  rounding up (9 at 1 per 8 is 2), a synchro pair counting as two. With any,
  the clubs' judges come from it and the judges above are only the
  organiser's own, with no club. Each discipline's quota is filled by its
  own judges, so a club brings their sum (4 trampoline and 2 tumbling judges
  is 6 people). A percent of the clubs' judges, spread through them all, can
  also judge every other discipline as well as filling their own quota (but
  chair only in their own). A competing judge
  is a gymnast of their own club where it has one.

Stand-ins are planned with the real setup, panels and rota (`PlanStaffed`,
then `Rota`), as the real timetable is, and the result kept with the
scenario: entries and gymnasts, each day's last flight and finish, what
doesn't fit (by event) with a change that would fit it, the judges clubs
brought, seats no one could take, and gymnasts with less rest than wanted. Scenarios sit side by side;
one planned with an earlier setup, panels or events says so until **Run
again**. The last 8 are kept, apart from the timetable (migration 9), so
planning never touches them. Coaches aren't simulated, and stand-in judges
judge any level, so a real day can be tighter.

# Coaches (coach link)

Where a competition requires it, each entry needs a coach's **sign-off**
(ADR 0004 Decision 11).[^adr-0004]

- The comp sec adds the club's coaches on the club's Coaches section; each gets a coach
  link (shown once; **New link** replaces it). Each coach **signs off
  routines** or not (ADR 0007 Decision 3; new coaches do): one who doesn't
  sees their members' entries, checked, but has no sign-off buttons, and a
  sign-off from them is refused.
- **Qualifications** (ADR 0007 Decision 4): the comp sec gives a coach up to
  four, each from the built-in list (British Gymnastics levels 1–4 and
  Gymnastics Ireland levels 1–3, in trampoline, tumbling and DMT) with a
  certificate: a JPEG, PNG, WebP or PDF up to 3 MB, its type checked from
  its bytes. A club's certificates take 60 MB at most together, and so do
  those of the coaches a competition's individuals name; none is taken
  while the server's disk has less than 1 GB free. Each refusal is a plain
  notice. It's stored in the database with the coach, opened by the comp
  sec (served as its own type, `nosniff`, not cached), and deleted with the
  qualification, the coach or the club.
- **Approval** (ADR 0007 Decisions 2, 5–7): where entries need sign-off, the
  organiser can tick **Coaches must be approved** and set the lowest level
  accepted per discipline (default Level 2; synchro goes by trampoline's).
  Each competition that does lists, on the club page, the club's coaches who
  sign off, to tick and **Send** (members' coaches ticked at first), with
  the organiser's decision and note; changing a coach's qualifications
  shows "send again", and sending them again makes them wait again. The
  organiser's **Coaches** page lists them by club, with their certificates
  (opened only for coaches sent there), whether they meet each level, and
  **Approve**, **Not approved** or **Withdraw approval**, with a note;
  approving a withdrawn coach again asks whether their earlier sign-offs
  count, or only those from then (`counts_from`). A sign-off records which
  coach gave it (`signed_coach`); it counts only while that coach is
  approved, worked out as entries are read, so nothing is erased. An
  unapproved coach sees why on their page and can't sign off; the
  organiser's entry shows "Signed off by Ann, who isn't an approved coach".
  Sign-offs from before this kept only a name: where it's exactly one of
  the club's coaches, it's theirs (migration 18) and counts once they're
  approved; otherwise it doesn't count at a competition that approves
  coaches.
- A member chooses their coach under Your details, or the comp sec does on
  the club's Members section. A coach sees their own members and every member without a
  coach; the club can let **every coach see every member**.
- The coach's page lists each competition's entries they see, with problems
  and sign-off, and how many are waiting. The coach and the comp sec count
  an entry's problems as the organiser does (`competitions.EntryProblems`):
  a synchro pair's levels, the card, and a missing sign-off, from the
  competition's copy while it's what the member has, else from the member's
  own. Opening one shows it checked, with
  **Sign off** and **Not yet**, and a note the member and comp sec see.
- A changed entry needs signing off again. The sign-off goes to the
  competition with the entry when the club sends it.
- Unsigned entries can still be sent: the organiser's dashboard has a Coach
  column, counts "Not signed off by a coach" as a problem, and the CSV says
  who signed off.
- An individual's entry page gives a **sign-off link** to send their coach,
  who signs off under their name, without a coach page. Where the
  competition approves coaches (ADR 0007 Decision 8), the entry page first
  asks for **Your coach**: a name, a qualification from the list and a
  certificate, stored with the entry (deleted with it, or the competition).
  The organiser's Coaches page lists them under **Gymnasts on their own**, to approve
  as clubs' coaches are. Until approved, the sign-off link says why and
  can't be used; once approved, the coach signs off as the named coach,
  and the sign-off counts while they stay approved. Naming another coach
  waits for approval again.
- Printed cards fill in the **Coach** field: the coach who signed the entry
  off, or else the member's chosen or assigned coach.
- The coach's own page has one search and filters (`id="coach-entries"`, noun
  "entries") over the entries of every competition: by member's name, and by
  competition (only when there is more than one), level, problems, and
  sign-off (signed off, not yet, waiting). Each competition is a group that
  hides when none of its entries match. The page isn't a section, as each
  competition has its own heading
  ([lists of people](../architecture/rendering.md#lists-of-people-search-and-filters)).
- The organiser's Coaches page (coaches sent for approval) has the same
  search and filters (`id="coaches"`): by coach's name, club (or
  "Gymnasts on their own") and status (approved, waiting, not approved, approval
  withdrawn). Each club's box is a group that hides when none of its coaches
  match.

# The club's comp sec (club admin link)

The comp sec's page has a menu of four sections (roadmap 2026-10-13, finding
your way round; the menu is `views/menu.templ`, built for each page in
`web/menus_club.go`), each a page of its own:

- **Competitions** (`/clubs/admin/{token}`, the default): the same table as
  the organiser's, but for the club's members only, by competition. It has
  **Send** and **Re-send changed** buttons, shows the organiser's notes,
  shows which members are still missing a required video, and lets the comp
  sec edit an entry on a member's behalf (**Change**, under the member's
  name; the status wraps, so the table fits a laptop). Each competition's
  entries are a list section (`id="entries-<competition id>"`) searched by
  name and filtered by problems and by whether they're still to send. The
  pages for editing a member's entry or offer are in this section; the edit
  page names the member once, at its head.
- **Members** (`/members`): the join link, the members (with **New link** for
  one who lost theirs, which ends the old one, a coach to choose, and
  **Remove**) and the synchro entries other clubs' gymnasts made with them.
- **Coaches** (`/coaches`): adding and removing coaches, their links,
  qualifications and whether they sign off, and whether every coach sees every
  member.
- The Members list (`id="members"`) can be searched by name and filtered by
  coach (each of the club's coaches, or no coach), by whether the member has
  entries, and by whether they are a coach too. The Coaches list
  (`id="coaches"`; each coach's two table rows are one `<tbody>`, ruled off
  below, so "Signs off routines" and "Add a qualification" sit with their
  coach) can be
  searched by name and filtered by whether they sign off routines and whether
  they have qualifications. Both are sections that can be collapsed and start
  open ([lists of people](../architecture/rendering.md#lists-of-people-search-and-filters)).
- **Settings** (`/settings`): replacing the admin link and deleting the club,
  when the club would be deleted if no one uses it, and who opens these pages
  from their own link, each with **Remove**
  ([one link, several roles](#one-link-several-roles-adr-0010)).

**How long a club is kept** (ADR 0011 Decision 7, built 2026-10-15): until 13
months after it was last used (`store.ClubRetentionMonths`, `KeptUntil`), so
a summer break doesn't lose it. Opening a page through the club's admin or
join link, a member's or a coach's link marks it used, at most once a day
(`markUsed`), so page loads rarely write; anything changed marks it at once
(`touch`). Competitions keep their own retention (120 days after their date),
and so do individuals.

On Members, **Make a coach too** beside a member (and on Coaches, a member to
choose) makes them a coach of the club as well: no coach link, their member
link opens the coach's page. Members shows them as "Coach too", Coaches as
"A member too: opens from their member link".

Each form returns to the section it's on, with its notice. The exceptions are
replacing the admin link, which shows the new link on Competitions (where it's
saved in the browser), and the new link made for a member or coach, which is
shown on that response only. Every page of the club's saves its admin link in
the browser when it's opened (`data-club-page`), as the organiser's dashboard
does (`data-competition-page`) and members', coaches' and gymnasts' pages do;
never on a page opened through someone's own link or a helper's link (a "~" in
the path). The new or replaced link is shown and saved too (`data-club-admin`).

# A member (personal link)

A member's pages are tabs (`memberMenu` in `web/menus_club.go`; skills will be
another, ADR 0011):

- **Competitions** (`/clubs/member/{token}`, the default): one row for each
  competition the club is in, with its date, the status line (for example
  "Not entered", "Saved, not sent yet", "2 entries"), a tag for problems on
  their cards, "Timetable published" and "Message from the organisers", each
  linking to the competition's page. Synchro entries others made with them stay
  here, above the list, because the competition may not be one their club is
  entered in. The page also holds a hidden `data-member-link`, so the link is
  saved in the browser on the first visit (as it is on every member page).
- **A competition's page** (`/clubs/member/{token}/competitions/{id}`): the
  facts, the one **My competition →** link (their flights and times, the
  timetables and calendars are all there), desk messages, Tell me about
  changes, each
  discipline's entry, officiating, and the judging offer. Each discipline is a
  row (discipline, level, status such as "Checked", "2 problems", "Not entered"
  or "Waiting list, 3rd") that opens to its card and forms. It's closed unless
  a form just posted had problems, or the notice is about it (the redirect after
  a save or withdrawal carries `discipline`). Saving, withdrawing and offering
  go back to this page; so do the late-change and timetable pages, and My
  competition links back to it ("← ISTO 2027: your entries").
- **Routines** (`/clubs/member/{token}/routines`; see
  [routines on your page](#routines-on-your-page-adr-0011)).
- **Your details** (`/clubs/member/{token}/details`): their personal link,
  their coach, and **Your other links**
  ([one link, several roles](#one-link-several-roles-adr-0010)); choosing a
  coach and adding or undoing a link return here.

Each of their entries per competition: the level, the routine(s), the check
result, and whether it has been **sent** or **changed since sent**. Where the
competition asks for video, the entry says which skills need it ("Video needed
for: Triple Back") and has a field for a link per exercise, with a reminder to
make a YouTube video unlisted, not private. They can replace the level or
routine at any time until the deadline.

The entry form (`entryForm` in `views/entry.templ`, used by members, the
comp sec and individuals) has one Men or Women choice, shown only while the
chosen level ranks them separately; changing level keeps what's ticked, and an
existing entry opens with its own ticked.

## Routines on your page (ADR 0011)

Built 2026-10-15 ([ADR 0011](../../docs/adr/0011-club-routines-and-skill-tracking.md)
Decision 1; `web/routinepages.go`, `store/routines.go`, migration 37). A
member keeps routines on their page, on the server, so they open on any phone
and their coaches and comp sec see them. The routine builder still keeps its
own in the browser; nothing is copied unless the member asks.

- **Keeping one.** In the routine builder, **More** has **Save to my page**
  for each member page saved in this browser (`RoutineStore.memberLinks`,
  from `trampolineMemberLinks`; "Save to <name>" when there are several). It
  posts the routine to the member's link and opens their Routines tab. The
  Routines tab also lists the routines saved in this browser, ticked, to
  **Add to my page** at once (`keepRoutines` in `competitions.js`; hidden
  without the script or without routines). Each is posted as
  `RoutineStore.keepable` gives it: its browser id, name, skills without names
  or tariffs, what it's checked against (a built-in reference, or its own
  requirements as JSON, `SetStore.payload`) and its checks. One with the same
  browser id is changed rather than added. Empty routines are skipped, and
  ones that can't be read are counted in the notice. A member keeps up to 30
  (`store.MaxRoutines`), each up to 30 skills; past that the notice says how
  many didn't fit. The request may be up to 512 KB (`keepsRoutines`), more
  than other forms.
- **The Routines tab** lists them by name, each a row (skills, difficulty
  where it's scored, when it changed, and the coach's role chip, "left a
  note", when there's a note) that opens to
  its card, checked as the builder checks it, against its requirements, or the
  Code of Points only. **Open in the Routine Builder** is a share link
  (`/#share=j…`, `builderLink`) with the routine and its own requirements, so
  the builder's usual "Shared with you" adds a copy. **Rename** and **Delete
  from your page** (asking first) are under it.
- **Entering a competition.** Each voluntary's routine choice offers
  **Kept on your page** (from the server, `EntryForm.Kept`) as well as
  **Saved in this browser**; choosing a kept one posts its skills as a browser
  routine's are. The comp sec changing a member's entry gets the member's
  ("Kept on their page"), not only the routines in their own browser.
- **Coaches.** A coach's page has a menu, **Entries** (the page it always
  was) and **Gymnasts** (`/clubs/coach/{token}/gymnasts`, a `comp-stack`
  table): the members they see (their own and those without a coach, or everyone where the club says
  so), with how many routines each keeps, searchable and filtered by having
  routines and by whether they chose this coach. Each opens the gymnast's
  routines, checked, with **Your note** on each: the member sees it under
  the coach's role chip (`WhoChip(RoleCoach, name)`), and "written before the routine last changed" once it's
  older than the routine. A coach who doesn't see the member gets 404.
- **The comp sec.** Members shows "3 routines" beside a member who keeps
  any, opening them to look at (no note, no changes).
- Routines go with the member when they're removed, and with the club.

## Skills (ADR 0011)

Built 2026-10-15 ([ADR 0011](../../docs/adr/0011-club-routines-and-skill-tracking.md)
Decisions 2 and 3; `web/skillpages.go`, `views/skills.templ`,
`store/marks.go`, migration 38). A member marks the skills they're
**Learning** and **Can do**; a coach who sees them marks them too.

- **Both marks show.** The gymnast's mark and a coach's are kept apart
  (`skill_marks.side`), so a skill can be Can do to the gymnast and Learning
  to their coach, and when both say Can do, both marks show it. Each is a
  role chip and a state chip from the one palette
  ([states and roles](../architecture/rendering.md#states-and-roles-one-palette)):
  `WhoChip(RoleGymnast, name)` or `WhoChip(RoleCoach, name)` ("You" for the
  viewer's own), then `StateChip(InProgress, "Learning")` or
  `StateChip(Done, "Can do")`, and when. Marked skills are listed as **Can
  do** (every mark says so), **Learning**, and **Marked differently** (the
  two disagree). Each side changes and takes off only its own mark; a coach's
  replaces another coach's.
- **Which skill.** A mark is found by its skill with
  `TrampolineSkill.Equal`, the app's one rule for "the same skill": a full
  twisting back marked in any shape is one item, while a tuck and a pike back
  are two. Marks are kept with the skill (`skill_marks.skill`) under an item
  id (`s:<id>`), so club skills and level items can be marks too later.
  Matching is pairwise ([open questions](../open-questions.md#code-to-restructure)).
- **The member's Skills tab** (`/clubs/member/{token}/skills`): their marked
  skills, each with **Learning**, **Can do** or **Take off** for their own
  mark; then **Mark a skill**: a search (the routine builder's,
  `catalog.Search`: a name or FIG notation), the skills in their kept
  routines, and the catalog by tab (`catalog.Categories`, each somersault in
  tuck, pike and straight), each with its marks if it has any. Marking
  returns to the tab, keeping the search.
- **Coaches.** Gymnasts shows how many skills each can do and, as an
  attention chip, how many the gymnast marked that no coach has marked the
  same ("2 to confirm"), with a filter for those. A gymnast's page links to
  their **Skills** (`/clubs/coach/{token}/gymnasts/{member}/skills`), where
  the coach marks skills the same way; **Confirm Can do** (or Learning) gives
  a skill the coach's mark in the gymnast's state. A coach who doesn't see
  the member gets 404.
- **The comp sec** sees "can do 3" beside a member with marks on Members,
  opening their skills to look at. On Settings, **Let every member see
  everyone's skills** (off to start with; `clubs.skills_open`): then each
  member's Skills tab lists the club's other members, each opening their
  skills to look at (`/clubs/member/{token}/skills/{member}`, 404 while it's
  off).
- Up to 500 skills marked a member (`store.MaxMarks`). Marks go with the
  member, and with the club.

## Club skills (ADR 0011)

Built 2026-10-15 ([ADR 0011](../../docs/adr/0011-club-routines-and-skill-tracking.md)
Decision 4; `store/clubskills.go`, migration 39). A coach's page has a third
section, **Club skills** (`/clubs/coach/{token}/club-skills`), where any of
the club's coaches keeps the club's own skills, up to 200
(`store.MaxClubSkills`):

- **A name for a skill.** Search the catalog (`catalog.Search`) and give a
  skill the club's name ("Swivel Hips" for Seat Half Twist To Seat). It's
  kept as the skill with the name as its `custom_name`, the routine builder's
  own label for a skill, so it's still that skill: its marks are the skill's
  (found with `TrampolineSkill.Equal`), and naming it again renames it. On
  the club's pages the name shows in place of the skill's own: on Skills
  (with the skill's own name and notation beside it), in the notice after
  marking it, and on kept routines' cards (`displayName` shows a
  `custom_name`), unless the routine gave the skill a name of its own.
- **Something else to track** (a drill, a conditioning test): a name and an
  optional note, not for routines. Its marks are kept under its own item
  (`c:<id>`); marking one posts `club` (its id) instead of `skill`.
- Names are unique in the club, ignoring case. Each shows who added it
  (`WhoChip(RoleCoach, name)`). **Remove** takes something to track with its
  marks; a name only stops showing, as the marks are the skill's.
- On every Skills page, **Mark a skill** starts with **Your club's skills**,
  and a search finds them by their club name as well as the catalog.

# One link, several roles (ADR 0010)

Roadmap item 5, built 2026-10-14. A person's **records** are their club
memberships, their coaching for a club, and their entering on their own;
records with the same `person_id` are one person, and **whichever of their
personal links they open, tabs above the page** (Member, Coach, Competition secretary,
On your own, Organiser · competition, or a helper link's name · competition)
switch between their roles, each keeping its own menu. With one role there
are no tabs. The tabs are worked out for any page a personal link opens
(`withRoles` in `web/roles.go`, shown by the shared layout), so every page of
each role has them.

- **How the other roles open.** Links are kept only as hashes, so a page
  can't link to another of the person's links. Each role opens at its own
  paths with `{personal token}~{id}` in place of its token: the member, coach,
  individual or entry's id, the club's id for the comp sec's pages, the
  competition's or helper link's id for an admin link. Every lookup by token
  (`MemberByLink`, `CoachByLink`, `ClubByAdmin`, `AdminLink`,
  `IndividualEntry`, `IndividualByLink`) also accepts this, so all their
  pages and forms work unchanged. Such a page never shows itself as a link to
  keep or share; the personal link is the one to keep.
- **Adding a link.** On Your details (a member's or individual's) or at the
  foot of a coach's page, **Your other links** takes a pasted link: a member,
  coach or individual's page, an entry link, a club's admin link or a
  competition's admin or helper link. Pasting it proves they hold it, so
  there's nothing to approve. Their own link, one already there, a link that
  leads nowhere, and a join or entry link each get a plain notice. The box
  lists the other roles, each with **Undo**; it is only on the page of the
  link they hold, not one opened through it.
- **Personal links tie both ways; shared links one way.** Two personal links
  become one person, so each opens both. A club's admin link, or a
  competition's admin or helper link, may be held by several people: added to
  a personal page, it opens from there (with no more than it can do: a chair's
  link is still a chair's), but it never opens anyone's personal pages, and
  its pages have no tabs. It's kept against the record whose page it was added
  on, by the link's hash, so replacing or removing the link ends it; the comp
  sec (club Settings) and the organiser (Settings, under Links) see who has it
  and can **Remove** it.
- **Undo.** Undoing a personal role makes it a person of its own again; a
  shared link added on that record goes with it. Removing a member or coach
  ends what came through them. `shared_ties` has no foreign keys, so
  deleting a club or a competition, and the expiry of clubs, individuals and
  competitions, delete the ties to or from them (`dropTies`); deleting an
  entry deletes its check-in, late changes and concerns (`dropEntryRows`).
- **A new link from the comp sec opens only the club's records** (ADR 0010
  Decision 8, 2026-10-15). The comp sec sees the links it makes, so **New
  link** for a coach makes the coach a person of their own; for a member,
  the member and the club's coach who is the same person become a person of
  their own. The shared links added on those pages go. The person pastes
  their other links again to tie them.
- **The comp sec makes a member a coach too** (above): a coach record that is
  the same person, whose own link is never shown. Making that coach a link
  of their own (New link) takes them apart from the member first.

## Entering on your own: one page

A gymnast entering on their own has one personal page,
`/competitions/gymnast/{token}` (`web/gymnastpages.go`), like a member's
without the club: **Competitions** (each competition entered, with how many
entries), each competition's page (a row per discipline, with its status, that
opens to its card and **Open this entry**, the My competition link, and
**Enter another discipline** while entries are open) and **Your details** (the
link, and Your other links).

- **Entering.** The competition's link for entering on their own asks for **your
  page's link, if you have one**, offering the pages saved in this browser
  (own pages, member pages, entry links). Given, the entry joins that person
  (a member's link makes them a person who also enters on their own); blank,
  it makes someone new with a personal link of their own. The entry opens at
  `/competitions/entry/{personal}~{entry id}`, on their page, with the
  personal link to keep (saved in the browser as `trampolineGymnastLinks`).
  Another discipline of the same competition is entered from their page,
  under the same link.
- **Entry links from before** still work: each opens its entry, now on its
  person's page (the menu, and back to the competition), and the same token
  opens that person's page at `/competitions/gymnast/{token}`. Migration 30
  made each existing individual entry an individual of its own, so no one
  gains access to anyone else's entries; the person joins their entries with
  **Your other links**.
- **Grouped by the person.** Each entry keeps its person's key for its
  competition (`entries.person_key`): new people get `i:<individual id>`,
  entries from before keep `i:<lower-cased name>`, so keys saved in
  timetables, desk messages, scratch warnings cleared and officials'
  check-ins still find them. A person's later entries in a competition take
  the key they already have there. When two people's links are joined and
  both have entries in one competition, they keep the key entered first, and
  what was saved under the other moves to it (unless someone else of that name
  from before still has it). Undoing gives the one leaving a new key. Your
  competition, the rota and seats, check-in, desk messages, calendars and All
  people group by this key, so a misspelling no longer splits someone and two
  people of one name stay apart (those from before stay as they were until
  joined).
- Individuals with no entries left are deleted 120 days after they were made
  (clubs are kept longer, ADR 0011 Decision 7).

# My competition (roadmap: competitions 5)

From a member's page for a competition or an individual's entry, **My
competition** opens one page for the day. It's one of their pages: their club
(or "Entered on your own") and name at the top, their tabs, a back link that
names the competition ("← ISTO 2027: your entries"), then **My competition**
with the competition's name and date, and their links: Your timetable, Club
timetable (a member's), Add to calendar, Club calendar (a member's) and Tell
me about changes. The page they came from has only the one link to it (and
Tell me about changes), not the timetables and calendars as well.

- Every event they're in that has reached the competition (a member's sent
  entries; an individual's entries, as one person; synchro as either
  partner), with its card's status: checked, problems to sort out (listed),
  or not checked yet.
- Once the timetable is published, their events come in the order they warm
  up (`byWarmUp`), any not in a flight last, each with its flight, area, day
  and warm-up, where
  they are in the running order ("3rd of 12"), and about when each routine
  is, worked out from the flight's timings (a guide only).
- What they officiate, and the whole timetable, every day and area, with
  their name in bold and the flights they're in or on the panel of open and
  outlined.

## Personal and club timetables

Once the timetable is published (roadmap 2026-10-08), the panel timeline
(days side by side, a column per area, time running down evenly) is also
shown focused (`timelineFocus`):

- **Your timetable** (on "My competition"): the flights they compete in ("You compete") and the panels
  they sit on ("You: Execution judge") picked out in green.
- **Club timetable** (the comp sec's page, a coach's, a member's My
  competition): every
  flight with one of the club's gymnasts, named, or one of its people
  officiating ("Officials: Ann (E2)"; members, and the organiser's judges
  from the club), picked out.

The rest is faint, or with **Only these**, left out (blocked time stays).
Each prints like the panel timeline; notes are in each cell's hover text
too.

**Add to calendar** (roadmap 2026-10-09): next to those links, the same
flights and duties as an iCalendar file (`calendar.ics`, `calendar.go`):
**Add to calendar** for a person (their flights, as a synchro partner too,
and the seats they hold) and **Club calendar** for a club (every flight one
of its gymnasts competes in and every seat one of its people holds, named;
the same rule as the club timetable). One event per flight and person, from
the warm-up to about the end, in UTC, with the area as the location and the
times in the description. The links are secret and stay the same, and each
event's UID is stable (competition, whose, flight, role), so a calendar app
subscribed to one picks up changes when the timetable is published again
(the file also asks for an hourly refresh). Before it is published the
file is a valid empty calendar, and fills in later. Only the published
timetable is used.

# Notifications (ADR 0008)

Members (each competition on their page, and "My competition"),
individuals (their entry), comp secs (each competition on the club page)
and coaches (each competition on theirs) have **Tell me about changes**:
**On this phone** (push), and where the server can send email, an email
with the tick box "I'm 18 or over, or this is a parent's email"; each says
what about: **when or where** they (or their
gymnasts) compete, **officiating**, and **cards** (checked, a note from the
organiser, removed or on hold). A member and an individual hear about
themselves, a comp sec about every member of the club, a coach about the
members they coach. It's per competition, and deleted with it. An
individual's belongs to them, not the entry they asked on, so withdrawing
that entry leaves them hearing about their others (migration 35 moved those
made before).

- An email is sent a link to confirm it first; nothing else is sent until
  it's tapped (a button, so link checkers don't). At most 10 confirmation
  emails an hour from one address, and 50 a day for one competition (kept
  in memory, so a restart forgets them). The email names no competition or
  club, as it goes to any address typed in: it says "a competition on
  tariff.pardy.ie", and the page its link opens names the competition.
- Before the organiser publishes or unpublishes the timetable, checks a
  card or changes its note, or removes, holds or restores an entry, what's
  live is kept as a **baseline** (unless changes are already waiting), due
  **10 minutes** later. The dashboard says when, with **Notify now**; **No
  wait on the competition's days** (Settings page) makes them due at
  once on those days.
- Every half minute the server compares each due baseline with what's live
  (`changes`): each entry's flight, area and warm-up time, each person's
  duties, and each card. Running orders, and anything changed and put back,
  tell no one. Each address gets **one email** for all of it, grouped by
  gymnast ("Ann Ryan" then "- BUCS L5 Women · flight 2 of 3: now warm-up
  Saturday 10:20, Panel 2 (was Saturday 09:40, Panel 1)"), saying to open
  their page from the link they saved, with a link to Competitions
  (`/competitions`, which lists the links saved in that browser), and a
  link to stop the emails (also as one-click `List-Unsubscribe`), which
  stops every email to that address for the competition. No email
  or push links to the person's page itself, and no notification keeps its
  path: it holds their secret link (ADR 0004; migration 33 cleared those
  kept before 2026-10-15). The emails and pushes go through the same
  background sender as [desk messages](#messages-from-the-organisers-desk),
  sharing its four workers and its cap on emails a second.

Email goes by SMTP when `SMTP_HOST` and `MAIL_FROM` are set
([deploy](../operations/deploy.md#notifications)); without them only push
is offered.

**Push** (ADR 0008 Decision 3) needs nothing set up. `static/js/push.js`
registers the service worker (`/sw.js`, the site's whole scope), asks to
show notifications and subscribes with the app's VAPID public key, made on
first use and kept in the `settings` table; the page keeps the browser's
endpoint and keys (up to 5 phones a person a competition) and lists them,
"This phone" for the one in hand, each with **Turn off**. Only push
services' addresses are taken (Google, Mozilla, Apple, Microsoft), so the
server posts nowhere else. A batch's push says the one change, or "3
changes for Ann Ryan and Bea Kelly. Tap to see them.", and opens
Competitions; it's encrypted for the browser (RFC 8291, `aes128gcm`) and signed
(RFC 8292, ES256), on the standard library. A push service answering 404
or 410 drops the phone. On an iPhone or iPad, push works only from the home
screen (iOS 16.4+): the page says to add it first (Share, Add to Home
Screen); the web manifest has no `start_url`, so the home screen opens the
person's own page.

## Offline

Venue wifi is often poor, so a person's own pages still open, as last seen,
when the connection drops. Every competition page registers the service
worker (`/sw.js`), which keeps a copy of each page opened on that phone: the
competition pages under `/clubs/`, `/competitions/` (admin pages included) and
`/notify/`, and the app's static files. Pages are **network first**: the live
page when the connection answers within about four seconds, else the copy
last opened, else a short page saying this one hasn't been opened on this
phone before. Static files are kept as first fetched (their addresses change
with each version). Only successful pages are kept, never what a form
posted, and at most 60 pages per phone, the oldest dropped first.
A copy is dropped after 30 days (by the date the server sent it), and when
the server answers 404 or 410 for its page (a link replaced or deleted), so
a phone doesn't keep an admin or helper page for good. The
calculator's own pages and the rest of the site are left to the network.

While the phone is offline, a banner at the top of each page says "You're
offline: this is the page as it was when last opened", with the day and time
the server made it (Irish time, e.g. "Saturday 14:05"). It goes when the
connection comes back; a page that is still the kept copy then needs a
reload.

Taps on the On the day page made without a connection are kept on the phone
too, and sent when it's back, at the time they were tapped: see [taps without
signal](#on-the-day-flights-started-and-finished).

The copies stay on the phone, in its browser's cache for this site; nothing
secret leaves it. The server still marks these pages `no-store`, which keeps
the browser's ordinary cache from holding them; the service worker's cache
ignores that on purpose. Clearing the browser's data for the site (in its
settings) clears them.

# Built from

- `views.TariffSheet` (`views/sheet.templ`) for printed cards.
- The view screen and level panel (`views/view.templ`, `views/routine.templ`)
  for one entry.
- `requirements.Evaluate`, `requirements.RequiredElements` and the shared level
  check (ADR 0004 Decision 4) for the table's columns.

[^adr-0004]: ADR 0004 — Server storage with secret links, no accounts
