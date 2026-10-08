# Architecture

* [Technology stack](stack.md) - Go 1.25 standard-library server, templ components, htmx 2, a little Alpine.js, SortableJS and Bulma, all vendored and embedded in one static binary, with SQLite on the way for competition entries.
* [Server-rendered UI](rendering.md) - Every page and fragment is HTML rendered by the server from templ components; the browser posts the routine on each change and swaps in the result, and domain rules exist only in Go.
* [HTTP routes](http-routes.md) - Every route the server exposes, what it takes and what it returns, along with the limits applied to all requests.
* [Static assets and versioning](static-assets.md) - CSS and JS are embedded in the binary and served at content-hashed URLs that are cached forever, and a version header tells open pages when a deploy has happened.
* [Testing and checks](testing.md) - What the Go tests cover (the 139 CoP examples, the engine, requirements, catalog and rendered HTML) and the checks to run before every commit.

# Decision records

* [ADR 0001](../../docs/adr/0001-architecture.md) - Architecture for growing the tool into a product (accepted; partly amended by 0002).
* [ADR 0002](../../docs/adr/0002-server-rendered-frontend.md) - Server-rendered frontend with templ and htmx (accepted, done).
* [ADR 0003](../../docs/adr/0003-requirements-framework.md) - Requirements framework (proposed, implemented).
* [ADR 0004](../../docs/adr/0004-server-storage-secret-links.md) - Server storage with secret links, no accounts (accepted, built and live: competition entries, clubs, video proof, coach sign-off).
* [ADR 0005](../../docs/adr/0005-multi-discipline-timetabling.md) - Timetabling multi-discipline competitions for ISTO: events across trampoline, synchro, tumbling and DMT, people who compete and judge, the venue's end time, blocked time, organiser rules and simulation (accepted; being built).
* [ADR 0006](../../docs/adr/0006-one-panel-per-event.md) - One panel for a whole event: an event's flights back to back on one area, its panel filled once for all of them, flights numbered in the order they run, and rarely a split by routine (accepted; amends 0005; runs and one panel per run built).
* [ADR 0007](../../docs/adr/0007-approved-coaches.md) - Approved coaches: a built-in list of coaching qualifications, coaches' certificates kept with the club, clubs saying who signs off and sending coaches, organisers approving them, and only approved coaches' sign-offs counting (accepted, built; amends 0004).
