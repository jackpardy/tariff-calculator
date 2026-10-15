package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// One link for a person with several roles (ADR 0010). A person's records
// are their club memberships (members), the coaching they do for clubs
// (coaches) and their entering on their own (individuals), each with its own
// personal link; records with the same person_id are one person. A personal
// link also opens the shared links the person has added to their page (a
// club's admin link, a competition's admin or helper link), kept in
// shared_ties against the record whose page they were added on.
//
// Tokens are only kept as hashes, so a page can't link to another of the
// person's links. Instead a personal link opens the person's other roles at
// their own paths with "{personal token}~{id}" in place of the role's token:
// the member, coach, individual or entry's id, the club's id for its comp
// sec's pages, or the competition's or helper link's id for an admin link.
// Every way of finding a role by its token accepts both. A shared link never
// opens a person: only a personal link's token goes before the "~".

var (
	// ErrOwnLink is a link added to its own page.
	ErrOwnLink = errors.New("that's this page's own link")
	// ErrTied is a link already on the person's page.
	ErrTied = errors.New("that link is already on your page")
)

// viaSep joins a personal link's token and the id of the role it opens.
const viaSep = "~"

// Via is the token by which a personal link opens one of the person's
// other roles, by the role's id.
func Via(personal, id string) string { return personal + viaSep + id }

// splitVia splits a token made by Via into its personal link and the id.
func splitVia(token string) (personal, id string, ok bool) {
	personal, id, ok = strings.Cut(token, viaSep)
	return personal, id, ok && personal != "" && id != ""
}

// IsVia says whether a token is a personal link's way into another role, so
// not a link to show as the role's own.
func IsVia(token string) bool {
	_, _, ok := splitVia(token)
	return ok
}

// Personal is the personal link a token is, or opens a role through.
func Personal(token string) string {
	if personal, _, ok := splitVia(token); ok {
		return personal
	}
	return token
}

// What a person's roles are.
const (
	RoleMember     = "member"     // a club's member
	RoleCoach      = "coach"      // a club's coach
	RoleIndividual = "individual" // entering competitions on their own
	RoleClub       = "club"       // a club's comp sec, by its admin link
	RoleOrganiser  = "organiser"  // a competition's organiser, by its admin link
	RoleHelper     = "helper"     // a competition's helper, by an extra admin link (ADR 0009)
)

// querier is the database or a transaction.
type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Holder is the record a personal link belongs to.
type Holder struct {
	Kind     string // RoleMember, RoleCoach or RoleIndividual
	ID       string
	PersonID string
	// Entry is a link to one of the individual's entries, from before an
	// individual had one link.
	Entry bool
}

// holderOf is the record a personal link (not a Via) belongs to: a member's,
// coach's or individual's, or one of an individual's entry links.
func holderOf(ctx context.Context, q querier, token string) (Holder, error) {
	var h Holder
	err := q.QueryRowContext(ctx, `SELECT 'member', id, COALESCE(person_id, id), FALSE FROM members WHERE token_hash = $1
		UNION ALL SELECT 'coach', id, COALESCE(person_id, id), FALSE FROM coaches WHERE token_hash = $1
		UNION ALL SELECT 'individual', id, person_id, FALSE FROM individuals WHERE token_hash = $1
		UNION ALL SELECT 'individual', i.id, i.person_id, TRUE FROM entries e JOIN individuals i ON i.id = e.individual_id
			WHERE e.token_hash = $1 AND e.individual`, hash(token)).Scan(&h.Kind, &h.ID, &h.PersonID, &h.Entry)
	return h, notFound(err)
}

// HolderOf is the record a personal link belongs to, or opens a role
// through.
func (s *Store) HolderOf(ctx context.Context, token string) (Holder, error) {
	return holderOf(ctx, s.db, Personal(token))
}

