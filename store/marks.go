package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"tariffCalculator/skills"
)

// Skill marks (ADR 0011 Decision 2): what a member is learning or can do,
// marked by them or by a coach who sees them. One mark an item: marking it
// again replaces the mark, and whoever marked it last is who it's by.

// The states a mark can be in.
const (
	Learning = "learning"
	CanDo    = "can"
)

// MaxMarks is the most marks a member has.
const MaxMarks = 500

// Mark is one item a member is learning or can do. A skill's item is "s:"
// and an id; which skill it is, is Skill (as JSON), and two skills are the
// same item when skills.TrampolineSkill.Equal says they're the same skill.
type Mark struct {
	MemberID string
	Item     string
	Skill    string
	State    string // Learning or CanDo
	ByCoach  string // the coach's id, "" when the gymnast marked it
	ByName   string // the name of who marked it
	At       time.Time
}

// ByGymnast says the gymnast marked it themselves.
func (m Mark) ByGymnast() bool { return m.ByCoach == "" }

// MarkIn is a mark to make: the skill (as JSON), and the state (Learning,
// CanDo, or "" to clear it). Item is left empty for a skill: the skill's mark
// is found by the skill.
type MarkIn struct {
	Item, Skill, State string
}

// MemberMarks are a member's marks, newest first.
func (s *Store) MemberMarks(ctx context.Context, memberID string) ([]Mark, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT member_id, item, skill, state, by_coach, by_name, at FROM skill_marks WHERE member_id = $1 ORDER BY at DESC, item`, memberID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Mark
	for rows.Next() {
		var m Mark
		var at string
		if err := rows.Scan(&m.MemberID, &m.Item, &m.Skill, &m.State, &m.ByCoach, &m.ByName, &at); err != nil {
			return nil, err
		}
		m.At = parseTime(at)
		out = append(out, m)
	}
	return out, rows.Err()
}

// MarkAsGymnast is a member marking an item themselves.
func (s *Store) MarkAsGymnast(ctx context.Context, m Member, in MarkIn) error {
	return s.mark(ctx, m.ClubID, m.ID, "", m.Name, in)
}

// MarkAsCoach is a coach marking an item for a member they see.
func (s *Store) MarkAsCoach(ctx context.Context, c Coach, memberID string, in MarkIn) error {
	if _, err := s.CoachMember(ctx, c, memberID); err != nil {
		return err
	}
	return s.mark(ctx, c.ClubID, memberID, c.ID, c.Name, in)
}

// mark makes, changes or clears a member's mark on an item, by a coach (by
// their id) or the gymnast (""), under the marker's name.
func (s *Store) mark(ctx context.Context, clubID, memberID, coachID, name string, in MarkIn) error {
	now := s.stamp()
	return s.tx(ctx, func(tx *sql.Tx) error {
		if in.Item == "" {
			item, err := skillMarkItem(ctx, tx, memberID, in.Skill)
			if err != nil {
				return err
			}
			in.Item = item
		}
		if in.State == "" {
			if _, err := tx.ExecContext(ctx, `DELETE FROM skill_marks WHERE member_id = $1 AND item = $2`, memberID, in.Item); err != nil {
				return err
			}
			return touch(ctx, tx, clubID, now)
		}
		if in.State != Learning && in.State != CanDo {
			return ErrNotFound
		}
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM skill_marks WHERE member_id = $1 AND item != $2`, memberID, in.Item).Scan(&n); err != nil {
			return err
		}
		if n >= MaxMarks {
			return ErrLimit
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO skill_marks (member_id, item, skill, state, by_coach, by_name, at) VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (member_id, item) DO UPDATE SET skill = excluded.skill, state = excluded.state, by_coach = excluded.by_coach, by_name = excluded.by_name, at = excluded.at`,
			memberID, in.Item, in.Skill, in.State, coachID, name, now); err != nil {
			return err
		}
		return touch(ctx, tx, clubID, now)
	})
}

// MarkCounts are, for each of a club's members with any marks, how many items
// they can do and how many of their marks only they made (for a coach to
// confirm). A member without marks isn't in either.
func (s *Store) MarkCounts(ctx context.Context, clubID string) (can, byGymnast map[string]int, err error) {
	rows, err := s.db.QueryContext(ctx, `SELECT k.member_id, SUM(k.state = 'can'), SUM(k.by_coach = '') FROM skill_marks k JOIN members m ON m.id = k.member_id
		WHERE m.club_id = $1 GROUP BY k.member_id`, clubID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	can, byGymnast = map[string]int{}, map[string]int{}
	for rows.Next() {
		var id string
		var c, g int
		if err := rows.Scan(&id, &c, &g); err != nil {
			return nil, nil, err
		}
		can[id], byGymnast[id] = c, g
	}
	return can, byGymnast, rows.Err()
}

// SetSkillsOpen says whether every member of a club sees everyone's skills.
func (s *Store) SetSkillsOpen(ctx context.Context, clubID string, on bool) error {
	return affected(s.db.ExecContext(ctx, `UPDATE clubs SET skills_open = $1 WHERE id = $2`, on, clubID))
}

// ClubByID is a club by its id, without marking it used.
func (s *Store) ClubByID(ctx context.Context, id string) (Club, error) {
	var c Club
	var created, used string
	err := s.db.QueryRowContext(ctx, `SELECT id, name, join_token, coaches_see_all, skills_open, created_at, last_used FROM clubs WHERE id = $1`, id).
		Scan(&c.ID, &c.Name, &c.JoinLink, &c.CoachesSeeAll, &c.SkillsOpen, &created, &used)
	c.CreatedAt, c.LastUsed = parseTime(created), parseTime(used)
	return c, notFound(err)
}

// skillMarkItem is the item of the member's mark on a skill the same as the
// one given (as JSON), by skills.TrampolineSkill.Equal, or a new item if
// there's none.
func skillMarkItem(ctx context.Context, tx *sql.Tx, memberID, skill string) (string, error) {
	var want skills.TrampolineSkill
	if err := json.Unmarshal([]byte(skill), &want); err != nil {
		return "", err
	}
	rows, err := tx.QueryContext(ctx, `SELECT item, skill FROM skill_marks WHERE member_id = $1 AND item LIKE 's:%'`, memberID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var item, data string
		if err := rows.Scan(&item, &data); err != nil {
			return "", err
		}
		var s skills.TrampolineSkill
		if json.Unmarshal([]byte(data), &s) == nil && s.Equal(&want) {
			return item, nil
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return "s:" + newID(), nil
}
