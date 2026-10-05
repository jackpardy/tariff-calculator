# ADR 0004 — Server storage with secret links, no accounts

- **Status:** Proposed
- **Date:** 2026-10-05
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

2. **Secret links instead of accounts.** Each stored thing has an unguessable
   token for each role, used in its URL:

   | Link | Lets you | Stored as |
   |---|---|---|
   | Competition admin | See and manage all entries, print, export, lock, delete | SHA-256 hash |
   | Competition submit | Add an entry | plain (it's meant to be shared) |
   | Entry edit | Change or withdraw one entry until the deadline | SHA-256 hash |

   Secret tokens are 128 bits from `crypto/rand`, written as base64url. Only
   their hashes are stored, so a copied database doesn't give anyone access.
   Hashes are compared in constant time. A wrong token gets **404**, exactly as
   a missing one does. A new link is shown once, with "save this link" and a
   copy button. The organiser can replace the admin link, which ends the old
   one.

3. **The domain stays framework-free.** A new `competitions` package holds the
   model (competition, entry), its validation and the checking of an entry. It
   imports `skills` and `requirements`, and nothing about HTTP, SQL or
   templates. The level checking in `main.go` (`checkRoutine`, `checkPosted` and
   the carry-over between exercises) moves into a function that both the
   routine view and stored entries call. That way an entry's check can't drift
   from what the gymnast saw in the builder.

4. **Stored data is canonical and checked on read.** An entry stores its
   routines as `TrampolineSkill` JSON, as the browser does, plus its checks
   (ADR 0001 §6). Names, tariffs and results are worked out again whenever the
   entry is shown, so engine fixes apply to stored entries too. A competition
   stores a **copy** of any custom requirements or levels it offers, because
   references to a browser's saved items can't be resolved on the server.
   Built-in requirements are stored by reference (`builtin:<id>`) and resolved
   from the binary.

5. **Minimal personal data, deleted automatically.** An entry holds the
   gymnast's name, club, level and routines. There are no emails, dates of
   birth or contact details. A competition and its entries are deleted **90
   days after the competition date**, and the organiser can delete them sooner.
   The submit page says what is stored, who can see it and when it's deleted.

6. **Abuse limits**, as in charades: a cap on competitions created per IP per
   hour, a cap on entries per competition, and the existing 64 KB body limit.
   Behind Caddy the client IP is the rightmost `X-Forwarded-For` value.

7. **Storage features ship only on durable storage.** Until the app runs where
   `DATA_DIR` survives deploys, the pages that use storage are built and tested
   but not linked from the app. Durable means the self-hosted server, with a
   `/srv/data/tariff` volume in its nightly backup, or a paid Render disk in the
   meantime. If storage can't be opened, the calculator's stateless pages keep
   working and only the storage pages report the problem.

8. **Accounts can come later without undoing this.** If clubs need records that
   last several seasons, an account can own the links it created. Nothing here
   rules out the magic-link accounts ADR 0001 planned.

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
- **Links can be forwarded.** Anyone with the admin link is the organiser.
  *Mitigation:* separate links per role; the submit link only adds entries; the
  admin link can be replaced.
- **SQLite has a single writer.** That's ample at this scale. *Mitigation:* WAL
  mode and short transactions; the Postgres-compatible SQL keeps a move cheap.
- **Backups now matter** for this app. *Mitigation:* the data volume goes into
  the server's nightly backup before launch.
- **Hosting is a precondition.** *Mitigation:* build and test locally first,
  then ship once storage is durable (Decision 7).

## Alternatives considered

- **Accounts now** (magic link). This has a better long-term identity story,
  but more friction for occasional users, more personal data (emails), and email
  sending to run. It's deferred, not rejected (Decision 8).
- **Browser-only, with share links.** There's no server state, but the
  organiser can't receive submissions without someone gathering links by hand,
  which is today's problem in another form.
- **A hosted database** (Postgres, Firebase). This means more running costs and
  moving parts for no benefit at this scale, and it takes data off the server
  that's already being set up.

## Migration

Each step is a separate, shippable branch:

1. This ADR.
2. A `store` package (open, migrate, competitions, entries, retention) and a
   `competitions` package (model, validation, checking), with unit tests and
   no UI. Move the level checking out of `main.go`.
3. Pages: create a competition, submit an entry, edit an entry, and the
   organiser's dashboard. These aren't linked from the app yet.
4. Print a level's cards, CSV export, marking cards checked, the deadline lock,
   and automatic deletion.
5. Hosting: a data volume, backups and the contract in the server repository,
   then link the feature from the app.
