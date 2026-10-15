package web

import (
	"fmt"

	"tariffCalculator/store"
)

// standing is where an entry stands. Each page words it for its reader:
// the comp sec's list, a member's and an individual's.
type standing int

const (
	entrySent    standing = iota // sent, and nothing more to say
	entryRemoved                 // removed by the organiser
	entryHeld                    // held by the organiser for changes
	entryNotSent                 // saved by a member, not sent by the club yet
	entryWaiting                 // on its event's waiting list
	entryChanged                 // changed since the club sent it
	entryChecked                 // checked by the organiser
)

// stateOf is where an entry stands, the first of these that holds: removed
// or held (removal), not sent, on a waiting list (place, 0 if not) unless
// changed since, changed since it was sent, checked, sent.
func stateOf(removal string, sent, changed bool, place int, checked bool) standing {
	switch {
	case removal == store.Removed:
		return entryRemoved
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
		return "Sent by your club · checked by the organiser ✓"
	}
	return "Sent by your club"
}

// brief is where an entry stands in a word or two, for the list of a
// competition's disciplines, and the Bulma colour of its tag: problems are on
// its card, place is its place on a waiting list, and sent is how a sent
// entry is put.
func brief(s standing, problems, place int, sent string) (string, string) {
	switch {
	case s == entryRemoved:
		return "Removed", "is-danger is-light"
	case s == entryHeld:
		return "On hold", "is-warning is-light"
	case problems == 1:
		return "1 problem", "is-danger is-light"
	case problems > 1:
		return fmt.Sprintf("%d problems", problems), "is-danger is-light"
	}
	switch s {
	case entryNotSent:
		return "Saved, not sent", "is-info is-light"
	case entryWaiting:
		return "Waiting list, " + waitingWord(place), "is-warning is-light"
	case entryChanged:
		return "Changed, send again", "is-warning is-light"
	case entryChecked:
		return "Checked", "is-success is-light"
	}
	return sent, "is-success is-light"
}
