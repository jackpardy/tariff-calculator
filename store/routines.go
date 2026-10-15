package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// A member's routines, kept on their page (ADR 0011 Decision 1). The member
// saves, renames and deletes them; the coaches who see the member see them,
// add their own to the member's page and write a note on one; the comp sec
// sees them.

const (
	// MaxRoutines is the most routines a member keeps on their page.
	MaxRoutines = 30
	// MaxRoutineSkills is the most skills a kept routine has.
	MaxRoutineSkills = 30
	// MaxRoutineName is the longest a routine's name is, as in the builder.
	MaxRoutineName = 60
	// MaxRoutineNote is the longest a coach's note on a routine is.
	MaxRoutineNote = 500
)

// Routine is one of a member's kept routines. Skills is its skills as JSON, as
// the routine builder saves them; names and tariffs are worked out again
// whenever it's shown (ADR 0004 Decision 5).
type Routine struct {
	ID, MemberID string
	Source       string // its id in the browser it came from, "" for none
	Name         string
	Skills       string
	Requirements string // a built-in requirements reference it's checked against, "" for none
	Checks       string // the checks chosen in the builder, as JSON
	Note, NoteBy string // a coach's note, and the coach's name
	NoteAt       time.Time
	UpdatedAt    time.Time
	AddedBy      string // the coach who added it, by id; "" for the member's own
	AddedName    string // that coach's name
}

// NoteBefore says the note was written before the routine last changed.
func (r Routine) NoteBefore() bool {
	return r.Note != "" && r.NoteAt.Before(r.UpdatedAt)
}

// RoutineIn is a routine to keep, from the builder.
type RoutineIn struct {
	Source, Name, Skills, Requirements, Checks string
}

const routineColumns = `id, member_id, source, name, skills, requirements, checks, note, note_by, note_at, updated_at, added_by, added_name`

func scanRoutine(row interface{ Scan(...any) error }) (Routine, error) {
	var r Routine
	var noteAt, updated string
	err := row.Scan(&r.ID, &r.MemberID, &r.Source, &r.Name, &r.Skills, &r.Requirements, &r.Checks, &r.Note, &r.NoteBy, &noteAt, &updated, &r.AddedBy, &r.AddedName)
	r.NoteAt, r.UpdatedAt = parseTime(noteAt), parseTime(updated)
	return r, err
}

