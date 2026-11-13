package store

import (
	"testing"
	"time"

	"tariffCalculator/competitions"
)

// CarryLevels rewrites an offer's "up to" level everywhere the competition
// keeps offers (members' own, the organiser's held copies, added people's),
// for that competition only, and the offer can then be seated at the new
// level.
func TestCarryLevels(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	offer := func(level string) competitions.Offer {
		return competitions.Offer{Recorder: true, Judge: map[string]competitions.JudgeOffer{
			competitions.Tumbling: {UpTo: map[string]string{competitions.RoleExecution: level, competitions.RoleChair: "T1"}},
			competitions.DMT:      {UpTo: map[string]string{competitions.RoleExecution: level}},
		}}
	}
	comp := competition()
	comp.Tumbling, comp.DMT = []string{"T1", "T2", "T3"}, []string{"T2"}
	one, _, err := s.CreateCompetition(ctx, comp)
	must(t, err)
	two, _, err := s.CreateCompetition(ctx, comp)
	must(t, err)
	club, _, _ := s.CreateClub(ctx, "UCD")
	must(t, s.AttachClub(ctx, club.ID, one.ID))
	bea, _, err := s.Join(ctx, club.ID, "Bea")
	must(t, err)
	must(t, s.SaveMemberOffer(ctx, bea.ID, one.ID, offer("T2")))
	_, err = s.AddOfficial(ctx, one.ID, "Ann", "", offer("T2"))
	must(t, err)
	_, err = s.AddOfficial(ctx, one.ID, "Cal", "", offer("T3"))
	must(t, err)
	_, err = s.AddOfficial(ctx, two.ID, "Dee", "", offer("T2"))
	must(t, err)

	if n, err := s.LevelOffers(ctx, one.ID, competitions.Tumbling, "T2"); err != nil || n != 2 {
		t.Fatalf("two people say up to T2: %d %v", n, err)
	}
	rename := competitions.LevelRename{Discipline: competitions.Tumbling, From: "T2", To: "Tier 2"}
	must(t, s.SetOfficialSettings(ctx, one.ID, competitions.OfficialSettings{Renamed: []competitions.LevelRename{rename}}))
	counts, err := s.CarryLevels(ctx, one.ID, []competitions.LevelRename{rename})
	if err != nil || len(counts) != 1 || counts[0] != 2 {
		t.Fatalf("carried: %v %v", counts, err)
	}
	people, _ := s.Officials(ctx, one.ID)
	got := map[string]competitions.Offer{}
	for _, o := range people {
		got[o.Name] = o.Offer
	}
	if j := got["Ann"].Judge[competitions.Tumbling]; j.UpTo[competitions.RoleExecution] != "Tier 2" || j.UpTo[competitions.RoleChair] != "T1" {
		t.Errorf("Ann's tumbling: %v", j.UpTo)
	}
	if got["Ann"].Judge[competitions.DMT].UpTo[competitions.RoleExecution] != "T2" {
		t.Error("another discipline's T2 isn't the same level")
	}
	if !got["Ann"].Recorder || got["Cal"].Judge[competitions.Tumbling].UpTo[competitions.RoleExecution] != "T3" {
		t.Error("the rest is as it was")
	}
	if mine, _ := s.MemberOffer(ctx, bea.ID, one.ID); mine.Judge[competitions.Tumbling].UpTo[competitions.RoleExecution] != "Tier 2" {
		t.Error("the member's own offer follows")
	}
	if other, _ := s.Officials(ctx, two.ID); other[0].Offer.Judge[competitions.Tumbling].UpTo[competitions.RoleExecution] != "T2" {
		t.Error("another competition's offers aren't touched")
	}
	if c, _ := s.Competition(ctx, one.ID); len(c.Officials.Renamed) != 0 {
		t.Errorf("a carried rename is forgotten: %v", c.Officials.Renamed)
	}

	// The carried offer can be seated at the new level, and not past it.
	c := competitions.Competition{Tumbling: []string{"T1", "Tier 2", "T3"}}
	if !c.CanJudge(got["Ann"], false, competitions.Tumbling, "Tier 2", competitions.RoleExecution) || c.CanJudge(got["Ann"], false, competitions.Tumbling, "T3", competitions.RoleExecution) {
		t.Error("Ann judges up to Tier 2")
	}
}
