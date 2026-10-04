---
type: Architecture
title: Server-rendered UI
description: Every page and fragment is HTML rendered by the server from templ components; the browser posts the routine on each change and swaps in the result, and domain rules exist only in Go.
resource: https://github.com/jackpardy/tariff-calculator/blob/master/docs/adr/0002-server-rendered-frontend.md
tags: [architecture, rendering, templ, htmx, adr-0002]
generated: { by: claude-code/cli, at: 2026-10-04T17:45:00Z }
sources:
  - id: adr-0001
    resource: ../../docs/adr/0001-architecture.md
    title: ADR 0001 — Architecture for growing the tariff tool into a product
    author: human:jackpardy
  - id: adr-0002
    resource: ../../docs/adr/0002-server-rendered-frontend.md
    title: ADR 0002 — Server-rendered frontend with templ and HTMX
    author: human:jackpardy
---

# The rule

**The server renders all UI as HTML, and domain rules exist only in Go.**[^adr-0002]

ADR 0001 originally planned to run the engine in the browser as WebAssembly.[^adr-0001]
A review in October 2026 found that the app had drifted into a hybrid: a
~560-line Alpine component rendered JSON from the server, and landing, equality,
phase and shape rules had been copied into JavaScript. A missing JSON field
meant card highlighting silently never worked. Offline use was confirmed as
not needed, so ADR 0002 moved everything to server rendering.

# How a change flows

1. The user changes something (adds, edits, reorders, picks requirements or
   checks).
2. The page's Alpine component updates `localStorage` and posts the
   **whole routine** as JSON, plus the requirements and checks, using htmx.
3. The server parses and validates it, re-derives names and tariffs, runs
   [routine validation](../domain/routine-validation.md) and the
   [requirements](../requirements/framework.md), and renders the HTML.
4. htmx swaps the HTML in.

The server is **stateless**: each request carries everything it needs.

The skill form works the same way. On each change the form posts to
`POST /skill-inputs` (search waits 200 ms after typing stops), and the server returns the editor with the right twist
boxes, shape visibility and straddle option. While the input is unscorable (for
example a rotation still being typed), it answers 204 so htmx leaves the field
alone. The [requirements editor](../requirements/framework.md#the-editor)
follows the same pattern.

# JavaScript's job

Only what must run in the browser: keeping `localStorage` in sync, expanding
and collapsing, toasts, drag to reorder, [sharing](../features/sharing.md)
(fragment encoding, share sheet), the tariff sheet's show/hide toggles, and the
[update notice](static-assets.md#update-notice). None of it computes a tariff,
landing or equality.

# Consequences

- Each interaction is a round trip. This is accepted because the engine answers
  in milliseconds and offline isn't needed.
- The whole UI is testable from Go: handler tests assert on rendered HTML
  ([testing](testing.md)).
- Template mistakes are compile errors.

All four migration steps in ADR 0002 were completed on 2026-10-03/04.

[^adr-0001]: ADR 0001 — Architecture for growing the tariff tool into a product
[^adr-0002]: ADR 0002 — Server-rendered frontend with templ and HTMX
