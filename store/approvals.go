package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"time"
)

// Approving coaches (ADR 0007 Decisions 5 to 7): a club sends the coaches
// who sign off to a competition, with their qualifications as they are then;
// the organiser approves each, or not, and can withdraw an approval. A
// coach's sign-off counts at the competition only while they're approved
// (signoffCounts).

// What the organiser has decided about a coach sent to them.
const (
	CoachWaiting   = ""          // not decided yet
	CoachApproved  = "approved"  // they can sign off
	CoachRefused   = "refused"   // not approved, with a note why
	CoachWithdrawn = "withdrawn" // approved, then withdrawn
)

// SentCoach is a club's coach sent to a competition.
type SentCoach struct {
	CompetitionID, CoachID, ClubID string
	ClubName, Name                 string
	// Qualifications are those sent that the club still keeps; Changed says
	// the coach's qualifications have changed since (so send them again).
	Qualifications []CoachQualification
	Changed        bool
	SentAt         time.Time
	Status         string // CoachWaiting, CoachApproved, CoachRefused or CoachWithdrawn
	Note           string // the organiser's, back to the club
	DecidedAt      time.Time
	// AllSignoffs says their sign-offs count whenever they were given, not
	// just since they were last approved.
	AllSignoffs bool
}

// Approved says whether the coach can sign off at the competition.
func (c SentCoach) Approved() bool { return c.Status == CoachApproved }

// qualificationIDs are a coach's qualifications now, as sent: by id, oldest
// first, as JSON.
func qualificationIDs(ctx context.Context, tx *sql.Tx, coachID string) (string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM coach_qualifications WHERE coach_id = $1 ORDER BY uploaded_at, rowid`, coachID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	b, err := json.Marshal(ids)
	return string(b), err
}

// SendCoaches sends a club's coaches to a competition it's entered in: those
// named, each one who signs off, become exactly what it has from the club. A
// coach sent again with different qualifications waits for the organiser
// again; one no longer named is taken back.
func (s *Store) SendCoaches(ctx context.Context, clubID, competitionID string, coachIDs []string) error {
	now := s.stamp()
	return s.tx(ctx, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM club_competitions WHERE club_id = $1 AND competition_id = $2`, clubID, competitionID).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		var sent []string
		for _, id := range coachIDs {
			var name string
			err := tx.QueryRowContext(ctx, `SELECT name FROM coaches WHERE id = $1 AND club_id = $2 AND signs_off`, id, clubID).Scan(&name)
			if err == sql.ErrNoRows {
				continue // not theirs, or doesn't sign off
			} else if err != nil {
				return err
			}
			quals, err := qualificationIDs(ctx, tx, id)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO competition_coaches (competition_id, coach_id, club_id, name, qualifications, sent_at)
				VALUES ($1, $2, $3, $4, $5, $6)
				ON CONFLICT (competition_id, coach_id) DO UPDATE SET name = excluded.name,
					sent_at = CASE WHEN competition_coaches.qualifications = excluded.qualifications THEN competition_coaches.sent_at ELSE excluded.sent_at END,
					status = CASE WHEN competition_coaches.qualifications = excluded.qualifications THEN competition_coaches.status ELSE '' END,
					note = CASE WHEN competition_coaches.qualifications = excluded.qualifications THEN competition_coaches.note ELSE '' END,
					qualifications = excluded.qualifications`,
				competitionID, id, clubID, name, quals, now); err != nil {
				return err
			}
			sent = append(sent, id)
		}
		rows, err := tx.QueryContext(ctx, `SELECT coach_id FROM competition_coaches WHERE competition_id = $1 AND club_id = $2`, competitionID, clubID)
		if err != nil {
			return err
		}
		var gone []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			if !slices.Contains(sent, id) {
				gone = append(gone, id)
			}
		}
		rows.Close()
		for _, id := range gone {
			if _, err := tx.ExecContext(ctx, `DELETE FROM competition_coaches WHERE competition_id = $1 AND coach_id = $2`, competitionID, id); err != nil {
				return err
			}
		}
		return touch(ctx, tx, clubID, now)
	})
}

const sentCoachColumns = `cc.competition_id, cc.coach_id, cc.club_id, COALESCE(cl.name, ''), cc.name, cc.qualifications, cc.sent_at, cc.status, cc.note, COALESCE(cc.decided_at, ''), cc.counts_from`

// sentCoaches reads coaches sent, filling in their qualifications.
func (s *Store) sentCoaches(ctx context.Context, where string, args ...any) ([]SentCoach, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+sentCoachColumns+` FROM competition_coaches cc LEFT JOIN clubs cl ON cl.id = cc.club_id
		WHERE `+where+` ORDER BY cl.name, cc.name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SentCoach
	var sentIDs [][]string
	for rows.Next() {
		var c SentCoach
		var quals, sent, decided, from string
		if err := rows.Scan(&c.CompetitionID, &c.CoachID, &c.ClubID, &c.ClubName, &c.Name, &quals, &sent, &c.Status, &c.Note, &decided, &from); err != nil {
			return nil, err
		}
		var ids []string
		if err := json.Unmarshal([]byte(quals), &ids); err != nil {
			return nil, err
		}
		c.SentAt, c.DecidedAt, c.AllSignoffs = parseTime(sent), parseTime(decided), from == ""
		out = append(out, c)
		sentIDs = append(sentIDs, ids)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		now, err := s.coachQualifications(ctx, out[i].CoachID)
		if err != nil {
			return nil, err
		}
		var nowIDs []string
		for _, q := range now {
			nowIDs = append(nowIDs, q.ID)
			if slices.Contains(sentIDs[i], q.ID) {
				out[i].Qualifications = append(out[i].Qualifications, q)
			}
		}
		out[i].Changed = !slices.Equal(nowIDs, sentIDs[i])
	}
	return out, nil
}

