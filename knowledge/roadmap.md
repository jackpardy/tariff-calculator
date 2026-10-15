---
type: Roadmap
title: Roadmap
description: Where the project could go beyond the routine builder, covering tools for competition organisers and attendees and for people running or training in a club, with what already exists and what to build first.
tags: [roadmap, competitions, clubs, planning]
status: draft
generated: { by: claude-code/cli, at: 2026-10-08T09:00:00Z }
stale_after: 2027-04-01T00:00:00Z
sources:
  - id: tramponline
    resource: https://www.tramponline.org/
    title: TrampOnline (online entries and scoring)
  - id: tscore
    resource: https://tscore.co.uk/wp/
    title: TScore (competition organiser and scoring)
  - id: brentwood
    resource: https://brentwood-trampoline.org/competitions/organisation-of-trampoline-competitions/
    title: Brentwood Trampoline, organisation of trampoline competitions
  - id: cop
    resource: https://www.gymnastics.sport/publicdir/rules/files/en_1.1%20-%20TRA%20CoP%202025-2028.pdf
    title: FIG Trampoline Code of Points 2025–2028
    author: org:fig
  - id: club-software
    resource: https://joinit.com/blog/best-gymnastics-club-management-software
    title: Gymnastics club management software round-up (2026)
  - id: generators
    resource: https://vbeaulieu.com/routine-trampoline/
    title: Vincent Beaulieu's trampoline routine generator
---

# Why

The skill, tariff and requirements engine sits between two groups with needs
nothing serves well yet: **competition organisers and attendees**, and **people
running or training in a club**. The aim is to help with trampoline problems
generally, whoever has them. Everything stays phone-first, plainly worded, and
built on the one engine, so rules exist only in Go ([rendering](architecture/rendering.md)).

# What already exists

- **Entries and live scoring are covered.** TrampOnline handles online entries
  and device-based scoring and results for 300+ organisers since 2012, and
  partners with ISTO, NEUT, the Southern Universities league and Scottish
  Student Trampolining.[^tramponline] TScore handles setup, entries,
  timetabling, tablet scoring with time of flight and horizontal displacement,
  and results.[^tscore] **Complement them first.** Replacing them is a possible
  later direction, not the starting point.
- **Card checking is still done by hand.** Gymnasts hand in competition cards
  (elements and tariff) before the event. Difficulty judges check them before the
  competition starts, and some rule books penalise a late card (0.2 off the
  difficulty).[^cop] The engine
  already does exactly this check.
- **Timetabling rule of thumb:** British Gymnastics allows 4½–6½ minutes per
  competitor for two rounds plus warm-up. A panel is two trampolines with their
  officials.[^brentwood]
- **Club admin tools are plentiful** (bookings, payments, attendance), and so
  are some training tools (session diaries, rotation planners).[^club-software]
  Routine generators exist,[^generators] but none build a routine that meets a
  competition's requirements.

# Competitions: organisers and attendees

