# ADR 0004 — Server storage with secret links, no accounts

- **Status:** Accepted
- **Date:** 2026-10-05 (Decision 10, video proof, added the same day;
  Decisions 3 and 5 clarified 2026-10-06)
- **Deciders:** jackpardy (solo maintainer)
- **Amends:** ADR 0001 §3 (persistence) and its roadmap, which tied storage to
  accounts ("accounts only when save routines forces it")

## Context

Everything the app does today happens in one browser. Routines, requirements
and levels live in `localStorage`, and the server is stateless: each request
carries what it needs (ADR 0002 §3). Sharing works by putting the data in a
link's fragment, so nothing is stored on the server.

The roadmap (`knowledge/roadmap.md`) widens the project to competition
organisers and clubs. Its first build is **competition card collection**:
gymnasts, or a club's competition secretary, send their routines before a
competition, and the organiser and difficulty judges see every card already
checked against its level. Today judges check cards by hand before the start.

Most entries reach a competition through a **club**. The club's competition
secretary gathers members' levels and cards, today through group chats and
spreadsheets, and enters them for the club. A few gymnasts enter on their own.
Members' plans change as a competition approaches: a different level, or a
different routine for the same level, and each competition can be different.

Card collection needs data that **several people share**: an organiser, many
gymnasts, and judges, on different devices. A share link can't do this,
because the organiser has to receive submissions. So the app needs storage on
the server.

Constraints:

- **No accounts yet.** Users are mostly students on phones who'd use this a few
  times a season. Sign-up friction would cost submissions, and accounts bring
  passwords, recovery and more personal data to look after.
- **Some competitors are under 18.** Personal data should be minimal and
  short-lived.
- **The engine stays the single source of truth** (ADR 0001 §1, ADR 0002 §2).
  A stored card must be checked exactly as the routine builder checks it.
- **Hosting:** Render's free web service has no persistent disk, so a file
  written there disappears on every deploy. The self-hosted server (see the
  server repository) will have volumes and nightly backups, but it waits on a
  domain.
- One maintainer, small scale: hundreds of entries per competition, a handful
  of competitions at once.

## Decision

1. **SQLite, in one file.** Use pure-Go `modernc.org/sqlite`, so the binary
   stays static with no cgo. The file lives in `DATA_DIR` (default `./data`) and
   runs in WAL mode. The schema is created and upgraded by numbered migrations
   in code, run at start-up. Queries are plain `database/sql` in a `store`
   package; `sqlc` (ADR 0001 §3) is deferred until there are enough queries to
   need it. SQL keeps to what Postgres also accepts, so moving later stays
   cheap.

2. **Clubs sit between members and competitions.**
   - A **club** is created by its competition secretary and lasts a season.
     Members join it through a link shared in the club's chat.
   - A **member entry** is one member's entry for one competition: a level,
     the routine(s) for it and their checks. A member can have entries for
     several competitions, and can **replace** any of them (a different level,
     or a different routine for the same level) until the deadline. Each change
     is checked straight away.
   - The comp sec **sends** the club's entries to a competition. The
     competition keeps a copy as sent. A member's later change shows on the club
     page as "changed since sent", and the comp sec re-sends (all, or just the
     changed ones) until the competition's deadline. So the organiser only sees
     what the club has sent.
   - **Individuals** can enter a competition directly through its individual
     link, which the organiser can switch off. Their entries are marked
     "individual" and listed apart from clubs.

