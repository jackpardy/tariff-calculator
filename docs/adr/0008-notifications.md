# ADR 0008 — Notifications, by opting in

- **Status:** Accepted; built 2026-10-08 (email waiting on a provider on
  the server)
- **Date:** 2026-10-08
- **Deciders:** jackpardy (solo maintainer)
- **Amends:** ADR 0004 Decision 6 (personal data: an email can be kept, to
  notify)

## Context

Once a competition's timetable is published, the organiser keeps changing
it: a flight moves, an official leaves, the rota is redrawn. With drafts
(roadmap 2026-10-07, built 2026-10-08) attendees only see a change once
it's published, but they only find out by looking. Card checks are the
same: a gymnast whose card has a problem hears nothing until they open
their page.

Decided on the roadmap (2026-10-07):

- **Who:** members, individuals, club comp secs and coaches, each opting
  in. A comp sec hears about the club's members, a coach about the
  gymnasts they coach.
- **What:** a flight they're in moves (time, area, day or warm-up), their
  officiating duties change, or their card is checked or found to have a
  problem. Running order changes alone don't notify.
- **When:** timetable and duty changes go out when the organiser
  publishes. After a change goes live there's a **grace period**; anything
  else in it joins the same notification, and each compares what was live
  before the first change with what's live at the end, so a change made
  and undone tells no one. **Notify now** skips the wait, and the organiser
  can turn the wait off for the competition's days.
- **How many:** one notification per person told, however many people or
  events changed (a club gets one email for all its members), summarising
  each change (who, which event, was and now) and linking to their page.
- **How:** phone push where it works (on iPhone, once the site is on the
  home screen) and email, which clubs always have. An email is given with a
  tick box, "I'm 18 or over, or this is a parent's email"; anyone younger
  can still use push. Emails are kept only to notify and deleted with the
  competition. Which service sends email is left to the build.

ADR 0004 Decision 6 keeps no emails or contact details, and the app sends
nothing today: no email, no push, no background work beyond deleting
expired competitions.

## Decision

1. **Opting in is per competition.** On a competition's page for them (a
   member's competition, an individual's entry, the club's competition for
   the comp sec, the coach's competition), each person can turn on
   notifications by email, by push on this phone, or both, and choose what
   about: **timetable** (their flights), **duties** (their officiating) and
   **cards** (checked, or a problem). A comp sec's and a coach's cover
   every member they see. Opting in again for the next competition is one
   tap; nothing carries over, so every email goes with its competition
   (deleted with it, ADR 0004 Decision 6) and nobody is told about a
   competition they didn't ask about.

2. **Emails are confirmed.** An email is sent a link to confirm it before
   anything else is sent to it, so a mistyped or someone else's address
   gets one email and no more. Every email has a link that turns its
   notifications off at once, without the person's page. The form says what
   is kept and when it's deleted, with the tick box "I'm 18 or over, or this
   is a parent's email", required for email.

3. **Push stores a phone's push address, nothing personal.** Standard web
   push (the Push API, with VAPID keys the app makes itself on first use and
   keeps in the database): the browser gives an address and keys, kept with
   the subscription and deleted with the competition, or when the push
   service says it's gone. A service worker shows each notification and
   opens its link. iPhones need the site added to the home screen first
   (iOS 16.4+), so the app gets a web manifest; the page says so where
   push isn't available.

4. **Changes are compared, not logged.** Whenever something that can notify
   changes (publishing the timetable or officials, marking a card checked,
   changing its note) and no notification is waiting, the competition
   records what was live just before as its **baseline**, and a time it's
   **due**: now plus the grace period. When it's due, a worker compares the
   baseline with what's live now, per person: each entry's placement
   (event and flight, area, day and warm-up time), each person's duties,
   and each card's checked state and note. Only differences notify: a change
   undone in the grace period, or a running order change, tells no one. The
   worker then clears the baseline. It runs every half minute in the
   server, as deleting expired competitions does; one waiting due when the
   server restarts goes out when it's back.

5. **The grace period is ten minutes, organiser's choice.** Ten minutes by
   default, long enough to publish, spot a mistake and publish again.
   **Notify now** on the dashboard and timetable sends what's waiting at
   once. **No wait on the competition's days** (off by default) sends each
   change as soon as it's published on the competition's days, when news
   can't wait. There are no quiet hours: the organiser publishes, so they
   choose when.

6. **Grouped per person told, and summarised.** Each change belongs to the
   gymnast or official it's about. A notification goes to each subscriber
   whose people changed and who asked for that kind of change: a member and
   an individual for themselves (as competitor and official), a comp sec
   for every member of the club, a coach for the gymnasts they coach. Each
   gets one email and one push for the whole batch: the email lists every
   change ("Ann Ryan · BUCS L5 Women: now Saturday 10:20, Panel 2 (was
   Saturday 09:40, Panel 1)"), the push says how many and whose ("3 changes
   for UCD at ISTO 2027"), and both link to the person's page for the
   competition.

7. **Email goes by SMTP, configured on the server.** `SMTP_HOST`,
   `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD` and `MAIL_FROM` set the
   sender: any provider that offers SMTP with STARTTLS (Amazon SES's
   Ireland region is the leaning).
   Without them, email isn't offered and only push is; nothing else
   changes. Push needs no configuration. Both use Go's standard library
   only (`net/smtp`; ECDH, HKDF and AES-GCM for push's encryption), so no
   new dependencies.

## Consequences

- The app holds emails for the first time, only for confirmed opt-ins, and
  deletes them with the competition; push addresses aren't personal data
  but go the same way. The privacy note on each page says so.
- Changes reach people without them checking, which makes publishing a
  heavier act: the dashboard and timetable say how many people are waiting
  to be told, and when.
- Email depends on an outside provider and its deliverability (SPF and DKIM
  for `tariff.pardy.ie`), to set up on the server; until then only push
  works.
- A person in two roles (a member who is also a coach) can be told twice,
  once as each; each notification is about what that page shows.

## Alternatives considered

- **One subscription for everything a club or member does.** Fewer taps,
  but an email outliving its competition breaks "deleted with the
  competition", and a person would hear about competitions they never
  looked at. Per competition instead (Decision 1).
- **Logging each change and sending the log.** Simpler to write, but a
  change undone still tells people, and a flight moved twice says so twice.
  Comparing the baseline with the end (Decision 4) does what was decided.
- **A push or email SDK.** Web push and SMTP are small enough to write on
  the standard library, keeping dependencies as they are (Decision 7).
- **Quiet hours overnight.** Left out: the organiser chooses when to
  publish; can come later if wanted.

## Build

1. Storage and the email path: subscriptions per competition with
   confirmation and turning off, the baseline and due time, the worker,
   the comparison, grouped emails, the opt-in form on the four pages, and
   Notify now and No wait on the competition's days.
2. Push: the manifest, service worker, VAPID keys and web push sending,
   and the push option on the same forms.
