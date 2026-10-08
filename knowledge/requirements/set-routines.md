---
type: Feature
title: Set routines
description: Compulsory routines, represented as requirements with a single sequence rule, which can be loaded straight into the builder and are scored without difficulty or the repeat rule.
resource: https://github.com/jackpardy/tariff-calculator/blob/master/requirements/requirements.go
tags: [requirements, set-routine, compulsory]
generated: { by: claude-code/cli, at: 2026-10-04T17:45:00Z }
---

# Model

A set (compulsory) routine is a list of [requirements](framework.md) whose
rules include a `sequence` ([rule types](rule-types.md)): one matcher per
element, in order, with exactly that many elements. Built-in set routines also
set `no_difficulty` and `repeats_allowed`, since set routines are usually
scored without difficulty and may repeat elements. Their descriptions note that
"any deviation is an interruption".

`Builtin.IsSetRoutine` reports whether a built-in has a sequence. Set routines
aren't requirements a routine is checked against: in the builder they're a
**starting point** ("Start from a set..."), and a [level](levels.md) shows them
as prescribed. The requirements page lists them apart, by source.

Where a level lets the gymnast choose between set routines (e.g. BUCS L3–L7
"option 1" and "option 2"), each option is its own entry, and the
[level](levels.md) offers both for its first exercise.

# Loading one

`requirements.SetRoutine(set)` turns the sequence into real skills using
`Matcher.Example()` (the simplest skill each matcher allows), named and priced.
The page asks for it with `POST /set-routine`, which returns the skills (and
whether a posted routine already **matches** them). "Start from a set..." puts
them in the routine on screen if it's empty, otherwise in a new routine named
after the set; a level's set tab is rendered from them read-only
(`/routine` with `prescribed=1`).

# Writing one

A set routine is built, not written: the whole routine is prescribed, so a
coach builds it like any routine in the Routine Builder and chooses **More →
Save as a set routine** (or, on the requirements page, **+ New set routine**
from a routine already built). `POST /requirements/set-routine` turns the
skills into requirements with `requirements.SetRoutineFrom`: one exact matcher
per skill (`MatcherFor`: rotation, twist by phase through its FIG notation,
direction, shape where it matters, take-off and landing, labelled with its
name), no difficulty and repeats allowed. They load back as the same skills
(`Matcher.Example` follows the FIG notation's twist phases). The routine
remembers the set routine it was saved as (`setRoutine`), so saving again can
update it. "Your set routines" on the requirements page lists them by element,
with **Edit in the Routine Builder** (`/?setRoutine=<id>`); a built-in one can
be copied there (`/?fromSet=<ref>`) to make one's own.

# Built-in set routines

| Source | Set routines |
|---|---|
| [ISTO](isto.md) | Novice–Advanced first exercises (Set A or B), Disability L1–L5. These count difficulty. |
| [BUCS](bucs.md) | L3–L7 and Disability L1–L2 first exercises, two options each |
| [BG club & regional](bg-regional.md) | Club L1–L3 (both exercises), Regional L1–L3 first exercises |
