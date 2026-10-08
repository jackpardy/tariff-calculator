package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"tariffCalculator/competitions"
)

// Late changes (roadmap 2026-10-08): after the deadline, a club or an
// individual asks to change an entry; the organiser accepts it, applying
// the change and charging its fee, or rejects it with a reason.

// What the organiser has decided about a late change.
const (
	LateWaiting  = ""
	LateAccepted = "accepted"
	LateRejected = "rejected"
)

// ErrNotAllowed is a late change of a kind the organiser doesn't allow.
var ErrNotAllowed = errors.New("that change isn't allowed after the deadline")

// LateRequest is a request to change an entry after the deadline.
type LateRequest struct {
	ID, CompetitionID, EntryID string
	Kind                       string // competitions.LateLevel or LateRoutines
	Entry                      competitions.Entry
	Who                        string // who asked, e.g. "UCD (comp sec)", "Ann Ryan"
	Note                       string // theirs
	At                         time.Time
	Status                     string // LateWaiting, LateAccepted or LateRejected
	Reason                     string // the organiser's
	DecidedAt                  time.Time
	DecidedBy                  string
	Fee                        int // in cents, charged when accepted
}

// SetLate changes which late changes can be asked for, and their fees.
func (s *Store) SetLate(ctx context.Context, competitionID string, late competitions.LateChanges) error {
	data, err := json.Marshal(late)
	if err != nil {
		return err
	}
	return affected(s.db.ExecContext(ctx, `UPDATE competitions SET late = $1 WHERE id = $2`, string(data), competitionID))
}

// RequestLateChange asks to change an entry after the deadline, replacing
// any request for it still waiting.
func (s *Store) RequestLateChange(ctx context.Context, competitionID, entryID string, e competitions.Entry, who, note string) (LateRequest, error) {
	c, err := s.Competition(ctx, competitionID)
	if err != nil {
		return LateRequest{}, err
	}
	current, err := s.CompetitionEntry(ctx, competitionID, entryID)
	if err != nil {
		return LateRequest{}, err
	}
	if current.Removal == Removed {
		return LateRequest{}, ErrRemoved
	}
	kind := competitions.LateKind(current.Entry, e)
	if !c.Late.Allowed(kind) {
		return LateRequest{}, ErrNotAllowed
	}
	data, err := json.Marshal(e)
	if err != nil {
		return LateRequest{}, err
	}
	r := LateRequest{ID: newID(), CompetitionID: competitionID, EntryID: entryID, Kind: kind, Entry: e, Who: who, Note: note, At: parseTime(s.stamp())}
	err = s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM late_requests WHERE competition_id = $1 AND entry_id = $2 AND status = ''`, competitionID, entryID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO late_requests (id, competition_id, entry_id, kind, entry, who, note, at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			r.ID, competitionID, entryID, kind, string(data), who, note, s.stamp())
		return err
	})
	return r, err
}

const lateColumns = `id, competition_id, entry_id, kind, entry, who, note, at, status, reason, decided_at, decided_by, fee`

func scanLate(row interface{ Scan(...any) error }) (LateRequest, error) {
	var r LateRequest
	var entry, at, decided string
	if err := row.Scan(&r.ID, &r.CompetitionID, &r.EntryID, &r.Kind, &entry, &r.Who, &r.Note, &at, &r.Status, &r.Reason, &decided, &r.DecidedBy, &r.Fee); err != nil {
		return LateRequest{}, notFound(err)
	}
	if err := json.Unmarshal([]byte(entry), &r.Entry); err != nil {
		return LateRequest{}, fmt.Errorf("reading late request %s: %w", r.ID, err)
	}
	r.At, r.DecidedAt = parseTime(at), parseTime(decided)
	return r, nil
}

// LateRequests are a competition's late change requests: waiting ones
// first, oldest first; then the rest, latest first.
func (s *Store) LateRequests(ctx context.Context, competitionID string) ([]LateRequest, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+lateColumns+` FROM late_requests WHERE competition_id = $1
		ORDER BY status <> '', CASE WHEN status = '' THEN at END, decided_at DESC, rowid`, competitionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LateRequest
	for rows.Next() {
		r, err := scanLate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// LatestLateRequest is the latest request to change an entry, if any.
func (s *Store) LatestLateRequest(ctx context.Context, competitionID, entryID string) (LateRequest, bool, error) {
	r, err := scanLate(s.db.QueryRowContext(ctx, `SELECT `+lateColumns+` FROM late_requests WHERE competition_id = $1 AND entry_id = $2
		ORDER BY at DESC, rowid DESC LIMIT 1`, competitionID, entryID))
	if errors.Is(err, ErrNotFound) {
		return LateRequest{}, false, nil
	}
	return r, err == nil, err
}

// RejectLateChange turns a waiting request down, with a reason.
func (s *Store) RejectLateChange(ctx context.Context, competitionID, id, reason, who string) error {
	return affected(s.db.ExecContext(ctx, `UPDATE late_requests SET status = $1, reason = $2, decided_at = $3, decided_by = $4
		WHERE competition_id = $5 AND id = $6 AND status = ''`, LateRejected, reason, s.stamp(), who, competitionID, id))
}

// AcceptLateChange applies a waiting request: the entry becomes the one
// asked for, on the competition's copy and (for a club's member) the club's,
// needing checking and signing off again as any changed entry does; it keeps
// its place on a waiting list unless its level changed; and the fee is
// charged. The request comes back as accepted.
func (s *Store) AcceptLateChange(ctx context.Context, competitionID, id, reason, who string, fee int) (LateRequest, error) {
	var r LateRequest
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var err error
		if r, err = scanLate(tx.QueryRowContext(ctx, `SELECT `+lateColumns+` FROM late_requests WHERE competition_id = $1 AND id = $2 AND status = ''`, competitionID, id)); err != nil {
			return err
		}
		data, err := json.Marshal(r.Entry)
		if err != nil {
			return err
		}
		now := s.stamp()
		var memberID, discipline string
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(member_id, ''), discipline FROM entries WHERE competition_id = $1 AND id = $2`, competitionID, r.EntryID).
			Scan(&memberID, &discipline); err != nil {
			return notFound(err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE entries SET entry = $1, gymnast = $2, sent_at = $3,
			entered_at = CASE WHEN json_extract(entry, '$.level') = json_extract($1, '$.level') THEN entered_at ELSE $3 END,
			checked_at = CASE WHEN entry = $1 THEN checked_at ELSE NULL END,
			video_review = CASE WHEN entry = $1 THEN video_review ELSE '' END,
			signed_at = CASE WHEN entry = $1 THEN signed_at ELSE NULL END
			WHERE competition_id = $4 AND id = $5`, string(data), r.Entry.Gymnast, now, competitionID, r.EntryID); err != nil {
			return err
		}
		if memberID != "" {
			if _, err := tx.ExecContext(ctx, `UPDATE member_entries SET entry = $1, updated_at = $2,
				signed_at = CASE WHEN entry = $1 THEN signed_at ELSE NULL END
				WHERE member_id = $3 AND competition_id = $4 AND discipline = $5`, string(data), now, memberID, competitionID, discipline); err != nil {
				return err
			}
		}
		r.Status, r.Reason, r.DecidedBy, r.Fee, r.DecidedAt = LateAccepted, reason, who, fee, parseTime(now)
		_, err = tx.ExecContext(ctx, `UPDATE late_requests SET status = $1, reason = $2, decided_at = $3, decided_by = $4, fee = $5 WHERE id = $6`,
			LateAccepted, reason, now, who, fee, id)
		return err
	})
	return r, err
}
