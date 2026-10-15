package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"tariffCalculator/skills"
)

// Club levels (ADR 0011 Decision 5): the levels a club tracks its gymnasts
// against, in order. A level's requirements are kept in the usual format
// (requirements.Set as JSON), made in the requirements editor; with them,
// the club's own skills to track, which requirements can't describe.

// MaxClubLevels is the most levels a club keeps.
const MaxClubLevels = 50

// ClubLevel is one of a club's levels.
type ClubLevel struct {
	ID, ClubID string
	Source     string // the browser's id for its requirements, "" for none
	Name       string
	Set        string   // its requirements, as JSON
	ClubItems  []string // the club skills it tracks too, by id, in order
	ByName     string   // the coach who last saved it
	UpdatedAt  time.Time
}

// ClubLevels are a club's levels, in order.
func (s *Store) ClubLevels(ctx context.Context, clubID string) ([]ClubLevel, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, club_id, source, name, set_json, club_items, by_name, updated_at FROM club_levels WHERE club_id = $1 ORDER BY position, name`, clubID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ClubLevel
	for rows.Next() {
		var l ClubLevel
		var items, at string
		if err := rows.Scan(&l.ID, &l.ClubID, &l.Source, &l.Name, &l.Set, &items, &l.ByName, &at); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(items), &l.ClubItems)
		l.UpdatedAt = parseTime(at)
		out = append(out, l)
	}
	return out, rows.Err()
}

// SaveClubLevel is a coach saving a level's requirements (set: the
// requirements as JSON, already checked; name: theirs). Requirements from the
// same browser set (source) change the level they made; any others are a new
// level, last. It says whether it made a new one.
func (s *Store) SaveClubLevel(ctx context.Context, c Coach, source, name, set string) (added bool, err error) {
	name = clip(strings.TrimSpace(name), 80)
	if name == "" {
		name = "Level"
	}
	now := s.stamp()
	err = s.tx(ctx, func(tx *sql.Tx) error {
		if source != "" {
			res, err := tx.ExecContext(ctx, `UPDATE club_levels SET name = $1, set_json = $2, by_name = $3, updated_at = $4 WHERE club_id = $5 AND source = $6`,
				name, set, c.Name, now, c.ClubID, source)
			if err != nil {
				return err
			}
			if n, err := res.RowsAffected(); err != nil || n > 0 {
				return err
			}
		}
		var n, last int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(MAX(position), 0) FROM club_levels WHERE club_id = $1`, c.ClubID).Scan(&n, &last); err != nil {
			return err
		}
		if n >= MaxClubLevels {
			return ErrLimit
		}
		added = true
		if _, err := tx.ExecContext(ctx, `INSERT INTO club_levels (id, club_id, source, name, set_json, position, by_name, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			newID(), c.ClubID, source, name, set, last+1, c.Name, now); err != nil {
			return err
		}
		return touch(ctx, tx, c.ClubID, now)
	})
	return added, err
}

// SetLevelClubItems are the club skills (by id) a level tracks besides its
// requirements. Ids that aren't the club's are left out.
func (s *Store) SetLevelClubItems(ctx context.Context, c Coach, levelID string, ids []string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		own, err := clubSkillsIn(ctx, tx, c.ClubID)
		if err != nil {
			return err
		}
		kept := []string{}
		for _, id := range ids {
			if slices.ContainsFunc(own, func(k ClubSkill) bool { return k.ID == id && k.Skill == "" }) && !slices.Contains(kept, id) {
				kept = append(kept, id)
			}
		}
		data, err := json.Marshal(kept)
		if err != nil {
			return err
		}
		return affected(tx.ExecContext(ctx, `UPDATE club_levels SET club_items = $1, updated_at = $2 WHERE id = $3 AND club_id = $4`, string(data), s.stamp(), levelID, c.ClubID))
	})
}

// MoveClubLevel moves a level one place earlier (by -1) or later (by 1).
func (s *Store) MoveClubLevel(ctx context.Context, c Coach, levelID string, by int) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT id FROM club_levels WHERE club_id = $1 ORDER BY position, name`, c.ClubID)
		if err != nil {
			return err
		}
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		rows.Close()
		i := slices.Index(ids, levelID)
		if i < 0 {
			return ErrNotFound
		}
		if j := i + by; j >= 0 && j < len(ids) {
			ids[i], ids[j] = ids[j], ids[i]
		}
		for k, id := range ids {
			if _, err := tx.ExecContext(ctx, `UPDATE club_levels SET position = $1 WHERE id = $2`, k+1, id); err != nil {
				return err
			}
		}
		return nil
	})
}

