---
type: Runbook
title: Build and deploy
description: How the app is built (a multi-stage Docker image that runs the tests), tested in CI on every pull request and push to master, and deployed to the self-hosted server at tariff.pardy.ie through an SSH forced command.
resource: https://github.com/jackpardy/tariff-calculator/blob/master/.github/workflows/deploy.yml
tags: [operations, deploy, ci, docker, self-hosted]
generated: { by: claude-code/cli, at: 2026-10-05T14:20:00Z }
---

# Live: tariff.pardy.ie

The live app is <https://tariff.pardy.ie>, on the self-hosted server. CI
deploys `master` on every push, so **merging to `master` is releasing**. Every
merge is approved by the maintainer first. The name stays for now and may
change later.

# Image

The `Dockerfile` builds in two stages:

1. `golang:1.26-alpine` downloads the modules, **runs `go test ./...`**, and
   builds a static binary (`CGO_ENABLED=0`, `-trimpath`, stripped).
2. `alpine:3.20` gets that one binary, running as non-root `app` (uid 1000) on
   `PORT=8080`, with a `HEALTHCHECK` that runs `wget` against `/healthz`. That answers 200 while the app is fit to serve, and 503 when `DATA_DIR` is set but competition storage didn't open or isn't answering (a migration that failed on deploy, say). The calculator still works then, as ADR 0004 Decision 8 has it, but the container shows as unhealthy and the deploy job fails. On SIGTERM the app lets requests under way finish (up to 15 seconds), waits up to 20 seconds for emails and pushes still queued, then closes the database.

