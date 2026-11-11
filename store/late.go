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

// Late requests (late changes, roadmap 2026-10-08; late requests,
// 2026-11-11): after the deadline, a club or an individual asks to change an
// entry, or to enter late; the organiser accepts it, applying the change (or
// taking the entry in) and charging its fee, or turns it down with a reason.
// Each carries a thread (migration 54): a message to the organiser of its
// own, which both sides add to before it's decided.

// What the organiser has decided about a late request.
const (
	LateWaiting  = "" // for the organiser
	LateAccepted = "accepted"
	LateRejected = "rejected"
)

// ErrNotAllowed is a late request of a kind the organiser doesn't allow.
var ErrNotAllowed = errors.New("that change isn't allowed after the deadline")

// ErrEntered is asking to enter late with an entry the competition has.
var ErrEntered = errors.New("that entry is in the competition already")

// LateRequest is a request to change an entry after the deadline, or (Kind
// competitions.LateNew) to enter late: then the entry is the competition's
// copy, Asked until it's accepted.
type LateRequest struct {
	ID, CompetitionID, EntryID string
	Kind                       string // competitions.LateNew, LateLevel or LateRoutines
	Entry                      competitions.Entry
	Who                        string // who asked, e.g. "UCD (comp sec)", "Ann Ryan"
	Note                       string // theirs
	At                         time.Time
	Status                     string // LateWaiting, LateAccepted or LateRejected
	Reason                     string // the organiser's
	DecidedAt                  time.Time
	DecidedBy                  string
	Fee                        int // in cents, charged when accepted
	// The coach's sign-off of the change, carried to the entry if accepted.
	SignedAt              time.Time
	SignedBy, SignedCoach string // the coach's name, and id for a club's coach
	SignNote              string
	Thread                string // its thread's concern id; "" for one asked before threads
}

// Open says whether it's still to be decided.
func (r LateRequest) Open() bool { return r.Status == LateWaiting }

// SignedOff says whether a coach has signed it off.
func (r LateRequest) SignedOff() bool { return !r.SignedAt.IsZero() }

// SetLate changes which late requests can be made, and their fees.
func (s *Store) SetLate(ctx context.Context, competitionID string, late competitions.LateChanges) error {
	data, err := json.Marshal(late)
	if err != nil {
		return err
	}
	return affected(s.db.ExecContext(ctx, `UPDATE competitions SET late = $1 WHERE id = $2`, string(data), competitionID))
}

// RequestLateChange asks to change an entry after the deadline, from
// someone (from, whose thread it is), replacing any request for it still
// to be decided and keeping that one's thread.
func (s *Store) RequestLateChange(ctx context.Context, competitionID, entryID string, e competitions.Entry, who, note string, from Sender) (LateRequest, error) {
	c, err := s.Competition(ctx, competitionID)
	if err != nil {
		return LateRequest{}, err
	}
	current, err := s.CompetitionEntry(ctx, competitionID, entryID)
	if err != nil {
		return LateRequest{}, err
	}
	if current.Removal == Removed || current.Removal == Asked {
		return LateRequest{}, ErrRemoved
	}
	kind := competitions.LateKind(current.Entry, e)
	if !c.Late.Allowed(kind) {
		return LateRequest{}, ErrNotAllowed
	}
	var r LateRequest
	err = s.tx(ctx, func(tx *sql.Tx) error {
		var err error
		r, err = s.addLate(ctx, tx, competitionID, entryID, kind, e, who, note, from)
		return err
	})
	return r, err
}

