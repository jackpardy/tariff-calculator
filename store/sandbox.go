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

// The sandbox (ADR 0016): a second instance of the app, filled by a seed
// with made-up clubs and two competitions. Everything a seed makes belongs
// to a copy, so a reset deletes exactly that; the competition on the day
// starts its day again when its days are over.

// What a sandbox copy holds is in sandbox_items, by kind: "competition",
// "club", "individual" and "feedback" (what the seed sent, on the
// maintainer's page).

// SandboxCopy is one seed's copy: when it was made, when it expires (zero
// for the shared copy, which lasts until the next reset), how big it is (its
// clubs), and its links by role, as the seed gave them (JSON).
type SandboxCopy struct {
	ID        string
	CreatedAt time.Time
	ExpiresAt time.Time
	Size      int
	Links     json.RawMessage
}

// Clocked is the store reading the time from now, on the same database: to
// replay a day's taps at the times they'd have been made, while the app goes
// on at its own time.
func (s *Store) Clocked(now func() time.Time) *Store {
	c := *s
	c.now = func() time.Time { return now().UTC() }
	return &c
}

// SaveCopy keeps a seed's copy: the competitions and clubs it made (by id),
// with every individual entered in those competitions and every piece of
// feedback sent from them, so far as the seed's own. It returns the copy's
// id.
func (s *Store) SaveCopy(ctx context.Context, c SandboxCopy, competitionIDs, clubIDs []string) (string, error) {
	id := newID()
	err := s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_copies (id, created_at, expires_at, size, links) VALUES ($1, $2, $3, $4, $5)`,
			id, formatTime(c.CreatedAt), formatTime(c.ExpiresAt), c.Size, string(c.Links)); err != nil {
			return err
		}
		add := func(kind string, ids []string) error {
			for _, item := range ids {
				if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_items (copy_id, kind, id) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, id, kind, item); err != nil {
					return err
				}
			}
			return nil
		}
		if err := add("competition", competitionIDs); err != nil {
			return err
		}
		if err := add("club", clubIDs); err != nil {
			return err
		}
		comps := `SELECT id FROM sandbox_items WHERE copy_id = $1 AND kind = 'competition'`
		for _, q := range []string{
			`INSERT INTO sandbox_items (copy_id, kind, id) SELECT DISTINCT $1, 'individual', individual_id FROM entries
				WHERE individual_id IS NOT NULL AND competition_id IN (` + comps + `) ON CONFLICT DO NOTHING`,
			`INSERT INTO sandbox_items (copy_id, kind, id) SELECT $1, 'feedback', id FROM site_feedback
				WHERE competition_id IN (` + comps + `) ON CONFLICT DO NOTHING`,
		} {
			if _, err := tx.ExecContext(ctx, q, id); err != nil {
				return err
			}
		}
		return nil
	})
	return id, err
}

// SandboxCopies are the copies kept, newest first.
func (s *Store) SandboxCopies(ctx context.Context) ([]SandboxCopy, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, created_at, expires_at, size, links FROM sandbox_copies ORDER BY created_at DESC, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SandboxCopy
	for rows.Next() {
		var c SandboxCopy
		var created, expires, links string
		if err := rows.Scan(&c.ID, &created, &expires, &c.Size, &links); err != nil {
			return nil, err
		}
		c.CreatedAt, c.ExpiresAt, c.Links = parseTime(created), parseTime(expires), json.RawMessage(links)
		out = append(out, c)
	}
	return out, rows.Err()
}

// SharedCopy is the shared copy, the newest one that doesn't expire;
// ErrNotFound if there's none yet.
func (s *Store) SharedCopy(ctx context.Context) (SandboxCopy, error) {
	all, err := s.SandboxCopies(ctx)
	if err != nil {
		return SandboxCopy{}, err
	}
	for _, c := range all {
		if c.ExpiresAt.IsZero() {
			return c, nil
		}
	}
	return SandboxCopy{}, ErrNotFound
}

// DeleteCopy deletes a copy's competitions, clubs and individuals, with
// everything they hold, the individuals entered only in its competitions,
// and the feedback its seed sent, in one transaction, as DeleteExpired
// deletes: anything outside the copy is untouched.
func (s *Store) DeleteCopy(ctx context.Context, id string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sandbox_copies WHERE id = $1`, id).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		items := `SELECT id FROM sandbox_items WHERE copy_id = $1 AND kind = `
		// Someone who entered one of its competitions on their own, while
		// it was there, and nothing else: theirs went with it.
		if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_items (copy_id, kind, id)
			SELECT DISTINCT $1, 'individual', e.individual_id FROM entries e
			WHERE e.individual_id IS NOT NULL AND e.competition_id IN (`+items+`'competition')
			AND NOT EXISTS (SELECT 1 FROM entries o WHERE o.individual_id = e.individual_id AND o.competition_id NOT IN (`+items+`'competition'))
			ON CONFLICT DO NOTHING`, id); err != nil {
			return err
		}
		for _, q := range []string{
			`DELETE FROM competitions WHERE id IN (` + items + `'competition')`,
			`DELETE FROM clubs WHERE id IN (` + items + `'club')`,
			`DELETE FROM individuals WHERE id IN (` + items + `'individual')`,
			`DELETE FROM site_feedback WHERE id IN (` + items + `'feedback')`,
		} {
			if _, err := tx.ExecContext(ctx, q, id); err != nil {
				return err
			}
		}
		if err := dropLeftovers(ctx, tx); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM sandbox_copies WHERE id = $1`, id)
		return err
	})
}

