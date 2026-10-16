package views

import (
	"context"
	"strings"
	"testing"

	"tariffCalculator/skills"
)

// The skill card links to how a skill is judged, small, beside its take-off
// and landing; a plain straight jump isn't judged, so it has none.
func TestSkillCardJudgingLink(t *testing.T) {
	card := func(s skills.TrampolineSkill) string {
		t.Helper()
		s.NormalizePhases()
		s.SetTariff()
		var b strings.Builder
		if err := editorCard(SkillEditor{Skill: s}).Render(context.Background(), &b); err != nil {
			t.Fatal(err)
		}
		return b.String()
	}
	front := card(skills.TrampolineSkill{Rotation: 4, TwistDistribution: []int{0}, TakeoffPosition: skills.Feet, Shape: skills.Straight})
	if !strings.Contains(front, `class="judging-link"`) || strings.Contains(front, "judging-button") {
		t.Errorf("a small link: %s", front)
	}
	jump := card(skills.TrampolineSkill{TakeoffPosition: skills.Feet, Shape: skills.Straight})
	if strings.Contains(jump, "How it&#39;s judged") || strings.Contains(jump, "How it's judged") {
		t.Errorf("a straight jump isn't judged: %s", jump)
	}
	if tuck := card(skills.TrampolineSkill{TakeoffPosition: skills.Feet, Shape: skills.Tuck}); !strings.Contains(tuck, `class="judging-link"`) {
		t.Error("a tuck jump is")
	}
}
