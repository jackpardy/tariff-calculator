---
type: Overview
title: Trampoline tariff calculator
description: A phone-first web app that works out trampoline difficulty (tariff), checks routines against the Code of Points and competition requirements, and runs competitions for organisers, clubs, coaches and gymnasts, from entries to the day itself.
resource: https://github.com/jackpardy/tariff-calculator
tags: [overview, trampoline, product]
generated: { by: claude-code/cli, at: 2026-10-05T14:20:00Z }
---

# What it is

A web app for trampoline, in two parts that share one rules engine.

**The routine tools** build routines and work out their difficulty (tariff)
under the FIG Code of Points 2025–2028. A coach or gymnast adds skills, sees
each one's FIG notation and tariff, and gets the routine checked: repeats, bad
transitions, interruptions, the tenth skill landing on feet, and, if a
competition is chosen, that competition's [requirements](requirements/framework.md).
A [level](requirements/levels.md) pairs a competition's two exercises, so both
are checked together. Routines can be shared by link or QR code, shown full
screen, compared and printed as a competition card. Everything here works in
the browser with nothing stored on the server.

**The competition pages** run a competition from entries to the day, for
organisers, clubs' competition secretaries, coaches, members and gymnasts
entering on their own: entries checked against their level, fees and limits,
coach sign-off, officials and judges, the timetable and rota, late requests and
messages, then check-in, who's up and the venue screen on the day. There are
no accounts: each person has a secret link, and a link can open several roles
([What's kept and for how long](https://tariff.pardy.ie/privacy) says what is
stored about people). See [competition entries](features/competition-entries.md)
for the whole of it, and the [sandbox](features/sandbox.md), a copy full of
made-up people to try it in.

For users, start with the [features](guide/features.md), the
[routine builder guide](guide/routine-guide.md) and the
[competitions guide](guide/competition-guide.md).

Live at <https://tariff.pardy.ie>, on the self-hosted server (see
[deploy](operations/deploy.md)). The name may change later; it stays for now.

# Who uses it

Mostly newer gymnasts and coaches in **Irish student trampoline** (ISTO), plus
**BUCS** (British student) competitors, using phones at the gym and at the
venue. Some also compete under British Gymnastics, Gymnastics Ireland and FIG
age-group rules. Wording in the app is plain and avoids jargon where it can
(see the [glossary](glossary.md)).

# How the pieces fit

| Layer | Where | Concept |
|---|---|---|
| Skill model and tariff | `skills/` | [Skill model](domain/skill-model.md), [Tariff](domain/tariff.md) |
| Routine rules | `skills/` (`ValidateRoutine`) | [Routine validation](domain/routine-validation.md) |
| Choosing skills | `catalog/` | [Skill catalog](domain/skill-catalog.md) |
| Competition rules | `requirements/` | [Requirements framework](requirements/framework.md), [Levels](requirements/levels.md) |
| Competitions, clubs, entries | `competitions/`, `store/` (SQLite) | [Competition entries](features/competition-entries.md) |
| Judging, timetable, demo data | `judging/`, `demo/` | [Competition entries](features/competition-entries.md) |
| Pages and fragments | `views/` (templ), `web/` | [Rendering](architecture/rendering.md), [HTTP routes](architecture/http-routes.md), [View screen](features/view.md) |
| Browser glue | `static/js/` | [Stack](architecture/stack.md) |

All domain rules live in Go, and the server renders every page and fragment as
HTML. The routine tools keep saved routines in the browser's `localStorage` and
each request carries the routine it needs. The competition pages keep their
data in one SQLite file, reached by secret links, and delete it on a schedule.

# Decisions

The architecture is recorded in nineteen ADRs, listed with a line each in the
[architecture index](architecture/index.md#decision-records): the Go full
stack and server rendering (0001, 0002), the requirements framework (0003),
storage with secret links and what's kept about people (0004), timetabling
(0005, 0006), coaches, notifications, helpers, roles and clubs (0007 to 0015),
the sandbox (0016) and judges, feedback and messages (0014, 0017 to 0019).

# Related

- The self-hosting setup that runs this app:
  [server knowledge bundle](https://github.com/jackpardy/server/blob/main/knowledge/index.md)
  (private repository).
