---
type: Requirements Family
title: ISTO, Irish Student Trampoline Open (2025)
description: Built-in requirements for ISTO's trampoline levels (Novice, Intermediate, Intervanced, Advanced, Elite, Elite-Pro) and Disability Levels 1–4, from its 2025 routines document, with set routines, voluntary rules and tariff bands.
resource: https://github.com/jackpardy/tariff-calculator/tree/master/requirements/sets/isto
tags: [requirements, builtin, isto, student]
generated: { by: claude-code/cli, at: 2026-10-08T22:00:00Z }
stale_after: 2027-02-01T00:00:00Z
sources:
  - id: isto-2025
    resource: "ISTO 2025 Routines (updated)"
    title: ISTO 2025 Routines (updated), the committee's routines document
    author: org:isto
---

# Summary

22 files in `requirements/sets/isto/`, all citing ISTO's 2025 routines
document.[^isto-2025] They are the levels for ISTO 2027 until its 2027
document arrives (hence `stale_after`). The group is listed first, as ISTO is
the app's main audience.

| Level | First exercise | Second exercise (voluntary) |
|---|---|---|
| Novice | Set A or B (tariff 1.1) | 1.1–1.5; nothing over 270° somersault; no twisting somersaults |
| Intermediate | Set A or B (1.6) | 1.6–2.3; 1–2 somersaults of 270°–360°, no twist in them, none linked |
| Intervanced | Set A or B (2.4) | 2.4–3.1; 3–4 somersaults of 270°–360°, at most 180° twist in them, at most 1 linked pair |
| Advanced | Set A or B (3.2) | 3.2–4.2; 5–7 somersaults of 270°–360°, at most 180° twist in them, at most 1 linked pair |
| Elite | Voluntary, both exercises the same rules | 4.3–6.2; 7–9 somersaults of 270°+; a full back **or** a rudi (from feet); a 270° to front or back then a 450° with at most 540° twist; nothing over 720° or 900° twist; at most one skill over 450°, untwisted |
| Elite-Pro | Voluntary, both exercises the same rules | at least 6.3; every skill 270°+; nothing over 1080° |
| Disability L1–L2 (Band 1) | Set routine (1.0, 1.2) | 1.0–1.1 / 1.2–1.4; nothing over 270°, no twisting somersaults, at most one somersault |
| Disability L3–L4 (Band 2) | Set routine (1.5, 2.0) | 1.5–1.8: as Intermediate / 1.9–2.3: 2–4 somersaults, at most 180° twist in them |

Novice to Advanced do Set A or Set B, then a voluntary that meets the
level's requirements. Every voluntary needs 10 different skills; below a
level's minimum tariff or above its maximum is a 2.0 penalty. A "somersault
skill" is one of 270° or more, and a "linked" pair two somersault skills one
straight after another (the [`linked` rule](rule-types.md)). Elite's "360°
twist" is exactly a full.
Set routines don't score difficulty: the tariff printed with each is for
information, and a level's Set A and B are the same
(`TestISTOSetsSameDifficulty`). All confirmed by an ISTO organiser
(2026-10-08).

# Checked against the document

- Each set routine's elements add up to the tariff printed, except Disability
  Level 4: the document says 1.9, its elements add up to 2.0 (the organiser
  agrees). It's the same routine as BUCS L5 option 1.
- Each set also meets its level's
  voluntary rules (`TestISTOSetsMeetVoluntaries`), a check that the rules are
  read as meant.
- **Disability Level 5 (Band 3) is left out.** Its set repeats a Pike Jump,
  yet the voluntary "may be a repeat of the set" and needs 10 different
  skills, "with rotation between 270° and 450° per skill". The organiser
  says it's an old routine that can go (2026-10-08).

# Not built in

- **Synchro** (Levels 1–3 pairing Novice/Intermediate, Intervanced/Advanced,
  Elite/Elite-Pro) is set up as [paired synchro levels](../features/competition-entries.md#events-and-synchro-adr-0005-step-2)
  on a competition, not as requirements.
- **DMT** (Levels 1–6) and **tumbling** (Levels 1–4): the app checks
  trampoline only. Their levels are names on a competition.

[^isto-2025]: ISTO 2025 Routines (updated), the committee's routines document
