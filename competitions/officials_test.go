package competitions

import (
	"strings"
	"testing"
)

func TestPanels(t *testing.T) {
	var s OfficialSettings
	if p := s.Panel(Tumbling); p != CodePanel || p.Judges() != 9 {
		t.Errorf("the Code of Points' panel by default: %+v", p)
	}
	if p := s.Panel(Synchro); p.Sync != 2 || p.Judges() != 11 || len(p.Seats()) != 13 || p.Seats()[3] != RoleSync {
		t.Errorf("synchro adds 2 synchronisation judges by default: %+v %v", p, p.Seats())
	}
	if !(RotaPerson{Judge: map[string]bool{"Synchro BUCS L3": true}}).Can(RoleSync, "Synchro BUCS L3") || RoleName(RoleSync) != "Synchronisation judge" {
		t.Error("a synchro judge can judge synchronisation")
	}
	s.Panels = map[string]Panel{Synchro: {Chair: 1, Execution: 4, Difficulty: 1}}
	if p := s.Panel(Synchro); p.Judges() != 6 || p.Recorder != 0 {
		t.Errorf("a changed panel: %+v", p)
	}
	if err := s.Check(); err != nil {
		t.Error(err)
	}
	for _, bad := range []OfficialSettings{
		{Judge: "anyone"},
		{Panels: map[string]Panel{"aerobics": CodePanel}},
		{Panels: map[string]Panel{Trampoline: {Execution: -1}}},
	} {
		if bad.Check() == nil {
			t.Errorf("%+v should be refused", bad)
		}
	}
	c := competition(t)
	c.Officials = OfficialSettings{Judge: "anyone"}
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "judging rule") {
		t.Errorf("the competition checks its officials' settings: %v", err)
	}
}

func TestOffers(t *testing.T) {
	c := competition(t)
	c.Tumbling = []string{"Novice", "Elite"}
	o := Offer{Judge: map[string]JudgeOffer{Trampoline: {UpTo: "FIG AG3 (17–21)", Chair: true}, Tumbling: {}}, Recorder: true}
	if err := c.ValidateOffer(o); err != nil {
		t.Fatal(err)
	}
	if got := o.Describe(); got != "Judge Trampoline up to FIG AG3 (17–21) (can chair) and Tumbling; recorder" {
		t.Errorf("described as %q", got)
	}
	if (Offer{}).Describe() != "" || !(Offer{}).Empty() || o.Empty() {
		t.Error("empty offers")
	}
	for _, bad := range []Offer{
		{Judge: map[string]JudgeOffer{DMT: {}}},
		{Judge: map[string]JudgeOffer{Tumbling: {UpTo: "Master"}}},
	} {
		if c.ValidateOffer(bad) == nil {
			t.Errorf("%+v should be refused", bad)
		}
	}

	// Levels in order: BUCS L3, FIG AG3, Club novice.
	upToAG3 := Offer{Judge: map[string]JudgeOffer{Trampoline: {UpTo: "FIG AG3 (17–21)"}}}
	if !c.CanJudge(upToAG3, false, nil, Trampoline, "BUCS L3") || c.CanJudge(upToAG3, false, nil, Trampoline, "Club novice") {
		t.Error("up to the level they say")
	}
	if c.CanJudge(upToAG3, false, nil, Tumbling, "Novice") || c.CanJudge(upToAG3, false, nil, Trampoline, "BUCS L9") {
		t.Error("not disciplines or levels they didn't offer")
	}
	any := Offer{Judge: map[string]JudgeOffer{Trampoline: {}}}
	c.Officials.Judge = JudgeBelow
	competing := map[string]string{Trampoline: "FIG AG3 (17–21)"}
	if !c.CanJudge(any, false, competing, Trampoline, "BUCS L3") || c.CanJudge(any, false, competing, Trampoline, "FIG AG3 (17–21)") {
		t.Error("only levels below their own")
	}
	if !c.CanJudge(any, false, nil, Trampoline, "Club novice") {
		t.Error("someone not competing in the discipline judges any level they offer")
	}
	c.Officials.Judge = JudgeQualified
	if c.CanJudge(any, false, nil, Trampoline, "BUCS L3") || !c.CanJudge(any, true, nil, Trampoline, "BUCS L3") {
		t.Error("only people marked qualified")
	}
}
