package web

import (
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// standing is where an entry stands. Each page words it for its reader:
// the comp sec's list, a member's and an individual's.
type standing int

const (
	entrySent      standing = iota // sent, and nothing more to say
	entryRemoved                   // removed by the organiser
	entryWithdrawn                 // withdrawn by its gymnast, entering on their own
	entryHeld                      // held by the organiser for changes
	entryNotSent                   // saved by a member, not sent by the club yet
	entryWaiting                   // on its event's waiting list
	entryChanged                   // changed since the club sent it
	entryChecked                   // checked by the organiser
)

// stateOf is where an entry stands, the first of these that holds: removed
// or held (removal), not sent, on a waiting list (place, 0 if not) unless
// changed since, changed since it was sent, checked, sent.
func stateOf(removal string, sent, changed bool, place int, checked bool) standing {
	switch {
	case removal == store.Removed:
		return entryRemoved
	case removal == store.Withdrew:
		return entryWithdrawn
	case removal == store.Held:
		return entryHeld
	case !sent:
		return entryNotSent
	case place > 0 && !changed:
		return entryWaiting
	case changed:
		return entryChanged
	case checked:
		return entryChecked
	}
	return entrySent
}

// memberState is where a member's entry stands: removal is what the
// organiser did with it, if removed, and place its place on a waiting list.
func memberState(e store.MemberEntry, removal store.Entry, removed bool, place int) standing {
	how := ""
	if removed {
		how = removal.Removal
	}
	return stateOf(how, e.Sent(), e.ChangedSinceSent(), place, e.Checked)
}

// individualState is where an individual's entry stands: always sent, as
// they send it themselves.
func individualState(e store.Entry, place int) standing {
	return stateOf(e.Removal, true, false, place, e.Checked())
}

// memberStatus says where a member's entry stands, for the member.
func memberStatus(s standing, removal store.Entry, changed bool) string {
	switch s {
	case entryRemoved, entryHeld:
		return removalStatus(removal, changed)
	case entryNotSent:
		return "Saved, not sent yet"
	case entryChanged:
		return "Changed since your club sent it"
	case entryChecked:
		return "Checked by the organiser"
	}
	return "Sent by your club"
}

// stateOfStanding is the state chip for where an entry stands: removed is
// stopped; held, or changed since sent, needs the club to look; not sent is
// not yet; sent, or on a waiting list, waits on the organiser; checked is
// done.
func stateOfStanding(s standing) views.State {
	switch s {
	case entryRemoved, entryWithdrawn:
		return views.Stopped
	case entryHeld, entryChanged:
		return views.Attention
	case entryNotSent:
		return views.NotYet
	case entryChecked:
		return views.Done
	}
	return views.InProgress
}

// brief is where an entry stands in a word or two, for the list of a
// competition's disciplines, and its state: problems are on its card, and
// place is its place on a waiting list. The words are the glossary's (ui.md),
// the same for a member's entry and an individual's.
func brief(s standing, problems, place int) (string, views.State) {
	switch {
	case s == entryRemoved:
		return "Removed", views.Stopped
	case s == entryWithdrawn:
		return "Withdrawn", views.Stopped
	case s == entryHeld:
		return "On hold", views.Attention
	case problems > 0:
		return views.Count(problems, "problem", "problems"), views.Attention
	}
	switch s {
	case entryNotSent:
		return "Not sent", views.NotYet
	case entryWaiting:
		return "Waiting list, " + waitingWord(place), views.InProgress
	case entryChanged:
		return "Changed since sent", views.Attention
	case entryChecked:
		return "Checked", views.Done
	}
	return "Sent", views.InProgress
}