// coachQualifications are a coach's qualifications, oldest first.
func (s *Store) coachQualifications(ctx context.Context, coachID string) ([]CoachQualification, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, coach_id, qualification, cert_type, uploaded_at FROM coach_qualifications
		WHERE coach_id = $1 ORDER BY uploaded_at, rowid`, coachID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CoachQualification
	for rows.Next() {
		var q CoachQualification
		var at string
		if err := rows.Scan(&q.ID, &q.CoachID, &q.Qualification, &q.CertType, &at); err != nil {
			return nil, err
		}
		q.UploadedAt = parseTime(at)
		out = append(out, q)
	}
	return out, rows.Err()
}

// CompetitionCoaches are the coaches sent to a competition, by club and name.
func (s *Store) CompetitionCoaches(ctx context.Context, competitionID string) ([]SentCoach, error) {
	return s.sentCoaches(ctx, `cc.competition_id = $1`, competitionID)
}

// ClubSentCoaches are the coaches a club has sent to a competition, by
// coach id.
func (s *Store) ClubSentCoaches(ctx context.Context, clubID, competitionID string) (map[string]SentCoach, error) {
	list, err := s.sentCoaches(ctx, `cc.club_id = $1 AND cc.competition_id = $2`, clubID, competitionID)
	if err != nil {
		return nil, err
	}
	out := map[string]SentCoach{}
	for _, c := range list {
		out[c.CoachID] = c
	}
	return out, nil
}

// CoachSentTo are the competitions a coach has been sent to, by competition
// id.
func (s *Store) CoachSentTo(ctx context.Context, coachID string) (map[string]SentCoach, error) {
	list, err := s.sentCoaches(ctx, `cc.coach_id = $1`, coachID)
	if err != nil {
		return nil, err
	}
	out := map[string]SentCoach{}
	for _, c := range list {
		out[c.CompetitionID] = c
	}
	return out, nil
}

// DecideCoach records the organiser's decision on a coach sent to their
// competition: CoachApproved, CoachRefused or CoachWithdrawn, with a note.
// Approving a coach again, afresh counts only the sign-offs they give from
// now on; otherwise those they gave before count again.
func (s *Store) DecideCoach(ctx context.Context, competitionID, coachID, status, note string, afresh bool) error {
	now := s.stamp()
	from := `counts_from`
	if status == CoachApproved && afresh {
		from = `$5`
	}
	return affected(s.db.ExecContext(ctx, `UPDATE competition_coaches SET status = $1, note = $2, decided_at = $5, counts_from = `+from+`
		WHERE competition_id = $3 AND coach_id = $4`, status, note, competitionID, coachID, now))
}

// CompetitionCertificate is the certificate of a coach's qualification sent
// to a competition, for its organiser: its media type and bytes.
func (s *Store) CompetitionCertificate(ctx context.Context, competitionID, coachID, id string) (string, []byte, error) {
	var certType string
	var data []byte
	err := s.db.QueryRowContext(ctx, `SELECT q.cert_type, q.certificate FROM coach_qualifications q
		JOIN competition_coaches cc ON cc.coach_id = q.coach_id
		WHERE q.id = $1 AND cc.competition_id = $2 AND cc.coach_id = $3
		AND EXISTS (SELECT 1 FROM json_each(cc.qualifications) WHERE json_each.value = q.id)`, id, competitionID, coachID).Scan(&certType, &data)
	return certType, data, notFound(err)
}

// EntryCoach is the coach an individual names on their entry, at a
// competition that approves coaches (ADR 0007 Decision 8).
type EntryCoach struct {
	EntryID, CompetitionID string
	Gymnast                string
	Name, Qualification    string
	CertType               string
	SentAt                 time.Time
	Status                 string // CoachWaiting, CoachApproved, CoachRefused or CoachWithdrawn
	Note                   string
	DecidedAt              time.Time
	AllSignoffs            bool // as SentCoach's
}

// Approved says whether the coach can sign off the entry.
func (c EntryCoach) Approved() bool { return c.Status == CoachApproved }

// SetEntryCoach names the coach of the individual entry a personal link
// opens, with their qualification and certificate, already checked, until
// the competition's deadline. A coach named again waits for the organiser
// again.
func (s *Store) SetEntryCoach(ctx context.Context, token, name, qualification, certType string, certificate []byte) error {
	now := s.stamp()
	return s.tx(ctx, func(tx *sql.Tx) error {
		var entryID, competitionID string
		if err := tx.QueryRowContext(ctx, `SELECT id, competition_id FROM entries WHERE token_hash = $1 AND individual`, hash(token)).Scan(&entryID, &competitionID); err != nil {
			return notFound(err)
		}
		if err := s.open(ctx, tx, competitionID, signing); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO entry_coaches (entry_id, name, qualification, cert_type, certificate, sent_at) VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (entry_id) DO UPDATE SET name = excluded.name, qualification = excluded.qualification, cert_type = excluded.cert_type,
				certificate = excluded.certificate, sent_at = excluded.sent_at, status = '', note = ''`,
			entryID, name, qualification, certType, certificate, now)
		return err
	})
}

