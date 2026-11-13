package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
)

func TestFindAndNewAdminLink(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	comp, oldAdmin, err := st.CreateCompetition(ctx, competitions.Competition{
		Name: "Ceilí Open", Date: "2027-03-13", Deadline: time.Date(2027, 3, 6, 23, 59, 0, 0, time.UTC),
		Levels: []competitions.Level{{Ref: "builtin-level:bucs-l3"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	club, oldClub, err := st.CreateClub(ctx, "Cork Trampoline Club")
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := findCommand(ctx, st, []string{"CEILI"}, &out); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, comp.ID) || !strings.Contains(got, "2027-03-13") || strings.Contains(got, club.ID) || strings.Contains(got, oldAdmin) {
		t.Errorf("find by name, case and accents ignored:\n%s", got)
	}
	out.Reset()
	if err := findCommand(ctx, st, []string{"trampoline", "cork"}, &out); err != nil || !strings.Contains(out.String(), "club") || !strings.Contains(out.String(), club.ID) {
		t.Errorf("find a club: %v\n%s", err, out.String())
	}

	// The name typed back wrong changes nothing.
	out.Reset()
	if err := newAdminLinkCommand(ctx, st, []string{"competition", comp.ID}, strings.NewReader("wrong\n"), &out, "https://tariff.example/"); err == nil {
		t.Error("a wrong name went ahead")
	}
	if _, err := st.CompetitionByAdmin(ctx, oldAdmin); err != nil {
		t.Errorf("the old link was changed: %v", err)
	}

	out.Reset()
	if err := newAdminLinkCommand(ctx, st, []string{"competition", comp.ID}, strings.NewReader("Ceilí Open\n"), &out, "https://tariff.example/"); err != nil {
		t.Fatal(err)
	}
	var link string
	for _, l := range strings.Split(out.String(), "\n") {
		if _, after, ok := strings.Cut(l, "https://tariff.example/competitions/admin/"); ok {
			link = "https://tariff.example/competitions/admin/" + after
		}
	}
	if link == "" {
		t.Fatalf("no link printed:\n%s", out.String())
	}
	if _, err := st.CompetitionByAdmin(ctx, oldAdmin); err == nil {
		t.Error("the old link still works")
	}
	if _, err := st.CompetitionByAdmin(ctx, strings.TrimPrefix(link, "https://tariff.example/competitions/admin/")); err != nil {
		t.Errorf("the new link doesn't work: %v", err)
	}
	if h, _ := st.History(ctx, comp.ID, 5); len(h) == 0 || !strings.Contains(h[0].What, "The maintainer replaced the admin link, at the organiser's request") {
		t.Errorf("history: %v", h)
	}

	out.Reset()
	if err := newAdminLinkCommand(ctx, st, []string{"club", club.ID, "--yes"}, strings.NewReader(""), &out, "https://tariff.example"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "https://tariff.example/clubs/admin/") {
		t.Errorf("club link:\n%s", out.String())
	}
	if _, _, err := st.ClubByLink(ctx, oldClub); err == nil {
		t.Error("the old club link still works")
	}
	if h, _ := st.ClubHistory(ctx, club.ID, 5); len(h) == 0 || !strings.Contains(h[0].What, "at the club's request") {
		t.Errorf("club history: %v", h)
	}
	if err := newAdminLinkCommand(ctx, st, []string{"club", "nope", "--yes"}, nil, &out, ""); err == nil {
		t.Error("an unknown id went ahead")
	}
}
