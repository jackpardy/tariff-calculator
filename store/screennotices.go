package store

import (
	"context"
	"time"
)

// Announcements on the venue screen (roadmap item 7): a line from the
// organisers' desk across the top of every venue screen, until a time or
// until someone takes it down.

// ScreenNotice is an announcement on the venue screen.
type ScreenNotice struct {
	ID    string
	Text  string
	Until time.Time // when it comes down; zero for when someone takes it down
	Who   string    // the link that put it up
	At    time.Time
}

// MaxScreenNotices is how many announcements a competition keeps, shown or
// not; the oldest go first.
const MaxScreenNotices = 50

// ShowOnScreen puts an announcement on the venue screen until a time (zero:
// until it's taken down), keeping the latest MaxScreenNotices.
func (s *Store) ShowOnScreen(ctx context.Context, competitionID, text string, until time.Time, who string) error {
	var untilText string
	if !until.IsZero() {
		untilText = formatTime(until)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO screen_notices (id, competition_id, text, until, who, at) VALUES ($1, $2, $3, $4, $5, $6)`,
		newID(), competitionID, text, untilText, who, s.stamp()); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM screen_notices WHERE competition_id = $1 AND rowid NOT IN (
		SELECT rowid FROM screen_notices WHERE competition_id = $1 ORDER BY at DESC, rowid DESC LIMIT $2)`, competitionID, MaxScreenNotices)
	return err
}

// ScreenNotices are a competition's announcements showing now, latest first.
func (s *Store) ScreenNotices(ctx context.Context, competitionID string) ([]ScreenNotice, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, text, until, who, at FROM screen_notices WHERE competition_id = $1 ORDER BY at DESC, rowid DESC`, competitionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	now := s.now()
	var out []ScreenNotice
	for rows.Next() {
		var n ScreenNotice
		var until, at string
		if err := rows.Scan(&n.ID, &n.Text, &until, &n.Who, &at); err != nil {
			return nil, err
		}
		if until != "" {
			if n.Until = parseTime(until); !n.Until.After(now) {
				continue
			}
		}
		n.At = parseTime(at)
		out = append(out, n)
	}
	return out, rows.Err()
}

// TakeDownNotice takes an announcement off the venue screen.
func (s *Store) TakeDownNotice(ctx context.Context, competitionID, id string) error {
	return affected(s.db.ExecContext(ctx, `DELETE FROM screen_notices WHERE id = $1 AND competition_id = $2`, id, competitionID))
}
