---
type: Glossary
title: Glossary
description: Trampolining and app terms as this project uses them, including the rule that requirement lists are called "requirements", never "sets".
tags: [glossary, wording]
generated: { by: claude-code/cli, at: 2026-10-04T17:45:00Z }
---

# Wording rule

In trampolining, a **set** is a set (compulsory) routine. So in the app a list
of competition rules is always called **requirements**, never a "set" or
"requirement set". The code still uses `requirements.Set` and "set" in
identifiers. That's fine internally, but it should not leak into text users
see. See [requirements framework](requirements/framework.md).

# Terms

| Term | Meaning |
|---|---|
| **Skill** / **element** | One move between two contacts with the bed. The app says "skill"; rule books say "element". |
| **Routine** / **exercise** | Ten elements performed one after another (CoP §4.1). See [routine validation](domain/routine-validation.md). |
| **Tariff** / **difficulty** | The difficulty value of a skill or routine under the Code of Points §17. See [tariff](domain/tariff.md). |
| **Rotation** | Somersault rotation, in quarter somersaults (90°). 4 is a single somersault, 8 a double, 16 a quadruple. |
| **Twist** | Twisting, in half twists (180°). 2 is a full twist. |
| **Twist phase** | One somersault of a multiple somersault, which has its own twist count, e.g. a Full Full is twists `2 2`. See [skill model](domain/skill-model.md). |
| **Shape** | Body shape: tuck, pike, straight, or straddle (jumps only). Only part of a skill's identity where the Code of Points says so. |
| **Take-off / landing position** | Feet, seat, front or back. |
| **FIG notation** | The Code of Points shorthand, e.g. `(8 2 3 /)`: rotation, twist per phase, shape. |
| **Repeat** / **duplicate** | The same element performed again; its difficulty counts once (CoP §14). |
| **Interruption** | A straight jump or impossible landing mid-routine; the routine stops counting there (CoP §15). |
| **Set routine** | A compulsory routine: a fixed sequence of elements. See [set routines](requirements/set-routines.md). |
| **Requirements** | A competition level's rules for a routine (required elements, limits, difficulty bounds), or a set routine. See [requirements framework](requirements/framework.md). |
| **Special requirements** | Elements a first exercise must include, each met by a different element and starred (*) on the competition card. |
| **Checks** | The routine's own choice of what is scored: difficulty on or off, repeats flagged or not, how many elements score. |
| **Tariff sheet** / **competition card** | The printed card handed to judges. See [tariff sheet](features/tariff-sheet.md). |
| **CoP** | The FIG Trampoline Code of Points 2025–2028. |
| **FIG** | Fédération Internationale de Gymnastique. |
| **BG** | British Gymnastics. |
| **GI** | Gymnastics Ireland. |
| **BUCS** | British Universities & Colleges Sport (student championships). |
| **ISTO** | Irish Student Trampoline Open, the app's main audience. |
| **AG1/AG2/AG3** | FIG age groups 11–12, 13–14 and 17–21; Junior is 15–16. |
