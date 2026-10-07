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

Answered 2026-10-07: ISTO runs over 2½–3 days; panels default to the Code of
Points' and can be changed; minutes per competitor and who may judge what are
settings (ADR 0005 Decisions 6 and 13). Still to learn:

- Better default minutes per competitor for tumbling and DMT (2 is a guess),
  and the usual rest between a person's turns (20 minutes is a guess).

# Notifications

Decided 2026-10-07 ([roadmap](roadmap.md#next-steps-proposed-2026-10-07)):
opt-in notifications by phone push and email. Push stores nothing
personal. An email is given with a tick box, "I'm 18 or over, or this is a
parent's email"; anyone younger can still use push (decided 2026-10-07).
Still to decide:

- **How long the grace period is** before a notification goes out (about
  10 minutes suggested; the organiser can skip it with Notify now or turn
  it off on the competition's days, see the
  [roadmap](roadmap.md#next-steps-proposed-2026-10-07)), and whether there
  are quiet hours overnight before the competition day.

Left to the build: which service sends email (Amazon SES in its Ireland
region is the leaning).

# Approved coaches

Decided 2026-10-07 ([roadmap](roadmap.md#next-steps-proposed-2026-10-07)):
clubs and individuals send coaches with a certificate, and the organiser
approves each for their competition. Still to decide:

- **Which qualifications** the organiser chooses from (British Gymnastics
  and Gymnastics Ireland coaching levels, by discipline), or whether they
  just describe what they need.
- **Uploads:** largest file, which types (photo, PDF), and whether a club
  can send the same certificate to its next competition without uploading
  it again (it would still be approved afresh).
- **A certificate's expiry date:** asked for, and checked against the
  competition's date?

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