// personRecords selects the ids of a person's records, the person being the
// query's parameter n.
func personRecords(n string) string {
	return `SELECT id FROM members WHERE person_id = $` + n + ` UNION ALL SELECT id FROM coaches WHERE person_id = $` + n +
		` UNION ALL SELECT id FROM individuals WHERE person_id = $` + n
}

// recordWhere is the condition finding a member, coach or individual by a
// token: their own link's, or a personal link's Via to them, the same
// person's.
func recordWhere(ctx context.Context, q querier, token string) (string, []any, error) {
	personal, id, ok := splitVia(token)
	if !ok {
		return `token_hash = $1`, []any{hash(token)}, nil
	}
	h, err := holderOf(ctx, q, personal)
	if err != nil {
		return "", nil, err
	}
	return `id = $1 AND person_id = $2`, []any{id, h.PersonID}, nil
}

// individualEntryID is the entry an individual's link opens: one of their
// entry links, or a personal link's Via to one of the person's entries.
func individualEntryID(ctx context.Context, q querier, token string) (string, error) {
	var id string
	var err error
	if personal, entryID, ok := splitVia(token); ok {
		var h Holder
		if h, err = holderOf(ctx, q, personal); err != nil {
			return "", err
		}
		err = q.QueryRowContext(ctx, `SELECT id FROM entries WHERE id = $1 AND individual
			AND individual_id IN (SELECT id FROM individuals WHERE person_id = $2)`, entryID, h.PersonID).Scan(&id)
	} else {
		err = q.QueryRowContext(ctx, `SELECT id FROM entries WHERE token_hash = $1 AND individual`, hash(token)).Scan(&id)
	}
	return id, notFound(err)
}

// Individual is a gymnast entering competitions on their own (ADR 0010).
type Individual struct {
	ID        string
	PersonID  string
	Name      string // as they first entered
	CreatedAt time.Time
}

// IndividualByLink is the individual a link opens: their personal link, one
// of their entry links, or a personal link's Via to them.
func (s *Store) IndividualByLink(ctx context.Context, token string) (Individual, error) {
	var i Individual
	var created string
	where, args, err := recordWhere(ctx, s.db, token)
	if err != nil {
		return i, err
	}
	err = s.db.QueryRowContext(ctx, `SELECT id, person_id, name, created_at FROM individuals WHERE `+where, args...).
		Scan(&i.ID, &i.PersonID, &i.Name, &created)
	if errors.Is(err, sql.ErrNoRows) && !IsVia(token) {
		err = s.db.QueryRowContext(ctx, `SELECT i.id, i.person_id, i.name, i.created_at FROM entries e JOIN individuals i ON i.id = e.individual_id
			WHERE e.token_hash = $1 AND e.individual`, hash(token)).Scan(&i.ID, &i.PersonID, &i.Name, &created)
	}
	i.CreatedAt = parseTime(created)
	return i, notFound(err)
}

// individualFor is the individual entering as the person a link is ("" for
// someone new, named name): the individual it opens, or one of the person's,
// or a new one of theirs (a member entering on their own). It returns the new
// individual's personal link when it makes someone new.
func (s *Store) individualFor(ctx context.Context, tx *sql.Tx, as, name string) (string, string, error) {
	now := s.stamp()
	if as == "" {
		id, token := newID(), newToken()
		_, err := tx.ExecContext(ctx, `INSERT INTO individuals (id, person_id, token_hash, name, created_at) VALUES ($1, $1, $2, $3, $4)`,
			id, hash(token), name, now)
		return id, token, err
	}
	h, err := holderOf(ctx, tx, Personal(as))
	if err != nil {
		return "", "", err
	}
	if h.Kind == RoleIndividual && !IsVia(as) {
		return h.ID, "", nil
	}
	var id string
	err = tx.QueryRowContext(ctx, `SELECT id FROM individuals WHERE person_id = $1 ORDER BY created_at, id LIMIT 1`, h.PersonID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		id = newID()
		_, err = tx.ExecContext(ctx, `INSERT INTO individuals (id, person_id, name, created_at) VALUES ($1, $2, $3, $4)`, id, h.PersonID, name, now)
	}
	return id, "", err
}

