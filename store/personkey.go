package store

import "strings"

// Person keys (ADR 0005 Decision 4, ADR 0010) name someone the same way
// wherever they appear in a competition: in its entries, its timetable's
// seats and rules, its desk messages, scratch warnings cleared and officials'
// check-ins. A key is a kind and an id, "<kind>:<id>". Every key is made and
// read here, so their forms are kept in one place.
const (
	KindMember     = "m" // a club's member, by member id
	KindIndividual = "i" // someone entering on their own, by individual id (or, from before, by name)
	KindCoach      = "c" // a club's coach, by coach id; an individual's coach, by name
	KindOfficial   = "o" // an official who is neither a member nor entering, by official id
	KindPartner    = "p" // a synchro partner not yet confirmed, by the entry naming them
)

func key(kind, id string) string { return kind + ":" + id }

// MemberKey is a club member's person key.
func MemberKey(memberID string) string { return key(KindMember, memberID) }

// IndividualKey is the person key of a new individual in a competition: their
// own (ADR 0010 Decision 7).
func IndividualKey(individualID string) string { return key(KindIndividual, individualID) }

// IndividualNameKey was an individual's person key before individuals had one
// link: their name, so two with one name were one person. Their entries from
// then keep it, so the keys a competition saved still find them.
func IndividualNameKey(name string) string { return key(KindIndividual, NameKey(name)) }

// CoachKey is a club coach's person key, by their record, so renaming them
// or a sign-off typed differently is still them.
func CoachKey(coachID string) string { return key(KindCoach, coachID) }

// NamedCoachKey is the person key of an individual's coach, who has no
// record: their name as typed ("" for none). The empty id before the name
// keeps it apart from any club coach's key.
func NamedCoachKey(name string) string {
	if name = NameKey(name); name != "" {
		return key(KindCoach, ":"+name)
	}
	return ""
}

// clubCoachNameKey was a club coach's person key before coaches were keyed by
// their record (migration 34): their club and name as typed.
func clubCoachNameKey(clubID, name string) string {
	if name = NameKey(name); name != "" {
		return key(KindCoach, clubID+":"+name)
	}
	return ""
}

// OfficialKey is the person key of an official who neither belongs to a club
// in the competition nor enters it on their own.
func OfficialKey(officialID string) string { return key(KindOfficial, officialID) }

// PartnerKey is the person key of an entry's synchro partner who hasn't
// confirmed, so is known only by that entry.
func PartnerKey(entryID string) string { return key(KindPartner, entryID) }

// KeyKind is a person key's kind (KindMember, ...) and the id after it.
func KeyKind(personKey string) (kind, id string) {
	kind, id, _ = strings.Cut(personKey, ":")
	return kind, id
}

// keyedJSON are the columns holding JSON with person keys in it, and the
// column naming the competition their row is for: the timetable's seats and
// rules, its simulations, and desk messages' people.
var keyedJSON = []struct{ table, column, competition string }{
	{"competitions", "timetable", "id"},
	{"competitions", "published_timetable", "id"},
	{"competitions", "scenarios", "id"},
	{"desk_messages", "people", "competition_id"},
}

// keyedRows are the tables with a person key in a column of their own (all
// named person), keyed by competition, person and also.
var keyedRows = []struct{ table, also string }{
	{"scratch_clears", "kind"},
	{"official_checkins", "flight"},
}
