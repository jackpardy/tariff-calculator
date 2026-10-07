package competitions

import (
	"slices"
	"testing"

	"tariffCalculator/requirements"
)

func TestLevelOrder(t *testing.T) {
	ref := func(id string) Level { return Level{Ref: requirements.BuiltinLevelPrefix + id} }
	own := func(name string) Level { return Level{Custom: &requirements.Level{Name: name}} }
	names := func(levels []Level) []string {
		var out []string
		for _, l := range levels {
			if l.Custom != nil {
				out = append(out, l.Custom.Name)
			} else {
				out = append(out, l.Ref[len(requirements.BuiltinLevelPrefix):])
			}
		}
		return out
	}

	// BUCS lists its levels hardest first; FIG's age groups go easiest first.
	posted := []Level{ref("bucs-l1"), ref("bucs-l5"), ref("bucs-l7"), ref("fig-ag1"), ref("fig-ag3"), own("Novice"), own("Elite")}
	want := []string{"bucs-l7", "bucs-l5", "bucs-l1", "fig-ag1", "fig-ag3", "Novice", "Elite"}
	if got := names(OrderLevels(nil, posted)); !slices.Equal(got, want) {
		t.Errorf("easiest first: %v, want %v", got, want)
	}

	// A level already in order keeps its place; a new one goes by rank.
	order := []Level{own("Novice"), ref("bucs-l7"), ref("bucs-l1")}
	posted = []Level{ref("bucs-l1"), ref("bucs-l3"), ref("bucs-l7"), own("Novice"), ref("bucs-l6")}
	want = []string{"Novice", "bucs-l7", "bucs-l6", "bucs-l3", "bucs-l1"}
	if got := names(OrderLevels(order, posted)); !slices.Equal(got, want) {
		t.Errorf("kept and placed: %v, want %v", got, want)
	}

	// Judging goes by the order: up to L5 is L7, L6 and L5.
	c := competition(t)
	c.Levels = OrderLevels(nil, []Level{ref("bucs-l1"), ref("bucs-l5"), ref("bucs-l6"), ref("bucs-l7")})
	upToL5 := Offer{Judge: map[string]JudgeOffer{Trampoline: {UpTo: "BUCS L5"}}}
	for level, can := range map[string]bool{"BUCS L7": true, "BUCS L6": true, "BUCS L5": true, "BUCS L1": false} {
		if c.CanJudge(upToL5, false, nil, Trampoline, level) != can {
			t.Errorf("up to BUCS L5 judging %s: want %v", level, can)
		}
	}
	c.Officials.Judge = JudgeBelow
	any := Offer{Judge: map[string]JudgeOffer{Trampoline: {}}}
	competing := map[string]string{Trampoline: "BUCS L5"}
	if !c.CanJudge(any, false, competing, Trampoline, "BUCS L6") || c.CanJudge(any, false, competing, Trampoline, "BUCS L1") {
		t.Error("only levels below their own are the easier ones")
	}

	// The organiser moves a level.
	c.Tumbling = []string{"Elite", "Novice"}
	if err := c.MoveLevel(Tumbling, 1, -1); err != nil || !slices.Equal(c.LevelOrder(Tumbling), []string{"Novice", "Elite"}) {
		t.Errorf("moved: %v, %v", c.LevelOrder(Tumbling), err)
	}
	if err := c.MoveLevel(Trampoline, 3, -1); err != nil || !slices.Equal(c.LevelOrder(Trampoline), []string{"BUCS L7", "BUCS L6", "BUCS L1", "BUCS L5"}) {
		t.Errorf("moved: %v, %v", c.LevelOrder(Trampoline), err)
	}
	for _, bad := range [][2]int{{0, -1}, {3, 1}, {1, 2}, {9, 1}} {
		if c.MoveLevel(Trampoline, bad[0], bad[1]) == nil {
			t.Errorf("moving %v should be refused", bad)
		}
	}
	if c.MoveLevel(Synchro, 0, 1) == nil {
		t.Error("no synchro levels to move")
	}
}