// NameKey is a name as compared: in lower case, with single spaces.
func NameKey(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(name), " "))
}

// personKeyFor is the person key of an individual's new entry in a
// competition: the one the person's entries there have, else one of its own.
func personKeyFor(ctx context.Context, tx *sql.Tx, competitionID, individualID string) (string, error) {
	var key string
	err := tx.QueryRowContext(ctx, `SELECT person_key FROM entries WHERE competition_id = $1 AND individual AND person_key <> ''
		AND individual_id IN (SELECT id FROM individuals WHERE person_id = (SELECT person_id FROM individuals WHERE id = $2))
		ORDER BY entered_at, id LIMIT 1`, competitionID, individualID).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return IndividualKey(individualID), nil
	}
	return key, err
}

// individualsFromEntries makes each individual entry from before ADR 0010 an
// individual of its own, with no personal link: it's reached by its entry
// link, and joins the person's other entries when they add that link to
// their page. Its person key stays its name, as it was.
func individualsFromEntries(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT id, gymnast, CASE WHEN entered_at <> '' THEN entered_at ELSE sent_at END
		FROM entries WHERE individual AND individual_id IS NULL`)
	if err != nil {
		return err
	}
	type old struct{ id, name, at string }
	var all []old
	for rows.Next() {
		var o old
		if err := rows.Scan(&o.id, &o.name, &o.at); err != nil {
			rows.Close()
			return err
		}
		all = append(all, o)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, o := range all {
		id := newID()
		if _, err := tx.ExecContext(ctx, `INSERT INTO individuals (id, person_id, name, created_at) VALUES ($1, $1, $2, $3)`, id, o.name, o.at); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE entries SET individual_id = $1, person_key = $2 WHERE id = $3`, id, IndividualNameKey(o.name), o.id); err != nil {
			return err
		}
	}
	return nil
}

// Role is one of a person's roles, as the tabs of their pages show them.
type Role struct {
	Kind  string // RoleMember, RoleCoach, RoleIndividual, RoleClub, RoleOrganiser or RoleHelper
	ID    string // the member, coach or individual; the club; the competition; the helper link
	Name  string // their name in the role, or the helper link's name
	Place string // the club or competition
	// TieID is a shared link's tie, to take it off the page; "" for the
	// person's own records.
	TieID string
	Own   bool // the record whose link this is
}

