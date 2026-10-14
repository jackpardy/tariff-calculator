package store

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

// roleKinds are the kinds of a person's roles, in order.
func roleKinds(t *testing.T, s *Store, token string) []string {
	t.Helper()
	_, roles, err := s.Roles(ctx, token)
	if err != nil {
		t.Fatalf("roles of %s: %v", token, err)
	}
	var out []string
	for _, r := range roles {
		out = append(out, r.Kind)
	}
	return out
}

func TestTieBothWays(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	club, _, _ := s.CreateClub(ctx, "UCD")
	m, mLink, err := s.Join(ctx, club.ID, "Aoife")
	must(t, err)
	c, cLink, err := s.CreateCoach(ctx, club.ID, "Aoife")
	must(t, err)

	// Before: a member and a coach of one name are two people.
	if _, err := s.CoachByLink(ctx, Via(mLink, c.ID)); !errors.Is(err, ErrNotFound) {
		t.Errorf("the member's link doesn't open the coach yet: %v", err)
	}
	if got := roleKinds(t, s, mLink); !slices.Equal(got, []string{RoleMember}) {
		t.Errorf("one role: %v", got)
	}

	if _, err := s.Tie(ctx, mLink, RoleMember, mLink); !errors.Is(err, ErrOwnLink) {
		t.Errorf("a page's own link: %v", err)
	}
	role, err := s.Tie(ctx, mLink, RoleCoach, cLink)
	must(t, err)
	if role.Kind != RoleCoach || role.ID != c.ID {
		t.Errorf("tied the coach: %+v", role)
	}
	if _, err := s.Tie(ctx, cLink, RoleMember, mLink); !errors.Is(err, ErrTied) {
		t.Errorf("already tied, from the other side: %v", err)
	}

	// Both ways: each link opens the other role.
	if got, err := s.CoachByLink(ctx, Via(mLink, c.ID)); err != nil || got.ID != c.ID {
		t.Errorf("the member's link opens the coach: %+v, %v", got, err)
	}
	if got, err := s.MemberByLink(ctx, Via(cLink, m.ID)); err != nil || got.ID != m.ID {
		t.Errorf("the coach's link opens the member: %+v, %v", got, err)
	}
	if got := roleKinds(t, s, Via(cLink, m.ID)); !slices.Equal(got, []string{RoleMember, RoleCoach}) {
		t.Errorf("both roles, from either link: %v", got)
	}
	h, roles, _ := s.Roles(ctx, cLink)
	if h.Kind != RoleCoach || !roles[1].Own || roles[0].Own {
		t.Errorf("the coach's link is the coach's own: %+v %+v", h, roles)
	}

	// A Via never opens anyone else's records.
	other, oLink, _ := s.Join(ctx, club.ID, "Brian")
	if _, err := s.MemberByLink(ctx, Via(mLink, other.ID)); !errors.Is(err, ErrNotFound) {
		t.Errorf("someone else's record: %v", err)
	}
	if _, err := s.MemberByLink(ctx, Via("nonsense", m.ID)); !errors.Is(err, ErrNotFound) {
		t.Errorf("a wrong link: %v", err)
	}

	// A replaced personal link is the same person: it keeps the tie.
	newLink, err := s.ReplaceMemberLink(ctx, club.ID, m.ID)
	must(t, err)
	if _, err := s.CoachByLink(ctx, Via(mLink, c.ID)); !errors.Is(err, ErrNotFound) {
		t.Errorf("the old link ends: %v", err)
	}
	if _, err := s.CoachByLink(ctx, Via(newLink, c.ID)); err != nil {
		t.Errorf("the new link opens the coach: %v", err)
	}

	// Undone from either side; a record can't be taken off its own page.
	if err := s.Untie(ctx, cLink, RoleCoach, c.ID); !errors.Is(err, ErrOwnLink) {
		t.Errorf("the coach's own record: %v", err)
	}
	if err := s.Untie(ctx, oLink, RoleMember, m.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("someone else can't undo it: %v", err)
	}
	must(t, s.Untie(ctx, cLink, RoleMember, m.ID))
	if _, err := s.CoachByLink(ctx, Via(newLink, c.ID)); !errors.Is(err, ErrNotFound) {
		t.Errorf("undone: %v", err)
	}
	if _, err := s.MemberByLink(ctx, Via(cLink, m.ID)); !errors.Is(err, ErrNotFound) {
		t.Errorf("undone both ways: %v", err)
	}

	// Tied again, then the coach removed: the member's link no longer opens it.
	must2(s.Tie(ctx, cLink, RoleMember, newLink))
	must(t, s.RemoveCoach(ctx, club.ID, c.ID))
	if got := roleKinds(t, s, newLink); !slices.Equal(got, []string{RoleMember}) {
		t.Errorf("a removed coach is gone from the member's roles: %v", got)
	}
}

