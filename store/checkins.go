package store

import (
	"context"
	"fmt"
	"time"
)

// Check-in and scratches on the day (roadmap 2026-10-09): whether each entry's
// gymnast turned up, as marked by the marshal or chair, and who said so.

// Check-in statuses; no row at all means not marked yet.
const (
	CheckedIn = "here"      // the gymnast has arrived
	Scratched = "scratched" // a no-show, or withdrawn on the day
)

// Checkin is an entry's check-in: its status (CheckedIn or Scratched), who
// marked it, and when.
type Checkin struct {
	Status, Who string
	At          time.Time
}

// SetCheckin marks an entry here or scratched, by who; a status of "" takes
// the mark away.
func (s *Store) SetCheckin(ctx context.Context, competitionID, entryID, status, who string) error {
	switch status {
	case "":
		_, err := s.db.ExecContext(ctx, `DELETE FROM checkins WHERE competition_id = $1 AND entry_id = $2`, competitionID, entryID)
		return err
	case CheckedIn, Scratched:
		_, err := s.db.ExecContext(ctx, `INSERT INTO checkins (competition_id, entry_id, status, at, who) VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (competition_id, entry_id) DO UPDATE SET status = excluded.status, at = excluded.at, who = excluded.who`,
			competitionID, entryID, status, s.stamp(), who)
		return err
	}
	return fmt.Errorf("unknown check-in status %q", status)
}

// Checkins are a competition's check-ins, by entry id.
func (s *Store) Checkins(ctx context.Context, competitionID string) (map[string]Checkin, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT entry_id, status, at, who FROM checkins WHERE competition_id = $1`, competitionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Checkin{}
	for rows.Next() {
		var id, at string
		var c Checkin
		if err := rows.Scan(&id, &c.Status, &at, &c.Who); err != nil {
			return nil, err
		}
		c.At = parseTime(at)
		out[id] = c
	}
	return out, rows.Err()
}
