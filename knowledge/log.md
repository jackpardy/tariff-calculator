# Update log

## 2026-10-04
* **Update**: The 0.1 for shaped jumps and seat changes is confirmed correct
  ([tariff](domain/tariff.md)); removed from the open questions.
* **Initialization**: Created this bundle (OKF v0.2) covering the domain,
  requirements, features, architecture and operations, linked to the
  [server bundle](https://github.com/jackpardy/server/blob/main/knowledge/index.md).
* **Context**: The same day the app gained saved routines, side-by-side
  comparison, the [requirements framework](requirements/framework.md) with
  built-in BUCS, FIG and British Gymnastics requirements,
  [sharing by link or QR](features/sharing.md), the
  [skill picker and search](domain/skill-catalog.md), and the
  [tariff sheet](features/tariff-sheet.md).

## 2026-10-03
* **Context**: [ADR 0002](../docs/adr/0002-server-rendered-frontend.md)
  accepted and carried out: the UI moved to templ, the static assets were
  embedded, and drag-to-reorder moved to SortableJS. The engine was pinned to
  the 139 CoP worked examples, and Docker plus a deploy workflow were added.

## 2026-06-23
* **Context**: [ADR 0001](../docs/adr/0001-architecture.md) written; naming
  and routine validation moved into the `skills` package.

## 2025-03-28
* **Context**: First version: a Go and htmx prototype.
