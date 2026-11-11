package competitions

import (
	"slices"
	"testing"
)

// An offer that says "up to" a level the competition hasn't (any more) takes
// that role at no level; HD and synchronisation stay at any level.
func TestOfferLevelGone(t *testing.T) {
	c := competition(t)
	c.Tumbling = []string{"Novice", "Elite"}
	o := Offer{Judge: map[string]JudgeOffer{
		Trampoline: {UpTo: map[string]string{RoleExecution: "BUCS L9", RoleDifficulty: "BUCS L9", RoleChair: "BUCS L9"}},
		Tumbling:   {UpTo: map[string]string{RoleExecution: "Elite", RoleDifficulty: ""}},
	}}
	if got := c.GoneLevels(o); !slices.Equal(got, []string{"BUCS L9"}) {
		t.Errorf("the level that has gone, once: %v", got)
	}
	if got := GoneLine(c.GoneLevels(o)); got != "Says up to BUCS L9, which isn't a level any more: ask them, or set it" ||
		GoneLine(nil) != "" || GoneLine([]string{"T2", "T3"}) != "Says up to T2 and T3, which aren't levels any more: ask them, or set it" {
		t.Errorf("the line: %q", got)
	}
	for _, role := range []string{RoleExecution, RoleDifficulty, RoleChair} {
		for _, l := range []string{"BUCS L3", "FIG AG3 (17–21)", "Club novice"} {
			if c.CanJudge(o, false, Trampoline, l, role) {
				t.Errorf("%s at %s: its level has gone", role, l)
			}
		}
	}
	for _, l := range []string{"BUCS L3", "Club novice"} {
		if !c.CanJudge(o, false, Trampoline, l, RoleHD) {
			t.Errorf("HD at %s stays at any level", l)
		}
	}
	c.Synchro = []Level{{Ref: "builtin-level:bucs-l3"}}
	if !c.CanJudge(o, false, Synchro, "BUCS L3", RoleSync) {
		t.Error("synchronisation stays at any level")
	}
	if !c.CanJudge(o, false, Tumbling, "Elite", RoleExecution) || !c.CanJudge(o, false, Tumbling, "Novice", RoleDifficulty) {
		t.Error("levels that are there still work")
	}
	c.Tumbling = []string{"Novice"} // Elite goes
	if c.CanJudge(o, false, Tumbling, "Novice", RoleExecution) || !c.CanJudge(o, false, Tumbling, "Novice", RoleDifficulty) {
		t.Error("execution at no level once Elite has gone; difficulty at any level stays")
	}
	if got := c.GoneLevels(o); !slices.Equal(got, []string{"BUCS L9", "Elite"}) {
		t.Errorf("both: %v", got)
	}
	if got := c.GoneLevels(Offer{Judge: map[string]JudgeOffer{Trampoline: {UpTo: map[string]string{RoleHD: ""}}}}); len(got) != 0 {
		t.Errorf("no level named, none gone: %v", got)
	}
}

// A seat held by someone whose offer's level has gone is reported, with why.
func TestHolderBreaksLevelGone(t *testing.T) {
	entries := people("BUCS L3", Trampoline, 30)
	s := PlanSchedule(entries, []string{"BUCS L3"}, venue(), 1)
	officials := judges("J", 12, "BUCS L3")
	s.Rota(entries, officials, OfficialSettings{}, 1)
	chair := s.Flights[0].Officials[0]
	var left []RotaPerson
	for _, o := range officials {
		if o.Key == chair.Person {
			o.Chair, o.Gone = nil, []string{"T2"}
		}
		left = append(left, o)
	}
	want := chair.Name + " can't take the chair of judges seat at BUCS L3: says up to T2, which isn't a level any more: ask them, or set it"
	if bs := s.HolderBreaks(left, nil); len(bs) != 1 || bs[0].Line != want {
		t.Errorf("held, reported: %v", bs)
	}
}