// MemberRoutines are a member's kept routines, by name.
func (s *Store) MemberRoutines(ctx context.Context, memberID string) ([]Routine, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+routineColumns+` FROM member_routines WHERE member_id = $1 ORDER BY name COLLATE NOCASE, updated_at`, memberID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Routine
	for rows.Next() {
		r, err := scanRoutine(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// MemberRoutine is one of a member's kept routines.
func (s *Store) MemberRoutine(ctx context.Context, memberID, id string) (Routine, error) {
	r, err := scanRoutine(s.db.QueryRowContext(ctx, `SELECT `+routineColumns+` FROM member_routines WHERE id = $1 AND member_id = $2`, id, memberID))
	return r, notFound(err)
}

// SaveRoutines keeps the member's own routines on their page. One from a
// browser routine already kept (the same source) changes it; any other is
// added, while there's room. It gives how many were kept, and how many weren't
// for want of room.
func (s *Store) SaveRoutines(ctx context.Context, clubID, memberID string, in []RoutineIn) (saved, full int, err error) {
	return s.saveRoutines(ctx, clubID, memberID, Coach{}, in)
}

// AddRoutinesAsCoach is a coach adding routines to the page of a member they
// see, as SaveRoutines: one they added from the same browser routine changes;
// the member's own are never changed by it.
func (s *Store) AddRoutinesAsCoach(ctx context.Context, c Coach, memberID string, in []RoutineIn) (saved, full int, err error) {
	var sees bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM members m WHERE m.id = $3 AND `+coachSees+`)`, c.ID, c.ClubID, memberID).Scan(&sees); err != nil {
		return 0, 0, err
	}
	if !sees {
		return 0, 0, ErrNotFound
	}
	return s.saveRoutines(ctx, c.ClubID, memberID, c, in)
}

// saveRoutines keeps routines on a member's page, the member's own or those
// a coach (by) adds.
func (s *Store) saveRoutines(ctx context.Context, clubID, memberID string, by Coach, in []RoutineIn) (saved, full int, err error) {
	now := s.stamp()
	err = s.tx(ctx, func(tx *sql.Tx) error {
		saved, full = 0, 0
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM member_routines WHERE member_id = $1`, memberID).Scan(&n); err != nil {
			return err
		}
		for _, r := range in {
			name := strings.TrimSpace(r.Name)
			if name == "" {
				name = "Routine"
			}
			name = clip(name, MaxRoutineName)
			if r.Source != "" {
				res, err := tx.ExecContext(ctx, `UPDATE member_routines SET name = $1, skills = $2, requirements = $3, checks = $4, updated_at = $5
					WHERE member_id = $6 AND source = $7 AND added_by = $8`, name, r.Skills, r.Requirements, r.Checks, now, memberID, r.Source, by.ID)
				if err != nil {
					return err
				}
				if changed, err := res.RowsAffected(); err != nil {
					return err
				} else if changed > 0 {
					saved++
					continue
				}
			}
			if n >= MaxRoutines {
				full++
				continue
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO member_routines (id, member_id, source, name, skills, requirements, checks, updated_at, added_by, added_name)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`, newID(), memberID, r.Source, name, r.Skills, r.Requirements, r.Checks, now, by.ID, by.Name); err != nil {
				return err
			}
			n++
			saved++
		}
		return touch(ctx, tx, clubID, now)
	})
	return saved, full, err
}

// RenameRoutine renames one of a member's kept routines.
func (s *Store) RenameRoutine(ctx context.Context, memberID, id, name string) error {
	name = clip(strings.TrimSpace(name), MaxRoutineName)
	if name == "" {
		return fmt.Errorf("a routine needs a name")
	}
	return affected(s.db.ExecContext(ctx, `UPDATE member_routines SET name = $1 WHERE id = $2 AND member_id = $3`, name, id, memberID))
}

// DeleteRoutine deletes one of a member's kept routines.
func (s *Store) DeleteRoutine(ctx context.Context, memberID, id string) error {
	return affected(s.db.ExecContext(ctx, `DELETE FROM member_routines WHERE id = $1 AND member_id = $2`, id, memberID))
}

// NoteRoutine is a coach's note on a routine of a member they see ("" clears
// it), under the coach's name.
func (s *Store) NoteRoutine(ctx context.Context, c Coach, memberID, id, note string) error {
	note = clip(strings.TrimSpace(note), MaxRoutineNote)
	by, at := c.Name, s.stamp()
	if note == "" {
		by, at = "", ""
	}
	return affected(s.db.ExecContext(ctx, `UPDATE member_routines SET note = $3, note_by = $4, note_at = $5
		WHERE id = $6 AND member_id = $7 AND member_id IN (SELECT m.id FROM members m WHERE `+coachSees+`)`,
		c.ID, c.ClubID, note, by, at, id, memberID))
}

// RoutineCounts is how many routines each of a club's members keeps, by member.
func (s *Store) RoutineCounts(ctx context.Context, clubID string) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT r.member_id, COUNT(*) FROM member_routines r JOIN members m ON m.id = r.member_id
		WHERE m.club_id = $1 GROUP BY r.member_id`, clubID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// CoachMembers are the members a coach sees, by name: their own, those
// without a coach, or everyone where the club says so.
func (s *Store) CoachMembers(ctx context.Context, c Coach) ([]Member, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT m.id, m.club_id, m.name, COALESCE(m.coach_id, ''), m.created_at, COALESCE(m.person_id, m.id)
		FROM members m WHERE `+coachSees+` ORDER BY m.name COLLATE NOCASE`, c.ID, c.ClubID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Member
	for rows.Next() {
		var m Member
		var created string
		if err := rows.Scan(&m.ID, &m.ClubID, &m.Name, &m.CoachID, &created, &m.PersonID); err != nil {
			return nil, err
		}
		m.CreatedAt = parseTime(created)
		out = append(out, m)
	}
	return out, rows.Err()
}

// CoachMember is a member a coach sees, by id.
func (s *Store) CoachMember(ctx context.Context, c Coach, memberID string) (Member, error) {
	var m Member
	var created string
	err := s.db.QueryRowContext(ctx, `SELECT m.id, m.club_id, m.name, COALESCE(m.coach_id, ''), m.created_at, COALESCE(m.person_id, m.id)
		FROM members m WHERE m.id = $3 AND `+coachSees, c.ID, c.ClubID, memberID).
		Scan(&m.ID, &m.ClubID, &m.Name, &m.CoachID, &created, &m.PersonID)
	m.CreatedAt = parseTime(created)
	return m, notFound(err)
}

// clip cuts text to at most n characters.
func clip(text string, n int) string {
	if r := []rune(text); len(r) > n {
		return string(r[:n])
	}
	return text
}
