---
type: Domain Model
title: Skill model
description: How a trampoline skill is represented (rotation, twist per phase, shape, take-off, direction, seat landing) and the rules derived from it, such as landing position, shape relevance, FIG notation, naming and repetition.
resource: https://github.com/jackpardy/tariff-calculator/blob/master/skills/skills.go
tags: [skills, domain-model, fig-notation, code-of-points]
generated: { by: claude-code/cli, at: 2026-10-04T17:45:00Z }
sources:
  - id: cop
    resource: FIG Trampoline Code of Points 2025–2028
    title: FIG Trampoline Code of Points 2025–2028
    author: org:fig
---

# Schema

`skills.TrampolineSkill` is the canonical form of a skill. Routines are stored
and posted as a JSON array of these, both in the browser and in
[share links](../features/sharing.md).

| Field | JSON | Meaning |
|---|---|---|
| `Rotation` | `rotation` | Somersault rotation in quarters (90°), 0–16. |
| `TwistDistribution` | `twist_distribution` | Half twists per twist phase: one entry per phase. |
| `TakeoffPosition` | `takeoff_position` | `Feet`, `Front`, `Back` or `Seat`. |
| `Shape` | `shape` | `Straight`, `Tuck`, `Pike` or `Straddle`. |
| `Backward` | `backward` | Direction of rotation. |
| `SeatLanding` | `seat_landing` | Lands in seat (only valid where the rotation would land on feet). |
| `Name` | `name` | Official name. Always derived by the server and never trusted from input. |
| `CustomName` | `custom_name` | Optional label set by the user, at most 60 characters, shown alongside the official name. |
| `Tariff` | `tariff` | Derived; see [tariff](tariff.md). |
| `Scores` | `scores` | The coach chose this element to score when only some do (see [routine validation](routine-validation.md#scoring-only-some-elements)). |

Positions and shapes are written as names in JSON (custom marshalers). Unknown
names parse to `Invalid` / `InvalidShape`, which `Validate` rejects.

# Twist phases

A multiple somersault has one twist count per somersault, so the Code can tell
a Half-Out `(8 - 1)` from a Half-In `(8 1 -)`. `CalculatePhases` gives the
number of phases: 1 up to 6 quarters, 2 up to 10, 3 up to 14, 4 for 15–16.
`NormalizePhases` trims or zero-pads the array to that length. The server does
this to every incoming skill, so a form whose rotation just changed still parses.

# Derived rules

All of these live only in Go ([ADR 0002](../../docs/adr/0002-server-rendered-frontend.md) §2).
Earlier versions had JavaScript copies of them, which were removed.

- **Landing position** (`LandingPosition`). Take-off angle ± rotation, mirrored
  by an odd number of half twists. Seat landing is only possible where the
  result is feet; anything else is `Invalid`.
- **Basic jump** (`IsBasicJump`). No rotation, no twist, no seat. The shape *is*
  the skill: Straight, Tuck, Pike or Straddle Jump. Straddle only exists here.
  The server turns a straddle on any other skill into straight.
- **Straight jump** (`IsStraightJump`). A plain straight jump, feet to feet.
  Inside a routine it interrupts the exercise ([routine validation](routine-validation.md)).
- **Shape relevance** (`ShapeIsRelevant`). Whether shape is part of the skill's
  identity. It is for basic jumps, singles of ¾ to 1¼ somersault with less than
  a full twist, and anything from 1½ somersaults up (CoP §14.5.3, §17.1.5). It
  isn't for twisting jumps, drops, or singles with a full twist or more, which
  are treated as straight. Naming, FIG notation, the form and
  [requirements matchers](../requirements/rule-types.md#matchers) all use this
  one rule.[^cop]
- **FIG notation** (`FIGNotation`). Rotation, then twist per phase (`-` for
  none), then the shape symbol where shape is relevant: `o` tuck, `<` pike, `/`
  straight, `v` straddle. Examples: `(4 - o)`, `(8 2 3 /)`, `(o)` for a tuck
  jump, and empty for a straight jump.
- **Name** (`FindCommonSkillName`). Matches the skill against the
  [common skills](skill-catalog.md) on rotation, take-off, direction, seat
  landing and twist per phase, then adds the shape where it is relevant:
  first for single fronts, backs and baranis ("Tuck Back", "Pike Barani"), as
  coaches say them, and after the name for everything else ("Ball-Out Pike",
  "Double Back Tuck"). Unknown combinations are "Custom Skill".
- **Repetition** (`Equal`). Two skills are the same element under CoP §14 if
  rotation, total twist, direction, take-off and seat landing match. Shape then
  distinguishes basic jumps, singles of ¾–1¼ with less than a full twist, and
  1½ and above. For 1½ and above the twist per phase must also match.

# Validation

`Validate` accepts only what the engine can score: rotation 0–16, exactly one
non-negative twist count per phase, a known take-off and shape, and a custom
name of at most 60 characters. Every handler validates input before using it
([HTTP routes](../architecture/http-routes.md)).

[^cop]: FIG Trampoline Code of Points 2025–2028