3. **Secret links instead of accounts.** Each stored thing has an unguessable
   token for each role, used in its URL:

   | Link | Who has it | Lets you | Stored as |
   |---|---|---|---|
   | Competition admin | Organiser | See and manage all entries, print, export, lock, delete | SHA-256 hash |
   | Club entry | Comp secs (from the organiser) | Attach a club to the competition and send its entries | plain (shared) |
   | Individual entry | Anyone (optional) | Enter as an individual | plain (shared) |
   | Club admin | Comp sec | See members and their entries, send them, edit on a member's behalf, remove members | SHA-256 hash |
   | Member join | The club's members | Join the club | plain (shared) |
   | Member (personal) | One member | Add, replace or withdraw their own entries | SHA-256 hash |

   **Why members need a personal link:** the join link is shared with the
   whole club, so on its own anyone in the club chat could change anyone's
   entry. To keep personal links painless:
   - a member's link is **saved in their browser automatically** (as charades
     does with device tokens), so on their own phone their entries are simply
     there. The link only matters on a new phone or after clearing the browser;
   - the club admin page can **give any member a new link** to send them, which
     ends their old one (only hashes are kept, so a link can't be shown
     again), and the comp sec can edit an entry on a member's behalf;
   - a member who loses their link can be removed and re-join.

   Secret tokens are 128 bits from `crypto/rand`, written as base64url. Only
   their hashes are stored, so a copied database doesn't give anyone access.
   Hashes are compared in constant time. A wrong token gets **404**, exactly as
   a missing one does. A new admin link is shown once, with "save this link" and
   a copy button, and its holder can replace it, which ends the old one.

4. **The domain stays framework-free.** A new `competitions` package holds the
   model (competition, club, member, entry), its validation and the checking
   of an entry. It imports `skills` and `requirements`, and nothing about HTTP, SQL or
   templates. The level checking in `main.go` (`checkRoutine`, `checkPosted` and
   the carry-over between exercises) moves into a function that both the
   routine view and stored entries call. That way an entry's check can't drift
   from what the gymnast saw in the builder.

5. **Stored data is canonical and checked on read.** An entry stores its
   routines as `TrampolineSkill` JSON, as the browser does (ADR 0001 §6).
   It doesn't store the gymnast's own choice of checks: a card is checked with
   the checks its requirements set (difficulty, repeats, scored elements), so
   a gymnast can't turn one off. Names, tariffs and results are worked out
   again whenever the entry is shown, so engine fixes apply to stored entries
   too. A competition
   stores a **copy** of any custom requirements or levels it offers, because
   references to a browser's saved items can't be resolved on the server.
   Built-in requirements are stored by reference (`builtin:<id>`) and resolved
   from the binary.

6. **Minimal personal data, deleted automatically.** An entry holds the
   gymnast's name, club, level and routines, and any video links (Decision
   10); a club holds its name and its members' names. There are no emails, dates of
   birth or contact details. A competition and its entries are deleted **120
   days after the competition date**, and the organiser can delete them sooner.
   A club and its members' entries are deleted **120 days after the club was last
   used**; the comp sec can delete the club or remove a member at any time.
   The submit page says what is stored, who can see it and when it's deleted.

7. **Abuse limits**, as in charades: a cap on competitions and clubs created per
   IP per hour, a cap on members per club, a cap on entries per competition, and the existing 64 KB body limit.
   Behind Caddy the client IP is the rightmost `X-Forwarded-For` value.

8. **Storage features ship only on durable storage.** Until the app runs where
   `DATA_DIR` survives deploys, the pages that use storage are built and tested
   but not linked from the app. Durable means the self-hosted server, with a
   `/srv/data/tariff` volume in its nightly backup, or a paid Render disk in the
   meantime. If storage can't be opened, the calculator's stateless pages keep
   working and only the storage pages report the problem.

9. **Accounts can come later without undoing this.** If clubs need records that
   last several seasons, an account can own the links it created. Nothing here
   rules out the magic-link accounts ADR 0001 planned.