func TestSharedTiesOneWay(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	club, admin, _ := s.CreateClub(ctx, "UCD")
	m, mLink, _ := s.Join(ctx, club.ID, "Ciara")
	c, cLink, _ := s.CreateCoach(ctx, club.ID, "Ciara")
	must2(s.Tie(ctx, mLink, RoleCoach, cLink))

	if _, err := s.ClubByAdmin(ctx, Via(mLink, club.ID)); !errors.Is(err, ErrNotFound) {
		t.Errorf("not before it's added: %v", err)
	}
	role, err := s.Tie(ctx, cLink, RoleClub, admin)
	must(t, err)
	if role.Kind != RoleClub || role.ID != club.ID || role.Place != "UCD" {
		t.Errorf("tied the club: %+v", role)
	}
	if _, err := s.Tie(ctx, mLink, RoleClub, admin); !errors.Is(err, ErrTied) {
		t.Errorf("already on the person's page: %v", err)
	}
	// Any of the person's links opens it, through the record it was added on.
	for _, link := range []string{mLink, cLink, Via(mLink, c.ID)} {
		if got, err := s.ClubByAdmin(ctx, Via(Personal(link), club.ID)); err != nil || got.ID != club.ID {
			t.Errorf("%s opens the comp sec's pages: %+v, %v", link, got, err)
		}
	}
	if got := roleKinds(t, s, mLink); !slices.Equal(got, []string{RoleMember, RoleCoach, RoleClub}) {
		t.Errorf("the comp sec role: %v", got)
	}
	if ties, err := s.ClubTies(ctx, club.ID); err != nil || len(ties) != 1 || ties[0].Name != "Ciara" {
		t.Errorf("the comp sec sees who has it: %+v, %v", ties, err)
	}

	// Never the other way: the admin link opens no one's personal pages.
	if _, err := s.MemberByLink(ctx, Via(admin, m.ID)); !errors.Is(err, ErrNotFound) {
		t.Errorf("the admin link doesn't open the member: %v", err)
	}
	if _, err := s.CoachByLink(ctx, Via(admin, c.ID)); !errors.Is(err, ErrNotFound) {
		t.Errorf("or the coach: %v", err)
	}
	if _, _, err := s.Roles(ctx, admin); !errors.Is(err, ErrNotFound) {
		t.Errorf("an admin link has no person: %v", err)
	}
	if _, err := s.Tie(ctx, admin, RoleMember, mLink); !errors.Is(err, ErrNotFound) {
		t.Errorf("nothing can be added to an admin link: %v", err)
	}
	if _, err := s.ClubByAdmin(ctx, Via(admin, club.ID)); !errors.Is(err, ErrNotFound) {
		t.Errorf("an admin link is no Via: %v", err)
	}

	// Replacing the admin link ends what it gave; adding the new one gives it again.
	newAdmin, err := s.ReplaceClubAdmin(ctx, club.ID)
	must(t, err)
	if _, err := s.ClubByAdmin(ctx, Via(mLink, club.ID)); !errors.Is(err, ErrNotFound) {
		t.Errorf("a replaced admin link ends the tie: %v", err)
	}
	if got := roleKinds(t, s, mLink); slices.Contains(got, RoleClub) {
		t.Errorf("and the role: %v", got)
	}
	if ties, _ := s.ClubTies(ctx, club.ID); len(ties) != 0 {
		t.Errorf("no one has the new link yet: %+v", ties)
	}
	must2(s.Tie(ctx, cLink, RoleClub, newAdmin))
	if _, err := s.ClubByAdmin(ctx, Via(mLink, club.ID)); err != nil {
		t.Errorf("the new link, added: %v", err)
	}

	// The comp sec can take it off someone's page; so can they.
	ties, _ := s.ClubTies(ctx, club.ID)
	must(t, s.RemoveClubTie(ctx, club.ID, ties[0].ID))
	if _, err := s.ClubByAdmin(ctx, Via(mLink, club.ID)); !errors.Is(err, ErrNotFound) {
		t.Errorf("taken off by the comp sec: %v", err)
	}
	role, _ = s.Tie(ctx, mLink, RoleClub, newAdmin)
	_, roles, _ := s.Roles(ctx, cLink)
	must(t, s.Untie(ctx, cLink, RoleClub, roles[len(roles)-1].TieID))
	if _, err := s.ClubByAdmin(ctx, Via(mLink, club.ID)); !errors.Is(err, ErrNotFound) {
		t.Errorf("taken off from their own page: %v", err)
	}

	// Undoing a person's tie leaves a shared link with the record it was added on.
	must2(s.Tie(ctx, cLink, RoleClub, newAdmin))
	must(t, s.Untie(ctx, mLink, RoleCoach, c.ID))
	if _, err := s.ClubByAdmin(ctx, Via(mLink, club.ID)); !errors.Is(err, ErrNotFound) {
		t.Errorf("the member no longer has it: %v", err)
	}
	if _, err := s.ClubByAdmin(ctx, Via(cLink, club.ID)); err != nil {
		t.Errorf("the coach, who added it, still does: %v", err)
	}
	// Removing the coach ends what came through them.
	must2(s.Tie(ctx, cLink, RoleMember, mLink))
	must(t, s.RemoveCoach(ctx, club.ID, c.ID))
	if _, err := s.ClubByAdmin(ctx, Via(mLink, club.ID)); !errors.Is(err, ErrNotFound) {
		t.Errorf("removing the coach ends it: %v", err)
	}
}

