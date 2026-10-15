package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"

	"tariffCalculator/competitions"
)

// flightIDs gives the flights of every timetable saved before flights had
// IDs one each (migration 34): the published timetable's first, then the
// draft's, a draft flight taking the ID of the published flight in the same
// place, or else of the one of its event with the same gymnasts, so the two
// stay one flight. The day's rows, kept by each flight's place
// (competitions.PlaceKey), move to its ID; and who's up, a place in the
// running order, becomes the gymnast and round there.
func flightIDs(ctx context.Context, tx *sql.Tx) error {
	type saved struct{ id, draft, published string }
	rows, err := tx.QueryContext(ctx, `SELECT id, timetable, published_timetable FROM competitions WHERE timetable <> '' OR published_timetable <> ''`)
	if err != nil {
		return err
	}
	var all []saved
	for rows.Next() {
		var c saved
		if err := rows.Scan(&c.id, &c.draft, &c.published); err != nil {
			rows.Close()
			return err
		}
		all = append(all, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, c := range all {
		if err := identifyCompetition(ctx, tx, c.id, c.draft, c.published); err != nil {
			return fmt.Errorf("competition %s: %w", c.id, err)
		}
	}
	_, err = tx.ExecContext(ctx, `ALTER TABLE up_now DROP COLUMN position`)
	return err
}

// identifyCompetition is flightIDs for a competition, its draft and
// published timetables as saved ("" for none).
func identifyCompetition(ctx context.Context, tx *sql.Tx, id, draftJSON, publishedJSON string) error {
	read := func(data string) (*competitions.Schedule, error) {
		if data == "" {
			return nil, nil
		}
		var s competitions.Schedule
		return &s, json.Unmarshal([]byte(data), &s)
	}
	draft, err := read(draftJSON)
	if err != nil {
		return err
	}
	published, err := read(publishedJSON)
	if err != nil {
		return err
	}
	ids := map[string]string{} // place → ID, the published flight's first
	if published != nil {
		published.Identify()
		for _, f := range published.Flights {
			ids[competitions.PlaceKey(f)] = f.ID
		}
	}
	if draft != nil {
		if published != nil {
			same := func(f, g competitions.ScheduledFlight) bool {
				return competitions.PlaceKey(f) == competitions.PlaceKey(g)
			}
			gymnasts := func(f, g competitions.ScheduledFlight) bool {
				a, b := slices.Clone(f.Entries), slices.Clone(g.Entries)
				slices.Sort(a)
				slices.Sort(b)
				return f.Group() == g.Group() && slices.Equal(a, b)
			}
			used := map[string]bool{}
			for _, match := range []func(f, g competitions.ScheduledFlight) bool{same, gymnasts} {
				for i := range draft.Flights {
					f := &draft.Flights[i]
					for _, g := range published.Flights {
						if f.ID == "" && !used[g.ID] && match(*f, g) {
							f.ID, used[g.ID] = g.ID, true
						}
					}
				}
			}
		}
		draft.Identify()
		for _, f := range draft.Flights {
			if _, ok := ids[competitions.PlaceKey(f)]; !ok {
				ids[competitions.PlaceKey(f)] = f.ID
			}
		}
	}
	for column, s := range map[string]*competitions.Schedule{"timetable": draft, "published_timetable": published} {
		if s == nil || len(s.Flights)+len(s.Unplaced) == 0 {
			continue // no flights to identify: left as saved
		}
		b, err := json.Marshal(s)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE competitions SET `+column+` = $1 WHERE id = $2`, string(b), id); err != nil {
			return err
		}
	}
	for place, flight := range ids {
		for _, table := range []string{"flight_times", "official_checkins", "up_now"} {
			if _, err := tx.ExecContext(ctx, `UPDATE `+table+` SET flight = $1 WHERE competition_id = $2 AND flight = $3`, flight, id, place); err != nil {
				return err
			}
		}
	}

	// Who's up: the gymnast and round at the place the marshal had reached,
	// in the flight on the day (the published one, or else the draft's).
	byID := map[string]competitions.ScheduledFlight{}
	for _, s := range []*competitions.Schedule{draft, published} {
		if s != nil {
			for _, f := range s.Flights {
				byID[f.ID] = f
			}
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT flight, position FROM up_now WHERE competition_id = $1`, id)
	if err != nil {
		return err
	}
	at := map[string]int{}
	for rows.Next() {
		var flight string
		var pos int
		if err := rows.Scan(&flight, &pos); err != nil {
			rows.Close()
			return err
		}
		at[flight] = pos
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for flight, pos := range at {
		n := len(byID[flight].Entries)
		if n == 0 || pos < 0 || pos >= 2*n {
			// A flight gone, or a place it no longer has: as if not tapped.
			if _, err := tx.ExecContext(ctx, `DELETE FROM up_now WHERE competition_id = $1 AND flight = $2`, id, flight); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE up_now SET entry = $1, round = $2 WHERE competition_id = $3 AND flight = $4`,
			byID[flight].Entries[pos%n], pos/n+1, id, flight); err != nil {
			return err
		}
	}
	return nil
}
