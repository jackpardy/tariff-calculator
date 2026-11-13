# ADR 0019 — Messages to the organiser and late requests

- **Status:** Accepted; built. Messages and their threads 2026-10-21
  (migrations 48 and 49); who may read them fixed 2026-11-11; late requests
  with replies and sign-off, and who sends, 2026-11-11 (migrations 54 and 55)
- **Date:** 2026-10-21, with decisions of 2026-11-11
- **Deciders:** jackpardy (solo maintainer)
- **Amends:** ADR 0009 (the table of concerns, which helpers flag, now also
  holds messages from the people at a competition, shown only to some links);
  ADR 0004 Decision 6 (what's kept about a person)

## Context

People at a competition need to tell the organiser things: a question, a
gymnast who's ill, an entry that's wrong. Until now that was a group chat or
an email, and the organiser had no record and no way to answer inside the app.
A button for feedback about the app (ADR 0018) had been used for it, and
confused both.

Once changes (and later new entries) close, clubs and gymnasts still need to
ask for one: a different level, new routines, or a late entry. The organiser
decides each, can charge a fee, and wants to know whether a coach has signed it
off. Clubs differ in whether members may send things themselves or the comp
sec does it for them.

## Decision

1. **Tell the organiser is its own thing, apart from feedback.** A one-line row
   with a **Write** button at the foot of a person's or club's page for a
   competition: a member's page and My competition; the comp sec's Entries; a
   coach's Entries for each competition; an individual's page and My
   competition. Not on the club link's page (whose sender isn't known) nor any
   admin page. The form says who it's from, worked out from the link and never
   typed ("Ann Ryan, a member of UCD", "Club admin, UCD", "Mary (Treasurer),
   UCD", "Bea Byrne, entered on their own"), and ends by pointing to the
   **Feedback** tab for anything wrong with the app.

2. **What a message holds, and its limits.** The words (up to 1,000
   characters), who sent it (role, name and club's name, kept as they were when
   it was sent) and what it's about: any number of the sender's entries the
   competition has, or the competition generally. A club's messages are the
   club's: every club link that can talk to the organiser sees and adds to
   them, each post saying who of the club. A sender can send 10 a competition
   in 24 hours, and a competition holds 2,000. The organiser can switch
   messages off on Settings; those sent stay.

3. **Messages share the concerns table, and only some links see them.**
   A message is a row in `concerns` (ADR 0009's table for what helpers flag),
   marked with its sender (migration 48). Helpers' links see the concerns
   helpers flagged and never the messages: only a link that may resolve
   concerns (the organiser's and the co-organisers') reads a message, its
   replies, the organiser's private note on resolving it and any count of it
   (`concernViews`; fixed 2026-11-11, after an audit found helpers reading them).

4. **A thread both ways.** The organiser can mark a message **Received**,
   **Reply** (words the sender sees on their page) and **Resolve** (with an
   optional reply and a private note). The sender sees their messages, each
   as **Sent**, **Received** or **Resolved** with its thread, and can **Add to
   this** (50 additions a thread); more from them reopens a resolved one. Each
   step is in the competition's history, and the sender is told by push or
   email if they asked to hear about changes on that page.

5. **Late requests are asks, with a thread of their own.** Once changes close
   the organiser ticks which kinds can be asked for, each with a fee charged
   only if accepted: a **late entry** (allowed unless turned off), a **level
   change** or a **routine change**. Asking for a late entry makes the
   competition's copy marked `asked`, left out of entries, counts, fees and the
   timetable until decided; asking for a change leaves the entry as it is until
   decided. Each request carries a thread (a `concerns` row marked `late`,
   kept apart from messages and flags; migration 54): the organiser can reply
   without deciding, and the asker answers on their page. Accepting replaces
   the entry (or brings it in, last on a full level's waiting list) and puts
   the fee on the invoice; turning down needs a reason the asker sees; the same
   person can ask again.

6. **Sign-off status goes with the request, and can hold back Accept.** Every
   request reaches the organiser at once, showing whether a coach has signed it
   off (a coach signs under "Late requests to sign off"; an individual's coach
   through their link). The organiser can tick that a request can't be
   accepted until a coach has signed it; Accept is then unavailable, with the
   reason, and a posted accept is refused.

7. **Who sends: the club chooses, the organiser can override.** A club's
   Settings, **Who sends**, has four ticks, all off until turned on (migration
   55): members can send their own entries, send changes to their entries,
   ask for late changes, and ask to enter late. Where a tick is off, a
   member's request is kept as theirs, "Not sent yet", out of the organiser's
   list, and goes when the comp sec sends it with **Send new and changed**. The
   comp sec asking for a member sends at once. What goes because of a tick is
   in the club's history under the member's name, "(sent themselves)". The
   organiser can take only what clubs send, for their competition, which
   overrides all four.

8. **What's kept, and for how long.** Messages, replies, notes, late requests and
   their threads, with the sender's name and club, are kept with the
   competition and deleted with it, 120 days after it begins ([ADR
   0004](0004-server-storage-secret-links.md) Decision 6). They're not in a
   kept set-up. What's kept says who sees them.

## Consequences

- The organiser has one place to see what people have said and asked, with an
  answer the asker can see, instead of a chat.
- Messages sit in a table helpers also use, so every read of `concerns` must
  leave messages out unless the link may resolve. A new place that shows
  concerns has to do the same.
- A message holds a name and sometimes health or other private words typed
  by the sender. Only the organiser and co-organisers read it, and it goes with
  the competition, but a backup may hold it a little longer.
- Clubs can choose to send everything themselves, which leaves members
  waiting on their comp sec; the page says "Not sent yet" so they know.
- A late entry is already a competition row, so an asked-for entry mustn't
  count anywhere an entry does; every counter has to leave `asked` out.