Templates and assets are compiled or embedded into the binary, so nothing else
is copied ([static assets](../architecture/static-assets.md)). The calculator
is stateless and needs no secrets; competition entries need a data directory
([competition storage](#competition-storage)).

# CI

`.github/workflows/deploy.yml` runs on every push to `master`, on every pull
request, and on demand. A pull request runs the **test** job only, so its
checks show on the PR before merging; nothing deploys from it. A newer push to
a PR cancels its older run.

`master` requires the **test** check to pass before a pull request merges
(branch protection; admins are exempt, so a docs-only commit can still go
straight to `master`). Auto-merge is allowed, so a PR can be set to merge
itself once its check passes.

**test** job, which first looks at what changed: if only documents did
(`knowledge/`, `docs/`, any `.md` or `.pdf`), it skips everything else and
passes in seconds, so docs-only PRs can still merge, and nothing deploys
(2026-10-08). Otherwise it:
- checks the generated templ code is current (`go tool templ generate` then
  `git diff --exit-code`),
- `go vet ./...`,
- `go test ./...`,
- `govulncheck`, which fails on any known vulnerability our code can reach in
  Go or a module (added 2026-10-15),
- `docker build`, so a broken image is caught before deploy.

The Go version is go.mod's `toolchain` line (Go 1.26.9 since 2026-10-15),
which CI and local builds both use; the image builds with the newest Go 1.26.
Go supports its two newest releases, so move up when a new one comes out and
govulncheck starts failing.

**deploy** job (after test, never for a pull request, nor for documents
alone): SSH to the server with
a dedicated key. Deploys run one at a time in their own concurrency group, so a
PR's tests never hold one up. The server
forces the command `/srv/infra/scripts/deploy.sh tariff` whatever is asked for.
If the `DEPLOY_HOST` secret is empty, the job **skips itself** with a notice.
After deploying it checks `https://tariff.pardy.ie/healthz` for up to two
minutes, and fails if the app isn't healthy by then.
The secrets are set, so every push to `master` deploys. The others are
`DEPLOY_USER`, `DEPLOY_SSH_KEY` and `DEPLOY_KNOWN_HOSTS`.

# The self-hosted server

The app runs as the `tariff` service in the shared server setup: Docker
Compose behind Caddy, built from a clone of this repo at
`/srv/apps/tariff-calculator`, and served at `${TARIFF_HOST}`
(`tariff.pardy.ie`).

The old Render address, <https://trampoline-tariff-calculator.onrender.com>,
is now a Render static site built from
[tariff-redirect](https://github.com/jackpardy/tariff-redirect). It sends
visitors to the same path here, keeping any `#share=` link. Routines saved in a
browser under the old address stay there.

See the server bundle (private repository):

- [GUIDE.md](https://github.com/jackpardy/server/blob/main/GUIDE.md): setup, deploy on push, backups, retiring Render
- [CONTRACT.md](https://github.com/jackpardy/server/blob/main/CONTRACT.md): paths, services, ports and env vars

# Competition storage

**Live on tariff.pardy.ie since 2026-10-06.** The server has the volume and
`DATA_DIR`, the app created `tariff.db` on start, the competition pages work
at `/competitions`, and the nightly backup includes the database (checked the
same day). The calculator doesn't link to the pages yet.

[Competition entries](../features/competition-entries.md) are stored in one
SQLite file, `tariff.db`, in `DATA_DIR`. Unset, storage is off: the
calculator works as ever and the competition pages answer "Not available
yet". Set (and writable), they work at `/competitions`, and the app deletes
expired competitions and clubs every 6 hours. The calculator doesn't link to
them, for now.

On the server, the `tariff` service mounts `/srv/data/tariff` at `/data` with
`DATA_DIR=/data`. The directory is owned by uid 1000 (the image's `app` user),
mode 700. The nightly backup copies the database with `sqlite3 .backup`, which
stays consistent while the app writes, and `scripts/restore.md` puts it back.
These are in the server repository (CONTRACT.md "Tariff calculator storage",
GUIDE.md section m).

**Rolling back.** The app won't open a database that a newer version has
migrated further than it knows: it stops at start with an error saying so,
rather than write rows without the newer columns. To roll back past a
migration, restore the backup taken before the newer version ran. Uploads
of coaches' certificates are refused while the data directory's disk has
less than 1 GB free.

# Notifications

[Notifications](../features/competition-entries.md#notifications-adr-0008)
send email by SMTP with STARTTLS (ADR 0008 Decision 7), set by:

- `SMTP_HOST`, and `SMTP_PORT` (default 587);
- `SMTP_USERNAME` and `SMTP_PASSWORD`;
- `MAIL_FROM`, the sender, e.g. `ISTO tools <notify@tariff.pardy.ie>`;
- `PUBLIC_URL`, the site's address for links in emails (default
  `https://tariff.pardy.ie`);
- `MAIL_PER_SECOND`, the most emails sent a second (default 14, SES's
  production rate; set `1` while SES is in its sandbox).

Desk messages and change notifications are sent in the background, four at
a time, emails no faster than that. On SIGTERM the app waits up to 20
seconds more for what's queued, then logs how many were left unsent, so
with the 15 seconds for requests a stop can take 35 seconds: give the
`tariff` service a `stop_grace_period` of at least `40s` in the server's
Compose file (Docker's default of 10 seconds kills it before the database is
closed cleanly).

[Feedback about the app](../features/competition-entries.md#feedback-about-the-app)
goes to the maintainer's own page. Make its link on the server, once and
whenever it needs replacing (the last one stops working), and keep it to
yourself:

```sh
docker compose exec tariff /app/tariffCalculator maintainer-link
```

It prints the link, from `PUBLIC_URL` (the path alone without it). Run
locally, `DATA_DIR=./data go run . maintainer-link` does the same for a
local copy. `FEEDBACK_EMAIL`, if set, is where each piece is emailed as it's
sent too; it needs the email settings above. Without it, feedback is only on
the maintainer's page.

Without `SMTP_HOST` and `MAIL_FROM`, email isn't offered. Phone push needs
nothing set: the app makes its push (VAPID) keys on first use and keeps them
in the database, so the nightly backup covers them (losing them stops push
to every phone already subscribed). **Not set up on
the server yet.** The provider is Amazon SES in its Ireland region
(decided 2026-10-09; Postmark if SES refuses). To set it up:

1. In SES (eu-west-1), add the domain `tariff.pardy.ie` as an identity.
2. Add its DNS records: the three DKIM CNAMEs; a custom MAIL FROM
   subdomain (e.g. `mail.tariff.pardy.ie`) with the MX and SPF TXT records
   SES lists; and `_dmarc.tariff.pardy.ie` TXT `v=DMARC1; p=none;`.
3. Ask for production access: low volume, opt-in, addresses confirmed by
   a link, a stop link in every email.
4. Create SMTP credentials in SES.
5. On the server, set `SMTP_HOST=email-smtp.eu-west-1.amazonaws.com`,
   `SMTP_USERNAME`, `SMTP_PASSWORD` and `MAIL_FROM`, and restart the
   service. Hetzner blocks ports 25 and 465 but not 587.
6. Test with a Gmail and an Outlook address: confirm, check it isn't in
   spam, and that SPF, DKIM and DMARC pass (Gmail's "Show original").
7. Optionally have SES send bounces and complaints to an email address.

# Garbage collector

What the app keeps in memory is small (about 14 MB with the demo), but a
page can make 10 to 100 MB of garbage on the way, so at Go's defaults the
collector ran 14 times a second under load. The app sets its own defaults
(`gcDefaults` in `main.go`): `GOGC=300` (the heap grows to four times what's
kept before a collection) and `GOMEMLIMIT=400MiB` (it collects sooner rather
than grow past that). The reviewer measured these cutting each page's
processor time by a third or more. Setting `GOGC` or `GOMEMLIMIT` in the
environment overrides them, e.g. a lower `GOMEMLIMIT` on a server with less
memory to spare; nothing needs setting otherwise.

# Demo competition

`tariffCalculator demo` (`go run . demo` from a checkout) fills the storage in
`DATA_DIR` with a made-up competition to show the tools off, prints the links
to open, and stops; it needs `DATA_DIR` set. It is "ISTO 2027 (demo)": 25
student clubs with coaches and about 400 gymnasts (6 entering on their own),
BUCS L7 to L1 (men and women apart), tumbling and DMT, and synchro in three
events of grouped levels (L6/L7, L4/L5, L1/L2/L3, mixed pairs, up to four a
club, about a third with a gymnast from another club, at the level the
pair's individual levels give). Coaches sign off most
entries, about 7% of routines are deliberately careless so the dashboard and
cards show problems, members offer to judge, and the organiser adds their own
judges. The timetable runs Friday to Sunday, 09:00 to 17:30, on Panels 1–3,
Track 1 and DMT 1: a 30-minute general warm-up on every area at 09:00, an
hour's lunch, and on Sunday synchro, an hour's display taking the whole
venue, fun synchro and awards. It's planned with its officials rota (HD
judges included) and published. Everything goes through the real pages
in-process (package `demo`), so it is only as right as they are. Each run makes
a new competition with new names; the links it prints are the only way in.

Locally: `DATA_DIR=$(mktemp -d) go run . demo`, then serve the same
`DATA_DIR`. On the server, only when the maintainer asks (it puts made-up
people in the live database, deleted 120 days after the competition's date
like any other): `docker compose exec tariff /app/tariffCalculator demo` from
the server's compose directory, then open the printed paths on
`https://tariff.pardy.ie`.

# Sandbox: sandbox.tariff.pardy.ie

The [sandbox](../features/sandbox.md) ([ADR 0016](../../docs/adr/0016-sandbox.md))
is the same image run a second time, never the live site. **Not set up on
the server yet.** To set it up:

1. **A second service** in the server's Compose file, beside `tariff`: the
   same image and build, with `SANDBOX=1`, its own data directory (e.g.
   `/srv/data/tariff-sandbox` mounted at `/data`, owned by uid 1000, mode
   700, with `DATA_DIR=/data`), `PUBLIC_URL=https://sandbox.tariff.pardy.ie`,
   the same `stop_grace_period`, and **no** `SMTP_*`, `MAIL_FROM` or
   `FEEDBACK_EMAIL` (nothing would be sent anyway: sandbox mode drops it).
   The nightly backup needn't include it: nothing in it is kept.
2. **DNS**: a record for `sandbox.tariff.pardy.ie` pointing at the server
   (e.g. a CNAME to `tariff.pardy.ie`), unless a wildcard `*.pardy.ie`
   already covers it.
3. **Caddy**: a site block for `sandbox.tariff.pardy.ie` passing to the
   sandbox service's port, as the `tariff` block does. Caddy gets its HTTPS
   certificate by itself once the DNS points at the server.
4. **Its maintainer's link**, in the sandbox service, kept to yourself:
   `docker compose exec tariff-sandbox /app/tariffCalculator maintainer-link`
   (the service's name as you called it). Feedback from the sandbox is on
   that page only.
5. **The first reset**: open that link and press **Reset the sandbox** (about
   half a minute), or `docker compose exec tariff-sandbox
   /app/tariffCalculator sandbox-reset`. Until then the start page,
   `https://sandbox.tariff.pardy.ie/sandbox`, says it's being set up.

After that, reset it from the maintainer's page whenever it wants a fresh
copy; the old one works until the new one is ready. Don't run
`sandbox-reset` while a reset from the page is under way: the two don't
know of each other. The competition on the day restarts its day by itself
every fourth day (checked at start and hourly), or at once with **Restart
the day**. Locally: `SANDBOX=1 DATA_DIR=$(mktemp -d) PORT=8081 go run .`,
then `maintainer-link` and a reset against the same `DATA_DIR`.

# Load test

`cmd/loadtest` acts as the people at a competition, to see how the app holds
up before the dry run. It reads a list of pages, one person's a line, which
`tariffCalculator demo -pages FILE` writes alongside the demo (every member's
My competition page, the gymnasts entering on their own, every coach and
club, the organiser's On the day page and the venue screen). Then, at once:

- a **burst**: every person opens their page within `-burst` (as after a
  desk message or a push to everyone);
- **steady use** for `-steady`: each opens it again every `-think` or so,
  `-staff` people reload On the day every 10 seconds, and `-screens` venue
  screens refresh every 30 seconds;
- **taps**: a gymnast checked in or cleared every `-taps`, from the first
  day's area pages (where the check-ins are), so the pages' cache is
  refreshed as it would be on the day.

It ends with each kind of page's loads, failures, median, 90th and 99th
percentile and slowest time. **Never point it at the live site**: the taps
change the competition, and the demo's made-up people would sit in the live
database. Run it against a test copy: its own `DATA_DIR`, seeded with the
demo, on a port of its own.

```
DATA_DIR=/tmp/lt go run . demo -pages /tmp/lt/pages.txt
PORT=8090 DATA_DIR=/tmp/lt GOMAXPROCS=2 go run .
go run ./cmd/loadtest -base http://localhost:8090 -pages /tmp/lt/pages.txt -people 400 -burst 2s -steady 40s -think 10s -taps 2s
```

On an M2 laptop held to two cores (`GOMAXPROCS=2`, like the server's two
shared processors), 19 October 2026, the full demo (700 entries), 400 people
all opening their page within 2 seconds, then 40 seconds of steady use, a
tap every 2 seconds; each run on a fresh copy of the same demo, twice before
and twice after the changes of that day (a check-in or other change the
pages don't read keeps their cache, no write lock to open a page, four
database connections, the garbage collector's defaults (above), and the
heaviest pages' work done once):

| | Before (5401983) | After |
|---|---|---|
| Gymnasts' pages in the burst | median 85–175 ms, 99% within 430–744 ms, slowest 0.6–0.9 s | median 3–4 ms, 99% within 72–77 ms, slowest 80 ms |
| Gymnasts' pages after it | median 9 ms, 99% within 93–156 ms, slowest 0.26–0.5 s | median 6 ms, 99% within 14 ms, slowest 17–22 ms |
| Clubs' and coaches' pages in the burst | median 111–654 ms, slowest 0.34–0.89 s | median 6–7 ms, slowest 16–82 ms |
| On the day | median 20–30 ms, slowest 0.26–0.5 s | median 14–15 ms, slowest 66–94 ms |
| Taps | median 5–7 ms | median 6 ms |
| The server's processor time, all of it | 26 s | 15 s |
| The server's memory at most | 150–194 MB | 175–179 MB |

None failed either way. Twice the crowd, 800 people within 2 seconds, each
opening their page again every 5 seconds or so and a tap every second:
before, the burst's median was 959 ms and its slowest 6.1 s, the server used
68 s of processor time and up to 641 MB; after, 11 ms and 105 ms, 31 s and
190 MB.

The server is slower per processor than the laptop and shared with other
services, so run it there too before the dry run, on a test copy: the steps
above in a second container or on a spare port, with `-base` pointing at it
from a laptop (through Caddy if it's to count the network and compression).