// dayMarks are the tables holding what was marked on a competition's day:
// flights started and finished, check-ins and scratches, panels'
// check-ins, who's up, desk messages and screen notices.
var dayMarks = []string{"flight_times", "checkins", "scratch_clears", "official_checkins", "up_now", "desk_messages", "screen_notices"}

// RestartDay starts a competition's days again from day (a date): its
// date, and the deadlines and times that go with it, move by as many days,
// and what was marked on the day is cleared, in one transaction. Its links,
// entries, timetable and messages stay.
func (s *Store) RestartDay(ctx context.Context, competitionID string, day time.Time) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		var date string
		var times [4]string
		if err := tx.QueryRowContext(ctx, `SELECT date, deadline, live_at, changes_until, signoffs_until FROM competitions WHERE id = $1`, competitionID).
			Scan(&date, &times[0], &times[1], &times[2], &times[3]); err != nil {
			return notFound(err)
		}
		was, err := time.Parse(competitions.DateLayout, date)
		if err != nil {
			return fmt.Errorf("the competition's date %q: %w", date, err)
		}
		to := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
		days := int(to.Sub(was).Hours() / 24)
		for i, v := range times {
			if v != "" {
				times[i] = formatTime(parseTime(v).AddDate(0, 0, days))
			}
		}
		moved := competitions.Competition{Date: to.Format(competitions.DateLayout)}
		if _, err := tx.ExecContext(ctx, `UPDATE competitions SET date = $1, deadline = $2, live_at = $3, changes_until = $4, signoffs_until = $5, delete_after = $6 WHERE id = $7`,
			moved.Date, times[0], times[1], times[2], times[3], moved.DeleteAfter().Format(timeLayout), competitionID); err != nil {
			return err
		}
		for _, table := range dayMarks {
			if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE competition_id = $1`, competitionID); err != nil {
				return err
			}
		}
		return nil
	})
}

// settingSandboxReset is the setting keeping the last reset: when, and how
// long it took.
const settingSandboxReset = "sandbox_reset"

// sandboxReset is the last reset, as kept.
type sandboxReset struct {
	At      string  `json:"at"`
	Seconds float64 `json:"seconds"`
}

// SandboxReset is when the sandbox was last reset and how long it took;
// zero if it never has been.
func (s *Store) SandboxReset(ctx context.Context) (at time.Time, took time.Duration, err error) {
	var value string
	switch err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = $1`, settingSandboxReset).Scan(&value); {
	case errors.Is(err, sql.ErrNoRows):
		return time.Time{}, 0, nil
	case err != nil:
		return time.Time{}, 0, err
	}
	var r sandboxReset
	if err := json.Unmarshal([]byte(value), &r); err != nil {
		return time.Time{}, 0, err
	}
	return parseTime(r.At), time.Duration(r.Seconds * float64(time.Second)), nil
}

// SetSandboxReset keeps when the sandbox was last reset, and how long it
// took.
func (s *Store) SetSandboxReset(ctx context.Context, at time.Time, took time.Duration) error {
	value, err := json.Marshal(sandboxReset{At: formatTime(at), Seconds: took.Seconds()})
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES ($1, $2) ON CONFLICT (key) DO UPDATE SET value = excluded.value`,
		settingSandboxReset, string(value))
	return err
}
