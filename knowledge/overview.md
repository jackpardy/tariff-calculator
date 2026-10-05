---
type: Overview
title: Trampoline tariff calculator
description: A phone-first web app that works out trampoline difficulty (tariff), checks routines against the Code of Points and competition requirements, and prints competition cards.
resource: https://github.com/jackpardy/tariff-calculator
tags: [overview, trampoline, product]
generated: { by: claude-code/cli, at: 2026-10-05T14:20:00Z }
---

# What it is

A web app for building trampoline routines and working out their difficulty
(tariff) under the FIG Code of Points 2025–2028. A coach or gymnast adds skills,
sees each one's FIG notation and tariff, and gets the routine checked: repeats,
bad transitions, interruptions, the tenth skill landing on feet, and, if a
competition is chosen, that competition's [requirements](requirements/framework.md).
A [level](requirements/levels.md) pairs a competition's two exercises (for
example a choice of set routines, then a voluntary), so both are checked
together. Routines can be shared by link or QR code, shown full screen, and
printed as a competition card.

For users, start with the [features](guide/features.md) and the
[user guide](guide/user-guide.md).

Live at <https://tariff.pardy.ie>, on the self-hosted server (see
[deploy](operations/deploy.md)). The name may change later; it stays for now.

# Who uses it

Mostly newer gymnasts and coaches in **Irish student trampoline** (ISTO), plus
**BUCS** (British student) competitors, using phones at the gym. Some also
compete under British Gymnastics, Gymnastics Ireland and FIG age-group rules.
Wording in the app is plain and avoids jargon where it can (see the
[glossary](glossary.md)).

# How the pieces fit

| Layer | Where | Concept |
|---|---|---|
| Skill model and tariff | `skills/` | [Skill model](domain/skill-model.md), [Tariff](domain/tariff.md) |
| Routine rules | `skills/` (`ValidateRoutine`) | [Routine validation](domain/routine-validation.md) |
| Choosing skills | `catalog/` | [Skill catalog](domain/skill-catalog.md) |
| Competition rules | `requirements/` | [Requirements framework](requirements/framework.md), [Levels](requirements/levels.md) |
| Pages and fragments | `views/` (templ), `main.go` | [Rendering](architecture/rendering.md), [HTTP routes](architecture/http-routes.md), [View screen](features/view.md) |
| Browser glue | `static/js/` | [Stack](architecture/stack.md) |

All domain rules live in Go. The server renders every page and fragment as
HTML, and the browser holds only the saved routines (in `localStorage`). There
is no database and there are no accounts yet. Each request carries the routine
it needs.

# Decisions

The architecture is recorded in four ADRs:

- [ADR 0001](../docs/adr/0001-architecture.md): Go full stack with the `skills`
  package as the single source of truth (accepted; its frontend parts are
  amended by 0002).
- [ADR 0002](../docs/adr/0002-server-rendered-frontend.md): the server renders all UI
  with templ and htmx (accepted, fully implemented).
- [ADR 0003](../docs/adr/0003-requirements-framework.md): the requirements
  framework (proposed; implemented).
- [ADR 0004](../docs/adr/0004-server-storage-secret-links.md): server storage
  with secret links and no accounts, for competition card collection
  (accepted; not built yet).

# Related

- The self-hosting setup that runs this app:
  [server knowledge bundle](https://github.com/jackpardy/server/blob/main/knowledge/index.md)
  (private repository).
