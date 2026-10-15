package web

import (
	"slices"
	"testing"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
)

// A person's flights: the entries they're in (a partner's too) and their
// seats; a club's: its entries, and seats held by its members or by the
// organiser's officials from it.
func TestOurFlights(t *testing.T) {
	ann, bea := store.MemberKey("ann"), store.MemberKey("bea")
	people := map[string][]string{"e1": {ann}, "e2": {bea, ann}, "e3": {bea}}
	seats := []competitions.Duty{{Role: "chair", Person: bea}, {Role: "execution", Person: ann}, {Role: "execution", Person: "o:mary"}}
	in, held := inSlot(ann, people, []string{"e1", "e2", "e3"}, seats)
	if !slices.Equal(in, []string{"e1", "e2"}) || len(held) != 1 || held[0].Role != "execution" {
		t.Errorf("Ann's: %v, %v", in, held)
	}
	if in, held := inSlot(store.MemberKey("cat"), people, []string{"e1"}, seats); in != nil || held != nil {
		t.Errorf("no one else's: %v, %v", in, held)
	}

	club := ourClub{id: "c1", name: "Leaps", people: map[string]bool{ann: true}}
	f := rotaFacts{
		entries:   []store.Entry{{ID: "e1", ClubID: "c1"}, {ID: "e3", ClubID: "c2"}},
		officials: []competitions.RotaPerson{{Key: "o:mary", Club: "Leaps"}, {Key: "o:tom", Club: "Bounce"}},
	}
	ours := club.at(f)
	entries, held := ours.inSlot([]string{"e1", "e3"}, seats)
	if len(entries) != 1 || entries[0].ID != "e1" {
		t.Errorf("the club's entries: %v", entries)
	}
	var who []string
	for _, d := range held {
		who = append(who, d.Person)
	}
	if !slices.Equal(who, []string{ann, "o:mary"}) {
		t.Errorf("the club's seats: %v", who)
	}
	if club.people["o:mary"] {
		t.Error("at leaves the club as it was")
	}
}
