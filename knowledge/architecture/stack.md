---
type: Architecture
title: Technology stack
description: Go standard-library server (Go 1.26 toolchain), templ components, htmx 2, a little Alpine.js, SortableJS and Bulma, all vendored and embedded in one static binary, with SQLite for competition entries.
resource: https://github.com/jackpardy/tariff-calculator/blob/master/go.mod
tags: [architecture, go, templ, htmx, alpine]
generated: { by: claude-code/cli, at: 2026-10-04T17:45:00Z }
---

# Components

| Part | Choice | Notes |
|---|---|---|
| Language | Go 1.26 (toolchain 1.26.9; the module allows 1.25) | Module `tariffCalculator`. |
| HTTP | `net/http` (stdlib `ServeMux` with method patterns) | Timeouts and a 64 KB body/header limit; see [HTTP routes](http-routes.md). |
| Templates | [templ](https://templ.guide) `v0.3.x` | Typed components in `views/*.templ`; the generated `*_templ.go` files are committed. The CLI is pinned as a `tool` in `go.mod`: `go tool templ generate`. |
| Interactivity | htmx 2.0.x | Vendored in `static/js/htmx.min.js`; no CDN scripts. |
| Page state | Alpine.js 3.x | Glue only: the calculator component (`static/js/app.js`) keeps saved routines in sync and asks the server to re-render. |
| Reordering | SortableJS | Vendored. |
| Styling | Bulma (vendored CSS) plus `styles.css`, `sheet.css` for print. | |
| QR codes | `rsc.io/qr` | [Sharing](../features/sharing.md). |
| Storage | Browser `localStorage`; SQLite (`modernc.org/sqlite`, pure Go) for competition entries | Live since 2026-10-06, on when `DATA_DIR` is set ([ADR 0004](../../docs/adr/0004-server-storage-secret-links.md)). Pinned to v1.59, from when the toolchain was Go 1.25; later versions now build too. No accounts. |

# Packages

| Package | Role |
|---|---|
| `skills` | The engine: [skill model](../domain/skill-model.md), [tariff](../domain/tariff.md), [routine validation](../domain/routine-validation.md). Framework-free. |
| `catalog` | [Picker and search](../domain/skill-catalog.md) over the common skills. |
| `requirements` | [Requirements framework](../requirements/framework.md) and built-ins, and checking a routine or a level's two exercises (`Check`, `CheckPair`). Framework-free. |
| `competitions` | [Competition entries](../features/competition-entries.md): competitions, the levels they offer, entries, and checking an entry with `CheckPair`; the timetable, its planner and rota, delays, lateness and who's up. Framework-free. |
| `judging` | [Execution judging](../domain/execution.md): how a skill is judged, for the calculator's pop-up. Framework-free. |
| `demo` | The seeded demo competition (`tariffCalculator demo`), made through the app's own routes. |
| `store` | SQLite storage for competitions, clubs, members and entries, reached by secret links (ADR 0004). Plain `database/sql`, numbered migrations, at most four connections; each change counted for the competition it touches (`Store.Writes`), for the pages' cache. |
| `views` | templ components: page, form, routine, sheet, compare, view screen, requirements page and the level editor. |
| `static` | Embedded CSS/JS with content-hashed URLs ([static assets](static-assets.md)). |
| `web` | Every handler: the calculator (`web/calculator.go`), the competition pages, notifications, push, mail and calendar, the requirements and level editors' form parsing (`web/setform.go`, `web/levelform.go`) and QR codes (`web/qrcode.go`). Exposes `web.New`, `Handler`, `Notify`, `Routes` and `DeleteExpired`. |
| `main` | The root `main.go` only: reads `PORT` and `DATA_DIR`, opens the store, seeds the demo, and starts the server with `web`. |

Domain packages import nothing HTTP or template-related, so the engine can
later be published as a module or put behind a JSON API (ADR 0002 §6) without
changes.

# Deferred

From ADR 0001 and 0002: browser WASM (only if the animation player needs it),
a JSON `/api/v1`, `sqlc` and Litestream, accounts with magic links,
video in R2, Gotenberg PDFs, and the stick-figure animation.
