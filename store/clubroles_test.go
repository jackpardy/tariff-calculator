package store

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"
)

// The club admin makes roles and a link for each person with one; the link
// opens the club as its role, and a change to the role counts at once.
func TestClubRolesAndLinks(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	club, admin, err := s.CreateClub(ctx, "UCD")
	must(t, err)
	other, _, err := s.CreateClub(ctx, "DCU")
	must(t, err)

	if roles, err := s.ClubRoles(ctx, club.ID); err != nil || len(roles) != 0 {
		t.Fatalf("a club starts with no roles: %+v, %v", roles, err)
	}
	if got, l, err := s.ClubByLink(ctx, admin); err != nil || got.ID != club.ID || !l.Admin() || l.Who() != "Club admin" || !l.Can(ClubSettings) {
		t.Errorf("the admin link is the club admin's: %+v, %+v, %v", got, l, err)
	}

	treasurer, err := s.AddClubRole(ctx, club.ID, "Treasurer", []string{ClubFees, "unknown", ClubFees})
	must(t, err)
	if !slices.Equal(treasurer.Permissions, []string{ClubFees}) {
		t.Errorf("only known permissions, once: %v", treasurer.Permissions)
	}
	manager, err := s.AddClubRole(ctx, club.ID, "Team manager", []string{ClubOrganiser, ClubEnter})
	must(t, err)
	if !slices.Equal(manager.Permissions, []string{ClubEnter, ClubOrganiser}) {
		t.Errorf("in the list's order: %v", manager.Permissions)
	}

	mary, token, err := s.AddClubLink(ctx, club.ID, treasurer.ID, "Mary")
	must(t, err)
	if _, _, err := s.AddClubLink(ctx, club.ID, "no-such-role", "Ann"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a role that isn't there: %v", err)
	}
	if _, _, err := s.AddClubLink(ctx, other.ID, treasurer.ID, "Ann"); !errors.Is(err, ErrNotFound) {
		t.Errorf("another club's role: %v", err)
	}
	got, l, err := s.ClubByLink(ctx, token)
	must(t, err)
	if got.ID != club.ID || l.ID != mary.ID || l.Admin() || l.Who() != "Mary (Treasurer)" || !l.Can(ClubFees) || l.Can(ClubSend) {
		t.Errorf("Mary's link opens the club as Treasurer: %+v, %+v", got, l)
	}
	if c, err := s.ClubByAdmin(ctx, token); err != nil || c.ID != club.ID {
		t.Errorf("ClubByAdmin finds the club by either: %+v, %v", c, err)
	}

	// A change to the role counts at once, for every link with it.
	must(t, s.SaveClubRole(ctx, club.ID, treasurer.ID, "Money", []string{ClubFees, ClubSend}))
	if _, l, err := s.ClubByLink(ctx, token); err != nil || !l.Can(ClubSend) || l.Who() != "Mary (Money)" {
		t.Errorf("the role's change counts at once: %+v, %v", l, err)
	}
	if err := s.SaveClubRole(ctx, other.ID, treasurer.ID, "Mine", nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("another club can't change it: %v", err)
	}

	// Moving the link to another role.
	must(t, s.SetClubLinkRole(ctx, club.ID, mary.ID, manager.ID))
	if _, l, _ := s.ClubByLink(ctx, token); l.Role != "Team manager" || l.Can(ClubFees) {
		t.Errorf("moved: %+v", l)
	}
	if err := s.SetClubLinkRole(ctx, club.ID, mary.ID, "no-such-role"); !errors.Is(err, ErrNotFound) {
		t.Errorf("not to a role that isn't there: %v", err)
	}

	// A role with links can't be removed; once they've moved, it can.
	if err := s.RemoveClubRole(ctx, club.ID, manager.ID); !errors.Is(err, ErrInUse) {
		t.Errorf("a role with links stays: %v", err)
	}
	roles, err := s.ClubRoles(ctx, club.ID)
	must(t, err)
	if len(roles) != 2 || roles[0].Name != "Money" || roles[0].Links != 0 || roles[1].Links != 1 {
		t.Errorf("the roles, with their links: %+v", roles)
	}
	must(t, s.SetClubLinkRole(ctx, club.ID, mary.ID, treasurer.ID))
	must(t, s.RemoveClubRole(ctx, club.ID, manager.ID))

	links, err := s.ClubLinks(ctx, club.ID)
	must(t, err)
	if len(links) != 1 || links[0].Name != "Mary" || links[0].Role != "Money" {
		t.Errorf("the links: %+v", links)
	}

	// Removing a link stops it at once.
	must(t, s.RemoveClubLink(ctx, club.ID, mary.ID))
	if _, _, err := s.ClubByLink(ctx, token); !errors.Is(err, ErrNotFound) {
		t.Errorf("a removed link stops: %v", err)
	}
	if err := s.RemoveClubLink(ctx, club.ID, mary.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("removed already: %v", err)
	}
}

