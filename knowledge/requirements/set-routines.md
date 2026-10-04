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

`Builtin.IsSetRoutine` reports whether a built-in has a sequence, and the
pickers list set routines **apart from** other requirements, grouped by source,
with shorter names.

Where a level lets the gymnast choose between set routines (e.g. BUCS L3–L7
"option 1" and "option 2"), each option is its own entry.

# Loading one

`requirements.SetRoutine(set)` turns the sequence into real skills using
`Matcher.Example()` (the simplest skill each matcher allows), named and priced.
The page asks for it with `POST /set-routine`, which returns the skills and
whether the current routine already **matches** them. The coach then chooses
between a new routine and replacing the current one.

# Built-in set routines

| Source | Set routines |
|---|---|
| [BUCS](bucs.md) | L3–L7 and Disability L1–L2 first exercises, two options each |
| [BG club & regional](bg-regional.md) | Club L1–L3 (both exercises), Regional L1–L3 first exercises |
