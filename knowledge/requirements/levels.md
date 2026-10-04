---
type: Feature
title: Levels
description: A level pairs requirements for a competition's two exercises (e.g. a choice of set routines, then a voluntary); a routine checked against one holds both exercises as tabs. Built-in levels cover every built-in requirement, and coaches can write their own.
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

Levels come first under "Check against". A routine checked against a level
holds **both of its exercises**, with a tab for each option above its cards:
BUCS L7 is **Set 1, Set 2, Voluntary**; FIG AG1 is **1st voluntary, 2nd
voluntary**; BG Club L1 (the same set routine twice) has just the set routine.
A set routine's tab is the set routine as prescribed, so switching between Set
1 and Set 2 loads nothing and asks nothing. Opening an option makes it the
exercise's choice (the set the gymnast performs), marked ● when there's more
than one. A set routine's tab has **Copy into the voluntary**, to start the
voluntary from it.

Choosing a level for a routine that already has skills makes them the
voluntary. A level with no voluntary (BG Club) starts a new routine instead,
leaving the old one alone. Choosing requirements on their own keeps the open
tab's skills, and asks first if another tab has skills of the coach's own.

The routine stores `level`, `exercise` (the open tab's exercise, 1 or 2) and
`requirements` (the open tab's option), and `skills` and `checks` are the open
tab's, so everything that reads a routine works unchanged. The other tabs'
skills and checks are kept in `exercises: [{option, slots: {<ref>: {skills,
checks}}}, ...]`, where `option` is each exercise's choice. `Exercises` in
`static/js/sets.js` works out the tabs and builds the values the server needs.
Routines paired the earlier way (one routine per exercise, with a `partner`)
become level routines holding the exercise they did.

# Checking both exercises

`/routine`, `/tariff-sheet` and `/view` take the level (`level`: a built-in
reference or a custom level's JSON), `exercise`, and the other exercise's
chosen option (`pairData`, `pairSet`, `pairChecks`, `pairName`). If that
option is a set routine whose tab hasn't been opened, `pairData` is empty and
the server uses the set routine. The server checks both
(`checkRoutine` in `main.go`). When difficulty carries over, the first
exercise's scoring elements are passed to the second's validation as
`ValidateOptions.ScoredEarlier`. A repeat of one is flagged "Can't Repeat:
Scored In 1st Exercise" and isn't counted
([routine validation](../domain/routine-validation.md)). The routine view shows
the level, which exercise is open, and how many of its requirements the other
exercise meets. When difficulty carries over, both exercises also show the
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
