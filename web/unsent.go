package web

import (
	"fmt"
	"slices"
	"time"

	"tariffCalculator/store"
)

// What a club's members saved but never sent isn't in the competition. Once
// the competition stops taking it from the club, the pages show what it holds
// as the entry, with a line saying what happened to the rest.

// unsentClosed says whether a member's entry has something the competition no
// longer takes from the club: a change made after it was sent, once changes
// have closed, or an entry never sent, once new entries have closed. (Not
// while the competition is private or paused: nothing has closed.)
func unsentClosed(c store.Competition, e store.MemberEntry, now time.Time) bool {
	switch {
	case !c.Live(now):
		return false
	case e.Sent():
		return e.ChangedSinceSent() && !c.ChangesOpen(now)
	}
	return !c.Open(now)
}

// unsent says what became of a member's entry the competition no longer
// takes from the club.
type unsent struct {
	Line  string // the plain line under the entry; "" if nothing was lost
	Never bool   // it was never sent, so the competition holds nothing of it
}

// state is where an entry stands once its unsent part is known: an entry
// never sent isn't "not sent yet", it can't be.
func (u unsent) state(s standing) standing {
	if u.Never && s == entryNotSent {
		return entryUnsent
	}
	return s
}

// heldAs is a member's entry as the competition holds it, if the club sent it
// (sent are the competition's entries) and changed it after, once that change
// can't be sent any more: the level, routine, checks and sign-off the
// organiser has. A never-sent entry stays as it is. Either way it says what
// became of the rest, in whose's words: "You", or the member's name.
func heldAs(c store.Competition, e store.MemberEntry, sent []store.Entry, now time.Time, whose string) (store.MemberEntry, unsent) {
	if !unsentClosed(c, e, now) {
		return e, unsent{}
	}
	closed := "entries"
	if c.ChangesClose().After(c.Deadline) && e.Sent() {
		closed = "changes"
	}
	day := e.UpdatedAt.In(local).Format("2 Jan")
	if !e.Sent() {
		return e, unsent{Never: true, Line: fmt.Sprintf("%s saved this on %s, but it wasn't sent before entries closed, so it isn't in the competition.", whose, day)}
	}
	i := slices.IndexFunc(sent, func(s store.Entry) bool { return s.MemberID == e.MemberID && s.Entry.Discipline == e.Discipline })
	if i < 0 {
		return e, unsent{} // removed or held by the organiser: its own words say where it stands
	}
	held := sent[i]
	line := fmt.Sprintf("%s changed this to %s on %s, but it wasn't sent before %s closed. The competition has %s.",
		whose, e.Entry.Event(), day, closed, held.Entry.Event())
	if e.Entry.Event() == held.Entry.Event() {
		line = fmt.Sprintf("%s changed this on %s, but it wasn't sent before %s closed. The competition has it as it was sent.", whose, day, closed)
	}
	e.Entry, e.UpdatedAt, e.SentAt = held.Entry, held.SentAt, held.SentAt
	e.Checked, e.Note, e.VideoReview, e.VideoNote = held.Checked(), held.Note, held.VideoReview, held.VideoNote
	e.SignedAt, e.SignedBy, e.SignNote = held.SignedAt, held.SignedBy, held.SignNote
	e.PartnerConfirmed = held.PartnerConfirmed
	return e, unsent{Line: line}
}

// heldCopy is the competition's copy of a member's entry, if it holds one the
// pages can show (sent are the competition's entries): not one removed or held.
func heldCopy(sent []store.Entry, e store.MemberEntry) (store.Entry, bool) {
	i := slices.IndexFunc(sent, func(s store.Entry) bool { return s.MemberID == e.MemberID && s.Entry.Discipline == e.Discipline })
	if i < 0 {
		return store.Entry{}, false
	}
	return sent[i], true
}

// heldToken says which copy a coach saw to sign off, to post back: the copy
// and when it was sent, so a form from before it was replaced is known.
func heldToken(h store.Entry) string { return fmt.Sprintf("%s@%d", h.ID, h.SentAt.UnixNano()) }

// newClosed says a member's entry can't be saved: new entries have closed (the
// competition is open for changes to what it holds) and the club hasn't sent
// it, so it never will be. existing is the member's entry, if they have one.
func newClosed(c store.Competition, existing *store.MemberEntry, now time.Time) bool {
	return c.ChangesOpen(now) && !c.Open(now) && (existing == nil || !existing.Sent())
}
