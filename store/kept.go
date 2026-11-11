package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Requirements kept with the person (ADR 0013): a person's own requirement
// sets and levels, as the browser keeps them, on their page, so a new phone,
// another browser or Safari clearing a site's storage loses nothing.

// The kinds of requirements a person keeps.
const (
	KeptSet   = "set"   // a requirement set (set routines among them)
	KeptLevel = "level" // a level: requirements for a competition level's two exercises
)

// MaxKept is the most of each kind a person keeps, and MaxKeptBytes the
// largest one.
const (
	MaxKept      = 50
	MaxKeptBytes = 64 << 10
)

// ErrTooBig is something to keep that's larger than there's room for.
var ErrTooBig = errors.New("too big")

// Kept is one of a person's kept requirement sets or levels.
type Kept struct {
	ID, Kind string
	Source   string // the browser's id for it
	Name     string
	Data     string    // as the browser keeps it, JSON
	Updated  time.Time // when it was last saved
	InLevel  bool      // a set written for a level only, listed under its level
}

// KeptRequirements are a person's kept requirement sets and levels, by kind
// then name.
func (s *Store) KeptRequirements(ctx context.Context, personID string) ([]Kept, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, kind, source, name, data, updated_at, in_level FROM person_requirements WHERE person_id = $1 ORDER BY kind DESC, name, updated_at`, personID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Kept
	for rows.Next() {
		var k Kept
		var at string
		if err := rows.Scan(&k.ID, &k.Kind, &k.Source, &k.Name, &k.Data, &at, &k.InLevel); err != nil {
			return nil, err
		}
		k.Updated = parseTime(at)
		out = append(out, k)
	}
	return out, rows.Err()
}

// KeepRequirements keeps a person's requirement set or level (kind) by the
// browser's id for it (source), changing the one kept by that id if there
// is one. data is its JSON, already read and checked by the caller. It says
// whether it's newly kept; ErrLimit when the person keeps MaxKept of the
// kind already, ErrTooBig past MaxKeptBytes.
func (s *Store) KeepRequirements(ctx context.Context, personID, kind, source, name, data string) (added bool, err error) {
	return s.KeepRequirementsIn(ctx, personID, kind, source, name, data, false)
}

// KeepRequirementsIn is KeepRequirements, saying whether a set was written for
// a level only (inLevel).
func (s *Store) KeepRequirementsIn(ctx context.Context, personID, kind, source, name, data string, inLevel bool) (added bool, err error) {
	switch {
	case kind != KeptSet && kind != KeptLevel, source == "", len(source) > 60:
		return false, errors.New("not something to keep")
	case len(data) > MaxKeptBytes:
		return false, ErrTooBig
	}
	name = clip(strings.TrimSpace(name), 120)
	now := s.stamp()
	err = s.tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE person_requirements SET name = $1, data = $2, updated_at = $3, in_level = $7 WHERE person_id = $4 AND kind = $5 AND source = $6`,
			name, data, now, personID, kind, source, inLevel)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil || n > 0 {
			return err
		}
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM person_requirements WHERE person_id = $1 AND kind = $2`, personID, kind).Scan(&n); err != nil {
			return err
		}
		if n >= MaxKept {
			return ErrLimit
		}
		added = true
		_, err = tx.ExecContext(ctx, `INSERT INTO person_requirements (id, person_id, kind, source, name, data, updated_at, in_level) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			newID(), personID, kind, source, name, data, now, inLevel)
		return err
	})
	return added, err
}

// DropKept takes one of a person's kept requirement sets or levels off their
// page, by the browser's id for it (source). Nothing kept by it is no error.
func (s *Store) DropKept(ctx context.Context, personID, kind, source string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM person_requirements WHERE person_id = $1 AND kind = $2 AND source = $3`, personID, kind, source)
	return err
}

// moveKept moves one person's kept requirements to another's (from to to),
// for when a record's person changes (a join, a new link). Where both keep
// the same kind and source, the newer by updated_at stays, with its content.
// The cap (MaxKept) isn't applied: past it, all are kept rather than losing
// someone's work, and the cap only stops new ones.
func moveKept(ctx context.Context, tx *sql.Tx, from, to string) error {
	if from == to {
		return nil
	}
	const theirs = `(SELECT f.%s FROM person_requirements f WHERE f.person_id = $1 AND f.kind = person_requirements.kind AND f.source = person_requirements.source)`
	for _, q := range []string{
		// Where the one moving is newer, the person's own takes its content...
		`UPDATE person_requirements SET
			name = ` + fmt.Sprintf(theirs, "name") + `, data = ` + fmt.Sprintf(theirs, "data") + `,
			in_level = ` + fmt.Sprintf(theirs, "in_level") + `, updated_at = ` + fmt.Sprintf(theirs, "updated_at") + `
		WHERE person_id = $2 AND EXISTS (SELECT 1 FROM person_requirements f WHERE f.person_id = $1 AND f.kind = person_requirements.kind
			AND f.source = person_requirements.source AND f.updated_at > person_requirements.updated_at)`,
		// ...then the doubles go, and the rest move over.
		`DELETE FROM person_requirements WHERE person_id = $1 AND EXISTS (SELECT 1 FROM person_requirements t
			WHERE t.person_id = $2 AND t.kind = person_requirements.kind AND t.source = person_requirements.source)`,
		`UPDATE person_requirements SET person_id = $2 WHERE person_id = $1`,
	} {
		if _, err := tx.ExecContext(ctx, q, from, to); err != nil {
			return err
		}
	}
	return nil
}

// moveKeptIfLeft moves a person's kept requirements to another's when no
// record has them as its person any more (the person's last record has
// moved on), and leaves them when one does: that page still exists.
func moveKeptIfLeft(ctx context.Context, tx *sql.Tx, from, to string) error {
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM members WHERE COALESCE(person_id, id) = $1)
		+ (SELECT COUNT(*) FROM coaches WHERE COALESCE(person_id, id) = $1)
		+ (SELECT COUNT(*) FROM individuals WHERE person_id = $1)`, from).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	return moveKept(ctx, tx, from, to)
}

// dropOrphanedKept deletes the requirements of people who no longer have a
// member, coach or individual record: they go with the person.
func dropOrphanedKept(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM person_requirements WHERE person_id NOT IN (
		SELECT COALESCE(person_id, id) FROM members
		UNION SELECT COALESCE(person_id, id) FROM coaches
		UNION SELECT person_id FROM individuals WHERE person_id IS NOT NULL)`)
	return err
}
