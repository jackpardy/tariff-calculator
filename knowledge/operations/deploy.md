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

1. `golang:1.25-alpine` downloads the modules, **runs `go test ./...`**, and
   builds a static binary (`CGO_ENABLED=0`, `-trimpath`, stripped).
2. `alpine:3.20` gets that one binary, running as non-root `app` (uid 1000) on
   `PORT=8080`, with a `HEALTHCHECK` that runs `wget` against `/`.

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

**test** job:
- checks the generated templ code is current (`go tool templ generate` then
  `git diff --exit-code`),
- `go vet ./...`,
- `go test ./...`,
- `docker build`, so a broken image is caught before deploy.

**deploy** job (after test, never for a pull request): SSH to the server with
a dedicated key. Deploys run one at a time in their own concurrency group, so a
PR's tests never hold one up. The server
forces the command `/srv/infra/scripts/deploy.sh tariff` whatever is asked for.
If the `DEPLOY_HOST` secret is empty, the job **skips itself** with a notice.
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

# Demo competition

`tariffCalculator demo` (`go run . demo` from a checkout) fills the storage in
`DATA_DIR` with a made-up competition to show the tools off, prints the links
to open, and stops; it needs `DATA_DIR` set. It is "ISTO 2027 (demo)": 25
student clubs with coaches and about 400 gymnasts (6 entering on their own),
BUCS L7 to L1 (men and women apart), tumbling and DMT, and synchro in three
events of grouped levels (L6/L7, L4/L5, L1/L2/L3, mixed pairs, up to four a
club, at the level the pair's individual levels give). Coaches sign off most
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
