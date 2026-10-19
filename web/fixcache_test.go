package web

import (
	"fmt"
	"reflect"
	"testing"
	"unsafe"

	"tariffCalculator/competitions"
)

// A timetable that doesn't fit works out its fixes once, and again only
// when its entries or setup change.
func TestFixesKept(t *testing.T) {
	p := newCompetitionPages(nil)
	var entries []competitions.SchedEntry
	for i := range 30 {
		id := fmt.Sprintf("e%d", i)
		entries = append(entries, competitions.SchedEntry{
			PlanEntry: competitions.PlanEntry{ID: id, Level: "L1", Club: "UCD"}, Discipline: competitions.Trampoline, People: []string{id},
		})
	}
	events := []string{"L1"}
	setup := competitions.DefaultSetup([]string{competitions.Trampoline})
	setup.Days[0].End = "10:00"
	first := p.fixesOf("c", entries, events, setup)
	if want := competitions.Fixes(entries, events, setup, 1); len(first) == 0 || !reflect.DeepEqual(first, want) {
		t.Fatalf("fixes %+v, want %+v", first, want)
	}
	if again := p.fixesOf("c", entries, events, setup); unsafe.SliceData(again) != unsafe.SliceData(first) {
		t.Error("with nothing changed, they're worked out once")
	}
	setup.Days[0].End = "10:30"
	if later := p.fixesOf("c", entries, events, setup); unsafe.SliceData(later) == unsafe.SliceData(first) ||
		!reflect.DeepEqual(later, competitions.Fixes(entries, events, setup, 1)) {
		t.Error("a change to the setup works them out again")
	}
	if fewer := p.fixesOf("c", entries[:20], events, setup); !reflect.DeepEqual(fewer, competitions.Fixes(entries[:20], events, setup, 1)) {
		t.Error("a change to the entries works them out again")
	}
}