// Roles are the roles of the person a personal link (or a Via through one)
// belongs to: their own records, then the shared links on their pages that
// still work.
func (s *Store) Roles(ctx context.Context, token string) (Holder, []Role, error) {
	h, err := s.HolderOf(ctx, token)
	if err != nil {
		return h, nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT 'member', m.id, m.name, c.name, '' FROM members m JOIN clubs c ON c.id = m.club_id WHERE m.person_id = $1
		UNION ALL SELECT 'coach', co.id, co.name, c.name, '' FROM coaches co JOIN clubs c ON c.id = co.club_id WHERE co.person_id = $1
		UNION ALL SELECT 'individual', id, name, '', '' FROM individuals WHERE person_id = $1
		UNION ALL SELECT 'club', c.id, '', c.name, t.id FROM shared_ties t JOIN clubs c ON c.id = t.target_id AND c.admin_hash = t.link_hash
			WHERE t.kind = 'club' AND t.record_id IN (`+personRecords("1")+`)
		UNION ALL SELECT 'organiser', c.id, '', c.name, t.id FROM shared_ties t JOIN competitions c ON c.id = t.target_id AND c.admin_hash = t.link_hash
			WHERE t.kind = 'organiser' AND t.record_id IN (`+personRecords("1")+`)
		UNION ALL SELECT 'helper', l.id, l.name, c.name, t.id FROM shared_ties t JOIN competition_links l ON l.id = t.target_id AND l.token_hash = t.link_hash
			JOIN competitions c ON c.id = l.competition_id
			WHERE t.kind = 'helper' AND t.record_id IN (`+personRecords("1")+`)`, h.PersonID)
	if err != nil {
		return h, nil, err
	}
	defer rows.Close()
	var out []Role
	seen := map[string]bool{} // a shared link added on two of the person's pages is one role
	for rows.Next() {
		var r Role
		if err := rows.Scan(&r.Kind, &r.ID, &r.Name, &r.Place, &r.TieID); err != nil {
			return h, nil, err
		}
		if seen[r.Kind+r.ID] {
			continue
		}
		seen[r.Kind+r.ID] = true
		r.Own = r.Kind == h.Kind && r.ID == h.ID
		out = append(out, r)
	}
	return h, out, rows.Err()
}

// Tie adds a link to the page of the person whose personal link token is (or
// opens a role through): "pasted", a link of kind role. Pasting a link proves
// holding it (ADR 0010). Another personal link (a member's, coach's or
// individual's, or an entry link) joins its person to this one, both ways; a
// shared link (RoleClub for a club's admin link, RoleOrganiser for any of a
// competition's admin links) lets this person's links open it, but never the
// other way. It returns the role added.
func (s *Store) Tie(ctx context.Context, token, kind, pasted string) (Role, error) {
	var role Role
	err := s.tx(ctx, func(tx *sql.Tx) error {
		own, err := holderOf(ctx, tx, Personal(token))
		if err != nil {
			return err
		}
		if IsVia(pasted) {
			kind = RoleMember // a Via is a personal link's: its person joins
		}
		switch kind {
		case RoleMember, RoleCoach, RoleIndividual:
			other, err := holderOf(ctx, tx, Personal(pasted))
			if err != nil {
				return err
			}
			switch {
			case other.Kind == own.Kind && other.ID == own.ID:
				return ErrOwnLink
			case other.PersonID == own.PersonID:
				return ErrTied
			}
			role = Role{Kind: other.Kind, ID: other.ID}
			return s.merge(ctx, tx, own.PersonID, other.PersonID)
		case RoleClub:
			var clubID string
			if err := tx.QueryRowContext(ctx, `SELECT id, name FROM clubs WHERE admin_hash = $1`, hash(pasted)).Scan(&clubID, &role.Place); err != nil {
				return notFound(err)
			}
			role.Kind, role.ID = RoleClub, clubID
			return s.addShared(ctx, tx, own, RoleClub, clubID, hash(pasted))
		case RoleOrganiser:
			err := tx.QueryRowContext(ctx, `SELECT id, name FROM competitions WHERE admin_hash = $1`, hash(pasted)).Scan(&role.ID, &role.Place)
			role.Kind = RoleOrganiser
			if errors.Is(err, sql.ErrNoRows) {
				role.Kind = RoleHelper
				err = tx.QueryRowContext(ctx, `SELECT l.id, l.name, c.name FROM competition_links l JOIN competitions c ON c.id = l.competition_id
					WHERE l.token_hash = $1`, hash(pasted)).Scan(&role.ID, &role.Name, &role.Place)
			}
			if err != nil {
				return notFound(err)
			}
			return s.addShared(ctx, tx, own, role.Kind, role.ID, hash(pasted))
		}
		return ErrNotFound
	})
	return role, err
}

// addShared ties a shared link to the record whose page it was added on,
// unless the person already has it. A tie to a link replaced since takes the
// new one.
func (s *Store) addShared(ctx context.Context, tx *sql.Tx, own Holder, kind, target, linkHash string) error {
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM shared_ties WHERE kind = $1 AND target_id = $2 AND link_hash = $3
		AND record_id IN (`+personRecords("4")+`)`, kind, target, linkHash, own.PersonID).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrTied
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO shared_ties (id, record_id, kind, target_id, link_hash, created_at) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (record_id, kind, target_id) DO UPDATE SET link_hash = excluded.link_hash, created_at = excluded.created_at`,
		newID(), own.ID, kind, target, linkHash, s.stamp())
	return err
}

// merge makes person from part of person into, and gives the person one key
// in each competition they've entered on their own.
func (s *Store) merge(ctx context.Context, tx *sql.Tx, into, from string) error {
	for _, table := range []string{"members", "coaches", "individuals"} {
		if _, err := tx.ExecContext(ctx, `UPDATE `+table+` SET person_id = $1 WHERE person_id = $2`, into, from); err != nil {
			return err
		}
	}
	// Each competition where they now have more than one key keeps the one
	// entered first; the others' saved keys become it, unless someone else
	// (another individual of the same name, from before) still has them.
	rows, err := tx.QueryContext(ctx, `SELECT competition_id, person_key FROM entries
		WHERE individual AND individual_id IN (SELECT id FROM individuals WHERE person_id = $1)
		GROUP BY competition_id, person_key ORDER BY competition_id, MIN(entered_at), person_key`, into)
	if err != nil {
		return err
	}
	type rekey struct{ competition, from, to string }
	var todo []rekey
	kept := map[string]string{}
	for rows.Next() {
		var competition, key string
		if err := rows.Scan(&competition, &key); err != nil {
			rows.Close()
			return err
		}
		if to, ok := kept[competition]; ok {
			todo = append(todo, rekey{competition, key, to})
		} else {
			kept[competition] = key
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, r := range todo {
		if _, err := tx.ExecContext(ctx, `UPDATE entries SET person_key = $1 WHERE competition_id = $2 AND person_key = $3 AND individual
			AND individual_id IN (SELECT id FROM individuals WHERE person_id = $4)`, r.to, r.competition, r.from, into); err != nil {
			return err
		}
		var others int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM entries WHERE competition_id = $1 AND person_key = $2`, r.competition, r.from).Scan(&others); err != nil {
			return err
		}
		if others == 0 {
			if err := renameKey(ctx, tx, r.competition, r.from, r.to); err != nil {
				return err
			}
		}
	}
	return nil
}

