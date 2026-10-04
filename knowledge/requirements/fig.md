---
type: Requirements Family
title: FIG age groups (2025–2028)
description: Built-in requirements for FIG Junior and World Age Group Competition (AG1, AG2 and Junior, AG3), covering first-exercise special requirements and second-exercise difficulty caps.
resource: https://github.com/jackpardy/tariff-calculator/tree/master/requirements/sets/fig
tags: [requirements, builtin, fig, age-group]
generated: { by: claude-code/cli, at: 2026-10-04T17:45:00Z }
stale_after: 2029-01-01T00:00:00Z
sources:
  - id: fig-wagc
    resource: FIG Rules for Junior and World Age Group Competition 2025–2028 (V1.0, May 2024), sections 3–5
    title: FIG Rules for Junior and World Age Group Competition 2025–2028
    author: org:fig
---

# Summary

Six files in `requirements/sets/fig/`.[^fig-wagc]

| Group | First exercise (Q1) | Second exercise |
|---|---|---|
| AG1 (11–12) | 10 different elements, at most 2 under ¾ somersault; [special requirements](rule-types.md) (`separate`): landing on the front, landing on the back, at least a full twist with at least a full somersault; no triples. No difficulty scored. | Voluntary; elements over **1.7** count as 1.7; no triples |
| AG2 (13–14) & Junior (15–16) | 10 different elements, at most 1 under ¾ somersault; special requirements: to front or back, from front or back (straight after it), a double front or back, at least 1½ twists with at most 1¼ somersaults; no quads. No difficulty scored. | Voluntary; cap **2.1**; no quads |
| AG3 (17–21) | 10 different elements, every one at least ¾ somersault; special requirements: to front or back, from front or back; no quads. **Two elements score difficulty** (`scored_elements: 2`). | Voluntary; cap **2.2**; no quads |

# How the app uses them

- Special requirements are ticked in the tariff sheet's Req. * column
  ([tariff sheet](../features/tariff-sheet.md)).
- The caps are `difficulty` rules with `cap` and no bounds, so they report the
  capped total ([rule types](rule-types.md)).
- AG3's two scoring elements come from `scored_elements`
  ([routine validation](../domain/routine-validation.md#scoring-only-some-elements)).
  The coach can mark which two score. Repeating one of them in the second
  exercise loses its difficulty there, which the app notes but does not check
  across exercises.

[^fig-wagc]: FIG Rules for Junior and World Age Group Competition 2025–2028