// A club has at most MaxClubRoles roles and MaxClubLinks links.
func TestClubRoleLimits(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	club, _, err := s.CreateClub(ctx, "UCD")
	must(t, err)
	var role ClubRole
	for i := range MaxClubRoles {
		role, err = s.AddClubRole(ctx, club.ID, fmt.Sprintf("Role %d", i), nil)
		must(t, err)
	}
	if _, err := s.AddClubRole(ctx, club.ID, "One more", nil); !errors.Is(err, ErrLimit) {
		t.Errorf("roles: %v", err)
	}
	for i := range MaxClubLinks {
		_, _, err := s.AddClubLink(ctx, club.ID, role.ID, fmt.Sprintf("Person %d", i))
		must(t, err)
	}
	if _, _, err := s.AddClubLink(ctx, club.ID, role.ID, "One more"); !errors.Is(err, ErrLimit) {
		t.Errorf("links: %v", err)
	}
}

// A person can add a role's link to their own page, which then opens the
// club as that role, while the link is there.
func TestClubLinkTies(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	club, admin, err := s.CreateClub(ctx, "UCD")
	must(t, err)
	_, mLink, err := s.Join(ctx, club.ID, "Mary")
	must(t, err)
	role, err := s.AddClubRole(ctx, club.ID, "Treasurer", []string{ClubFees})
	must(t, err)
	l, token, err := s.AddClubLink(ctx, club.ID, role.ID, "Mary")
	must(t, err)

	tied, err := s.Tie(ctx, mLink, RoleClub, token)
	must(t, err)
	if tied.Kind != RoleClubLink || tied.ID != l.ID || tied.Name != "Treasurer" || tied.Place != "UCD" {
		t.Errorf("tied the role: %+v", tied)
	}
	if _, err := s.Tie(ctx, mLink, RoleClub, token); !errors.Is(err, ErrTied) {
		t.Errorf("already there: %v", err)
	}
	got, opened, err := s.ClubByLink(ctx, Via(mLink, l.ID))
	if err != nil || got.ID != club.ID || opened.ID != l.ID || !opened.Can(ClubFees) || opened.Can(ClubSend) {
		t.Errorf("her link opens the club as Treasurer: %+v, %+v, %v", got, opened, err)
	}
	if _, _, err := s.ClubByLink(ctx, Via(mLink, club.ID)); !errors.Is(err, ErrNotFound) {
		t.Errorf("but not as the club admin: %v", err)
	}
	if got := roleKinds(t, s, mLink); !slices.Equal(got, []string{RoleMember, RoleClubLink}) {
		t.Errorf("the role: %v", got)
	}

	// The admin link too: then both.
	must2(s.Tie(ctx, mLink, RoleClub, admin))
	if _, opened, err := s.ClubByLink(ctx, Via(mLink, club.ID)); err != nil || !opened.Admin() {
		t.Errorf("the club admin's pages: %+v, %v", opened, err)
	}

	// Removing the link ends it, on her page too.
	must(t, s.RemoveClubLink(ctx, club.ID, l.ID))
	if _, _, err := s.ClubByLink(ctx, Via(mLink, l.ID)); !errors.Is(err, ErrNotFound) {
		t.Errorf("removed: %v", err)
	}
	if got := roleKinds(t, s, mLink); slices.Contains(got, RoleClubLink) {
		t.Errorf("gone from her roles: %v", got)
	}
	var n int
	must(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM shared_ties WHERE kind = 'clublink'`).Scan(&n))
	if n != 0 {
		t.Errorf("its tie is dropped: %d", n)
	}
}

// The club's history keeps the latest MaxClubHistory changes, latest first.
func TestClubHistory(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	club, _, err := s.CreateClub(ctx, "UCD")
	must(t, err)
	other, _, err := s.CreateClub(ctx, "DCU")
	must(t, err)
	must(t, s.RecordClub(ctx, other.ID, "Club admin", "Not UCD's"))
	for i := range MaxClubHistory + 5 {
		must(t, s.RecordClub(ctx, club.ID, "Mary (Treasurer)", fmt.Sprintf("Change %d", i)))
	}
	got, err := s.ClubHistory(ctx, club.ID, MaxClubHistory+10)
	must(t, err)
	if len(got) != MaxClubHistory || got[0].What != fmt.Sprintf("Change %d", MaxClubHistory+4) || got[0].Who != "Mary (Treasurer)" || !got[0].At.Equal(now) {
		t.Errorf("%d kept, latest first: %+v", len(got), got[0])
	}
	if theirs, _ := s.ClubHistory(ctx, other.ID, 10); len(theirs) != 1 {
		t.Errorf("another club's stays: %+v", theirs)
	}
}

// Deleting a club takes its roles, links and history with it.
func TestDeletingAClubTakesItsRoles(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	club, _, err := s.CreateClub(ctx, "UCD")
	must(t, err)
	role, err := s.AddClubRole(ctx, club.ID, "Treasurer", []string{ClubFees})
	must(t, err)
	_, token, err := s.AddClubLink(ctx, club.ID, role.ID, "Mary")
	must(t, err)
	must(t, s.RecordClub(ctx, club.ID, "Club admin", "Made a role"))
	must(t, s.DeleteClub(ctx, club.ID))
	if _, _, err := s.ClubByLink(ctx, token); !errors.Is(err, ErrNotFound) {
		t.Errorf("the link goes: %v", err)
	}
	for _, table := range []string{"club_roles", "club_links", "club_log"} {
		var n int
		must(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&n))
		if n != 0 {
			t.Errorf("%s: %d left", table, n)
		}
	}
}

// Migration 50 starts each club with no roles, links or history, and its
// admin link the club admin's.
func TestClubRolesMigrate(t *testing.T) {
	all := migrations
	t.Cleanup(func() { migrations = all })
	migrations = all[:50-1]
	dir := t.TempDir()
	s, err := Open(ctx, dir)
	must(t, err)
	club, admin, err := s.CreateClub(ctx, "UCD")
	must(t, err)
	migrations = all
	s = reopen(t, s, dir)
	if roles, err := s.ClubRoles(ctx, club.ID); err != nil || len(roles) != 0 {
		t.Errorf("no roles: %+v, %v", roles, err)
	}
	if links, err := s.ClubLinks(ctx, club.ID); err != nil || len(links) != 0 {
		t.Errorf("no links: %+v, %v", links, err)
	}
	if got, l, err := s.ClubByLink(ctx, admin); err != nil || got.ID != club.ID || !l.Admin() {
		t.Errorf("the admin link is the club admin's: %+v, %v", l, err)
	}
	if h, err := s.ClubHistory(ctx, club.ID, 10); err != nil || len(h) != 0 {
		t.Errorf("no history: %+v, %v", h, err)
	}
}
