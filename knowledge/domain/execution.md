---
type: Domain Model
title: Execution judging
description: How execution judges assess a skill under the FIG Code of Points 2025–2028 (shape, legs, arms, opening, twist, landing) and how the app works out which deductions apply to any skill from its attributes, for the "How it's judged" pop-up.
resource: https://github.com/jackpardy/tariff-calculator/blob/master/judging/judging.go
tags: [execution, judging, deductions, code-of-points, skills]
generated: { by: claude-code/cli, at: 2026-10-07T17:00:00Z }
stale_after: 2029-01-01T00:00:00Z
sources:
  - id: cop
    resource: FIG Trampoline Code of Points 2025–2028, §13, §16, §17.2, §20, §22 and Part II §A drawings (pages 49–53)
    title: FIG Trampoline Code of Points 2025–2028
    author: org:fig
---

# What it's for

Every skill card in the [routine builder](../features/routine-builder.md), and
the "Add a skill" card, has a **How it's judged** button. It opens a pop-up
listing what execution judges can take off for that skill, each fault with its
amount, a plain-English explanation and the Code of Points reference.[^cop]
It's for students and coaches learning what to work on, not a score.

# Derived, not listed

Nothing is stored per skill. `judging.For(skill, last)` works out a few facts
from the [skill model](skill-model.md) and runs a table of rules, each a
condition on those facts and the item it adds. So every skill the builder can
make, named or custom, gets its own list.

| Fact | From the skill |
|---|---|
| jump | No somersault rotation, feet to feet (basic and twisting jumps) |
| somersault | ¾ somersault (270°) or more |
| multiple | 630° or more, the Code's "multiple somersault" |
| shape judged | The skill's shape where [shape is relevant](skill-model.md#derived-rules); straight for twisting jumps and singles with a full twist or more; none for drops |
| twist | Half twists in all, and in the last somersault |
| last | The element ends the exercise: the 10th, or the last of a shorter routine |

# The rules

Each element loses 0.0–0.5 from each execution judge (§20.1); form isn't
judged after 3 o'clock, the landing preparation (§20.2.1).

| Group | Fault | Amount | Applies to | CoP |
|---|---|---|---|---|
| Body shape | Open tuck | 0.1–0.2 | Tuck | §13.4, Part II A |
| | Hands behind the knees | 0.1 | Tuck | §13.5, §20.2.1.1 |
| | Open pike | 0.1–0.2 | Pike, not jumps | §13.4–13.5, Part II A |
| | Legs below horizontal | 0.1–0.2 | Pike and straddle jumps | Part II A |
| | Piked or arched body | 0.1–0.2 | Straight (incl. judged straight) | §13.2.1, Part II A |
| | Bent knees | 0.1–0.2 | Pike, straight, straddle | §20.2.1.2 |
| | *When the shape is judged* (note) | | Multiple somersaults: from 90° backward, 135° forward | Part II A |
| Legs and feet | Feet apart, knees apart | 0.1 each | All but straddle jumps | §13.3, §20.2.1.2 |
| | Toes not pointed | 0.1 | All | §13.3, §20.2.1.2 |
| Arms | Elbows away from the body | 0.1 | All | §13.6, §20.2.1.1 |
| | Bent elbows | 0.1 | 540° (1½ twists) or less | §20.2.1.1 |
| | *Bent elbows allowed* (note) | | 720° or more | §20.2.1.1 |
| | Arms too wide stopping the twist | 0.1 | Twisting somersaults: 45° for a barani, full or multiple with only a half out; 90° otherwise | §13.6, Part II A |
| Opening and twist | Late or no opening | 0.1–0.3 | Tuck and pike somersaults: open by 1 o'clock; 1–2: 0.1, 2–3: 0.2, never: 0.3 | §20.2.1.3 |
| | *No opening needed* (note) | | Straight somersaults | §20.2.1.3 |
| | Piking down | 0.1–0.2 | Somersaults: 170°–136° at the hips: 0.1; 135° or less: 0.2 | §20.2.1.5 |
| | Twist finishing late | 0.3 | More than 360° of twist in the last somersault | §20.2.1.4 |
| Landing | Steps or bounces, arm movements | 0.1 each | Last element | §20.2.2.1.2 |
| | Not standing upright | 0.1 | Last element | §20.2.2.1.2 |
| | Turning to the judges too soon | 0.2 | Last element | §16.3, §20.2.2.1.2 |
| | Uncontrolled out-bounce | 0.1 | Last element | §16.2, §20.2.2.1.1 |
| | Touching the bed with hands; touching anything else | 0.5 each | Last element | §20.2.2.2–3 |
| | Falling; off the bed or a somersault to save it; an extra element | 1.0 each | Last element | §20.2.2.4–6 |

Under every list, **How the score adds up** says: up to 0.5 per element
(§20.1); execution is 20 minus, for each element and the landing, the two
middle judges' deductions added (§17.2.3.2); horizontal displacement takes
0.0–0.3 off 10 per element by landing zone (§17.2.4, §22); time of flight is
measured (§17.2.5).

# Limits

- Trampoline only: synchro, tumbling and DMT aren't covered.
- Each fault's range is the Code's; judges decide where in it a skill falls.
  The pop-up doesn't score a performance.
- The Code's [interruptions](routine-validation.md) (§15) are flagged on the
  routine itself, not here.

# Tests

`judging/judging_test.go` pins named cases (tuck and straddle jumps, a twisting
jump, seat and back drops, tuck and straight backs, Rudi, double full, Half-Out
pike, the last element, and the arm angle for each kind of twist) and runs the
rules over every skill the builder can make, checking each list is complete
and consistent: somersaults always get piking down, nothing else gets opening
rules, and nothing both opens and needn't. `main_test.go` covers `GET /judging`
and the cards' links.

[^cop]: FIG Trampoline Code of Points 2025–2028, Part I (TRA) §13, §16, §17.2,
    §20, §22 and Part II §A.