10. **Video proof is a link, never an upload.** A competition can ask for
    proof that a gymnast can perform their routine safely:
    - The organiser chooses **none**, **for some skills** or **whole routine**.
      "Some skills" is described with the requirements framework's skill
      matchers (e.g. "any triple", "tariff 1.5 or more"), so each entry shows
      exactly which of its skills need video ("Video needed for: Triple
      Back").
    - The member adds **one link per exercise**, with an optional note of the
      skills it covers or a timestamp ("Full-in at 0:42").
    - The server checks only that it is an `https` URL from a known video host
      (YouTube, Google Drive, Vimeo, Dropbox, OneDrive) and stores the URL with
      the entry. It never fetches, embeds or plays the video: the organiser
      opens it in a new tab, so no third-party content or tracking reaches our
      pages.
    - Pasting a link explains that a YouTube video must be **unlisted, not
      private** (a private one plays only for the Google accounts it is shared
      with), and a Drive file shared with "anyone with the link".
    - Each video is **missing**, **provided** or **reviewed** (OK, or "need
      more" with a note back to the club). The comp sec sees missing videos
      before sending.
    - Whether a video shows that gymnast performing safely is the organiser's
      judgement; the app records the link and the review, nothing more.

## Consequences

**Positive**
- Organisers, gymnasts and judges share data without anyone signing up.
- Personal data is small and short-lived, which suits under-18 competitors.
- One engine checks everything: builder, view screen, tariff sheet and stored
  entries agree by construction.
- One file is easy to back up and restore, and needs no database server.

**Negative / risks**
- **A lost admin link can't be recovered.** *Mitigation:* show it once
  prominently with a copy button, offer a printable summary, and let the holder
  replace it if it leaks.
- **Members rely on their comp sec** to send (and re-send) their entries.
  *Mitigation:* each member sees whether their current entry has been sent,
  and individual entry stays available where the organiser allows it.
- **A comp sec's lost admin link** strands the club's view. *Mitigation:* as
  for organisers, it's shown once with a copy button and can be replaced;
  members' own entries are unaffected.
- **Links can be forwarded.** Anyone with the admin link is the organiser.
  *Mitigation:* separate links per role; the submit link only adds entries; the
  admin link can be replaced.
- **SQLite has a single writer.** That's ample at this scale. *Mitigation:* WAL
  mode and short transactions; the Postgres-compatible SQL keeps a move cheap.
- **Backups now matter** for this app. *Mitigation:* the data volume goes into
  the server's nightly backup before launch.
- **Video links can break** (deleted, made private, wrong sharing setting).
  *Mitigation:* the organiser marks "need more" and the club sends a new link;
  the paste hint covers the common YouTube and Drive settings.
- **Hosting is a precondition.** *Mitigation:* build and test locally first,
  then ship once storage is durable (Decision 8).

## Alternatives considered

- **No club level**: every gymnast enters the competition directly. This is
  simpler, but it's not how entries are gathered, and the organiser would get
  each club's entries one by one.
- **Members' changes flow straight through** to the competition. This is less
  work for the comp sec, but the club couldn't check a change before the
  organiser sees it.

- **Accounts now** (magic link). This has a better long-term identity story,
  but more friction for occasional users, more personal data (emails), and email
  sending to run. It's deferred, not rejected (Decision 9).
- **Browser-only, with share links.** There's no server state, but the
  organiser can't receive submissions without someone gathering links by hand,
  which is today's problem in another form.
- **Uploading videos to the server.** One place to watch everything, but
  routine videos are large (disk and nightly backups would fill quickly), and
  hosting videos of under-18s makes us responsible for them. A link leaves the
  video under the gymnast's or club's own control.
- **A hosted database** (Postgres, Firebase). This means more running costs and
  moving parts for no benefit at this scale, and it takes data off the server
  that's already being set up.

## Migration

Each step is a separate, shippable branch:

1. This ADR.
2. A `store` package (open, migrate, competitions, clubs, members, entries,
   retention) and a `competitions` package (model, validation, checking), with
   unit tests and no UI. Move the level checking out of `main.go`.
3. Competition pages: create a competition, the organiser's dashboard (by
   club, individuals apart), and individual entry. These aren't linked from the
   app yet.
4. Club pages: create a club, members join and keep their entries (personal
   links saved in the browser), the comp sec's page, and sending and
   re-sending to a competition.
5. Print a level's cards, CSV export, marking cards checked, the deadline lock,
   and automatic deletion.
6. Video proof (Decision 10): the organiser's setting, video links on entries,
   and the review column.
7. Hosting: a data volume, backups and the contract in the server repository,
   then link the feature from the app.
