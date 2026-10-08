---
type: Reference
title: Rule types and matchers
description: The ten rule types a list of requirements can contain (count, linked, every, elements, difficulty, position, sequence, separate, includes, different) and the matcher that describes elements.
resource: https://github.com/jackpardy/tariff-calculator/blob/master/requirements/requirements.go
tags: [requirements, reference, json]
generated: { by: claude-code/cli, at: 2026-10-04T17:45:00Z }
---

# Rule types

Each rule is a JSON object with a `type` and an optional `label`, the author's
own wording. Without a label, a plain-English description is generated
(`requirements/describe.go`). The editor offers the types in the order below.

| Type | Fields | Passes when | Example |
|---|---|---|---|
| `separate` | `each`: matchers | Each matcher is met by a **different** element. This is a maximum bipartite matching, so an element that could meet two requirements is used where it's needed most. | FIG/BG first exercise special requirements: "to front or back", "from front or back" |
| `includes` | `options`: lists of matchers | At least one option appears, its elements one straight after another | BUCS L1: "¾ to front or back then 1¼, **or** a full with a full twist" |
| `count` | `match`, `min`/`max` (whole) | The number of matching elements is in range | Required (`min: 1`), forbidden (`max: 0`), "at most 2 doubles" |
| `linked` | `match`, `min`/`max` (whole) | The number of pairs of matching elements one straight after another is in range. Pairs overlap: three in a row are two pairs. | ISTO: "no linked somersaults" (`max: 0`), "at most 1 linked somersault pair" |
| `every` | `match` | Every element matches | "Every element has at least ¾ somersault" |
| `different` | — | No element repeats, using CoP §14's definition ([skill model](../domain/skill-model.md#derived-rules)) | "10 different elements" |
| `elements` | `min`/`max` (whole) | The number of elements is in range | Exactly 10 |
| `difficulty` | `min`/`max`, optional `cap` | The counted difficulty is in range. With `cap`, each element counts at most the cap (CoP §17.1, age groups); a cap with no bounds just reports the capped total. | BUCS L1 second: 5.9–8.0; FIG AG1: cap 1.7 |
| `position` | `position` (1-based), `match` | The element at that position matches | "Finish with a back somersault" |
| `sequence` | `sequence`: matchers | Element *i* matches matcher *i*, with exactly that many elements. This makes the requirements a [set routine](set-routines.md). | BG Club L1 |

What each result reports in `Elements`: the matching elements for `count` and
`includes`, those in a linked pair for `linked`, and the offending ones for `every`, `position`, `sequence` and
`different`. For `separate`, `Assigned` holds the element meeting each
requirement (0 where none can). These are the elements starred on a competition
card.

# Matchers

A matcher describes elements. Every condition it gives must hold, and anything
left out matches anything.

| Field | Type | Meaning |
|---|---|---|
| `label` | text | The author's wording, e.g. "Back somersault (T)", "Landing on the front" |
| `rotation` | `{min, max}` | Quarter somersaults |
| `direction` | `forward` / `backward` | |
| `twist` | `{min, max}` | Total half twists |
| `shapes` | list | `tuck`, `pike`, `straight`, `straddle`. A skill whose shape [isn't relevant](../domain/skill-model.md#derived-rules) counts as straight. |
| `takeoff`, `landing` | list | `feet`, `front`, `back`, `seat` |
| `tariff` | `{min, max}` | The skill's tariff |
| `fig` | text | Exact FIG notation; brackets and spaces are ignored, so `4-o` equals `(4 - o)` |

Matchers cover nearly every element requirement in the known rule books. The
`cel-go` expression rule that ADR 0001 planned was dropped until a real rule
needs it. If a rule doesn't fit, add a rule type and bump `format`.

`Matcher.Example()` builds the simplest skill a matcher allows. This is how a
[set routine](set-routines.md) is loaded into the builder.
