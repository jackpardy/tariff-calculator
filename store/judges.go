package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Judging qualifications (ADR 0014): a judge's qualifications, each with its
// certificate, kept like a coach's (ADR 0007 Decisions 4 and 9). A club
// member's are kept with them, for every competition their club enters, and
// count towards the club's room for certificates; an individual's are kept
// with their entry (for the same gymnast, whichever of their entries at the
// competition) and count towards the competition's. The organiser sees
// them, with the certificates, beside the judge's offer.

// JudgeQualification is one of a judge's qualifications.
type JudgeQualification struct {
	ID            string
	Qualification string // its key in competitions.JudgeQualifications
	CertType      string
	UploadedAt    time.Time
}

const judgeQualificationColumns = `q.id, q.qualification, q.cert_type, q.uploaded_at`

// judgeQualifications runs a query for judgeQualificationColumns.
func (s *Store) judgeQualifications(ctx context.Context, query string, args ...any) ([]JudgeQualification, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []JudgeQualification
	for rows.Next() {
		var q JudgeQualification
		var at string
		if err := rows.Scan(&q.ID, &q.Qualification, &q.CertType, &at); err != nil {
			return nil, err
		}
		q.UploadedAt = parseTime(at)
		out = append(out, q)
	}
	return out, rows.Err()
}

// clubCertificateBytes are what a club's certificates take: its coaches' and
// its members' judging ones.
func clubCertificateBytes(ctx context.Context, tx *sql.Tx, clubID string) (int64, error) {
	var used int64
	err := tx.QueryRowContext(ctx, `SELECT
		COALESCE((SELECT SUM(length(q.certificate)) FROM coach_qualifications q JOIN coaches c ON c.id = q.coach_id WHERE c.club_id = $1), 0) +
		COALESCE((SELECT SUM(length(q.certificate)) FROM judge_qualifications q JOIN members m ON m.id = q.member_id WHERE m.club_id = $1), 0)`, clubID).Scan(&used)
	return used, err
}

// competitionCertificateBytes are what a competition's individuals'
// certificates take, their coaches' (but entry except's, being replaced) and
// their judging ones.
func competitionCertificateBytes(ctx context.Context, tx *sql.Tx, competitionID, except string) (int64, error) {
	var used int64
	err := tx.QueryRowContext(ctx, `SELECT
		COALESCE((SELECT SUM(length(ec.certificate)) FROM entry_coaches ec JOIN entries e ON e.id = ec.entry_id WHERE e.competition_id = $1 AND ec.entry_id <> $2), 0) +
		COALESCE((SELECT SUM(length(q.certificate)) FROM judge_qualifications q JOIN entries e ON e.id = q.entry_id WHERE e.competition_id = $1), 0)`, competitionID, except).Scan(&used)
	return used, err
}

// MemberJudgeQualifications are a member's judging qualifications, in the
// order they were added.
func (s *Store) MemberJudgeQualifications(ctx context.Context, memberID string) ([]JudgeQualification, error) {
	return s.judgeQualifications(ctx, `SELECT `+judgeQualificationColumns+` FROM judge_qualifications q WHERE q.member_id = $1 ORDER BY q.uploaded_at, q.rowid`, memberID)
}

