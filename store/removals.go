package store

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
)

// Removing entries (roadmap 2026-10-08): the organiser removes entries, so
// they can't be sent or changed again until restored, or holds them, asking
// for changes, which the club or gymnast makes and sends again for the
// organiser to accept. Either way there's an optional reason for the club,
// the member or both. Removed and held entries are left out of Entries, so
// out of everything counted, printed or timetabled.

// What the organiser has done with an entry.
const (
	Removed = "removed" // taken out, and can't be sent again
	Held    = "held"    // taken out until changed, sent again and accepted
)

// Who sees the reason an entry was removed or held.
const (
	ToClub   = "club"
	ToMember = "member"
	ToBoth   = "both"
)

// ForClub and ForMember say whether the club, or the member (or gymnast
// entering on their own), sees the reason.
func (e Entry) ForClub() bool { return e.RemovalTo == ToClub || e.RemovalTo == ToBoth }
func (e Entry) ForMember() bool {
	return e.RemovalTo == ToMember || e.RemovalTo == ToBoth || e.Individual
}

// RemoveEntries removes (Removed) or holds (Held) a competition's entries,
// with a note for the club, the member or both (to). It returns how many it
// changed.
func (s *Store) RemoveEntries(ctx context.Context, competitionID string, ids []string, removal, note, to string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	args := []any{removal, note, to, competitionID}
	marks := make([]string, len(ids))
	for i, id := range ids {
		args = append(args, id)
		marks[i] = "$" + strconv.Itoa(len(args))
	}
	res, err := s.db.ExecContext(ctx, `UPDATE entries SET removal = $1, removal_note = $2, removal_to = $3, resent = FALSE
		WHERE competition_id = $4 AND id IN (`+strings.Join(marks, ", ")+`)`, args...)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}

// RestoreEntry puts a removed or held entry back in the competition: restored,
// or a held one accepted.
func (s *Store) RestoreEntry(ctx context.Context, competitionID, id string) error {
	return affected(s.db.ExecContext(ctx, `UPDATE entries SET removal = '', removal_note = '', removal_to = '', resent = FALSE
		WHERE competition_id = $1 AND id = $2 AND removal <> ''`, competitionID, id))
}

// RemovedEntries are a competition's removed and held entries, by gymnast.
func (s *Store) RemovedEntries(ctx context.Context, competitionID string) ([]Entry, error) {
	return s.entriesWhere(ctx, `competition_id = $1 AND removal <> ''`, competitionID)
}

// ClubRemovals are a club's entries the organiser of a competition removed or
// holds, by member id and discipline ("<member>/<discipline>").
func (s *Store) ClubRemovals(ctx context.Context, clubID, competitionID string) (map[string]Entry, error) {
	list, err := s.entriesWhere(ctx, `club_id = $1 AND competition_id = $2 AND removal <> ''`, clubID, competitionID)
	if err != nil {
		return nil, err
	}
	out := map[string]Entry{}
	for _, e := range list {
		out[e.MemberID+"/"+e.Entry.Discipline] = e
	}
	return out, nil
}

// MemberRemovals are a member's entries removed or held, by competition id
// and discipline ("<competition>/<discipline>").
func (s *Store) MemberRemovals(ctx context.Context, memberID string) (map[string]Entry, error) {
	list, err := s.entriesWhere(ctx, `member_id = $1 AND removal <> ''`, memberID)
	if err != nil {
		return nil, err
	}
	out := map[string]Entry{}
	for _, e := range list {
		out[e.CompetitionID+"/"+e.Entry.Discipline] = e
	}
	return out, nil
}

// removalOf is the organiser's removal of the competition's copy of a
// member's entry, in a transaction: "", Removed or Held.
func removalOf(ctx context.Context, tx *sql.Tx, competitionID, memberID, discipline string) (string, error) {
	var removal string
	err := tx.QueryRowContext(ctx, `SELECT removal FROM entries WHERE competition_id = $1 AND member_id = $2 AND discipline = $3`,
		competitionID, memberID, discipline).Scan(&removal)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return removal, err
}
