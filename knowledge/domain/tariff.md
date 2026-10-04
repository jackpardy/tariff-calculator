---
type: Calculation
title: Tariff (difficulty) calculation
description: How a skill's difficulty is computed under the FIG Code of Points 2025–2028 §17, pinned by all 139 of the Code's worked examples.
resource: https://github.com/jackpardy/tariff-calculator/blob/master/skills/skills.go
tags: [tariff, difficulty, code-of-points, skills]
generated: { by: claude-code/cli, at: 2026-10-04T17:45:00Z }
stale_after: 2029-01-01T00:00:00Z
sources:
  - id: cop
    resource: FIG Trampoline Code of Points 2025–2028, §17 and Part II §C (pages 55–56)
    title: FIG Trampoline Code of Points 2025–2028
    author: org:fig
---

# Formula

`TrampolineSkill.SetTariff` in `skills/skills.go` works in tenths, with
*r* = rotation in quarter somersaults and *t* = total half twists.[^cop]

| Rotation | Tariff (tenths) |
|---|---|
| none (*r* = 0) | *t* if twisting; otherwise 1 for a shaped jump (tuck, pike, straddle) or a change between seat and feet; 0 for a straight jump |
| single (1–7) | *r* + *t*, plus 1 for a full somersault or more (*r* ≥ 4), plus 1 more if that has no twist and is straight or piked |
| double (8–11) | *r* + *t* + 2, +1 backward, +2 straight or piked, + (*t* − 4) for twist beyond two full twists |
| triple (12–15) | *r* + *t* + 4, +2 backward, +3 straight or piked, + 2 × (*t* − 2) for twist beyond a full twist |
| quadruple (16) | *r* + 3*t* + 6, +3 backward, +4 straight or piked |

The tariff only depends on rotation, total twist, direction, shape and seat
positions. How the twist is split between somersaults affects the skill's
identity (see [skill model](skill-model.md)), but not its tariff.

The maximum rotation is 16 (a quadruple, the most the Code scores, §17.1.1.5);
`Validate` rejects anything outside 0–16.

# Examples

Computed by the engine:

| Skill | FIG | Tariff |
|---|---|---|
| Half Twist | `(0 1)` | 0.1 |
| Front Tuck | `(4 - o)` | 0.5 |
| Front Pike | `(4 - <)` | 0.6 |
| Tuck Barani | `(4 1 o)` | 0.6 |
| Full Back | `(4 2)` | 0.7 |
| Rudi | `(4 3)` | 0.8 |
| Double Back Tuck | `(8 - - o)` | 1.1 |
| Double Back Pike | `(8 - - <)` | 1.3 |
| Full Full Straight | `(8 2 2 /)` | 1.7 |
| Miller Straight | `(8 3 3 /)` | 2.1 |
| Triple Back Tuck | `(12 - - - o)` | 1.8 |

# How it is verified

`skills/cop_examples_test.go` holds every worked example in the Code of Points
(Part II §C, pages 55–56): 139 singles, doubles, triples and quads in each listed
shape. Each must produce the Code's value exactly. Any change to the formula
that breaks the Code fails the build (see [testing](../architecture/testing.md)).

The worked examples don't cover jumps without rotation or seat landings. The
0.1 for a shaped jump (tuck, pike, straddle) and for a change between seat and
feet (seat drop, seat to feet) is correct under the Code; the maintainer
confirmed it. `skills_test.go` pins it.

# Where it is used

- Every skill card, the picker's tariffs and modifiers ([skill catalog](skill-catalog.md)).
- The routine total, which counts only some skills ([routine validation](routine-validation.md)).
- `difficulty` rules and tariff matchers in [requirements](../requirements/rule-types.md),
  including a per-element cap for age-group competition.

[^cop]: FIG Trampoline Code of Points 2025–2028
