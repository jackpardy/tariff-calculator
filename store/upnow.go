package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"tariffCalculator/competitions"
)

// Who's up now (roadmap item 7): how far the marshal has got through each
// flight on the day, as the routine on now (competitions.Routine: the
// gymnast, and their 1st or 2nd). Which routine comes next is
// competitions.UpOrder's to say; this keeps where each flight is. Flights are
// identified by competitions.FlightKey.

// Up is how far the marshal has got through a flight: the routine on now,
// who tapped to it, and when.
type Up struct {
	competitions.Routine
	Who string
	At  time.Time
}

// ErrUpMoved is a tap from a routine the flight is no longer at: a tap sent
// again (on poor wifi, say), or one from a page out of date. Nothing changes.
var ErrUpMoved = errors.New("the flight has moved on since")

// MoveUp moves a flight on (or back) from the routine from, the one the
// marshal saw it at (the zero Routine before the first tap), to the routine
// move gives (competitions.UpOrder's Next or Back), by who; read and written
// in one transaction, so the same tap sent twice moves it only once. moved is
// false, and nothing changes, if move has nowhere to go; it's ErrUpMoved if
// the flight isn't at from.
func (s *Store) MoveUp(ctx context.Context, competitionID, flight string, from competitions.Routine, move func(competitions.Routine) (competitions.Routine, bool), who string) (up Up, moved bool, err error) {
	return s.MoveUpAt(ctx, competitionID, flight, from, move, who, time.Time{})
}

// MoveUpAt is MoveUp with when the marshal tapped: at, for a tap kept on a
// phone without signal and sent later; now if at is zero.
func (s *Store) MoveUpAt(ctx context.Context, competitionID, flight string, from competitions.Routine, move func(competitions.Routine) (competitions.Routine, bool), who string, at time.Time) (up Up, moved bool, err error) {
	if at.IsZero() {
		at = s.now()
	}
	err = s.tx(ctx, func(tx *sql.Tx) error {
		var was string
		err := tx.QueryRowContext(ctx, `SELECT entry, round, who, at FROM up_now WHERE competition_id = $1 AND flight = $2`, competitionID, flight).
			Scan(&up.Entry, &up.Round, &up.Who, &was)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		up.At = parseTime(was)
		if up.Routine != from {
			return ErrUpMoved
		}
		to, ok := move(from)
		if !ok {
			return nil
		}
		up, moved = Up{Routine: to, Who: who, At: at.UTC()}, true
		_, err = tx.ExecContext(ctx, `INSERT INTO up_now (competition_id, flight, entry, round, who, at) VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (competition_id, flight) DO UPDATE SET entry = excluded.entry, round = excluded.round, who = excluded.who, at = excluded.at`,
			competitionID, flight, to.Entry, to.Round, who, formatTime(up.At))
		return err
	})
	if err != nil {
		return Up{}, false, err
	}
	return up, moved, nil
}

// ClearUp forgets how far a flight has got, as if the marshal hadn't started.
func (s *Store) ClearUp(ctx context.Context, competitionID, flight string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM up_now WHERE competition_id = $1 AND flight = $2`, competitionID, flight)
	return err
}

// UpNow is how far the marshal has got through each of a competition's
// flights, by flight key; a flight not started has none.
func (s *Store) UpNow(ctx context.Context, competitionID string) (map[string]Up, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT flight, entry, round, who, at FROM up_now WHERE competition_id = $1`, competitionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Up{}
	for rows.Next() {
		var key, at string
		var u Up
		if err := rows.Scan(&key, &u.Entry, &u.Round, &u.Who, &at); err != nil {
			return nil, err
		}
		u.At = parseTime(at)
		out[key] = u
	}
	return out, rows.Err()
}
