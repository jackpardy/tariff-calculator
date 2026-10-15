# ADR 0010 — One link for a person with several roles

- **Status:** Accepted; built 2026-10-14
- **Date:** 2026-10-14 (Decision 8, links the comp sec makes, added
  2026-10-15)
- **Deciders:** jackpardy (solo maintainer)
- **Amends:** ADR 0004 Decision 3 (secret links: a personal link opens all
  of its person's roles); ADR 0005 Decision 4 (an individual is matched by
  person, not by name)

## Context

Every role has its own secret link (ADR 0004): a member's, a coach's, the
club's admin link (the comp sec's, which several committee members may
share), each individual entry's, and a competition's admin and helper links
(ADR 0009). A member who also coaches, or is the comp sec, juggles several
links, and a member and a coach with the same name aren't connected at all.
A gymnast entering on their own gets a link per entry, and Your competition,
the rota and the desk group their entries by the name typed, so a misspelling
splits one person and two people with one name are merged.

Links are stored only as hashes, so no page can link to another of a
person's links: nothing knows the other token. Person keys (`m:<member>`,
`i:<name>`, ...) are saved in the timetable's seats and rules, desk messages,
scratch warnings cleared and officials' check-ins, so changing how individuals
are keyed must not lose what was saved.

Asked on the roadmap (item 5, 2026-10-13 and 2026-10-14).

## Decision

1. **A person is a set of records with one `person_id`.** Members and
   coaches gain a `person_id` (each their own id at first), and a new
   `individuals` table holds gymnasts entering on their own, each with a
   `person_id`, an optional personal link (hash) and a name; their entries
   point to it (`entries.individual_id`). There's no people table: a person
   is just the id their records share. Joining two people sets one id on
   both's records; undoing gives the record leaving a new id.
2. **Personal links tie both ways; shared links one way.** Pasting another
   personal link (member, coach, individual, or an entry link) joins its
   person. Pasting a shared link (a club's admin link, a competition's admin
   or helper link) adds a row to `shared_ties`: the personal record it was
   added on, the kind, the club, competition or helper link, and the link's
   hash at the time. It's honoured only while that hash is still the link's,
   so replacing or removing the link ends it, and only for that person's
   records, so removing the record ends it. A shared link never finds a
   person: only a personal link's token does.
3. **Other roles open through the personal link: `{personal token}~{id}`.**
   A role opens at its own existing paths with this in place of its token
   (the id of the member, coach, individual or entry; of the club for the
   comp sec's pages; of the competition or helper link for an admin link).
   Every store lookup by token accepts it (`MemberByLink`, `CoachByLink`,
   `ClubByAdmin`, `AdminLink`, `IndividualEntry`, `IndividualByLink`, and the
   functions that change an individual's entry), checking that the id is the
   person's, or a shared link they hold as it is now. So every existing page,
   form and redirect works through a personal link without change, and a
   helper's link opened this way can still do only what it could. Pages
   opened this way never show themselves as a link to keep or share.
4. **Tabs come from the request.** For any page a personal link opens, the
   person's roles are worked out once and kept in the request's context; the
   shared layout shows them as tabs above the page's own menu.
5. **The comp sec makes a member a coach too** by making a coach record that
   is the same person; its own link is made but never shown.
   *Changed 2026-10-15:* if the comp sec later makes that coach a link of
   its own, the coach first becomes a person of its own (Decision 8).
6. **Individuals have one page.** A new individual gets a personal link
   (`/competitions/gymnast/{token}`); their entries have no link of their
   own. Migration 30 makes each existing individual entry an individual of
   its own, reached by its existing entry link, which keeps working (and
   opens the person's page too). Grouping same-named entries into one person
   in the migration would let one holder open another's entries, so the
   person joins their entries themselves, by pasting the other links.
7. **Person keys are kept on the entry.** `entries.person_key` is an
   individual's key in that competition: entries from before keep
   `i:<lower-cased name>`, so every saved key still finds them; new people
   get `i:<individual id>`, and later entries take the key the person
   already has there. When joining leaves a person with two keys in one
   competition, the one entered first stays, the other's entries take it,
   and the saved keys move with them (a JSON-string replace in the saved
   timetables and desk messages, and an update of scratch warnings and
   check-ins), unless someone else of that name from before still has the
   key. Undoing gives the record leaving a new key; what was saved stays with
   the person.
8. **A link the comp sec makes or replaces opens only the club's records**
   (added 2026-10-15, replacing the first rule that a member's or coach's
   replaced link kept their ties). **New link** for a coach makes the coach
   a person of their own; for a member, the member and the club's coach who
   is the same person (one made with Make a coach too, whose page their link
   opens) become a person of their own. Either way the shared links added on
   those records' pages are dropped. The person ties their other roles
   again by pasting their other links.
   *Why:* the comp sec sees every link it makes. Under the first rule it
   could make a member a coach too, make that coach a link, and open
   through it everything the member had tied: an organiser's or helper's
   link, and their roles at other clubs. Replacing the club's admin link
   didn't undo links it had already made. Now a link the comp sec holds
   opens nothing the club's admin link doesn't already reach.

## Alternatives considered

- **Storing the other links' tokens, encrypted under the personal link**, so
  tabs could link straight to `/clubs/admin/{token}`. Replacing a personal
  link would lose its ties, the role's pages couldn't show the person's tabs
  (their shared link has no person), and a member made a coach has no token
  to store.
- **New routes for each role under each personal link** (`/clubs/member/{t}/coach/...`).
  Every page and redirect builds its paths from its token, so each would need
  rewriting for every pair of roles; the `~` form reuses them all.
- **A people table, now.** It holds nothing yet but the id; one can be added
  later from the distinct `person_id`s when a person needs data of their own
  (a date of birth, a BG number).
- **Rewriting every saved individual key at migration** (to `i:<id>`). Keys
  live inside JSON in several tables; keeping the old keys stable needs no
  rewrite, and the rename on joining is rare and contained.
- **Keying a tied member and their own entries alike in a competition.** A
  member who also enters on their own at the same competition still has two
  keys there (`m:` and `i:`); All people joins them by person, but the rota
  and Your competition treat them apart. Rare, and left for later.

## Consequences

- One link opens all of a person's roles; each other link still works.
- A member or coach given a **New link** by their comp sec adds their other
  links to their page again (Decision 8).
- A personal link opened somewhere now carries the person's every role:
  losing it loses more, which the guide says.
- Anyone holding a shared link can't reach anyone's personal pages through
  it; the comp sec and organiser see, and can remove, who opens their pages
  from a personal link.
- Same-named individuals from before stay one key in their competition (as
  they were) until they join their entries; new ones are apart.
