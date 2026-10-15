package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"tariffCalculator/skills"
)

// Skill marks (ADR 0011 Decision 2): what a member is learning or can do.
// The gymnast's mark and a coach's are kept apart, so both show: a skill can
// be Can do to the gymnast and Learning to their coach. Each side changes
// only its own; a coach marking replaces another coach's mark.

// The states a mark can be in.
const (
	Learning = "learning"
	CanDo    = "can"
)

// The sides a mark is on.
const (
	SideGymnast = "gymnast"
	SideCoach   = "coach"
)

// MaxMarks is the most skills a member has marked.
const MaxMarks = 500

// Mark is one side's mark on an item. A skill's item is "s:" and an id;
// which skill it is, is Skill (as JSON), and two skills are the same item
// when skills.TrampolineSkill.Equal says they're the same skill.
type Mark struct {
	MemberID string
	Item     string
	Side     string // SideGymnast or SideCoach
	Skill    string
	State    string // Learning or CanDo
	ByCoach  string // the coach's id, "" on the gymnast's side
	ByName   string // the name of who marked it
	At       time.Time
}

// MarkIn is a mark to make: the skill (as JSON), and the state (Learning,
// CanDo, or "" to take the side's mark off). Item is left empty for a skill:
// the skill's item is found by the skill.
type MarkIn struct {
	Item, Skill, State string
}

// MemberMarks are a member's marks, both sides, newest first.
func (s *Store) MemberMarks(ctx context.Context, memberID string) ([]Mark, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT member_id, item, side, skill, state, by_coach, by_name, at FROM skill_marks WHERE member_id = $1 ORDER BY at DESC, item, side`, memberID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Mark
	for rows.Next() {
		var m Mark
		var at string
		if err := rows.Scan(&m.MemberID, &m.Item, &m.Side, &m.Skill, &m.State, &m.ByCoach, &m.ByName, &at); err != nil {
			return nil, err
		}
		m.At = parseTime(at)
		out = append(out, m)
	}
	return out, rows.Err()
}

// MarkAsGymnast is a member marking an item on their own side.
func (s *Store) MarkAsGymnast(ctx context.Context, m Member, in MarkIn) error {
	return s.mark(ctx, m.ClubID, m.ID, SideGymnast, "", m.Name, in)
}

// MarkAsCoach is a coach marking an item, on the coach's side, for a member
// they see.
func (s *Store) MarkAsCoach(ctx context.Context, c Coach, memberID string, in MarkIn) error {
	if _, err := s.CoachMember(ctx, c, memberID); err != nil {
		return err
	}
	return s.mark(ctx, c.ClubID, memberID, SideCoach, c.ID, c.Name, in)
}

// mark makes, changes or takes off one side's mark on a member's item, under
// the marker's name (and coach's id, on the coach's side).
func (s *Store) mark(ctx context.Context, clubID, memberID, side, coachID, name string, in MarkIn) error {
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
			if _, err := tx.ExecContext(ctx, `DELETE FROM skill_marks WHERE member_id = $1 AND item = $2 AND side = $3`, memberID, in.Item, side); err != nil {
				return err
			}
			return touch(ctx, tx, clubID, now)
		}
		if in.State != Learning && in.State != CanDo {
			return ErrNotFound
		}
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(DISTINCT item) FROM skill_marks WHERE member_id = $1 AND item != $2`, memberID, in.Item).Scan(&n); err != nil {
			return err
		}
		if n >= MaxMarks {
			return ErrLimit
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO skill_marks (member_id, item, side, skill, state, by_coach, by_name, at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (member_id, item, side) DO UPDATE SET skill = excluded.skill, state = excluded.state, by_coach = excluded.by_coach, by_name = excluded.by_name, at = excluded.at`,
			memberID, in.Item, side, in.Skill, in.State, coachID, name, now); err != nil {
			return err
		}
		return touch(ctx, tx, clubID, now)
	})
}

// skillMarkItem is the item of the member's marks on a skill the same as the
// one given (as JSON), by skills.TrampolineSkill.Equal, or a new item if
// there's none.
func skillMarkItem(ctx context.Context, tx *sql.Tx, memberID, skill string) (string, error) {
	var want skills.TrampolineSkill
	if err := json.Unmarshal([]byte(skill), &want); err != nil {
		return "", err
	}
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT item, skill FROM skill_marks WHERE member_id = $1 AND item LIKE 's:%'`, memberID)
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

// MarkCounts are, for each of a club's members with any marks, how many
// items someone has marked Can do, and how many the gymnast has marked that
// no coach has marked the same (for a coach to confirm). A member without
// marks isn't in either.
func (s *Store) MarkCounts(ctx context.Context, clubID string) (can, toConfirm map[string]int, err error) {
	rows, err := s.db.QueryContext(ctx, `SELECT k.member_id, COUNT(DISTINCT CASE WHEN k.state = 'can' THEN k.item END),
		COUNT(DISTINCT CASE WHEN k.side = 'gymnast' AND NOT EXISTS (SELECT 1 FROM skill_marks c
			WHERE c.member_id = k.member_id AND c.item = k.item AND c.side = 'coach' AND c.state = k.state) THEN k.item END)
		FROM skill_marks k JOIN members m ON m.id = k.member_id
		WHERE m.club_id = $1 GROUP BY k.member_id`, clubID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	can, toConfirm = map[string]int{}, map[string]int{}
	for rows.Next() {
		var id string
		var c, g int
		if err := rows.Scan(&id, &c, &g); err != nil {
			return nil, nil, err
		}
		can[id], toConfirm[id] = c, g
	}
	return can, toConfirm, rows.Err()
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
