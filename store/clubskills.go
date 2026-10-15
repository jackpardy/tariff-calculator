package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"tariffCalculator/skills"
)

// The club's own skills (ADR 0011 Decision 4), kept by its coaches: the
// club's name for a skill the catalog can describe (which is still that
// skill, as skills.TrampolineSkill.Equal finds it, so its marks are the
// skill's), or something else to track that isn't a Code of Points skill (a
// drill, a conditioning test), marked by its own item.

const (
	// MaxClubSkills is the most skills a club keeps of its own.
	MaxClubSkills = 200
	// MaxClubSkillNote is the longest a club skill's note is.
	MaxClubSkillNote = 300
)

// ErrSameName is a club skill named as another of the club's already is.
var ErrSameName = errors.New("the club already has a skill of that name")

// ClubSkill is one of the club's own skills. Skill is the skill as JSON for
// a club's name for a catalog skill (with the name as its custom_name), ""
// for something else to track.
type ClubSkill struct {
	ID, ClubID string
	Name, Note string
	Skill      string
	ByName     string // the coach who added it
	CreatedAt  time.Time
}

// Item is what marks on a club skill are kept under: a tracking-only skill's
// own item; a named catalog skill is marked as that skill.
func (c ClubSkill) Item() string { return "c:" + c.ID }

// ClubSkills are a club's own skills, by name.
func (s *Store) ClubSkills(ctx context.Context, clubID string) ([]ClubSkill, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, club_id, name, note, skill, by_name, created_at FROM club_skills WHERE club_id = $1 ORDER BY name COLLATE NOCASE`, clubID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ClubSkill
	for rows.Next() {
		var c ClubSkill
		var at string
		if err := rows.Scan(&c.ID, &c.ClubID, &c.Name, &c.Note, &c.Skill, &c.ByName, &at); err != nil {
			return nil, err
		}
		c.CreatedAt = parseTime(at)
		out = append(out, c)
	}
	return out, rows.Err()
}

// AddClubSkill is a coach adding one of the club's own skills: a name for a
// catalog skill (skill as JSON), or, with skill "", something else to track.
// Naming a catalog skill the club has already named renames it.
func (s *Store) AddClubSkill(ctx context.Context, c Coach, name, note, skill string) error {
	name = clip(strings.TrimSpace(name), skills.MaxCustomNameLength)
	note = clip(strings.TrimSpace(note), MaxClubSkillNote)
	if name == "" {
		return errors.New("a club skill needs a name")
	}
	now := s.stamp()
	return s.tx(ctx, func(tx *sql.Tx) error {
		existing, err := clubSkillsIn(ctx, tx, c.ClubID)
		if err != nil {
			return err
		}
		var want skills.TrampolineSkill
		if skill != "" {
			if err := json.Unmarshal([]byte(skill), &want); err != nil {
				return err
			}
			want.CustomName = name
			data, err := json.Marshal(want)
			if err != nil {
				return err
			}
			skill = string(data)
		}
		for _, e := range existing {
			var named skills.TrampolineSkill
			if skill != "" && e.Skill != "" && json.Unmarshal([]byte(e.Skill), &named) == nil && named.Equal(&want) {
				if strings.EqualFold(e.Name, name) || !nameTaken(existing, name, e.ID) {
					_, err := tx.ExecContext(ctx, `UPDATE club_skills SET name = $1, note = $2, skill = $3 WHERE id = $4`, name, note, skill, e.ID)
					if err == nil {
						err = touch(ctx, tx, c.ClubID, now)
					}
					return err
				}
				return ErrSameName
			}
		}
		if nameTaken(existing, name, "") {
			return ErrSameName
		}
		if len(existing) >= MaxClubSkills {
			return ErrLimit
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO club_skills (id, club_id, name, note, skill, by_name, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			newID(), c.ClubID, name, note, skill, c.Name, now); err != nil {
			return err
		}
		return touch(ctx, tx, c.ClubID, now)
	})
}

// nameTaken says one of the skills, other than the one with id except, has
// the name (ignoring case).
func nameTaken(list []ClubSkill, name, except string) bool {
	for _, e := range list {
		if e.ID != except && strings.EqualFold(e.Name, name) {
			return true
		}
	}
	return false
}

// clubSkillsIn are a club's own skills, in a transaction.
func clubSkillsIn(ctx context.Context, tx *sql.Tx, clubID string) ([]ClubSkill, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, name, skill FROM club_skills WHERE club_id = $1`, clubID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ClubSkill
	for rows.Next() {
		c := ClubSkill{ClubID: clubID}
		if err := rows.Scan(&c.ID, &c.Name, &c.Skill); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// RemoveClubSkill is a coach removing one of the club's own skills. The marks
// on something else to track go with it; a named catalog skill's marks stay,
// as they're the skill's.
func (s *Store) RemoveClubSkill(ctx context.Context, c Coach, id string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		if err := affected(tx.ExecContext(ctx, `DELETE FROM club_skills WHERE id = $1 AND club_id = $2`, id, c.ClubID)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM skill_marks WHERE item = $1 AND member_id IN (SELECT id FROM members WHERE club_id = $2)`,
			ClubSkill{ID: id}.Item(), c.ClubID); err != nil {
			return err
		}
		return touch(ctx, tx, c.ClubID, s.stamp())
	})
}