// RemoveClubLevel removes one of a club's levels. Marks on its items stay:
// they're the gymnast's, and other levels may have the same items.
func (s *Store) RemoveClubLevel(ctx context.Context, c Coach, levelID string) error {
	return affected(s.db.ExecContext(ctx, `DELETE FROM club_levels WHERE id = $1 AND club_id = $2`, levelID, c.ClubID))
}

// SharedTrack is a club skill to track that comes with a shared level: its
// name and note.
type SharedTrack struct {
	Name string `json:"name"`
	Note string `json:"note,omitempty"`
}

// ImportClubLevel is a coach adding a level shared by another club (ADR 0011
// Decision 6): its requirements (set, as JSON, already checked) as a new
// level, last, tracking the club skills it came with. Each is the club's own
// skill to track of that name if it has one, or a new one; a name the club
// uses for a catalog skill isn't tracked. It gives the level's id, and how
// many club skills it added.
func (s *Store) ImportClubLevel(ctx context.Context, c Coach, name, set string, track []SharedTrack) (id string, added int, err error) {
	name = clip(strings.TrimSpace(name), 80)
	if name == "" {
		name = "Level"
	}
	now := s.stamp()
	err = s.tx(ctx, func(tx *sql.Tx) error {
		var n, last int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(MAX(position), 0) FROM club_levels WHERE club_id = $1`, c.ClubID).Scan(&n, &last); err != nil {
			return err
		}
		if n >= MaxClubLevels {
			return ErrLimit
		}
		own, err := clubSkillsIn(ctx, tx, c.ClubID)
		if err != nil {
			return err
		}
		items := []string{}
		for _, t := range track {
			t.Name = clip(strings.TrimSpace(t.Name), skills.MaxCustomNameLength)
			if t.Name == "" {
				continue
			}
			i := slices.IndexFunc(own, func(k ClubSkill) bool { return strings.EqualFold(k.Name, t.Name) })
			switch {
			case i >= 0 && own[i].Skill == "":
				if !slices.Contains(items, own[i].ID) {
					items = append(items, own[i].ID)
				}
				continue
			case i >= 0, len(own) >= MaxClubSkills:
				continue // a name for a catalog skill, or no room
			}
			k := ClubSkill{ID: newID(), Name: t.Name}
			if _, err := tx.ExecContext(ctx, `INSERT INTO club_skills (id, club_id, name, note, skill, by_name, created_at) VALUES ($1, $2, $3, $4, '', $5, $6)`,
				k.ID, c.ClubID, t.Name, clip(strings.TrimSpace(t.Note), MaxClubSkillNote), c.Name, now); err != nil {
				return err
			}
			own = append(own, k)
			items = append(items, k.ID)
			added++
		}
		data, err := json.Marshal(items)
		if err != nil {
			return err
		}
		id = newID()
		if _, err := tx.ExecContext(ctx, `INSERT INTO club_levels (id, club_id, source, name, set_json, club_items, position, by_name, updated_at) VALUES ($1, $2, '', $3, $4, $5, $6, $7, $8)`,
			id, c.ClubID, name, set, string(data), last+1, c.Name, now); err != nil {
			return err
		}
		return touch(ctx, tx, c.ClubID, now)
	})
	return id, added, err
}
