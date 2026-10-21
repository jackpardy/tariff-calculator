# ADR 0015 — Club roles: the club admin makes roles and links for people

- **Status:** Proposed (2026-10-21); not built. Decisions for the owner are
  listed at the end
- **Date:** 2026-10-21
- **Deciders:** jackpardy (solo maintainer)
- **Amends:** ADR 0004 Decision 3 (a club has more than one admin link);
  ADR 0010 (a personal link can open a club role, not only the whole club)

## Context

A club has one admin link (`/clubs/admin/{token}`), the comp sec's. Whoever
holds it can do everything: add and remove members and coaches, enter,
change, withdraw and send entries, read coaches' and judges' certificates,
change the club's settings, replace the link and delete the club. In
practice it's shared: a second comp sec, the treasurer looking at invoices,
a team manager on the day, someone keeping the members list. So everyone
can do everything, and nothing says who did what. Feedback about the app
(`web/feedback.go`) and messages to the organiser (`tellpages.go`)
from it all say "UCD's comp sec", whoever sent them.

Competitions solved the same thing with helper links of fixed kinds (ADR
0009). For clubs the owner asked (2026-10-21) for the top level to be the
**club admin**, who makes and names **roles** and chooses each role's
**permissions**, rather than a fixed list of kinds: clubs are organised
differently, and what a "comp sec" or "team manager" does varies.

## Decision (proposed)

1. **The club's link is the club admin.** The link the club has today
   becomes the club admin's, and keeps doing everything. Only the club
   admin can make, change and remove roles and links, see every link,
   replace the admin link and delete the club. Pages, feedback and messages
   say "club admin" where they now say "comp sec" (the club can call a role
   "Comp sec"; see 3).

2. **Permissions are a fixed list; roles are the club's.** The app defines
   the permissions, each a set of club routes (one table, like `allowed` in
   ADR 0009 Decision 2), and every route under `/clubs/admin/{token}` goes
   through one gate that checks the link's role. Proposed permissions, with
   what each allows on top of seeing the club:

   | Permission | Allows |
   |---|---|
   | *(every role)* **See the club** | Club home, the competitions and each one's entries, the members and coaches lists (names only), timetables and calendars, desk messages; Feedback |
   | **Enter and change entries** | Enter a member, change or withdraw a member's entry, offers to help, late changes (`enter-member`, `members/{member}/competitions/{id}`…, `/offer`, `/late`, `/withdraw`, `add-offer`) |
   | **Send entries** | Send the club's entries and coaches to the organiser (`/competitions/{id}/send`, `/competitions/{id}/coaches`) |
   | **Fees** | Each competition's fees and invoice (`/fees`, `/invoice`) |
   | **Members** | Remove members, give a member a new link, choose a member's coach, members' judging qualifications (`members/{member}/remove`, `/new-link`, `/coach`, `/judging`…) |
   | **Coaches** | Add, remove and re-link coaches, make a member a coach, who signs off, coaches' qualifications (`/coaches`…, `coaches/from-member`) |
   | **Certificates** | Open coaches' and judges' certificate files (personal documents; off unless chosen) |
   | **Talk to the organiser** | Tell the organiser and add to the club's messages; hear about changes by push or email (`/organiser`, `/notify`) |
   | **Club settings** | Coaches see everyone, members' skills open (`coaches-see-all`, `skills-open`) |

   The club admin makes a role by naming it and ticking permissions
   ("Treasurer": Fees; "Team manager": Talk to the organiser). To start
   them off, a new club gets four roles it can change or remove: **Comp
   sec** (all but Club settings and Certificates), **Entries** (Enter and
   change entries, Send entries), **Treasurer** (Fees) and **Team
   manager** (Talk to the organiser). Up to 10 roles a club.

3. **Links are for people, each with a role.** The club admin makes a link
   by giving a person's name and choosing a role. Like every link, it's
   secret, kept as a hash and shown once; **Remove** stops it at once;
   **Change role** moves it to another role. Changing a role's permissions
   changes them for every link with that role at once. A role with links
   can't be removed until they're moved or removed. Up to 30 links a club.
   The link has the same address shape as the club admin's
   (`/clubs/admin/{token}`), so every club page, bookmark and saved link
   works the same; the store finds the club by either hash (`ClubByAdmin`
   becomes a lookup of the club and the link's role).

4. **Pages show only what the role can do.** Like `views.Access` for
   competitions, a `ClubAccess` says what each control may offer: no
   Remove buttons without Members, no Send without Send entries, no
   Settings tab without Club settings. A route the role can't use says
   "This link can't do that. Ask your club admin."

5. **The person's name goes with what they do.** Feedback says "Mary
   (Treasurer), UCD" instead of "Comp sec"; a message to the organiser and
   each thing added to it says "Mary (Treasurer), UCD" to the organiser.
   Messages stay the club's (`club:<id>` as the sender), so every link
   with Talk to the organiser sees and answers the club's messages, each
   post saying who.

6. **The club keeps a history** (as ADR 0009 Decision 3): each change made
   through any club link, when, which link (its person's name and role;
   "Club admin" for the top link) and what ("Withdrew Ann Ryan · BUCS L3").
   The latest 1,000 are kept. The club admin, and roles with Club settings,
   read it on the club's Settings.

7. **One link for several roles still works** (ADR 0010). A personal link
   tied to the club today opens the club admin's pages. A person can also
   paste a club role's link into their own page, which ties that role to
   them (a new tie kind, as competitions' helper links already are), and
   their personal link then opens the club's pages as that role.

## How it's built (for the owner's review, not yet planned in detail)

- Migration: `club_roles (id, club_id, name, permissions JSON, position)`,
  `club_links (id, club_id, role_id, name, token_hash UNIQUE, created_at)`,
  `club_log (id, club_id, at, who, what)`; the four starter roles for clubs
  already made, with no links.
- `web/clubaccess.go`: the permission table, the gate wrapping every
  `/clubs/admin/{token}` route (they're registered in `registerClubs` and
  under the shared roots for late changes, timelines, calendars,
  notifications, feedback and messages), `ClubAccess`, and the history's
  descriptions.
- Club **Settings** gains **Roles** (name, permissions as ticks, Save,
  Remove) and **Links** (name, role, Make link, then shown once; Change
  role, Remove) for the club admin, and **History**.
- Feedback roles (`feedbackRoots`), message senders (`teller`) and the
  dashboard's thread use the link's name and role.
- Tests: every club route against every permission (so a new route can't
  be left out of the table), the starter roles, a role change taking effect at once,
  removing a role with links refused, the names in feedback and messages,
  the history, and the migration.
- Rough size: a little more than ADR 0009 (roles add a page and the
  permission ticks). One PR, then the demo gets a treasurer's and a team
  manager's link.

## Decisions for the owner

1. **The permissions list** (Decision 2): are these the right ones? Should
   **Send entries** be separate from changing them (recommended: yes,
   sending is what the organiser sees), and **Certificates** separate
   (recommended: yes, they're personal documents)?
2. **Starter roles**: these four, other names, or none (the club admin
   makes their own)? Recommended: these four, to show what roles are for.
3. **"Comp sec" to "club admin"** in the app's wording for the top link
   (Decision 1). Recommended: yes, with "Comp sec" as a starter role.
4. **A club history** (Decision 6). Recommended: yes; it's what tells a
   club who withdrew an entry.
5. **What every role sees** (the "See the club" row): is a members list
   with names only fine for any role, e.g. a treasurer? Recommended: yes.
6. **When**: before the dry run (if the dry-run clubs will share their
   link between several people) or after. Recommended: after, unless they
   will.
