---
type: Runbook
title: Build and deploy
description: How the app is built (a multi-stage Docker image that runs the tests), tested in CI on every push, served from Render today, and moved later to the self-hosted server through an SSH forced command.
resource: https://github.com/jackpardy/tariff-calculator/blob/master/.github/workflows/deploy.yml
tags: [operations, deploy, ci, docker, render]
generated: { by: claude-code/cli, at: 2026-10-04T17:45:00Z }
---

# Today: Render

The live app is <https://trampoline-tariff-calculator.onrender.com>. Render
deploys `master` automatically on every push, so **merging to `master` is
releasing**. Every merge is approved by the maintainer first.

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
While the `DEPLOY_HOST` secret is empty, the job **skips itself** with a notice.
That is the state today. The other secrets are `DEPLOY_USER`, `DEPLOY_SSH_KEY`
and `DEPLOY_KNOWN_HOSTS`.

# Next: the self-hosted server

The app will run as the `tariff` service in the shared server setup: Docker
Compose behind Caddy, built from a clone of this repo at
`/srv/apps/tariff-calculator`, and served at `${TARIFF_HOST}`. This waits on
registering a domain. Then: set the GitHub secrets, run both hosts side by
side for a few days, switch DNS, and retire Render (optionally leaving a
redirect at the old `onrender.com` address).

See the server bundle (private repository):

- [tariff-calculator service](https://github.com/jackpardy/server/blob/main/knowledge/services/tariff-calculator.md)
- [deploy](https://github.com/jackpardy/server/blob/main/knowledge/operations/deploy.md)
