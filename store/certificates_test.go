package store

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

// certificate is a certificate of n bytes.
func certificate(n int) []byte {
	return append([]byte("%PDF-1.4\n"), make([]byte, n-9)...)
}

func TestClubCertificatesHaveATotal(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	club, _, _ := s.CreateClub(ctx, "UCD")
	other, _, _ := s.CreateClub(ctx, "TCD")
	ann, _, _ := s.CreateCoach(ctx, club.ID, "Ann")
	if _, err := s.AddQualification(ctx, club.ID, ann.ID, "bg-trampoline-2", "application/pdf", certificate(MaxCertificateBytes+1)); !errors.Is(err, ErrCertificateTooBig) {
		t.Errorf("3 MB at most: %v", err)
	}

	// 60 MB a club: twenty of the largest, four to a coach.
	full := MaxClubCertificateBytes / MaxCertificateBytes
	var last string
	for i := range full {
		if i%MaxQualifications == 0 {
			c, _, err := s.CreateCoach(ctx, club.ID, fmt.Sprintf("Coach %d", i))
			must(t, err)
			last = c.ID
		}
		if _, err := s.AddQualification(ctx, club.ID, last, "bg-trampoline-2", "application/pdf", certificate(MaxCertificateBytes)); err != nil {
			t.Fatalf("certificate %d: %v", i+1, err)
		}
	}
	extra, _, _ := s.CreateCoach(ctx, club.ID, "One too many")
	if _, err := s.AddQualification(ctx, club.ID, extra.ID, "bg-trampoline-2", "application/pdf", certificate(10)); !errors.Is(err, ErrCertificatesFull) {
		t.Errorf("the club's total: %v", err)
	}
	// Another club has its own.
	o, _, _ := s.CreateCoach(ctx, other.ID, "Bob")
	if _, err := s.AddQualification(ctx, other.ID, o.ID, "bg-trampoline-2", "application/pdf", certificate(10)); err != nil {
		t.Errorf("another club's: %v", err)
	}
	// Removing one makes room.
	qs, _ := s.Qualifications(ctx, club.ID)
	must(t, s.RemoveQualification(ctx, club.ID, qs[last][0].ID))
	if _, err := s.AddQualification(ctx, club.ID, extra.ID, "bg-trampoline-2", "application/pdf", certificate(10)); err != nil {
		t.Errorf("room again: %v", err)
	}
}

func TestCompetitionCertificatesHaveATotal(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	comp := competition()
	comp.Signoff, comp.Individuals = true, true
	c, _, _ := s.CreateCompetition(ctx, comp)
	must(t, s.SetApproveCoaches(ctx, c.ID, true, nil))

	full := MaxCompetitionCertificateBytes / MaxCertificateBytes
	var first string
	for i := range full {
		_, token, err := enter(s, c.ID, entry(fmt.Sprintf("Gymnast %d", i)))
		must(t, err)
		if i == 0 {
			first = token
		}
		if err := s.SetEntryCoach(ctx, token, "Ann", "gi-trampoline-2", "application/pdf", certificate(MaxCertificateBytes)); err != nil {
			t.Fatalf("certificate %d: %v", i+1, err)
		}
	}
	_, token, err := enter(s, c.ID, entry("One too many"))
	must(t, err)
	if err := s.SetEntryCoach(ctx, token, "Ann", "gi-trampoline-2", "application/pdf", certificate(10)); !errors.Is(err, ErrCertificatesFull) {
		t.Errorf("the competition's total: %v", err)
	}
	// Naming another coach replaces the certificate, so it fits.
	if err := s.SetEntryCoach(ctx, first, "Bob", "gi-trampoline-2", "application/pdf", certificate(MaxCertificateBytes)); err != nil {
		t.Errorf("a certificate replaced: %v", err)
	}
	if err := s.SetEntryCoach(ctx, first, "Bob", "gi-trampoline-2", "application/pdf", certificate(MaxCertificateBytes+1)); !errors.Is(err, ErrCertificateTooBig) {
		t.Errorf("3 MB at most: %v", err)
	}
}

func TestNoCertificatesOnANearlyFullDisk(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	if free, err := s.free(); err != nil || free == 0 {
		t.Errorf("the disk's free space is told: %d, %v", free, err)
	}
	club, _, _ := s.CreateClub(ctx, "UCD")
	ann, _, _ := s.CreateCoach(ctx, club.ID, "Ann")
	s.free = func() (uint64, error) { return minFreeBytes - 1, nil }
	if _, err := s.AddQualification(ctx, club.ID, ann.ID, "bg-trampoline-2", "application/pdf", certificate(10)); !errors.Is(err, ErrDiskFull) {
		t.Errorf("a nearly full disk: %v", err)
	}
	comp := competition()
	comp.Signoff, comp.Individuals = true, true
	c, _, _ := s.CreateCompetition(ctx, comp)
	must(t, s.SetApproveCoaches(ctx, c.ID, true, nil))
	_, token, err := enter(s, c.ID, entry("Mary"))
	must(t, err)
	if err := s.SetEntryCoach(ctx, token, "Ann", "gi-trampoline-2", "application/pdf", certificate(10)); !errors.Is(err, ErrDiskFull) {
		t.Errorf("a nearly full disk, for an individual's coach: %v", err)
	}
	s.free = func() (uint64, error) { return minFreeBytes, nil }
	if _, err := s.AddQualification(ctx, club.ID, ann.ID, "bg-trampoline-2", "application/pdf", certificate(10)); err != nil {
		t.Errorf("enough room: %v", err)
	}
}
