# ADR 0007 — Approved coaches

- **Status:** Accepted; step 2 (qualifications and certificates) built
  2026-10-08
- **Date:** 2026-10-08
- **Deciders:** jackpardy (solo maintainer)
- **Amends:** ADR 0004 Decisions 6 (personal data: certificates are kept),
  7 (the 64 KB body limit, for uploads) and 11 (coach sign-off: only
  approved coaches, where the organiser asks)

## Context

A competition can require a coach's sign-off on each entry (ADR 0004
Decision 11), but anyone the comp sec adds as a coach can sign off, and an
individual can send their sign-off link to anyone. Trying the tools with an
ISTO organiser (roadmap, 2026-10-08), approving coaches was one of their two
big asks: organisers need to know the person signing off a routine is
qualified to coach it.

Decided on the roadmap (2026-10-07 and 2026-10-08):

- The organiser can require coaches to be **approved**, and picks the
  qualification they need from a list (British Gymnastics and Gymnastics
  Ireland coaching levels, by discipline); a higher level counts for a
  lower one.
- Clubs send their coaches to the competition with a **certificate** (a
  photo or PDF); an individual names their coach and uploads theirs.
- The organiser approves each coach for their competition alone. An
  unapproved coach can't sign off, and if an approval is withdrawn, that
  coach's sign-offs stop counting.
- No expiry date is asked for. A club's certificate is kept with the club
  for its next competitions (approved afresh at each).
- The comp sec ticks which coaches go to each competition; clubs can say
  which of their coaches sign off at all; approving a coach again, the
  organiser chooses whether their earlier sign-offs count; sign-offs from
  before this count as not signed off where approval is needed
  (2026-10-08).

Today a sign-off records the coach's name only (`signed_by`), not which
coach, so a competition can't tell whether the person who signed is one it
approved.

## Decision

1. **Qualifications are a built-in list.** Each is a governing body
   (British Gymnastics, Gymnastics Ireland), a discipline (trampoline, which
   covers synchro; tumbling; DMT) and a **level** (1 to 4), with its name,
   e.g. "British Gymnastics Level 2 Trampoline Coach". Like the levels'
   rules, the list lives in Go. Levels compare across bodies: a competition
   asks for a level, and either body's qualification at that level or above
   meets it.

2. **The organiser turns it on.** Where entries need a coach's sign-off, the
   organiser can tick **Coaches must be approved**, and set, for each
   discipline the competition offers, the lowest level accepted (default:
   Level 2, the first level that coaches somersaults unsupervised).

3. **A club says which coaches sign off.** Each of a club's coaches **can
   sign off** or not, the comp sec's choice (existing coaches can, as now).
   A coach who can't still sees their members' entries, as checked, for the
   club's own use; only coaches who can sign off do, and only they can be
   sent to a competition for approval.

4. **A club's coaches have qualifications.** On the club page, the comp sec
   gives each coach their qualifications, each with its certificate: a
   JPEG, PNG, WebP or PDF of up to 10 MB, at most 4 per coach. They're kept
   with the coach until the comp sec replaces or removes them, removes the
   coach, or the club is deleted (ADR 0004 Decision 6). Only the comp sec
   and the organisers the coach is sent to can open them.

5. **Clubs send coaches to a competition.** For a competition that requires
   approved coaches, the club's page lists its coaches who can sign off, to
   **send**, each ticked by the comp sec, the coaches of members entered
   ticked to start with. Sending a coach gives the competition their name, qualifications
   and certificates as they are then; changing them later sends them again,
   for approving again.

6. **The organiser approves.** A **Coaches** page lists each club's coaches
   sent, and individuals' coaches, with their qualifications (flagged where
   none reaches the level asked for that discipline) and certificates to
   open. Each is **waiting**, **approved** or **not approved** (with a note
   back to the club or individual). An approval can be withdrawn at any
   time. Approving a coach again, the organiser chooses whether the
   sign-offs they gave before count again, or only those they give from
   then on.

7. **Only approved coaches sign off.** A sign-off records **which coach**
   gave it (a new `signed_coach`, beside `signed_by`), on the club's copy
   and the competition's. Where coaches must be approved:
   - a coach not approved for a competition sees why on their page
     ("waiting for the organiser", or the organiser's note) and can't sign
     off its entries;
   - the organiser's dashboard counts a sign-off only while its coach is
     approved, and (if the organiser started them afresh) only one given
     since the approval: if the approval is withdrawn, the entry shows "not
     signed off" (and why) until an approved coach signs it off. Nothing in
     the club's own records is erased;
   - entries signed off before this existed (no `signed_coach`) count as not
     signed off at a competition that requires approval.

8. **Individuals name their coach.** An individual's entry page asks for
   their coach's name, qualification and certificate. The sign-off link
   works once the organiser approves that coach. The coach and certificate
   belong to the entry, and are deleted with the competition.

9. **Certificates are stored in the database.** A certificate is a row in
   SQLite (`certificates`: its owner, type, size, bytes and when it was
   uploaded), so it's in the nightly backup and goes when its owner does.
   The server checks the type from the file's own bytes, not its name, and
   serves it only to the comp sec or an organiser it was sent to, with its
   type, `nosniff`, and no caching. The upload routes allow 10 MB; every
   other request keeps the 64 KB limit.

## Consequences

**Positive**
- Organisers can rely on a sign-off coming from someone they've checked.
- Clubs upload a coach's certificates once, not for every competition.
- Nothing changes for a competition that doesn't ask for approval.

**Negative / risks**
- **Documents are personal data.** A certificate names a person and may
  show more (a membership number, a date of birth). The pages say who sees
  it and when it's deleted; only the comp sec and the organisers it's sent
  to can open it.
- **Storage grows.** Up to 10 MB per certificate, 4 per coach and 50 coaches
  a club; in practice a phone photo is 1–3 MB. The backup grows with it.
- **The list needs checking.** The names and levels of the bodies'
  qualifications are from their published pathways (2026) and need
  confirming; a coach whose qualification isn't on the list can't be sent
  until it is.
- **Withdrawn approval is quiet for clubs** unless notifications exist
  (roadmap); the club and member see "not signed off" on their pages.

## Alternatives considered

- **A membership number instead of a certificate.** Less to store, but the
  organiser would have to look each one up on the governing body's system,
  which isn't open to them.
- **Approval carried across competitions.** Less work for organisers, but
  there's no one to trust across competitions, and each organiser's
  requirements differ.
- **Certificates as files on disk.** Smaller database, but they'd need their
  own backup and deletion, and could outlive their owners.

## Migration

Each step is a separate, shippable branch:

1. This ADR.
2. **Qualifications and certificates on the club page:** the list; which
   coaches can sign off; coaches' qualifications with certificates (`certificates` table, uploads, viewing
   by the comp sec).
3. **Approval:** the competition's setting and levels; sending coaches;
   the organiser's Coaches page; `signed_coach`; only approved coaches sign
   off, and the dashboard counts only their sign-offs.
4. **Individuals' coaches:** named on the entry with a certificate; the
   sign-off link waits for approval.
