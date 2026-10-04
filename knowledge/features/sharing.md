---
type: Feature
title: Sharing by link and QR code
description: Routines and requirements are shared as a compressed link fragment that never reaches the server, with a QR code for sharing in person.
resource: https://github.com/jackpardy/tariff-calculator/blob/master/static/js/share.js
tags: [feature, sharing, qr, privacy]
generated: { by: claude-code/cli, at: 2026-10-04T17:45:00Z }
---

# How it works

Sharing happens entirely in the browser (`static/js/share.js`). The user ticks
routines (on the calculator) or saved requirements (on the requirements page).
The link carries what is shared after `#share=`:

```
https://<host>/#share=z<base64url(deflate-raw(JSON))>
https://<host>/requirements#share=j<base64url(JSON)>   (browsers without CompressionStream)
```

The payload is `{v: 1, routines: [...], sets: [...]}`:

- Routines carry `name`, skills **without** names or tariffs (the server works
  those out again), and `checks`.
- A routine checked against built-in requirements keeps its `builtin:<id>`
  reference. If it uses custom requirements, those travel with it in `sets`, and
  the routine refers to them as `set:<n>`.

Because the data is in the URL fragment, **nothing is sent to or stored on the
server**, and there are no accounts. The only server call is `POST /qr`,
described below.

# Receiving

Opening a link shows "Shared with you" with everything ticked. The user chooses
what to add. Routines are always added on the calculator page, so a requirements
link carrying routines redirects there. Added items get unique names (" (2)"),
and custom requirements identical to ones already saved are reused, not
duplicated. The fragment is then cleared and the page reloads with a
confirmation.

# QR code

`POST /qr` with `text=<link>` returns an SVG QR code (`qrcode.go`, using
`rsc.io/qr`). It uses medium error correction, falling back to low for long
links. If a link is too long even for that, the user is told to share fewer
routines at a time. The response is never cached.

On phones, **Share…** uses the system share sheet (`navigator.share`), and
**Copy link** copies the link. Neither has been tried on a real device yet
([open questions](../open-questions.md)).
