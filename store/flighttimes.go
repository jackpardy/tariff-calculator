package store

import (
	"context"
	"fmt"
	"time"
)

// Flights started and finished on the day (roadmap 2026-10-08): when each
// published flight really started, its warm-up was over (roadmap 15) and it
// finished, and who said so. Flights are
// identified by competitions.FlightKey.

// FlightTime is when a flight started, its warm-up was over, and it finished
// (zero for not yet, or for a Warm-up over the marshal skipped), and who last
// marked it.
type FlightTime struct {
	Started, WarmUpOver, Finished time.Time
	Who                           string
}

// MarkFlight records that a flight started, its warm-up was over or it
// finished now ("start", "warm-up" or "finish"), or takes one back
// ("undo-finish"; "undo-warm-up" while it isn't finished; "undo-start" while
// it has neither), by who. An undo that finds nothing to take back, as when
// the same tap is sent twice, changes nothing.
func (s *Store) MarkFlight(ctx context.Context, competitionID, flight, what, who string) error {
	return s.MarkFlightAt(ctx, competitionID, flight, what, who, time.Time{})
}

// MarkFlightAt is MarkFlight with when it started or finished: at, for a tap
// kept on a phone without signal and sent later; now if at is zero.
func (s *Store) MarkFlightAt(ctx context.Context, competitionID, flight, what, who string, at time.Time) error {
	switch what {
	case "start":
		_, err := s.db.ExecContext(ctx, `INSERT INTO flight_times (competition_id, flight, started_at, who) VALUES ($1, $2, $3, $4)
			ON CONFLICT (competition_id, flight) DO UPDATE SET started_at = excluded.started_at, who = excluded.who`,
			competitionID, flight, s.stampAt(at), who)
		return err
	case "warm-up":
		_, err := s.db.ExecContext(ctx, `INSERT INTO flight_times (competition_id, flight, warm_up_over_at, who) VALUES ($1, $2, $3, $4)
			ON CONFLICT (competition_id, flight) DO UPDATE SET warm_up_over_at = excluded.warm_up_over_at, who = excluded.who`,
			competitionID, flight, s.stampAt(at), who)
		return err
	case "finish":
		_, err := s.db.ExecContext(ctx, `INSERT INTO flight_times (competition_id, flight, finished_at, who) VALUES ($1, $2, $3, $4)
			ON CONFLICT (competition_id, flight) DO UPDATE SET finished_at = excluded.finished_at, who = excluded.who`,
			competitionID, flight, s.stampAt(at), who)
		return err
	case "undo-finish":
		_, err := s.db.ExecContext(ctx, `UPDATE flight_times SET finished_at = '', who = $3
			WHERE competition_id = $1 AND flight = $2 AND finished_at != ''`, competitionID, flight, who)
		return err
	case "undo-warm-up":
		_, err := s.db.ExecContext(ctx, `UPDATE flight_times SET warm_up_over_at = '', who = $3
			WHERE competition_id = $1 AND flight = $2 AND warm_up_over_at != '' AND finished_at = ''`, competitionID, flight, who)
		return err
	case "undo-start":
		_, err := s.db.ExecContext(ctx, `UPDATE flight_times SET started_at = '', who = $3
			WHERE competition_id = $1 AND flight = $2 AND started_at != '' AND warm_up_over_at = '' AND finished_at = ''`, competitionID, flight, who)
		return err
	}
	return fmt.Errorf("unknown flight mark %q", what)
}

// SetFlightTimes puts right when a flight started, its warm-up was over and
// it finished, by who: for when the paper sheet was used, or a tap was wrong.
// A zero time is not yet (or, for the warm-up, not tapped).
func (s *Store) SetFlightTimes(ctx context.Context, competitionID, flight string, started, warmUpOver, finished time.Time, who string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO flight_times (competition_id, flight, started_at, warm_up_over_at, finished_at, who) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (competition_id, flight) DO UPDATE SET started_at = excluded.started_at, warm_up_over_at = excluded.warm_up_over_at,
			finished_at = excluded.finished_at, who = excluded.who`,
		competitionID, flight, formatTime(started), formatTime(warmUpOver), formatTime(finished), who)
	return err
}

// FlightTimes are the times recorded for a competition's flights, by flight
// key.
func (s *Store) FlightTimes(ctx context.Context, competitionID string) (map[string]FlightTime, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT flight, started_at, warm_up_over_at, finished_at, who FROM flight_times WHERE competition_id = $1`, competitionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]FlightTime{}
	for rows.Next() {
		var key, started, warmUpOver, finished string
		var t FlightTime
		if err := rows.Scan(&key, &started, &warmUpOver, &finished, &t.Who); err != nil {
			return nil, err
		}
		t.Started, t.WarmUpOver, t.Finished = parseTime(started), parseTime(warmUpOver), parseTime(finished)
		out[key] = t
	}
	return out, rows.Err()
}