// AddMemberJudgeQualification gives a club's member a judging
// qualification, with its certificate, already checked, of a type certType.
func (s *Store) AddMemberJudgeQualification(ctx context.Context, clubID, memberID, qualification, certType string, certificate []byte) error {
	now := s.stamp()
	return s.tx(ctx, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM judge_qualifications WHERE member_id = $1`, memberID).Scan(&n); err != nil {
			return err
		}
		if n >= MaxQualifications {
			return ErrLimit
		}
		used, err := clubCertificateBytes(ctx, tx, clubID)
		if err != nil {
			return err
		}
		if err := s.roomFor(certificate, used, MaxClubCertificateBytes); err != nil {
			return err
		}
		if err := affected(tx.ExecContext(ctx, `INSERT INTO judge_qualifications (id, member_id, qualification, cert_type, certificate, uploaded_at)
			SELECT $1, id, $3, $4, $5, $6 FROM members WHERE id = $2 AND club_id = $7`, newID(), memberID, qualification, certType, certificate, now, clubID)); err != nil {
			return err
		}
		return touch(ctx, tx, clubID, now)
	})
}

// RemoveMemberJudgeQualification removes one of a member's judging
// qualifications, and its certificate.
func (s *Store) RemoveMemberJudgeQualification(ctx context.Context, memberID, id string) error {
	return affected(s.db.ExecContext(ctx, `DELETE FROM judge_qualifications WHERE id = $1 AND member_id = $2`, id, memberID))
}

// MemberJudgeCertificate is the certificate of one of a member's judging
// qualifications: its media type and bytes.
func (s *Store) MemberJudgeCertificate(ctx context.Context, memberID, id string) (string, []byte, error) {
	var certType string
	var data []byte
	err := s.db.QueryRowContext(ctx, `SELECT cert_type, certificate FROM judge_qualifications WHERE id = $1 AND member_id = $2`, id, memberID).Scan(&certType, &data)
	return certType, data, notFound(err)
}

// EntryJudgeQualifications are the judging qualifications of the gymnast of
// an individual's entry, given on it or another of theirs at the
// competition.
func (s *Store) EntryJudgeQualifications(ctx context.Context, entryID string) ([]JudgeQualification, error) {
	return s.judgeQualifications(ctx, `SELECT `+judgeQualificationColumns+` FROM judge_qualifications q
		WHERE q.entry_id IN (`+sameGymnast+`) ORDER BY q.uploaded_at, q.rowid`, entryID)
}

// AddEntryJudgeQualification gives the gymnast of the entry a link (token)
// opens a judging qualification, with its certificate.
func (s *Store) AddEntryJudgeQualification(ctx context.Context, token, qualification, certType string, certificate []byte) error {
	now := s.stamp()
	return s.tx(ctx, func(tx *sql.Tx) error {
		entryID, err := individualEntryID(ctx, tx, token)
		if err != nil {
			return err
		}
		var competitionID string
		if err := tx.QueryRowContext(ctx, `SELECT competition_id FROM entries WHERE id = $1`, entryID).Scan(&competitionID); err != nil {
			return notFound(err)
		}
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM judge_qualifications WHERE entry_id IN (`+sameGymnast+`)`, entryID).Scan(&n); err != nil {
			return err
		}
		if n >= MaxQualifications {
			return ErrLimit
		}
		used, err := competitionCertificateBytes(ctx, tx, competitionID, "")
		if err != nil {
			return err
		}
		if err := s.roomFor(certificate, used, MaxCompetitionCertificateBytes); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO judge_qualifications (id, entry_id, qualification, cert_type, certificate, uploaded_at) VALUES ($1, $2, $3, $4, $5, $6)`,
			newID(), entryID, qualification, certType, certificate, now)
		return err
	})
}

// RemoveEntryJudgeQualification removes one of the judging qualifications of
// the gymnast of the entry a link opens.
func (s *Store) RemoveEntryJudgeQualification(ctx context.Context, token, id string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		entryID, err := individualEntryID(ctx, tx, token)
		if err != nil {
			return err
		}
		return affected(tx.ExecContext(ctx, `DELETE FROM judge_qualifications WHERE id = $2 AND entry_id IN (`+sameGymnast+`)`, entryID, id))
	})
}

// EntryJudgeCertificate is the certificate of one of the judging
// qualifications of the gymnast of an entry.
func (s *Store) EntryJudgeCertificate(ctx context.Context, entryID, id string) (string, []byte, error) {
	var certType string
	var data []byte
	err := s.db.QueryRowContext(ctx, `SELECT cert_type, certificate FROM judge_qualifications WHERE id = $2 AND entry_id IN (`+sameGymnast+`)`, entryID, id).Scan(&certType, &data)
	return certType, data, notFound(err)
}

// OfficialsJudgeQualifications are the judging qualifications of a
// competition's officials, by official id: a club's member's, or an
// individual's.
func (s *Store) OfficialsJudgeQualifications(ctx context.Context, competitionID string) (map[string][]JudgeQualification, error) {
	officials, err := s.Officials(ctx, competitionID)
	if err != nil {
		return nil, err
	}
	out := map[string][]JudgeQualification{}
	for _, o := range officials {
		var qs []JudgeQualification
		switch {
		case o.MemberID != "":
			qs, err = s.MemberJudgeQualifications(ctx, o.MemberID)
		case o.EntryID != "":
			qs, err = s.EntryJudgeQualifications(ctx, o.EntryID)
		}
		if err != nil {
			return nil, err
		}
		if len(qs) > 0 {
			out[o.ID] = qs
		}
	}
	return out, nil
}

// OfficialJudgeCertificate is the certificate of one of the judging
// qualifications of one of a competition's officials.
func (s *Store) OfficialJudgeCertificate(ctx context.Context, competitionID, officialID, id string) (string, []byte, error) {
	var memberID, entryID string
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(member_id, ''), COALESCE(entry_id, '') FROM officials WHERE competition_id = $1 AND id = $2`,
		competitionID, officialID).Scan(&memberID, &entryID)
	switch {
	case err != nil:
		return "", nil, notFound(err)
	case memberID != "":
		return s.MemberJudgeCertificate(ctx, memberID, id)
	case entryID != "":
		return s.EntryJudgeCertificate(ctx, entryID, id)
	}
	return "", nil, ErrNotFound
}

// MemberOfficial says whether a member's offer at a competition has reached
// the organiser (their club sent it), and whether the organiser marked them
// qualified.
func (s *Store) MemberOfficial(ctx context.Context, memberID, competitionID string) (sent, qualified bool, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT qualified FROM officials WHERE member_id = $1 AND competition_id = $2`, memberID, competitionID).Scan(&qualified)
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, nil
	}
	return err == nil, qualified, err
}

// EntryOfficial says whether an individual has an offer at the competition
// of their entry (on it or another of theirs there), and whether the
// organiser marked them qualified.
func (s *Store) EntryOfficial(ctx context.Context, entryID string) (made, qualified bool, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT qualified FROM officials WHERE entry_id IN (`+sameGymnast+`) LIMIT 1`, entryID).Scan(&qualified)
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, nil
	}
	return err == nil, qualified, err
}
