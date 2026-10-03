# ADR 0002 — Server-rendered frontend with templ and HTMX

- **Status:** Proposed
- **Date:** 2026-10-03
- **Deciders:** jackpardy (solo maintainer)
- **Amends:** ADR 0001 §1 (browser WASM), §2 (UI as an API client), §8 (frontend) and the MVP roadmap

## Context

ADR 0001 kept HTMX/Alpine/Bulma and planned to run the `skills` engine in the
browser via WebAssembly so the editor would feel live without server round trips.
A review of the app as built (October 2026) found the frontend is not really
HTMX-driven. It is a hybrid:

- The routine is held in `localStorage` and rendered by a ~560-line inline Alpine
  component from JSON returned by `/validate-routine-client-state`. HTMX is mostly
  used as a wrapper around `fetch` (`htmx.ajax(..., {swap: 'none'})` plus parsing
  JSON in an event listener).
- The client depends on the exact shape of that JSON, and nothing checked it. The
  per-skill validation flags were tagged `json:"-"`, so card highlighting never
  worked, and nobody noticed (fixed in `ce908e7`).
- Domain rules have already been copied into JavaScript: landing position, skill
  equality, twist phases and shape relevance. This is the drift ADR 0001 set out to
  avoid, and it happened without WASM.
- `html/template` errors only show at runtime (e.g. an unused template referencing
  a field that does not exist).

Offline use has been confirmed as **not required**. That removes the one strong
reason to run the engine in the browser.

The product's planned surfaces (calculator, routine builder, printable tariff
sheet, side-by-side comparison, requirement-set results, saved routines and
scores) are forms, lists and documents. The stick-figure animation player is the
only one that needs rich client-side interaction.

## Decision

1. **The server renders all UI as HTML.** On every change, the browser posts the
   routine and swaps in the HTML the server returns: cards, validation highlighting,
   messages and totals. The form is server-driven too: which twist boxes are
   enabled, whether shape is shown and the straddle option are all decided by
   re-rendering the form inputs on change (lightly debounced).

2. **Domain rules exist only in Go.** No JavaScript copies of tariff, landing,
   equality, phase or shape logic. The `skills` package stays framework-free as in
   ADR 0001 §4; HTML handlers are thin adapters over it.

3. **Routine state.** For anonymous use the routine stays in `localStorage` as
   canonical `TrampolineSkill` JSON (the format already stored, so existing saved
   routines keep working). The server stays stateless: each request carries the
   routine. When accounts arrive (ADR 0001 v1), saved routines load from the
   database into the same rendering path.

4. **Templates use [templ](https://templ.guide)** (`github.com/a-h/templ`) instead of
   `html/template`:
   - Components are typed Go functions, so a missing field or wrong type is a
     compile error.
   - The templ CLI is pinned with a `tool` directive in `go.mod` and run as
     `go tool templ generate`.
   - The generated `*_templ.go` files are committed, so `go build`, `go test` and
     the Dockerfile need no extra step. CI fails if they are out of date.
   - Components compile into the binary, which replaces ADR 0001's `embed.FS` step
     for templates. Static assets (CSS, JS) still move to `embed.FS`.

5. **JavaScript is limited to UI behaviour that has to run in the browser:**
   - htmx, vendored and pinned (no CDN scripts);
   - a small amount of Alpine (or plain JS) for state local to the page:
     expand/collapse, toasts, keeping `localStorage` in sync — target well under
     100 lines;
   - drag-to-reorder via SortableJS (vendored), posting the new order to the server.

6. **The JSON API is a separate adapter, not what the UI uses.** `/api/v1` is built
   over the same domain packages when an embedder or integration needs it. It is not
   a prerequisite for any UI work.

7. **Islands stay as ADR 0001 defined them.** The animation player is the expected
   island: a small reactive component for that page, consuming the JSON API.
   Running the engine in the browser via WASM is deferred to that island, and only
   if it needs it.

## Consequences

**Positive**
- One implementation of every rule, reused by HTML handlers, the future JSON API and
  tests. The server/client JSON contract that caused the highlighting bug goes away.
- The whole UI can be tested from Go: handler tests assert on rendered HTML.
- Template mistakes become compile errors.
- Far less JavaScript, no build step for it, and no ~930 KB WASM download.
- Plays to the maintainer's Go skills; one language for almost everything.

**Negative / risks**
- Every interaction needs a network round trip. *Accepted:* offline is not required,
  and the engine responds in milliseconds. HTMX request indicators cover slow
  connections.
- More requests per session than a client-rendered app. *Mitigation:* requests are
  small and stateless, and server costs are unchanged at this scale.
- templ adds a code-generation step. *Mitigation:* generated code is committed and
  CI checks it is current; the CLI version is pinned in `go.mod`.
- Rewriting ~1,200 lines of templates and the Alpine component is real work.
  *Mitigation:* migrate in independently shippable steps (below).

## Alternatives considered

- **Keep the hybrid, tidy the Alpine code.** Leaves the JSON contract and the JS
  copies of domain rules in place; still untyped and untested.
- **A client app (e.g. Svelte) with the engine as WASM and a JSON API.** The right
  choice if offline were required. Without that, it adds a second language, a build
  pipeline and a full UI rewrite for little user benefit.

## Migration

Each step leaves the app working and deployable:

1. Add templ and render the routine list (cards, flags, messages, totals) on the
   server. This replaces Alpine's routine rendering and the
   `/validate-routine-client-state` JSON endpoint.
2. Move the skill form and evaluation preview to templ, with server-driven
   twist-box and shape behaviour.
3. Delete the JavaScript domain copies, the unused handlers and the old
   `html/template` files. Move static assets to `embed.FS`.
4. Replace the custom drag code with SortableJS.

The `skills` package, its tests and the saved-routine format are unchanged
throughout.
