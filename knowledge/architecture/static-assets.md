---
type: Architecture
title: Static assets and versioning
description: CSS and JS are embedded in the binary and served at content-hashed URLs that are cached forever, and a version header tells open pages when a deploy has happened.
resource: https://github.com/jackpardy/tariff-calculator/blob/master/static/static.go
tags: [architecture, caching, assets, deploy]
generated: { by: claude-code/cli, at: 2026-10-04T17:45:00Z }
---

# Embedded, hashed assets

`static/css` and `static/js` are embedded with `embed.FS`, so the binary is the
whole app and has no working-directory dependency. Templates link to assets
through `static.URL("js/app.js")`, which appends `?v=<first 12 hex of SHA-256>`.
An unknown path panics, so a bad link fails the tests.

| Request | Cache-Control |
|---|---|
| Current version (`?v=` matches) | `public, max-age=31536000, immutable` |
| Anything else | `no-cache`, revalidated cheaply with the hash as `ETag` |

Vendored libraries (htmx, Alpine, SortableJS, Bulma) live alongside the app's
own files. Nothing loads from a CDN.

# Update notice

`static.Version` is the first 12 hex digits of the SHA-256 of the running
executable, so any change to code, templates or assets changes it. Each page
carries it in `<meta name="app-version">`, and every response carries it in the
`X-App-Version` header. `static/js/version.js` compares the two on htmx
requests and the page's own `fetch` calls. When they differ, it shows "The app
has been updated" with a Refresh button. Without this, a page left open across
a deploy would have scripts that no longer match the server.

Related: [deploy](../operations/deploy.md).
