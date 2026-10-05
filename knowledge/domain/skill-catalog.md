---
type: Component
title: Skill catalog and search
description: The common skills with their official names, how the picker groups them into tabs, and how search finds skills by name, alias or FIG notation.
resource: https://github.com/jackpardy/tariff-calculator/tree/master/catalog
tags: [skills, catalog, search, picker]
generated: { by: claude-code/cli, at: 2026-10-05T15:30:00Z }
---

# Common skills

`skills.CommonSkills` is a map of 41 named skills, from Shape Jump and
Seat Drop through Barani, Rudi, Randi, Ball-Out and Cody, to Full Full, Miller
and Triple Back. These are the source of official names: a skill is named by
matching it against this list ([skill model](skill-model.md#derived-rules)). To
add a named skill, add an entry here.

Where the shape matters it's part of the name: first for the single somersaults
(Front, Back, Barani), as coaches say them ("Tuck Back", "Pike Barani"), and
after the name for everything else ("Ball-Out Pike", "Double Back Tuck"). The
picker shows names without the shape, which is chosen on the skill card.

# The picker

`catalog.Categories()` turns the common skills into the picker's tabs:
**Jumps**, **Body landings**, **Singles**, **Doubles**, **Triples**. Each
skill is placed by its rotation and positions: Singles holds every single
somersault, twisting or not, to line up with Doubles and Triples. Body
landings holds only the skills short of a somersault (seat, back and front
drops and their twists); somersaults landing on the body, such as Crash Dive,
Back s/s To Seat and Ball-Out, are Singles. The
drops are further grouped into To seat, From seat, To back or front, and From
back or front, so the tab isn't a wall of buttons.

- The basic jump appears once per shape (Tuck, Pike, Straddle Jump), because
  for a jump the shape is the skill.
- Skills whose shape matters offer tuck, pike and straight. The picker shows the
  base (tuck) tariff and each shape's difference, e.g. +0.1 piked (CoP §17.1.4).
- Labels within a group are short ("½ Twist To Feet" under "From seat").

# Search

`catalog.Search(query)` returns up to 12 results:

- **By name**: each common skill in each shape it can be done in, plus aliases
  people use ("back salto", "back s/s", "3/4 front", "full in rudy out",
  "triffis") and word-level spellings (rudy → rudi, randy → randi).
- **By FIG notation**: a query that looks like notation, e.g. `8 2 3 /`, is
  parsed into a skill. Notation doesn't say which direction, so these results
  are marked `FromNotation` and the user picks one.

A result loads into the skill editor (`POST /skill-inputs`, see
[HTTP routes](../architecture/http-routes.md)).