const entryCoachColumns = `ec.entry_id, e.competition_id, e.gymnast, ec.name, ec.qualification, ec.cert_type, ec.sent_at, ec.status, ec.note, COALESCE(ec.decided_at, ''), ec.counts_from`

// entryCoaches reads individuals' coaches.
func (s *Store) entryCoaches(ctx context.Context, where string, args ...any) ([]EntryCoach, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+entryCoachColumns+` FROM entry_coaches ec JOIN entries e ON e.id = ec.entry_id
		WHERE `+where+` ORDER BY e.gymnast, ec.name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EntryCoach
	for rows.Next() {
		var c EntryCoach
		var sent, decided, from string
		if err := rows.Scan(&c.EntryID, &c.CompetitionID, &c.Gymnast, &c.Name, &c.Qualification, &c.CertType, &sent, &c.Status, &c.Note, &decided, &from); err != nil {
			return nil, err
		}
		c.SentAt, c.DecidedAt, c.AllSignoffs = parseTime(sent), parseTime(decided), from == ""
		out = append(out, c)
	}
	return out, rows.Err()
}

// EntryCoachOf is the coach named on an entry.
func (s *Store) EntryCoachOf(ctx context.Context, entryID string) (EntryCoach, error) {
	list, err := s.entryCoaches(ctx, `ec.entry_id = $1`, entryID)
	if err != nil {
		return EntryCoach{}, err
	}
	if len(list) == 0 {
		return EntryCoach{}, ErrNotFound
	}
	return list[0], nil
}

// CompetitionEntryCoaches are the coaches individuals have named for a
// competition, by gymnast.
func (s *Store) CompetitionEntryCoaches(ctx context.Context, competitionID string) ([]EntryCoach, error) {
	return s.entryCoaches(ctx, `e.competition_id = $1`, competitionID)
}

// DecideEntryCoach is DecideCoach for the coach an individual named.
func (s *Store) DecideEntryCoach(ctx context.Context, competitionID, entryID, status, note string, afresh bool) error {
	now := s.stamp()
	from := `counts_from`
	if status == CoachApproved && afresh {
		from = `$5`
	}
	return affected(s.db.ExecContext(ctx, `UPDATE entry_coaches SET status = $1, note = $2, decided_at = $5, counts_from = `+from+`
		WHERE entry_id = $4 AND entry_id IN (SELECT id FROM entries WHERE competition_id = $3)`, status, note, competitionID, entryID, now))
}

// EntryCertificate is the certificate of the coach an individual named, for
// the competition's organiser.
func (s *Store) EntryCertificate(ctx context.Context, competitionID, entryID string) (string, []byte, error) {
	var certType string
	var data []byte
	err := s.db.QueryRowContext(ctx, `SELECT ec.cert_type, ec.certificate FROM entry_coaches ec JOIN entries e ON e.id = ec.entry_id
		WHERE ec.entry_id = $1 AND e.competition_id = $2`, entryID, competitionID).Scan(&certType, &data)
	return certType, data, notFound(err)
}
