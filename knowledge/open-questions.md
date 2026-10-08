---
type: Open Questions
title: Open questions
description: Rule documents, wording questions and device checks the project is waiting on, and who or what each one waits for.
tags: [todo, requirements, testing]
generated: { by: claude-code/cli, at: 2026-10-05T16:00:00Z }
---

# Waiting on documents

- **ISTO's score sheets.** Example paper score sheets for each event
  (trampoline, synchro, tumbling, DMT), to be got from ISTO (2026-10-08).
  The [recorders' score sheets](features/competition-entries.md) are built
  to a general layout; their format may change to match ISTO's.

- **ISTO's 2027 routines.** The [ISTO levels](requirements/isto.md) are built
  in from the 2025 document (2026-10-08); swap in 2027's when it comes.
- **Gymnastics Ireland levels.** Not published; the development plan goes to
  club secretaries on request.

# Rule wording to confirm

From the [British Gymnastics requirements](requirements/bg-national.md):

- Does "360° somersault" mean exactly 360°, or at least 360°?
- What counts as a "double" in each rule?
- Are the national qualifying scores (difficulty and total) in the set
  descriptions current?

# ISTO's routines to confirm

Answered 2026-10-08 by an ISTO organiser (see [ISTO](requirements/isto.md)):
set A or B then a voluntary; set routines don't score difficulty; 2.0
penalty outside the tariff band; linked somersaults as built; Elite's full
twist is exactly a full; Disability L4's set is 2.0. Disability Level 5 is an old routine and
is left out.

# For ISTO's timetable (ADR 0005)

Answered 2026-10-07: ISTO runs over 2½–3 days; panels default to the Code of
Points' and can be changed; minutes per competitor and who may judge what are
settings (ADR 0005 Decisions 6 and 13). Still to learn:

- Better default minutes per competitor for tumbling and DMT (2 is a guess),
  and the usual rest between a person's turns (20 minutes is a guess).

# Notifications

Decided 2026-10-07 ([roadmap](roadmap.md#next-steps-2026-10-08)):
opt-in notifications by phone push and email. Push stores nothing
personal. An email is given with a tick box, "I'm 18 or over, or this is a
parent's email"; anyone younger can still use push (decided 2026-10-07).
Designed in [ADR 0008](../docs/adr/0008-notifications.md) (2026-10-08):
opting in per competition, emails confirmed, a ten-minute grace period by
default, no quiet hours, and email by SMTP (Amazon SES in its Ireland
region is the leaning). Still to do on the server: an email provider, with
SPF and DKIM for `tariff.pardy.ie`.

# Removing entries

Decided 2026-10-08 ([roadmap](roadmap.md#next-steps-2026-10-08)): remove
(blocked) or put on hold asking for changes, ticked one by one or a club at
once, with an optional reason for the club, the member or both, and
restorable; a held entry sent again waits for the organiser to accept it.
Nothing left to decide.

# Approved coaches

Decided 2026-10-07 and 2026-10-08
([roadmap](roadmap.md#next-steps-2026-10-08)): clubs and individuals send
coaches with a certificate, and the organiser approves each for their
competition against a qualification picked from a list (a higher level
counting for a lower one). No expiry date is asked for. A club's
certificate is kept with the club for its next competitions. Still to
decide:

- **The list itself:** which British Gymnastics and Gymnastics Ireland
  coaching levels, by discipline, and their names.
- **Uploads:** largest file, and which types (photo, PDF).

# Decisions pending

- Whether to mark [ADR 0003](../docs/adr/0003-requirements-framework.md)
  Accepted.

# Not yet tested on a real phone

Push notifications (on Android, and on an iPhone from the home screen), drag to reorder, printing the [tariff sheet](features/tariff-sheet.md), the
share sheet and QR scanning ([sharing](features/sharing.md)), the keyboard
appearing with the skill card open, Levels mode's tabs on a narrow screen, and
the [view screen](features/view.md) filling a phone or tablet held either way.

Checked on an iPhone (2026-10-05): the tariff sheet scrolls to its toolbar, and
the pop-up menus (the builder's More, "Show on sheet") stay on screen.
