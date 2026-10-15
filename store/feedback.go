package store

import (
	"context"
	"time"
)

// Feedback about the app (roadmap, ready for the dry run): anyone using a
// competition's pages can say what confused them, for the organiser to read
// and the maintainer to hear. It keeps the words, a name or email to reply to
// if given, the role and the page as its route pattern: never a link.

// Feedback is something someone said about the app.
type Feedback struct {
	ID      string
	Text    string
	ReplyTo string // a name or email, if they gave one
	Role    string // e.g. "Member", "Organiser", "Helper link: Mary (checking cards)"
	Page    string // the route pattern of the page, e.g. "/clubs/member/{token}/competitions/{id}"
	At      time.Time
	DoneAt  time.Time // zero until it's marked dealt with
	DoneBy  string
}

// Done says whether it's been dealt with.
func (f Feedback) Done() bool { return !f.DoneAt.IsZero() }

// MaxFeedbackPerDay is how much feedback a competition takes in 24 hours.
const MaxFeedbackPerDay = 200

// SetFeedbackButton shows the Feedback button on the competition's pages, or
// takes it off.
func (s *Store) SetFeedbackButton(ctx context.Context, competitionID string, on bool) error {
	return affected(s.db.ExecContext(ctx, `UPDATE competitions SET feedback = $1 WHERE id = $2`, on, competitionID))
}

// AddFeedback keeps feedback, refusing it with ErrLimit once the
// competition has had MaxFeedbackPerDay in the last 24 hours.
func (s *Store) AddFeedback(ctx context.Context, competitionID string, f Feedback) (Feedback, error) {
	var n int
	since := formatTime(s.now().Add(-24 * time.Hour))
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM feedback WHERE competition_id = $1 AND at > $2`, competitionID, since).Scan(&n); err != nil {
		return Feedback{}, err
	}
	if n >= MaxFeedbackPerDay {
		return Feedback{}, ErrLimit
	}
	at := s.stamp()
	f.ID, f.At, f.DoneAt, f.DoneBy = newID(), parseTime(at), time.Time{}, ""
	_, err := s.db.ExecContext(ctx, `INSERT INTO feedback (id, competition_id, text, reply_to, role, page, at) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		f.ID, competitionID, f.Text, f.ReplyTo, f.Role, f.Page, at)
	return f, err
}

// FeedbackFor is a competition's feedback, newest first.
func (s *Store) FeedbackFor(ctx context.Context, competitionID string) ([]Feedback, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, text, reply_to, role, page, at, done_at, done_by FROM feedback
		WHERE competition_id = $1 ORDER BY at DESC, rowid DESC`, competitionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Feedback
	for rows.Next() {
		var f Feedback
		var at, done string
		if err := rows.Scan(&f.ID, &f.Text, &f.ReplyTo, &f.Role, &f.Page, &at, &done, &f.DoneBy); err != nil {
			return nil, err
		}
		f.At, f.DoneAt = parseTime(at), parseTime(done)
		out = append(out, f)
	}
	return out, rows.Err()
}

// MarkFeedback marks feedback dealt with (done), or not.
func (s *Store) MarkFeedback(ctx context.Context, competitionID, id, who string, done bool) error {
	at := ""
	if done {
		at = s.stamp()
	} else {
		who = ""
	}
	return affected(s.db.ExecContext(ctx, `UPDATE feedback SET done_at = $1, done_by = $2 WHERE competition_id = $3 AND id = $4`, at, who, competitionID, id))
}

// FeedbackWaiting is how much of a competition's feedback isn't dealt with.
func (s *Store) FeedbackWaiting(ctx context.Context, competitionID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM feedback WHERE competition_id = $1 AND done_at = ''`, competitionID).Scan(&n)
	return n, err
}
