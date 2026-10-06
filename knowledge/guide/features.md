---
type: Feature List
title: Features
description: Everything the trampoline tariff calculator can do, in plain language for gymnasts, coaches, clubs and competition organisers.
resource: https://tariff.pardy.ie
tags: [guide, features, users]
generated: { by: claude-code/cli, at: 2026-10-06T14:00:00Z }
---

# Work out difficulty

- **Tariff for any skill** under the FIG Code of Points 2025–2028, from a tuck
  jump to a quadruple somersault. Each skill shows its name, its FIG notation
  (e.g. `8 2 3 /`) and its tariff. The maths is checked against every worked
  example in the Code of Points ([tariff](../domain/tariff.md)).
- **Three ways to add a skill**: tap it in the picker (Jumps, Body landings,
  Singles, Doubles, Triples); search by name ("barani", "back
  s/s", "rudy") or by FIG notation ("8 1 1 <"); or build it yourself from
  somersault, twists, direction, take-off and seat landing.
- **Shape made easy**: tuck, pike and straight are offered only where the shape
  changes the skill, and each shows how much it adds or takes away (e.g. +0.1
  for a piked front).
- **Your own labels**: give any skill your club's name for it. The official
  name stays alongside it.

# Build and check routines

- **Routine total**, counting only what the judges would count: the first ten
  skills, each repeated skill once, and nothing after an interruption.
- **Problems flagged on each skill**: a repeat, a take-off that doesn't match
  the last landing, a straight jump or impossible landing that interrupts the
  routine, a tenth skill that doesn't land on feet, and more than ten skills
  ([routine validation](../domain/routine-validation.md)).
- **As many routines as you like**, saved on your phone, each with its own
  name. Reorder skills by dragging or with ↑/↓, edit any skill, duplicate a
  routine to try a change.
- **Side by side**: show two routines together and edit both. Skills that
  differ are shaded.

# Check against a competition

- **Built-in requirements** for BUCS (2026), FIG age groups (2025–2028) and
  British Gymnastics national (2026) and club & regional (2027) levels. Each
  names the rule book it came from
  ([requirements](../requirements/framework.md)).
- **Pass or fail for each requirement**, with the skills that meet it
  highlighted, and special requirements starred.
- **Your own requirements**, for any competition the app doesn't know yet
  (including ISTO): required skills, banned skills, skill counts, difficulty
  limits or caps. Start from scratch, or duplicate a built-in one and adjust
  it. The editor flags rules that contradict each other or can't be met.
- **Choose what is scored**: turn difficulty or the repeat rule off, or score
  only some elements (e.g. 2 in an AG3 first exercise) and pick which ones.

# Levels and set routines

- **Levels** pair a competition's two exercises, such as "set routine option 1
  or option 2, then a voluntary". You see the set routines exactly as
  prescribed, link your voluntary, and both are checked together
  ([levels](../requirements/levels.md)).
- **Carry-over**: where the first exercise scores only some elements, their
  difficulty carries over, and the app flags them if they're repeated in the
  second exercise.
- **Start from a set routine**: copy any set routine into a routine and change
  it from there.
- **Your own set routines**: build one in the Routine Builder and save it as a
  set routine.
- **Your own levels**: choose the structure (a set routine then a voluntary,
  one or two voluntaries, or a set routine for both), then fill in each
  exercise from built-in or your own set routines and requirements.

# Show, print and share

- **View screen**: a full-screen view of a routine, or of a whole level side by
  side, sized to fit without scrolling. Choose what shows: names, FIG notation,
  difficulty, requirements, warnings ([view screen](../features/view.md)).
- **Tariff sheet**: a printable competition card with your details, elements in
  order, FIG notation, difficulty and the Req. * ticks. Print it or save it as a
  PDF ([tariff sheet](../features/tariff-sheet.md)).
- **Compare page**: two routines side by side, read-only.
- **Share by link or QR code**: send routines, requirements or levels to
  someone else's phone, from the Routine Builder or the view screen. Nothing is uploaded; everything travels inside the link
  ([sharing](../features/sharing.md)).

# Competitions and clubs

At <https://tariff.pardy.ie/competitions> (not linked from the calculator yet).

- **Collect every card before the day.** Organisers create a competition, and
  every entry arrives already checked against its level, with its problems
  listed ([competition entries](../features/competition-entries.md)).
- **Clubs gather their members' entries.** Members join with a link and keep
  their own entries; the comp sec sees them all and sends them, then re-sends
  what's changed.
- **Gymnasts without a club** can enter on their own, if the organiser allows.
- **For the organiser**: one table per level with every problem, each entry in
  full, notes back to the club, printed competition cards, a CSV, and closing
  entries when you choose.
- **Video proof by link**, for chosen skills or whole routines: never uploaded.
- **Coach sign-off**: clubs add their coaches, and each coach signs off their
  members' routines on their own page.
- **No accounts**: everyone gets a private link, saved in their browser.

# Good to know

- **No account.** The calculator saves everything in your phone's browser.
  Clearing the browser's data deletes it, so share or export what you want to
  keep. Competition entries are kept on the server, reached by private links,
  and deleted 120 days after the competition.
- **Built-in rules can go out of date.** Check them against your competition's
  current rules.
- The app tells you when it has been updated, with a Refresh button.

See the [user guide](user-guide.md) for how to do each of these.
