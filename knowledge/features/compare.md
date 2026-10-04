---
type: Feature
title: Compare page
description: Shows two saved routines side by side, read-only, with their validation and totals.
resource: https://github.com/jackpardy/tariff-calculator/blob/master/views/compare.templ
tags: [feature, compare]
generated: { by: claude-code/cli, at: 2026-10-04T17:45:00Z }
---

# What it does

`GET /compare` serves the page. Its script (`static/js/compare.js`) fills two
choices from the routines saved in this browser: A is the current routine and B
the next one. Whenever either choice changes, it posts both routines to
`POST /compare`. The server validates each one
([routine validation](../domain/routine-validation.md)) and renders them side by
side.

Requirements aren't applied here. To compare while editing, with requirements
and checks, use the builder's [side by side](routine-builder.md#side-by-side)
view instead.
