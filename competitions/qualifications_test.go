package competitions

import "testing"

func TestQualifications(t *testing.T) {
	q, ok := QualificationByKey("bg-trampoline-3")
	if !ok || q.Name() != "British Gymnastics Level 3 Trampoline Coach" {
		t.Fatalf("%+v %v", q, ok)
	}
	// A higher level counts for a lower one, trampoline's for synchro, and
	// only in its own discipline.
	for _, c := range []struct {
		discipline string
		level      int
		want       bool
	}{{Trampoline, 2, true}, {Trampoline, 3, true}, {Trampoline, 4, false}, {Synchro, 3, true}, {Tumbling, 1, false}} {
		if got := q.Meets(c.discipline, c.level); got != c.want {
			t.Errorf("%s level %d: %v", DisciplineName(c.discipline), c.level, got)
		}
	}
	if gi, ok := QualificationByKey("gi-dmt-2"); !ok || gi.Name() != "Gymnastics Ireland Level 2 DMT Coach" || !gi.Meets(DMT, 2) {
		t.Errorf("%+v", gi)
	}
	if _, ok := QualificationByKey("gi-dmt-4"); ok || len(Qualifications) != 21 {
		t.Errorf("Gymnastics Ireland's go to level 3: %d in all", len(Qualifications))
	}
}
