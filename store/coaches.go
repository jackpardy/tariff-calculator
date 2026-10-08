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
	SignsOff  bool // the club lets them sign off routines (ADR 0007 Decision 3)
	CreatedAt time.Time
}

// CreateCoach adds a coach to a club and returns them with their link. The
// name must already be checked.
func (s *Store) CreateCoach(ctx context.Context, clubID, name string) (Coach, string, error) {
	now := s.stamp()
	c := Coach{ID: newID(), ClubID: clubID, Name: name, SignsOff: true, CreatedAt: parseTime(now)}
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
	rows, err := s.db.QueryContext(ctx, `SELECT id, club_id, name, signs_off, created_at FROM coaches WHERE club_id = $1 ORDER BY name, created_at`, clubID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Coach
	for rows.Next() {
		var c Coach
		var created string
		if err := rows.Scan(&c.ID, &c.ClubID, &c.Name, &c.SignsOff, &created); err != nil {
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
		err := tx.QueryRowContext(ctx, `SELECT id, club_id, name, signs_off, created_at FROM coaches WHERE token_hash = $1`, hash(token)).
			Scan(&c.ID, &c.ClubID, &c.Name, &c.SignsOff, &created)
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
// and comp sec. The coach must see the member and be one the club lets sign
// off. Changing the entry clears it; sending it takes the sign-off to the
// competition with it.
func (s *Store) SignOff(ctx context.Context, c Coach, memberID, competitionID, discipline string, signed bool, note string) error {
	var at any
	if signed {
		at = s.stamp()
	}
	return affected(s.db.ExecContext(ctx, fmt.Sprintf(`UPDATE member_entries SET signed_at = $3, signed_by = $4, sign_note = $5
		WHERE member_id = $6 AND competition_id = $7 AND discipline = $8 AND member_id IN (SELECT m.id FROM members m WHERE %s)
		AND (SELECT signs_off FROM coaches WHERE id = $1)`, coachSees),
		c.ID, c.ClubID, at, c.Name, note, memberID, competitionID, discipline))
}

// SetCoachSignsOff says whether one of a club's coaches signs off routines.
func (s *Store) SetCoachSignsOff(ctx context.Context, clubID, coachID string, on bool) error {
	return affected(s.db.ExecContext(ctx, `UPDATE coaches SET signs_off = $1 WHERE id = $2 AND club_id = $3`, on, coachID, clubID))
}

// CoachQualification is one of a coach's qualifications, with its
// certificate (ADR 0007 Decision 4), kept until the comp sec removes it, or
// the coach or club goes.
type CoachQualification struct {
	ID            string
	CoachID       string
	Qualification string // its key in competitions.Qualifications
	CertType      string // the certificate's media type, e.g. "application/pdf"
	UploadedAt    time.Time
}

// MaxQualifications is the most qualifications a coach can have.
const MaxQualifications = 4

// AddQualification gives one of a club's coaches a qualification, with its
// certificate, already checked, of a type certType.
func (s *Store) AddQualification(ctx context.Context, clubID, coachID, qualification, certType string, certificate []byte) (CoachQualification, error) {
	now := s.stamp()
	q := CoachQualification{ID: newID(), CoachID: coachID, Qualification: qualification, CertType: certType, UploadedAt: parseTime(now)}
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM coach_qualifications q JOIN coaches c ON c.id = q.coach_id WHERE c.id = $1 AND c.club_id = $2`, coachID, clubID).Scan(&n); err != nil {
			return err
		}
		if n >= MaxQualifications {
			return ErrLimit
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO coach_qualifications (id, coach_id, qualification, cert_type, certificate, uploaded_at)
			SELECT $1, id, $3, $4, $5, $6 FROM coaches WHERE id = $2 AND club_id = $7`, q.ID, coachID, qualification, certType, certificate, now, clubID)
		if err := affected(res, err); err != nil {
			return err
		}
		return touch(ctx, tx, clubID, now)
	})
	return q, err
}

// Qualifications are a club's coaches' qualifications, by coach id, in the
// order they were added.
func (s *Store) Qualifications(ctx context.Context, clubID string) (map[string][]CoachQualification, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT q.id, q.coach_id, q.qualification, q.cert_type, q.uploaded_at
		FROM coach_qualifications q JOIN coaches c ON c.id = q.coach_id WHERE c.club_id = $1 ORDER BY q.uploaded_at, q.rowid`, clubID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]CoachQualification{}
	for rows.Next() {
		var q CoachQualification
		var at string
		if err := rows.Scan(&q.ID, &q.CoachID, &q.Qualification, &q.CertType, &at); err != nil {
			return nil, err
		}
		q.UploadedAt = parseTime(at)
		out[q.CoachID] = append(out[q.CoachID], q)
	}
	return out, rows.Err()
}

// RemoveQualification removes one of a club's coaches' qualifications, and
// its certificate.
func (s *Store) RemoveQualification(ctx context.Context, clubID, id string) error {
	return affected(s.db.ExecContext(ctx, `DELETE FROM coach_qualifications
		WHERE id = $1 AND coach_id IN (SELECT id FROM coaches WHERE club_id = $2)`, id, clubID))
}

// Certificate is the certificate of one of a club's coaches' qualifications:
// its media type and bytes.
func (s *Store) Certificate(ctx context.Context, clubID, id string) (string, []byte, error) {
	var certType string
	var data []byte
	err := s.db.QueryRowContext(ctx, `SELECT q.cert_type, q.certificate FROM coach_qualifications q JOIN coaches c ON c.id = q.coach_id
		WHERE q.id = $1 AND c.club_id = $2`, id, clubID).Scan(&certType, &data)
	return certType, data, notFound(err)
}

// CoachesSeeAll says whether every coach of a club sees every member.
func (s *Store) CoachesSeeAll(ctx context.Context, clubID string) (bool, error) {
	var on bool
	err := s.db.QueryRowContext(ctx, `SELECT coaches_see_all FROM clubs WHERE id = $1`, clubID).Scan(&on)
	return on, notFound(err)
}