// addLate keeps a request, replacing any for the entry still to be decided
// and taking over its thread, or starting one from from.
func (s *Store) addLate(ctx context.Context, tx *sql.Tx, competitionID, entryID, kind string, e competitions.Entry, who, note string, from Sender) (LateRequest, error) {
	data, err := json.Marshal(e)
	if err != nil {
		return LateRequest{}, err
	}
	r := LateRequest{ID: newID(), CompetitionID: competitionID, EntryID: entryID, Kind: kind, Entry: e, Who: who, Note: note, At: parseTime(s.stamp())}
	if err := tx.QueryRowContext(ctx, `SELECT thread FROM late_requests WHERE competition_id = $1 AND entry_id = $2 AND status = '' AND thread <> ''`,
		competitionID, entryID).Scan(&r.Thread); err != nil && err != sql.ErrNoRows {
		return LateRequest{}, err
	}
	if r.Thread == "" {
		if r.Thread, err = s.newThread(ctx, tx, competitionID, entryID, from, lateText(kind, e, note)); err != nil {
			return LateRequest{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM late_requests WHERE competition_id = $1 AND entry_id = $2 AND status = ''`, competitionID, entryID); err != nil {
		return LateRequest{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO late_requests (id, competition_id, entry_id, kind, entry, who, note, at, status, thread) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, '', $9)`,
		r.ID, competitionID, entryID, kind, string(data), who, note, s.stamp(), r.Thread)
	return r, err
}

// lateText is what a request's thread starts with: what was asked, and
// their note.
func lateText(kind string, e competitions.Entry, note string) string {
	text := competitions.LateKindName(kind) + ": " + e.Gymnasts() + " · " + e.Event()
	if note != "" {
		text += "\n" + note
	}
	return text
}

// newThread starts a request's thread: a message from from, about the
// entry, kept out of the messages and concerns (late).
func (s *Store) newThread(ctx context.Context, tx *sql.Tx, competitionID, entryID string, from Sender, text string) (string, error) {
	id := newID()
	_, err := tx.ExecContext(ctx, `INSERT INTO concerns (id, competition_id, entry_id, who, text, at, role, club, sender, entries, events, late)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, '[]', '[]', TRUE)`, id, competitionID, entryID, from.Name, text, s.stamp(), from.Role, from.Club, from.ID)
	return id, err
}

// LateThread is a request's thread, started from from if it was asked
// before requests had threads.
func (s *Store) LateThread(ctx context.Context, competitionID, id string, from Sender) (string, error) {
	var thread string
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var entryID, kind, entry, note string
		if err := tx.QueryRowContext(ctx, `SELECT thread, entry_id, kind, entry, note FROM late_requests WHERE competition_id = $1 AND id = $2`, competitionID, id).
			Scan(&thread, &entryID, &kind, &entry, &note); err != nil {
			return notFound(err)
		}
		if thread != "" {
			return nil
		}
		var e competitions.Entry
		if err := json.Unmarshal([]byte(entry), &e); err != nil {
			return err
		}
		var err error
		if thread, err = s.newThread(ctx, tx, competitionID, entryID, from, lateText(kind, e, note)); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE late_requests SET thread = $1 WHERE id = $2`, thread, id)
		return err
	})
	return thread, err
}

// AskToEnterLate asks to enter a club's member late, once new entries have
// closed: their saved entry for discipline, or e if given (kept as their
// entry first). The competition's copy is made, Asked, and the request with
// it; asked again while it's still to be decided, the copy and the request
// are replaced. ErrEntered if the competition has the entry already (or
// turned it down).
func (s *Store) AskToEnterLate(ctx context.Context, competitionID, memberID, discipline string, e *competitions.Entry, who, note string, from Sender) (LateRequest, error) {
	c, err := s.Competition(ctx, competitionID)
	if err != nil {
		return LateRequest{}, err
	}
	if !c.Late.Allowed(competitions.LateNew) {
		return LateRequest{}, ErrNotAllowed
	}
	var r LateRequest
	err = s.tx(ctx, func(tx *sql.Tx) error {
		var clubID, clubName, name string
		if err := tx.QueryRowContext(ctx, `SELECT m.club_id, cl.name, m.name FROM members m JOIN clubs cl ON cl.id = m.club_id
			JOIN club_competitions cc ON cc.club_id = m.club_id AND cc.competition_id = $2
			WHERE m.id = $1`, memberID, competitionID).Scan(&clubID, &clubName, &name); err != nil {
			if err == sql.ErrNoRows {
				return ErrNotAttached
			}
			return err
		}
		now := s.stamp()
		if e != nil {
			e.Gymnast = name
			data, err := json.Marshal(e)
			if err != nil {
				return err
			}
			if err := upsertMemberEntry(ctx, tx, memberID, competitionID, *e, data, now); err != nil {
				return err
			}
		}
		var copyID, removal string
		switch err := tx.QueryRowContext(ctx, `SELECT id, removal FROM entries WHERE competition_id = $1 AND member_id = $2 AND discipline = $3`,
			competitionID, memberID, discipline).Scan(&copyID, &removal); {
		case err == sql.ErrNoRows:
		case err != nil:
			return err
		case removal != Asked:
			return ErrEntered
		default:
			var open int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM late_requests WHERE entry_id = $1 AND status = ''`, copyID).Scan(&open); err != nil {
				return err
			}
			if open == 0 {
				return ErrEntered // turned down
			}
		}
		var entry string
		var signedAt, partnerMember, partnerEntry sql.NullString
		var signedBy, signNote, signedCoach string
		var partnerConfirmed bool
		if err := tx.QueryRowContext(ctx, `SELECT entry, signed_at, signed_by, sign_note, signed_coach, partner_confirmed, partner_member, partner_entry
			FROM member_entries WHERE member_id = $1 AND competition_id = $2 AND discipline = $3`, memberID, competitionID, discipline).
			Scan(&entry, &signedAt, &signedBy, &signNote, &signedCoach, &partnerConfirmed, &partnerMember, &partnerEntry); err != nil {
			return notFound(err)
		}
		if copyID == "" {
			copyID = newID()
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO entries (id, competition_id, club_id, member_id, discipline, club_name, individual, gymnast, entry, sent_at,
			signed_at, signed_by, sign_note, partner_confirmed, partner_member, partner_entry, signed_coach, entered_at, removal)
			VALUES ($1, $2, $3, $4, $5, $6, FALSE, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $9, 'asked')
			ON CONFLICT (competition_id, member_id, discipline) DO UPDATE
			SET entry = excluded.entry, gymnast = excluded.gymnast, sent_at = excluded.sent_at, entered_at = excluded.entered_at,
				signed_at = excluded.signed_at, signed_by = excluded.signed_by, sign_note = excluded.sign_note, signed_coach = excluded.signed_coach,
				partner_confirmed = excluded.partner_confirmed, partner_member = excluded.partner_member, partner_entry = excluded.partner_entry`,
			copyID, competitionID, clubID, memberID, discipline, clubName, name, entry, now, signedAt, signedBy, signNote,
			partnerConfirmed, partnerMember, partnerEntry, signedCoach); err != nil {
			return err
		}
		var asked competitions.Entry
		if err := json.Unmarshal([]byte(entry), &asked); err != nil {
			return err
		}
		var err error
		if r, err = s.addLate(ctx, tx, competitionID, copyID, competitions.LateNew, asked, who, note, from); err != nil {
			return err
		}
		if err := full(ctx, tx, competitionID); err != nil {
			return err
		}
		return touch(ctx, tx, clubID, now)
	})
	return r, err
}

// AskToEnterLateOnOwn asks to enter late as an individual, once new entries
// have closed, as AddIndividualEntry enters: the entry is made, Asked, with
// the request. The thread is theirs, as the individual the entry is.
func (s *Store) AskToEnterLateOnOwn(ctx context.Context, competitionID, as string, e competitions.Entry, note string) (Entry, string, LateRequest, error) {
	c, err := s.Competition(ctx, competitionID)
	if err != nil {
		return Entry{}, "", LateRequest{}, err
	}
	if !c.Late.Allowed(competitions.LateNew) {
		return Entry{}, "", LateRequest{}, ErrNotAllowed
	}
	var r LateRequest
	added, token, err := s.addIndividualEntry(ctx, competitionID, as, e, func(tx *sql.Tx, added Entry) error {
		// The sender as a message from their own page names them (web's individualTeller).
		from := Sender{Role: RoleIndividual, Name: e.Gymnast, ID: "individual:" + added.Key()}
		var err error
		r, err = s.addLate(ctx, tx, competitionID, added.ID, competitions.LateNew, e, e.Gymnast, note, from)
		return err
	})
	return added, token, r, err
}

const lateColumns = `id, competition_id, entry_id, kind, entry, who, note, at, status, reason, decided_at, decided_by, fee, signed_at, signed_by, signed_coach, sign_note, thread`

func scanLate(row interface{ Scan(...any) error }) (LateRequest, error) {
	var r LateRequest
	var entry, at, decided, signed string
	if err := row.Scan(&r.ID, &r.CompetitionID, &r.EntryID, &r.Kind, &entry, &r.Who, &r.Note, &at, &r.Status, &r.Reason, &decided, &r.DecidedBy, &r.Fee,
		&signed, &r.SignedBy, &r.SignedCoach, &r.SignNote, &r.Thread); err != nil {
		return LateRequest{}, notFound(err)
	}
	if err := json.Unmarshal([]byte(entry), &r.Entry); err != nil {
		return LateRequest{}, fmt.Errorf("reading late request %s: %w", r.ID, err)
	}
	r.At, r.DecidedAt, r.SignedAt = parseTime(at), parseTime(decided), parseTime(signed)
	return r, nil
}

// LateRequests are a competition's late requests: those still to be
// decided first, oldest first; then the rest, latest first.
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

// LateRequest is one of a competition's late requests.
func (s *Store) LateRequest(ctx context.Context, competitionID, id string) (LateRequest, error) {
	return scanLate(s.db.QueryRowContext(ctx, `SELECT `+lateColumns+` FROM late_requests WHERE competition_id = $1 AND id = $2`, competitionID, id))
}

// LatestLateRequest is the latest request about an entry, if any.
func (s *Store) LatestLateRequest(ctx context.Context, competitionID, entryID string) (LateRequest, bool, error) {
	r, err := scanLate(s.db.QueryRowContext(ctx, `SELECT `+lateColumns+` FROM late_requests WHERE competition_id = $1 AND entry_id = $2
		ORDER BY at DESC, rowid DESC LIMIT 1`, competitionID, entryID))
	if errors.Is(err, ErrNotFound) {
		return LateRequest{}, false, nil
	}
	return r, err == nil, err
}

// LatestLateRequests are the latest request about each of a competition's
// entries that has one (LatestLateRequest), by entry id.
func (s *Store) LatestLateRequests(ctx context.Context, competitionID string) (map[string]LateRequest, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+lateColumns+` FROM late_requests WHERE competition_id = $1 ORDER BY at, rowid`, competitionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]LateRequest{}
	for rows.Next() {
		r, err := scanLate(rows)
		if err != nil {
			return nil, err
		}
		out[r.EntryID] = r // the latest comes last
	}
	return out, rows.Err()
}

// closeThread marks a decided request's thread resolved, with the reason.
func (s *Store) closeThread(ctx context.Context, tx *sql.Tx, id, reason, who, now string) error {
	_, err := tx.ExecContext(ctx, `UPDATE concerns SET resolved_at = $1, resolved_by = $2, resolution = $3
		WHERE id = (SELECT thread FROM late_requests WHERE id = $4) AND resolved_at = ''`, now, who, reason, id)
	return err
}

// RejectLateChange turns a waiting request down, with a reason, which
// closes its thread. A late entry stays out of the competition.
func (s *Store) RejectLateChange(ctx context.Context, competitionID, id, reason, who string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		now := s.stamp()
		if err := affected(tx.ExecContext(ctx, `UPDATE late_requests SET status = $1, reason = $2, decided_at = $3, decided_by = $4
			WHERE competition_id = $5 AND id = $6 AND status = ''`, LateRejected, reason, now, who, competitionID, id)); err != nil {
			return err
		}
		return s.closeThread(ctx, tx, id, reason, who, now)
	})
}

// SignLateRequest records a coach's sign-off of a late request still to be
// decided. coachID is a club's coach's id, "" for an individual's.
func (s *Store) SignLateRequest(ctx context.Context, competitionID, id, coach, coachID, note string) error {
	return affected(s.db.ExecContext(ctx, `UPDATE late_requests SET signed_at = $1, signed_by = $2, signed_coach = $3, sign_note = $4
		WHERE competition_id = $5 AND id = $6 AND status = ''`, s.stamp(), coach, coachID, note, competitionID, id))
}

// AcceptLateChange applies a request waiting for the organiser: the entry
// becomes the one asked for, on the competition's copy and (for a club's
// member) the club's, needing checking again, and signed off as the request
// was (or not); it keeps its place on a waiting list unless its level
// changed; and the fee is charged. A late entry comes into the competition,
// entered now: over its event's limit, it's last on the waiting list. The
// thread closes, and the request comes back as accepted.
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
		if r.Kind == competitions.LateNew {
			if err := affected(tx.ExecContext(ctx, `UPDATE entries SET removal = '', entered_at = $1 WHERE id = $2 AND removal = 'asked'`, now, r.EntryID)); err != nil {
				return err
			}
		}
		var signedAt any // the request's sign-off, carried over; none clears the entry's
		if r.SignedOff() {
			signedAt = formatTime(r.SignedAt)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE entries SET entry = $1, gymnast = $2, sent_at = $3,
			entered_at = CASE WHEN json_extract(entry, '$.level') = json_extract($1, '$.level') THEN entered_at ELSE $3 END,
			checked_at = CASE WHEN entry = $1 THEN checked_at ELSE NULL END,
			video_review = CASE WHEN entry = $1 THEN video_review ELSE '' END,
			signed_at = $6, signed_by = $7, sign_note = $8, signed_coach = $9
			WHERE competition_id = $4 AND id = $5`, string(data), r.Entry.Gymnast, now, competitionID, r.EntryID, signedAt, r.SignedBy, r.SignNote, r.SignedCoach); err != nil {
			return err
		}
		if memberID != "" {
			if _, err := tx.ExecContext(ctx, `UPDATE member_entries SET entry = $1, updated_at = $2,
				signed_at = $6, signed_by = $7, sign_note = $8, signed_coach = $9
				WHERE member_id = $3 AND competition_id = $4 AND discipline = $5`, string(data), now, memberID, competitionID, discipline, signedAt, r.SignedBy, r.SignNote, r.SignedCoach); err != nil {
				return err
			}
		}
		r.Status, r.Reason, r.DecidedBy, r.Fee, r.DecidedAt = LateAccepted, reason, who, fee, parseTime(now)
		if _, err = tx.ExecContext(ctx, `UPDATE late_requests SET status = $1, reason = $2, decided_at = $3, decided_by = $4, fee = $5 WHERE id = $6`,
			LateAccepted, reason, now, who, fee, id); err != nil {
			return err
		}
		return s.closeThread(ctx, tx, id, reason, who, now)
	})
	return r, err
}
