---
type: Feature
title: Levels
description: A level pairs requirements for a competition's two exercises (e.g. a choice of set routines, then a voluntary). The builder's Levels mode shows a level's set routines and links routines to its voluntaries; built-in levels cover every built-in requirement, and coaches can write their own.
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

Levels and routines are kept apart. A routine is checked against
**requirements** only: "Check against" lists each level's voluntary
requirements, named after the level, or by exercise when its two voluntaries
differ ([routine builder](../features/routine-builder.md)). A level is worked on in the builder's **Levels**
mode (Routines | Levels at the top of the Routine Builder):

- **+ New level** starts a level entry; a coach can keep several for the same
  level (e.g. one per gymnast), and rename or delete them under More. Deleting
  one keeps its routines. **Share** shares it by link, with its voluntaries'
  routines ([sharing](../features/sharing.md)).
- The level's tabs sit above its columns: BUCS L7 is **Set 1 · Set 2 ·
  Voluntary**, FIG AG1 **1st voluntary · 2nd voluntary**, BG Club L1 just the
  set routine. **Side by side with** shows a second tab beside the open one; a
  new entry opens its first tab beside the other exercise (Set 1 beside the
  Voluntary). On a phone only the open tab shows.
- A **set routine** tab is the set as prescribed, read-only (`/routine` with
  `prescribed=1` builds it from its requirements). Opening one makes it the
  exercise's choice, marked ●. **Start the voluntary from this set** copies it
  into the voluntary.
- A **voluntary** tab shows the ordinary routine linked to it: **+ New
  routine**, one already built, or **Start from Set 1/2**. Linking a routine
  checks it against the voluntary's requirements; it's edited like any routine
  and also listed under Routines, and "Add to" starts on it.

Entries are kept in the browser as `localStorage['trampolineLevelEntries']`
(`LevelEntries` in `static/js/sets.js`): `{current, entries: [{id, name,
level, exercises: [{option, routine}, ...], open, beside}]}`, where `option`
is each exercise's choice, `routine` the id of the routine doing its voluntary,
and `open` and `beside` tab keys (`"<exercise>:<ref>"`). Set routines aren't
stored. In Levels mode the voluntaries shown are the builder's current and
compared routines, so editing, the skill card and checks work unchanged.
Routines that held a level's exercises as tabs (an earlier version) become
entries, their voluntary skills routines of their own.

# Checking both exercises

A voluntary shown in Levels mode, and the view screen, post the level
(`level`: a built-in reference or a custom level's JSON), `exercise`, and the
other exercise's choice (`pairData`, `pairSet`, `pairChecks`, `pairName`). A set
routine is posted without skills and the server builds it. The server checks
both together with `requirements.CheckPair`, which stored competition entries
use too, so the two can't disagree. When difficulty carries over, the first
exercise's scoring elements are passed to the second's validation as
`ValidateOptions.ScoredEarlier`. A repeat of one is flagged "Can't Repeat:
Scored In 1st Exercise" and isn't counted
([routine validation](../domain/routine-validation.md)). The routine view shows
the level, which exercise it is, and how many of its requirements the other
exercise meets. When difficulty carries over, it also shows the requirement
"The second exercise doesn't repeat an element whose difficulty carries over
from the first", naming the carried elements and any repeats.

# Writing your own

On the requirements page, "Your levels" lists the levels saved in the browser
(`localStorage['trampolineLevels']`, `LevelStore`). The level editor
(`POST /requirements/level-editor`, `levelform.go`) follows the requirements
editor's pattern. It asks for the **structure** first, then fills its slots:

| Structure | First exercise | Second exercise |
|---|---|---|
| Set routine, then a voluntary | set routines (one or more options) | voluntary requirements |
| One voluntary | voluntary requirements | (the same) |
| Two voluntaries | voluntary requirements | voluntary requirements |
| Set routine for both exercises | set routines | (the same) |

A set routine slot offers only set routines and a voluntary slot only
requirements, built-in or the user's own; changing the structure keeps what
still fits (`parseLevelForm`). An opened level's structure is worked out from
it (`levelStructure`). Description and source fold away under "More details".
Any built-in level can be duplicated to start from. Levels are shared by link like requirements, bringing the custom
requirements they use ([sharing](../features/sharing.md)).