func TestCompetitionLinkTies(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	c, admin, _ := s.CreateCompetition(ctx, competition())
	club, _, _ := s.CreateClub(ctx, "UCD")
	_, mLink, _ := s.Join(ctx, club.ID, "Dara")
	helper, helperLink, err := s.AddLink(ctx, c.ID, "Difficulty judges", LinkChair)
	must(t, err)

	must2(s.Tie(ctx, mLink, RoleOrganiser, admin))
	role, err := s.Tie(ctx, mLink, RoleOrganiser, helperLink)
	must(t, err)
	if role.Kind != RoleHelper || role.ID != helper.ID || role.Name != "Difficulty judges" {
		t.Errorf("a helper link: %+v", role)
	}
	if got, l, err := s.AdminLink(ctx, Via(mLink, c.ID)); err != nil || got.ID != c.ID || l.Kind != LinkOrganiser {
		t.Errorf("the organiser's pages: %v %+v %v", got.ID, l, err)
	}
	if _, l, err := s.AdminLink(ctx, Via(mLink, helper.ID)); err != nil || l.Kind != LinkChair || l.ID != helper.ID {
		t.Errorf("the helper's pages: %+v %v", l, err)
	}
	if ties, err := s.CompetitionTies(ctx, c.ID); err != nil || len(ties) != 2 {
		t.Errorf("the organiser sees both: %+v, %v", ties, err)
	}

	must(t, s.RemoveLink(ctx, c.ID, helper.ID))
	if _, _, err := s.AdminLink(ctx, Via(mLink, helper.ID)); !errors.Is(err, ErrNotFound) {
		t.Errorf("a removed helper link ends: %v", err)
	}
	must2(s.ReplaceCompetitionAdmin(ctx, c.ID))
	if _, _, err := s.AdminLink(ctx, Via(mLink, c.ID)); !errors.Is(err, ErrNotFound) {
		t.Errorf("a replaced admin link ends: %v", err)
	}
	if got := roleKinds(t, s, mLink); !slices.Equal(got, []string{RoleMember}) {
		t.Errorf("neither is a role now: %v", got)
	}
}

