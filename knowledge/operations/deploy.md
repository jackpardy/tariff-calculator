---
type: Runbook
title: Build and deploy
description: How the app is built (a multi-stage Docker image that runs the tests), tested in CI on every push to master, and deployed to the self-hosted server at tariff.pardy.ie through an SSH forced command.
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
is copied ([static assets](../architecture/static-assets.md)). The app is
stateless and needs no volumes or secrets.

# CI

`.github/workflows/deploy.yml` runs on every push to `master` and on demand.

**test** job:
- checks the generated templ code is current (`go tool templ generate` then
  `git diff --exit-code`),
- `go vet ./...`,
- `go test ./...`,
- `docker build`, so a broken image is caught before deploy.

**deploy** job (after test): SSH to the server with a dedicated key. The server
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
