# Update log

## 2026-10-08
* **Update**: [Late changes](features/competition-entries.md#late-changes):
  after the deadline, clubs and individuals ask for the changes the
  organiser allows (level or routines, each with a fee if accepted); the
  organiser sees whether each fits and accepts it, into the draft
  timetable and onto the invoice, or says why not.
* **Update**: [ISTO's levels](requirements/isto.md) built in from its 2025
  routines: Novice to Advanced (Set A or B, then a voluntary), Elite and
  Elite-Pro (two voluntaries), and Disability Levels 1–4. A new
  [`linked` rule](requirements/rule-types.md) counts skills one straight
  after another, for "no linked somersaults". Questions on the document are
  in [open questions](open-questions.md#istos-routines-to-confirm); synchro,
  DMT and tumbling gaps on the [roadmap](roadmap.md#possible-additions-not-planned-yet).
  An ISTO organiser answered the questions the same day: set routines don't
  score difficulty, a 2.0 penalty outside the tariff band, and Disability
  Level 5 (an old routine that contradicts itself) is left out.
* **Update**: [Limits and waiting lists](features/competition-entries.md#limits-and-waiting-lists):
  a limit per event; later entries wait in the order they came in and move
  up as places free; the organiser can let one in over the limit; clubs
  and gymnasts see their place.
* **Update**: [Entry fees](features/competition-entries.md#entry-fees): the
  organiser sets fees per entry (by discipline) and per club, records
  payments and prints invoices; clubs and individuals see what they owe.
* **Update**: [Helpers' links](features/competition-entries.md#helpers-links-history-and-concerns-adr-0009)
  ([ADR 0009](../docs/adr/0009-helper-links.md)): the organiser makes links
  for checking cards, chairs of judges, the timetable and officials, or a
  co-organiser, each doing only that; a **History** of every change and who
  made it; and **concerns** any helper can flag for the organiser to
  resolve.
* **Update**: [Roadmap](roadmap.md) priorities: while it's being demoed,
  admin links that can do less with a change history, entry fees, limits
  with a waiting list, then the day itself; ISTO 2027's entries open early
  in 2027, the competition is early April.
* **Update**: On the [roadmap](roadmap.md), still to put in order: flights
  started and finished by the marshal, check-in and scratches, a "now on"
  screen, pages that work offline, copying last year's competition, more
  admin links each able to do less, a change history, entry fees, limits
  per level with a waiting list, judges' qualifications, add to calendar,
  and messages from the organisers' desk to a club.
* **Update**: Recorders' **score sheets** to print from the timetable: a
  landscape page a flight, a row per gymnast in running order, a box for
  each judge's mark of each routine (the panel's own execution, HD and
  synchronisation judges), difficulty, penalty and totals, all left to
  fill in. Officials can open their own panels' sheets on their phone.
* **Update**: Sign-offs made before approved coaches existed (they kept only
  the coach's name) now count once that coach is approved, where the name
  is exactly one of the club's coaches (migration 18).
* **Update**: [Notifications](features/competition-entries.md#notifications-adr-0008)
  by phone push (ADR 0008 step 2): **On this phone** on the same pages, with
  a service worker and web manifest (iPhones from the home screen), the
  app's own VAPID keys, and messages encrypted and signed on the standard
  library. Needs nothing set up, so "Tell me about changes" shows now.
* **Update**: [Notifications](features/competition-entries.md#notifications-adr-0008)
  by email ([ADR 0008](../docs/adr/0008-notifications.md) step 1): members,
  individuals, comp secs and coaches opt in per competition, confirm their
  email, and get one email per batch of changes (timetable, officiating,
  cards) after a 10-minute wait, with Notify now and no wait on the
  competition's days. Needs `SMTP_HOST` and `MAIL_FROM` on the server. The
  delay "what if" no longer brings an event's flights after lunch earlier,
  and its Ease it options show they open.
* **Update**: [Going live](features/competition-entries.md#going-live): a
  new competition starts private, its links showing when entries open; the
  organiser goes live now or at a set time, and can make it private again
  to pause entries, keeping those made. Dashboard levels start folded, with
  Expand all and Collapse all, remembered in the browser. The
  [roadmap](roadmap.md)'s next steps: notifications, the "what ifs'"
  missing checks, saying what a manual change breaks, then splitting an
  event by routine.
* **Update**: Draft and published timetables: once published, every change
  the organiser makes (planning, moves, officials' seats, a kept delay or
  leaving official) is a draft until **Publish changes**; attendees see the
  published copy. **Discard changes** goes back to it. The delay and
  leaving-official "what ifs" can be kept in the draft.
* **Update**: [Personal and club timetables](features/competition-entries.md#personal-and-club-timetables):
  once published, the panel timeline with a person's flights and panels
  picked out, or everywhere a club has someone competing or officiating,
  named; the rest faint, or hidden with "Only these". Linked from members',
  individuals', coaches' and comp secs' pages.
* **Update**: [Finding entries](features/competition-entries.md#dashboard)
  on the organiser's dashboard: search by name, filters by club, coach,
  checked, sign-off, video, men or women and problems, sorting, levels that
  fold away with "12 of 96" counts and a list to jump to, and printing or
  downloading what's shown.
* **Update**: [Removing and holding entries](features/competition-entries.md#removing-and-holding-entries):
  the organiser ticks entries and removes them (not sent or changed again
  until restored) or holds them for changes (sent again, then accepted
  back in), with an optional reason for the club, the gymnast or both;
  they're out of every count, card, CSV and timetable, and listed to
  restore. On the [roadmap](roadmap.md): personal and club timetables laid
  out like the panel timeline.
* **Update**: On the [roadmap](roadmap.md): ways round the organiser's
  entries: collapsing levels, searching for a person, filtering by coach,
  checked, sign-off, video and more, sorting, and printing what's shown.
* **Update**: [Synchro partners](features/competition-entries.md#events-and-synchro-adr-0005-step-2)
  see the pair's entry once they confirm: on their own page, their coach's
  and their club's, under **Synchro as a partner**, even if their club
  isn't entered. Both gymnasts' entries warn when the pair is at the wrong
  level for their individual ones.
* **Update**: "Hand changes" are "manual changes" on the pages and in the
  guide. On the [roadmap](roadmap.md): officials' changes go in the draft
  timetable too, and a manual change is to say at once what it breaks and
  offer fixes.
* **Update**: On the [roadmap](roadmap.md): a synchro partner, their coach
  and their club see the pair's entry once the partner confirms (decided);
  a coach per discipline, and synchro signed off by both partners' coaches,
  noted as possible additions.
* **Fix**: synchro panels can have HD judges too, like trampoline's (none
  until the organiser adds them); the demo gives synchro 2.
* **Update**: Individuals' coaches (ADR 0007 step 4, the last): where a
  competition approves coaches, a gymnast entering on their own names their
  coach with a qualification and certificate; the organiser approves them
  under **Individuals** on the Coaches page; the sign-off link works once
  they're approved, and the coach signs off as themselves.
* **Update**: [Approved coaches](features/competition-entries.md#coaches-coach-link)
  (ADR 0007 step 3): organisers can require coaches approved, with the
  lowest level per discipline; clubs tick and send coaches; the organiser's
  **Coaches** page shows certificates and whether each meets the level, to
  approve, refuse or withdraw (approving again, choosing whether earlier
  sign-offs count). Sign-offs record their coach and count only while that
  coach is approved; unapproved coaches can't sign off.
* **Update**: [Coaches](features/competition-entries.md#coaches-coach-link)
  (ADR 0007 step 2): the comp sec says whether each coach signs off
  routines (one who doesn't sees entries without the buttons), and gives
  coaches qualifications from the built-in list, each with a photo or PDF
  of its certificate (up to 10 MB, four per coach), stored with the coach.
* **Update**: [ADR 0007](../docs/adr/0007-approved-coaches.md) accepted:
  approved coaches. A built-in list of coaching qualifications (levels 1–4
  by discipline, either body), certificates stored in the database and kept
  with the club, clubs saying which coaches sign off and ticking which go
  to each competition, organisers approving them (choosing, on approving
  again, whether earlier sign-offs count), and only approved coaches'
  sign-offs counting.
* **Update**: [Officials rota](features/competition-entries.md#officials-rota-adr-0005-step-5)
  (ADR 0006 step 3): each event's panel is filled once, with people free
  for all its flights, and judges all of them; the organiser can let
  recorders, or marshals, change between flights (each on its own). Seats
  no one could take are counted a seat per event ("on 3 panels"); a seat
  given by hand goes on the event's other flights; the officials rota gives
  one line per event ("all 5 flights"); and **What if an official has to
  leave?** fills each seat for the rest of the event, with one person.
  Planning prefers times when an event's judges are free for all of it.
* **Update**: [Timetable](features/competition-entries.md#timetable-adr-0005-step-4)
  runs (ADR 0006 step 2): each event's flights are planned back to back on
  one area and day, numbered in the order they run, and by default all
  before or all after a break (**Events can run across breaks** to allow
  it). **Move event** moves all of an event's flights, **Earlier** reorders
  them, and an event split up by hand is listed as a problem. The delay
  "what if" keeps an event's flights together. Planning tries events tied
  to a day first, then the longest, puts what didn't fit first next time,
  and tries a windowed lunch at different times; the demo's biggest events
  run across lunch.
* **Update**: [ADR 0006](../docs/adr/0006-one-panel-per-event.md) proposed:
  one panel for a whole event, amending ADR 0005. The scheduler places an
  event's flights as one run on one area and day, the rota fills its panel
  once, flights are numbered in the order they run, and an organiser can
  rarely split an event by routine across two areas.
* **Update**: Tried with an ISTO organiser: approving coaches and removing
  entries were their two big asks, next on the [roadmap](roadmap.md) after
  the biggest priority, one panel for all of an event's flights.
  An organiser ticks entries (one by one or a club at once) and removes
  them or puts them on hold asking for changes, with an optional reason for
  the club, the member or both, and can restore them. Approved coaches'
  qualification is picked from a list of BG and Gymnastics Ireland levels,
  a higher level counting; no expiry date; a club's certificate is kept for
  its next competitions. A held entry sent again waits for the organiser.

## 2026-10-07
* **Update**: Decided on the [roadmap](roadmap.md): one panel, every seat,
  for all of an event's flights, which run back to back on one area; an
  event split across panels only rarely, by routine. Flights are to be
  numbered in the order they run.
* **Update**: Decided on the [roadmap](roadmap.md): an organiser can
  require approved coaches, sent by clubs (or named by individuals) with a
  certificate and approved per competition; only approved coaches sign off,
  and a withdrawn approval undoes their sign-offs.
* **Update**: Decided on the [roadmap](roadmap.md): opt-in notifications
  for members, individuals, comp secs and coaches when a flight moves,
  duties change or a card is checked, by phone push and email (clubs
  always email), an email given only by someone 18 or over or as a
  parent's. Notifications wait for a grace period after a change goes live,
  then go out one per person told (a club's covers all its members, a
  gymnast's all their events), summarising what changed; the organiser can
  notify now or turn the wait off on the competition's days; questions left in
  [open questions](open-questions.md#notifications).
* **Update**: Decided on the [roadmap](roadmap.md): a kept "what if" is a
  draft the organiser publishes, as is every change to a published
  timetable; and a competition is set up in private, its links showing
  when entries open, and goes live by a button or at a set time (and can be
  made private again to pause entries).
* **Update**: Confirmed that "the lower level" for a synchro pair a level
  apart means the easier one, as built; the [roadmap](roadmap.md) keeps only
  warning the gymnast on their own page.
* **Update**: The [competitions guide](guide/competition-guide.md) says a
  synchro pair enters one routine (the level's voluntary) wherever it
  mattered, and gives synchro's default timing.
* **Update**: The [roadmap](roadmap.md) says where things are (every step of
  ADR 0005 built, synchro as ISTO runs it, the on-the-day "what ifs") and
  proposes next steps.
* **Update**: [What if an official has to leave?](features/competition-entries.md#what-if-an-official-has-to-leave):
  each of their seats filled by someone free or by moving others round,
  easiest to fill or fewest changes, without changing the rota.
* **Update**: [What if there's a delay?](features/competition-entries.md#what-if-theres-a-delay):
  hold up areas from a time for some minutes and see the later flights shift
  (or move to another area when they'd overrun or clash), without changing
  the timetable. It can be eased: breaks moved, shortened or worked through,
  shorter changeovers, quicker turns, bigger flights, or running over.
* **Update**: A synchro pair enters one routine, the level's voluntary: the
  entry form, card, dashboard, printed cards, CSV and sheets show just it.
* **Fix**: The timetable report's spare time counted blocked time, so a day
  ending with awards looked full however little ran. Each day now says when
  its flights end and the time free after them that isn't blocked off for
  the whole venue.
* **Fix**: The rota report's "Seats no one could take" counted flights, not
  seats; it now says both ("11, on 3 flights").
* **Update**: The [demo](operations/deploy.md#demo-competition) is bigger:
  25 clubs and about 400 gymnasts over three days of 09:00 to 17:30, with a
  general warm-up, an hour's lunch, and on Sunday synchro (three events of
  grouped levels, mixed pairs) and an hour's display. Synchro defaults to 2½
  minutes per pair, who do one routine, and its flights count pairs.
* **Update**: [Synchro](features/competition-entries.md#events-and-synchro-adr-0005-step-2)
  levels can be paired into one event ("BUCS L1/L2"), each pair saying which
  level they're doing; a pair entered at a level other than their individual
  levels give (the same, or the easier of two a level apart) is flagged.
* **Update**: Synchro panels have 2 synchronisation judges by default, a
  setting on the [officials](features/competition-entries.md#officials-adr-0005-step-3)
  page, filled by the rota from synchro's judges.
* **New**: **How it's judged**. Every skill card, and the "Add a skill" card,
  opens a pop-up with the execution deductions that can apply to that skill
  (shape, legs and feet, arms, opening and twist, and the landing on the last
  element), each with its amount, an explanation and the Code of Points
  reference, and how the score adds up. The list is worked out from the
  skill's attributes by rules from the FIG Code of Points 2025–2028 §13, §16,
  §20 and the Part II drawings, so custom skills get one too
  ([execution judging](domain/execution.md)).
* **Update**: The [panel timeline](features/competition-entries.md#timetable-adr-0005-step-4)
  runs at an even time scale, and comes in two views: without officials
  (every day side by side), and **with officials**, a sheet for each day's
  area with a column for each seat on its panel and who has it.
* **Update**: Step 6 of ADR 0005, [simulation](features/competition-entries.md#simulation-adr-0005-step-6):
  the organiser tries entries per event, gymnasts in all, clubs, judges,
  chairs and helpers against the venue setup; stand-ins are planned with the
  real scheduler and rota, and scenarios sit side by side (when each day's
  flights end, what doesn't fit and what would, empty seats), without
  touching the timetable. Every step of ADR 0005 is now built. A scenario
  can say what judges each club must bring for every so many of its
  competitors in a discipline, and how many of them can chair; a club
  brings the sum of its quotas, and a percent of the clubs' judges can also
  judge other disciplines.
* **Update**: The [panel timeline](features/competition-entries.md#timetable-adr-0005-step-4)
  (roadmap, from the demo): every day side by side, a column per area, time
  running down, each flight and blocked time with who officiates it; printed
  on A3 landscape or downloaded as CSV. Linked from the timetable as
  **Panel timeline**.
* **Fix**: Judging levels ran the wrong way for BUCS: "up to" a level and
  "only levels below their own" went by the order the form sent, hardest
  first for BUCS, so "up to BUCS L5" allowed L1 to L5. A competition's levels
  now have an [order](features/competition-entries.md#officials-adr-0005-step-3),
  easiest first: built-ins by rank (BUCS turned round), the organiser's own
  after them, and any level moved under Links and settings. Older
  competitions are put in order when read.
* **Update**: A [demo competition](operations/deploy.md#demo-competition):
  `tariffCalculator demo` fills `DATA_DIR` with a made-up student
  competition (clubs, coaches, entries with some problems, officials, a
  planned and published timetable) through the real pages, and prints the
  links to open.
* **Update**: Trampoline panels can have HD (horizontal displacement) judges,
  2 where no machine measures it: a setting on the
  [officials](features/competition-entries.md#officials-adr-0005-step-3)
  page, none by default, filled by the rota like any judge.
* **Update**: Timetable tidy-ups: names that look like one person entered
  twice are flagged; capping an event's entries is among the fixes; planning
  prefers times when an event's judges are free; blocked time (an ad hoc
  event) can need officials, staffed by the rota.
* **Update**: [My competition](features/competition-entries.md#my-competition-roadmap-competitions-5)
  (competitions 5): one page per person per competition with their events
  and cards, flights, place in the order, rough routine times, duties and
  the published timetable with them picked out.
* **Update**: Step 5 of ADR 0005, the [officials rota](features/competition-entries.md#officials-rota-adr-0005-step-5):
  planning fills every flight's panel from the people who offered, never
  while they compete, with own-club judging shared, rules about people (a
  role at an event, not officiating, hours), seats changed by hand, coach
  clashes avoided and reported, the rota printed and duties shown once
  published. The [competitions guide](guide/competition-guide.md) covers it.
* **Update**: The [timetable](features/competition-entries.md#timetable-adr-0005-step-4)
  puts someone with little rest between two flights (less than twice the rest
  setting) early in the first running order and late in the second; a redraw
  keeps it.
* **Update**: Step 4 of ADR 0005, the [timetable](features/competition-entries.md#timetable-adr-0005-step-4)
  plans over the venue's days and named areas: blocked time, the organiser's
  rules (must or prefer), timings per discipline, rest between a person's
  turns, and no one in two places at once. The report says when each day
  finishes, what doesn't fit and which change would make it fit. The
  [competitions guide](guide/competition-guide.md) covers it.
* **Update**: Step 3 of ADR 0005, [officials](features/competition-entries.md#officials-adr-0005-step-3):
  members, comp secs and individuals say what each can judge and help with
  (sent with the club's entries), the organiser adds people with no club and
  marks people qualified, and each discipline's panel defaults to the Code of
  Points' and can be changed, with the rule for who may judge what. The
  [competitions guide](guide/competition-guide.md) covers it.
* **Update**: [ADR 0005](../docs/adr/0005-multi-discipline-timetabling.md):
  ISTO runs over 2½–3 days, so the venue has days, each with its own hours;
  panels default to the Code of Points' (a CJP, 6 execution and 2 difficulty
  judges for every discipline) and every number is a setting: panels, minutes
  per competitor, minutes between flights, who can judge what, and rest
  ([open questions](open-questions.md)).

## 2026-10-06
* **Update**: Step 2 of [ADR 0005](../docs/adr/0005-multi-discipline-timetabling.md),
  [events](features/competition-entries.md#events-and-synchro-adr-0005-step-2):
  synchro, tumbling and DMT alongside trampoline; a member's entry per
  discipline (migration 6 keeps existing entries as trampoline); synchro pairs
  of any two gymnasts confirmed by a partner link. The
  [competitions guide](guide/competition-guide.md) covers them and the
  timetable, and its PDF is rebuilt.
* **Update**: The user guide is split in two: the
  [routine builder guide](guide/routine-guide.md) (building, checking,
  requirements, levels, view, print, share) and the
  [competitions guide](guide/competition-guide.md) (organisers, comp secs,
  members, coaches, individual entrants), each with a PDF built by
  `scripts/guide-pdfs.py`.
* **Update**: The requirements editor flags rules that contradict each other
  or can't be met ([framework](requirements/framework.md)), without blocking
  the save. The [view screen](features/view.md) has a **Share** button, as in
  the builder. [User guide](guide/routine-guide.md) and its PDF updated.
* **Creation**: [ADR 0005](../docs/adr/0005-multi-discipline-timetabling.md)
  (accepted): timetabling for ISTO, across trampoline, synchro, tumbling and
  DMT, with people who compete and judge, the venue's strict end time, rest
  between turns, own-club judging balanced, and simulation. The officials rota
  (competitions 4) becomes part of it, and organisers can block off time
  for lunch, awards and ad hoc events taking entries on the day, and set
  their own rules (what happens where, who does what), each must or prefer
  ([roadmap](roadmap.md), [open questions](open-questions.md)).
* **Update**: The [timetable](features/competition-entries.md#timetable-adr-0005-step-4)
  (roadmap: competitions 3): men and women ranked separately where the
  organiser chooses, flights planned on panels (split separately from
  ranking), adjusted by hand, printed for marshals and the chair of judges,
  and published to clubs and gymnasts.
* **Update**: [Roadmap](roadmap.md) priorities: the timetable and flight
  planner (competitions 3) is next; the difficulty judge helper (2) moves to
  the bottom, as it would be hard to make reliable enough to use.
* **Update**: CI runs the tests on every pull request, so checks show on a PR
  before merging; only pushes to `master` deploy ([deploy](operations/deploy.md#ci)).
  `master` now requires the test check before a PR merges, and auto-merge is
  allowed.
* **Update**: The [user guide](guide/routine-guide.md) has a section 7,
  "Competitions and clubs": private links, running a competition, running a
  club, entering through a club, signing off as a coach, entering on your own,
  and what's kept. [Features](guide/features.md) lists competitions and clubs,
  and the [PDF](guide/routine-guide.pdf) is rebuilt.
* **Update**: Competition storage is live on tariff.pardy.ie: the server has
  the `/srv/data/tariff` volume and `DATA_DIR`, the pages work at
  `/competitions` (not linked from the calculator), and the nightly backup
  includes the database ([deploy](operations/deploy.md#competition-storage),
  [roadmap](roadmap.md), [competition entries](features/competition-entries.md)).
* **Update**: Coach sign-off for [competition entries](features/competition-entries.md#coaches-coach-link)
  (ADR 0004 Decision 11): the comp sec adds coaches, members choose theirs
  (or the comp sec assigns), a coach's page lists the entries they see to
  sign off or say not yet, and a competition can require sign-off, flagging
  entries without one. Individuals get a sign-off link for their coach.
* **Update**: [Competition entries](features/competition-entries.md): an
  entry a club sent for a member who has since withdrawn it (or left the
  club) is marked "Withdrawn" on the organiser's dashboard and entry page,
  and isn't counted, printed or listed as a problem, until the club sends
  again and it goes. The CSV has a Withdrawn column. On a member's page and
  the comp sec's page for a member, the form to change an entry starts
  closed under the checked entry.
* **Update**: Step 7 of ADR 0004: competition storage on the server is
  described in [deploy](operations/deploy.md#competition-storage). The
  `/srv/data/tariff` volume, `DATA_DIR` and nightly SQLite backup are a change
  to the server repository, to apply before entries go live. The pages are
  reached at `/competitions`; the calculator doesn't link to them, for now.
* **Update**: Step 6 of ADR 0004, video proof for [competition entries](features/competition-entries.md):
  the organiser asks for none, some skills (any triple, any double or more,
  any skill of a tariff or more) or each whole routine, and can change it.
  Gymnasts add a link per exercise (YouTube, Google Drive, Vimeo, Dropbox or
  OneDrive, https only) with a note; each entry shows which skills need video.
  The dashboard and CSV show missing, provided, OK or need more; the organiser
  opens videos in a new tab (never embedded) and marks them OK or "need more"
  with a note the club and gymnast see. A new link clears the review.
* **Update**: Step 5 of ADR 0004 for [competition entries](features/competition-entries.md):
  the organiser marks entries checked with a note (shown to the club and the
  gymnast; a changed entry needs checking again), prints cards (the tariff
  sheet filled in, one exercise per page, for everything, a level, the
  dashboard's filter, the unchecked or one entry), downloads a CSV, and closes
  or reopens entries. Expired competitions and clubs are deleted every 6 hours.
* **Update**: Step 4 of ADR 0004, the club pages for
  [competition entries](features/competition-entries.md): a comp sec creates a
  club and enters it through a competition's club link; members join through
  the join link and keep their entries on their own page (its link saved in
  their browser); the comp sec sees each competition's entries (not sent,
  sent, changed since sent, withdrawn), sends new and changed ones or all
  again, changes an entry on a member's behalf, gives a member a new link, or
  removes them. `/competitions` lists every link saved in the browser. Stored
  times now keep microseconds, so a change straight after a send shows.
* **Update**: Step 3 of ADR 0004, the first [competition entries](features/competition-entries.md)
  pages: create a competition (built-in levels and the browser's own), the
  organiser's dashboard by level (filter by club or problems), each entry
  checked, replacing the admin link, deleting, and individual entry with a
  personal link to change or withdraw it. Not linked from the calculator, and
  on only where `DATA_DIR` is set ([routes](architecture/http-routes.md#competition-pages)).
* **Update**: Step 2 of [ADR 0004](../docs/adr/0004-server-storage-secret-links.md)
  for [competition entries](features/competition-entries.md): a `competitions`
  package (competitions, the levels they offer, entries and their checking)
  and a `store` package (SQLite with secret links, deadlines, limits, and
  deletion 120 days after the competition or a club's last use). No pages
  use them yet. A level's two exercises are now checked by
  `requirements.CheckPair` in both the builder and stored entries
  ([levels](requirements/levels.md), [stack](architecture/stack.md)).
* **Update**: [ADR 0004](../docs/adr/0004-server-storage-secret-links.md)
  clarified: a comp sec gives a member who lost their link a new one (links
  are stored as hashes, so can't be shown again), and a competition entry is
  checked with its requirements' checks, never the gymnast's own.
* **Update**: The [roadmap](roadmap.md) says a competition's entries are
  deleted 120 days after it, as ADR 0004 does.

## 2026-10-05
* **Update**: Competitions can ask for video proof that a gymnast can perform
  their routine safely: a link per exercise (an unlisted YouTube video, a
  Drive file), for chosen skills or the whole routine, reviewed by the
  organiser. Never uploaded or played in the app
  ([ADR 0004](../docs/adr/0004-server-storage-secret-links.md) Decision 10,
  [competition entries](features/competition-entries.md)).
* **Update**: [Deploy](operations/deploy.md): the old Render address now
  redirects to tariff.pardy.ie, keeping the path and any share link; links to
  the server repo point at its GUIDE.md and CONTRACT.md.
* **Update**: [Roadmap](roadmap.md) priorities: card collection first, the
  routine suggester moved to the bottom, and ISTO's levels
  ([open questions](open-questions.md)) not expected for a while. The tariff
  sheet and menus checked on an iPhone; the domain question is closed.
* **Update**: The [user guide](guide/routine-guide.md) covers the new picker
  tabs, Expand and Collapse all, and the sheet's toolbar staying on screen. It
  now comes as a [PDF](guide/routine-guide.pdf) too, rebuilt with
  `scripts/user-guide-pdf.py` whenever the guide changes.
* **Update**: The picker's Somersaults and Twists tabs are one tab, Singles,
  lining up with Doubles and Triples, and "Drops & seat" is now "Body
  landings" ([skill catalog](domain/skill-catalog.md)).
* **Update**: Pop-up menus (the builder's More, the sheet's "Show on sheet",
  the view's Show) keep themselves on screen; the More menu ran off the left
  edge when its button wrapped on a phone. The [tariff sheet](features/tariff-sheet.md)'s
  toolbar stays at the top as the sheet scrolls, and its detail fields no
  longer push a 320px screen sideways.
* **Update**: The app is live at <https://tariff.pardy.ie> on the self-hosted
  server, deployed by CI on every push to `master`
  ([deploy](operations/deploy.md)); Render is to be retired. The name stays
  for now, competition and club tools included ([roadmap](roadmap.md)).
* **Update**: Fixed the [tariff sheet](features/tariff-sheet.md), which stayed
  on "Loading the current routine" since 2026-10-04: its `hx-vals` wasn't an
  object literal, so htmx couldn't send the routine. A test now checks every
  template's `js:` `hx-vals` ([testing](architecture/testing.md)).
* **Creation**: [Competition entries](features/competition-entries.md)
  (planned): the organiser's dashboard, one-entry view, printing and export,
  and what comp secs and members see.
* **Creation**: [ADR 0004](../docs/adr/0004-server-storage-secret-links.md)
  (accepted): SQLite storage with secret links and no accounts, for
  competition card collection ([roadmap](roadmap.md)). Members enter through
  their club, whose competition secretary sends entries to each competition;
  individuals can enter directly where the organiser allows.
* **Creation**: A [roadmap](roadmap.md) for competitions and clubs, starting
  with competition card collection.
* **Creation**: A [guide](guide/) for gymnasts and coaches:
  [features](guide/features.md) and a step-by-step
  [user guide](guide/routine-guide.md).
* **Update**: Brought the bundle up to date with levels: skill names lead with
  the shape for single somersaults, the [view screen](features/view.md) opens a
  level entry from Levels mode, and the tariff sheet, stack, tests and overview
  pages mention levels.
* **Update**: View with two routines side by side shows both (each column also
  has its own View), and a level being worked on can be shared by link with
  its routines.
* **Update**: Set routines are built in the Routine Builder (More → Save as a
  set routine) or made from a routine already built, and edited there; the
  requirements editor no longer writes them element by element. The level
  editor asks for the structure first (set routine then voluntary, one
  voluntary, two voluntaries, set routine for both) and offers each slot only
  what fits.
* **Update**: "Check against" lists each level's voluntary requirements, named
  after the level (or by exercise when its two differ); set routines are a
  starting point ("Start from a set...") rather than requirements.

## 2026-10-04
* **Update**: Levels and routines are kept apart. "Check against" lists
  requirements only; the builder's new Levels mode shows a level's tabs (set
  routines as prescribed, side by side if wanted) and links ordinary routines
  to its voluntaries. Level routines from earlier became level entries. On the
  view screen, rows now fit their column, so difficulty no longer runs off the
  edge.
* **Update**: The [view screen](features/view.md)'s Show menu lists the
  routines on screen by name (Set 1, Set 2, Voluntary) to show or hide, in
  place of "The other exercise" and "Other set routine options".
* **Update**: "Add to" lists every saved routine (a level routine by
  voluntary tab), not just the ones on screen. Single fronts, backs and baranis
  are named shape first ("Tuck Back", "Pike Barani").
* **Update**: A [level](requirements/levels.md) routine holds both exercises
  as tabs (e.g. Set 1, Set 2, Voluntary) instead of two paired routines:
  set routines show as prescribed without loading, and a set can be copied into
  the voluntary. On wider screens the skill card's Add button sits at the end of
  its row instead of stretching across the card.
* **Update**: The [view screen](features/view.md) shows a routine, or a
  level's pair, full screen without scrolling, with a Show menu to turn each
  kind of information on or off. Linked from the routine builder's View button.
  A level's set routine options that no routine is doing show too, so both
  options appear beside the voluntary.
* **Update**: [Levels](requirements/levels.md) pair requirements for a
  competition's two exercises: built in for every BUCS, FIG and BG level, and
  written by coaches in a level editor. Routines doing a level's exercises are
  paired in the builder and checked together: when the first exercise scores
  only some elements, their difficulty carries over and they can't be repeated
  in the second. Pairs and levels travel in share links.
* **Update**: The 0.1 for shaped jumps and seat changes is confirmed correct
  ([tariff](domain/tariff.md)); removed from the open questions.
* **Initialization**: Created this bundle (OKF v0.2) covering the domain,
  requirements, features, architecture and operations, linked to the
  [server bundle](https://github.com/jackpardy/server/blob/main/knowledge/index.md).
* **Context**: The same day the app gained saved routines, side-by-side
  comparison, the [requirements framework](requirements/framework.md) with
  built-in BUCS, FIG and British Gymnastics requirements,
  [sharing by link or QR](features/sharing.md), the
  [skill picker and search](domain/skill-catalog.md), and the
  [tariff sheet](features/tariff-sheet.md).

## 2026-10-03
* **Context**: [ADR 0002](../docs/adr/0002-server-rendered-frontend.md)
  accepted and carried out: the UI moved to templ, the static assets were
  embedded, and drag-to-reorder moved to SortableJS. The engine was pinned to
  the 139 CoP worked examples, and Docker plus a deploy workflow were added.

## 2026-06-23
* **Context**: [ADR 0001](../docs/adr/0001-architecture.md) written; naming
  and routine validation moved into the `skills` package.

## 2025-03-28
* **Context**: First version: a Go and htmx prototype.
