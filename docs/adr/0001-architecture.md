# ADR 0001 — Architecture for growing the tariff tool into a product

- **Status:** Accepted
- **Date:** 2026-06-23
- **Deciders:** jackpardy (solo maintainer)

## Context

The project began as a prototype to try out HTMX: a Go (`net/http` + `html/template`)
server rendering fragments, with Alpine.js for client state, Bulma for styling, and
the routine held in `localStorage`. There is no database, auth, or persistence.

The valuable asset is the **`skills` Go package**: tariff calculation (FIG Code of
Points 2025–2028 §17), landing-position geometry, FIG notation, and
duplicate/repetition rules — verified correct and covered by unit tests.

The goal is now a real tool that trampolinists/coaches/judges use, and that can grow
to cover: saved routines; scores per competition (with optional video); a gymnast
session journal; printable/PDF tariff sheets; side-by-side routine comparison; a
requirements engine (standard + customisable rule sets that check whether a routine
is legal for a category); and, as a stretch, stick-figure animation of a routine.
It should also be embeddable in a larger project later. The maintainer is a solo dev
who currently writes Go.

The pivotal question: **where does the domain logic live**, and what frontend/backend
shape best fits a client-heavy, computation-driven, offline-friendly, single-maintainer
tool?

## Decision

Build a **pragmatic Go full-stack application** with the `skills` package as the
single source of truth, run in two places from one source.

1. **Domain logic stays in Go, untouched, and runs both server-native and in the
   browser via WebAssembly.** Never reimplement it in TypeScript (a second
   implementation would drift and discard the tests); never keep it server-only
   (that loses the live, no-round-trip editor feel). Stock-Go WASM (~930 KB gzipped,
   cached) is the baseline; TinyGo is a later "measure it" optimisation, not a
   dependency (its JSON path is risky against the package's custom enum marshaling).
   Because the server can always recompute natively, a WASM failure is a UX
   regression, not a correctness failure.

2. **Server:** Go stdlib `net/http`, serving both a versioned JSON API (`/api/v1/...`)
   and the UI. The UI is just another client of the API. Templates/static/WASM are
   embedded via `embed.FS` (fixing today's relative-path glob).

3. **Persistence:** pure-Go SQLite (`modernc.org/sqlite`) with Litestream backups,
   accessed via `sqlc` (compile-time-safe generated Go, not an ORM). SQL is written
   to the Postgres-compatible subset so a later swap is driver+DSN only.

4. **Package discipline (framework-free, no HTTP/DB/template types leaking in):**
   `skill` (existing engine), `routine` (validation/aggregation engine extracted from
   `main.go`), `requirements` (new rule engine), `kinematics` (new animation-track
   generator). This decoupling is what makes embeddability essentially free later.

5. **Requirements engine:** typed discriminated-union `Rule`s
   (`type` discriminator + params), deserialised via a registry — the same
   polymorphic-JSON pattern already used for enums. `Rule.Evaluate(routine) RuleResult`;
   compose via a nestable `RuleSet{rules, combinator: all|any|n_of}`. Standard FIG sets
   ship as `embed.FS` JSON, versioned to the CoP edition; custom sets are user rows
   (clone-then-tweak). Evaluation collects all results (never short-circuits) and
   reports per-rule pass/fail + offending skill indices. A single `expression` rule
   kind backed by `cel-go` is the sandboxed escape hatch.

6. **Data model:** routines stored as canonical `TrampolineSkill` JSON
   (`skills_json`), never rendered HTML, so the engine can always recompute. Totals
   and validation are derived on demand; the only deliberate denormalisation is a
   frozen `difficulty_tariff_snapshot` on a `score` (historical accuracy if the routine
   is later edited). Video bytes live in S3-compatible object storage (Cloudflare R2,
   presigned uploads); the DB stores only references.

7. **PDF tariff sheets:** one print layout, two render paths — print CSS + browser
   "Save as PDF" for the MVP; the *same* HTML POSTed to a containerised Gotenberg
   (headless Chromium) for server-generated PDFs later.

8. **Frontend, for now:** keep HTMX/Alpine/Bulma. Allow small JS "islands" only where
   genuinely needed (the editor's live feedback via WASM; later the animation player).
   **Trigger rule:** when a *second* page needs rich reactivity, introduce one small
   reactive client (e.g. Svelte/Lit) for those pages, consuming the existing JSON API +
   WASM. The API contract makes that additive, not a rewrite. Do not pre-empt it.

## Consequences

**Positive**
- Plays to the maintainer's existing Go skill; fastest path to MVP for a solo dev.
- The crown-jewel domain logic has exactly one implementation, reused three ways
  (Go module, WASM widget, JSON API) — embeddability falls out of the package split.
- Cheap to run (~$5–15/mo, single binary + SQLite).
- Client/server agreement on tariffs is guaranteed by construction (same compiled code).

**Negative / risks**
- HTMX/Alpine is a weak fit for the animation scrubber and a snappy drag-reorder
  editor. *Mitigation:* the island trigger-rule above; the JSON+WASM contract keeps a
  future reactive client additive.
- WASM first-load (~930 KB) on flaky gym wifi. *Mitigation:* server-render the initial
  calculator so it works before WASM boots (progressive enhancement); cache WASM via a
  service worker; lazy-load it only on calculator/editor pages.
- Auth/data isolation matters (private scores/videos). *Mitigation:* authorization in
  plain, unit-testable Go (not browser-direct RLS); integration tests that user A
  cannot read user B's rows.
- SQLite single-writer under a popular launch. *Mitigation:* Postgres-compatible SQL +
  `sqlc` so the swap is trivial; Litestream for backups.

## Roadmap (summary)

- **MVP (no accounts):** `embed.FS`; extract stranded domain functions; package split;
  WASM live calculator/editor; print-CSS export; ad-hoc side-by-side over localStorage;
  Level-0 static animation glyphs.
- **v1 (backend inflection):** SQLite + `sqlc` + auth → save routines; scores (no video);
  side-by-side over saved routines; standard requirement-set checking; Gotenberg PDFs.
- **Later:** video upload (R2); session journal; customisable requirement sets +
  authoring UI; Level-1 2D stick-figure animation; published Go module / WASM widget if
  an embedder appears.

## Key decisions deferred to when they matter (with defaults)

- **Accounts:** anonymous localStorage at MVP; accounts only when "save routines" forces it.
- **Auth:** magic-link primary; authorization always server-side in Go.
- **DB:** SQLite + Litestream now; Postgres swap pre-planned.
- **Video:** Cloudflare R2, deferred; presigned uploads.
- **Styling:** keep Bulma for MVP.
- **WASM toolchain:** stock Go; try TinyGo only after verifying the custom marshalers survive.

## Migration

Tracked separately; step 1 (extract `findCommonSkillName` → `skills.FindCommonSkillName`
and `performRoutineValidation` → `skills.ValidateRoutine`, with the HTTP layer adapting
to preserve the JSON contract) is the first concrete move and commits to none of the
deferred decisions above.
