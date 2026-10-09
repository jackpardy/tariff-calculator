package store

import (
	"context"
	"fmt"
	"time"
)

// Flights started and finished on the day (roadmap 2026-10-08): when each
// published flight really started and finished, and who said so. Flights are
// identified by competitions.FlightKey.

// FlightTime is when a flight started and finished (zero for not yet), and
// who last marked it.
type FlightTime struct {
	Started, Finished time.Time
	Who               string
}

// MarkFlight records that a flight started or finished now ("start" or
// "finish"), or undoes the latest thing recorded ("undo": the finish, else the
// start), by who.
func (s *Store) MarkFlight(ctx context.Context, competitionID, flight, what, who string) error {
	switch what {
	case "start":
		_, err := s.db.ExecContext(ctx, `INSERT INTO flight_times (competition_id, flight, started_at, who) VALUES ($1, $2, $3, $4)
			ON CONFLICT (competition_id, flight) DO UPDATE SET started_at = excluded.started_at, who = excluded.who`,
			competitionID, flight, s.stamp(), who)
		return err
	case "finish":
		_, err := s.db.ExecContext(ctx, `INSERT INTO flight_times (competition_id, flight, finished_at, who) VALUES ($1, $2, $3, $4)
			ON CONFLICT (competition_id, flight) DO UPDATE SET finished_at = excluded.finished_at, who = excluded.who`,
			competitionID, flight, s.stamp(), who)
		return err
	case "undo":
		_, err := s.db.ExecContext(ctx, `UPDATE flight_times SET
			started_at = CASE WHEN finished_at = '' THEN '' ELSE started_at END,
			finished_at = '',
			who = $3
			WHERE competition_id = $1 AND flight = $2`, competitionID, flight, who)
		return err
	}
	return fmt.Errorf("unknown flight mark %q", what)
}

// FlightTimes are the times recorded for a competition's flights, by flight
// key.
func (s *Store) FlightTimes(ctx context.Context, competitionID string) (map[string]FlightTime, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT flight, started_at, finished_at, who FROM flight_times WHERE competition_id = $1`, competitionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]FlightTime{}
	for rows.Next() {
		var key, started, finished string
		var t FlightTime
		if err := rows.Scan(&key, &started, &finished, &t.Who); err != nil {
			return nil, err
		}
		t.Started, t.Finished = parseTime(started), parseTime(finished)
		out[key] = t
	}
	return out, rows.Err()
}
