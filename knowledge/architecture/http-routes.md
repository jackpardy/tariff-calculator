---
type: API Reference
title: HTTP routes
description: Every route the server exposes, what it takes and what it returns, along with the limits applied to all requests.
resource: https://github.com/jackpardy/tariff-calculator/blob/master/main.go
tags: [architecture, http, routes]
generated: { by: claude-code/cli, at: 2026-10-04T17:45:00Z }
---

# Routes

All routes are defined in `routes()` in `main.go`. Forms are
`application/x-www-form-urlencoded`. "Routine" means a JSON array of
[skills](../domain/skill-model.md#schema).

| Method and path | Input | Returns |
|---|---|---|
| `GET /` | — | The calculator page ([routine builder](../features/routine-builder.md)) |
| `POST /skill-form` | optional `skill` (JSON) + `editIndex` | The "Add a skill" panel, for the default skill or the one being edited |
| `POST /skill-inputs` | the form's fields, or `load=common` + `commonSkillKey` (+ `shape`), or `load=skill` + `skill` | The re-rendered editor; 204 while the form holds something unscorable |
| `GET /skill-search` | `q` | Search results ([skill catalog](../domain/skill-catalog.md)) |
| `POST /calculate-skill` | the form's fields | The skill as JSON, named and priced, for the page to store |
| `POST /routine` | `routineData`, `requirementSet`, `checks`, `side` (`a`/`b`), optional `compareData` + `compareName`; for a [level](../requirements/levels.md)'s exercise, `level`, `exercise` and the partner's `pairData`, `pairSet`, `pairChecks`, `pairName` | The routine view: cards, flags, totals, requirement results, and the level panel |
| `POST /set-routine` | `requirementSet`, optional `routineData` | JSON `{name, skills, matches}` ([set routines](../requirements/set-routines.md)) |
| `POST /qr` | `text` | An SVG QR code ([sharing](../features/sharing.md)) |
| `GET /compare`, `POST /compare` | `aData`, `aName`, `bData`, `bName` | The [compare page](../features/compare.md) and its comparison |
| `GET /requirements` | — | The requirements page |
| `POST /requirements/editor` | `set` (JSON), or the editor's form fields + `action`/`add` | The re-rendered editor with problems listed |
| `POST /requirements/level-editor` | `level` (JSON) and `custom`, or the editor's own fields and `action` | The [level](../requirements/levels.md) editor |
| `GET /tariff-sheet`, `POST /tariff-sheet` | as `/routine` | The [tariff sheet](../features/tariff-sheet.md) page and card |
| `GET /view`, `POST /view` | as `/routine`, plus `routineName` | The [view screen](../features/view.md) page, and a routine or level pair to fill it |
| `GET /static/...` | — | [Static assets](static-assets.md) |

`requirementSet` is either `builtin:<id>` or the custom requirements as JSON.
If the requirements can't be used, the problem is reported in the rendered
check rather than failing the request.

# Limits and hardening

- Request bodies are capped at **64 KB** (`http.MaxBytesReader`), as are headers.
  A full routine is a few kilobytes.
- Timeouts: read header 5 s, read 10 s, write 10 s, idle 60 s.
- Every incoming skill is normalised to its phase count and validated (rotation
  0–16, non-negative twists, known shape and positions). Errors are
  `400 Bad Request: <message>`, which the page shows to the user.
- Official names are always re-derived and never taken from input.
- Every response carries `X-App-Version` ([static assets](static-assets.md#update-notice)).
- `PORT` sets the port (default 8080).
