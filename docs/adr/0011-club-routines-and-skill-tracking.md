# ADR 0011 — Club routines and skill tracking

- **Status:** Accepted; Decisions 1 to 7 built 2026-10-15; 8 (moving club) left for later
- **Date:** 2026-10-15
- **Deciders:** jackpardy (solo maintainer)
- **Amends:** ADR 0004 Decision 2 (a club lasts beyond a season), Decision 5
  (routines are kept with a member, not only in an entry) and Decision 6
  (what a club keeps and when it's deleted)

## Context

Everything a club has today is there for competitions (ADR 0004): members
join to enter, coaches sign off entries, the comp sec sends them. Between
competitions there's nothing to do on it. The roadmap's club section
(`knowledge/roadmap.md`, "Next for clubs", decided 2026-10-15) plans two
features that are about the club itself:

1. **Routines on your page.** The routine builder keeps routines in the
   browser's `localStorage`; on the server a routine exists only inside a
   competition entry. A coach can't see a gymnast's routine until it's
   entered, a comp sec changing an entry for a member picks from routines
   saved in their own browser, and a new phone or cleared data loses them.
2. **Skill tracking** (clubs 2). Gymnasts and coaches mark the skills a
   gymnast is learning or can do, and coaches make **club levels** out of
   skills and requirements (so linking one skill into another is tracked),
   to track gymnasts against and share with other clubs.

Both keep data for longer than an entry, and skill tracking keeps new
personal data: a record of what a gymnast, often under 18, can do. ADR 0004
kept personal data to names and clubs, and deletes a club 120 days after
its admin or join link was last used, which a summer break would reach.

The requirements framework (ADR 0003) already describes skills and kinds of
skill (matchers), links (`includes`), counts (`count`, `separate`) and set
routines (`sequence`), with an editor, plain-English descriptions and checks.
Club levels should reuse it rather than add a second way to describe skills.

## Decision

1. **A member's routines are kept with the member.** A table of routines,
   each against a member: its name, its skills as `TrampolineSkill` JSON (as
   the browser and entries store them, ADR 0004 Decision 5: names and
   tariffs worked out again on read), its checks, and when it was last
   changed. At most 30 a member, each at most 30 skills. They're deleted with
   the member (and the club).
   - **Who sees them:** the member, the coaches who see the member (by the
     same rule as entries: their own gymnasts and those without a coach, or
     everyone where the club lets every coach see every member), and the comp
     sec. Only the member adds, renames, changes or deletes them.
   - **Getting them there:** the routine builder offers **Save to my page**
     when a member's link is saved in the browser, posting the routine to
     that member's link; the member's page can add routines from this
     browser in one go. Nothing is copied without the member asking.
   - **Using them:** an entry's routine choice offers the member's saved
     routines as well as the browser's, so the comp sec changing an entry
     for a member, or the member on another phone, can choose them. Opening
     one in the builder goes through the same share-link fragment the
     builder already reads (ADR 0001 §6), so the builder needs no new way
     in.
   - A coach writes a note on a routine for the member to see. Coaches
     adding routines to a gymnast's page is left for later.
2. **Skill marks.** A mark is a member, an item, a side (the gymnast's or a
   coach's), a state (**Learning** or **Can do**), who made it and when. The
   gymnast's mark and a coach's are kept apart, so both show: a skill can be
   Can do to the gymnast and Learning to their coach, and when both say Can
   do, both marks say so (decided 2026-10-15). Each side marks again to
   change its own, and takes off only its own; a coach's mark replaces
   another coach's. The gymnast and the coaches who see them can mark. A
   coach can filter for marks the gymnast made that no coach has made the
   same, and **Confirm** them. An item is:
   - **a skill**, found by the skill: a member's mark on a skill is the one
     whose skill `TrampolineSkill.Equal` calls the same, the one rule the app
     has for that (repeats, carry-over and requirements use it too), so a
     skill marked once counts everywhere it appears. Matching is pairwise,
     as `Equal` is; a canonical form of a skill, to look marks up by, would
     have to come from `Equal` itself rather than repeat its rules;
   - **a club skill** that isn't a Code of Points skill (Decision 4), keyed
     by its id;
   - **a link or a set routine** from a club level (Decision 5), keyed by a
     hash of its rule's canonical JSON, so the same link in two levels
     shares one mark.
3. **Who sees skill tracking.** The gymnast, the coaches who see them, and
   the comp sec. The comp sec can turn on **Every member sees everyone's
   skills** for the club (off to start with). A member's routines are not
   part of that setting.
4. **The club's own skills.** Coaches keep a **club catalogue**:
   - a club name for a skill the catalog can describe, kept as the skill
     with its `custom_name` (the label the skill model already has), so it
     keeps its notation and tariff and works in routines and requirements;
   - anything else worth tracking that isn't a Code of Points skill (a
     drill, a conditioning test): a name and an optional note, for tracking
     only, never in a routine.
5. **Club levels are ordered lists of items, each a requirement or a club
   skill.** A requirement item is one `requirements.Rule` in the existing
   format, written in the requirements editor and described by
   `describe.go`; the requirements format is unchanged. The rule types that
   make sense for a set of skills are allowed: `includes` (a skill, a kind
   of skill, or a link, with options for "any of these"), `count`,
   `separate` and `sequence` (a set routine, so a level can start from a
   competition level's). Rules about a single routine (`elements`,
   `difficulty`, `every`, `different`, `position`, `linked`) aren't offered.
   - **Done from marks:** an `includes` whose options are all one matcher, a
     `count` and a `separate` are checked by the engine against the
     gymnast's Can do skills as a list with no order: done when they pass,
     **working on** when they pass with Learning skills too.
   - **Marked as a whole:** an `includes` with a link, and a `sequence`, are
     marked like a skill (Decision 2). Where one of the gymnast's routines
     already passes the rule, it's shown beside the item as evidence for the
     coach; it doesn't mark the item by itself.
   - A club skill item is done when that skill is marked Can do.
   - **As built (2026-10-15).** The requirements editor is the Requirements
     page's, unchanged: a coach writes a level's requirements there and
     **Save to club levels** posts them to the club (as the routine builder's
     Save to my page does for routines), and the club's page opens them there
     again by a share link. So any rule type can be written; the ones about a
     single routine are left out when saved, saying so. The club's own skills
     to track can't be written as requirements, so a level keeps them as a
     list after its requirements, ticked on the club's Levels page. Each item
     is worked out on each side apart, from the gymnast's marks and from a
     coach's, and both show (Decision 2).
6. **Sharing club levels.** A club level is shared by link like
   requirements and levels (`static/js/share.js`: the level in the link's
   fragment, never stored on the server), bringing the club skills it uses.
   The coach who opens it adds a copy to their club, to track against and
   change. No marks or names travel with it. As built (2026-10-15): the link
   opens a page of its own (`/clubs/levels/shared`), whose script reads the
   fragment and posts it to be shown, and the coach adds it by their coach
   page's link (offered from the ones saved in the browser). Only the club
   skills to track travel; a club's names for catalog skills are its own.
7. **A club is kept until 13 months after it was last used** (was 120 days
   after its admin or join link). Any page opened from the comp sec's,
   a coach's or a member's link, or anything they change, counts as use.
   To keep page loads from writing, `last_used` is updated at most once a
   day. The comp sec's Settings says when the club would be deleted.
   Competitions keep their own retention (120 days after the date), and so
   do individuals.
8. **Moving club is left for later.** A member's routines, marks and
   progress stay with the club they're in, and go when the member is
   removed.

## Alternatives considered

- **Keep routines only in the browser, and share them to the coach by
  link.** No storage, but a coach still never sees the current version,
  and the comp sec's changes still use the wrong browser.
- **A new item format for club levels** (a list of skills and links of its
  own). Simpler to check, but a second way to describe skills, with its own
  editor, wording and tests, when matchers and `includes` already say
  "back drop, then ½ twist to feet" and "any single with a full twist".
- **Marking every item as a whole**, requirements included. Coaches would
  mark "a back somersault" and "Back Tuck" separately; checking skill-like
  items from the gymnast's skill marks means a skill is marked once.
- **Counting a link as done when a routine does it.** A routine shows the
  gymnast meant to do it, not that they can; it's evidence for the coach.
- **Keying skill marks by FIG notation.** It doesn't say direction or
  take-off: a front and a back tuck are both `(4 - o)`.
- **A key of its own for each skill**, written out from `Equal`'s rules, to
  look marks up by. It would be a second copy of the rule for "the same
  skill", which could drift from `Equal`; marks are matched with `Equal`
  itself (2026-10-15).
- **Keeping clubs that track skills longer than others.** Two rules where
  one does; 13 months suits every club over a summer.
- **Keeping every mark's history.** One mark per item, with who and when,
  answers "who says so and since when"; a history can come later if
  coaches ask for progress over time.

## Consequences

- A club is useful between competitions, and a coach sees their gymnasts'
  routines and progress in one place.
- The server keeps more about each gymnast: their routines and what they can
  do, with the names of the coaches who marked it, for up to 13 months
  after the club was last used. The member and comp sec pages say what's
  kept, who sees it and when it's deleted, and the guide's "What's kept"
  is updated.
- Clubs are deleted later than before; unused ones still go on their own.
- Club levels reuse the requirements editor and engine, so a fix there is a
  fix for both.
- Moving club loses a member's routines and progress until moving is built.
