---
type: Validation Rules
title: Routine validation
description: How a routine is checked against the Code of Points (repeats, transitions, interruptions, the tenth skill, the ten-element limit) and which skills count towards the total.
resource: https://github.com/jackpardy/tariff-calculator/blob/master/skills/skills.go
tags: [routine, validation, code-of-points, skills]
generated: { by: claude-code/cli, at: 2026-10-04T17:45:00Z }
sources:
  - id: cop
    resource: FIG Trampoline Code of Points 2025–2028, §4, §12, §14–§16
    title: FIG Trampoline Code of Points 2025–2028
    author: org:fig
---

# What is checked

`skills.ValidateRoutine` (and `ValidateRoutineWith`, which takes options) walks
the routine once and returns a `RoutineValidation`. This holds the per-skill
flags and messages, the counted `TotalTariff`, the uncounted `RawTariff`, and
routine-level flags. It recomputes every tariff, so input never needs to be
pre-priced.[^cop]

| Rule | CoP | Effect |
|---|---|---|
| First skill starts from feet | §12.3 | "Must Start From Feet"; flagged as a bad transition |
| Each take-off matches the previous landing | — | "Bad Transition: Back -> Feet"; flagged, does **not** interrupt |
| Repeated element | §14.1 | Both copies marked "Duplicate"; only the first counts |
| Straight jump mid-routine | §15.1.3 | Interrupts the routine |
| Impossible landing | §15.1.4 | Interrupts the routine |
| After an interruption | §15.2, §15.3 | The interrupting skill and everything after count nothing |
| Tenth skill lands on feet | §16.1 | "10th Must Land Feet" warning |
| Only ten elements | §4.1, §16.5 | Skills after the tenth count nothing; `RoutineTooLong` set |

Bad transitions are flagged but don't interrupt. They describe a routine that
couldn't be performed as written, not something that happened during one.

**Counted** means the skill's tariff is in `TotalTariff`: it isn't a repeat,
it's within the first ten, and the routine hasn't been interrupted yet.

# Options

`ValidateOptions` relax the Code for routines that aren't judged by it in full.
The routine's [checks](../features/routine-builder.md#checks) set them, starting
from what its [requirements](../requirements/framework.md#scoring) say:

- `AllowRepeats`: repeats are neither flagged nor discounted. Used for
  [set routines](../requirements/set-routines.md).
- `ScoredElements`: only *n* elements score difficulty.
- `ScoredEarlier`: elements that scored in the first exercise. A repeat of
  one (by the §14 definition) is marked `ScoredEarlier`, flagged "Scored In 1st
  Exercise (No Tariff)" and not counted. Set for a second exercise at a
  [level](../requirements/levels.md) with `scored_once` (FIG AG3).

## Scoring only some elements

With `ScoredElements` = *n* (for example 2 in a FIG AG3 or BG 17–21 first
exercise), the counted elements that score are:

1. those the coach marked with `Scores`, the first *n* of them in order; or
2. if none are marked, the *n* highest tariffs, taking the earlier one on a tie.

The rest are marked `Unscored`. `ScoringOverChosen` reports how many more are
marked than may score.

# Where it shows

The routine builder's cards, flags and totals, the
[tariff sheet](../features/tariff-sheet.md) and the [comparison](../features/compare.md)
all render a `RoutineValidation`. [Requirements](../requirements/framework.md) are
evaluated on it too. Difficulty rules use the counted total, while every other
rule looks at the routine as written.

[^cop]: FIG Trampoline Code of Points 2025–2028
