# Update log

## 2026-10-05
* **Creation**: A [guide](guide/) for gymnasts and coaches:
  [features](guide/features.md) and a step-by-step
  [user guide](guide/user-guide.md).
* **Update**: Brought the bundle up to date with levels: skill names lead with
  the shape for single somersaults, the [view screen](features/view.md) opens a
  level entry from Levels mode, and the tariff sheet, stack, tests and overview
  pages mention levels.
* **Update**: View with two routines side by side shows both (each column also
  has its own View), and a level being worked on can be shared by link with
  its routines.
* **Update**: Set routines are built in the Routine Builder (More → Save as a
  set routine) or made from a routine already built, and edited there; the
  requirements editor no longer writes them element by element. The level
  editor asks for the structure first (set routine then voluntary, one
  voluntary, two voluntaries, set routine for both) and offers each slot only
  what fits.
* **Update**: "Check against" lists each level's voluntary requirements, named
  after the level (or by exercise when its two differ); set routines are a
  starting point ("Start from a set...") rather than requirements.

## 2026-10-04
* **Update**: Levels and routines are kept apart. "Check against" lists
  requirements only; the builder's new Levels mode shows a level's tabs (set
  routines as prescribed, side by side if wanted) and links ordinary routines
  to its voluntaries. Level routines from earlier became level entries. On the
  view screen, rows now fit their column, so difficulty no longer runs off the
  edge.
* **Update**: The [view screen](features/view.md)'s Show menu lists the
  routines on screen by name (Set 1, Set 2, Voluntary) to show or hide, in
  place of "The other exercise" and "Other set routine options".
* **Update**: "Add to" lists every saved routine (a level routine by
  voluntary tab), not just the ones on screen. Single fronts, backs and baranis
  are named shape first ("Tuck Back", "Pike Barani").
* **Update**: A [level](requirements/levels.md) routine holds both exercises
  as tabs (e.g. Set 1, Set 2, Voluntary) instead of two paired routines:
  set routines show as prescribed without loading, and a set can be copied into
  the voluntary. On wider screens the skill card's Add button sits at the end of
  its row instead of stretching across the card.
* **Update**: The [view screen](features/view.md) shows a routine, or a
  level's pair, full screen without scrolling, with a Show menu to turn each
  kind of information on or off. Linked from the routine builder's View button.
  A level's set routine options that no routine is doing show too, so both
  options appear beside the voluntary.
* **Update**: [Levels](requirements/levels.md) pair requirements for a
  competition's two exercises: built in for every BUCS, FIG and BG level, and
  written by coaches in a level editor. Routines doing a level's exercises are
  paired in the builder and checked together: when the first exercise scores
  only some elements, their difficulty carries over and they can't be repeated
  in the second. Pairs and levels travel in share links.
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