// renameKey changes a person key wherever a competition saved it (keyedJSON
// and keyedRows): its timetables' seats and rules, its simulations, its desk
// messages, scratch warnings cleared and officials' check-ins.
func renameKey(ctx context.Context, tx *sql.Tx, competitionID, from, to string) error {
	// Keys are JSON strings in the saved JSON, quoted as Go quotes them; a
	// quoted key can't be part of a longer string, whose quotes are escaped.
	f, err := json.Marshal(from)
	if err != nil {
		return err
	}
	t, err := json.Marshal(to)
	if err != nil {
		return err
	}
	for _, c := range keyedJSON {
		if _, err := tx.ExecContext(ctx, `UPDATE `+c.table+` SET `+c.column+` = replace(`+c.column+`, $2, $3) WHERE `+c.competition+` = $1`,
			competitionID, string(f), string(t)); err != nil {
			return err
		}
	}
	for _, table := range keyedRows {
		// Where both keys have a row, the one the person had keeps it.
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+table.table+` WHERE competition_id = $1 AND person = $2 AND EXISTS (
			SELECT 1 FROM `+table.table+` o WHERE o.competition_id = $1 AND o.person = $3 AND o.`+table.also+` = `+table.table+`.`+table.also+`)`,
			competitionID, from, to); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE `+table.table+` SET person = $3 WHERE competition_id = $1 AND person = $2`, competitionID, from, to); err != nil {
			return err
		}
	}
	return nil
}

// Untie takes a role off the page of the person whose personal link token is
// (or opens a role through): one of their other records (kind RoleMember,
// RoleCoach or RoleIndividual), which becomes a person of its own again, or a
// shared link (any other kind), by its tie. A record can't be taken off its
// own page.
func (s *Store) Untie(ctx context.Context, token, kind, id string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		own, err := holderOf(ctx, tx, Personal(token))
		if err != nil {
			return err
		}
		switch kind {
		case RoleMember, RoleCoach, RoleIndividual:
			if kind == own.Kind && id == own.ID {
				return ErrOwnLink
			}
			return s.split(ctx, tx, kind, id, own.PersonID)
		}
		return affected(tx.ExecContext(ctx, `DELETE FROM shared_ties WHERE id = $1 AND record_id IN (`+personRecords("2")+`)`, id, own.PersonID))
	})
}

// split makes one of a person's records a person of its own. An individual
// leaving takes a key of its own in each competition it shares one with the
// person; what was saved under the key stays with the person.
func (s *Store) split(ctx context.Context, tx *sql.Tx, kind, id, personID string) error {
	table := map[string]string{RoleMember: "members", RoleCoach: "coaches", RoleIndividual: "individuals"}[kind]
	if err := affected(tx.ExecContext(ctx, `UPDATE `+table+` SET person_id = $1 WHERE id = $2 AND person_id = $3`, newID(), id, personID)); err != nil {
		return err
	}
	if kind != RoleIndividual {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT competition_id, person_key FROM entries e WHERE individual_id = $1 AND EXISTS (
		SELECT 1 FROM entries o WHERE o.competition_id = e.competition_id AND o.person_key = e.person_key
		AND o.individual_id IN (SELECT id FROM individuals WHERE person_id = $2))`, id, personID)
	if err != nil {
		return err
	}
	var shared [][2]string
	for rows.Next() {
		var c, k string
		if err := rows.Scan(&c, &k); err != nil {
			rows.Close()
			return err
		}
		shared = append(shared, [2]string{c, k})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, ck := range shared {
		if _, err := tx.ExecContext(ctx, `UPDATE entries SET person_key = $1 WHERE competition_id = $2 AND person_key = $3 AND individual_id = $4`,
			IndividualKey(newID()), ck[0], ck[1], id); err != nil {
			return err
		}
	}
	return nil
}

