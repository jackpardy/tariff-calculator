---
type: Open Questions
title: Open questions
description: Rule documents, wording questions and device checks the project is waiting on, and who or what each one waits for.
tags: [todo, requirements, testing]
generated: { by: claude-code/cli, at: 2026-10-05T16:00:00Z }
---

# Waiting on documents

- **ISTO levels.** The Irish Student Trampoline Open's level rules aren't
  online (its site has lapsed). They need its documents before they can become
  [built-in requirements](requirements/framework.md#built-in-requirements).
  Not expected for a while (2026-10-05); until then, users write them as
  their own requirements.
- **Gymnastics Ireland levels.** Not published; the development plan goes to
  club secretaries on request.

# Rule wording to confirm

From the [British Gymnastics requirements](requirements/bg-national.md):

- Does "360° somersault" mean exactly 360°, or at least 360°?
- What counts as a "double" in each rule?
- Are the national qualifying scores (difficulty and total) in the set
  descriptions current?

# For ISTO's timetable (ADR 0005)

- How many officials each discipline's panel needs at ISTO (execution and
  difficulty judges, chair, recorder), and whether that changes by level.
- Typical minutes per competitor for tumbling and DMT (two passes each).
- Whether judging any level needs a judging qualification, or a competitor
  can judge any level below their own.
- Whether ISTO runs over one day or two, and its breaks.

# Decisions pending

- Whether to mark [ADR 0003](../docs/adr/0003-requirements-framework.md)
  Accepted.

# Not yet tested on a real phone

Drag to reorder, printing the [tariff sheet](features/tariff-sheet.md), the
share sheet and QR scanning ([sharing](features/sharing.md)), the keyboard
appearing with the skill card open, Levels mode's tabs on a narrow screen, and
the [view screen](features/view.md) filling a phone or tablet held either way.

Checked on an iPhone (2026-10-05): the tariff sheet scrolls to its toolbar, and
the pop-up menus (the builder's More, "Show on sheet") stay on screen.
