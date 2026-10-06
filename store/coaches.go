package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Coaches sign off their members' routines (ADR 0004 Decision 11). The comp
// sec adds them to the club; each has a personal link, stored as its hash. A
// member's coach is chosen by the member or the comp sec. A coach sees their
// own members and those without a coach, or every member when the club says so.

// Coach is one of a club's coaches.
type Coach struct {
	ID        string
	ClubID    string
	Name      string
	CreatedAt time.Time
}

// CreateCoach adds a coach to a club and returns them with their link. The
// name must already be checked.
func (s *Store) CreateCoach(ctx context.Context, clubID, name string) (Coach, string, error) {
	now := s.stamp()
	c := Coach{ID: newID(), ClubID: clubID, Name: name, CreatedAt: parseTime(now)}
	token := newToken()
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM coaches WHERE club_id = $1`, clubID).Scan(&n); err != nil {
			return err
		}
		if n >= MaxCoaches {
			return ErrLimit
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO coaches (id, club_id, token_hash, name, created_at) VALUES ($1, $2, $3, $4, $5)`,
			c.ID, clubID, hash(token), name, now); err != nil {
			return err
		}
		return touch(ctx, tx, clubID, now)
	})
	if err != nil {
		return Coach{}, "", err
	}
	return c, token, nil
}

// MaxCoaches is the most coaches a club can have.
const MaxCoaches = 50

// Coaches are a club's coaches, by name.
func (s *Store) Coaches(ctx context.Context, clubID string) ([]Coach, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, club_id, name, created_at FROM coaches WHERE club_id = $1 ORDER BY name, created_at`, clubID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Coach
	for rows.Next() {
		var c Coach
		var created string
		if err := rows.Scan(&c.ID, &c.ClubID, &c.Name, &created); err != nil {
			return nil, err
		}
		c.CreatedAt = parseTime(created)
		out = append(out, c)
	}
	return out, rows.Err()
}

// CoachByLink is the coach a link belongs to. It marks their club used.
func (s *Store) CoachByLink(ctx context.Context, token string) (Coach, error) {
	var c Coach
	var created string
	err := s.tx(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `SELECT id, club_id, name, created_at FROM coaches WHERE token_hash = $1`, hash(token)).
			Scan(&c.ID, &c.ClubID, &c.Name, &created)
		if err != nil {
			return notFound(err)
		}
		return touch(ctx, tx, c.ClubID, s.stamp())
	})
	c.CreatedAt = parseTime(created)
	return c, err
}

// ReplaceCoachLink gives a coach a new link, which ends the old one.
func (s *Store) ReplaceCoachLink(ctx context.Context, clubID, coachID string) (string, error) {
	token := newToken()
	return token, affected(s.db.ExecContext(ctx, `UPDATE coaches SET token_hash = $1 WHERE id = $2 AND club_id = $3`, hash(token), coachID, clubID))
}

// RemoveCoach removes a coach. Their members have no coach until one is
// chosen; sign-offs they gave stay.
func (s *Store) RemoveCoach(ctx context.Context, clubID, coachID string) error {
	return affected(s.db.ExecContext(ctx, `DELETE FROM coaches WHERE id = $1 AND club_id = $2`, coachID, clubID))
}

// SetCoachesSeeAll says whether every coach sees every member of the club.
func (s *Store) SetCoachesSeeAll(ctx context.Context, clubID string, on bool) error {
	return affected(s.db.ExecContext(ctx, `UPDATE clubs SET coaches_see_all = $1 WHERE id = $2`, on, clubID))
}

// SetMemberCoach chooses a member's coach, one of their club's ("" for none).
func (s *Store) SetMemberCoach(ctx context.Context, clubID, memberID, coachID string) error {
	if coachID == "" {
		return affected(s.db.ExecContext(ctx, `UPDATE members SET coach_id = NULL WHERE id = $1 AND club_id = $2`, memberID, clubID))
	}
	return affected(s.db.ExecContext(ctx, `UPDATE members SET coach_id = $1
		WHERE id = $2 AND club_id = $3 AND EXISTS (SELECT 1 FROM coaches WHERE id = $1 AND club_id = $3)`, coachID, memberID, clubID))
}

// coachSees is the condition, on members m and their club, for the members a
// coach ($1, of club $2) sees.
const coachSees = `m.club_id = $2 AND (m.coach_id = $1 OR m.coach_id IS NULL
	OR (SELECT coaches_see_all FROM clubs WHERE id = $2))`

// CoachEntries are the entries a coach sees: their members', those of members
// without a coach, or every member's if the club says so.
func (s *Store) CoachEntries(ctx context.Context, c Coach) ([]MemberEntry, error) {
	return s.memberEntries(ctx, coachSees, c.ID, c.ClubID)
}

// SignOff records a coach's sign-off of a member's entry for a competition
// (signed false: not yet), under the coach's name, with a note for the member
// and comp sec. The coach must see the member. Changing the entry clears it;
// sending it takes the sign-off to the competition with it.
func (s *Store) SignOff(ctx context.Context, c Coach, memberID, competitionID string, signed bool, note string) error {
	var at any
	if signed {
		at = s.stamp()
	}
	return affected(s.db.ExecContext(ctx, fmt.Sprintf(`UPDATE member_entries SET signed_at = $3, signed_by = $4, sign_note = $5
		WHERE member_id = $6 AND competition_id = $7 AND member_id IN (SELECT m.id FROM members m WHERE %s)`, coachSees),
		c.ID, c.ClubID, at, c.Name, note, memberID, competitionID))
}

// CoachesSeeAll says whether every coach of a club sees every member.
func (s *Store) CoachesSeeAll(ctx context.Context, clubID string) (bool, error) {
	var on bool
	err := s.db.QueryRowContext(ctx, `SELECT coaches_see_all FROM clubs WHERE id = $1`, clubID).Scan(&on)
	return on, notFound(err)
}