// MakeCoach makes one of a club's members a coach too (ADR 0010): a coach of
// the club who is the same person, whose member link opens the coach's page,
// so they need no link of their own.
func (s *Store) MakeCoach(ctx context.Context, clubID, memberID string) (Coach, error) {
	now := s.stamp()
	c := Coach{ID: newID(), ClubID: clubID, SignsOff: true, CreatedAt: parseTime(now)}
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var personID string
		if err := tx.QueryRowContext(ctx, `SELECT name, COALESCE(person_id, id) FROM members WHERE id = $1 AND club_id = $2`, memberID, clubID).
			Scan(&c.Name, &personID); err != nil {
			return notFound(err)
		}
		var n, already int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*), COUNT(CASE WHEN person_id = $2 THEN 1 END) FROM coaches WHERE club_id = $1`, clubID, personID).
			Scan(&n, &already); err != nil {
			return err
		}
		if already > 0 {
			return ErrTied
		}
		if n >= MaxCoaches {
			return ErrLimit
		}
		// The coach's own link is made but never shown: the comp sec can
		// make them one later if they want it apart.
		if _, err := tx.ExecContext(ctx, `INSERT INTO coaches (id, club_id, token_hash, name, created_at, person_id) VALUES ($1, $2, $3, $4, $5, $6)`,
			c.ID, clubID, hash(newToken()), c.Name, now, personID); err != nil {
			return err
		}
		return touch(ctx, tx, clubID, now)
	})
	return c, err
}

// SharedTie is someone holding a shared link through their own page: who
// added it, so the link's owner can see and undo it.
type SharedTie struct {
	ID     string
	Kind   string // RoleClub, RoleOrganiser or RoleHelper
	Target string // the club, competition or helper link
	Name   string // the person, as named on the page they added it on
	At     time.Time
}

// sharedTies are the ties that still work to a set of shared links: kind
// and target, as pairs of parameters from $1.
func (s *Store) sharedTies(ctx context.Context, cond string, args ...any) ([]SharedTie, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT t.id, t.kind, t.target_id, COALESCE(m.name, co.name, i.name, ''), t.created_at FROM shared_ties t
		LEFT JOIN members m ON m.id = t.record_id LEFT JOIN coaches co ON co.id = t.record_id LEFT JOIN individuals i ON i.id = t.record_id
		WHERE (m.id IS NOT NULL OR co.id IS NOT NULL OR i.id IS NOT NULL) AND (`+cond+`)
		ORDER BY t.created_at, t.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SharedTie
	for rows.Next() {
		var t SharedTie
		var at string
		if err := rows.Scan(&t.ID, &t.Kind, &t.Target, &t.Name, &at); err != nil {
			return nil, err
		}
		t.At = parseTime(at)
		out = append(out, t)
	}
	return out, rows.Err()
}

// ClubTies are the people whose own page opens a club's comp sec pages,
// through its admin link as it is now.
func (s *Store) ClubTies(ctx context.Context, clubID string) ([]SharedTie, error) {
	return s.sharedTies(ctx, `t.kind = 'club' AND t.target_id = $1 AND t.link_hash = (SELECT admin_hash FROM clubs WHERE id = $1)`, clubID)
}

// RemoveClubTie stops someone's page opening a club's comp sec pages.
func (s *Store) RemoveClubTie(ctx context.Context, clubID, id string) error {
	return affected(s.db.ExecContext(ctx, `DELETE FROM shared_ties WHERE id = $1 AND kind = 'club' AND target_id = $2`, id, clubID))
}

// CompetitionTies are the people whose own page opens a competition's admin
// pages, through its admin link or a helper link, as they are now.
func (s *Store) CompetitionTies(ctx context.Context, competitionID string) ([]SharedTie, error) {
	return s.sharedTies(ctx, `(t.kind = 'organiser' AND t.target_id = $1 AND t.link_hash = (SELECT admin_hash FROM competitions WHERE id = $1))
		OR (t.kind = 'helper' AND EXISTS (SELECT 1 FROM competition_links l WHERE l.id = t.target_id AND l.competition_id = $1 AND l.token_hash = t.link_hash))`, competitionID)
}

// RemoveCompetitionTie stops someone's page opening a competition's admin
// pages.
func (s *Store) RemoveCompetitionTie(ctx context.Context, competitionID, id string) error {
	return affected(s.db.ExecContext(ctx, `DELETE FROM shared_ties WHERE id = $1 AND (
		(kind = 'organiser' AND target_id = $2)
		OR (kind = 'helper' AND target_id IN (SELECT id FROM competition_links WHERE competition_id = $2)))`, id, competitionID))
}

// PersonGroups are the people of a competition's clubs and individuals, by
// person key, each with their person: so a member who also coaches, or also
// enters on their own, is one person (ADR 0010).
func (s *Store) PersonGroups(ctx context.Context, competitionID string) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT 'm', m.id, COALESCE(m.person_id, m.id) FROM members m
			JOIN club_competitions cc ON cc.club_id = m.club_id WHERE cc.competition_id = $1
		UNION ALL SELECT 'c', co.id, COALESCE(co.person_id, co.id) FROM coaches co
			JOIN club_competitions cc ON cc.club_id = co.club_id WHERE cc.competition_id = $1
		UNION ALL SELECT DISTINCT 'i', e.person_key, i.person_id FROM entries e JOIN individuals i ON i.id = e.individual_id
			WHERE e.competition_id = $1 AND e.person_key <> ''`, competitionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var kind, id, person string
		if err := rows.Scan(&kind, &id, &person); err != nil {
			return nil, err
		}
		switch kind {
		case KindMember:
			out[MemberKey(id)] = person
		case KindCoach:
			out[CoachKey(id)] = person
		default:
			out[id] = person // already a key
		}
	}
	return out, rows.Err()
}
