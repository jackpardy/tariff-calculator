---
type: Component
title: Skill catalog and search
description: The common skills with their official names, how the picker groups them into tabs, and how search finds skills by name, alias or FIG notation.
resource: https://github.com/jackpardy/tariff-calculator/tree/master/catalog
tags: [skills, catalog, search, picker]
generated: { by: claude-code/cli, at: 2026-10-05T15:30:00Z }
---

# Common skills

`skills.CommonSkills` is a map of 61 named skills, from Shape Jump and
Seat Drop through Barani, Rudi, Randi, Ball-Out and Cody, to the twisting
doubles (Full-In Back, Full-Out Back, Half-Out, Rudi-Out, Randi-Out, Half
Half, Half-In Rudi-Out, Full Full, Full Rudi, Miller…) and triples (Triple
Back, Trif Half-Out, Trif Rudi-Out, Trif Half-In Half-Out, Full Full Full…).
These are the source of official names: a skill is named by matching it
against this list ([skill model](skill-model.md#derived-rules)). To add a
named skill, add an entry here.

Its direction and where its twists go must be right, or it names the wrong
skill. Each named twisting skill is checked against the Code of Points'
worked examples (`skills/cop_examples_test.go`, `TestCommonNamesAreTheCoPs`):
an odd number of half twists turns a somersault round, so a half-in (Half
Half, Half-In Rudi-Out, Miller) is taken off backwards and an odd twist out
(half, rudi, randi) is forwards. Added 2026-10-19 after the audit found
everyday doubles named "Custom Skill". Body landings with names of their own
(a cradle, a corkscrew, a cat twist, swivel hips) keep their descriptive names
("Back Half Twist To Back"; the corkscrew, `2 3` back to back, is "Back 1½
Twist To Back", added 2026-10-19), with the coaches' name said beside them in
a search.

Where the shape matters it's part of the name: first for the single somersaults
(Front, Back, Barani, Back Half), as coaches say them ("Tuck Back", "Pike Barani"), and
after the name for everything else ("Ball-Out Pike", "Double Back Tuck"). The
picker shows names without the shape, which is chosen on the skill card.

# The picker

`catalog.Categories()` turns the common skills into the picker's tabs:
**Jumps**, **Body landings**, **Singles**, **Doubles**, **Triples**. Each
skill is placed by its rotation and positions: Singles holds every single
somersault, twisting or not, to line up with Doubles and Triples. Body
landings holds only the skills short of a somersault (seat, back and front
drops and their twists); somersaults landing on the body, such as Crash Dive,
Back s/s To Seat and Ball-Out, are Singles.

## Body landings follow the skill before

The Body landings tab is grouped by where the skill lands: **To feet**,
**To seat**, **To back**, **To front**. Each tile is a landing and a twist
(`catalog.Tile`), not a take-off: "½ Twist To Seat" takes off from wherever
the skill before it landed (the end of the routine Add puts it in, or the one
before the skill being edited; feet at the start). The server works the
version out (`Entry.Follow`): with that take-off, that landing and that twist,
the least rotation that lands there by the skills package's own rules (a back
drop from feet is a quarter backward, front to back a half somersault, a back
to back with a full twist none), up to a half somersault. Between two that
turn as little (a half somersault either way) it's the catalogue's own, else
forward. It must have a name: the catalogue's, or one `FindCommonSkillName`
makes from its parts, as long as that isn't another skill's name (a back drop
to seat would read "Back To Seat", the somersault's name). Otherwise, and from
feet to feet (a jump), it's the tile's usual skill, and the routine flags the
bad transition as before.

| Tile | after feet | after seat | after back | after front |
|---|---|---|---|---|
| To Feet | Seat To Feet* | Seat To Feet | Back To Feet | Front To Feet |
| ½ Twist To Feet | Seat Half Twist To Feet* | Seat Half Twist To Feet | Back Half Twist To Feet | Front Half Twist To Feet |
| To Seat | Seat Drop | Seat To Seat | Seat Drop* | Seat Drop* |
| ½ Twist To Seat | Half Twist To Seat | Seat Half Twist To Seat (Swivel Hips) | Back Half Twist To Seat | Front Half Twist To Seat |
| To Back | Back Drop | Seat To Back | Back To Back | Front To Back |
| ½ Twist To Back | Half Twist To Back | Seat Half Twist To Back | Back Half Twist To Back (Cradle) | Front Half Twist To Back |
| Full Twist To Back | Full Twist To Back | Seat Full Twist To Back | Back Full Twist To Back (Cat Twist) | Front Full Twist To Back |
| 1½ Twist To Back | 1½ Twist To Back | Seat 1½ Twist To Back | Back 1½ Twist To Back (Corkscrew) | Front 1½ Twist To Back |
| To Front | Front Drop | Seat To Front | Back To Front | Front To Front |
| ½ Twist To Front | Half Twist To Front | Seat Half Twist To Front | Back Half Twist To Front | Front Half Twist To Front |

\* the tile's usual skill: nothing lands like that from there.

Every body landing in the catalogue is some tile after some landing, so none
is a box of its own (`Categories` would list one that weren't). The tiles
show no tariff, as it depends on the skill before; the card shows the version
chosen, its tariff, and why ("From seat, after Seat Drop · also called Swivel Hips"). The club skills
page lists each tile's catalogue versions. Search is unchanged: it finds
exact skills by name or notation, and the names say where they take off.

- The basic jump appears once per shape (Tuck, Pike, Straddle Jump), because
  for a jump the shape is the skill.
- Skills whose shape matters offer tuck, pike and straight. The picker shows the
  base (tuck) tariff and each shape's difference, e.g. +0.1 piked (CoP §17.1.4).
- Labels within a group say where the skill lands ("½ Twist To Back" under "To back").

# Search

`catalog.Search(query)` returns up to 12 results:

- **By name**: each common skill in each shape it can be done in, plus aliases
  people use ("back salto", "back s/s", "3/4 front", "full in rudy out",
  "half twist jump", "back drop to feet", "rudi out triffis", the CoP's own
  "back in full out" and "front front half"), the rule books' and coaches'
  names said beside a result (`AlsoCalled`: "Back s/s", "Swivel Hips",
  "Cradle", "Corkscrew", "Cat Twist"), and word-level spellings (rudy → rudi, randy →
  randi). "½", "1/2" and "half" are the same, and a shape word (tuck/tucked,
  pike/piked, straight/layout/laid out, straddle) picks the shape rather than
  being looked for in the name, so "tuck back s/s" is the Tuck Back. Exact
  names come first, then names starting with the query, then the rest,
  shorter first. "Triffis" means a twisting triple (as the CoP's
  "quadriffis"), so it finds the Trif skills, not the Triple Back (2026-10-19).
- **By FIG notation**: a query that looks like notation, e.g. `8 2 3 /`, is
  parsed into a skill. Notation doesn't say which direction or where it takes
  off, so each reading is offered forwards and backwards from feet, the back,
  the front and the seat (2026-10-16: "6 0 /" lands only from the back or
  front, and "5 0 /" is Ball-Out and Cody), named skills first, each saying
  its take-off and landing. Where one reading has a name, the unnamed ones
  from the back, front or seat are left out ("8 2 - o" is the Full-In Back,
  or one from feet forwards; 2026-10-19). These results are marked `FromNotation`: the
  first loads onto the skill card as it's typed, and one that isn't a named
  skill opens **Build or adjust this skill** too, to finish it. Notation that
  no take-off lands (`catalog.NotationReading`) still goes into the builder,
  opened.

A result loads into the skill editor (`POST /skill-inputs`, see
[HTTP routes](../architecture/http-routes.md)).
