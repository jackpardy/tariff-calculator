package store

import (
	"context"
	"database/sql"
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

// The kinds of scratch warning that can be cleared: a scratched person who is
// still here to officiate, or to compete in another entry.
const (
	ClearOfficiating = "officiating"
	ClearCompeting   = "competing"
)

// ScratchClear is a scratch warning that's been cleared: who said the person
// is still here, and when.
type ScratchClear struct {
	Who string
	At  time.Time
}

// ClearScratch clears a kind of scratch warning (ClearOfficiating or
// ClearCompeting) for a person, by who, for the whole competition. Clearing
// again replaces who and when.
func (s *Store) ClearScratch(ctx context.Context, competitionID, person, kind, who string) error {
	if kind != ClearOfficiating && kind != ClearCompeting {
		return fmt.Errorf("unknown scratch warning %q", kind)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO scratch_clears (competition_id, person, kind, who, at) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (competition_id, person, kind) DO UPDATE SET who = excluded.who, at = excluded.at`,
		competitionID, person, kind, who, s.stamp())
	return err
}

// ScratchClears are a competition's cleared scratch warnings, by person and
// then kind.
func (s *Store) ScratchClears(ctx context.Context, competitionID string) (map[string]map[string]ScratchClear, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT person, kind, who, at FROM scratch_clears WHERE competition_id = $1`, competitionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]map[string]ScratchClear{}
	for rows.Next() {
		var person, kind, at string
		var c ScratchClear
		if err := rows.Scan(&person, &kind, &c.Who, &at); err != nil {
			return nil, err
		}
		c.At = parseTime(at)
		if out[person] == nil {
			out[person] = map[string]ScratchClear{}
		}
		out[person][kind] = c
	}
	return out, rows.Err()
}

// Official check-in statuses on a panel; no row at all means not marked yet.
const (
	OfficialHere    = "here"    // the official has arrived for the panel
	OfficialMissing = "missing" // the official hasn't turned up
)

// SetOfficialCheckin marks a person here or missing for each of these
// flights (by key), by who, in one go; a status of "" takes the mark away.
func (s *Store) SetOfficialCheckin(ctx context.Context, competitionID, person string, flights []string, status, who string) error {
	if status != "" && status != OfficialHere && status != OfficialMissing {
		return fmt.Errorf("unknown official check-in status %q", status)
	}
	return s.tx(ctx, func(tx *sql.Tx) error {
		for _, f := range flights {
			if status == "" {
				if _, err := tx.ExecContext(ctx, `DELETE FROM official_checkins WHERE competition_id = $1 AND person = $2 AND flight = $3`, competitionID, person, f); err != nil {
					return err
				}
				continue
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO official_checkins (competition_id, person, flight, status, who, at) VALUES ($1, $2, $3, $4, $5, $6)
				ON CONFLICT (competition_id, person, flight) DO UPDATE SET status = excluded.status, who = excluded.who, at = excluded.at`,
				competitionID, person, f, status, who, s.stamp()); err != nil {
				return err
			}
		}
		return nil
	})
}

// OfficialCheckins are a competition's panel check-ins, by flight key and
// then person.
func (s *Store) OfficialCheckins(ctx context.Context, competitionID string) (map[string]map[string]Checkin, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT flight, person, status, at, who FROM official_checkins WHERE competition_id = $1`, competitionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]map[string]Checkin{}
	for rows.Next() {
		var flight, person, at string
		var c Checkin
		if err := rows.Scan(&flight, &person, &c.Status, &at, &c.Who); err != nil {
			return nil, err
		}
		c.At = parseTime(at)
		if out[flight] == nil {
			out[flight] = map[string]Checkin{}
		}
		out[flight][person] = c
	}
	return out, rows.Err()
}
