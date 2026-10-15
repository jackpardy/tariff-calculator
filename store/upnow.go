package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Who's up now (roadmap item 7): as each gymnast competes, the marshal taps on
// to the next, so the venue screen can say who's on. A flight of n gymnasts
// has 2n positions: everyone's 1st routine in running order (0 to n-1), then
// everyone's 2nd in the same order (n to 2n-1). Flights are identified by
// competitions.FlightKey.

// Up is how far the marshal has got through a flight: the position on now,
// who tapped to it, and when.
type Up struct {
	Position int
	Who      string
	At       time.Time
}

// NotUp is the position of a flight the marshal hasn't tapped through yet.
const NotUp = -1

// ErrUpMoved is a tap from a position the flight is no longer at: a tap sent
// again (on poor wifi, say), or one from a page out of date. Nothing changes.
var ErrUpMoved = errors.New("the flight has moved on since")

// UpNext moves a flight on to the next open position after from, the one the
// marshal saw it at (open[i]: someone competes at position i), or to the first
// open one from NotUp, by who. moved is false, and nothing changes, if there's
// no open position to go to; it's ErrUpMoved if the flight isn't at from.
func (s *Store) UpNext(ctx context.Context, competitionID, flight string, from int, open []bool, who string) (up Up, moved bool, err error) {
	return s.moveUp(ctx, competitionID, flight, from, open, 1, who)
}

// UpBack moves a flight back to the open position before from, the one the
// marshal saw it at, by who. moved is false, and nothing changes, if it's at
// the first open position, or hasn't started; it's ErrUpMoved if the flight
// isn't at from.
func (s *Store) UpBack(ctx context.Context, competitionID, flight string, from int, open []bool, who string) (up Up, moved bool, err error) {
	return s.moveUp(ctx, competitionID, flight, from, open, -1, who)
}

// moveUp moves a flight a step (1 or -1) from the position from to the nearest
// open position, read and written in one transaction, so the same tap sent
// twice moves it only once.
func (s *Store) moveUp(ctx context.Context, competitionID, flight string, from int, open []bool, step int, who string) (up Up, moved bool, err error) {
	err = s.tx(ctx, func(tx *sql.Tx) error {
		var at string
		err := tx.QueryRowContext(ctx, `SELECT position, who, at FROM up_now WHERE competition_id = $1 AND flight = $2`, competitionID, flight).
			Scan(&up.Position, &up.Who, &at)
		started := err == nil
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		up.At = parseTime(at)
		if started && up.Position != from || !started && from != NotUp {
			return ErrUpMoved
		}
		if !started && step < 0 {
			return nil
		}
		i := up.Position + step
		if !started {
			i = 0
		} else if step < 0 {
			i = min(i, len(open)-1) // the flight may have fewer gymnasts since
		}
		for i >= 0 && i < len(open) && !open[i] {
			i += step
		}
		if i < 0 || i >= len(open) {
			return nil
		}
		up, moved = Up{Position: i, Who: who, At: s.now().UTC()}, true
		_, err = tx.ExecContext(ctx, `INSERT INTO up_now (competition_id, flight, position, who, at) VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (competition_id, flight) DO UPDATE SET position = excluded.position, who = excluded.who, at = excluded.at`,
			competitionID, flight, i, who, formatTime(up.At))
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
	rows, err := s.db.QueryContext(ctx, `SELECT flight, position, who, at FROM up_now WHERE competition_id = $1`, competitionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Up{}
	for rows.Next() {
		var key, at string
		var u Up
		if err := rows.Scan(&key, &u.Position, &u.Who, &at); err != nil {
			return nil, err
		}
		u.At = parseTime(at)
		out[key] = u
	}
	return out, rows.Err()
}
