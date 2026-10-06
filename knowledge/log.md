# Update log

## 2026-10-06
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
* **Update**: The [user guide](guide/user-guide.md) covers the new picker
  tabs, Expand and Collapse all, and the sheet's toolbar staying on screen. It
  now comes as a [PDF](guide/user-guide.pdf) too, rebuilt with
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
  [user guide](guide/user-guide.md).
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
