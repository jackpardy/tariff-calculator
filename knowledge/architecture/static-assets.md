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

Vendored libraries (htmx, Alpine, SortableJS, Bulma, and modern-screenshot
4.7.0, MIT, which draws the picture sent with
[feedback](../features/competition-entries.md#feedback-about-the-app) and is
loaded only when the Feedback form opens) live alongside the app's own files.
Nothing loads from a CDN.

# Compression

The app compresses its own text responses, so it doesn't depend on the web
server in front of it (`web/gzip.go`, wrapped round every route). Pages, JSON,
SVG, CSV, calendar files and the manifest go out gzipped when the request's
`Accept-Encoding` allows it, with `Vary: Accept-Encoding` and no
`Content-Length`; a strong `ETag` is made weak. Left alone: responses under
1 KB, `HEAD`, 204, 304 and partial content, anything already encoded, and
binary types (images, PDFs, uploaded certificates). On the demo the
organiser's On the day page falls from 1.0 MB to 54 KB.

The CSS and JS files are gzipped once, on first request, and kept. The gzipped
copy has its own `ETag` (`"<hash>-gz"`), so a revalidation never mixes the two.
The service worker and push code only see decoded bytes, so they are unaffected.

# Update notice

`static.Version` is the first 12 hex digits of the SHA-256 of the running
executable, so any change to code, templates or assets changes it. Each page
carries it in `<meta name="app-version">`, and every response carries it in the
`X-App-Version` header. `static/js/version.js` compares the two on htmx
requests and the page's own `fetch` calls. When they differ, it shows "The app
has been updated" with a Refresh button. Without this, a page left open across
a deploy would have scripts that no longer match the server.

Related: [deploy](../operations/deploy.md).
