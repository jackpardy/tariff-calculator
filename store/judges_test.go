package store

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"tariffCalculator/competitions"
)

// A member's judging qualifications are kept with them, count towards the
// club's room for certificates, and reach the organiser with their offer; an
// individual's go with their entry, whichever of the gymnast's it's on.
func TestJudgeQualifications(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	comp := competition()
	comp.Individuals = true
	c, _, err := s.CreateCompetition(ctx, comp)
	must(t, err)
	club, _, _ := s.CreateClub(ctx, "UCD")
	must(t, s.AttachClub(ctx, club.ID, c.ID))
	ann, _, err := s.Join(ctx, club.ID, "Ann")
	must(t, err)

	must(t, s.AddMemberJudgeQualification(ctx, club.ID, ann.ID, "bg-trampoline-judge-2", "application/pdf", certificate(100)))
	other, _, _ := s.CreateClub(ctx, "TCD")
	if err := s.AddMemberJudgeQualification(ctx, other.ID, ann.ID, "bg-trampoline-judge-2", "application/pdf", certificate(100)); !errors.Is(err, ErrNotFound) {
		t.Errorf("only by her own club: %v", err)
	}
	for range MaxQualifications - 1 {
		must(t, s.AddMemberJudgeQualification(ctx, club.ID, ann.ID, "gi-tumbling-judge-1", "application/pdf", certificate(10)))
	}
	if err := s.AddMemberJudgeQualification(ctx, club.ID, ann.ID, "gi-dmt-judge-1", "application/pdf", certificate(10)); !errors.Is(err, ErrLimit) {
		t.Errorf("four at most: %v", err)
	}
	qs, _ := s.MemberJudgeQualifications(ctx, ann.ID)
	if len(qs) != MaxQualifications || qs[0].Qualification != "bg-trampoline-judge-2" {
		t.Fatalf("hers: %+v", qs)
	}
	if typ, data, err := s.MemberJudgeCertificate(ctx, ann.ID, qs[0].ID); err != nil || typ != "application/pdf" || len(data) != 100 {
		t.Errorf("its certificate: %s %d %v", typ, len(data), err)
	}
	must(t, s.RemoveMemberJudgeQualification(ctx, ann.ID, qs[3].ID))

	// Her offer, sent with the club's entries, brings them to the organiser.
	must(t, s.SaveMemberOffer(ctx, ann.ID, c.ID, competitions.Offer{Judge: map[string]competitions.JudgeOffer{"": {}}}))
	_, err = s.Send(ctx, club.ID, c.ID, nil)
	must(t, err)
	// An individual's, given on one entry, are on their other too.
	ind, token, err := enter(s, c.ID, entry("Ivy"))
	must(t, err)
	must(t, s.SetIndividualOffer(ctx, token, competitions.Offer{Recorder: true}))
	must(t, s.AddEntryJudgeQualification(ctx, token, "gi-trampoline-judge-1", "image/png", certificate(50)))
	tumbling := entry("Ivy")
	tumbling.Discipline = competitions.Tumbling
	second, _, err := s.AddIndividualEntry(ctx, c.ID, token, tumbling)
	must(t, err)
	if qs, _ := s.EntryJudgeQualifications(ctx, second.ID); len(qs) != 1 {
		t.Errorf("on her other entry: %+v", qs)
	}

	officials, _ := s.Officials(ctx, c.ID)
	byOfficial, err := s.OfficialsJudgeQualifications(ctx, c.ID)
	must(t, err)
	for _, o := range officials {
		// The same as each official's own, read one at a time.
		var own []JudgeQualification
		if o.MemberID != "" {
			own, _ = s.MemberJudgeQualifications(ctx, o.MemberID)
		} else {
			own, _ = s.EntryJudgeQualifications(ctx, o.EntryID)
		}
		if !reflect.DeepEqual(byOfficial[o.ID], own) {
			t.Errorf("%s's, read at once: %+v; one at a time: %+v", o.Name, byOfficial[o.ID], own)
		}
		want := map[string]int{"Ann": 3, "Ivy": 1}[o.Name]
		if len(byOfficial[o.ID]) != want {
			t.Errorf("%s's: %+v", o.Name, byOfficial[o.ID])
		}
		if want > 0 {
			if _, _, err := s.OfficialJudgeCertificate(ctx, c.ID, o.ID, byOfficial[o.ID][0].ID); err != nil {
				t.Errorf("%s's certificate, for the organiser: %v", o.Name, err)
			}
			if _, _, err := s.OfficialJudgeCertificate(ctx, "elsewhere", o.ID, byOfficial[o.ID][0].ID); !errors.Is(err, ErrNotFound) {
				t.Errorf("not another competition's: %v", err)
			}
		}
	}
	if q, _ := s.EntryJudgeQualifications(ctx, ind.ID); len(q) == 1 {
		must(t, s.RemoveEntryJudgeQualification(ctx, token, q[0].ID))
	}
	if q, _ := s.EntryJudgeQualifications(ctx, second.ID); len(q) != 0 {
		t.Error("removed")
	}
}
