# ADR 0013 — Requirements kept with the person

- **Status:** Proposed
- **Date:** 2026-10-15
- **Deciders:** jackpardy (solo maintainer)
- **Amends:** ADR 0003 (requirements are kept in the browser) and ADR 0004
  Decision 5 (what the server keeps)

## Context

A coach's or gymnast's own requirements, set routines and levels live only
in the browser's `localStorage` (`SetStore` and `LevelStore` in
`static/js/sets.js`). That was the whole of storage when ADR 0003 was
written. Since then:

- **They're easily lost.** Safari on an iPhone clears a site's script-written
  storage after 7 days without a visit, unless the site is on the home
  screen. Clearing site data, a new phone, or another browser loses them
  too. A coach who wrote their competition's levels in September may open
  the app in February to find nothing.
- **They're on one device.** A coach writing requirements on a laptop can't
  check a gymnast's routine against them on their phone at the gym without
  a share link.
- **People now have a page.** ADR 0010 gave everyone one personal link
  (member, coach, entering on their own), and ADR 0011 keeps a member's
  routines on theirs, with **Save to my page** from the routine builder.
  Club levels are already kept with the club (ADR 0011 Decision 5), written
  on the club's page.

The roadmap's review of the requirements workflow (2026-10-15) recommends
keeping requirements and levels with the person, and building it alongside
the club's level editor (built the same day).

## Decision

1. **A person keeps their requirements on their page**: requirement sets
   (set routines among them, being sets) and levels, each as the JSON the
   browser keeps today (ADR 0003 Decision 2), with a name and when it was
   last changed. At most 50 of each a person, each at most 64 KB. They
   belong to the person (ADR 0010's person, so a member who also coaches
   has one list, whichever link they open), and are deleted with the person.
   Only the person sees them. Sharing stays by link, as now.
2. **Getting them there.** The Requirements page offers **Save to my page**
   on each of your requirements and levels when a personal link is saved in
   this browser (as the routine builder does for routines), posting it to
   that link; saving the same one again (the browser's id for it) changes
   it. The person's page has a **Requirements** section listing them, with
   **Add from this browser** to add those saved here in one go, and
   **Remove**. Nothing is copied without the person asking.
3. **Using them.** Where a personal link is saved in the browser, the
   Requirements page and the routine builder's **Check against** read that
   person's requirements from the server too, listed first as **On your
   page**, and keep a copy in the browser for when there's no signal.
   Editing one opens it in the editor as now; **Save** saves it back to the
   page as well as the browser.
4. **The browser stays as it is** for anyone without a personal link: the
   calculator works with no account and no link, as it always has. The
   browser's copy is a convenience; the page's is the one kept.

## Alternatives considered

- **Leave them in the browser** (status quo): simplest, but loses work on
  iPhones within a week of not visiting, which is how most gymnasts use it.
- **Export and import files** (already possible): nobody does it until it's
  too late.
- **Keep them with the club** rather than the person: covers club levels
  (done) but not a gymnast entering on their own, nor a coach's own
  requirements that aren't the club's.
- **Accounts**: ruled out by ADR 0004 (secret links, no accounts).

## Consequences

- A new table (the person's requirements), with a migration, and routes
  under each personal link to list, save and remove them, and to read them
  as JSON for the Requirements page and the builder.
- `sets.js` merges the page's requirements with the browser's, by id, the
  page's winning when both changed (it's the one kept).
- Personal data stays names and clubs: requirements describe skills, not
  people.
- The club's level editor (ADR 0011 Decision 5, built 2026-10-15) is
  unaffected: a club's levels are the club's, not a person's.