func TestMakeCoach(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	club, _, _ := s.CreateClub(ctx, "UCD")
	m, mLink, _ := s.Join(ctx, club.ID, "Eoin")
	c, err := s.MakeCoach(ctx, club.ID, m.ID)
	must(t, err)
	if c.Name != "Eoin" {
		t.Errorf("named as the member: %+v", c)
	}
	if got, err := s.CoachByLink(ctx, Via(mLink, c.ID)); err != nil || got.ID != c.ID {
		t.Errorf("the member's link opens the coach's page: %+v, %v", got, err)
	}
	if _, err := s.MakeCoach(ctx, club.ID, m.ID); !errors.Is(err, ErrTied) {
		t.Errorf("a coach already: %v", err)
	}
	coaches, _ := s.Coaches(ctx, club.ID)
	members, _ := s.Members(ctx, club.ID)
	if len(coaches) != 1 || coaches[0].Member != "Eoin" || len(members) != 1 || members[0].AlsoCoach != c.ID {
		t.Errorf("each list says so: %+v %+v", coaches, members)
	}
	other, _, _ := s.CreateClub(ctx, "TCD")
	if _, err := s.MakeCoach(ctx, other.ID, m.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("only a club's own member: %v", err)
	}
}

func TestIndividualsHaveOneLink(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	c, _, _ := s.CreateCompetition(ctx, competition())
	dmt := entry("Fiadh O'Neill")
	dmt.Discipline = "dmt"

	first, link, err := s.AddIndividualEntry(ctx, c.ID, "", entry("Fiadh O'Neill"))
	must(t, err)
	if link == "" || first.IndividualID == "" || first.PersonKey != "i:"+first.IndividualID {
		t.Fatalf("someone new, with a link and a key of their own: %q %+v", link, first)
	}
	second, again, err := s.AddIndividualEntry(ctx, c.ID, link, dmt)
	must(t, err)
	if again != "" || second.IndividualID != first.IndividualID || second.PersonKey != first.PersonKey {
		t.Errorf("another discipline under the same link and key: %q %+v", again, second)
	}
	ind, err := s.IndividualByLink(ctx, link)
	must(t, err)
	if mine, err := s.IndividualEntries(ctx, ind.PersonID); err != nil || len(mine) != 2 {
		t.Errorf("both on their page: %+v, %v", mine, err)
	}
	if got, err := s.IndividualEntry(ctx, Via(link, second.ID)); err != nil || got.ID != second.ID {
		t.Errorf("each opened through their link: %+v, %v", got, err)
	}
	if _, err := s.IndividualEntry(ctx, link); !errors.Is(err, ErrNotFound) {
		t.Errorf("their link alone is no one entry: %v", err)
	}

	// Someone else of the same name is someone else.
	twin, twinLink, _ := s.AddIndividualEntry(ctx, c.ID, "", entry("Fiadh O'Neill"))
	if twin.PersonKey == first.PersonKey {
		t.Errorf("two people of one name have two keys: %q", twin.PersonKey)
	}
	if _, err := s.IndividualEntry(ctx, Via(twinLink, first.ID)); !errors.Is(err, ErrNotFound) {
		t.Errorf("and can't open each other's entries: %v", err)
	}

	// A member entering on their own is the same person, with a new role.
	club, _, _ := s.CreateClub(ctx, "UCD")
	_, mLink, _ := s.Join(ctx, club.ID, "Gráinne")
	got, made, err := s.AddIndividualEntry(ctx, c.ID, mLink, entry("Gráinne"))
	must(t, err)
	if made != "" {
		t.Errorf("no new link for a member: %q", made)
	}
	if kinds := roleKinds(t, s, mLink); !slices.Equal(kinds, []string{RoleMember, RoleIndividual}) {
		t.Errorf("the member enters on their own too: %v", kinds)
	}
	if _, err := s.IndividualEntry(ctx, Via(mLink, got.ID)); err != nil {
		t.Errorf("opened through the member's link: %v", err)
	}
}

