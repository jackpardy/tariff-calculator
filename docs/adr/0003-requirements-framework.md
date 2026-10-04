# ADR 0003 — A flexible framework for routine requirements

- **Status:** Proposed
- **Date:** 2026-10-04
- **Deciders:** jackpardy (solo maintainer)
- **Amends:** ADR 0001 §5 (requirements engine) and its roadmap items for standard and
  customisable requirement sets

## Context

Coaches need to check a routine against the rules of the competition it's for: which
elements are required, which are not allowed, how many elements, difficulty limits, or a
whole set (compulsory) routine. The users are mostly newer gymnasts and coaches in
**student trampoline**, whose rules change from season to season and between
competitions, and who also compete under **Gymnastics Ireland (GI)**, **British
Gymnastics (BG)** and **FIG age-group** rules.

So the important property is flexibility: a coach must be able to write or adjust a rule
set in the app, without code changes, and share it with others. Built-in sets are a
convenience, not the main path.

ADR 0001 §5 sketched typed rules with a registry, nestable rule sets, standard sets as
embedded JSON, custom sets stored per user, and a `cel-go` expression rule as an escape
hatch. Since then ADR 0002 moved the UI to server rendering, and there are still no
accounts: routines live in the browser.

## Decision

1. **A framework-free `requirements` package** holds the model and the evaluation, like
   `skills`. HTTP handlers and templ views are thin adapters over it.

2. **A requirement set is plain JSON** — `{format, name, description, source, rules}` —
   so it can be stored, exported, imported and shared as a file or pasted text. `format`
   is versioned so later changes can migrate old sets. In the app's wording a requirement set is
   just "requirements": in trampolining "set" means a set (compulsory) routine, so the
   word is kept for that.

3. **A small set of general rule types**, each a JSON object with a `type`:
   - `count` — at least `min` and/or at most `max` elements match a *matcher*. Covers
     required elements (`min: 1`), forbidden ones (`max: 0`) and limits ("no more than
     2 doubles").
   - `every` — every element matches (e.g. "no element over 1¼ somersaults").
   - `elements` — the number of elements is within `min`/`max`.
   - `difficulty` — the routine's counted difficulty (tariff) is within `min`/`max`. An
     optional `cap` limits what one element counts for, as in age-group competition,
     where a harder element may be performed but counts as the cap (CoP §17.1).
   - `position` — the element at a given position matches (e.g. "finish with a back
     somersault").
   - `sequence` — a set routine: each element in turn matches the given matchers, and
     there are exactly that many.
   - `separate` — special requirements: each matcher in `each` is met by a *different*
     element ("these requirements cannot be fulfilled by combining them into one
     element"). Elements are assigned by maximum matching, so an element that could meet
     two requirements is used where it's needed; the assigned elements are the ones
     starred on a competition card, and the tariff sheet ticks them.
   - `includes` — at least one of several `options` appears, each option one or more
     elements performed one straight after another (e.g. BUCS Level 1: "a ¾ somersault
     to front or back followed by a 1¼, or a full somersault with a full twist").
   - `different` — no element is repeated, using the Code of Points' definition of a
     repetition (§14: shape, and twist phase in multiple somersaults, distinguish
     elements only where it says they do).

   Each rule may carry a `label` written by the set's author ("A back somersault");
   otherwise a plain-English description is generated.

4. **A matcher** describes elements with optional conditions, all of which must hold:
   rotation (quarter somersaults, min/max), direction, total twist (half twists,
   min/max), shapes, take-off and landing positions, tariff (min/max), and an exact FIG
   notation. A matcher may carry its own `label`, used for set-routine elements ("Back
   somersault (T)") and special requirements ("Landing on the front"). This expresses nearly every element requirement in the rule books we know
   of without a general expression language, so the `cel-go` escape hatch is dropped
   until a real rule needs it.

5. **Evaluation collects every result** (never stops at the first failure): for each
   rule, pass/fail, a description, and the element numbers involved (the matching ones,
   or the offending ones). Rules look at the routine as written.

6. **Where sets live.** Built-in sets ship embedded in the binary as JSON, each naming its
   source document and season. Custom sets live in the browser alongside saved routines,
   and can be created from scratch, duplicated from a built-in or another set, edited,
   exported and imported. They are shared, with routines, by link: the link carries the
   compressed JSON after "#share=", so nothing is stored on the server, and opening it
   offers to add what it carries (a routine brings the custom set it's checked
   against). A QR code of the link suits sharing in person. When accounts arrive, the
   same JSON moves into the database.

7. **The editor follows the skill editor's pattern**: the browser posts the set being
   edited, the server parses and validates it into the model and re-renders the editor,
   and the browser saves the result. Validation of sets is in Go, once.

8. **A set says how routines checked against it are scored**: `no_difficulty` (difficulty
   isn't scored), `repeats_allowed` (elements may be repeated), as for set
   (compulsory) routines and some first exercises, and `scored_elements` (only the
   highest N elements score difficulty, as in an AG3 first exercise). The routine builder's "Checks" start
   from these, and a coach can change them for one routine.

9. **Built-in sets are only added from a cited source** (rule book, handbook or
   competition document) and say which season they reflect; the app states that users
   should check them against the current rules. They live in `requirements/sets/<group>/`,
   and `sets/groups.json` names and orders the groups (one per organisation and
   pathway). The first drafts cover the BUCS Championships competition structure (2026),
   the FIG Junior and WAGC rules (2025–2028) and the British Gymnastics national pathway
   (2026) and club & regional pathway (2027) technical requirements. Where a level lets
   the gymnast choose between set routines, each option is its own set. The Irish
   Student Trampoline Open's levels aren't online (its site has lapsed), so they wait
   for its documents.
   Gymnastics Ireland's levels are not published (its development plan is sent to club
   secretaries on request), so they wait for that document.

10. **Levels pair the requirements for a competition's two exercises.** A gymnast
    competes at a level with two routines, e.g. one of two set routines and then a
    voluntary, or two voluntaries. A level is plain JSON like a set:
    `{format, name, description, source, first, second}`, where each
    exercise lists `options`, the requirements the gymnast chooses between, by
    reference (`builtin:<id>` or a saved set's id). With no `second`, both
    exercises use the first's requirements. When the first exercise scores only
    some elements (`scored_elements`), their difficulty carries over and they
    can't be repeated in the second; this follows from the first exercise's
    requirements, so a level doesn't restate it. Built-in levels are listed in
    `sets/groups.json` and cover every built-in set; coaches write their own in a
    level editor that follows decision 7, and share them like sets. In the routine
    builder, a routine checked against a level holds both exercises, one tab per
    option (e.g. Set 1, Set 2, Voluntary); set routines show as prescribed, and
    the page posts the open tab and the other exercise's choice so the server
    checks them together.

## Consequences

**Positive**
- Coaches can keep up with changing student rules themselves, and share sets.
- One model and one evaluator serve built-in and custom sets, the routine view and the
  tariff sheet (e.g. marking required elements).
- No expression language to secure or explain.
- A gymnast's two routines are checked as the pair they are, and a level's set
  routine options are offered together instead of as unrelated lists.

**Negative / risks**
- Some rule might not fit the matcher. *Mitigation:* the format is versioned; add a rule
  type when a real rule needs it.
- Custom sets live only in that browser until accounts exist. *Mitigation:* export and
  import.
- A level refers to sets rather than copying them, so deleting a set leaves a
  level with a missing choice. *Mitigation:* the level editor lists it as a problem;
  sharing a level carries the sets it uses.
- Built-in sets go out of date. *Mitigation:* each names its source and season, and the
  app tells users to check them.
