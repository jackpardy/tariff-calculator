---
type: Requirements Family
title: BUCS student championships (2026)
description: Built-in requirements for the BUCS Trampoline Championships 2026, covering levels 1–7, Disability 1–2 and FIG level, with set-routine options and difficulty bands.
resource: https://github.com/jackpardy/tariff-calculator/tree/master/requirements/sets/bucs
tags: [requirements, builtin, bucs, student]
generated: { by: claude-code/cli, at: 2026-10-04T17:45:00Z }
stale_after: 2027-01-01T00:00:00Z
sources:
  - id: bucs-2026
    resource: "BUCS Trampoline and DMT Championships 2026: Competition Structure (version 1)"
    title: BUCS Trampoline and DMT Championships 2026, Competition Structure v1
    author: org:bucs
---

# Summary

26 files in `requirements/sets/bucs/`, all citing the 2026 competition
structure.[^bucs-2026] Re-check against the next season's structure once it is
published (hence `stale_after`).

| Level | First exercise | Second exercise (and final) difficulty |
|---|---|---|
| FIG level | Any exercise: 10 elements, at most two front or back landings (more is an interruption; seat landings don't count), difficulty at least 8.1. Two voluntaries, the same one may be repeated; the higher counts. | (same) |
| L1 | Voluntary: 10 different elements, at least 9 somersaults of 270°+, includes "¾ to front or back then 1¼" **or** "a full with a full twist", no triples | 5.9–8.0, no triples |
| L2 | Voluntary: 10 different elements, at least 7 somersaults of 270°+, an includes rule like L1, nothing over 630° | 4.1–5.8, nothing over 630° |
| L3 | [Set routine](set-routines.md), option 1 or 2 | 3.2–4.0, nothing over 360°, no more than ½ twist in a somersault |
| L4 | Set routine, option 1 or 2 | 2.4–3.1, as L3 |
| L5 | Set routine, option 1 or 2 | 2.0–2.3, at most 2 somersaults, no twisting somersaults |
| L6 | Set routine, option 1 or 2 | 1.6–1.9, at most 1 somersault, no twisting somersaults |
| L7 | Set routine, option 1 or 2 | 1.0–1.5, every element at most 90° somersault and 180° twist |
| Disability L1 | Set routine, option 1 or 2 | at least 2.0 |
| Disability L2 | Set routine, option 1 or 2 | 1.0–1.9 |

Penalties are given in the descriptions. In a voluntary first exercise each
missing requirement is a 2.0 penalty, and exceeding the level is
disqualification. In a second exercise, difficulty below the minimum is a 2.0
penalty and no final, and above the maximum is disqualification.

# Notes

- The L1/L2 "includes" requirement is why the `includes` rule type exists
  ([rule types](rule-types.md)).
- These are the most relevant built-ins for the app's student users. ISTO's
  own levels are still [waiting for documents](../open-questions.md).

[^bucs-2026]: BUCS Trampoline and DMT Championships 2026, Competition Structure v1