func TestIndividualsFromEntries(t *testing.T) {
	dir := t.TempDir()
	all := migrations
	migrations = all[:29] // before one link a person
	old, err := Open(ctx, dir)
	if err != nil {
		migrations = all
		t.Fatal(err)
	}
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	old.now = func() time.Time { return now }
	c, _, err := old.CreateCompetition(ctx, competition())
	must(t, err)
	// Two entries by Ann, one misspelt, and one by another Ann.
	for _, e := range []struct{ id, gymnast, token, discipline string }{
		{"e1", "Ann Ó Sé", "t1", ""}, {"e2", "anne ó sé", "t2", "dmt"}, {"e3", "ANN  Ó SÉ", "t3", "tumbling"},
	} {
		_, err := old.db.Exec(`INSERT INTO entries (id, competition_id, club_name, individual, gymnast, token_hash, entry, sent_at, discipline, entered_at)
			VALUES ($1, $2, '', TRUE, $3, $4, '{"gymnast":"x","level":"BUCS L3"}', '2027-01-02T00:00:00.000000Z', $5, '2027-01-0`+e.id[1:]+`T00:00:00.000000Z')`,
			e.id, c.ID, e.gymnast, hash(e.token), e.discipline)
		must(t, err)
	}
	// What the competition saved under the misspelt name's key.
	_, err = old.db.Exec(`UPDATE competitions SET timetable = '{"seats":{"f1":["i:anne ó sé"]}}' WHERE id = $1`, c.ID)
	must(t, err)
	_, err = old.db.Exec(`INSERT INTO scratch_clears (competition_id, person, kind, who, at) VALUES ($1, 'i:anne ó sé', 'officiate', 'Desk', '')`, c.ID)
	must(t, err)
	old.Close()
	migrations = all

	s, err := Open(ctx, dir)
	must(t, err)
	defer s.Close()
	s.now = func() time.Time { return now }
	list, err := s.Entries(ctx, c.ID)
	must(t, err)
	keys := map[string]string{}
	inds := map[string]bool{}
	for _, e := range list {
		keys[e.ID] = e.PersonKey
		inds[e.IndividualID] = true
	}
	if keys["e1"] != "i:ann ó sé" || keys["e3"] != "i:ann ó sé" || keys["e2"] != "i:anne ó sé" {
		t.Errorf("each keeps the key it had, by name: %v", keys)
	}
	if len(inds) != 3 || inds[""] {
		t.Errorf("each entry is an individual of its own: %v", inds)
	}

	// The entry links still work, each opening its own entry and person.
	for id, token := range map[string]string{"e1": "t1", "e2": "t2", "e3": "t3"} {
		if e, err := s.IndividualEntry(ctx, token); err != nil || e.ID != id {
			t.Errorf("%s still opens %s: %+v, %v", token, id, e, err)
		}
		if _, err := s.IndividualByLink(ctx, token); err != nil {
			t.Errorf("%s opens its person's page: %v", token, err)
		}
	}
	if _, err := s.IndividualEntry(ctx, Via("t1", "e2")); !errors.Is(err, ErrNotFound) {
		t.Errorf("not each other's, until added: %v", err)
	}

	// Ann adds her misspelt entry's link: one person, with one key, and
	// what was saved under the other key follows her.
	must2(s.Tie(ctx, "t1", RoleIndividual, "t2"))
	if _, err := s.IndividualEntry(ctx, Via("t1", "e2")); err != nil {
		t.Errorf("one person now: %v", err)
	}
	list, _ = s.Entries(ctx, c.ID)
	for _, e := range list {
		if e.PersonKey != "i:ann ó sé" {
			t.Errorf("%s has the key entered first: %q", e.ID, e.PersonKey)
		}
	}
	var timetable, person string
	must(t, s.db.QueryRow(`SELECT timetable FROM competitions WHERE id = $1`, c.ID).Scan(&timetable))
	must(t, s.db.QueryRow(`SELECT person FROM scratch_clears WHERE competition_id = $1`, c.ID).Scan(&person))
	if !strings.Contains(timetable, `"i:ann ó sé"`) || strings.Contains(timetable, "anne") || person != "i:ann ó sé" {
		t.Errorf("saved keys follow: %s %q", timetable, person)
	}

	// Undone, the misspelt entry is someone else, with a key of its own.
	ind, _ := s.IndividualByLink(ctx, "t2")
	must(t, s.Untie(ctx, "t1", RoleIndividual, ind.ID))
	list, _ = s.Entries(ctx, c.ID)
	for _, e := range list {
		if (e.ID == "e2") == (e.PersonKey == "i:ann ó sé") {
			t.Errorf("%s after undoing: %q", e.ID, e.PersonKey)
		}
	}
}
