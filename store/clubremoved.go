package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"

	"tariffCalculator/competitions"
)

// What a club removes stays with the organiser (migration 56, 2026-11-11):
// when a club removes a member, a coach or itself, the competitions' copies
// of what it sent are kept, marked removed by the club, rather than going or
// quietly reading as withdrawn. An entry removed is out of the competition,
// as a withdrawn one is (Entry.Withdrawn), but listed; a coach removed stays
// on the organiser's Coaches page and their sign-offs keep counting, until
// the organiser asks for the entries they signed to be signed off again
// (AskSignoffAgain).

// sentMatch is the condition, on a row of entries, that its member still
// has the entry: one they haven't withdrawn.
const sentMatch = `EXISTS (SELECT 1 FROM member_entries me
	WHERE me.member_id = entries.member_id AND me.competition_id = entries.competition_id AND me.discipline = entries.discipline)`

// entriesClosed is the condition, on a row of entries, that new entries to
// its competition have closed at $1: it has gone live and the deadline has
// passed (a private or paused competition hasn't closed).
const entriesClosed = `(SELECT c.live_at <> '' AND c.live_at <= $1 AND c.deadline <= $1 FROM competitions c WHERE c.id = entries.competition_id)`

// keepEntries marks the competitions' copies of a club's entries, those
// meeting cond (with its arguments from $3), removed by the club at now by
// who, before the members or club they belong to go, and records it in each
// competition's history. A copy the organiser removed or holds stays as it
// is; with withdrawn false, so does one its member had already withdrawn.
// A synchro pair whose partner (by partnerOf, an id) goes keeps its entry:
// only its history says so.
func keepEntries(ctx context.Context, tx *sql.Tx, now, who, club string, withdrawn bool, cond, partnerOf string, args ...any) error {
	where := `NOT individual AND removal = '' AND club_removed_at = '' AND ` + cond
	if !withdrawn {
		where += ` AND ` + sentMatch
	}
	all := append([]any{now, who}, args...)
	rows, err := tx.QueryContext(ctx, `SELECT competition_id, gymnast, entry, `+entriesClosed+` AND `+sentMatch+` FROM entries WHERE `+where, all...)
	if err != nil {
		return err
	}
	type line struct{ competition, what string }
	var lines []line
	for rows.Next() {
		var competition, gymnast, data string
		var late bool
		if err := rows.Scan(&competition, &gymnast, &data, &late); err != nil {
			rows.Close()
			return err
		}
		var e competitions.Entry
		if err := json.Unmarshal([]byte(data), &e); err != nil {
			rows.Close()
			return err
		}
		what := e.Gymnasts() + " · " + e.Event() + ": removed by the club"
		if late {
			what += ", after entries closed"
		}
		lines = append(lines, line{competition, what})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	// The member's key stays with the copy, so their page still finds it.
	if _, err := tx.ExecContext(ctx, `UPDATE entries SET club_removed_at = $1, club_removed_by = $2,
		club_removed_late = COALESCE(`+entriesClosed+` AND `+sentMatch+`, FALSE),
		person_key = CASE WHEN person_key = '' AND member_id IS NOT NULL THEN '`+KindMember+`:' || member_id ELSE person_key END
		WHERE `+where, all...); err != nil {
		return err
	}
	if partnerOf != "" {
		prows, err := tx.QueryContext(ctx, `SELECT competition_id, entry FROM entries
			WHERE NOT individual AND removal = '' AND club_removed_at = '' AND partner_member IN (`+partnerOf+`)
			AND (member_id IS NULL OR member_id NOT IN (`+partnerOf+`))`, all...)
		if err != nil {
			return err
		}
		for prows.Next() {
			var competition, data string
			if err := prows.Scan(&competition, &data); err != nil {
				prows.Close()
				return err
			}
			var e competitions.Entry
			if err := json.Unmarshal([]byte(data), &e); err != nil {
				prows.Close()
				return err
			}
			partner := "their partner"
			if e.Partner != nil {
				partner = e.Partner.Name
			}
			lines = append(lines, line{competition, fmt.Sprintf("%s · %s: the club removed %s, but the pair's entry is %s's and stays in, as when a partner withdraws",
				e.Gymnasts(), e.Event(), partner, e.Gymnast)})
		}
		prows.Close()
		if err := prows.Err(); err != nil {
			return err
		}
	}
	by := clubBy(who, club)
	for _, l := range lines {
		if err := recordTx(ctx, tx, l.competition, by, l.what, now); err != nil {
			return err
		}
	}
	return nil
}

// keepCoaches marks the coaches meeting cond (on competition_coaches cc,
// with its arguments from $3) removed by the club at now by who, before they
// go: each keeps their row at each competition, with their club's name and
// the qualifications sent, by kind (their certificates go with the club's
// record), and records it in each competition's history.
func keepCoaches(ctx context.Context, tx *sql.Tx, now, who, club, cond string, args ...any) error {
	all := append([]any{now, who}, args...)
	rows, err := tx.QueryContext(ctx, `SELECT cc.competition_id, cc.coach_id, cc.name, cc.qualifications FROM competition_coaches cc
		WHERE cc.removed_at = '' AND `+cond, all...)
	if err != nil {
		return err
	}
	type kept struct{ competition, coach, name, quals string }
	var all2 []kept
	for rows.Next() {
		var k kept
		if err := rows.Scan(&k.competition, &k.coach, &k.name, &k.quals); err != nil {
			rows.Close()
			return err
		}
		all2 = append(all2, k)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, k := range all2 {
		var ids []string
		if err := json.Unmarshal([]byte(k.quals), &ids); err != nil {
			return err
		}
		qrows, err := tx.QueryContext(ctx, `SELECT id, qualification FROM coach_qualifications WHERE coach_id = $1 ORDER BY uploaded_at, rowid`, k.coach)
		if err != nil {
			return err
		}
		kinds := []string{}
		for qrows.Next() {
			var id, q string
			if err := qrows.Scan(&id, &q); err != nil {
				qrows.Close()
				return err
			}
			if slices.Contains(ids, id) {
				kinds = append(kinds, q)
			}
		}
		qrows.Close()
		if err := qrows.Err(); err != nil {
			return err
		}
		b, err := json.Marshal(kinds)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE competition_coaches SET removed_at = $1, removed_by = $2, removed_qualifications = $3,
			club_name = COALESCE((SELECT name FROM clubs WHERE id = competition_coaches.club_id), club_name)
			WHERE competition_id = $4 AND coach_id = $5`, now, who, string(b), k.competition, k.coach); err != nil {
			return err
		}
		if err := recordTx(ctx, tx, k.competition, clubBy(who, club), k.name+": removed as a coach by the club; the sign-offs they gave still count", now); err != nil {
			return err
		}
	}
	return nil
}

// clubBy is who of a club made a change, for a competition's history: the
// link, with the club's name ("UCD (Club admin)"), or the club alone when
// its record expired.
func clubBy(who, club string) string {
	if who == "" {
		return club
	}
	return club + " (" + who + ")"
}

// recordTx is Record, in a transaction.
func recordTx(ctx context.Context, tx *sql.Tx, competitionID, who, what, now string) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO competition_log (competition_id, at, who, what) VALUES ($1, $2, $3, $4)`,
		competitionID, now, who, what); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM competition_log WHERE competition_id = $1 AND id <= (
		SELECT id FROM competition_log WHERE competition_id = $1 ORDER BY id DESC LIMIT 1 OFFSET $2)`, competitionID, MaxHistory)
	return err
}

// keepClubs marks what the clubs meeting cond (on clubs, with its arguments
// from $3) sent removed by the club, before they're deleted: their members'
// entries, withdrawn ones too (they'd otherwise be no club's, and live), and
// their coaches.
func keepClubs(ctx context.Context, tx *sql.Tx, now, who, cond string, args ...any) error {
	all := append([]any{now, who}, args...)
	rows, err := tx.QueryContext(ctx, `SELECT id, name FROM clubs WHERE `+cond, all...)
	if err != nil {
		return err
	}
	type club struct{ id, name string }
	var clubs []club
	for rows.Next() {
		var c club
		if err := rows.Scan(&c.id, &c.name); err != nil {
			rows.Close()
			return err
		}
		clubs = append(clubs, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, c := range clubs {
		if err := keepEntries(ctx, tx, now, who, c.name, true, `club_id = $3`, `SELECT id FROM members WHERE club_id = $3`, c.id); err != nil {
			return err
		}
		if err := keepCoaches(ctx, tx, now, who, c.name, `cc.club_id = $3`, c.id); err != nil {
			return err
		}
	}
	return nil
}

// SignedByCoach are a competition's entries a club's coach signed off that
// are still in it, by club then gymnast: those the organiser can ask to be
// signed off again once the club has removed the coach.
func (s *Store) SignedByCoach(ctx context.Context, competitionID, coachID string) ([]Entry, error) {
	all, err := s.entriesWhere(ctx, `competition_id = $1 AND signed_coach = $2 AND COALESCE(signed_at, '') <> '' AND NOT individual AND removal = ''`, competitionID, coachID)
	if err != nil {
		return nil, err
	}
	var out []Entry
	for _, e := range all {
		if !e.Withdrawn {
			out = append(out, e)
		}
	}
	return out, nil
}

// AskSignoffAgain takes the sign-offs a coach the club removed gave on a
// competition's entries (SignedByCoach), on the competition's copies and
// the members' own, so each waits for another coach's sign-off, with the
// reason shown to the club, the gymnast and their coaches. It returns the
// entries it took them from. A coach the club hasn't removed is
// ErrNotFound.
func (s *Store) AskSignoffAgain(ctx context.Context, competitionID, coachID, reason string) ([]Entry, error) {
	entries, err := s.SignedByCoach(ctx, competitionID, coachID)
	if err != nil {
		return nil, err
	}
	now := s.stamp()
	err = s.tx(ctx, func(tx *sql.Tx) error {
		if err := affected(tx.ExecContext(ctx, `UPDATE competition_coaches SET asked_again_at = $1
			WHERE competition_id = $2 AND coach_id = $3 AND removed_at <> ''`, now, competitionID, coachID)); err != nil {
			return err
		}
		for _, e := range entries {
			if _, err := tx.ExecContext(ctx, `UPDATE entries SET signed_at = NULL, signed_by = '', sign_note = '', signed_coach = '', signoff_again = $1
				WHERE id = $2 AND signed_coach = $3`, reason, e.ID, coachID); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, `UPDATE member_entries SET signed_at = NULL, signed_by = '', sign_note = '', signed_coach = '', signoff_again = $1
			WHERE competition_id = $2 AND signed_coach = $3 AND signed_at IS NOT NULL`, reason, competitionID, coachID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// MemberSent are the competitions' copies of a member's entries still in
// them: what removing the member would mark removed by the club.
func (s *Store) MemberSent(ctx context.Context, memberID string) ([]Entry, error) {
	all, err := s.entriesWhere(ctx, `member_id = $1 AND removal = '' AND club_removed_at = ''`, memberID)
	if err != nil {
		return nil, err
	}
	var out []Entry
	for _, e := range all {
		if !e.Withdrawn {
			out = append(out, e)
		}
	}
	return out, nil
}

// clubName is a club's name, in a transaction.
func clubName(ctx context.Context, tx *sql.Tx, clubID string) (string, error) {
	var name string
	err := tx.QueryRowContext(ctx, `SELECT name FROM clubs WHERE id = $1`, clubID).Scan(&name)
	return name, notFound(err)
}
