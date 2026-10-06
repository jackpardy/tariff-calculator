package demo

import (
	"math/rand/v2"
	"testing"

	"tariffCalculator/competitions"
)

func TestVoluntariesPass(t *testing.T) {
	var c competitions.Competition
	for _, l := range []string{"l2", "l3", "l4", "l5", "l6", "l7"} {
		c.Levels = append(c.Levels, competitions.Level{Ref: "builtin-level:bucs-" + l})
	}
	rng := rand.New(rand.NewPCG(1, 2))
	for _, l := range []string{"l3", "l4", "l5", "l6", "l7"} {
		got := voluntaries(c, "BUCS "+string(l[0]-32)+l[1:], 1, [2]string{"builtin:bucs-" + l + "-option-1", "builtin:bucs-" + l + "-second"}, 5, 20000, rng)
		t.Logf("%s: %d", l, len(got))
		if len(got) == 0 {
			t.Errorf("no voluntary for %s", l)
		}
	}
}