| # | Area | What it does | Builds on |
|---|---|---|---|
| 1 | **Card collection and checking** (built, live at `/competitions` since 2026-10-06; [what each person sees](features/competition-entries.md)) | Members keep their entries with their club and change them freely; the club's competition secretary sends them to the competition (individuals can enter directly where allowed); the organiser and difficulty judges see every card already checked against its level, print them, mark them checked, and export CSV; optionally asks for video proof by link (an unlisted YouTube video, say) for chosen skills. Synchro (2026-10-07): levels paired into one event (e.g. L1/L2), a pair entering one routine (the level's voluntary), and a pair at the wrong level for their individual ones flagged; levels kept easiest first, which judging "up to" goes by | requirements, levels, tariff sheet, sharing |
| 3 | Timetable and flight planner (built: every step of [ADR 0005](../docs/adr/0005-multi-discipline-timetabling.md), 2026-10-07: [timetable](features/competition-entries.md#timetable-adr-0005-step-4), [simulation](features/competition-entries.md#simulation-adr-0005-step-6)) | Entries per category, panels and minutes per competitor give flights, running orders and estimated times over days and named areas, around blocked time and the organiser's rules; printable marshal and chair-of-judges sheets; a panel timeline at an even time scale, with or without each official's seat; simulation of entry numbers, judges and the judges each club must bring. On the day: [what if there's a delay](features/competition-entries.md#what-if-theres-a-delay) (shift or move flights, eased by moving or shortening breaks, shorter changeovers, quicker turns, bigger flights or running over) | — |
| 4 | Officials rota (built, ADR 0005 step 5: [officials rota](features/competition-entries.md#officials-rota-adr-0005-step-5)) | Judges, recorders and marshals per panel and flight (spotters aren't scheduled), drawn from competitors and the organiser's own judges, with clashes flagged and own-club judging balanced; HD and synchronisation judges where no machine measures. On the day: [what if an official has to leave](features/competition-entries.md#what-if-an-official-has-to-leave) (each seat filled by someone free or by moving others round, easiest to fill or fewest changes) | 3 |
| 5 | "My competition" page (built: [my competition](features/competition-entries.md#my-competition-roadmap-competitions-5)) | For attendees: flight, panel, time, card status | 1, 3 |
| 6 | Results history | Import results CSVs (TrampOnline, TScore) for personal bests and progression, and for next year's level planning | — |
| 7 | Later, if wanted | Entries, scoring with time of flight and displacement, live results: the "replace" path | 1–6 |
| 2 | Difficulty judge helper (low priority) | Tap elements as they're performed, compared live with the submitted card: changes, interruptions, the new tariff. Hard to make reliable enough to use on the day | picker, search, validation |

# Clubs: coaches, gymnasts, committees

| # | Area | What it does | Builds on |
|---|---|---|---|
| 1 | Club competition secretary | Now part of card collection (competitions 1): members' entries gathered by the club and sent to each competition; still to add: export for TrampOnline entry | competitions 1 |
| 2 | Skill tracking | Coaches tick skills per gymnast (club link); each gymnast sees what's next and which levels they're ready for | catalog |
| 3 | Session planner | Trampolines, gymnasts and session time give a turn rotation with spotters and coach ratio, on a phone | — |
| 4 | Judge and coach practice | Tariff and FIG-notation quizzes, card-checking practice with planted errors; execution practice could build on the "How it's judged" rules ([execution judging](domain/execution.md), built 2026-10-07) | engine |
| 5 | Committee handover kit | Safety checks, incident log and policy templates (mostly documents) | — |
| 6 | Routine suggester (low priority) | From the skills a gymnast can do and a chosen level, suggests legal routines (landing to take-off, no repeats, requirements, caps), highest tariff first. Hard to do well: a legal, high-tariff routine isn't necessarily one a gymnast should compete | skills, requirements, 2 |

# Next for clubs (proposed 2026-10-15)

**Where clubs stand.** Everything a club has today is there for
competitions: members join to enter, coaches sign off entries, the comp sec
sends them. Between competitions there's nothing to do on it. A member's
page opens on Competitions, and is empty until the club enters one; a
coach's page is only entries to sign off, with no menu. What's there to
build on:

- **People are already in place.** Members, coaches (with qualifications),
  who coaches whom (a member's chosen coach, or every coach sees every
  member), and one link for several roles (ADR 0010).
- **Routines live only in the browser.** The routine builder keeps them in
  `localStorage`; on the server a routine exists only inside a competition
  entry. A coach can't see a gymnast's routine until it's entered. A comp
  sec changing an entry for a member picks from routines saved in *their
  own* browser. A new phone, or cleared data, loses them.
- **Nothing records what a gymnast can do.** The catalog and the
  requirements engine already know what each level's set routines and
  voluntaries need.
- **Clubs expire.** A club is deleted, with its members, 120 days after its
  admin or join link was last used (`DeleteExpired`). Over a summer break
  that would take a season's records with it. That's fine while a club holds
  only entries, but not once it keeps anything for the long term.
- **Personal data is names and clubs** (ADR 0004). Routines are already
  stored, with entries. Skill progress, often of under-18s, is new.

**The next two club features, in order:**

1. **Routines on your page** (new; the foundation for 2 and for clubs 6).
   **Built 2026-10-15** ([routines on your page](features/competition-entries.md#routines-on-your-page-adr-0011)).
   Each routine is checked against what it was checked against in the
   builder; a coach can check it against a level they pick, and add
   routines to a gymnast's page (built 2026-10-15).
   - **Member:** a **Routines** tab (in `memberMenu`) with routines kept on
     the server: open one in the routine builder, rename, delete. The
     builder gets **Save to my page** when a member's link is saved in this
     browser, and the tab has **Add routines from this browser** to bring
     existing ones over once. Entering a competition offers these as well as
     the browser's.
   - **Coach:** the coach's page gets a menu: **Entries** (today's page) and
     **Gymnasts**. Gymnasts lists each gymnast they see (by the same rule as
     entries) with their routines, each checked, and a note the gymnast
     sees.
   - **Comp sec:** changing a member's entry offers that member's routines,
     not the comp sec's browser's; Members links to each member's routines.
   - **Storage:** a routine per row, against the member (name, the routine
     as the builder saves it, updated), deleted with the member. A cap of
     30 a member ([ADR 0011](../docs/adr/0011-club-routines-and-skill-tracking.md)
     Decision 1).
2. **Skill tracking** (clubs 2). Designed with routines in
   [ADR 0011](../docs/adr/0011-club-routines-and-skill-tracking.md)
   (accepted 2026-10-15), amending 0004 for the new personal data
   (progress, often of under-18s) and club retention.
   Built in steps, each usable on its own:
   1. **Clubs kept 13 months after last real use** (decided and built
      2026-10-15; member and coach pages already counted, now at most a
      write a day),
      for every club, not 120 days. Any comp sec, coach or member opening
      their pages or changing something counts as use, not only the admin
      and join links as now. The comp sec's Settings says when the club
      would be deleted. Comes first, so no progress is lost over a summer.
   2. **Marking skills** (built 2026-10-15, with 3: [skills](features/competition-entries.md#skills-adr-0011)).
      The gymnast's mark and a coach's both show (decided 2026-10-15), as
      role and state chips from the one palette.
      Any skill the catalog can describe, found with
      the routine builder's picker and search, can be marked **Learning** or
      **Can do**, by the gymnast or by a coach (decided 2026-10-15). Each
      mark says who made it and when ("Can do · marked by Declan (coach),
      3 Oct", "Learning · marked by you"). Each side keeps its own mark;
      the coach can filter for the gymnast's marks they haven't made the
      same, to confirm. The skills in the gymnast's routines (from 1) are
      offered first. A routine with a skill no coach has marked Can do
      says so to the coach (built 2026-10-15).
   3. **Who sees it** (decided 2026-10-15): the gymnast, the coaches who
      see them, and the comp sec, at least. The comp sec can open it to
      **every member of the club** (a setting, off to start with). The
      member's **Skills** tab, a **Skills** page per gymnast under the
      coach's Gymnasts, and the comp sec's Members, each with the shared
      search and filters (by skill, by level, marked by whom).
   4. **The club's own skills** (decided and built 2026-10-15,
      [club skills](features/competition-entries.md#club-skills-adr-0011)). Coaches add skills to
      a **club catalogue**, used for tracking alongside the built-in one:
      - **the club's name for a skill the catalog can describe**, one it
        names already ("Swivel Hips" for Seat Half Twist To Seat) or one
        that shows as "Custom Skill" today. It keeps its notation and
        tariff, so it can be used in routines too, and the club's name is
        shown to the club;
      - **anything else to track** that isn't a trampoline skill in the
        Code of Points (a drill, a skill on another apparatus, a
        conditioning test): a name and an optional note, for tracking only,
        not in routines.
   5. **Club levels** (decided and built 2026-10-15,
      [club levels](features/competition-entries.md#club-levels-adr-0011)). Coaches make levels to track
      gymnasts against ("Club Grade 1"), in order, out of **skills and
      requirements** (decided 2026-10-15), so linking one skill into
      another is tracked as well as each skill. This reuses the
      requirements framework and its editor rather than adding new
      machinery: a club level is a list of requirements in the existing
      JSON format (`requirements.Set`), written in the requirements editor
      with its matchers and plain-English descriptions (`describe.go`),
      saved to the club instead of the browser. Each item is one
      requirement:
      - **a skill**: a matcher with its FIG notation (built-in or a club
        skill from 4);
      - **a kind of skill**: a matcher without one ("any back somersault",
        "a single with a full twist");
      - **a link**: an `includes` option of two or more matchers one
        straight after another ("back drop, then ½ twist to feet");
        several options mean "any of these";
      - **how many**: `count` or `separate` ("three different singles");
      - **a whole set routine**: `sequence`, so a club level can start
        from a competition level's (BUCS L5's set routine, say).

      **How an item counts as done.** Skill and kind-of-skill items, `count`
      and `separate` are checked by the engine against the gymnast's
      Can do skills, taken as a list with no order. A skill's mark counts
      in every level that has it. A link or set routine is marked as a
      whole, like a skill, by the gymnast or a coach, with who and when;
      one of the gymnast's routines (from 1) that already does it (the
      engine's `includes` or `sequence` check) is shown beside it as
      evidence for the coach to mark. Each gymnast sees how far through
      each level they are and what's left. The comp sec's Members, and a
      coach's Gymnasts, filter by level, "completed" (by a coach's marks)
      and "working on" (built 2026-10-15). These are
      tracking levels, separate from the competition levels that check a
      single routine.

   6. **Sharing club levels** (decided and built 2026-10-15,
      [sharing club levels](features/competition-entries.md#sharing-club-levels-adr-0011)). A club level can be
      shared with other clubs by link, as requirements and levels are
      today ([sharing](features/sharing.md)), bringing the club skills it
      uses, so a regional scheme's grades are made once. The club that
      opens the link gets its own copy to track against and change.

**Later (decided 2026-10-15):** a member moving club takes their routines,
marks and progress with them. Not to start with: they stay with the club.

**After these:** the session planner (clubs 3) needs no storage and can be
built at any time, as a page that works like the calculator. Judge and coach
practice (4) builds on the engine alone. The routine suggester (6) can now
start from a gymnast's ticked skills. The TrampOnline entry export (clubs 1)
waits on a sample of TrampOnline's entry file. The handover kit (5) is
documents.

# The requirements workflow (reviewed 2026-10-15; 1 and 2 built 2026-10-15)

A review of writing requirements, from the routine builder and from club
levels, which are built on the same editor. 1 and 2 are built
([the editor](requirements/framework.md#the-editor)); the rest is the plan.

**What's clunky today.**

- **From the routine builder:**
  - **Three places for one idea.** Requirements are written on the
    Requirements page. Set routines are made in the builder (More, Save as a
    set routine). Levels are made on the Requirements page but used in the
    builder's Levels mode. Three nouns and two pages stand between a coach
    and checking a routine against their own competition.
  - **Editing means leaving the routine.** Manage requirements opens the
    Requirements page in a new tab: rules are written without the routine in
    sight, then the coach comes back and picks them under Check against.
  - **Skills are described by numbers, not picked.** "A back somersault" or
    "back drop, then back to feet" means somersault and twist ranges in
    quarters and half twists, then direction, shape, take-off and landing
    (the last two under More conditions). The builder's skill picker and
    search, which everyone already knows, isn't there.
  - **Rule kinds are named after the engine,** not what people mean:
    "Elements that must (or mustn't) appear", "Includes one of…", "Special
    requirements". A link of two skills is an "Includes one of…" rule with
    one option of two steps.
  - **Check against is one long list,** about 50 on a phone, with your own
    requirements at the bottom.
  - **Requirements and levels stay in one browser,** easily lost.
- **From club levels:**
  - **A round trip and two copies.** Requirements are written on the
    Requirements page, saved to the club, and opened back there to change.
    The browser's copy drifts from the club's, and another phone can't
    change a level without that round trip.
  - **The save button depends on the browser.** Save to club levels only
    shows where the coach's link (or the link of a member who coaches) was
    opened in the same browser.
  - **The rule kinds don't fit a club grade.** A grade is mostly a checklist
    (these skills, this link, any three singles, a set routine), but the
    editor offers rules about a whole routine, which are dropped when saved.
  - **Club skills sit outside the requirements,** so they're listed after
    them, not in the order a coach would give.
  - **No one-tap start** from a competition level's set routine, another
    club level, or a shared one.

**Recommended, in order.**

1. **Pick a skill in the rule editor** (helps both). *Built 2026-10-15:* one
   dropdown of kinds of skill (any single, a barani, landing on the
   seat…) and the catalogue's named skills, the number fields under "Or
   describe it"; each step of "this, then this" takes a kind too, so "a
   back somersault, then a barani" prescribes no particular skills (the
   owner's addition). The club's named skills join the list with 3.
2. **Name rule kinds by what they mean, and offer only those that fit.**
   *Built 2026-10-15* for the Requirements page, as listed in the editor;
   the club-level kinds come with 3.
   - In the builder: Must include, Must not include, At least / at most N of
     a kind, Linked (this, then this), No repeats, Number of skills,
     Difficulty between, Skill at position.
   - For club levels only: Skill, Any of these skills, Link, N skills of a
     kind, Set routine, Club skill.
   - Both map onto today's rule types, so the stored format and the engine
     are unchanged. Common starters ("10 skills, no repeats, difficulty
     x–y") could be one tap.
3. **Edit club levels on the club page, saved straight to the club.**
   *Built 2026-10-15:* New level, Change and Copy as a new level on the
   Levels section, the editor with the level kinds and the club's own
   skills to pick, Save level straight to the club. Not yet: the club's
   things to track as items among the rules (still a list after), and
   starting from a competition level's set routine or a shared level.
   - The editor is a server-rendered form, so the coach's Levels page can
     host it, with the club-level kinds from 2: one copy, any phone, and a
     club skill as an item like any other, in order.
   - Start from a competition level's set routine, another club level, or a
     shared one (with sharing, step 6 above).
   - Save to club levels on the Requirements page stays as a way in.
4. **Write requirements without leaving the builder.** At least, after
   saving, offer "Check the routine against these now". Better, edit them in
   a panel under Check against, each rule's ✓ or ✗ for the routine on screen
   as it's written.
5. **A better Check against picker:** your own and recently used first,
   then by organisation and level, with a search like the skill picker's.
   The same picker would serve Start from a set….
6. **Later:** keep requirements and levels with the person, as routines on a
   club page are, rather than in one browser. *Suggested 2026-10-15:* build
   it with 3, as one change: the editor saving to where it's opened from,
   the person or the club. And sooner than later: Safari on an iPhone
   clears a site's browser storage after 7 days without a visit (unless
   it's on the home screen), so requirements and levels kept only in the
   browser can be lost. Needs a short ADR, as it amends ADR 0003.

1 and 2 give the most for the least: rule writing gets easier in both
places with no change to the engine or storage. 3 removes the club round
trip.

# Decisions so far (2026-10-05)

- Complement TrampOnline and TScore first, and keep replacement possible.
- Anything stored on the server uses **secret links, no accounts**: an admin
  link and a share or submit link, like a form. Personal data is kept to names
  and clubs, and a competition's entries are deleted 120 days after it
  (sooner if the organiser chooses). Accounts can come later.
- **Competition card collection is the first thing to build.** Its design goes
  in [ADR 0004](../docs/adr/0004-server-storage-secret-links.md) (server storage with secret links), amending ADR 0001's plan for
  accounts.
- **Storage is live (2026-10-06).** The app runs on the self-hosted server at
  `tariff.pardy.ie` (Render's free service had no persistent disk), with
  competition entries in SQLite on its `/srv/data/tariff` volume, backed up
  nightly ([deploy](operations/deploy.md#competition-storage)). Card
  collection (competitions 1) is built and live at `/competitions`, with coach
  sign-off; the calculator doesn't link to it yet.
- **Priorities (2026-10-05):** card collection first. The routine suggester
  drops to the bottom, as it would be hard to do right. ISTO's levels won't
  be available for a while, so building them in waits; users can write them
  as their own requirements meanwhile.
- **The aim is ISTO (2026-10-06).** The competition tools work towards running
  the Irish Student Trampoline Open's day: trampoline, synchro, tumbling and
  DMT, with competitors judging and a venue that must finish on time
  ([ADR 0005](../docs/adr/0005-multi-discipline-timetabling.md), accepted).
- **Priorities (2026-10-06):** card collection is live. Next is the timetable
  and flight planner (competitions 3), then the officials rota (4). The
  difficulty judge helper (2) drops to the bottom: it would be hard to get
  right enough to be useful on the day, and there's easier value elsewhere.
- **Next for the timetable (from the demo, 2026-10-06; built 2026-10-07 as
  the [panel timeline](features/competition-entries.md#timetable-adr-0005-step-4)):**
  a timeline per panel as well as the rota per person. One view (printable, and as a CSV)
  with the days side by side (Friday, Saturday, Sunday), a column per area
  (Panels 1–3, Track 1, DMT 1) and time running down, showing what's on
  when: each flight's event, warm-up to finish, who is officiating it (chair,
  difficulty, HD and execution judges, recorder, marshal), and blocked time
  (lunch, awards, with any officials it needs). The grid at the bottom of
  the timetable page is close, but it's for editing: on screen only, days
  one under another, and each area a list of flights rather than a time
  scale, so areas don't line up by time.
- **Where things are (2026-10-07).** Every step of ADR 0005 is built and
  live: events across disciplines, officials and the rota, the timetable over
  days and areas, and simulation. Since then: the panel timeline (with and
  without officials), synchro as ISTO runs it (paired levels, one routine, a
  pair's level from their individual ones, synchronisation judges), clearer
  reports (when flights end and the time free after them; empty seats
  counted), and two on-the-day "what ifs", a delay and an official leaving,
  neither of which changes the timetable. The demo is a 25-club, 400-gymnast
  ISTO over three days. The fix for judging "up to" a BUCS level (levels
  were hardest first) went out the same day.
- **Keep the name for now.** Competition and club tools stay on
  `tariff.pardy.ie` even though they go beyond tariffs; a broader name can
  come later.

# Next steps (2026-10-08)

**Tried with an ISTO organiser (2026-10-08).** Their two big asks were
approving coaches and removing entries, built that day after the biggest
priority, one panel for all of an event's flights (all but splitting an
event by routine, now 10).

**Priorities (2026-10-08, evening).** ISTO 2027's entries open early in
2027 and the competition is early April 2027. For now the tools are being
demoed to show they'd be useful, so for the next week or two the work is
features that show that, in this order:

1. **More admin links, each able to do less, and a change history** (built
   2026-10-08, [ADR 0009](../docs/adr/0009-helper-links.md)): checking
   cards, chairs of judges, timetable, and co-organiser links, and concerns
   any helper can flag for the organiser.
2. **Entry fees** (built 2026-10-08,
   [entry fees](features/competition-entries.md#entry-fees)): per entry by
   discipline and per club, payments recorded, invoices for clubs and
   individuals.
3. **Limits per level, with a waiting list** (built 2026-10-08,
   [limits and waiting lists](features/competition-entries.md#limits-and-waiting-lists)).
4. **Late changes** (asked and built 2026-10-08,
   [late changes](features/competition-entries.md#late-changes)): after the
   deadline clubs and individuals ask for the changes the organiser allows
   (a level change, new routines), each with its own fee charged if
   accepted, optionally signed off by a coach first; the organiser sees
   whether it fits (limits, the draft timetable) and accepts it or says
   why not. Changes to entries and coaches' sign-offs can close later than
   new entries (built 2026-10-09). Late new entries could follow.
5. **The day itself,** as far as it goes: flights started and finished by
   the marshal (built 2026-10-09,
   [on the day](features/competition-entries.md#on-the-day-flights-started-and-finished):
   planned against actual, how late each area is, shown to attendees and
   feeding the delay "what if"), check-in and scratches (built 2026-10-09,
   same page: marked here or scratched, struck through on the printed
   sheets, a scratched official flagged), panel check-in and a Notify
   button (built 2026-10-09, same page: officials marked here or missing for
   their run, and a message to someone not here or their club's staff),
   messages from the organisers'
   desk (built 2026-10-09,
   [desk messages](features/competition-entries.md#messages-from-the-organisers-desk):
   to a club, a flight, the officials or picked people, by push and email at
   once and on each person's page), a "now on" screen (built 2026-10-09,
   [the venue screen](features/competition-entries.md#the-venue-screen):
   a Venue screen link shows what's on each area now and next), pages that
   work offline (built 2026-10-09,
   [offline](features/competition-entries.md#offline): a person's opened
   pages open without a connection, as last seen), add to calendar.

**Next (asked 2026-10-09 and 2026-10-13), before the rest.** The menus come early because they touch every page, so the new people pages go straight into the new layout.

1. **Check-in keeps the flight open** (built 2026-10-14): marking someone
   Here, Scratched or Clear on the On the day page updates the page in place,
   so the flight's check-in list stays open and the page doesn't move; without
   JavaScript the page reloads at that flight's box. See
   [competition entries](features/competition-entries.md#on-the-day-flights-started-and-finished).
2. **Finding your way round (asked 2026-10-13):**
   - **A menu for the organiser**, with sections and subsections:
     entries, timetable, coaches, panels and officials, routines and
     competition settings. **Built 2026-10-14**: Entries, Timetable, On
     the day and Settings, each with its pages, on every organiser page.
   - **Links and settings on a page of their own**, not a section of
     the entries page. **Built 2026-10-14**: the Settings page.
   - **The same for clubs and gymnasts entering on their own**, with a
     menu in place of one long page. **Built 2026-10-14 for clubs:** a
     comp sec's page is Competitions, Members, Coaches and Settings.
     Gymnasts entering on their own have one competition, so no menu
     yet.
   - **Individuals: a Competitions tab** listing their competitions, each
     opening that competition's page. Within it, each entry is collapsed
     by default and the list view comes first. Other tabs come later,
     such as routines, or a coach's view once one link covers several
     roles. **Built 2026-10-14 for club members:** Competitions (a row
     for each, opening its page, with each discipline a collapsed row)
     and Your details; more tabs go in `memberMenu`
     (`web/menus_club.go`). The coach's page and gymnasts entering on
     their own got the same with item 5 (role tabs, Your details).
3. **Make dropdowns look like dropdowns** (asked 2026-10-13, built
   2026-10-14). **Limits and waiting lists** (now a plain group on the Settings page)
   opened when tapped but showed no arrow, because Bulma's `label` class
   on its `<summary>` makes it a block and hides the marker; the
   dashboard's level headings (`title`) had the same problem. Every
   `<summary>` in all 52 places now shows one arrow, right when closed
   and down when open, from a single rule in `static/css/styles.css`;
   summaries styled as buttons, the routine menu and the inline "move"
   link opt out (see [rendering](architecture/rendering.md#dropdowns)).
4. **All people, for the organiser**: a list of everyone at the
   competition, one row a person whatever their roles, which the coaches,
   officials and entries lists link to. Each person's page shows everything
   they're doing, like their own Your competition page, read-only.
   **Built 2026-10-14**: People under Entries in the menu, a page for each
   person, and links to it from the dashboard's tables, the Officials page
   and the Coaches page. A coach who is also a member was two people
   until item 5 joined them (built 2026-10-14). See
   [competition entries](features/competition-entries.md#all-people-roadmap-2026-10-14).
5. **One link for a person with several roles**: a member who also coaches
   or is comp sec gets one link, with tabs between their views. The comp
   sec can make a member a coach too, and anyone can add another of their
   links to their page to join them. A join can be undone.
   **Gymnasts entering on their own too** (asked 2026-10-14). Today each
   discipline and each competition entered on your own gets its own link,
   and Your competition groups them by the name typed, so a misspelling
   splits them and two people with one name are merged. Instead they get
   a personal page like a club member's, without the club: Competitions
   (each competition's page with a row per discipline) and Your details.
   Another discipline is entered from that page under the same link; a
   new competition's individual entry link offers to enter as the person
   whose link is saved in this browser (or whose link they paste).
   Existing entries join their page with the same "add your other link"
   step. Entries are grouped by the person, not the name, and every
   entry link already sent keeps working, opening that entry on their
   page. **Built 2026-10-14** (ADR 0010): tabs for each role above every
   page a personal link opens; Your other links to add a link and undo
   it; Make a coach too for the comp sec; a personal page for gymnasts
   entering on their own, which a competition's entry link offers to
   enter as; All people shows a person's roles as one row. Shared links
   (a club's or competition's admin link, a helper's) open from a
   personal page but never the other way. See
   [competition entries](features/competition-entries.md#one-link-several-roles-adr-0010).
6. **Search and filter every list of people**: a name search on each, and
   filters that suit the list (for officials: club, role, event, level,
   qualified). Long lists can be collapsed too, starting with the club's
   Members list (asked 2026-10-13). **Built 2026-10-14:** one shared search
   and filters on every list of people: the dashboard's entries (still
   filtered on the server, for printing), All people, Officials, the
   organiser's Coaches, the desk's People I pick, the club's Members and
   Coaches, and a coach's own entries
   ([how](architecture/rendering.md#lists-of-people-search-and-filters)).
7. **Venue screen changes**: **built 2026-10-14.** Colours per area, cards
   that fill the screen for up to seven busy areas (quiet ones as strips),
   gymnasts with clubs and running numbers for now and next, lateness beside
   the area's name with times kept to the timetable, announcements from the
   desk, a screen per group of areas, and keeping the last update when the
   wifi drops. Who's up now, tapped through by the marshal, shows who's on,
   their routine and the next two. Refine after the dry run.

**Review and cleanup stop: done 2026-10-15.** A full audit of the code,
pages, tests, docs and running it ([the report](audits/2026-10-cleanup.md))
and every finding fixed, apart from those left below for the dry-run work or
after it. Among them: links a comp sec makes open only that club's records;
certificate limits; no secret link stored or logged; timetable edits that
can't overwrite each other; stable flight ids for the day's marks; one person
across roles in the timetable; taps safe to repeat on poor wifi; Go 1.26 with
govulncheck in CI; `/healthz` and a graceful shutdown; an end-to-end test on a
fixed clock; the timetable page 6.1 MB to 0.8 MB; search on the day and
timetable pages; wide tables as cards on a phone.

**One palette for states and roles (planned 2026-10-15).** Fixed colours for
states (grey not yet, blue in progress, green done, amber needs attention,
red stopped) and for who did something (gymnast, coach, club, organiser),
each always with a short label, used the same way on every page. The shared
pieces come first, so skill tracking (ADR 0011) uses them from the start; the
competition side's day check-in, entry rows and rota seats follow in the
dry-run work, and the rest after it. The plan is in
[the cleanup audit](audits/2026-10-cleanup.md#colour-and-status-added-15-october-2026).
The shared pieces and the dry-run pages are **built 2026-10-15**.

**Ready for the dry run (decided 2026-10-09).** A dry run of the core tools,
entries through to the day, with about 20 people from 3 clubs, is planned for
late 2026: entries from mid November, the day in late November or early
December. Before it:

- **Faster pages** (built 2026-10-15). Every load read all the entries
  again and worked out the officials rota and every card's checks from
  scratch; now each competition's are kept until something is saved. On
  the full demo a gymnast's own page went from 65 ms to 6 ms, the
  dashboard from 61 ms to 7 ms and a club's page from 58 ms to 18 ms
  ([how](architecture/http-routes.md)).
- **Smaller pages** (built 2026-10-15). The app gzips its own text
  responses and, once, its static files ([compression](architecture/static-assets.md#compression)):
  on the demo the organiser's dashboard went from 674 KB to 57 KB, On the
  day from 1.0 MB to 54 KB, the timetable from 1.0 MB to 67 KB, a
  gymnast's page from 109 KB to 11 KB and a club's from 41 KB to 4 KB.
- **A Feedback button** on every competition page while the dry run is on,
  saving the words, the page, the role and the time for the maintainer.
  Helper links already have Flag a concern; this is for everyone. *Built
  2026-10-15*
  ([feedback](features/competition-entries.md#feedback-about-the-app-ready-for-the-dry-run)):
  the organiser reads it and can turn it off; `FEEDBACK_EMAIL` emails it.
- **Send in the background** (built 2026-10-15). A desk message, or a round of change
  notifications, emailed and pushed each person one at a time before the
  organiser's page answered: about half a second to a second an email, so
  a message to 300 people hung for minutes and might time out part way
  through. Now four go at a time in the background, emails capped a
  second, the page says at once how many it's going to, and the desk shows
  sent, still sending and failed.
- **Email on the server**: Amazon SES in Ireland (decided 2026-10-09:
  pennies a competition and no daily cap that matters, unlike the free
  tiers of Resend or Brevo; Postmark is the fallback if SES refuses
  production access). Sending from `tariff.pardy.ie`, with DKIM, SPF on a
  MAIL FROM subdomain and a DMARC record starting at `p=none`; apply for
  production access early. The steps are in
  [deploy](operations/deploy.md#notifications).
- **Taps without signal**: Started, Finished, Here and Scratched saved on
  the phone with the time they were tapped, and sent when the signal
  comes back; and a way to correct a start or finish time afterwards,
  for when the paper sheet was used instead. *Built 2026-10-15*
  ([taps without signal](features/competition-entries.md#on-the-day-flights-started-and-finished)):
  every tap on On the day, who's up included, recorded at the time tapped
  by the server's clock, with rules for two phones' taps, and **Correct the
  times** under each flight.
- **One palette on the day's pages**: **built 2026-10-15.** On the day
  (flights, check-in, panels and their empty seats), the dashboard's and the
  club's entry rows and who fills each rota seat show states and roles with
  the shared chips, and a test keeps Bulma's colours off them
  ([what each shows](features/competition-entries.md#states-and-roles-on-the-pages-one-palette-step-2)).
- **A load test**: a script acting as 300 people against a test copy on
  the server, to get real numbers before the dry run. The script is built
  (2026-10-15, [load test](operations/deploy.md#load-test)): on a laptop
  held to two cores, 400 people opening their pages within 2 seconds went
  from 191 of 343 failing to none, with a median of 171 ms. Still to do: a
  run on the server.

For ISTO itself: a 4G/5G router for the organiser, marshals and chairs
(and a cable for the venue screen); everyone's times by push and email
the night before, so their page is saved on their phone before they
arrive; and the server scaled up for the weekend (Hetzner charges by the
hour; keep the disk size so it can scale back down; a minute or two
offline, so do it the day before). The March rehearsal runs on a copy of
the real competition with its entries, at the venue if possible, with a
script standing in for the crowd and 15 minutes with the app switched off
to practise the paper fallback; copying a competition with its entries
builds on copying last year's.

Then, by when ISTO needs them: link the calculator (before entries open),
the "what ifs'" missing checks (8) and saying what a manual change breaks
(9), judges' qualifications, and details like better defaults (14);
splitting an event by routine (10) if ISTO's timetable needs it. A dry run of the core tools in late 2026 (above), a full rehearsal of the day in March, and nothing but fixes in the last two weeks.
After ISTO: copying last year's competition, results history, the club
tools and the possible additions.

Notifications (7) are built; numbers 1–7 below are built.

1. **Approved coaches** (decided 2026-10-07; designed in
   [ADR 0007](../docs/adr/0007-approved-coaches.md); built 2026-10-08).
   Where a competition needs
   coach sign-off, the organiser can also require coaches to be
   **approved**, and pick the qualification they need from a list (British
   Gymnastics and Gymnastics Ireland coaching levels, by discipline; a
   higher level counts for a lower one). Each club sends its
   coaches to the competition with a **certificate** (a photo or PDF); an
   individual names their coach and uploads theirs. The organiser approves
   each coach for that competition alone, and an unapproved coach can't sign
   off (nor can an individual's coach through their sign-off link). If the
   organiser withdraws an approval, that coach's sign-offs stop counting and
   the entries need signing off again. Certificates are a change to
   [ADR 0004](../docs/adr/0004-server-storage-secret-links.md), which stores
   no documents. A club's certificate is kept with the club and sent with
   the coach to each competition (approved afresh each time) until the
   coach is removed; an individual's goes with the competition. Only the
   organisers it's sent to see it (see
   [open questions](open-questions.md#approved-coaches)).
2. **Organisers remove entries** (decided and built 2026-10-08). The organiser ticks
   entries on the dashboard, one by one or a whole club at once, and either
   **removes** them, so they can't be sent again, or puts them **on hold**,
   asking for changes: the club or individual changes the entry and sends
   it again, and the organiser accepts it back in. Either way there's an
   optional reason, shown to the club, the
   member, or both. Removed and held entries aren't counted, printed or
   timetabled, and stay in a list the organiser can **restore** them from.
   Today only the club or the gymnast can withdraw an entry; the organiser
   can only take someone out of a flight.
3. **Find your way round the entries** (asked and built 2026-10-08). The organiser's
   dashboard is one long table per level; with hundreds of entries it needs
   more ways in:
   - **collapse and expand** each level (or event), and jump to one from a
     list at the top, with its count;
   - **search for a person** by name, across every level (gymnast or synchro
     partner);
   - **filter** by coach, checked or not, signed off or not, video (missing,
     to review, need more), men or women, and changed since checked or
     withdrawn, beside today's club and problems only; filters combine, and
     stay in the link so a view can be shared or kept;
   - **sort** a level's table by gymnast, club, sent or coach;
   - counts that update with the filters ("12 of 96 shown").
   Levels start collapsed, with Expand all and Collapse all, and the
   browser remembers which were left open; a search or filter opens the
   levels it finds people in.
   The same filters could serve the printed cards and the CSV ("print what
   I'm looking at").
4. **Personal and club timetables** (asked and built 2026-10-08). A timetable laid
   out like the panel timeline (days side by side, a column per area, time
   running down evenly) for:
   - **a person**: everywhere they are, competing or officiating, picked
     out, with the rest faint (or hidden, to see just their own day);
   - **a club**: every flight one of its gymnasts competes in, or one of its
     people officiates on, highlighted, with who; for the comp sec, coaches
     and members to see the club's day at a glance.
   Both printable. Today "My competition" lists a person's flights and shows
   the whole timetable with them in bold, area by area rather than on one
   time scale, and a club has no timetable of its own.
5. **Synchro partners see the pair's entry** (decided and built 2026-10-08). A synchro
   entry belongs to the gymnast who entered it; today the partner only
   confirms they're the partner, and neither they, their coach nor their
   club sees it again. Once the partner confirms, the entry shows in the
   partner's own entries (a member's page, or an individual's), and their
   coach and their club's comp sec see it on their pages, marked as entered
   by the partner (and their club, if it's another). The entrant still
   enters and changes it; for the partner's side it's to see, as with any
   entry: its checks, sign-off and, once published, their flight. Also warn
   both gymnasts on their own pages, not only the organiser, when a pair is
   at the wrong level. ("The lower level" for a pair a level apart means the
   easier one, as built: confirmed 2026-10-07.)
6. **Set up in private, go live when ready** (decided 2026-10-07; built
   2026-10-08: [going live](features/competition-entries.md#going-live)). A new
   competition starts **private**: the organiser sets up levels, events,
   officials and the timetable, and the club and individual entry links
   show the competition's name, date and when entries open, so they can be
   shared early, but take no entries. It goes **live** when the organiser
   presses Go live, or at a date and time they set. The organiser can make
   a live competition private again to pause entries, keeping those already
   made. (Before this, a competition took entries from the moment it was
   created until its deadline; those made before stay live.)
7. **Notifications, by opting in** (decided 2026-10-07; designed in
   [ADR 0008](../docs/adr/0008-notifications.md); email and push built 2026-10-08,
   [notifications](features/competition-entries.md#notifications-adr-0008);
   email waits on a provider on the server). Members,
   individuals, club comp secs and coaches can each ask to be told when
   something affects them: a flight they're in moves (time, area or day, or
   its warm-up), their officiating duties change, or their card is checked
   or found to have a problem. A comp sec hears about the club's members, a
   coach about the gymnasts they coach. Timetable and duty changes go out
   when the organiser **publishes**, comparing the new timetable with the
   last one and telling only those whose part changed; running order
   changes alone don't notify. After a change goes live there's a **grace
   period** before anyone is told, and anything else published in it joins
   the same notification. Each one compares what was live before the first
   change with what's live at the end, so a change made and then undone
   tells no one. The organiser can skip the wait with **Notify now**, or
   turn the grace period off for the competition's days, when news can't
   wait. Notifications are **grouped per person told**: a club gets
   one email however many of its members changed, and a gymnast one however
   many of their events. Each **summarises the changes** (who, which event,
   was and now) and links to their page. Notified by **phone push** where it works
   (on iPhone, once the site is on the home screen) and by **email**, which
   clubs always have. Emails are a change to
   [ADR 0004](../docs/adr/0004-server-storage-secret-links.md), which keeps
   none: they'd be given only to be notified and deleted with the
   competition, with a tick box, "I'm 18 or over, or this is a parent's
   email". Push stores nothing personal, so anyone can use it (see
   [open questions](open-questions.md#notifications)).
8. **Draft and published timetables, and applying a "what if"** (decided
   2026-10-07; drafts, officials in them, and keeping a "what if" built
   2026-10-08; the "what ifs'" missing checks still to do). Once a timetable is published, attendees see that copy and
   the organiser works on a **draft**: every change (moving a flight,
   redrawing an order, planning again, keeping a delay's or a leaving
   official's result) goes into the draft, and **Publish** makes it what
   attendees see. The officials go in the draft too (2026-10-08): assigning
   officials again or changing a seat waits for **Publish** like any other
   change, so officials see their duties change only then. Today the
   published timetable is the one being edited, so every change goes live
   straight away. Also close the gaps the "what ifs" don't check yet:
   coaches in two places, the organiser's rules about events and people,
   officials on blocked time, and rest as a must.
9. **Say what a manual change breaks, and how to fix it** (proposed
   2026-10-08). Today a manual change (moving a flight or a gymnast, giving
   someone a seat) is made, and what it broke is listed afterwards under
   "Manual changes have broken". Instead, making a change says at once what
   it causes ("Eoin Hughes would officiate BUCS L7 Men and Tumbling
   Intermediate Women at once") and offers fixes where there are some
   (someone free to take Eoin's tumbling seat; another time for the
   flight), to apply or to make the change anyway. With drafts (8), nothing
   reaches attendees until it's published.
10. **One panel for a whole event** (decided 2026-10-07; designed in
   [ADR 0006](../docs/adr/0006-one-panel-per-event.md); runs and
   one panel per run built 2026-10-08; splitting by routine is left). Everyone on an
   event's panel, recorders and marshals too, stays for all of its flights,
   so every gymnast in it is judged by the same people (the organiser can
   let recorders, or marshals, change between flights). For that, the
   timetable runs an event's flights **back to back on one area**, a must
   rather than today's preference, not across lunch unless the organiser
   allows it, and the rota fills the panel once for the
   event instead of flight by flight. An event too big for one panel in the
   time there is is reported with fixes, as anything else that doesn't fit.
   Rarely, and only if the organiser chooses, an event can be split by
   routine instead: everyone does their first routine on one panel and
   their second on another, each panel's officials staying for their
   round. Today both rounds are one flight and the rota only prefers to keep
   a panel together through an area's flights, so an event's flights can
   land on different days and panels with different judges. Also a fix:
   **number flights in the order they run** (BUCS L7 Men's "flight 2 of 5"
   now runs before its flight 1).
11. **Link the calculator to the competition tools.** Nothing on the routine
   builder leads to `/competitions` yet.
12. **Results history** (competitions 6): import TrampOnline and TScore
   results CSVs for personal bests, progression and next year's levels.
13. **Clubs:** the TrampOnline entry export (clubs 1), then skill tracking
    (clubs 2).
14. **Better defaults** from ISTO: minutes per competitor for tumbling and DMT,
    and the usual rest between turns (both guesses for now).

**Added 2026-10-08** (all wanted; ordered under Priorities above):

- **Recorders' score sheets** (asked and built 2026-10-08): ISTO records
  scores online and on paper, so the timetable prints a landscape sheet a
  flight, a row per gymnast in running order and a box for each judge's
  mark of each routine, left to fill in. No competitor numbers.
- **On the day:**
  - **Flights started and finished**, tapped by the marshal: planned and
    actual times side by side, so the delay "what if" fills itself in,
    attendees see "running 20 min late", and it can notify.
  - **Check-in and scratches**: the marshal marks who's there; a no-show
    comes out of the running order and the rota.
  - **A "now on" screen** for the venue: what's on each panel, and what's
    warming up next. *Built 2026-10-09* as a Venue screen link
    ([the venue screen](features/competition-entries.md#the-venue-screen)).
  - **Pages that work offline** (built 2026-10-09): the service worker
    (from notifications) keeps a person's own pages for poor venue wifi.
  - **Messages from the organisers' desk** (asked 2026-10-08, built
    2026-10-09): the organiser sends a message, or asks someone to come to the desk, by push
    and email at once, without waiting, to more than clubs: a **club** (its
    comp sec, coaches, or everyone who asked to hear), **individuals**,
    everyone in a **flight**, **officials** (all, or a panel), or an **ad
    hoc group** the organiser picks. Builds on notifications.
- **Organising:**
  - **Copy last year's competition**: levels, events, officials, timetable
    setup and rules, without the entries.
  - **More admin links, each able to do less**: for example a card-checking
    link for difficulty judges that can't change settings, and a timetable
    link. Today one link does everything.
  - **A change history**: who changed what, and when, once more than one
    link can.
  - **Entry fees**: each club marked paid or not, with an invoice per club
    to print. No money is handled.
  - **Limits per level, with a waiting list.**
  - **Judges' qualifications**, approved like coaches'.
- **For attendees:**
  - **Add to calendar**: a person's or club's flights and duties as a
    calendar link that updates when the timetable is published again
    (built 2026-10-09).

# Possible additions (not planned yet)

Ideas to come back to (2026-10-08), not decided:

- **A coach per discipline.** A member (or the comp sec) chooses a coach for
  each discipline, e.g. trampoline with Ann and tumbling with Bob, and that
  coach sees and signs off that entry. Today a member has one coach, and a
  second coach can sign off only where the club lets every coach see every
  member. (Individuals already name a coach per entry.)
- **Synchro signed off by both partners' coaches.** A pair's entry counts as
  signed off only when each gymnast's coach has signed it off, as fits "has
  seen them perform it safely" for a pair from different coaches or clubs.
  Today only the entrant's coach signs it off. Builds on the partner seeing
  the entry (next steps).

- **ISTO's synchro, DMT and tumbling rules** (from its 2025 routines,
  2026-10-08). A gymnast may enter synchro **twice** (two partners); today
  it's one entry per gymnast per discipline. Synchro pairs may do **the set
  routine or** the lower level's voluntary; today synchro checks the
  voluntary only. DMT (Levels 1–6: a mounter, spotter and dismount per pass,
  or round difficulty limits) and tumbling (Levels 1–4: set passes, or 5–8
  elements with a required twist) aren't checked.
- **Results** for Elite and Elite-Pro: two voluntaries, the higher-scoring
  ranks, with both tariffs recorded for ties.

See [open questions](open-questions.md) for what's waiting on documents and
decisions.

[^tramponline]: TrampOnline (online entries and scoring)
[^tscore]: TScore (competition organiser and scoring)
[^brentwood]: Brentwood Trampoline, organisation of trampoline competitions
[^cop]: FIG Trampoline Code of Points 2025–2028
[^club-software]: Gymnastics club management software round-up (2026)
[^generators]: Vincent Beaulieu's trampoline routine generator
