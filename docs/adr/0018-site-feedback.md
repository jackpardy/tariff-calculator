# ADR 0018 — Feedback about the app goes to the maintainer

- **Status:** Accepted; built. A first version went to organisers (2026-10-15,
  migration 40); it moved to the maintainer with a picture and a copy of the
  page (2026-10-20 and 2026-10-21, migration 47); it was made safe, with the
  90-day deletion and the limits, on 2026-11-13
- **Date:** 2026-10-20, with the owner's decisions of 2026-10-21 (Full for
  the dry run) and 2026-11-13 (what's private, 90 days, the limits)
- **Deciders:** jackpardy (solo maintainer)
- **Amends:** ADR 0004 Decision 6 (personal data: what feedback keeps, and
  when it's deleted)

## Context

The dry run (late 2026) gives people only their link and a task sheet, no
walkthrough, to see whether they manage on their own. People who are stuck
on a phone rarely write down what they did. The first feedback button put the
words on the competition organiser's own Feedback page, but what confused
someone is almost always about the app, not the competition, and the
organiser can't fix the app. Messages for the organiser are a different thing
([ADR 0019](0019-messages-and-late-requests.md)).

Feedback keeps personal data of its own: a picture of someone's screen shows
whatever was on it, and a copy of the page holds every name on it. Under 18s
use the app, but the dry run's testers are adults who have been told. Before
real entries open that has to be reconsidered.

## Decision

1. **Feedback goes to the maintainer, not the organiser.** A page for the
   maintainer lists it (`/site/{token}`, opened by a link made on the server
   with `tariffCalculator maintainer-link`; only its hash is kept, and making
   a new one stops the last). The first version's organiser page, menu tab and
   Settings switch are gone, and what had been sent moved to the maintainer
   (migration 47). With `FEEDBACK_EMAIL` set and email set up, each piece is
   also emailed there: the words, the role, the page, the time and the
   reply-to as `Reply-To`; never a link, a picture or a copy.

2. **A tab on every page.** A **Feedback** tab at the right edge, on every page
   drawn with the app's layout: the calculator's pages and every competition
   and club page, including the pages before a link. Not the venue screen,
   printouts or downloads. It opens a short form: what happened (required, up
   to 2,000 characters) and a name or email for a reply (optional, up to 200,
   never confirmed). It says, on the form, that it doesn't go to the
   organiser, and points to **Tell the organiser** where there is one.

3. **What's kept.** The words; the reply-to; the sender's role, worked out
   from the link the form is posted under and never from the form ("Member",
   "Coach", "Organiser", "Helper link: Chairs", "Mary (Treasurer), UCD"...);
   the page as its **route pattern** (never its path, which holds a secret
   link); the competition and its name where the sender can see one; the
   browser's details (window size, user agent, language, app version, the page
   errors and failed requests without addresses, the last pages opened as
   patterns); and the time.

4. **Full or Light, a setting.** The maintainer chooses on their page, kept in
   the database so it takes effect at once. **Full** (to start with, for the
   demo and the dry run, where everyone is an adult and has been told): a
   picture of the screen and a copy of the page go with each piece, the form
   saying so. **Light** (before real entries open): no copy of the page ever;
   a picture only if the sender ticks for it and sees it first. A copy is
   drawn and taken in the browser with the vendored modern-screenshot, loaded
   only as the form opens. The picture and the copy leave out `<noscript>`.

5. **What's private is left out.** Anything marked `data-private` (every link
   box, a notification email, the "your other link" box) is blurred in the
   picture in both modes and taken out of the copy. The server, which doesn't
   trust the browser, replaces any secret link's token with `{token}` in the
   copy and the details, and takes a link's text out of attributes.

6. **The copy is cleaned and shown in a cage.** It's cleaned once as it
   arrives (no scripts, frames, forms, handlers, remote styles or pictures,
   every link going nowhere), and shown in a sandboxed frame under a policy
   that runs nothing and loads nothing from elsewhere.

7. **Kept 90 days.** The pictures and copies, and the feedback itself with its
   reply-to, are deleted 90 days after it's sent, by the same job that
   deletes expired competitions. The maintainer can delete a piece (asking
   first) or all pictures and copies at once. Past 500 MB of pictures and
   copies, or with the disk nearly full, pictures and copies are left off and
   the words are still kept. The form says "What you send, and any name or
   email you leave, is kept for 90 days, then deleted" and links to What's
   kept ([ADR 0004](0004-server-storage-secret-links.md) Decision 6).

8. **Limits.** The app takes 300 pieces a day in all. An address can send 30
   an hour from pages without a link and 200 an hour from pages under a link
   (a hall's wifi shares one address on a competition day), and one link
   takes 20 a day from anyone. A picture is up to 2 MB (JPEG, PNG or WebP
   only), a copy 3 MB as sent and 12 MB unzipped. Refusals say so, and aren't
   counted in the 300.

9. **The sandbox has its own.** It posts to its own maintainer page, and emails
   nothing ([ADR 0016](0016-sandbox.md)).

## Consequences

- The maintainer sees what testers saw, which is what makes the dry run
  useful.
- A Full picture can show other people's names and entries on the page, so
  Full is for adults who have been told. It must be switched to Light before
  real entries open (a step in the deploy notes).
- An emailed copy of the words isn't deleted by the app and outlives the 90
  days. What's kept says so.
- A link on a page that isn't marked `data-private` and isn't a token in a
  known path stays in the copy. A new field that shows a link must be marked.
- Feedback is limited by address and by link; a determined person can still
  send 300 a day, which fills the day's allowance.
- The picture library draws the page's text, so anything it draws that the
  browser doesn't show (a `<noscript>`'s contents, which it printed as raw
  tags on the club's Members page) has to be filtered out of the picture and
  the copy (2026-11-13).
