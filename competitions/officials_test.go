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
		{Judge: "below"}, // gone: the organiser's note asks for it instead
		{Note: strings.Repeat("x", 501)},
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
	o := Offer{Judge: map[string]JudgeOffer{
		Trampoline: {UpTo: map[string]string{RoleExecution: "FIG AG3 (17–21)", RoleDifficulty: "FIG AG3 (17–21)", RoleChair: ""}},
		Tumbling:   {UpTo: map[string]string{RoleDifficulty: ""}},
	}, Recorder: true}
	if err := c.ValidateOffer(o); err != nil {
		t.Fatal(err)
	}
	if got := o.Describe(); got != "Judge Trampoline (execution and difficulty up to FIG AG3 (17–21), chair at any level, and HD) and Tumbling (difficulty at any level); Recorder" {
		t.Errorf("described as %q", got)
	}
	if got := (Offer{Judge: map[string]JudgeOffer{Trampoline: {}}}).Describe(); got != "Judge Trampoline (HD only)" {
		t.Errorf("HD only, described as %q", got)
	}
	if got := (Offer{Judge: map[string]JudgeOffer{Synchro: {}}}).Describe(); got != "" {
		t.Errorf("an offer from before for synchro counts for nothing: %q", got)
	}
	// Tumbling and DMT have no HD, so an offer from before levels were by
	// role offers no seat there: said, not empty brackets.
	old := Offer{Judge: map[string]JudgeOffer{Trampoline: {}, Tumbling: {}, DMT: {}}, Marshal: true}
	if got := old.Describe(); got != "Judge Trampoline (HD only), Tumbling (no role chosen yet) and DMT (no role chosen yet); Marshal" {
		t.Errorf("no role, described as %q", got)
	}
	if !old.Judges(Trampoline) || old.Judges(Tumbling) || old.Judges(DMT) || !old.Judges(Synchro) || !old.JudgesAny() {
		t.Error("a seat only where judging has HD or a role")
	}
	if (Offer{Judge: map[string]JudgeOffer{DMT: {}, Synchro: {}}}).JudgesAny() || !(Offer{Judge: map[string]JudgeOffer{DMT: {UpTo: map[string]string{RoleChair: ""}}}}).JudgesAny() {
		t.Error("DMT with no role, or synchro alone, judges nothing; DMT with a role does")
	}
	if (Offer{}).Describe() != "" || !(Offer{}).Empty() || o.Empty() {
		t.Error("empty offers")
	}
	for _, bad := range []Offer{
		{Judge: map[string]JudgeOffer{DMT: {}}},
		{Judge: map[string]JudgeOffer{Tumbling: {UpTo: map[string]string{RoleExecution: "Master"}}}},
		{Judge: map[string]JudgeOffer{Tumbling: {UpTo: map[string]string{RoleHD: ""}}}},
		{Judge: map[string]JudgeOffer{Tumbling: {}}}, // nothing to judge: tumbling has no HD
		{Judge: map[string]JudgeOffer{Synchro: {}}},  // trampoline's judges judge it
	} {
		if c.ValidateOffer(bad) == nil {
			t.Errorf("%+v should be refused", bad)
		}
	}

	// Levels in order: BUCS L3, FIG AG3, Club novice. Each role up to its
	// own level: difficulty at a level they don't judge execution at.
	j := Offer{Judge: map[string]JudgeOffer{Trampoline: {UpTo: map[string]string{RoleExecution: "BUCS L3", RoleDifficulty: "FIG AG3 (17–21)"}}}}
	if !c.CanJudge(j, false, Trampoline, "BUCS L3", RoleExecution) || c.CanJudge(j, false, Trampoline, "FIG AG3 (17–21)", RoleExecution) {
		t.Error("execution up to the level they say")
	}
	if !c.CanJudge(j, false, Trampoline, "FIG AG3 (17–21)", RoleDifficulty) || c.CanJudge(j, false, Trampoline, "Club novice", RoleDifficulty) {
		t.Error("difficulty up to its own level")
	}
	if c.CanJudge(j, false, Trampoline, "BUCS L3", RoleChair) {
		t.Error("not a role they didn't offer")
	}
	if !c.CanJudge(j, false, Trampoline, "Club novice", RoleHD) {
		t.Error("HD at any level of a discipline they judge")
	}
	if c.CanJudge(j, false, Tumbling, "Novice", RoleExecution) || c.CanJudge(j, false, Trampoline, "BUCS L9", RoleHD) {
		t.Error("not disciplines or levels they didn't offer")
	}
	c.Officials.Judge = JudgeQualified
	if c.CanJudge(j, false, Trampoline, "BUCS L3", RoleExecution) || c.CanJudge(j, false, Trampoline, "BUCS L3", RoleHD) || !c.CanJudge(j, true, Trampoline, "BUCS L3", RoleExecution) {
		t.Error("only people marked qualified, for every judging role")
	}
}

func TestRotaPersonRoles(t *testing.T) {
	var p RotaPerson
	p.Allow(RoleDifficulty, "BUCS L7")
	p.Allow(RoleHD, "BUCS L5")
	if !p.Can(RoleDifficulty, "BUCS L7") || p.Can(RoleExecution, "BUCS L7") || !p.Can(RoleHD, "BUCS L7") || !p.Can(RoleHD, "BUCS L5") || p.Can(RoleDifficulty, "BUCS L5") {
		t.Errorf("each role on its own, HD wherever they judge: %+v", p)
	}
	if !p.CanAny(RoleDifficulty) || p.CanAny(RoleChair) || p.CanAny(RoleRecorder) {
		t.Errorf("blocked time: %+v", p)
	}
}
