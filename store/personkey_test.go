package store

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestPersonKeys(t *testing.T) {
	for _, c := range []struct{ key, kind, id string }{
		{MemberKey("abc"), KindMember, "abc"},
		{IndividualKey("abc"), KindIndividual, "abc"},
		{IndividualNameKey("  Ann  SMITH "), KindIndividual, "ann smith"},
		{CoachKey("abc"), KindCoach, "abc"},
		{NamedCoachKey("Bob"), KindCoach, ":bob"},
		{OfficialKey("abc"), KindOfficial, "abc"},
		{PartnerKey("abc"), KindPartner, "abc"},
	} {
		if kind, id := KeyKind(c.key); kind != c.kind || id != c.id {
			t.Errorf("%q: %q %q, want %q %q", c.key, kind, id, c.kind, c.id)
		}
	}
	if NamedCoachKey(" ") != "" {
		t.Error("no name has no key")
	}
	// An individual's coach, by name, is never a club coach's key.
	if NamedCoachKey("x") == CoachKey("x") {
		t.Error("a named coach's key is a club coach's")
	}
}

// Every column a competition saves person keys in is one renameKey changes:
// columns named for a person or people hold keys, as do the timetables and
// simulations (whose seats and rules name people). A new one fails here
// until it's added to keyedJSON or keyedRows.
func TestRenameKeyCoversEveryKeyColumn(t *testing.T) {
	s := open(t, nil)
	ctx := context.Background()
	covered := map[string]bool{}
	for _, c := range keyedJSON {
		covered[c.table+"."+c.column] = true
	}
	for _, r := range keyedRows {
		covered[r.table+".person"] = true
	}
	// Ids of people, not keys, and an entry's own key, which joining people
	// changes itself (merge).
	notKeys := regexp.MustCompile(`^(person_id|person_key)$`)
	holdsKeys := regexp.MustCompile(`person|people|timetable|scenarios`)
	tables, err := s.db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table'`)
	must(t, err)
	var names []string
	for tables.Next() {
		var name string
		must(t, tables.Scan(&name))
		names = append(names, name)
	}
	tables.Close()
	found := map[string]bool{}
	for _, table := range names {
		cols, err := s.db.QueryContext(ctx, `SELECT name FROM pragma_table_info($1)`, table)
		must(t, err)
		for cols.Next() {
			var col string
			must(t, cols.Scan(&col))
			found[table+"."+col] = true
			if holdsKeys.MatchString(col) && !notKeys.MatchString(col) && !covered[table+"."+col] {
				t.Errorf("%s.%s may hold person keys, but renameKey doesn't change it", table, col)
			}
		}
		cols.Close()
	}
	for c := range covered {
		if !found[c] {
			t.Errorf("renameKey changes %s, which isn't in the schema", c)
		}
	}
}

// PersonGroups keys a club's coach by their record, the same person as the
// member they were made from.
func TestPersonGroupsCoachByRecord(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	comp, _, err := s.CreateCompetition(ctx, competition())
	must(t, err)
	club, _, _ := s.CreateClub(ctx, "UCD")
	must(t, s.AttachClub(ctx, club.ID, comp.ID))
	m, _, _ := s.Join(ctx, club.ID, "Eoin")
	c, err := s.MakeCoach(ctx, club.ID, m.ID)
	must(t, err)
	other, _, _ := s.CreateCoach(ctx, club.ID, "Eoin")
	groups, err := s.PersonGroups(ctx, comp.ID)
	must(t, err)
	if groups[CoachKey(c.ID)] == "" || groups[CoachKey(c.ID)] != groups[MemberKey(m.ID)] {
		t.Errorf("the coach made from Eoin is Eoin: %v", groups)
	}
	if groups[CoachKey(other.ID)] == groups[MemberKey(m.ID)] {
		t.Errorf("another coach of the same name isn't: %v", groups)
	}
}

// An entry's coach is keyed by the coach's record, whatever name is typed.
func TestEntryCoachKey(t *testing.T) {
	signed := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		name string
		e    Entry
		want string
	}{
		{"the member's coach", Entry{ClubID: "club", CoachID: "c1", Coach: "Pat"}, CoachKey("c1")},
		{"the coach who signed it off", Entry{ClubID: "club", CoachID: "c1", SignedAt: signed, SignedBy: "Patricia", SignedCoach: "c2"}, CoachKey("c2")},
		{"a sign-off from before coaches were recorded", Entry{ClubID: "club", SignedAt: signed, SignedBy: "Pat"}, "c:club:pat"},
		{"an unapproved sign-off", Entry{ClubID: "club", CoachID: "c1", SignedAt: signed, SignedCoach: "c2", Unapproved: true}, CoachKey("c1")},
		{"no coach", Entry{ClubID: "club"}, ""},
		{"an individual's", Entry{Individual: true, SignedAt: signed, SignedBy: "Pat Kelly"}, NamedCoachKey("pat kelly")},
	} {
		if got := c.e.CoachKey(); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

// Migration 34 moves the keys saved for a club's coaches, by club and name,
// to their records, unless two coaches had the key.
func TestCoachKeysByRecord(t *testing.T) {
	dir := t.TempDir()
	all := migrations
	migrations = all[:33]
	old, err := Open(ctx, dir)
	if err != nil {
		migrations = all
		t.Fatal(err)
	}
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	old.now = func() time.Time { return now }
	comp, _, err := old.CreateCompetition(ctx, competition())
	must(t, err)
	club, _, _ := old.CreateClub(ctx, "UCD")
	must(t, old.AttachClub(ctx, club.ID, comp.ID))
	pat, _, _ := old.CreateCoach(ctx, club.ID, "Pat Kelly")
	old.CreateCoach(ctx, club.ID, "Sam")
	old.CreateCoach(ctx, club.ID, "sam")
	patKey, samKey := "c:"+club.ID+":pat kelly", "c:"+club.ID+":sam"
	_, err = old.db.Exec(`UPDATE competitions SET timetable = $1 WHERE id = $2`, `{"rules":[{"person":"`+patKey+`"},{"person":"`+samKey+`"}]}`, comp.ID)
	must(t, err)
	_, err = old.db.Exec(`INSERT INTO desk_messages (id, competition_id, text, audience, people, clubs, who, at) VALUES ('d1', $1, 'Hi', 'Pat', $2, '[]', 'Desk', '')`,
		comp.ID, `["`+patKey+`"]`)
	must(t, err)
	_, err = old.db.Exec(`INSERT INTO scratch_clears (competition_id, person, kind, who, at) VALUES ($1, $2, 'officiate', 'Desk', '')`, comp.ID, patKey)
	must(t, err)
	old.Close()
	migrations = all

	s, err := Open(ctx, dir)
	must(t, err)
	defer s.Close()
	var timetable, people, cleared string
	must(t, s.db.QueryRow(`SELECT timetable FROM competitions WHERE id = $1`, comp.ID).Scan(&timetable))
	must(t, s.db.QueryRow(`SELECT people FROM desk_messages WHERE id = 'd1'`).Scan(&people))
	must(t, s.db.QueryRow(`SELECT person FROM scratch_clears WHERE competition_id = $1`, comp.ID).Scan(&cleared))
	want := CoachKey(pat.ID)
	if !strings.Contains(timetable, `"`+want+`"`) || strings.Contains(timetable, patKey) || !strings.Contains(people, want) || cleared != want {
		t.Errorf("Pat's saved keys are by record: %s %s %s", timetable, people, cleared)
	}
	if !strings.Contains(timetable, samKey) {
		t.Errorf("the key two Sams had stays: %s", timetable)
	}
}
