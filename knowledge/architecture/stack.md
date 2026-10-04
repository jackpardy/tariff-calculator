---
type: Architecture
title: Technology stack
description: Go 1.25 standard-library server, templ components, htmx 2, a little Alpine.js, SortableJS and Bulma, all vendored and embedded in one static binary, with no database.
resource: https://github.com/jackpardy/tariff-calculator/blob/master/go.mod
tags: [architecture, go, templ, htmx, alpine]
generated: { by: claude-code/cli, at: 2026-10-04T17:45:00Z }
---

# Components

| Part | Choice | Notes |
|---|---|---|
| Language | Go 1.25 | Module `tariffCalculator`. |
| HTTP | `net/http` (stdlib `ServeMux` with method patterns) | Timeouts and a 64 KB body/header limit; see [HTTP routes](http-routes.md). |
| Templates | [templ](https://templ.guide) `v0.3.x` | Typed components in `views/*.templ`; the generated `*_templ.go` files are committed. The CLI is pinned as a `tool` in `go.mod`: `go tool templ generate`. |
| Interactivity | htmx 2.0.x | Vendored in `static/js/htmx.min.js`; no CDN scripts. |
| Page state | Alpine.js 3.x | Glue only: the calculator component (`static/js/app.js`) keeps saved routines in sync and asks the server to re-render. |
| Reordering | SortableJS | Vendored. |
| Styling | Bulma (vendored CSS) plus `styles.css`, `sheet.css` for print. | |
| QR codes | `rsc.io/qr` | [Sharing](../features/sharing.md). |
| Storage | Browser `localStorage` only | No database or accounts yet ([ADR 0001](../../docs/adr/0001-architecture.md) plans SQLite when accounts arrive). |

# Packages

| Package | Role |
|---|---|
| `skills` | The engine: [skill model](../domain/skill-model.md), [tariff](../domain/tariff.md), [routine validation](../domain/routine-validation.md). Framework-free. |
| `catalog` | [Picker and search](../domain/skill-catalog.md) over the common skills. |
| `requirements` | [Requirements framework](../requirements/framework.md) and built-ins. Framework-free. |
| `views` | templ components: page, form, routine, sheet, compare, view screen, requirements page and the level editor. |
| `static` | Embedded CSS/JS with content-hashed URLs ([static assets](static-assets.md)). |
| `main` | Handlers (`main.go`), the requirements and level editors' form parsing (`setform.go`, `levelform.go`) and QR codes (`qrcode.go`). |

Domain packages import nothing HTTP or template-related, so the engine can
later be published as a module or put behind a JSON API (ADR 0002 §6) without
changes.

# Deferred

From ADR 0001 and 0002: browser WASM (only if the animation player needs it),
a JSON `/api/v1`, SQLite with `sqlc` and Litestream, accounts with magic links,
video in R2, Gotenberg PDFs, and the stick-figure animation.
