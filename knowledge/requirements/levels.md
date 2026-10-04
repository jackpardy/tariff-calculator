---
type: Feature
title: Levels
description: A level pairs requirements for a competition's two exercises (e.g. a choice of set routines, then a voluntary), so a coach checks a gymnast's two routines together; built-in levels cover every built-in requirement, and coaches can write their own.
resource: https://github.com/jackpardy/tariff-calculator/blob/master/requirements/level.go
tags: [requirements, level, set-routine, voluntary]
generated: { by: claude-code/cli, at: 2026-10-04T22:00:00Z }
---

# Model

A gymnast competes at a level with two exercises. `requirements.Level` says what
each one is checked against:

```json
{
  "format": 1,
  "name": "BUCS L3",
  "first":  { "options": ["builtin:bucs-l3-option-1", "builtin:bucs-l3-option-2"] },
  "second": { "options": ["builtin:bucs-l3-second"] }
}
```

- Each exercise lists **options**: the [requirements](framework.md) the gymnast
  chooses between. Two options are usually two [set routines](set-routines.md)
  ("option 1 or option 2"). One option is a single set routine or the
  requirements for a voluntary.
- With **no `second`**, both exercises use the first's requirements: two
  voluntaries under the same rules (BUCS FIG level), or the same set routine
  twice (BG Club L1–L3).
- Options are **references**, not copies: `builtin:<id>` or the id of
  requirements saved in the browser. Editing those requirements changes the
  level. `Level.Validate` checks built-in references exist; the level editor
  checks saved ones.

## Carry-over

When the first exercise scores only some elements (its requirements'
`scored_elements`, e.g. 2 in FIG AG3 and BG National 17–21), their difficulty
carries over, and **those elements can't be repeated in the second exercise**.
This is the general rule (confirmed by the maintainer), so it isn't a setting on
the level: it follows from the first exercise's checks, including a coach's own
change to how many elements score.

# Built-in levels

`sets/groups.json` lists each group's levels by set ID, alongside its sets
(`BuiltinGroup.Levels`, referenced as `builtin-level:<id>`). A level's source is
its first set's. The loader panics if a level names a set that doesn't exist,
and a test checks every built-in set belongs to a level.

| Group | Levels |
|---|---|
| [BUCS](bucs.md) | FIG Level (both exercises the same), L1–L2 (voluntary, voluntary), L3–L7 and Disability L1–L2 (set routine option 1 or 2, then a voluntary) |
| [FIG age groups](fig.md) | AG1, AG2 & Junior, AG3 (two elements carry over) |
| [BG national](bg-national.md) | 10, 11–12, 13–14 & 15–16, 17–21 (two elements carry over) |
| [BG club & regional](bg-regional.md) | Club L1–L3 (the set routine twice), Regional L1–L2 and L3 9–12 / 13+ (set routine, then a voluntary), Regional L4 10, 11–12, 13–14, 15–16, 17+ |

# In the routine builder

Levels come first under "Check against". Choosing one makes the routine the
level's **first exercise**, checked against its first option (a set routine
loads in as usual). Under the picker:

- **First / Second exercise** chooses which one this routine is.
- A second select chooses between the exercise's options when there's more than
  one, e.g. set routine option 1 or 2.
- **Second (or First) exercise** pairs the routine with another one, or starts
  a new routine for the other exercise ("+ New routine") and shows the two side
  by side.

A routine doing an exercise stores `level`, `exercise` (1 or 2),
`requirements` (the chosen option, so everything else that reads requirements
works unchanged) and `partner`, the id of the routine doing the other exercise.
The two point at each other. `Pairs` in `static/js/sets.js` finds a routine's
partner and builds the values the server needs. Choosing a different level, or
requirements on their own, separates the pair. Deleting a routine unpairs its
partner. Side by side, a pair's columns aren't shaded for differences.

# Checking a pair

`/routine` and `/tariff-sheet` take the level (`level`: a built-in reference or
a custom level's JSON), `exercise`, and the partner's routine (`pairData`,
`pairSet`, `pairChecks`, `pairName`). The server checks both
(`checkRoutine` in `main.go`). When difficulty carries over, the first
exercise's scoring elements are passed to the second's validation as
`ValidateOptions.ScoredEarlier`. A repeat of one is flagged "Can't Repeat:
Scored In 1st Exercise" and isn't counted
([routine validation](../domain/routine-validation.md)). The routine view shows
the level, which exercise this is, and how many of its requirements the other
routine meets. When difficulty carries over, both routines also show the
requirement "The second exercise doesn't repeat an element whose difficulty
carries over from the first", naming the carried elements and any repeats.

# Writing your own

On the requirements page, "Your levels" lists the levels saved in the browser
(`localStorage['trampolineLevels']`, `LevelStore`). The level editor
(`POST /requirements/level-editor`, `levelform.go`) follows the requirements
editor's pattern. Each exercise is a list of choices from the built-in set
routines and requirements and the user's own. "The same requirements as the
first exercise" drops the second exercise. Any built-in level can be duplicated
to start from. Levels are shared by link like requirements, bringing the custom
requirements they use ([sharing](../features/sharing.md)).
